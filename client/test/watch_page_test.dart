import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/catalog_models.dart';
import 'package:sameframe_client/src/design.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/player/catalog_resolver.dart';
import 'package:sameframe_client/src/screens/watch_page.dart';

final _session = Session(
  accessToken: 'test',
  refreshToken: 'test',
  tokenType: 'bearer',
  expiresIn: 900,
  user: const AppUser(
    id: 'viewer',
    displayName: 'Viewer',
    email: 'viewer@example.test',
    emailVerified: true,
  ),
);

CatalogFilm _film(String source) => CatalogFilm.fromJson({
  'id': 'movie',
  'source_id': source,
  'source_name': source,
  'title': '测试电影',
  'year': '2026',
  'kind': 'movie',
  'group_key': 'movie-2026',
  'groups': [
    {
      'id': 'main',
      'name': '主线',
      'episodes': [
        {'id': 'feature', 'name': '正片'},
      ],
    },
  ],
});

class _Api extends ApiClient {
  final attempts = <String>[];
  final late = Completer<CatalogFilm>();
  bool delayLast = false;
  @override
  Future<List<CatalogSource>> catalogSources(Session session) async => [];
  @override
  Future<CatalogFilm> catalogDetail(Session session, CatalogFilm film) async {
    if (delayLast && film.sourceId == 'source-5') return late.future;
    return film;
  }

  @override
  Future<MediaPlaybackTicket> watchCatalog(
    Session session,
    Map<String, dynamic> selection,
  ) async {
    attempts.add((selection['refs'] as List).first['source_id'] as String);
    throw const ApiException('播放地址不可访问');
  }
}

Future<void> _open(WidgetTester tester, _Api api) async {
  SharedPreferences.setMockInitialValues({});
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = const Size(1440, 1000);
  addTearDown(tester.view.resetDevicePixelRatio);
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(api.close);
  await tester.pumpWidget(
    MaterialApp(
      theme: frameTheme(),
      home: WatchPage(
        api: api,
        session: _session,
        target: const WatchTarget(title: '测试电影', year: '2026', kind: 'movie'),
        films: List.generate(6, (i) => _film('source-$i')),
      ),
    ),
  );
}

void main() {
  testWidgets('unavailable first four lines do not hide remaining sources', (
    tester,
  ) async {
    final api = _Api();
    await _open(tester, api);
    await tester.pumpAndSettle();
    expect(api.attempts, List.generate(6, (i) => 'source-$i'));
    expect(find.text('播放地址不可访问'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pump(const Duration(seconds: 30));
    expect(api.attempts.length, 6, reason: 'Exhausted sources must not loop');
  });

  testWidgets('late source is tried once after earlier lines are exhausted', (
    tester,
  ) async {
    final api = _Api()..delayLast = true;
    await _open(tester, api);
    for (var i = 0; i < 30; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    expect(api.attempts, List.generate(5, (i) => 'source-$i'));
    api.late.complete(_film('source-5'));
    await tester.pumpAndSettle();
    expect(api.attempts, List.generate(6, (i) => 'source-$i'));
    expect(tester.takeException(), isNull);
  });

  testWidgets('leaving the player cancels attempts for a late source', (
    tester,
  ) async {
    final api = _Api()..delayLast = true;
    await _open(tester, api);
    for (var i = 0; i < 30; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    final count = api.attempts.length;
    await tester.pumpWidget(const SizedBox());
    api.late.complete(_film('source-5'));
    await tester.pumpAndSettle();
    expect(api.attempts.length, count);
    expect(tester.takeException(), isNull);
  });
}
