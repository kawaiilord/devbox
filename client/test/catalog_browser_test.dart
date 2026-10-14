import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/catalog_models.dart';
import 'package:sameframe_client/src/design.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/screens/catalog_browser.dart';

final session = Session(
  accessToken: 'test',
  refreshToken: 'test',
  tokenType: 'bearer',
  expiresIn: 900,
  user: const AppUser(
    id: 'viewer',
    displayName: '观众',
    email: 'viewer@example.test',
    emailVerified: true,
  ),
);
Map<String, dynamic> film(
  String source, {
  String key = 'same-2026',
  String id = '1',
}) => {
  'id': id,
  'source_id': source,
  'source_name': source,
  'title': '测试电影',
  'year': '2026',
  'kind': 'movie',
  'group_key': key,
  'groups': [
    {
      'id': 'main',
      'name': '主线',
      'episodes': List.generate(
        100,
        (i) => {'id': 'episode-$i', 'name': '第${i + 1}集'},
      ),
    },
  ],
};

class CatalogApi extends ApiClient {
  final slow = Completer<CatalogSearch>();
  bool empty = false;
  Map<String, dynamic>? requestedSelection;
  Map<String, dynamic>? soloSelection;
  @override
  Future<MediaPlaybackTicket> watchCatalog(
    Session session,
    Map<String, dynamic> selection,
  ) async {
    soloSelection = selection;
    throw const ApiException('测试播放服务离线');
  }

  @override
  Future<List<CatalogSource>> catalogSources(Session session) async => empty
      ? []
      : [
          for (final id in ['fast', 'slow', 'broken'])
            CatalogSource.fromJson({
              'id': id,
              'name': id,
              'kind': 'cms',
              'health': {},
            }),
        ];
  @override
  Future<CatalogSearch> searchCatalog(
    Session session,
    String sourceId,
    String query, {
    int page = 1,
  }) async {
    if (sourceId == 'slow') return slow.future;
    if (sourceId == 'broken') throw const ApiException('测试源离线');
    return CatalogSearch.fromJson({
      'items': [film('fast')],
      'has_more': false,
    });
  }

  @override
  Future<CatalogFilm> catalogDetail(Session session, CatalogFilm value) async =>
      CatalogFilm.fromJson(film(value.sourceId));
  @override
  Future<Room> createRoom({
    required Session session,
    required String name,
    String sourceUrl = '',
    String mediaSourceId = '',
    String mediaPath = '',
    double startPosition = 0,
    Map<String, dynamic>? settings,
    Map<String, dynamic>? catalog,
  }) async {
    requestedSelection = catalog;
    throw const ApiException('建房申请已提交，请在我的申请中查看进度');
  }
}

Future<void> open(WidgetTester tester, CatalogApi api, double width) async {
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = Size(width, 900);
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  addTearDown(api.close);
  await tester.pumpWidget(
    MaterialApp(
      theme: frameTheme(),
      home: Scaffold(
        body: CatalogBrowser(api: api, session: session, onOpenRoom: (_) {}),
      ),
    ),
  );
}

void main() {
  test('grouping retains versions and ignores repeated source results', () {
    final grouped = groupCatalogFilms([
      CatalogFilm.fromJson(film('a')),
      CatalogFilm.fromJson(film('b')),
      CatalogFilm.fromJson(film('a')),
      CatalogFilm.fromJson(film('c', key: 'same-2020')),
    ]);
    expect(grouped.length, 2);
    expect(grouped.first.length, 2);
  });
  for (final width in [320.0, 390.0, 1440.0]) {
    testWidgets(
      'incremental sources, details and selected episode fit $width',
      (tester) async {
        final api = CatalogApi();
        await open(tester, api, width);
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 100));
        expect(find.text('测试电影'), findsOneWidget);
        expect(find.byType(LinearProgressIndicator), findsOneWidget);
        api.slow.complete(
          CatalogSearch.fromJson({
            'items': [film('slow')],
            'has_more': false,
          }),
        );
        await tester.pumpAndSettle();
        expect(find.text('测试电影'), findsOneWidget);
        expect(find.text('1 个片源暂不可用'), findsOneWidget);
        await tester.tap(find.text('测试电影'));
        await tester.pumpAndSettle();
        expect(find.text('选择剧集 · 共 100 集'), findsOneWidget);
        expect(api.soloSelection, isNotNull);
        expect(api.requestedSelection, isNull);
        await tester.ensureVisible(find.widgetWithText(ChoiceChip, '第2集'));
        await tester.tap(find.widgetWithText(ChoiceChip, '第2集'));
        await tester.pumpAndSettle();
        expect(api.soloSelection?['episode_id'], 'episode-1');
        expect(api.requestedSelection, isNull);
        await tester.ensureVisible(find.text('创建房间'));
        await tester.tap(find.text('创建房间'));
        await tester.pumpAndSettle();
        expect(api.requestedSelection?['episode_id'], 'episode-1');
        expect((api.requestedSelection?['refs'] as List).length, 2);
        expect(find.text('建房申请已提交，请在我的申请中查看进度'), findsOneWidget);
        expect(tester.takeException(), isNull);
      },
    );
  }
  testWidgets('empty catalog has honest setup state', (tester) async {
    final api = CatalogApi()..empty = true;
    await open(tester, api, 390);
    await tester.pumpAndSettle();
    expect(find.text('片库等待上架'), findsOneWidget);
    expect(find.text('创建房间 · 立即观看'), findsNothing);
  });
}
