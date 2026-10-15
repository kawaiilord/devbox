import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/catalog_models.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/player/catalog_resolver.dart';

final session = Session(
  accessToken: 'a',
  refreshToken: 'r',
  tokenType: 'bearer',
  expiresIn: 900,
  user: const AppUser(
    id: 'viewer',
    displayName: 'Viewer',
    email: 'viewer@example.test',
    emailVerified: true,
  ),
);
CatalogFilm film(
  String source, {
  String title = '测试电影',
  String year = '2026',
  String kind = 'movie',
  List<String> episodes = const ['正片'],
}) => CatalogFilm.fromJson({
  'id': '1',
  'source_id': source,
  'source_name': source,
  'title': title,
  'year': year,
  'kind': kind,
  'group_key': '$title-$year',
  'groups': [
    {
      'id': 'main',
      'name': '主线',
      'episodes': [
        for (var i = 0; i < episodes.length; i++)
          {'id': '$source-$i', 'name': episodes[i]},
      ],
    },
  ],
});

class ResolverApi extends ApiClient {
  final slow = Completer<CatalogSearch>();
  final fast = Completer<CatalogSearch>();
  final requests = <String>[];
  @override
  Future<List<CatalogSource>> catalogSources(Session session) async => [
    for (final id in ['fast', 'slow', 'live'])
      CatalogSource.fromJson({
        'id': id,
        'name': id,
        'kind': id == 'live' ? 'm3u' : 'cms',
      }),
  ];
  @override
  Future<CatalogSearch> searchCatalog(
    Session session,
    String sourceId,
    String query, {
    int page = 1,
  }) {
    requests.add(sourceId);
    return sourceId == 'fast' ? fast.future : slow.future;
  }

  @override
  Future<CatalogFilm> catalogDetail(
    Session session,
    CatalogFilm candidate,
  ) async => candidate;
}

CatalogSearch results(String source) => CatalogSearch.fromJson({
  'items': [
    {
      'id': '1',
      'source_id': source,
      'source_name': source,
      'title': '测试电影',
      'year': '2026',
      'kind': 'movie',
      'group_key': 'a',
      'groups': [
        {
          'id': 'main',
          'name': '主线',
          'episodes': [
            {'id': '1', 'name': '正片'},
          ],
        },
      ],
    },
  ],
  'has_more': false,
});
void main() {
  test(
    'identity matching rejects remakes, other seasons and different kinds',
    () {
      const target = WatchTarget(title: '测试电影', year: '2026', kind: 'movie');
      expect(target.match(film('a')), 2);
      expect(target.match(film('a', title: ' 测试电影【4K】 ')), 2);
      expect(target.match(film('a', year: '1999')), 0);
      expect(target.match(film('a', title: '测试电影2')), 0);
      expect(target.match(film('a', kind: 'tv')), 0);
      expect(target.match(film('a', year: '')), 1);
      const season = WatchTarget(title: '流星 第二季', kind: 'tv');
      expect(season.match(film('a', title: '流星 第一季', kind: 'tv')), 0);
    },
  );
  test('source switching maps episode labels and refuses missing episodes', () {
    final a = film('a', episodes: ['第01集', '第02集', '第03集']).groups.single;
    final b = film('b', episodes: ['预告', '第2集', '第1集']).groups.single;
    expect(matchingEpisode(a, 1, b), 1);
    expect(matchingEpisode(a, 2, b), -1);
    expect(
      matchingEpisode(a, 0, film('c', episodes: ['E01']).groups.single),
      0,
    );
    expect(
      matchingEpisode(a, 0, film('c', episodes: ['S02E01']).groups.single),
      -1,
    );
    expect(
      matchingEpisode(
        film('a').groups.single,
        0,
        film('b', episodes: ['1080P']).groups.single,
      ),
      0,
    );
  });
  test('first candidate becomes playable before slow sources finish, without rooms', () async {
    final api = ResolverApi();
    final resolver = CatalogResolver(
      api,
      session,
      const WatchTarget(title: '测试电影', year: '2026', kind: 'movie'),
    );
    addTearDown(api.close);
    addTearDown(resolver.dispose);
    final done = resolver.resolve();
    await Future<void>.delayed(Duration.zero);
    api.fast.complete(results('fast'));
    await Future<void>.delayed(Duration.zero);
    expect(resolver.loading, true);
    expect(resolver.lines.single.film.sourceId, 'fast');
    expect(api.requests, ['fast', 'slow']);
    api.slow.complete(results('slow'));
    await done;
    expect(resolver.loading, false);
    expect(resolver.lines.length, 2);
  });
  test('a known short drama loads by ID without catalog search', () async {
    final api = ResolverApi();
    final resolver = CatalogResolver(
      api,
      session,
      const WatchTarget(title: '短剧', kind: 'short'),
    );
    addTearDown(api.close);
    addTearDown(resolver.dispose);
    await resolver.resolve(
      seeds: [film('guoguo', title: '短剧', kind: 'short')],
    );
    expect(resolver.lines.length, 1);
    expect(api.requests, isEmpty);
  });
  test('responses after disposal cannot publish stale lines', () async {
    final api = ResolverApi();
    final resolver = CatalogResolver(
      api,
      session,
      const WatchTarget(title: '测试电影', kind: 'movie'),
    );
    addTearDown(api.close);
    final done = resolver.resolve();
    await Future<void>.delayed(Duration.zero);
    resolver.dispose();
    api.fast.complete(results('fast'));
    api.slow.complete(results('slow'));
    await done;
    expect(resolver.lines, isEmpty);
  });
}
