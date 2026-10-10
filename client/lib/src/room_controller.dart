import 'dart:async';

import 'package:flutter/foundation.dart';

import 'api/api_client.dart';
import 'models.dart';
import 'player/media_player_kernel.dart';
import 'player/playback_failure.dart';
import 'sync/playback_synchronizer.dart';
import 'sync/room_socket.dart';
import 'voice_session.dart';

class RoomController extends ChangeNotifier {
  RoomController({
    required this.api,
    required this.session,
    required this.room,
    MediaPlayerKernel? player,
    PlaybackSynchronizer? synchronizer,
  }) : player = player ?? MediaPlayerKernel(),
       _synchronizer = synchronizer ?? const PlaybackSynchronizer() {
    onlineUserIds.add(session.user.id);
  }

  final ApiClient api;
  final Session session;
  final MediaPlayerKernel player;
  final PlaybackSynchronizer _synchronizer;

  Room room;
  RoomSocket? _socket;
  StreamSubscription<RoomEnvelope>? _eventSubscription;
  StreamSubscription<bool>? _connectionSubscription;
  StreamSubscription<String>? _playerErrorSubscription;
  Duration _clockOffset = Duration.zero;
  Future<void> _alignmentQueue = Future.value();
  Timer? _mediaRenewal;
  Timer? _watchProgress;
  bool _renewingMedia = false;
  bool _syncingWatchProgress = false;
  int _danmakuEpisode = -1;
  int _clientSequence = DateTime.now().microsecondsSinceEpoch;
  int _lastServerSequence = -1;
  bool connected = false;
  bool loading = true;
  String? error;
  AlignmentAction? lastAlignment;
  PlaybackFailure? playbackFailure;
  final List<ChatMessage> messages = <ChatMessage>[];
  final List<DanmakuMessage> danmakuMessages = <DanmakuMessage>[];
  final Set<String> blockedUserIds = <String>{};
  final Set<String> blockedDanmakuKeywords = <String>{};
  bool danmakuEnabled = true;
  String? selectedSubtitlePath;
  String? selectedSubtitleName;
  final Set<String> onlineUserIds = <String>{};
  VoiceSession? voice;
  List<RTCIceServerConfig> _iceServers = const [];
  String? voiceError;
  AppAnnouncement? roomAnnouncement;

  bool _disposed = false;
  bool _autoAdvancing = false;
  int _sourceLoad = 0;
  StreamSubscription<bool>? _completionSubscription;
  bool get isOwner => room.ownerId == session.user.id;
  RoomPermissions get permissions =>
      room.features?.permissions ??
      RoomPermissions(playback: isOwner, playlist: isOwner);
  bool get canControl => permissions.playback;
  bool get isLive => room.features?.activeItem?.selectedSource?.isLive ?? false;
  bool get canManagePlaylist => permissions.playlist;
  @override
  void notifyListeners() {
    if (!_disposed) {
      super.notifyListeners();
    }
  }

  Future<void> refreshRoom() async =>
      acceptRoom(await api.getRoom(session, room.code));

