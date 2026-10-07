import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models.dart';

class ApiClient {
  ApiClient({http.Client? client, String? baseUrl})
    : _client = client ?? http.Client(),
      baseUrl =
          (baseUrl ??
                  const String.fromEnvironment(
                    'API_BASE_URL',
                    defaultValue: 'http://localhost:8080',
                  ))
              .replaceFirst(RegExp(r'/$'), '');

  final http.Client _client;
  final String baseUrl;

  Future<Session> createDemoSession(String displayName) async {
    final data = await _request(
      'POST',
      '/api/v1/session/demo',
      body: {'display_name': displayName},
    );
    return Session.fromJson(data);
  }

  Future<Room> createRoom({
    required Session session,
    required String name,
    required String sourceUrl,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms',
      token: session.accessToken,
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
      token: session.accessToken,
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
      token: session.accessToken,
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
    String? token,
    Map<String, dynamic>? body,
  }) async {
    final request = http.Request(method, Uri.parse('$baseUrl$path'));
    request.headers['Content-Type'] = 'application/json';
    if (token != null) {
      request.headers['Authorization'] = 'Bearer $token';
    }
    if (body != null) {
      request.body = jsonEncode(body);
    }
    final streamed = await _client.send(request);
    final response = await http.Response.fromStream(streamed);
    final decoded = jsonDecode(response.body) as Map<String, dynamic>;
    if (response.statusCode >= 300 || decoded['code'] != 0) {
      throw ApiException(decoded['msg']?.toString() ?? 'Request failed');
    }
    return decoded['data'] as Map<String, dynamic>;
  }

  void close() => _client.close();
}

class ApiException implements Exception {
  const ApiException(this.message);
  final String message;

  @override
  String toString() => message;
}
