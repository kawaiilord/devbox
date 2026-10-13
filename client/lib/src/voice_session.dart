import 'dart:async';

import 'package:flutter_webrtc/flutter_webrtc.dart';

import 'models.dart';
import 'voice_audio_output.dart';

typedef SignalSender = void Function(Map<String, dynamic> signal);
typedef VoicePeerFactory = Future<RTCPeerConnection> Function(
  Map<String, dynamic> configuration,
);

class _VoicePeer {
  _VoicePeer(this.connection, this.output);
  final RTCPeerConnection connection;
  final VoiceAudioOutput output;
  final candidates = <RTCIceCandidate>[];
  bool ready = false, offered = false, remoteDescription = false;
  bool connected = false, playbackBlocked = false;
  MediaStream? remoteStream;
  Future<void> audioWork = Future.value();
}

class VoiceSession {
  VoiceSession({
    required this.userId,
    required this.sendSignal,
    this.onChanged,
    Future<MediaStream> Function()? openMicrophone,
    VoicePeerFactory? createPeer,
    VoiceAudioOutput Function()? createOutput,
  }) : _openMicrophone = openMicrophone ?? _microphone,
       _createPeer = createPeer ?? ((config) => createPeerConnection(config)),
       _createOutput = createOutput ?? RendererVoiceAudioOutput.new;

  final String userId;
  final SignalSender sendSignal;
  final void Function()? onChanged;
  final Future<MediaStream> Function() _openMicrophone;
  final VoicePeerFactory _createPeer;
  final VoiceAudioOutput Function() _createOutput;
  MediaStream? _local;
  final Map<String, _VoicePeer> _peers = {};
  List<RTCIceServerConfig> _servers = const [];
  Future<void> _signaling = Future.value();
  int _generation = 0;
  bool _starting = false;
  bool muted = false;
  String? error;
  bool get active => _local != null;
  int get peerCount => _peers.values.where((peer) => peer.connected).length;
  bool get playbackBlocked => _peers.values.any((peer) => peer.playbackBlocked);

  static Future<MediaStream> _microphone() =>
      navigator.mediaDevices.getUserMedia({
        'audio': {
          'echoCancellation': true,
          'noiseSuppression': true,
          'autoGainControl': true,
        },
        'video': false,
      });

  Future<void> start(
    List<RTCIceServerConfig> servers,
    List<String> members,
  ) async {
    if (active || _starting) return;
    _starting = true;
    final generation = ++_generation;
    try {
      final stream = await _openMicrophone();
      if (generation != _generation) {
        await _releaseMicrophone(stream);
        return;
      }
      _local = stream;
      _servers = servers;
      muted = false;
      error = null;
      // Wait for reciprocal ready; the lower ID offers exactly once, including
      // when users join simultaneously or after the initial announcement.
      for (final member in members.toSet()) {
        if (member != userId) _send(member, 'ready');
      }
      onChanged?.call();
    } finally {
      _starting = false;
    }
  }

  void _send(
    String peer,
    String type, [
    Map<String, dynamic> fields = const {},
  ]) {
    if (!active && type != 'hangup') return;
    sendSignal({'type': type, 'target_user_id': peer, ...fields});
  }

  // Serialize SDP, ICE and track callbacks so asynchronous peer creation and
  // setRemoteDescription cannot race another message on the same connection.
  Future<void> _enqueue(Future<void> Function() action) {
    final generation = _generation;
    final operation = _signaling.then((_) async {
      if (generation == _generation && active) await action();
    });
    _signaling = operation.catchError((Object exception) {
      if (generation == _generation && active) {
        error = voiceFailureMessage(exception);
        onChanged?.call();
      }
    });
    return _signaling;
  }

  Future<_VoicePeer?> _peer(String id) async {
    if (_peers.containsKey(id)) return _peers[id];
    final generation = _generation;
    final connection = await _createPeer({
      'iceServers': [
        for (final server in _servers)
          {
            'urls': server.urls,
            'username': server.username,
            'credential': server.credential,
          },
      ],
      'sdpSemantics': 'unified-plan',
    });
    if (!active || generation != _generation) {
      await connection.close();
      await connection.dispose();
      return null;
    }
    final peer = _VoicePeer(connection, _createOutput());
    _peers[id] = peer;
    connection.onIceCandidate = (candidate) {
      if (identical(_peers[id], peer) &&
          candidate.candidate?.isNotEmpty == true) {
        _send(id, 'candidate', {
          'candidate': candidate.candidate,
          'sdp_mid': candidate.sdpMid,
          'sdp_mline_index': candidate.sdpMLineIndex ?? 0,
        });
      }
    };
    connection.onTrack = (event) {
      if (event.track.kind != 'audio') return;
      peer.audioWork = peer.audioWork
          .then((_) async {
            if (!active || !identical(_peers[id], peer)) return;
            final stream = event.streams.isNotEmpty
                ? event.streams.first
                : await createLocalMediaStream('voice-remote-$id');
            if (event.streams.isEmpty) await stream.addTrack(event.track);
            peer.remoteStream = stream;
            peer.playbackBlocked = !await peer.output.attach(stream);
            onChanged?.call();
          })
          .catchError((Object exception) {
            if (active && identical(_peers[id], peer)) {
              error = voiceFailureMessage(exception);
              onChanged?.call();
            }
          });
    };
    connection.onConnectionState = (state) {
      if (!identical(_peers[id], peer)) return;
      peer.connected =
          state == RTCPeerConnectionState.RTCPeerConnectionStateConnected;
      if (state == RTCPeerConnectionState.RTCPeerConnectionStateFailed) {
        error = '与成员的语音连接中断，请退出语音后重新加入。';
        unawaited(_enqueue(() => _removePeer(id)));
      }
      onChanged?.call();
    };
    for (final track in _local!.getAudioTracks()) {
      await connection.addTrack(track, _local!);
    }
    return peer;
  }

