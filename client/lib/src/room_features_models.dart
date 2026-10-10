const platformSourceTypes = {
  'bilibili',
  'youtube',
  'douyin',
  'tiktok',
  'twitch',
  'huya',
  'douyu',
  'acfun',
  'cctv',
};
const nasSourceTypes = {
  'synology',
  'fnos',
  'qnap',
  'nextcloud',
  'seafile',
  'truenas',
};

class ProviderDescriptor {
  const ProviderDescriptor({
    required this.id,
    required this.name,
    required this.kind,
    required this.available,
  });
  final String id, name, kind;
  final bool available;
  factory ProviderDescriptor.fromJson(Map<String, dynamic> j) =>
      ProviderDescriptor(
        id: j['id'] as String,
        name: j['name'] as String,
        kind: j['kind'] as String,
        available: j['available'] == true,
      );
}

class RoomPermissions {
  const RoomPermissions({
    this.playback = false,
    this.playlist = false,
    this.chat = true,
    this.danmaku = true,
    this.voice = true,
  });
  final bool playback, playlist, chat, danmaku, voice;
  factory RoomPermissions.fromJson(Map<String, dynamic> j) => RoomPermissions(
    playback: j['playback'] == true,
    playlist: j['playlist'] == true,
    chat: j['chat'] == true,
    danmaku: j['danmaku'] == true,
    voice: j['voice'] == true,
  );
  Map<String, dynamic> toJson() => {
    'playback': playback,
    'playlist': playlist,
    'chat': chat,
    'danmaku': danmaku,
    'voice': voice,
  };
}

class PlaylistSource {
  const PlaylistSource({
    this.id = '',
    required this.label,
    this.mediaSourceId = '',
    this.mediaPath = '',
    this.url = '',
    this.variantId = '',
    this.isLive = false,
  });
  final bool isLive;
  final String id, label, mediaSourceId, mediaPath, url, variantId;
  factory PlaylistSource.fromJson(Map<String, dynamic> j) => PlaylistSource(
    id: j['id']?.toString() ?? '',
    label: j['label']?.toString() ?? '片源',
    mediaSourceId: j['media_source_id']?.toString() ?? '',
    mediaPath: j['media_path']?.toString() ?? '',
    url: j['url']?.toString() ?? '',
    variantId: j['variant_id']?.toString() ?? '',
    isLive: j['is_live'] == true,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'label': label,
    'media_source_id': mediaSourceId,
    'media_path': mediaPath,
    'url': url,
    'variant_id': variantId,
    'is_live': isLive,
  };
}

class PlaylistEntry {
  const PlaylistEntry({
    this.id = '',
    required this.title,
    required this.sources,
    this.selectedSourceId = '',
    this.episode = 0,
  });
  final String id, title, selectedSourceId;
  final int episode;
  final List<PlaylistSource> sources;
  PlaylistSource? get selectedSource =>
      sources.where((s) => s.id == selectedSourceId).firstOrNull;
  factory PlaylistEntry.fromJson(Map<String, dynamic> j) => PlaylistEntry(
    id: j['id']?.toString() ?? '',
    title: j['title'] as String,
    sources: (j['sources'] as List<dynamic>)
        .map((v) => PlaylistSource.fromJson(v as Map<String, dynamic>))
        .toList(),
    selectedSourceId: j['selected_source_id']?.toString() ?? '',
    episode: (j['episode'] as num?)?.toInt() ?? 0,
  );
  Map<String, dynamic> toJson() => {
    'title': title,
    'sources': sources.map((s) => s.toJson()).toList(),
  };
}

class RoomFeatures {
  const RoomFeatures({
    required this.version,
    required this.visibility,
    required this.description,
    required this.category,
    required this.tags,
    required this.allowGuests,
    required this.passwordProtected,
    required this.permissions,
    required this.memberPermissions,
    required this.bannedIds,
    required this.playlist,
    required this.activeItemId,
    required this.autoNext,
  });
  final int version;
  final String visibility, description, category, activeItemId;
  final List<String> tags, bannedIds;
  final bool allowGuests, passwordProtected, autoNext;
  final RoomPermissions permissions;
  final Map<String, RoomPermissions> memberPermissions;
  final List<PlaylistEntry> playlist;
  PlaylistEntry? get activeItem =>
      playlist.where((item) => item.id == activeItemId).firstOrNull;
  factory RoomFeatures.fromJson(Map<String, dynamic> j) => RoomFeatures(
    version: (j['version'] as num?)?.toInt() ?? 0,
    visibility: j['visibility']?.toString() ?? 'private',
    description: j['description']?.toString() ?? '',
    category: j['category']?.toString() ?? '',
    tags: (j['tags'] as List<dynamic>? ?? []).map((v) => v.toString()).toList(),
    allowGuests: j['allow_guests'] == true,
    passwordProtected: j['password_protected'] == true,
    permissions: RoomPermissions.fromJson(
      j['permissions'] as Map<String, dynamic>? ?? {},
    ),
    memberPermissions: (j['member_permissions'] as Map<String, dynamic>? ?? {})
        .map(
          (k, v) =>
              MapEntry(k, RoomPermissions.fromJson(v as Map<String, dynamic>)),
        ),
    bannedIds: (j['banned_ids'] as List<dynamic>? ?? [])
        .map((v) => v.toString())
        .toList(),
    playlist: (j['playlist'] as List<dynamic>? ?? [])
        .map((v) => PlaylistEntry.fromJson(v as Map<String, dynamic>))
        .toList(),
    activeItemId: j['active_item_id']?.toString() ?? '',
    autoNext: j['auto_next'] == true,
  );
}

class MediaVariant {
  const MediaVariant({required this.id, required this.label, this.height = 0});
  final String id, label;
  final int height;
  factory MediaVariant.fromJson(Map<String, dynamic> j) => MediaVariant(
    id: j['id'] as String,
    label: j['label'] as String,
    height: (j['height'] as num?)?.toInt() ?? 0,
  );
}

class PublicRoom {
  const PublicRoom({
    required this.code,
    required this.name,
    required this.description,
    required this.category,
    required this.tags,
    required this.allowGuests,
    required this.passwordProtected,
    required this.members,
    required this.maxMembers,
  });
  final String code, name, description, category;
  final List<String> tags;
  final bool allowGuests, passwordProtected;
  final int members, maxMembers;
  factory PublicRoom.fromJson(Map<String, dynamic> j) => PublicRoom(
    code: j['code'] as String,
    name: j['name'] as String,
    description: j['description']?.toString() ?? '',
    category: j['category']?.toString() ?? '',
    tags: (j['tags'] as List<dynamic>? ?? []).map((v) => v.toString()).toList(),
    allowGuests: j['allow_guests'] == true,
    passwordProtected: j['password_protected'] == true,
    members: (j['members'] as num).toInt(),
    maxMembers: (j['max_members'] as num).toInt(),
  );
}
