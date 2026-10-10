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
  Future<void> Function(Session session)? onSessionUpdated;
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

  Future<void> requestAccountDeletion(Session session, String reason) =>
      _request(
        'POST',
        '/api/v1/account/deletion',
        session: session,
        body: {'reason': reason, 'confirm': 'DELETE MY ACCOUNT'},
      );
  Future<void> cancelAccountDeletion(Session session) =>
      _request('DELETE', '/api/v1/account/deletion', session: session);
  Future<void> submitCopyrightComplaint({
    required Session session,
    required String claimantName,
    required String claimantEmail,
    required String rightsBasis,
    required String infringementUrl,
    required String roomCode,
    required List<String> evidence,
    required String signatureName,
  }) => _request(
    'POST',
    '/api/v1/copyright-complaints',
    session: session,
    body: {
      'claimant_name': claimantName,
      'claimant_email': claimantEmail,
      'rights_basis': rightsBasis,
      'infringement_url': infringementUrl,
      'room_code': roomCode,
      'evidence': evidence,
      'statement_accurate': true,
      'signature_name': signatureName,
    },
  );

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

  Future<MediaSource> saveQuarkSource({
    required Session session,
    required String cookie,
    String name = '我的夸克网盘',
    String? sourceId,
  }) async {
    final data = await _request(
      sourceId == null ? 'POST' : 'PUT',
      sourceId == null
          ? '/api/v1/sources/quark'
          : '/api/v1/sources/$sourceId/quark-cookie',
      session: session,
      body: {'name': name, 'cookie': cookie},
    );
    return MediaSource.fromJson(data);
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

  Future<VipInfo> vipInfo() async =>
      VipInfo.fromJson(await _request('GET', '/api/v1/vip'));

  Future<MembershipStatus> membership(Session session) async =>
      MembershipStatus.fromJson(
        await _request('GET', '/api/v1/membership', session: session),
      );

  Future<List<AppAnnouncement>> announcements() async {
    final data = await _request('GET', '/api/v1/announcements');
    return (data['announcements'] as List<dynamic>)
        .map((value) => AppAnnouncement.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<VipOrder> createOrder(Session session, String planId) async =>
      VipOrder.fromJson(
        await _request(
          'POST',
          '/api/v1/orders',
          session: session,
          body: {'plan_id': planId},
        ),
      );

  Future<VipOrder> order(Session session, String orderNo) async =>
      VipOrder.fromJson(
        await _request('GET', '/api/v1/orders/$orderNo', session: session),
      );

  Future<List<VipOrder>> orders(Session session) async {
    final data = await _request('GET', '/api/v1/orders', session: session);
    return (data['orders'] as List<dynamic>)
        .map((value) => VipOrder.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<int> redeemActivationCode(Session session, String code) async {
    final data = await _request(
      'POST',
      '/api/v1/activation-codes/redeem',
      session: session,
      body: {'code': code},
    );
    return (data['vip_expires_at'] as num).toInt();
  }

  Future<CheckInStatus> checkInStatus(Session session) async =>
      CheckInStatus.fromJson(
        await _request('GET', '/api/v1/check-ins/status', session: session),
      );

  Future<CheckInStatus> dailyCheckIn(Session session) async =>
      CheckInStatus.fromJson(
        await _request('POST', '/api/v1/check-ins', session: session),
      );

  Future<int> redeemPoints(Session session, int days) async {
    final data = await _request(
      'POST',
      '/api/v1/points/redeem-vip',
      session: session,
      body: {'days': days},
    );
    return (data['vip_expires_at'] as num).toInt();
  }

  Future<List<PointsTransaction>> pointsTransactions(Session session) async {
    final data = await _request(
      'GET',
      '/api/v1/points/transactions',
      session: session,
    );
    return (data['transactions'] as List<dynamic>)
        .map(
          (value) => PointsTransaction.fromJson(value as Map<String, dynamic>),
        )
        .toList(growable: false);
  }

  Future<ObjectUpload> createReviewUpload({
    required Session session,
    required String filename,
    required String contentType,
    required int size,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/reviews/uploads',
      session: session,
      body: {'filename': filename, 'content_type': contentType, 'size': size},
    );
    return ObjectUpload.fromJson(data);
  }

  Future<String> uploadReviewImage({
    required Session session,
    required String filename,
    required String contentType,
    required List<int> bytes,
  }) async {
    final upload = await createReviewUpload(
      session: session,
      filename: filename,
      contentType: contentType,
      size: bytes.length,
    );
    final response = await _client.put(
      Uri.parse(upload.uploadUrl),
      headers: {'Content-Type': contentType},
      body: bytes,
    );
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw const ApiException('图片上传失败');
    }
    return upload.objectKey;
  }

  Future<Review> saveReview({
    required Session session,
    required String targetType,
    required String targetId,
    required String title,
    required int rating,
    required String content,
    List<String> imageKeys = const [],
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/reviews',
      session: session,
      body: {
        'target_type': targetType,
        'target_id': targetId,
        'title': title,
        'rating': rating,
        'content': content,
        'image_keys': imageKeys,
      },
    );
    return Review.fromJson(data);
  }

  Future<List<Review>> reviews(
    Session session,
    String targetType,
    String targetId,
  ) async {
    final uri = Uri(
      path: '/api/v1/reviews',
      queryParameters: {'target_type': targetType, 'target_id': targetId},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['reviews'] as List<dynamic>)
        .map((value) => Review.fromJson(value as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<void> deleteReview(Session session, int id) =>
      _request('DELETE', '/api/v1/reviews/$id', session: session);

  Future<ReviewComment> addReviewComment(
    Session session,
    int reviewId,
    String body,
  ) async {
    final data = await _request(
      'POST',
      '/api/v1/reviews/$reviewId/comments',
      session: session,
      body: {'body': body},
    );
    return ReviewComment.fromJson(data);
  }

  Future<List<ReviewComment>> reviewComments(
    Session session,
    int reviewId,
  ) async {
    final data = await _request(
      'GET',
      '/api/v1/reviews/$reviewId/comments',
      session: session,
    );
    return (data['comments'] as List<dynamic>)
        .map((value) => ReviewComment.fromJson(value as Map<String, dynamic>))
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
    Map<String, dynamic>? settings,
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
        if (settings != null) 'settings': settings,
      },
    );
    return _roomFromJson(data);
  }

  Future<Room> joinRoom({
    required Session session,
    required String code,
    String password = '',
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms/${code.trim().toUpperCase()}/join',
      session: session,
      body: password.isEmpty ? null : {'password': password},
    );
    return _roomFromJson(data);
  }

  Future<List<ProviderDescriptor>> providerCatalog() async {
    final data = await _request('GET', '/api/v1/providers');
    return (data['providers'] as List<dynamic>)
        .map((v) => ProviderDescriptor.fromJson(v as Map<String, dynamic>))
        .toList();
  }

  Future<MediaSource> saveProviderSource(
    Session session,
    Map<String, dynamic> values, {
    String? sourceId,
  }) async {
    final kind = platformSourceTypes.contains(values['provider'])
        ? 'platform'
        : 'nas';
    final data = await _request(
      sourceId == null ? 'POST' : 'PUT',
      sourceId == null
          ? '/api/v1/sources/$kind'
          : '/api/v1/sources/$sourceId/$kind',
      session: session,
      body: values,
    );
    return MediaSource.fromJson(data);
  }

  Future<List<MediaFile>> resolvePlatformSource(
    Session session,
    String sourceId,
    String url, {
    int offset = 0,
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/sources/$sourceId/resolve',
      session: session,
      body: {'url': url, 'offset': offset},
    );
    return (data['files'] as List<dynamic>)
        .map((v) => MediaFile.fromJson(v as Map<String, dynamic>))
        .toList();
  }

  Future<({List<PublicRoom> rooms, bool hasMore, int nextOffset})>
  discoverRooms({
    String query = '',
    String category = '',
    String tag = '',
    int offset = 0,
  }) async {
    final uri = Uri(
      path: '/api/v1/public/rooms',
      queryParameters: {
        'q': query,
        'category': category,
        'tag': tag,
        'offset': '$offset',
        'limit': '20',
      },
    );
    final data = await _request('GET', uri.toString());
    return (
      rooms: (data['rooms'] as List<dynamic>)
          .map((v) => PublicRoom.fromJson(v as Map<String, dynamic>))
          .toList(),
      hasMore: data['has_more'] == true,
      nextOffset: (data['next_offset'] as num).toInt(),
    );
  }

  Future<({Session session, Room room})> joinAsGuest({
    required String code,
    required String displayName,
    String password = '',
  }) async {
    final data = await _request(
      'POST',
      '/api/v1/rooms/${code.trim().toUpperCase()}/guest',
      body: {'display_name': displayName, 'password': password},
    );
    return (
      session: Session.fromJson(data['session'] as Map<String, dynamic>),
      room: _roomFromJson(data['room'] as Map<String, dynamic>),
    );
  }

  Future<Room> getRoom(Session session, String code) async => _roomFromJson(
    await _request('GET', '/api/v1/rooms/$code', session: session),
  );

  Future<Room> updateRoomSettings(
    Session session,
    Room room,
    Map<String, dynamic> settings,
  ) async => _roomFromJson(
    await _request(
      'PUT',
      '/api/v1/rooms/${room.code}/settings',
      session: session,
      body: {...settings, 'expected_version': room.features?.version ?? 0},
    ),
  );

  Future<Room> updateRoomMember(
    Session session,
    Room room,
    String userId, {
    String role = 'member',
    RoomPermissions? permissions,
    bool unban = false,
  }) async => _roomFromJson(
    await _request(
      'PUT',
      '/api/v1/rooms/${room.code}/members/$userId',
      session: session,
      body: {
        'expected_version': room.features?.version ?? 0,
        'role': role,
        'permissions': permissions?.toJson(),
        'unban': unban,
      },
    ),
  );

  Future<Room> removeRoomMember(
    Session session,
    Room room,
    String userId,
  ) async => _roomFromJson(
    await _request(
      'DELETE',
      '/api/v1/rooms/${room.code}/members/$userId',
      session: session,
    ),
  );

  Future<Room> addPlaylistItems(
    Session session,
    Room room,
    List<PlaylistEntry> items,
  ) async => _roomFromJson(
    await _request(
      'POST',
      '/api/v1/rooms/${room.code}/playlist',
      session: session,
      body: {
        'expected_version': room.features?.version ?? 0,
        'items': items.map((e) => e.toJson()).toList(),
      },
    ),
  );

  Future<Room> reorderPlaylist(
    Session session,
    Room room,
    List<String> order,
  ) async => _roomFromJson(
    await _request(
      'PUT',
      '/api/v1/rooms/${room.code}/playlist',
      session: session,
      body: {'expected_version': room.features?.version ?? 0, 'order': order},
    ),
  );

  Future<Room> removePlaylistItem(
    Session session,
    Room room,
    String itemId,
  ) async => _roomFromJson(
    await _request(
      'DELETE',
      '/api/v1/rooms/${room.code}/playlist/$itemId',
      session: session,
      body: {'expected_version': room.features?.version ?? 0},
    ),
  );

  Future<Room> addPlaylistSource(
    Session session,
    Room room,
    String itemId,
    PlaylistSource source,
  ) async => _roomFromJson(
    await _request(
      'POST',
      '/api/v1/rooms/${room.code}/playlist/$itemId/sources',
      session: session,
      body: {
        'expected_version': room.features?.version ?? 0,
        'source': source.toJson(),
      },
    ),
  );

  Future<Room> selectPlaylistMedia(
    Session session,
    Room room, {
    String itemId = '',
    String sourceId = '',
    String? variantId,
    String direction = '',
    bool auto = false,
  }) async => _roomFromJson(
    await _request(
      'POST',
      '/api/v1/rooms/${room.code}/playback/select',
      session: session,
      body: {
        'expected_version': room.features?.version ?? 0,
        'item_id': itemId,
        'source_id': sourceId,
        ...(variantId == null ? {} : {'variant_id': variantId}),
        'direction': direction,
        'auto': auto,
        'expected_item_id': room.features?.activeItemId ?? '',
      },
    ),
  );

  Future<List<MediaVariant>> roomMediaVariants(
    Session session,
    Room room, {
    String itemId = '',
    String sourceId = '',
  }) async {
    final uri = Uri(
      path: '/api/v1/rooms/${room.code}/variants',
      queryParameters: {'item_id': itemId, 'source_id': sourceId},
    );
    final data = await _request('GET', uri.toString(), session: session);
    return (data['variants'] as List<dynamic>)
        .map((v) => MediaVariant.fromJson(v as Map<String, dynamic>))
        .toList();
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
    if (response.statusCode == 401 &&
        session != null &&
        session.refreshToken.isNotEmpty &&
        allowRefresh) {
      if (session.accessToken == requestAccessToken) {
        final refreshed = await _refreshSingleFlight(session.refreshToken);
        session.replaceTokens(refreshed);
        await onSessionUpdated?.call(session);
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
