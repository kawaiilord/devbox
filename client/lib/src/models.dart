class AppUser {
  const AppUser({
    required this.id,
    required this.displayName,
    required this.email,
    required this.emailVerified,
    this.isAdmin = false,
    this.vipExpiresAt = 0,
  });

  final String id;
  final String displayName;
  final String email;
  final bool emailVerified;
  final bool isAdmin;
  final int vipExpiresAt;

  factory AppUser.fromJson(Map<String, dynamic> json) => AppUser(
    id: json['id'] as String,
    displayName: json['display_name'] as String,
    email: json['email']?.toString() ?? '',
    emailVerified: json['email_verified'] as bool? ?? false,
    isAdmin: json['is_admin'] as bool? ?? false,
    vipExpiresAt: (json['vip_expires_at'] as num?)?.toInt() ?? 0,
  );
}

class VipPlan {
  const VipPlan({
    required this.id,
    required this.title,
    required this.priceMinor,
    required this.originalPriceMinor,
    required this.durationDays,
    required this.lifetime,
    required this.popular,
  });
  final String id;
  final String title;
  final int priceMinor;
  final int originalPriceMinor;
  final int durationDays;
  final bool lifetime;
  final bool popular;
  factory VipPlan.fromJson(Map<String, dynamic> json) => VipPlan(
    id: json['id'] as String,
    title: json['title'] as String,
    priceMinor: (json['price_minor'] as num).toInt(),
    originalPriceMinor: (json['original_price_minor'] as num?)?.toInt() ?? 0,
    durationDays: (json['duration_days'] as num).toInt(),
    lifetime: json['lifetime'] as bool? ?? false,
    popular: json['popular'] as bool? ?? false,
  );
}

class VipInfo {
  const VipInfo({
    required this.announcement,
    required this.paymentMethod,
    required this.plans,
  });
  final String announcement;
  final String paymentMethod;
  final List<VipPlan> plans;
  factory VipInfo.fromJson(Map<String, dynamic> json) => VipInfo(
    announcement: json['announcement']?.toString() ?? '',
    paymentMethod: json['payment_method']?.toString() ?? 'disabled',
    plans: (json['plans'] as List<dynamic>)
        .map((value) => VipPlan.fromJson(value as Map<String, dynamic>))
        .toList(growable: false),
  );
}

class MembershipStatus {
  const MembershipStatus({required this.active, required this.vipExpiresAt});
  final bool active;
  final int vipExpiresAt;
  factory MembershipStatus.fromJson(Map<String, dynamic> json) =>
      MembershipStatus(
        active: json['active'] as bool,
        vipExpiresAt: (json['vip_expires_at'] as num?)?.toInt() ?? 0,
      );
}

class VipOrder {
  const VipOrder({
    required this.orderNo,
    required this.planTitle,
    required this.amountMinor,
    required this.status,
    required this.checkoutUrl,
    required this.qrExpiresAt,
  });
  final String orderNo;
  final String planTitle;
  final int amountMinor;
  final String status;
  final String checkoutUrl;
  final int qrExpiresAt;
  factory VipOrder.fromJson(Map<String, dynamic> json) => VipOrder(
    orderNo: json['order_no'] as String,
    planTitle: json['plan_title'] as String,
    amountMinor: (json['amount_minor'] as num).toInt(),
    status: json['status'] as String,
    checkoutUrl: json['checkout_url']?.toString() ?? '',
    qrExpiresAt: (json['qr_expires_at'] as num).toInt(),
  );
}

