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

  Future<PrivacySettings> privacy(Session session) async {
    final data = await _request('GET', '/api/v1/privacy', session: session);
    return PrivacySettings.fromJson(data);
  }

  Future<PrivacySettings> updatePrivacy(
    Session session,
    PrivacySettings settings,
  ) async {
    final data = await _request(
      'PATCH',
      '/api/v1/privacy',
      session: session,
      body: settings.toJson(),
    );
    return PrivacySettings.fromJson(data);
  }

  Future<List<AppUser>> blockedUsers(Session session) async {
    final data = await _request('GET', '/api/v1/blocks', session: session);
    return (data['users'] as List<dynamic>)
        .map((value) => AppUser.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> blockUser(Session session, String userId) async {
    await _request('POST', '/api/v1/blocks/$userId', session: session);
  }

  Future<void> unblockUser(Session session, String userId) async {
    await _request('DELETE', '/api/v1/blocks/$userId', session: session);
  }

  Future<void> createReport({
    required Session session,
    required String targetType,
    required String targetId,
    required String reason,
    String details = '',
  }) async {
    await _request(
      'POST',
      '/api/v1/reports',
      session: session,
      body: {
        'target_type': targetType,
        'target_id': targetId,
        'reason': reason,
        'details': details,
      },
    );
  }

  Future<MediaSource> createWebDAVSource({
    required Session session,
    required String name,
    required String baseUrl,
    required String username,
    required String password,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/sources/webdav',
      session: session,
      body: {
        'name': name,
        'base_url': baseUrl,
        'username': username,
        'password': password,
      },
    );
    return MediaSource.fromJson(data);
  }

  Future<MediaSource> createEmbySource({
    required Session session,
    required String name,
    required String baseUrl,
    required String username,
    required String password,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/sources/emby',
      session: session,
      body: {
        'name': name,
        'base_url': baseUrl,
        'username': username,
        'password': password,
      },
    );
    return MediaSource.fromJson(data);
  }

  Future<List<MediaSource>> listMediaSources(Session session) async {
    final data = await _request('GET', '/api/v1/sources', session: session);
    return (data['sources'] as List<dynamic>)
        .map((value) => MediaSource.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<MediaFile>> browseMediaSource(
    Session session,
    String sourceId,
    String path,
  ) async {
    final uri = Uri(
      path: '/api/v1/sources/$sourceId/files',
      queryParameters: {'path': path},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['files'] as List<dynamic>)
        .map((value) => MediaFile.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<MediaPlaybackTicket> issueMediaTicket(
    Session session,
    String sourceId,
    String path,
  ) async {
    final data = await _request(
      'POST',
      '/api/v1/sources/$sourceId/ticket',
      session: session,
      body: {'path': path},
    );
    final rawUrl = data['url'] as String;
    final resolved = Uri.parse(baseUrl).resolve(rawUrl).toString();
    return MediaPlaybackTicket(
      url: resolved,
      expiresAt: (data['expires_at'] as num).toInt(),
    );
  }

  Future<MediaPlaybackTicket> renewRoomMediaTicket(
    Session session,
    String roomCode,
  ) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms/$roomCode/media-ticket',
      session: session,
    );
    final resolved = Uri.parse(baseUrl)
        .resolve(data['url'] as String)
        .toString();
    return MediaPlaybackTicket(
      url: resolved,
      expiresAt: (data['expires_at'] as num).toInt(),
    );
  }

  Future<List<ChatMessage>> roomMessages(
    Session session,
    String roomCode, {
    int before = 0,
    int limit = 50,
  }) async {
    final uri = Uri(
      path: '/api/v1/rooms/$roomCode/messages',
      queryParameters: {'before': '$before', 'limit': '$limit'},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['messages'] as List<dynamic>)
        .map((value) => ChatMessage.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<MediaFile>> roomSubtitles(
    Session session,
    String roomCode,
  ) async {
    final data = await _request(
      'GET',
      '/api/v1/rooms/$roomCode/subtitles',
      session: session,
    );
    return (data['files'] as List<dynamic>)
        .map((value) => MediaFile.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<MediaPlaybackTicket> roomSubtitleTicket(
    Session session,
    String roomCode,
    String path,
  ) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms/$roomCode/subtitle-ticket',
      session: session,
      body: {'path': path},
    );
    return MediaPlaybackTicket(
      url: Uri.parse(baseUrl).resolve(data['url'] as String).toString(),
      expiresAt: (data['expires_at'] as num).toInt(),
    );
  }

  Future<void> deleteMediaSource(Session session, String sourceId) async {
    await _request('DELETE', '/api/v1/sources/$sourceId', session: session);
  }

  Future<Favorite> addFavorite({
    required Session session,
    required String sourceId,
    required MediaFile file,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/favorites',
      session: session,
      body: {
        'source_id': sourceId,
        'media_path': file.path,
        'title': file.name,
        'content_type': file.contentType,
        'size': file.size,
      },
    );
    return Favorite.fromJson(data);
  }

  Future<List<Favorite>> favorites(Session session) async {
    final data = await _request('GET', '/api/v1/favorites', session: session);
    return (data['favorites'] as List<dynamic>)
        .map((value) => Favorite.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> deleteFavorite(Session session, int id) async {
    await _request('DELETE', '/api/v1/favorites/$id', session: session);
  }

  Future<List<WatchRecord>> watchHistory(Session session) async {
    final data = await _request('GET', '/api/v1/history', session: session);
    return (data['records'] as List<dynamic>)
        .map((value) => WatchRecord.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<DanmakuMessage>> roomDanmaku(
    Session session,
    String roomCode, {
    int episode = 0,
  }) async {
    final uri = Uri(
      path: '/api/v1/rooms/$roomCode/danmaku',
      queryParameters: {'episode': '$episode', 'limit': '2000'},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['messages'] as List<dynamic>)
        .map((value) => DanmakuMessage.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<MetadataResult>> searchMetadata(
    Session session,
    String query, {
    String language = 'zh-CN',
  }) async {
    final uri = Uri(
      path: '/api/v1/metadata/search',
      queryParameters: {'q': query, 'language': language},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['results'] as List<dynamic>)
        .map((value) => MetadataResult.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<SocialProfile>> searchSocialUsers(
    Session session,
    String query,
  ) async {
    final uri = Uri(
      path: '/api/v1/social/users',
      queryParameters: {'q': query},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['users'] as List<dynamic>)
        .map((value) => SocialProfile.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> followUser(Session session, String id) =>
      _request('POST', '/api/v1/social/users/$id/follow', session: session);
  Future<void> unfollowUser(Session session, String id) =>
      _request('DELETE', '/api/v1/social/users/$id/follow', session: session);
  Future<Conversation> createConversation(
    Session session,
    String userId,
  ) async {
    final data = await _request(
      'POST',
      '/api/v1/social/conversations',
      session: session,
      body: {'user_id': userId},
    );
    return Conversation.fromJson(data);
  }

  Future<List<Conversation>> conversations(Session session) async {
    final data = await _request(
      'GET',
      '/api/v1/social/conversations',
      session: session,
    );
    return (data['conversations'] as List<dynamic>)
        .map((value) => Conversation.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<DirectMessage>> directMessages(Session session, int id) async {
    final data = await _request(
      'GET',
      '/api/v1/social/conversations/$id/messages',
      session: session,
    );
    return (data['messages'] as List<dynamic>)
        .map((value) => DirectMessage.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<DirectMessage> sendDirectMessage(
    Session session,
    int id,
    String body,
  ) async {
    final data = await _request(
      'POST',
      '/api/v1/social/conversations/$id/messages',
      session: session,
      body: {'body': body},
    );
    return DirectMessage.fromJson(data);
  }

  Future<int> directUnread(Session session) async {
    final data = await _request(
      'GET',
      '/api/v1/social/unread',
      session: session,
    );
    return (data['count'] as num).toInt();
  }

  Future<Uri> socialSocketUri(Session session) async {
    final ticket = await _request(
      'POST',
      '/api/v1/social/socket-ticket',
      session: session,
    );
    final httpUri = Uri.parse(baseUrl);
    return httpUri.replace(
      scheme: httpUri.scheme == 'https' ? 'wss' : 'ws',
      path: '/ws/v1/social',
      queryParameters: {'ticket': ticket['ticket'] as String},
    );
  }

  Future<Couple?> coupleInfo(Session session) async {
    final data = await _request('GET', '/api/v1/couple', session: session);
    if (data['has_couple'] != true) return null;
    return Couple.fromJson(data['couple'] as Map<String, dynamic>);
  }

  Future<void> requestCouple(Session session, String userId) async {
    await _request(
      'POST',
      '/api/v1/couple/requests',
      session: session,
      body: {'user_id': userId},
    );
  }

  Future<List<CoupleRequest>> coupleRequests(Session session) async {
    final data = await _request(
      'GET',
      '/api/v1/couple/requests',
      session: session,
    );
    return (data['requests'] as List<dynamic>)
        .map((v) => CoupleRequest.fromJson(v as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> respondCouple(Session session, int id, bool accept) async {
    await _request(
      'POST',
      '/api/v1/couple/requests/$id/respond',
      session: session,
      body: {'accept': accept},
    );
  }

  Future<void> separateCouple(Session session) async {
    await _request('POST', '/api/v1/couple/separate', session: session);
  }

  Future<void> restoreCouple(Session session) async {
    await _request('POST', '/api/v1/couple/restore', session: session);
  }

  Future<List<CoupleMoment>> coupleMoments(Session session) async {
    final data = await _request(
      'GET',
      '/api/v1/couple/moments',
      session: session,
    );
    return (data['moments'] as List<dynamic>)
        .map((v) => CoupleMoment.fromJson(v as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> addCoupleMoment(Session session, String body) async {
    await _request(
      'POST',
      '/api/v1/couple/moments',
      session: session,
      body: {'body': body},
    );
  }

  Future<List<RTCIceServerConfig>> rtcConfig(
    Session session,
    String roomCode,
  ) async {
    final data = await _request(
      'GET',
      '/api/v1/rooms/$roomCode/rtc-config',
      session: session,
    );
    return (data['ice_servers'] as List<dynamic>)
        .map((v) => RTCIceServerConfig.fromJson(v as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> deleteWatchRecord(Session session, int id) async {
    await _request('DELETE', '/api/v1/history/$id', session: session);
  }

  Future<void> updateWatchProgress(
    Session session,
    String roomCode,
    double durationSeconds,
  ) async {
    await _request(
      'PUT',
      '/api/v1/rooms/$roomCode/watch-progress',
      session: session,
      body: {'duration_seconds': durationSeconds},
    );
  }

  Future<Room> createRoom({
    required Session session,
    required String name,
    String sourceUrl = '',
    String mediaSourceId = '',
    String mediaPath = '',
    double startPosition = 0,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms',
      session: session,
      body: {
        'name': name,
        'source_url': sourceUrl,
        'media_source_id': mediaSourceId,
        'media_path': mediaPath,
        'max_members': 8,
        'start_position': startPosition,
      },
    );
    return _roomFromJson(data);
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
    return _roomFromJson(data);
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

  Room _roomFromJson(Map<String, dynamic> data) {
    final room = Room.fromJson(data);
    if (room.sourceUrl.isEmpty) return room;
    return room.withSourceUrl(
      Uri.parse(baseUrl).resolve(room.sourceUrl).toString(),
    );
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
