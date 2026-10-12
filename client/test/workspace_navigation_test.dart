import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/appearance.dart';
import 'package:sameframe_client/src/design.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/screens/lobby_page.dart';
import 'package:sameframe_client/src/secure_session_store.dart';

void main() {
  testWidgets('details stay in the workspace and survive viewport changes', (
    tester,
  ) async {
    final semantics = tester.ensureSemantics();
    try {
      await _open(tester, signedIn: true, size: const Size(1440, 1000));
      expect(find.bySemanticsLabel('我的媒体源'), findsOneWidget);
      await tester.tap(find.text('搜索电影 / 剧集'));
      await tester.pump();
      expect(find.text('今晚，一起看点什么？'), findsNothing);
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), '共同的电影');
      await tester.tap(find.text('搜索'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('测试影片'));
      await tester.pumpAndSettle();
      expect(find.text('测试影片 · 影评'), findsOneWidget);
      expect(find.text('我的媒体源'), findsOneWidget);
      expect(find.bySemanticsLabel('我的媒体源'), findsOneWidget);

      tester.view.physicalSize = const Size(390, 844);
      await tester.pumpAndSettle();
      expect(find.text('测试影片 · 影评'), findsOneWidget);
      expect(find.byType(NavigationBar), findsOneWidget);
      await tester.pageBack();
      await tester.pumpAndSettle();
      expect(find.text('共同的电影'), findsOneWidget);

      // System back exits the section only after its detail route is gone.
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(find.text('影视元数据搜索'), findsNothing);
      expect(find.byType(NavigationBar), findsOneWidget);

      tester.view.physicalSize = const Size(1440, 1000);
      await tester.pumpAndSettle();
      await tester.tap(find.text('搜索电影 / 剧集'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), '第二次搜索');
      await tester.tap(find.text('搜索'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('测试影片'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('设备管理'));
      await tester.pumpAndSettle();
      expect(find.text('登录设备'), findsOneWidget);
      expect(find.text('测试影片 · 影评'), findsNothing);
      expect(find.byType(BackButton), findsNothing);
      await tester.tap(find.text('观影大厅'));
      await tester.pumpAndSettle();
      expect(find.text('今晚，一起看点什么？'), findsOneWidget);
      expect(tester.takeException(), isNull);
    } finally {
      semantics.dispose();
    }
  });

  testWidgets('all menu switches sections within the same mobile shell', (
    tester,
  ) async {
    await _open(tester, signedIn: true, size: const Size(390, 844));
    final shell = tester.state<ScaffoldState>(find.byType(Scaffold));
    final topbar = tester.element(find.byType(AppBar));
    await tester.tap(find.text('全部'));
    await tester.pumpAndSettle();
    await tester.scrollUntilVisible(
      find.text('安全更新'),
      180,
      scrollable: find.byType(Drawer).evaluate().isNotEmpty
          ? find
                .descendant(
                  of: find.byType(Drawer),
                  matching: find.byType(Scrollable),
                )
                .first
          : find.byType(Scrollable).first,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('安全更新'));
    await tester.pumpAndSettle();
    expect(tester.state<ScaffoldState>(find.byType(Scaffold)), same(shell));
    expect(tester.element(find.byType(AppBar)), same(topbar));
    expect(find.text('检查更新'), findsOneWidget);
    expect(
      tester.widget<NavigationBar>(find.byType(NavigationBar)).selectedIndex,
      4,
    );

    // The full menu is available from the section itself; no profile detour.
    await tester.tap(find.text('全部'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('设备管理'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('设备管理'));
    await tester.pumpAndSettle();
    expect(find.text('登录设备'), findsOneWidget);
    expect(find.text('检查更新'), findsNothing);
    expect(find.byType(BackButton), findsNothing);
    expect(tester.state<ScaffoldState>(find.byType(Scaffold)), same(shell));
    expect(tester.element(find.byType(AppBar)), same(topbar));
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'signed-out sections switch through bottom tabs without a back button',
    (tester) async {
      await _open(tester, signedIn: false, size: const Size(390, 844));
      await tester.tap(find.text('我的'));
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(
        find.text('安全更新'),
        180,
        scrollable: find.byType(Drawer).evaluate().isNotEmpty
            ? find
                  .descendant(
                    of: find.byType(Drawer),
                    matching: find.byType(Scrollable),
                  )
                  .first
            : find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('安全更新'));
      await tester.pumpAndSettle();
      expect(find.text('检查更新'), findsOneWidget);
      expect(find.byType(NavigationBar), findsOneWidget);
      expect(find.byType(BackButton), findsNothing);
      await tester.tap(find.text('我的'));
      await tester.pumpAndSettle();
      expect(find.text('登录或注册'), findsOneWidget);
      await tester.tap(find.text('片库'));
      await tester.pumpAndSettle();
      expect(find.byType(NavigationBar), findsOneWidget);
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(find.text('注册并进入'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}

Future<void> _open(
  WidgetTester tester, {
  required bool signedIn,
  required Size size,
}) async {
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = size;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  final api = _Api();
  final appearance = FrameAppearance();
  addTearDown(api.close);
  addTearDown(appearance.dispose);
  await tester.pumpWidget(
    FrameAppearanceScope(
      appearance: appearance,
      child: MaterialApp(
        theme: frameTheme(brightness: Brightness.light),
        home: LobbyPage(api: api, sessionStore: _Store(signedIn)),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _Store implements SessionStore {
  _Store(this.signedIn);
  final bool signedIn;
  @override
  Future<Session?> read() async => signedIn
      ? Session(
          accessToken: 'access',
          refreshToken: 'refresh',
          tokenType: 'bearer',
          expiresIn: 900,
          user: const AppUser(
            id: 'viewer',
            displayName: 'Viewer',
            email: 'viewer@example.test',
            emailVerified: true,
          ),
        )
      : null;
  @override
  Future<void> clear() async {}
  @override
  Future<void> write(Session session) async {}
}

class _Api extends ApiClient {
  @override
  Future<List<UserDevice>> listDevices(Session session) async => [];
  @override
  Future<List<AppAnnouncement>> announcements() async => [];
  @override
  Future<List<MetadataResult>> searchMetadata(
    Session session,
    String query, {
    String language = 'zh-CN',
  }) async => [
    MetadataResult.fromJson({
      'id': 1,
      'title': '测试影片',
      'media_type': 'movie',
      'release_date': '2026-01-01',
      'overview': '用于验证详情返回的测试影片。',
      'rating': 8,
      'poster_url': '',
    }),
  ];
  @override
  Future<List<Review>> reviews(
    Session session,
    String targetType,
    String targetId,
  ) async => [];
}
