import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'device_identity.dart';
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
    const surface = Color(0xFF111A1A);
    final colors = ColorScheme.fromSeed(
      seedColor: const Color(0xFF42D3B1),
      brightness: Brightness.dark,
      surface: surface,
    );
    return MaterialApp(
      title: 'SameFrame · 同帧',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: colors,
        scaffoldBackgroundColor: const Color(0xFF091010),
        cardTheme: const CardThemeData(
          color: surface,
          elevation: 0,
          margin: EdgeInsets.zero,
        ),
        inputDecorationTheme: InputDecorationTheme(
          filled: true,
          fillColor: const Color(0xFF172222),
          border: OutlineInputBorder(
            borderRadius: BorderRadius.circular(14),
            borderSide: BorderSide.none,
          ),
        ),
        useMaterial3: true,
      ),
      home: LobbyPage(api: api, sessionStore: sessionStore),
    );
  }
}