  Future<void> acceptRoom(Room incoming, {bool forceReload = false}) async {
    if (_disposed ||
        incoming.playback.sourceVersion < room.playback.sourceVersion ||
        (incoming.features?.version ?? 0) < (room.features?.version ?? 0)) {
      return;
    }
    final old = room;
    final changed =
        incoming.playback.sourceVersion != old.playback.sourceVersion ||
        incoming.mediaSourceId != old.mediaSourceId ||
        incoming.mediaPath != old.mediaPath;
    final operation = ++_sourceLoad;
    if (incoming.sourceUrl.isNotEmpty) {
      incoming = incoming.withSourceUrl(
        Uri.parse(api.baseUrl).resolve(incoming.sourceUrl).toString(),
      );
    } else if (!changed && incoming.mediaSourceId.isNotEmpty) {
      incoming = incoming.withSourceUrl(old.sourceUrl);
    }
    room = incoming;
    error = null;
    if (!permissions.chat) {
      messages.clear();
    }
    if (!permissions.danmaku) {
      danmakuMessages.clear();
    }
    if (!permissions.voice && voice != null) {
      await stopVoice();
    }
    if (changed &&
        (old.features?.activeItemId != incoming.features?.activeItemId ||
            old.mediaSourceId != incoming.mediaSourceId ||
            old.mediaPath != incoming.mediaPath)) {
      selectedSubtitlePath = null;
      selectedSubtitleName = null;
    }
    notifyListeners();
    if (room.closed) {
      _mediaRenewal?.cancel();
      await player.pause();
      error = '房间已关闭。';
      notifyListeners();
      return;
    }
    try {
      if (changed && room.mediaSourceId.isNotEmpty && room.sourceUrl.isEmpty) {
        final hydrated = await api.getRoom(session, room.code);
        if (_disposed || operation != _sourceLoad) {
          return;
        }
        room = hydrated;
      }
      if (_disposed || operation != _sourceLoad) {
        return;
      }
      if (room.sourceUrl.isEmpty) {
        await player.pause();
        _mediaRenewal?.cancel();
        notifyListeners();
        return;
      }
      if (forceReload || (old.sourceUrl != room.sourceUrl && !changed)) {
        await player.open(
          room.sourceUrl,
          version: room.playback.sourceVersion,
          episodeIndex: room.playback.episode,
          initialPosition: Duration(
            milliseconds: (room.playback.position * 1000).round(),
          ),
          cacheIdentity: _mediaCacheIdentity,
          live: isLive,
        );
        if (_disposed || operation != _sourceLoad) {
          return;
        }
      }
      if (_danmakuEpisode != room.playback.episode) {
        unawaited(_loadDanmaku(room.playback.episode));
      }
      _queueAlignment(room.playback);
      _scheduleMediaRenewal();
    } catch (e) {
      if (!_disposed && operation == _sourceLoad) {
        error = '影片切换失败：$e';
        notifyListeners();
      }
    }
  }

  Future<void> _completed(bool completed) async {
    if (isLive ||
        !completed ||
        !isOwner ||
        _autoAdvancing ||
        room.features?.autoNext != true) {
      return;
    }
    final features = room.features!;
    final index = features.playlist.indexWhere(
      (item) => item.id == features.activeItemId,
    );
    if (index < 0 ||
        index + 1 >= features.playlist.length ||
        player.sourceVersion != room.playback.sourceVersion) {
      return;
    }
    _autoAdvancing = true;
    try {
      await acceptRoom(
        await api.selectPlaylistMedia(
          session,
          room,
          direction: 'next',
          auto: true,
        ),
      );
    } catch (e) {
      error = '自动切集失败：$e';
      notifyListeners();
    } finally {
      _autoAdvancing = false;
    }
  }

  void dismissAnnouncement() {
    roomAnnouncement = null;
    notifyListeners();
  }

