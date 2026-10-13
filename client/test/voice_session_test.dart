import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:sameframe_client/src/voice_audio_output.dart';
import 'package:sameframe_client/src/voice_session.dart';

class TestTrack extends Fake implements MediaStreamTrack {
  bool stopped = false;
  @override
  bool enabled = true;
  @override
  String get kind => 'audio';
  @override
  Future<void> stop() async {
    stopped = true;
  }
}

class TestStream extends Fake implements MediaStream {
  final track = TestTrack();
  bool disposed = false;
  @override
  List<MediaStreamTrack> getAudioTracks() => [track];
  @override
  List<MediaStreamTrack> getTracks() => [track];
  @override
  Future<void> dispose() async {
    disposed = true;
  }
}

class TestSender extends Fake implements RTCRtpSender {}

class TestPeer extends Fake implements RTCPeerConnection {
  int offers = 0, answers = 0;
  bool remoteSet = false, closed = false;
  final candidates = <RTCIceCandidate>[];
  @override
  Function(RTCIceCandidate)? onIceCandidate;
  @override
  Function(RTCTrackEvent)? onTrack;
  @override
  Function(RTCPeerConnectionState)? onConnectionState;
  @override
  Future<RTCRtpSender> addTrack(
    MediaStreamTrack track, [
    MediaStream? stream,
  ]) async => TestSender();
  @override
  Future<RTCSessionDescription> createOffer([
    Map<String, dynamic>? constraints,
  ]) async {
    offers++;
    return RTCSessionDescription('offer-sdp', 'offer');
  }

  @override
  Future<RTCSessionDescription> createAnswer([
    Map<String, dynamic>? constraints,
  ]) async {
    answers++;
    return RTCSessionDescription('answer-sdp', 'answer');
  }

  @override
  Future<void> setLocalDescription(RTCSessionDescription description) async {}
  @override
  Future<void> setRemoteDescription(RTCSessionDescription description) async {
    remoteSet = true;
  }

  @override
  Future<void> addCandidate(RTCIceCandidate candidate) async {
    if (!remoteSet) throw StateError('ICE before remote description');
    candidates.add(candidate);
  }

  @override
  Future<void> close() async {
    closed = true;
  }

  @override
  Future<void> dispose() async {}
}

class TestOutput implements VoiceAudioOutput {
  MediaStream? stream;
  bool disposed = false, canPlay = true;
  Completer<bool>? pending;
  @override
  Future<bool> attach(MediaStream value) async {
    stream = value;
    if (pending != null) return pending!.future;
    return canPlay;
  }

  @override
  Future<bool> resume() async => canPlay;
  @override
  Future<void> dispose() async {
    disposed = true;
  }
}

