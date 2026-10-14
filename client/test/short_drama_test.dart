import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/catalog_models.dart';
import 'package:sameframe_client/src/design.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/short_drama_models.dart';
import 'package:sameframe_client/src/screens/short_drama_page.dart';

final session = Session(
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
Map<String, dynamic> drama(String title) => {
  'id': '7000000000000000001',
  'source_id': 'builtin-guoguo',
  'source_name': '果果短剧',
  'title': title,
  'kind': 'short',
  'group_key': 'guoguo-1',
  'groups': [
    {
      'id': 'main',
      'name': '果果短剧',
      'episodes': [
        {'id': '11', 'name': '第1集'},
        {'id': '22', 'name': '第2集'},
      ],
    },
  ],
};

class ShortDramaApi extends ApiClient {
  final delayed = Completer<ShortDramaFeed>();
  Map<String, dynamic>? selection;
  int catalogSearches = 0;
  @override
  Future<ShortDramaFeed> shortDramas(
    Session session,
    Map<String, dynamic> query,
  ) async {
    if (query['section'] == 'short_play') return delayed.future;
    return ShortDramaFeed.fromJson({
      'items': [drama('漫剧样本')],
      'has_more': false,
    });
  }

  @override
  Future<List<CatalogSource>> catalogSources(Session session) async {
    catalogSearches++;
    return [];
  }

  @override
  Future<CatalogFilm> catalogDetail(Session session, CatalogFilm film) async =>
      CatalogFilm.fromJson(drama(film.title));
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
    selection = catalog;
    throw const ApiException('建房申请已提交');
  }
}

void main() {
  testWidgets(
    'web short dramas ignore stale categories and preserve episode selection',
    (tester) async {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = const Size(1440, 1000);
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = ShortDramaApi();
      addTearDown(api.close);
      await tester.pumpWidget(
        MaterialApp(
          theme: frameTheme(),
          home: ShortDramaPage(api: api, session: session, onOpenRoom: (_) {}),
        ),
      );
      await tester.pump();
      await tester.tap(find.text('漫剧'));
      await tester.pumpAndSettle();
      expect(find.text('漫剧样本'), findsOneWidget);
      api.delayed.complete(
        ShortDramaFeed.fromJson({
          'items': [drama('过期真人剧')],
          'has_more': false,
        }),
      );
      await tester.pumpAndSettle();
      expect(find.text('过期真人剧'), findsNothing);
      await tester.tap(find.text('漫剧样本'));
      await tester.pumpAndSettle();
      expect(find.text('第2集'), findsOneWidget);
      expect(api.catalogSearches, 0);
      await tester.tap(find.text('第2集'));
      await tester.tap(find.text('创建房间 · 立即观看'));
      await tester.pumpAndSettle();
      expect(api.selection?['episode_id'], '22');
      expect(api.selection?['all_episodes'], true);
      expect(
        (api.selection?['refs'] as List).first['source_id'],
        'builtin-guoguo',
      );
      expect(find.text('建房申请已提交'), findsOneWidget);
      await tester.tap(find.text('短剧列表'));
      await tester.pumpAndSettle();
      expect(find.text('漫剧样本'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}
