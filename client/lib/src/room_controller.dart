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
      );
      await _synchronizer.apply(
        snapshot: room.playback,
        player: player,
        sourceUrl: room.sourceUrl,
        clockOffset: _clockOffset,
      );
      final socket = RoomSocket(() => api.roomSocketUri(room, session));
      _socket = socket;
      _eventSubscription = socket.events.listen(_handleEnvelope);
      _connectionSubscription = socket.connection.listen((value) {
        connected = value;
        notifyListeners();
      });
      await socket.connect();
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
      room = Room.fromJson(envelope.payload);
      _lastServerSequence = envelope.sequence;
      _queueAlignment(room.playback);
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
          );
          lastAlignment = alignment.action;
          notifyListeners();
        })
        .catchError((Object exception) {
          error = exception.toString();
          notifyListeners();
        });
  }

  @override
  void dispose() {
    _eventSubscription?.cancel();
    _connectionSubscription?.cancel();
    _socket?.close();
    player.dispose();
    super.dispose();
  }
}
