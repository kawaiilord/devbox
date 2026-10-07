class AppUser {
  const AppUser({required this.id, required this.displayName});

  final String id;
  final String displayName;

  factory AppUser.fromJson(Map<String, dynamic> json) => AppUser(
    id: json['id'] as String,
    displayName: json['display_name'] as String,
  );
}

class Session {
  const Session({required this.accessToken, required this.user});

  final String accessToken;
  final AppUser user;

  factory Session.fromJson(Map<String, dynamic> json) => Session(
    accessToken: json['access_token'] as String,
    user: AppUser.fromJson(json['user'] as Map<String, dynamic>),
  );
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
    required this.maxMembers,
    required this.expiresAt,
    required this.members,
    required this.playback,
  });

  final String code;
  final String name;
  final String ownerId;
  final String sourceUrl;
  final int maxMembers;
  final int expiresAt;
  final List<RoomMember> members;
  final PlaybackSnapshot playback;

  factory Room.fromJson(Map<String, dynamic> json) => Room(
    code: json['code'] as String,
    name: json['name'] as String,
    ownerId: json['owner_id'] as String,
    sourceUrl: json['source_url'] as String,
    maxMembers: (json['max_members'] as num).toInt(),
    expiresAt: (json['expires_at'] as num).toInt(),
    members: (json['members'] as List<dynamic>)
        .map((item) => RoomMember.fromJson(item as Map<String, dynamic>))
        .toList(growable: false),
    playback: PlaybackSnapshot.fromJson(
      json['playback'] as Map<String, dynamic>,
    ),
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