class CheckInStatus {
  const CheckInStatus({
    required this.availablePoints,
    required this.totalPoints,
    required this.usedPoints,
    required this.checkedInToday,
    required this.pointsPerCheckIn,
    required this.pointsPerVipDay,
    required this.totalCheckIns,
    required this.consecutiveDays,
  });
  final int availablePoints;
  final int totalPoints;
  final int usedPoints;
  final bool checkedInToday;
  final int pointsPerCheckIn;
  final int pointsPerVipDay;
  final int totalCheckIns;
  final int consecutiveDays;
  factory CheckInStatus.fromJson(Map<String, dynamic> json) => CheckInStatus(
    availablePoints: (json['available_points'] as num).toInt(),
    totalPoints: (json['total_points'] as num).toInt(),
    usedPoints: (json['used_points'] as num).toInt(),
    checkedInToday: json['checked_in_today'] as bool,
    pointsPerCheckIn: (json['points_per_check_in'] as num).toInt(),
    pointsPerVipDay: (json['points_per_vip_day'] as num).toInt(),
    totalCheckIns: (json['total_check_ins'] as num).toInt(),
    consecutiveDays: (json['consecutive_days'] as num).toInt(),
  );
}

class PointsTransaction {
  const PointsTransaction({
    required this.id,
    required this.change,
    required this.balanceAfter,
    required this.type,
    required this.createdAt,
  });
  final int id;
  final int change;
  final int balanceAfter;
  final String type;
  final int createdAt;
  factory PointsTransaction.fromJson(Map<String, dynamic> json) =>
      PointsTransaction(
        id: (json['id'] as num).toInt(),
        change: (json['change'] as num).toInt(),
        balanceAfter: (json['balance_after'] as num).toInt(),
        type: json['type'] as String,
        createdAt: (json['created_at'] as num).toInt(),
      );
}

class PrivacySettings {
  const PrivacySettings({
    required this.allowRoomChat,
    required this.allowPrivateChat,
    required this.allowProfileFind,
    required this.showWatchActivity,
  });

  final bool allowRoomChat;
  final bool allowPrivateChat;
  final bool allowProfileFind;
  final bool showWatchActivity;

  factory PrivacySettings.fromJson(Map<String, dynamic> json) =>
      PrivacySettings(
        allowRoomChat: json['allow_room_chat'] as bool? ?? true,
        allowPrivateChat: json['allow_private_chat'] as bool? ?? true,
        allowProfileFind: json['allow_profile_find'] as bool? ?? true,
        showWatchActivity: json['show_watch_activity'] as bool? ?? true,
      );

  Map<String, dynamic> toJson() => {
    'allow_room_chat': allowRoomChat,
    'allow_private_chat': allowPrivateChat,
    'allow_profile_find': allowProfileFind,
    'show_watch_activity': showWatchActivity,
  };
}

class UserDevice {
  const UserDevice({
    required this.id,
    required this.label,
    required this.platform,
    required this.current,
    required this.lastSeen,
  });

  final String id;
  final String label;
  final String platform;
  final bool current;
  final int lastSeen;

  factory UserDevice.fromJson(Map<String, dynamic> json) => UserDevice(
    id: json['id'] as String,
    label: json['label'] as String,
    platform: json['platform'] as String,
    current: json['current'] as bool? ?? false,
    lastSeen: (json['last_seen'] as num).toInt(),
  );
}

class MediaSource {
  const MediaSource({
    required this.id,
    required this.type,
    required this.name,
    required this.baseUrl,
  });

  final String id;
  final String type;
  final String name;
  final String baseUrl;

  factory MediaSource.fromJson(Map<String, dynamic> json) => MediaSource(
    id: json['id'] as String,
    type: json['type'] as String,
    name: json['name'] as String,
    baseUrl: json['base_url'] as String,
  );
}

class MediaFile {
  const MediaFile({
    required this.name,
    required this.path,
    required this.isDirectory,
    required this.size,
    required this.contentType,
  });

  final String name;
  final String path;
  final bool isDirectory;
  final int size;
  final String contentType;

  factory MediaFile.fromJson(Map<String, dynamic> json) => MediaFile(
    name: json['name'] as String,
    path: json['path'] as String,
    isDirectory: json['is_directory'] as bool,
    size: (json['size'] as num).toInt(),
    contentType: json['content_type']?.toString() ?? '',
  );
}

class MediaPlaybackTicket {
  const MediaPlaybackTicket({required this.url, required this.expiresAt});

  final String url;
  final int expiresAt;
}

class Favorite {
  const Favorite({
    required this.id,
    required this.sourceId,
    required this.sourceType,
    required this.sourceName,
    required this.mediaPath,
    required this.title,
    required this.contentType,
    required this.size,
    required this.updatedAt,
  });