  Future<void> initialize() async {
    try {
      _clockOffset = await api.measureClockOffset();
      blockedUserIds
        ..clear()
        ..addAll((await api.blockedUsers(session)).map((user) => user.id));
      messages
        ..clear()
        ..addAll(await api.roomMessages(session, room.code));
      danmakuMessages
        ..clear()
        ..addAll(
          await api.roomDanmaku(
            session,
            room.code,
            episode: room.playback.episode,
          ),
        );
      _danmakuEpisode = room.playback.episode;
      if (room.sourceUrl.isNotEmpty) {
        await player.open(
          room.sourceUrl,
          version: room.playback.sourceVersion,
          episodeIndex: room.playback.episode,
          initialPosition: Duration(
            milliseconds: (room.playback.position * 1000).round(),
          ),
          cacheIdentity: _mediaCacheIdentity,
          live: isLive,
        );
        if (isLive) {
          if (room.playback.playing) {
            await player.play();
          }
        } else {
          await _synchronizer.apply(
            snapshot: room.playback,
            player: player,
            sourceUrl: room.sourceUrl,
            clockOffset: _clockOffset,
            cacheIdentity: _mediaCacheIdentity,
          );
        }
      }
      if (_disposed) {
        return;
      }
      final socket = RoomSocket(() => api.roomSocketUri(room, session));
      _socket = socket;
      _eventSubscription = socket.events.listen(_handleEnvelope);
      _connectionSubscription = socket.connection.listen((value) {
        if (connected && !value) {
          unawaited(player.pause());
        }
        connected = value;
        notifyListeners();
      });
      _playerErrorSubscription = player.errorStream.listen(_handlePlayerError);
      _completionSubscription = player.player.stream.completed.listen(
        (value) => unawaited(_completed(value)),
      );
      await socket.connect();
      if (_disposed) {
        return;
      }
      _scheduleMediaRenewal();
      _watchProgress = Timer.periodic(
        const Duration(seconds: 30),
        (_) => _syncWatchProgress(),
      );
    } catch (exception) {
      error = exception.toString();
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  void play() => _sendControl({'action': 'play'});
  void pause() => _sendControl({'action': 'pause'});
  void seek(Duration position) => _sendControl({
    'action': 'seek',
    'position': position.inMilliseconds / 1000,
  });

  void setSpeed(double speed) =>
      _sendControl({'action': 'speed', 'speed': speed});

  void sendChat(String body) {
    final trimmed = body.trim();
    if (trimmed.isEmpty || !connected || !permissions.chat) return;
    _socket?.send('chat.message', ++_clientSequence, {'body': trimmed});
  }

  Future<void> blockUser(String userId) async {
    if (userId == session.user.id) return;
    await api.blockUser(session, userId);
    blockedUserIds.add(userId);
    messages.removeWhere((message) => message.userId == userId);
    danmakuMessages.removeWhere((message) => message.userId == userId);
    notifyListeners();
  }

  void sendDanmaku(
    String body, {
    int color = 0xffffff,
    String mode = 'scroll',
  }) {
    final trimmed = body.trim();
    if (trimmed.isEmpty ||
        !connected ||
        !danmakuEnabled ||
        !permissions.danmaku)
      return;
    _socket?.send('danmaku.message', ++_clientSequence, {
      'body': trimmed,
      'position_seconds': player.position.inMilliseconds / 1000,
      'color': color,
      'mode': mode,
    });
  }

  Future<void> startVoice() async {
    if (!permissions.voice) return;
    if (voice?.active == true) return;
    try {
      _iceServers = await api.rtcConfig(session, room.code);
      final next = VoiceSession(
        userId: session.user.id,
        sendSignal: (signal) =>
            _socket?.send('rtc.signal', ++_clientSequence, signal),
      );
      voice = next;
      await next.start(
        _iceServers,
        room.members.map((member) => member.userId).toList(growable: false),
      );
      voiceError = null;
    } catch (exception) {
      voiceError = exception.toString();
      await voice?.stop();
      voice = null;
    }
    notifyListeners();
  }

  void setVoiceMuted(bool value) {
    voice?.setMuted(value);
    notifyListeners();
  }

  Future<void> stopVoice() async {
    await voice?.stop();
    voice = null;
    notifyListeners();
  }

  void setDanmakuEnabled(bool value) {
    danmakuEnabled = value;
    notifyListeners();
  }

  void blockDanmakuKeyword(String value) {
    value = value.trim().toLowerCase();
    if (value.isEmpty) {
      return;
    }
    blockedDanmakuKeywords.add(value);
    notifyListeners();
  }

  bool danmakuVisible(DanmakuMessage message) {
    if (!danmakuEnabled || blockedUserIds.contains(message.userId)) {
      return false;
    }
    final body = message.body.toLowerCase();
    return !blockedDanmakuKeywords.any(body.contains);
  }

  Future<void> reportMessage(
    ChatMessage message, {
    required String reason,
    String details = '',
  }) {
    return api.createReport(
      session: session,
      targetType: 'message',
      targetId: message.id.toString(),
      reason: reason,
      details: details,
    );
  }

  Future<void> reportRoom({required String reason, String details = ''}) {
    return api.createReport(
      session: session,
      targetType: 'room',
      targetId: room.code,
      reason: reason,
      details: details,
    );
  }

  Future<List<MediaFile>> availableSubtitles() {
    return api.roomSubtitles(session, room.code);
  }

  Future<void> selectSubtitle(MediaFile file) async {
    final ticket = await api.roomSubtitleTicket(session, room.code, file.path);
    selectedSubtitlePath = file.path;
    selectedSubtitleName = file.name;
    await player.loadSubtitle(ticket.url, title: file.name);
    notifyListeners();
  }

  Future<void> disableSubtitles() async {
    selectedSubtitlePath = null;
    selectedSubtitleName = null;
    await player.disableSubtitles();
    notifyListeners();
  }

  void _sendControl(Map<String, dynamic> payload) {
    if (!canControl || !connected) return;
    _socket?.sendControl(++_clientSequence, {
      ...payload,
      'source_version': room.playback.sourceVersion,
    });
  }

  void _handleEnvelope(RoomEnvelope envelope) {
    if (envelope.type == 'announcement') {
      roomAnnouncement = AppAnnouncement.fromJson(envelope.payload);
      notifyListeners();
      return;
    }
    if (envelope.type == 'room.state') {
      _lastServerSequence = envelope.sequence;
      unawaited(acceptRoom(Room.fromJson(envelope.payload)));
      return;
    }
    if (envelope.type == 'error') {
      error = envelope.payload['message']?.toString() ?? 'Room error';
      notifyListeners();
      return;
    }
    if (envelope.type == 'room.presence') {
      final ids = envelope.payload['online_user_ids'] as List<dynamic>?;
      if (ids != null) {
        onlineUserIds
          ..clear()
          ..addAll(ids.map((value) => value.toString()));
        notifyListeners();
      }
      return;
    }
    if (envelope.type == 'chat.message') {
      final message = ChatMessage.fromJson(envelope.payload);
      if (blockedUserIds.contains(message.userId)) return;
      if (!messages.any((existing) => existing.id == message.id)) {
        messages.add(message);
        if (messages.length > 200) {
          messages.removeAt(0);
        }
        notifyListeners();
      }
      return;
    }
    if (envelope.type == 'danmaku.message') {
      final message = DanmakuMessage.fromJson(envelope.payload);
      if (!blockedUserIds.contains(message.userId) &&
          !danmakuMessages.any((existing) => existing.id == message.id)) {
        danmakuMessages.add(message);
        if (danmakuMessages.length > 5000) {
          danmakuMessages.removeAt(0);
        }
        notifyListeners();
      }
      return;
    }
    if (envelope.type == 'rtc.signal') {
      unawaited(_handleRTCSignal(envelope));
      return;
    }
    if (envelope.type != 'playback.snapshot' ||
        envelope.sequence <= _lastServerSequence) {
      return;
    }
    _lastServerSequence = envelope.sequence;
    final snapshot = PlaybackSnapshot.fromJson(envelope.payload);
    if (snapshot.sourceVersion != room.playback.sourceVersion) {
      if (snapshot.sourceVersion > room.playback.sourceVersion) {
        unawaited(
          refreshRoom().catchError((Object e) {
            error = '房间更新失败：$e';
            notifyListeners();
          }),
        );
      }
      return;
    }
    room = room.withPlayback(snapshot);
    if (_danmakuEpisode != snapshot.episode) {
      unawaited(_loadDanmaku(snapshot.episode));
    }
    _queueAlignment(snapshot);
  }

  Future<void> _loadDanmaku(int episode) async {
    try {
      final incoming = await api.roomDanmaku(
        session,
        room.code,
        episode: episode,
      );
      if (room.playback.episode != episode) return;
      danmakuMessages
        ..clear()
        ..addAll(incoming);
      _danmakuEpisode = episode;
      notifyListeners();
    } catch (_) {
      // Danmaku history failure does not interrupt playback.
    }
  }

  Future<void> _handleRTCSignal(RoomEnvelope envelope) async {
    if (voice == null) return;
    final signal = Map<String, dynamic>.from(envelope.payload);
    signal['from_user_id'] = envelope.fromUserId;
    await voice?.handle(signal, _iceServers);
    notifyListeners();
  }

  void _queueAlignment(PlaybackSnapshot snapshot) {
    final source = room.sourceUrl;
    _alignmentQueue = _alignmentQueue
        .then((_) async {
          if (_disposed ||
              source.isEmpty ||
              source != room.sourceUrl ||
              snapshot.sourceVersion != room.playback.sourceVersion) {
            return;
          }
          if (isLive) {
            if (player.sourceVersion != snapshot.sourceVersion ||
                player.episode != snapshot.episode) {
              await player.open(
                source,
                version: snapshot.sourceVersion,
                episodeIndex: snapshot.episode,
                live: true,
              );
            }
            if (snapshot.playing) {
              await player.play();
            } else {
              await player.pause();
            }
            notifyListeners();
            return;
          }
          final alignment = await _synchronizer.apply(
            snapshot: snapshot,
            player: player,
            sourceUrl: room.sourceUrl,
            clockOffset: _clockOffset,
            cacheIdentity: _mediaCacheIdentity,
          );
          lastAlignment = alignment.action;
          if (alignment.action == AlignmentAction.reloadSource ||
              alignment.action == AlignmentAction.switchEpisode) {
            await _renewSubtitleIfNeeded();
          }
          notifyListeners();
        })
        .catchError((Object exception) {
          error = exception.toString();
          notifyListeners();
        });
  }

  void _scheduleMediaRenewal() {
    _mediaRenewal?.cancel();
    if (room.mediaSourceId.isEmpty) return;
    _mediaRenewal = Timer.periodic(
      const Duration(minutes: 4),
      (_) => _renewMediaTicket(),
    );
  }

  Future<void> _renewMediaTicket() async {
    if (_renewingMedia || room.mediaSourceId.isEmpty || _disposed) {
      return;
    }
    _renewingMedia = true;
    try {
      final fresh = await api.getRoom(session, room.code);
      if (!_disposed) {
        await acceptRoom(fresh, forceReload: true);
      }
    } catch (e) {
      error = '播放地址更新失败：$e';
      notifyListeners();
    } finally {
      _renewingMedia = false;
    }
  }

  Future<void> _syncWatchProgress() async {
    if (loading || _syncingWatchProgress) return;
    if (player.position == Duration.zero && player.duration == Duration.zero) {
      return;
    }
    _syncingWatchProgress = true;
    final duration = player.duration.inMilliseconds / 1000;
    try {
      await api.updateWatchProgress(session, room.code, duration);
    } catch (_) {
      // Progress sync is best-effort and must never interrupt playback.
    } finally {
      _syncingWatchProgress = false;
    }
  }

  String? get _mediaCacheIdentity => room.mediaSourceId.isEmpty
      ? null
      : '${room.mediaSourceId}:${room.mediaPath}:${room.playback.sourceVersion}';

  Future<void> _renewSubtitleIfNeeded() async {
    final path = selectedSubtitlePath;
    if (path == null) return;
    final ticket = await api.roomSubtitleTicket(session, room.code, path);
    await player.loadSubtitle(ticket.url, title: selectedSubtitleName);
  }

  void _handlePlayerError(String raw) {
    final failure = PlaybackFailure.classify(raw);
    playbackFailure = failure;
    error = switch (failure.kind) {
      PlaybackFailureKind.ticketExpired => '播放票据已过期，正在续签…',
      PlaybackFailureKind.network => '网络连接异常，请检查连接后重试。',
      PlaybackFailureKind.decoding => '当前播放器无法解码此媒体。',
      PlaybackFailureKind.sourceUnavailable => '媒体文件已不存在或不可访问。',
      PlaybackFailureKind.unknown => '播放失败：${failure.message}',
    };
    notifyListeners();
    if (failure.kind == PlaybackFailureKind.ticketExpired &&
        room.mediaSourceId.isNotEmpty) {
      _renewMediaTicket();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _sourceLoad++;
    _completionSubscription?.cancel();
    _eventSubscription?.cancel();
    _connectionSubscription?.cancel();
    _playerErrorSubscription?.cancel();
    _mediaRenewal?.cancel();
    _watchProgress?.cancel();
    unawaited(_syncWatchProgress());
    _socket?.close();
    unawaited(voice?.stop());
    player.dispose();
    super.dispose();
  }
}
