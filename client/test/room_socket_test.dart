import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/sync/room_socket.dart';

void main() {
  test(
    'late voice hangup during room disposal cannot write to a closed socket',
    () async {
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      WebSocket? incoming;
      final listener = server.listen((request) async {
        incoming = await WebSocketTransformer.upgrade(request);
        incoming!.listen((_) {});
      });
      final socket = RoomSocket(
        () async => Uri.parse('ws://127.0.0.1:${server.port}/'),
      );
      try {
        await socket.connect();
        final closing = socket.close();
        expect(
          () => socket.send('rtc.signal', 1, {'type': 'hangup'}),
          returnsNormally,
        );
        await closing;
        expect(
          () => socket.send('rtc.signal', 2, {'type': 'hangup'}),
          returnsNormally,
        );
      } finally {
        await socket.close();
        await incoming?.close();
        await listener.cancel();
        await server.close(force: true);
      }
    },
  );
}
