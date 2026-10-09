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
          expect(request.headers['X-Device-ID'], 'test-device-0001');
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
          emailVerified: true,
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

  test(
    'privacy, block, and report calls use authenticated endpoints',
    () async {
      final methods = <String>[];
      final client = MockClient((request) async {
        methods.add('${request.method} ${request.url.path}');
        expect(request.headers['Authorization'], 'Bearer old-access');
        if (request.url.path == '/api/v1/privacy' && request.method == 'GET') {
          return http.Response(
            jsonEncode({
              'code': 0,
              'data': {
                'allow_room_chat': true,
                'allow_profile_find': false,
                'show_watch_activity': true,
              },
              'msg': 'ok',
            }),
            200,
          );
        }
        if (request.url.path == '/api/v1/privacy' &&
            request.method == 'PATCH') {
          final body = jsonDecode(request.body) as Map<String, dynamic>;
          expect(body['allow_room_chat'], false);
          return http.Response(
            jsonEncode({'code': 0, 'data': body, 'msg': 'updated'}),
            200,
          );
        }
        if (request.url.path == '/api/v1/blocks' && request.method == 'GET') {
          return http.Response(
            jsonEncode({
              'code': 0,
              'data': {
                'users': [
                  {
                    'id': 'blocked-user',
                    'display_name': 'Blocked',
                    'email_verified': true,
                  },
                ],
              },
              'msg': 'ok',
            }),
            200,
          );
        }
        if (request.url.path == '/api/v1/reports') {
          final body = jsonDecode(request.body) as Map<String, dynamic>;
          expect(body['target_type'], 'message');
          expect(body['target_id'], '42');
          return http.Response(
            jsonEncode({
              'code': 0,
              'data': <String, dynamic>{},
              'msg': 'created',
            }),
            201,
          );
        }
        return http.Response(
          jsonEncode({'code': 0, 'data': <String, dynamic>{}, 'msg': 'ok'}),
          200,
        );
      });
      final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
      final session = _session();

      final privacy = await api.privacy(session);
      expect(privacy.allowProfileFind, false);
      final saved = await api.updatePrivacy(
        session,
        const PrivacySettings(
          allowRoomChat: false,
          allowProfileFind: true,
          showWatchActivity: false,
        ),
      );
      expect(saved.allowRoomChat, false);
      final blocked = await api.blockedUsers(session);
      expect(blocked.single.id, 'blocked-user');
      await api.blockUser(session, 'blocked-user');
      await api.unblockUser(session, 'blocked-user');
      await api.createReport(
        session: session,
        targetType: 'message',
        targetId: '42',
        reason: 'spam',
      );

      expect(methods, [
        'GET /api/v1/privacy',
        'PATCH /api/v1/privacy',
        'GET /api/v1/blocks',
        'POST /api/v1/blocks/blocked-user',
        'DELETE /api/v1/blocks/blocked-user',
        'POST /api/v1/reports',
      ]);
      api.close();
    },
  );

  test('creates an Emby source through the authenticated API', () async {
    final client = MockClient((request) async {
      expect(request.method, 'POST');
      expect(request.url.path, '/api/v1/sources/emby');
      expect(request.headers['Authorization'], 'Bearer old-access');
      final body = jsonDecode(request.body) as Map<String, dynamic>;
      expect(body, {
        'name': 'Home Emby',
        'base_url': 'https://emby.example.com/emby/',
        'username': 'viewer',
        'password': 'temporary-password',
      });
      return http.Response(
        jsonEncode({
          'code': 0,
          'data': {
            'id': 'source-emby',
            'type': 'emby',
            'name': 'Home Emby',
            'base_url': 'https://emby.example.com/emby/',
          },
          'msg': 'created',
        }),
        201,
      );
    });
    final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
    final source = await api.createEmbySource(
      session: _session(),
      name: 'Home Emby',
      baseUrl: 'https://emby.example.com/emby/',
      username: 'viewer',
      password: 'temporary-password',
    );
    expect(source.type, 'emby');
    expect(source.id, 'source-emby');
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
    emailVerified: true,
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
