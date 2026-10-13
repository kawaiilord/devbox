import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../models.dart';

class RoomSocket {
  RoomSocket(this._uriProvider);

  final Future<Uri> Function() _uriProvider;
  final StreamController<RoomEnvelope> _events =
      StreamController<RoomEnvelope>.broadcast();
  final StreamController<bool> _connection = StreamController<bool>.broadcast();

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _subscription;
  Timer? _retryTimer;
  bool _closed = false;
  int _attempt = 0;

  Stream<RoomEnvelope> get events => _events.stream;
  Stream<bool> get connection => _connection.stream;

  Future<void> connect() async {
    if (_closed) return;
    try {
      final uri = await _uriProvider();
      final channel = WebSocketChannel.connect(uri);
      await channel.ready;
      if (_closed) {
        await channel.sink.close();
        return;
      }
      _channel = channel;
      _attempt = 0;
      _connection.add(true);
      _subscription = channel.stream.listen(
        _onMessage,
        onError: (_) => _scheduleReconnect(),
        onDone: _scheduleReconnect,
        cancelOnError: true,
      );
    } catch (_) {
      _scheduleReconnect();
    }
  }

  void sendControl(int sequence, Map<String, dynamic> payload) {
    send('playback.control', sequence, payload);
  }

  void send(String type, int sequence, Map<String, dynamic> payload) {
    final channel = _channel;
    if (_closed || channel == null) return;
    try {
      channel.sink.add(
        jsonEncode({
          'type': type,
          'seq': sequence,
          'ts': DateTime.now().millisecondsSinceEpoch,
          'payload': payload,
        }),
      );
    } on StateError {
      // A transport can close before its onDone callback is delivered.
      _scheduleReconnect();
    }
  }

  void _onMessage(dynamic message) {
    final decoded = jsonDecode(message as String) as Map<String, dynamic>;
    _events.add(RoomEnvelope.fromJson(decoded));
  }

  void _scheduleReconnect() {
    if (_closed || _retryTimer?.isActive == true) return;
    _connection.add(false);
    _subscription?.cancel();
    _subscription = null;
    _channel = null;
    final exponent = _attempt > 5 ? 5 : _attempt;
    final seconds = 1 << exponent;
    _attempt++;
    _retryTimer = Timer(Duration(seconds: seconds), connect);
  }

  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    _retryTimer?.cancel();
    final subscription = _subscription;
    final channel = _channel;
    _subscription = null;
    _channel = null;
    await subscription?.cancel();
    await channel?.sink.close();
    await _events.close();
    await _connection.close();
  }
}