  final int id;
  final String sourceId;
  final String sourceType;
  final String sourceName;
  final String mediaPath;
  final String title;
  final String contentType;
  final int size;
  final int updatedAt;

  factory Favorite.fromJson(Map<String, dynamic> json) => Favorite(
    id: (json['id'] as num).toInt(),
    sourceId: json['source_id'] as String,
    sourceType: json['source_type'] as String,
    sourceName: json['source_name'] as String,
    mediaPath: json['media_path'] as String,
    title: json['title'] as String,
    contentType: json['content_type']?.toString() ?? '',
    size: (json['size'] as num).toInt(),
    updatedAt: (json['updated_at'] as num).toInt(),
  );
}

class WatchRecord {
  const WatchRecord({
    required this.id,
    required this.sourceId,
    required this.mediaPath,
    required this.title,
    required this.positionSeconds,
    required this.durationSeconds,
    required this.episode,
    required this.completed,
    required this.companionCount,
    required this.roomCode,
    required this.resumable,
    required this.watchedAt,
  });

  final int id;
  final String sourceId;
  final String mediaPath;
  final String title;
  final double positionSeconds;
  final double durationSeconds;
  final int episode;
  final bool completed;
  final int companionCount;
  final String roomCode;
  final bool resumable;
  final int watchedAt;

  factory WatchRecord.fromJson(Map<String, dynamic> json) => WatchRecord(
    id: (json['id'] as num).toInt(),
    sourceId: json['source_id']?.toString() ?? '',
    mediaPath: json['media_path']?.toString() ?? '',
    title: json['title'] as String,
    positionSeconds: (json['position_seconds'] as num).toDouble(),
    durationSeconds: (json['duration_seconds'] as num).toDouble(),
    episode: (json['episode'] as num).toInt(),
    completed: json['completed'] as bool,
    companionCount: (json['companion_count'] as num).toInt(),
    roomCode: json['room_code']?.toString() ?? '',
    resumable: json['resumable'] as bool? ?? false,
    watchedAt: (json['watched_at'] as num).toInt(),
  );
}

class DanmakuMessage {
  const DanmakuMessage({
    required this.id,
    required this.userId,
    required this.displayName,
    required this.body,
    required this.positionSeconds,
    required this.color,
    required this.mode,
    required this.createdAt,
  });

  final int id;
  final String userId;
  final String displayName;
  final String body;
  final double positionSeconds;
  final int color;
  final String mode;
  final int createdAt;

  factory DanmakuMessage.fromJson(Map<String, dynamic> json) => DanmakuMessage(
    id: (json['id'] as num).toInt(),
    userId: json['user_id'] as String,
    displayName: json['display_name'] as String,
    body: json['body'] as String,
    positionSeconds: (json['position_seconds'] as num).toDouble(),
    color: (json['color'] as num).toInt(),
    mode: json['mode'] as String,
    createdAt: (json['created_at'] as num).toInt(),
  );
}

class MetadataResult {
  const MetadataResult({
    required this.id,
    required this.mediaType,
    required this.title,
    required this.originalTitle,
    required this.overview,
    required this.releaseDate,
    required this.posterUrl,
    required this.rating,
  });

  final int id;
  final String mediaType;
  final String title;
  final String originalTitle;
  final String overview;
  final String releaseDate;
  final String posterUrl;
  final double rating;

  factory MetadataResult.fromJson(Map<String, dynamic> json) => MetadataResult(
    id: (json['id'] as num).toInt(),
    mediaType: json['media_type'] as String,
    title: json['title'] as String,
    originalTitle: json['original_title']?.toString() ?? '',
    overview: json['overview']?.toString() ?? '',
    releaseDate: json['release_date']?.toString() ?? '',
    posterUrl: json['poster_url']?.toString() ?? '',
    rating: (json['rating'] as num).toDouble(),
  );
}

