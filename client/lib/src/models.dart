class AppUser {
  const AppUser({
    required this.id,
    required this.displayName,
    required this.email,
    required this.emailVerified,
  });

  final String id;
  final String displayName;
  final String email;
  final bool emailVerified;

  factory AppUser.fromJson(Map<String, dynamic> json) => AppUser(
    id: json['id'] as String,
    displayName: json['display_name'] as String,
    email: json['email']?.toString() ?? '',
    emailVerified: json['email_verified'] as bool? ?? false,
  );
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
  );
}

class RoomEnvelope {
  const RoomEnvelope({
    required this.type,
    required this.sequence,
    required this.serverTimestamp,
    required this.payload,
  });

  final String type;
  final int sequence;
  final int serverTimestamp;
  final Map<String, dynamic> payload;

  factory RoomEnvelope.fromJson(Map<String, dynamic> json) => RoomEnvelope(
    type: json['type'] as String,
    sequence: (json['seq'] as num).toInt(),
    serverTimestamp: (json['ts'] as num).toInt(),
    payload: json['payload'] as Map<String, dynamic>,
  );
}
