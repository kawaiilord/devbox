import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/models.dart';

void main() {
  test(
    'a 401 rotates the refresh token and replays the request once',
    () async {
      var roomAttempts = 0;
      final client = MockClient((request) async {
        if (request.url.path == '/api/v1/auth/refresh') {
          expect(jsonDecode(request.body)['refresh_token'], 'old-refresh');
          return http.Response(
            jsonEncode({
              'code': 0,
              'data': {
                'access_token': 'new-access',
                'refresh_token': 'new-refresh',
                'token_type': 'bearer',
                'expires_in': 900,
                'user': {
                  'id': 'owner',
                  'display_name': 'Owner',
                  'email': 'owner@example.com',
                },
              },
              'msg': 'ok',
            }),
            200,
          );
        }
        if (request.url.path == '/api/v1/rooms') {
          roomAttempts++;
          if (roomAttempts == 1) {
            expect(request.headers['Authorization'], 'Bearer old-access');
            return http.Response(
              jsonEncode({'code': 401, 'msg': 'unauthorized'}),
              401,
            );
          }
          expect(request.headers['Authorization'], 'Bearer new-access');
          return http.Response(
            jsonEncode({
              'code': 0,
              'data': {
                'code': 'ABC123',
                'name': 'Room',
                'owner_id': 'owner',
                'source_url': 'https://example.com/movie.mp4',
                'max_members': 8,
                'expires_at': 10000,
                'members': <Object>[],
                'playback': {
                  'position': 0,
                  'playing': false,
                  'speed': 1,
                  'episode': 0,
                  'position_ts': 1,
                  'source_version': 1,
                },
              },
              'msg': 'created',
            }),
            201,
          );
        }
        return http.Response('not found', 404);
      });
      final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
      final session = Session(
        accessToken: 'old-access',
        refreshToken: 'old-refresh',
        tokenType: 'bearer',
        expiresIn: 1,
        user: const AppUser(
          id: 'owner',
          displayName: 'Owner',
          email: 'owner@example.com',
        ),
      );

      final room = await api.createRoom(
        session: session,
        name: 'Room',
        sourceUrl: 'https://example.com/movie.mp4',
      );

      expect(room.code, 'ABC123');
      expect(roomAttempts, 2);
      expect(session.accessToken, 'new-access');
      expect(session.refreshToken, 'new-refresh');
      api.close();
    },
  );

  test('concurrent 401 responses share one refresh request', () async {
    var refreshRequests = 0;
    final client = MockClient((request) async {
      if (request.url.path == '/api/v1/auth/refresh') {
        refreshRequests++;
        await Future<void>.delayed(const Duration(milliseconds: 20));
        return _sessionResponse('new-access', 'new-refresh');
      }
      if (request.url.path == '/api/v1/rooms') {
        if (request.headers['Authorization'] == 'Bearer old-access') {
          return http.Response(
            jsonEncode({'code': 401, 'msg': 'unauthorized'}),
            401,
          );
        }
        return _roomResponse();
      }
      return http.Response('not found', 404);
    });
    final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
    final session = _session();

    await Future.wait([
      api.createRoom(
        session: session,
        name: 'First',
        sourceUrl: 'https://example.com/first.mp4',
      ),
      api.createRoom(
        session: session,
        name: 'Second',
        sourceUrl: 'https://example.com/second.mp4',
      ),
    ]);

    expect(refreshRequests, 1);
    expect(session.refreshToken, 'new-refresh');
    api.close();
  });
}

Session _session() => Session(
  accessToken: 'old-access',
  refreshToken: 'old-refresh',
  tokenType: 'bearer',
  expiresIn: 1,
  user: const AppUser(
    id: 'owner',
    displayName: 'Owner',
    email: 'owner@example.com',
  ),
);

http.Response _sessionResponse(String access, String refresh) => http.Response(
  jsonEncode({
    'code': 0,
    'data': {
      'access_token': access,
      'refresh_token': refresh,
      'token_type': 'bearer',
      'expires_in': 900,
      'user': {
        'id': 'owner',
        'display_name': 'Owner',
        'email': 'owner@example.com',
      },
    },
    'msg': 'ok',
  }),
  200,
);

http.Response _roomResponse() => http.Response(
  jsonEncode({
    'code': 0,
    'data': {
      'code': 'ABC123',
      'name': 'Room',
      'owner_id': 'owner',
      'source_url': 'https://example.com/movie.mp4',
      'max_members': 8,
      'expires_at': 10000,
      'members': <Object>[],
      'playback': {
        'position': 0,
        'playing': false,
        'speed': 1,
        'episode': 0,
        'position_ts': 1,
        'source_version': 1,
      },
    },
    'msg': 'created',
  }),
  201,
);