  Future<void> handle(Map<String, dynamic> signal) => _enqueue(() async {
    final from = signal['from_user_id']?.toString() ?? '';
    if (from.isEmpty || from == userId) return;
    final type = signal['type'];
    if (type == 'hangup') {
      await _removePeer(from);
      return;
    }
    if (!['ready', 'offer', 'answer', 'candidate'].contains(type)) return;
    final peer = await _peer(from);
    if (peer == null || !active) return;
    final pc = peer.connection;
    if (type == 'ready') {
      if (!peer.ready) {
        peer.ready = true;
        _send(from, 'ready');
      }
      if (userId.compareTo(from) < 0 &&
          !peer.offered &&
          !peer.remoteDescription) {
        peer.offered = true;
        final offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        if (identical(_peers[from], peer))
          _send(from, 'offer', {'sdp': offer.sdp});
      }
    } else if (type == 'offer' || type == 'answer') {
      await pc.setRemoteDescription(
        RTCSessionDescription(signal['sdp']?.toString(), type as String),
      );
      peer.remoteDescription = true;
      for (final candidate in peer.candidates) {
        await pc.addCandidate(candidate);
      }
      peer.candidates.clear();
      if (type == 'offer') {
        final answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        if (identical(_peers[from], peer))
          _send(from, 'answer', {'sdp': answer.sdp});
      }
    } else if (type == 'candidate') {
      final candidate = RTCIceCandidate(
        signal['candidate']?.toString(),
        signal['sdp_mid']?.toString(),
        (signal['sdp_mline_index'] as num?)?.toInt(),
      );
      if (peer.remoteDescription) {
        await pc.addCandidate(candidate);
      } else if (peer.candidates.length < 256) {
        peer.candidates.add(candidate);
      }
    }
  });

  Future<void> retainMembers(Set<String> ids) => _enqueue(() async {
    for (final id in _peers.keys.toList()) {
      if (!ids.contains(id)) await _removePeer(id);
    }
  });

  Future<void> resumeAudio() async {
    // Call every play() within this user gesture before awaiting promises.
    await Future.wait(
      _peers.values.where((peer) => peer.remoteStream != null).map((
        peer,
      ) async {
        peer.playbackBlocked = !await peer.output.resume();
      }),
    );
    onChanged?.call();
  }

  void setMuted(bool value) {
    muted = value;
    for (final track in _local?.getAudioTracks() ?? <MediaStreamTrack>[]) {
      track.enabled = !value;
    }
    onChanged?.call();
  }

  Future<void> _removePeer(String id) async {
    final peer = _peers.remove(id);
    if (peer == null) return;
    try {
      await peer.connection.close();
      await peer.connection.dispose();
    } finally {
      await peer.audioWork;
      await peer.output.dispose();
      await peer.remoteStream?.dispose();
      onChanged?.call();
    }
  }

  static Future<void> _releaseMicrophone(MediaStream stream) async {
    for (final track in stream.getTracks()) {
      await track.stop();
    }
    await stream.dispose();
  }

  Future<void> stop() async {
    ++_generation;
    final local = _local;
    _local = null;
    // Release capture even while signaling is still awaiting an operation.
    if (local != null) await _releaseMicrophone(local);
    for (final peer in _peers.keys.toList()) {
      _send(peer, 'hangup');
    }
    await _signaling;
    for (final peer in _peers.keys.toList()) {
      await _removePeer(peer);
    }
    onChanged?.call();
  }
}

String voiceFailureMessage(Object exception) {
  final message = exception.toString().toLowerCase();
  if (message.contains('notallowed') ||
      message.contains('permission') ||
      message.contains('denied')) {
    return '麦克风权限未开启，请在浏览器设置中允许本站使用麦克风后重试。';
  }
  if (message.contains('notfound') || message.contains('devicesnotfound')) {
    return '未检测到可用麦克风，请连接麦克风后重试。';
  }
  if (message.contains('notreadable') || message.contains('trackstarterror')) {
    return '无法读取麦克风，请检查是否被其他应用占用。';
  }
  if (message.contains('voice unavailable') || message.contains('503')) {
    return '语音服务暂时不可用，请稍后重试。';
  }
  return '语音连接未完成，请检查麦克风权限和网络后重试。';
}
