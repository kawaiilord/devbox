import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'device_identity.dart';
import 'design.dart';
import 'screens/lobby_page.dart';
import 'secure_session_store.dart';

class SameFrameApp extends StatefulWidget {
  const SameFrameApp({super.key, this.device = DeviceIdentity.test});

  final DeviceIdentity device;

  @override
  State<SameFrameApp> createState() => _SameFrameAppState();
}

class _SameFrameAppState extends State<SameFrameApp> {
  final SessionStore sessionStore = SecureSessionStore();
  late final ApiClient api = ApiClient(device: widget.device)
    ..onSessionUpdated = sessionStore.write;

  @override
  void dispose() {
    api.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'SameFrame · 同帧',
      debugShowCheckedModeBanner: false,
      theme: frameTheme(),
      home: LobbyPage(api: api, sessionStore: sessionStore),
    );
  }
}