void main() {
  for (final order in [
    ['a', 'b'],
    ['b', 'a'],
    ['both'],
  ]) {
    test(
      'voice handshake works for join order $order without duplicate offers',
      () async {
        final sessions = <String, VoiceSession>{};
        final peers = <String, List<TestPeer>>{'a': [], 'b': []};
        final messages = <Map<String, dynamic>>[];
        Future<void> drain() async {
          var count = 0;
          while (messages.isNotEmpty) {
            expect(++count, lessThan(30), reason: 'ready must not loop');
            final message = messages.removeAt(0);
            await sessions[message['target_user_id']]!.handle(message);
          }
        }

        for (final id in ['a', 'b']) {
          sessions[id] = VoiceSession(
            userId: id,
            sendSignal: (signal) =>
                messages.add({...signal, 'from_user_id': id}),
            openMicrophone: () async => TestStream(),
            createPeer: (_) async {
              final peer = TestPeer();
              peers[id]!.add(peer);
              return peer;
            },
            createOutput: TestOutput.new,
          );
        }
        if (order.first == 'both') {
          await Future.wait(
            sessions.values.map((s) => s.start([], ['a', 'b'])),
          );
        } else {
          await sessions[order.first]!.start([], ['a', 'b']);
          await drain();
          expect(
            peers.values.expand((p) => p),
            isEmpty,
            reason: 'never call someone before they join',
          );
          await sessions[order.last]!.start([], ['a', 'b']);
        }
        await drain();
        expect(peers['a'], hasLength(1));
        expect(peers['b'], hasLength(1));
        expect(peers['a']!.single.offers, 1);
        expect(peers['b']!.single.offers, 0);
        expect(peers['b']!.single.answers, 1);
        expect(peers.values.expand((p) => p).every((p) => p.remoteSet), isTrue);
        await sessions['a']!.handle({'type': 'ready', 'from_user_id': 'b'});
        expect(peers['a']!.single.offers, 1);
        for (final session in sessions.values) {
          await session.stop();
        }
      },
    );
  }

  test(
    'early ICE waits for SDP; remote audio playback does not block signaling',
    () async {
      final peer = TestPeer(), stream = TestStream(), output = TestOutput();
      final session = VoiceSession(
        userId: 'b',
        sendSignal: (_) {},
        openMicrophone: () async => stream,
        createPeer: (_) async => peer,
        createOutput: () => output,
      );
      await session.start([], []);
      final candidate = {
        'type': 'candidate',
        'from_user_id': 'a',
        'candidate': 'candidate:test',
        'sdp_mid': '0',
        'sdp_mline_index': 0,
      };
      await session.handle(candidate);
      expect(peer.candidates, isEmpty);
      await session.handle({
        'type': 'offer',
        'from_user_id': 'a',
        'sdp': 'offer',
      });
      expect(peer.candidates, hasLength(1));
      output.pending = Completer<bool>();
      final remote = TestStream();
      peer.onTrack!(RTCTrackEvent(streams: [remote], track: remote.track));
      await Future<void>.delayed(Duration.zero);
      expect(output.stream, same(remote));
      await session.handle(candidate);
      expect(peer.candidates, hasLength(2));
      output.pending!.complete(false);
      await Future<void>.delayed(Duration.zero);
      expect(session.playbackBlocked, isTrue);
      await session.resumeAudio();
      expect(session.playbackBlocked, isFalse);
      expect(session.peerCount, 0);
      peer.onConnectionState!(
        RTCPeerConnectionState.RTCPeerConnectionStateConnected,
      );
      expect(session.peerCount, 1);
      session.setMuted(true);
      expect(stream.track.enabled, isFalse);
      session.setMuted(false);
      expect(stream.track.enabled, isTrue);
      await session.handle({'type': 'hangup', 'from_user_id': 'a'});
      expect(peer.closed && output.disposed && remote.disposed, isTrue);
      expect(session.peerCount, 0);
      await session.stop();
      expect(stream.track.stopped && stream.disposed, isTrue);
    },
  );

  test('no microphone from incoming signals; cancel while permission prompt is open', () async {
    final microphone = Completer<MediaStream>();
    var opens = 0;
    final session = VoiceSession(
      userId: 'b',
      sendSignal: (_) {},
      openMicrophone: () {
        opens++;
        return microphone.future;
      },
    );
    await session.handle({
      'type': 'offer',
      'from_user_id': 'a',
      'sdp': 'offer',
    });
    expect(opens, 0);
    final starting = session.start([], []);
    await session.start([], []);
    expect(opens, 1);
    await session.stop();
    final stream = TestStream();
    microphone.complete(stream);
    await starting;
    expect(session.active, isFalse);
    expect(stream.track.stopped, isTrue);
  });

  test(
    'concurrent messages create a single peer and departed members release it',
    () async {
      final opening = Completer<RTCPeerConnection>();
      var creates = 0;
      final output = TestOutput(), peer = TestPeer();
      final session = VoiceSession(
        userId: 'b',
        sendSignal: (_) {},
        openMicrophone: () async => TestStream(),
        createPeer: (_) {
          creates++;
          return opening.future;
        },
        createOutput: () => output,
      );
      await session.start([], []);
      final one = session.handle({'type': 'ready', 'from_user_id': 'a'});
      final two = session.handle({'type': 'ready', 'from_user_id': 'a'});
      await Future<void>.delayed(Duration.zero);
      opening.complete(peer);
      await Future.wait([one, two]);
      expect(creates, 1);
      await session.retainMembers({'b'});
      expect(peer.closed && output.disposed, isTrue);
      await session.stop();
    },
  );

  test('permission failures explain the next action', () {
    expect(voiceFailureMessage(Exception('NotAllowedError')), contains('权限'));
    expect(voiceFailureMessage(Exception('NotFoundError')), contains('未检测到'));
    expect(
      voiceFailureMessage(Exception('voice unavailable')),
      contains('服务暂时不可用'),
    );
  });
}