class Review {
  const Review({
    required this.id,
    required this.author,
    required this.targetType,
    required this.targetId,
    required this.title,
    required this.rating,
    required this.content,
    required this.imageUrls,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final SocialProfile author;
  final String targetType;
  final String targetId;
  final String title;
  final int rating;
  final String content;
  final List<String> imageUrls;
  final int createdAt;
  final int updatedAt;

  factory Review.fromJson(Map<String, dynamic> json) => Review(
    id: (json['id'] as num).toInt(),
    author: SocialProfile.fromJson(json['author'] as Map<String, dynamic>),
    targetType: json['target_type'] as String,
    targetId: json['target_id'] as String,
    title: json['title'] as String,
    rating: (json['rating'] as num).toInt(),
    content: json['content'] as String,
    imageUrls: (json['image_urls'] as List<dynamic>? ?? const [])
        .map((value) => value.toString())
        .toList(growable: false),
    createdAt: (json['created_at'] as num).toInt(),
    updatedAt: (json['updated_at'] as num).toInt(),
  );
}

class ReviewComment {
  const ReviewComment({
    required this.id,
    required this.reviewId,
    required this.author,
    required this.body,
    required this.createdAt,
  });

  final int id;
  final int reviewId;
  final SocialProfile author;
  final String body;
  final int createdAt;

  factory ReviewComment.fromJson(Map<String, dynamic> json) => ReviewComment(
    id: (json['id'] as num).toInt(),
    reviewId: (json['review_id'] as num).toInt(),
    author: SocialProfile.fromJson(json['author'] as Map<String, dynamic>),
    body: json['body'] as String,
    createdAt: (json['created_at'] as num).toInt(),
  );
}

class ObjectUpload {
  const ObjectUpload({
    required this.objectKey,
    required this.uploadUrl,
    required this.expiresAt,
  });

  final String objectKey;
  final String uploadUrl;
  final int expiresAt;

  factory ObjectUpload.fromJson(Map<String, dynamic> json) => ObjectUpload(
    objectKey: json['object_key'] as String,
    uploadUrl: json['upload_url'] as String,
    expiresAt: (json['expires_at'] as num).toInt(),
  );
}

class SocialProfile {
  const SocialProfile({
    required this.id,
    required this.displayName,
    required this.signature,
    required this.following,
    required this.followsViewer,
    required this.followerCount,
    required this.followingCount,
  });
  final String id;
  final String displayName;
  final String signature;
  final bool following;
  final bool followsViewer;
  final int followerCount;
  final int followingCount;
  factory SocialProfile.fromJson(Map<String, dynamic> json) => SocialProfile(
    id: json['id'] as String,
    displayName: json['display_name'] as String,
    signature: json['signature']?.toString() ?? '',
    following: json['following'] as bool? ?? false,
    followsViewer: json['follows_viewer'] as bool? ?? false,
    followerCount: (json['follower_count'] as num).toInt(),
    followingCount: (json['following_count'] as num).toInt(),
  );
}

class Conversation {
  const Conversation({
    required this.id,
    required this.peer,
    required this.lastMessage,
    required this.lastMessageAt,
    required this.unreadCount,
  });
  final int id;
  final SocialProfile peer;
  final String lastMessage;
  final int lastMessageAt;
  final int unreadCount;
  factory Conversation.fromJson(Map<String, dynamic> json) => Conversation(
    id: (json['id'] as num).toInt(),
    peer: SocialProfile.fromJson(json['peer'] as Map<String, dynamic>),
    lastMessage: json['last_message']?.toString() ?? '',
    lastMessageAt: (json['last_message_at'] as num?)?.toInt() ?? 0,
    unreadCount: (json['unread_count'] as num?)?.toInt() ?? 0,
  );
}

class DirectMessage {
  const DirectMessage({
    required this.id,
    required this.conversationId,
    required this.senderId,
    required this.body,
    required this.createdAt,
  });
  final int id;
  final int conversationId;
  final String senderId;
  final String body;
  final int createdAt;
  factory DirectMessage.fromJson(Map<String, dynamic> json) => DirectMessage(
    id: (json['id'] as num).toInt(),
    conversationId: (json['conversation_id'] as num).toInt(),
    senderId: json['sender_id'] as String,
    body: json['body'] as String,
    createdAt: (json['created_at'] as num).toInt(),
  );
}

class Couple {
  const Couple({
    required this.id,
    required this.partner,
    required this.status,
    required this.boundAt,
    required this.separatedAt,
    required this.coolingPeriodEnd,
  });
  final int id;
  final SocialProfile partner;
  final String status;
  final int boundAt;
  final int separatedAt;
  final int coolingPeriodEnd;
  factory Couple.fromJson(Map<String, dynamic> json) => Couple(
    id: (json['id'] as num).toInt(),
    partner: SocialProfile.fromJson(json['partner'] as Map<String, dynamic>),
    status: json['status'] as String,
    boundAt: (json['bound_at'] as num).toInt(),
    separatedAt: (json['separated_at'] as num?)?.toInt() ?? 0,
    coolingPeriodEnd: (json['cooling_period_end'] as num?)?.toInt() ?? 0,
  );
}

class CoupleRequest {
  const CoupleRequest({
    required this.id,
    required this.requester,
    required this.status,
    required this.createdAt,
  });
  final int id;
  final SocialProfile requester;
  final String status;
  final int createdAt;
  factory CoupleRequest.fromJson(Map<String, dynamic> json) => CoupleRequest(
    id: (json['id'] as num).toInt(),
    requester: SocialProfile.fromJson(
      json['requester'] as Map<String, dynamic>,
    ),
    status: json['status'] as String,
    createdAt: (json['created_at'] as num).toInt(),
  );
}

class CoupleMoment {
  const CoupleMoment({
    required this.id,
    required this.author,
    required this.body,
    required this.createdAt,
  });
  final int id;
  final SocialProfile author;
  final String body;
  final int createdAt;
  factory CoupleMoment.fromJson(Map<String, dynamic> json) => CoupleMoment(
    id: (json['id'] as num).toInt(),
    author: SocialProfile.fromJson(json['author'] as Map<String, dynamic>),
    body: json['body'] as String,
    createdAt: (json['created_at'] as num).toInt(),
  );
}

class RTCIceServerConfig {
  const RTCIceServerConfig({
    required this.urls,
    required this.username,
    required this.credential,
  });
  final List<String> urls;
  final String username;
  final String credential;
  factory RTCIceServerConfig.fromJson(Map<String, dynamic> json) =>
      RTCIceServerConfig(
        urls: (json['urls'] as List<dynamic>)
            .map((v) => v.toString())
            .toList(growable: false),
        username: json['username']?.toString() ?? '',
        credential: json['credential']?.toString() ?? '',
      );
}

class ChatMessage {
  const ChatMessage({
    required this.id,
    required this.roomCode,
    required this.userId,
    required this.displayName,
    required this.body,
    required this.createdAt,
  });

  final int id;
  final String roomCode;
  final String userId;
  final String displayName;
  final String body;
  final int createdAt;

  factory ChatMessage.fromJson(Map<String, dynamic> json) => ChatMessage(
    id: (json['id'] as num).toInt(),
    roomCode: json['room_code'] as String,
    userId: json['user_id'] as String,
    displayName: json['display_name'] as String,
    body: json['body'] as String,
    createdAt: (json['created_at'] as num).toInt(),
  );
}

class Session {
  Session({
    required this.accessToken,
    required this.refreshToken,
    required this.tokenType,
    required this.expiresIn,
    required this.user,
  });

  String accessToken;
  String refreshToken;
  String tokenType;
  int expiresIn;
  final AppUser user;

  factory Session.fromJson(Map<String, dynamic> json) => Session(
    accessToken: json['access_token'] as String,
    refreshToken: json['refresh_token'] as String,
    tokenType: json['token_type']?.toString() ?? 'bearer',
    expiresIn: (json['expires_in'] as num).toInt(),
    user: AppUser.fromJson(json['user'] as Map<String, dynamic>),
  );

  void replaceTokens(Session next) {
    accessToken = next.accessToken;
    refreshToken = next.refreshToken;
    tokenType = next.tokenType;
    expiresIn = next.expiresIn;
  }
}

class RoomMember {
  const RoomMember({
    required this.userId,
    required this.displayName,
    required this.joinedAt,
  });

  final String userId;
  final String displayName;
  final int joinedAt;

  factory RoomMember.fromJson(Map<String, dynamic> json) => RoomMember(
    userId: json['user_id'] as String,
    displayName: json['display_name'] as String,
    joinedAt: (json['joined_at'] as num).toInt(),
  );
}

class PlaybackSnapshot {
  const PlaybackSnapshot({
    required this.position,
    required this.playing,
    required this.speed,
    required this.episode,
    required this.positionTimestamp,
    required this.sourceVersion,
  });

  final double position;
  final bool playing;
  final double speed;
  final int episode;
  final int positionTimestamp;
  final int sourceVersion;

  factory PlaybackSnapshot.fromJson(Map<String, dynamic> json) =>
      PlaybackSnapshot(
        position: (json['position'] as num).toDouble(),
        playing: json['playing'] as bool,
        speed: (json['speed'] as num).toDouble(),
        episode: (json['episode'] as num).toInt(),
        positionTimestamp: (json['position_ts'] as num).toInt(),
        sourceVersion: (json['source_version'] as num).toInt(),
      );
}

class Room {
  const Room({
    required this.code,
    required this.name,
    required this.ownerId,
    required this.sourceUrl,
    required this.mediaSourceId,
    required this.mediaPath,
    required this.maxMembers,
    required this.expiresAt,
    required this.members,
    required this.playback,
    this.closed = false,
  });

  final String code;
  final String name;
  final String ownerId;
  final String sourceUrl;
  final String mediaSourceId;
  final String mediaPath;
  final int maxMembers;
  final int expiresAt;
  final List<RoomMember> members;
  final PlaybackSnapshot playback;
  final bool closed;

  factory Room.fromJson(Map<String, dynamic> json) => Room(
    code: json['code'] as String,
    name: json['name'] as String,
    ownerId: json['owner_id'] as String,
    sourceUrl: json['source_url'] as String,
    mediaSourceId: json['media_source_id']?.toString() ?? '',
    mediaPath: json['media_path']?.toString() ?? '',
    maxMembers: (json['max_members'] as num).toInt(),
    expiresAt: (json['expires_at'] as num).toInt(),
    members: (json['members'] as List<dynamic>)
        .map((item) => RoomMember.fromJson(item as Map<String, dynamic>))
        .toList(growable: false),
    playback: PlaybackSnapshot.fromJson(
      json['playback'] as Map<String, dynamic>,
    ),
    closed: json['closed'] as bool? ?? false,
  );

  Room withSourceUrl(String value) => Room(
    code: code,
    name: name,
    ownerId: ownerId,
    sourceUrl: value,
    mediaSourceId: mediaSourceId,
    mediaPath: mediaPath,
    maxMembers: maxMembers,
    expiresAt: expiresAt,
    members: members,
    playback: playback,
    closed: closed,
  );

  Room withPlayback(PlaybackSnapshot value) => Room(
    code: code,
    name: name,
    ownerId: ownerId,
    sourceUrl: sourceUrl,
    mediaSourceId: mediaSourceId,
    mediaPath: mediaPath,
    maxMembers: maxMembers,
    expiresAt: expiresAt,
    members: members,
    playback: value,
    closed: closed,
  );
}

class RoomEnvelope {
  const RoomEnvelope({
    required this.type,
    required this.sequence,
    required this.serverTimestamp,
    required this.payload,
    this.fromUserId = '',
  });

  final String type;
  final int sequence;
  final int serverTimestamp;
  final Map<String, dynamic> payload;
  final String fromUserId;

  factory RoomEnvelope.fromJson(Map<String, dynamic> json) => RoomEnvelope(
    type: json['type'] as String,
    sequence: (json['seq'] as num).toInt(),
    serverTimestamp: (json['ts'] as num).toInt(),
    payload: json['payload'] as Map<String, dynamic>,
    fromUserId: json['from']?.toString() ?? '',
  );
}
