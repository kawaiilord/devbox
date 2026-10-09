import 'dart:async';

import 'package:flutter_webrtc/flutter_webrtc.dart';

import 'models.dart';

typedef SignalSender = void Function(Map<String, dynamic> signal);

class VoiceSession {
  VoiceSession({required this.userId, required this.sendSignal});
  final String userId;
  final SignalSender sendSignal;
  MediaStream? _local;
  final Map<String, RTCPeerConnection> _peers = {};
  bool muted = false;
  bool get active => _local != null;
  int get peerCount => _peers.length;
  Future<void> start(
    List<RTCIceServerConfig> servers,
    List<String> members,
  ) async {
    if (active) return;
    _local = await navigator.mediaDevices.getUserMedia({
      'audio': true,
      'video': false,
    });
    for (final peer in members) {
      if (peer == userId) continue;
      sendSignal({'type': 'ready', 'target_user_id': peer});
      if (userId.compareTo(peer) < 0) {
        await _sendOffer(peer, servers);
      }
    }
  }

  Future<void> _sendOffer(String peer, List<RTCIceServerConfig> servers) async {
    final pc = await _peer(peer, servers);
    final offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    sendSignal({
      'type': 'offer',
      'target_user_id': peer,
      'sdp': offer.sdp ?? '',
    });
  }

  Future<RTCPeerConnection> _peer(
    String peer,
    List<RTCIceServerConfig> servers,
  ) async {
    final existing = _peers[peer];
    if (existing != null) return existing;
    final pc = await createPeerConnection({
      'iceServers': [
        for (final server in servers)
          {
            'urls': server.urls,
            'username': server.username,
            'credential': server.credential,
          },
      ],
      'sdpSemantics': 'unified-plan',
    });
    for (final track in _local?.getAudioTracks() ?? <MediaStreamTrack>[]) {
      await pc.addTrack(track, _local!);
    }
    pc.onIceCandidate = (candidate) {
      if (candidate.candidate != null) {
        sendSignal({
          'type': 'candidate',
          'target_user_id': peer,
          'candidate': candidate.candidate,
          'sdp_mid': candidate.sdpMid,
          'sdp_mline_index': candidate.sdpMLineIndex,
        });
      }
    };
    pc.onConnectionState = (state) {
      if (state == RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
          state == RTCPeerConnectionState.RTCPeerConnectionStateClosed) {
        _peers.remove(peer);
      }
    };
    _peers[peer] = pc;
    return pc;
  }

  Future<void> handle(
    Map<String, dynamic> signal,
    List<RTCIceServerConfig> servers,
  ) async {
    final from = signal['from_user_id']?.toString() ?? '';
    if (from.isEmpty || from == userId) return;
    final type = signal['type']?.toString();
    if (type == 'hangup') {
      await _peers.remove(from)?.close();
      return;
    }
    if (type == 'ready') {
      if (userId.compareTo(from) < 0) {
        await _sendOffer(from, servers);
      }
      return;
    }
    final pc = await _peer(from, servers);
    if (type == 'offer') {
      await pc.setRemoteDescription(
        RTCSessionDescription(signal['sdp']?.toString(), 'offer'),
      );
      final answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      sendSignal({
        'type': 'answer',
        'target_user_id': from,
        'sdp': answer.sdp ?? '',
      });
    } else if (type == 'answer') {
      await pc.setRemoteDescription(
        RTCSessionDescription(signal['sdp']?.toString(), 'answer'),
      );
    } else if (type == 'candidate') {
      await pc.addCandidate(
        RTCIceCandidate(
          signal['candidate']?.toString(),
          signal['sdp_mid']?.toString(),
          signal['sdp_mline_index'] as int?,
        ),
      );
    }
  }

  void setMuted(bool value) {
    muted = value;
    for (final track in _local?.getAudioTracks() ?? <MediaStreamTrack>[]) {
      track.enabled = !value;
    }
  }

  Future<void> stop() async {
    for (final peer in _peers.keys.toList()) {
      sendSignal({'type': 'hangup', 'target_user_id': peer});
      await _peers.remove(peer)?.close();
    }
    for (final track in _local?.getTracks() ?? <MediaStreamTrack>[]) {
      await track.stop();
    }
    _local?.dispose();
    _local = null;
  }
}
