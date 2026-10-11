import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'appearance.dart';
import 'device_identity.dart';
import 'design.dart';
import 'screens/lobby_page.dart';
import 'secure_session_store.dart';

class SameFrameApp extends StatefulWidget {
  const SameFrameApp({
    super.key,
    this.device = DeviceIdentity.test,
    this.appearance,
  });

  final DeviceIdentity device;
  final FrameAppearance? appearance;

  @override
  State<SameFrameApp> createState() => _SameFrameAppState();
}

class _SameFrameAppState extends State<SameFrameApp> {
  late final FrameAppearance _appearance =
      widget.appearance ?? FrameAppearance();
  final SessionStore sessionStore = SecureSessionStore();
  late final ApiClient api = ApiClient(device: widget.device)
    ..onSessionUpdated = sessionStore.write;

  @override
  void dispose() {
    api.close();
    _appearance.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return FrameAppearanceScope(
      appearance: _appearance,
      child: ListenableBuilder(
        listenable: _appearance,
        builder: (context, child) => MaterialApp(
          title: 'SameFrame · 同帧',
          debugShowCheckedModeBanner: false,
          themeMode: _appearance.mode,
          theme: frameTheme(brightness: Brightness.light),
          darkTheme: frameTheme(),
          home: child,
        ),
        child: LobbyPage(api: api, sessionStore: sessionStore),
      ),
    );
  }
}
