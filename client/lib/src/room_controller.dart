import 'dart:async';

import 'package:flutter/foundation.dart';

import 'api/api_client.dart';
import 'models.dart';
import 'player/media_player_kernel.dart';
import 'sync/playback_synchronizer.dart';
import 'sync/room_socket.dart';

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
  Duration _clockOffset = Duration.zero;
  Future<void> _alignmentQueue = Future.value();
  Timer? _mediaRenewal;
  bool _renewingMedia = false;
  int _clientSequence = DateTime.now().microsecondsSinceEpoch;
  int _lastServerSequence = -1;
  bool connected = false;
  bool loading = true;
  String? error;
  AlignmentAction? lastAlignment;
  final Set<String> onlineUserIds = <String>{};

  bool get isOwner => room.ownerId == session.user.id;

  Future<void> initialize() async {
    try {
      _clockOffset = await api.measureClockOffset();
      await player.open(
        room.sourceUrl,
        version: room.playback.sourceVersion,
        episodeIndex: room.playback.episode,
        initialPosition: Duration(
          milliseconds: (room.playback.position * 1000).round(),
        ),
        cacheIdentity: _mediaCacheIdentity,
      );
      await _synchronizer.apply(
        snapshot: room.playback,
        player: player,
        sourceUrl: room.sourceUrl,
        clockOffset: _clockOffset,
        cacheIdentity: _mediaCacheIdentity,
      );
      final socket = RoomSocket(() => api.roomSocketUri(room, session));
      _socket = socket;
      _eventSubscription = socket.events.listen(_handleEnvelope);
      _connectionSubscription = socket.connection.listen((value) {
        connected = value;
        notifyListeners();
      });
      await socket.connect();
      _scheduleMediaRenewal();
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

  void _sendControl(Map<String, dynamic> payload) {
    if (!isOwner || !connected) return;
    _socket?.sendControl(++_clientSequence, payload);
  }

  void _handleEnvelope(RoomEnvelope envelope) {
    if (envelope.type == 'room.state') {
      final incoming = Room.fromJson(envelope.payload);
      room = incoming.mediaSourceId.isNotEmpty && room.sourceUrl.isNotEmpty
          ? incoming.withSourceUrl(room.sourceUrl)
          : incoming;
      _lastServerSequence = envelope.sequence;
      _queueAlignment(room.playback);
      _scheduleMediaRenewal();
      notifyListeners();
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
    if (envelope.type != 'playback.snapshot' ||
        envelope.sequence <= _lastServerSequence) {
      return;
    }
    _lastServerSequence = envelope.sequence;
    _queueAlignment(PlaybackSnapshot.fromJson(envelope.payload));
  }

  void _queueAlignment(PlaybackSnapshot snapshot) {
    _alignmentQueue = _alignmentQueue
        .then((_) async {
          final alignment = await _synchronizer.apply(
            snapshot: snapshot,
            player: player,
            sourceUrl: room.sourceUrl,
            clockOffset: _clockOffset,
            cacheIdentity: _mediaCacheIdentity,
          );
          lastAlignment = alignment.action;
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
    if (_renewingMedia || room.mediaSourceId.isEmpty) return;
    _renewingMedia = true;
    try {
      final ticket = await api.renewRoomMediaTicket(session, room.code);
      final position = player.position;
      final wasPlaying = player.playing;
      room = room.withSourceUrl(ticket.url);
      await player.open(
        ticket.url,
        version: player.sourceVersion,
        episodeIndex: player.episode,
        initialPosition: position,
        cacheIdentity: _mediaCacheIdentity,
      );
      if (wasPlaying) await player.play();
      notifyListeners();
    } catch (exception) {
      error = '媒体票据续期失败：$exception';
      notifyListeners();
    } finally {
      _renewingMedia = false;
    }
  }

  String? get _mediaCacheIdentity => room.mediaSourceId.isEmpty
      ? null
      : '${room.mediaSourceId}:${room.mediaPath}';

  @override
  void dispose() {
    _eventSubscription?.cancel();
    _connectionSubscription?.cancel();
    _mediaRenewal?.cancel();
    _socket?.close();
    player.dispose();
    super.dispose();
  }
}
