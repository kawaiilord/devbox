import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/design.dart';
import 'package:sameframe_client/src/discovery_models.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/screens/film_discovery_page.dart';

final _session = Session(
  accessToken: 'access',
  refreshToken: 'refresh',
  tokenType: 'bearer',
  expiresIn: 900,
  user: const AppUser(
    id: 'viewer',
    displayName: '观众',
    email: 'viewer@example.test',
    emailVerified: true,
  ),
);

DiscoveryFeed _feed(String title, {int page = 1, bool more = false}) =>
    DiscoveryFeed(
      items: [
        DiscoveryItem(
          id: '$title-$page',
          title: title,
          kind: 'movie',
          rating: 8.8,
          url: 'https://movie.douban.com/subject/123/',
        ),
      ],
      source: '豆瓣',
      page: page,
      fetchedAt: DateTime.utc(2026, 10, 13),
      hasMore: more,
    );

class _Api extends ApiClient {
  final requests =
      <({String section, String category, int page, int weekday})>[];
  bool fail = false;
  bool pagination = false;
  Completer<DiscoveryFeed>? pending;
  @override
  Future<DiscoveryFeed> discoverFilms(
    Session session, {
    String section = 'movies',
    String category = 'all',
    int page = 1,
    int weekday = 0,
  }) async {
    requests.add((
      section: section,
      category: category,
      page: page,
      weekday: weekday,
    ));
    if (fail) throw const ApiException('影视资料暂时无法加载');
    if (section == 'movies' && pending != null) return pending!.future;
    return _feed(
      '$section/$category/$weekday',
      page: page,
      more: pagination && page == 1,
    );
  }
}

Future<void> _open(
  WidgetTester tester,
  _Api api,
  Size size, {
  Brightness brightness = Brightness.light,
}) async {
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = size;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  addTearDown(api.close);
  await tester.pumpWidget(
    MaterialApp(
      theme: frameTheme(brightness: brightness),
      home: FilmDiscoveryPage(api: api, session: _session),
    ),
  );
}

void main() {
  for (final width in [320.0, 390.0, 1440.0]) {
    testWidgets('discovery filters and detail sheet fit width $width', (
      tester,
    ) async {
      final api = _Api();
      await _open(
        tester,
        api,
        Size(width, 900),
        brightness: width == 390 ? Brightness.dark : Brightness.light,
      );
      await tester.pumpAndSettle();
      expect(find.text('movies/all/0'), findsOneWidget);
      await tester.tap(find.text('movies/all/0'));
      await tester.pumpAndSettle();
      expect(find.text('复制片名'), findsOneWidget);
      expect(find.text('查看资料'), findsOneWidget);
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      await tester.tap(find.text('剧集分类'));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.widgetWithText(OutlinedButton, '美剧'));
      await tester.pumpAndSettle();
      await tester.tap(find.widgetWithText(OutlinedButton, '美剧'));
      await tester.pumpAndSettle();
      expect(api.requests.last.category, 'us');
      await tester.ensureVisible(find.text('高分榜'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('高分榜'));
      await tester.pumpAndSettle();
      expect(api.requests.last.category, 'movie');
      await tester.ensureVisible(find.text('新番放送'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('新番放送'));
      await tester.pumpAndSettle();
      await tester.tap(find.widgetWithText(OutlinedButton, '周一'));
      await tester.pumpAndSettle();
      expect(api.requests.last.weekday, 1);
      expect(api.requests.last.section, 'anime');
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('late response cannot overwrite a newer selected category', (
    tester,
  ) async {
    final api = _Api()..pending = Completer<DiscoveryFeed>();
    await _open(tester, api, const Size(1440, 1000));
    await tester.pump();
    await tester.tap(find.text('剧集分类'));
    await tester.pumpAndSettle();
    api.pending!.complete(_feed('过期电影结果'));
    await tester.pumpAndSettle();
    expect(find.text('过期电影结果'), findsNothing);
    expect(find.text('series/all/0'), findsOneWidget);
  });

  testWidgets('upstream error offers a working retry', (tester) async {
    final api = _Api()..fail = true;
    await _open(tester, api, const Size(390, 844));
    await tester.pumpAndSettle();
    expect(find.text('影视资料暂时无法加载'), findsOneWidget);
    api.fail = false;
    await tester.tap(find.text('重新加载'));
    await tester.pumpAndSettle();
    expect(find.text('movies/all/0'), findsOneWidget);
    expect(api.requests, hasLength(2));
    expect(tester.takeException(), isNull);
  });

  testWidgets('load more requests the next page without losing the first', (
    tester,
  ) async {
    final api = _Api()..pagination = true;
    await _open(tester, api, const Size(1440, 1000));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('加载更多'));
    await tester.tap(find.text('加载更多'));
    await tester.pumpAndSettle();
    expect(api.requests.last.page, 2);
    expect(find.text('movies/all/0'), findsNWidgets(2));
    expect(find.text('加载更多'), findsNothing);
  });
}
