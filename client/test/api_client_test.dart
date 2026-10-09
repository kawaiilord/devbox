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
                'allow_private_chat': true,
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
          allowPrivateChat: true,
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

  test('syncs favorites, history, and watch progress', () async {
    final methods = <String>[];
    final client = MockClient((request) async {
      methods.add('${request.method} ${request.url.path}');
      if (request.url.path == '/api/v1/favorites' && request.method == 'POST') {
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['media_path'], '/movie.mp4');
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'id': 1,
              'source_id': 'source-1',
              'source_type': 'webdav',
              'source_name': 'Media',
              'media_path': '/movie.mp4',
              'title': 'Movie',
              'content_type': 'video/mp4',
              'size': 100,
              'updated_at': 1000,
            },
            'msg': 'saved',
          }),
          200,
        );
      }
      if (request.url.path == '/api/v1/favorites' && request.method == 'GET') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {'favorites': <Object>[]},
            'msg': 'ok',
          }),
          200,
        );
      }
      if (request.url.path == '/api/v1/history' && request.method == 'GET') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'records': [
                {
                  'id': 2,
                  'source_id': 'source-1',
                  'media_path': '/movie.mp4',
                  'title': 'Movie',
                  'position_seconds': 42.5,
                  'duration_seconds': 100,
                  'episode': 0,
                  'completed': false,
                  'companion_count': 1,
                  'room_code': 'ABC123',
                  'resumable': true,
                  'watched_at': 2000,
                },
              ],
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      if (request.url.path.endsWith('/watch-progress')) {
        expect(request.method, 'PUT');
        expect(jsonDecode(request.body)['duration_seconds'], 100.0);
      }
      return http.Response(
        jsonEncode({'code': 0, 'data': <String, dynamic>{}, 'msg': 'ok'}),
        200,
      );
    });
    final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
    final session = _session();
    final favorite = await api.addFavorite(
      session: session,
      sourceId: 'source-1',
      file: const MediaFile(
        name: 'Movie',
        path: '/movie.mp4',
        isDirectory: false,
        size: 100,
        contentType: 'video/mp4',
      ),
    );
    expect(favorite.id, 1);
    expect(await api.favorites(session), isEmpty);
    final history = await api.watchHistory(session);
    expect(history.single.positionSeconds, 42.5);
    expect(history.single.resumable, true);
    await api.updateWatchProgress(session, 'ABC123', 100);
    await api.deleteFavorite(session, 1);
    await api.deleteWatchRecord(session, 2);
    expect(methods, [
      'POST /api/v1/favorites',
      'GET /api/v1/favorites',
      'GET /api/v1/history',
      'PUT /api/v1/rooms/ABC123/watch-progress',
      'DELETE /api/v1/favorites/1',
      'DELETE /api/v1/history/2',
    ]);
    api.close();
  });

  test('loads room danmaku and metadata search results', () async {
    final client = MockClient((request) async {
      if (request.url.path.endsWith('/danmaku')) {
        expect(request.url.queryParameters['episode'], '2');
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'messages': [
                {
                  'id': 1,
                  'user_id': 'viewer',
                  'display_name': 'Viewer',
                  'body': 'Hello',
                  'position_seconds': 12.5,
                  'color': 16777215,
                  'mode': 'scroll',
                  'created_at': 1000,
                },
              ],
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      if (request.url.path == '/api/v1/metadata/search') {
        expect(request.url.queryParameters['q'], 'Movie');
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'results': [
                {
                  'id': 42,
                  'media_type': 'movie',
                  'title': 'Movie',
                  'rating': 8.5,
                },
              ],
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      return http.Response('not found', 404);
    });
    final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
    final session = _session();
    final danmaku = await api.roomDanmaku(session, 'ABC123', episode: 2);
    final metadata = await api.searchMetadata(session, 'Movie');
    expect(danmaku.single.positionSeconds, 12.5);
    expect(metadata.single.id, 42);
    expect(metadata.single.rating, 8.5);
    api.close();
  });

  test('loads social profiles, conversations, and direct messages', () async {
    final client = MockClient((request) async {
      if (request.url.path == '/api/v1/social/users') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'users': [
                {
                  'id': 'peer',
                  'display_name': 'Peer',
                  'following': false,
                  'follows_viewer': true,
                  'follower_count': 2,
                  'following_count': 1,
                },
              ],
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      if (request.url.path == '/api/v1/social/conversations' &&
          request.method == 'POST') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'id': 7,
              'peer': {
                'id': 'peer',
                'display_name': 'Peer',
                'following': false,
                'follows_viewer': true,
                'follower_count': 2,
                'following_count': 1,
              },
              'unread_count': 0,
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      if (request.url.path.endsWith('/messages') && request.method == 'GET') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'messages': [
                {
                  'id': 9,
                  'conversation_id': 7,
                  'sender_id': 'peer',
                  'body': 'hello',
                  'created_at': 1000,
                },
              ],
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      if (request.url.path == '/api/v1/social/unread') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {'count': 1},
            'msg': 'ok',
          }),
          200,
        );
      }
      return http.Response(
        jsonEncode({'code': 0, 'data': <String, dynamic>{}, 'msg': 'ok'}),
        200,
      );
    });
    final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
    final session = _session();
    expect((await api.searchSocialUsers(session, 'Peer')).single.id, 'peer');
    final conversation = await api.createConversation(session, 'peer');
    expect(conversation.id, 7);
    expect((await api.directMessages(session, 7)).single.body, 'hello');
    expect(await api.directUnread(session), 1);
    await api.followUser(session, 'peer');
    await api.unfollowUser(session, 'peer');
    api.close();
  });

  test('review API signs upload, uploads bytes, and parses reviews', () async {
    final requests = <String>[];
    final client = MockClient((request) async {
      requests.add('${request.method} ${request.url}');
      if (request.url.host == 'objects.example.com') {
        expect(request.method, 'PUT');
        expect(request.headers['content-type'], 'image/png');
        expect(request.bodyBytes, [1, 2, 3]);
        return http.Response('', 200);
      }
      expect(request.headers['Authorization'], 'Bearer old-access');
      if (request.url.path == '/api/v1/reviews/uploads') {
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'object_key': 'reviews/owner/picture.png',
              'upload_url': 'https://objects.example.com/signed-put',
              'expires_at': 1000,
            },
            'msg': 'created',
          }),
          201,
        );
      }
      if (request.url.path == '/api/v1/reviews' && request.method == 'POST') {
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['image_keys'], ['reviews/owner/picture.png']);
        return _reviewResponse();
      }
      if (request.url.path == '/api/v1/reviews') {
        expect(request.url.queryParameters['target_type'], 'movie');
        expect(request.url.queryParameters['target_id'], '42');
        final review = jsonDecode(_reviewResponse().body)['data'];
        return http.Response(
          jsonEncode({
            'code': 0,
            'data': {
              'reviews': [review],
            },
            'msg': 'ok',
          }),
          200,
        );
      }
      return http.Response('not found', 404);
    });
    final api = ApiClient(client: client, baseUrl: 'https://api.example.com');
    final session = _session();

    final key = await api.uploadReviewImage(
      session: session,
      filename: 'picture.png',
      contentType: 'image/png',
      bytes: [1, 2, 3],
    );
    expect(key, 'reviews/owner/picture.png');
    final saved = await api.saveReview(
      session: session,
      targetType: 'movie',
      targetId: '42',
      title: 'Movie',
      rating: 9,
      content: 'Excellent',
      imageKeys: [key],
    );
    expect(saved.rating, 9);
    expect((await api.reviews(session, 'movie', '42')).single.id, 12);
    expect(requests, hasLength(4));
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

http.Response _reviewResponse() => http.Response(
  jsonEncode({
    'code': 0,
    'data': {
      'id': 12,
      'author': {
        'id': 'owner',
        'display_name': 'Owner',
        'signature': '',
        'following': false,
        'follows_viewer': false,
        'follower_count': 0,
        'following_count': 0,
      },
      'target_type': 'movie',
      'target_id': '42',
      'title': 'Movie',
      'rating': 9,
      'content': 'Excellent',
      'image_urls': ['https://objects.example.com/signed-get'],
      'created_at': 1000,
      'updated_at': 1000,
    },
    'msg': 'ok',
  }),
  200,
);
