import 'dart:convert';

import 'package:http/http.dart' as http;

import '../device_identity.dart';
import '../models.dart';

class ApiClient {
  ApiClient({
    http.Client? client,
    String? baseUrl,
    this.device = DeviceIdentity.test,
  }) : _client = client ?? http.Client(),
       baseUrl =
           (baseUrl ??
                   const String.fromEnvironment(
                     'API_BASE_URL',
                     defaultValue: 'http://localhost:8080',
                   ))
               .replaceFirst(RegExp(r'/$'), '');

  final http.Client _client;
  final String baseUrl;
  final DeviceIdentity device;
  Future<Session>? _refreshInFlight;

  Future<Session> createDemoSession(String displayName) async {
    final data = await _request(
      'POST',
      '/api/v1/session/demo',
      body: {'display_name': displayName},
    );
    return Session.fromJson(data);
  }

  Future<Session> register({
    required String email,
    required String displayName,
    required String password,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/auth/register',
      body: {'email': email, 'display_name': displayName, 'password': password},
    );
    return Session.fromJson(data);
  }

  Future<Session> login({
    required String email,
    required String password,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/auth/login',
      body: {'email': email, 'password': password},
    );
    return Session.fromJson(data);
  }

  Future<void> logout(Session session) async {
    await _request(
      'POST',
      '/api/v1/auth/logout',
      body: {'refresh_token': session.refreshToken},
      allowRefresh: false,
    );
  }

  Future<void> requestEmailVerification(Session session) async {
    await _request(
      'POST',
      '/api/v1/auth/email/verify-request',
      session: session,
    );
  }

  Future<void> verifyEmail(String token) async {
    await _request('POST', '/api/v1/auth/email/verify', body: {'token': token});
  }

  Future<void> requestPasswordReset(String email) async {
    await _request(
      'POST',
      '/api/v1/auth/password/request',
      body: {'email': email},
    );
  }

  Future<void> resetPassword({
    required String token,
    required String password,
  }) async {
    await _request(
      'POST',
      '/api/v1/auth/password/reset',
      body: {'token': token, 'password': password},
    );
  }

  Future<List<UserDevice>> listDevices(Session session) async {
    final data = await _request('GET', '/api/v1/devices', session: session);
    return (data['devices'] as List<dynamic>)
        .map((value) => UserDevice.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> revokeDevice(Session session, String deviceId) async {
    await _request('DELETE', '/api/v1/devices/$deviceId', session: session);
  }

  Future<Room> createRoom({
    required Session session,
    required String name,
    required String sourceUrl,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms',
      session: session,
      body: {'name': name, 'source_url': sourceUrl, 'max_members': 8},
    );
    return Room.fromJson(data);
  }

  Future<Room> joinRoom({
    required Session session,
    required String code,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms/${code.trim().toUpperCase()}/join',
      session: session,
    );
    return Room.fromJson(data);
  }

  Future<Duration> measureClockOffset() async {
    var bestRoundTrip = const Duration(days: 1);
    var bestOffset = Duration.zero;
    for (var attempt = 0; attempt < 3; attempt++) {
      final before = DateTime.now().millisecondsSinceEpoch;
      final data = await _request('GET', '/api/v1/clock');
      final after = DateTime.now().millisecondsSinceEpoch;
      final roundTrip = Duration(milliseconds: after - before);
      final midpoint = before + ((after - before) ~/ 2);
      final serverTime = (data['server_time'] as num).toInt();
      if (roundTrip < bestRoundTrip) {
        bestRoundTrip = roundTrip;
        bestOffset = Duration(milliseconds: serverTime - midpoint);
      }
    }
    return bestOffset;
  }

  Future<Uri> roomSocketUri(Room room, Session session) async {
    final ticket = await _request(
      'POST',
      '/api/v1/rooms/${room.code}/socket-ticket',
      session: session,
    );
    final httpUri = Uri.parse(baseUrl);
    return httpUri.replace(
      scheme: httpUri.scheme == 'https' ? 'wss' : 'ws',
      path: '/ws/v1/rooms/${room.code}',
      queryParameters: {'ticket': ticket['ticket'] as String},
    );
  }

  Future<Map<String, dynamic>> _request(
    String method,
    String path, {
    Session? session,
    Map<String, dynamic>? body,
    bool allowRefresh = true,
  }) async {
    final request = http.Request(method, Uri.parse('$baseUrl$path'));
    request.headers['Content-Type'] = 'application/json';
    request.headers['X-Device-ID'] = device.id;
    request.headers['X-Device-Name'] = device.label;
    request.headers['X-Device-Platform'] = device.platform;
    final requestAccessToken = session?.accessToken;
    if (session != null) {
      request.headers['Authorization'] = 'Bearer $requestAccessToken';
    }
    if (body != null) {
      request.body = jsonEncode(body);
    }
    final streamed = await _client.send(request);
    final response = await http.Response.fromStream(streamed);
    final decoded = jsonDecode(response.body) as Map<String, dynamic>;
    if (response.statusCode == 401 && session != null && allowRefresh) {
      if (session.accessToken == requestAccessToken) {
        final refreshed = await _refreshSingleFlight(session.refreshToken);
        session.replaceTokens(refreshed);
      }
      return _request(
        method,
        path,
        session: session,
        body: body,
        allowRefresh: false,
      );
    }
    if (response.statusCode >= 300 || decoded['code'] != 0) {
      throw ApiException(decoded['msg']?.toString() ?? 'Request failed');
    }
    return decoded['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
  }

  Future<Session> _refresh(String refreshToken) async {
    final data = await _request(
      'POST',
      '/api/v1/auth/refresh',
      body: {'refresh_token': refreshToken},
      allowRefresh: false,
    );
    return Session.fromJson(data);
  }

  Future<Session> _refreshSingleFlight(String refreshToken) async {
    final active = _refreshInFlight;
    if (active != null) return active;
    final refresh = _refresh(refreshToken);
    _refreshInFlight = refresh;
    try {
      return await refresh;
    } finally {
      if (identical(_refreshInFlight, refresh)) {
        _refreshInFlight = null;
      }
    }
  }

  void close() => _client.close();
}

class ApiException implements Exception {
  const ApiException(this.message);
  final String message;

  @override
  String toString() => message;
}
