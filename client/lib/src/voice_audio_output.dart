import 'package:flutter_webrtc/flutter_webrtc.dart';

import 'voice_playback_stub.dart'
    if (dart.library.js_interop) 'voice_playback_web.dart'
    as playback;

abstract class VoiceAudioOutput {
  Future<bool> attach(MediaStream stream);
  Future<bool> resume();
  Future<void> dispose();
}

class RendererVoiceAudioOutput implements VoiceAudioOutput {
  final _renderer = RTCVideoRenderer();
  bool _initialized = false;

  @override
  Future<bool> attach(MediaStream stream) async {
    if (!_initialized) {
      await _renderer.initialize();
      _initialized = true;
    }
    _renderer.srcObject = stream;
    _renderer.muted = false;
    return resume();
  }

  @override
  Future<bool> resume() => playback.resumeVoicePlayback(_renderer);

  @override
  Future<void> dispose() async {
    if (_initialized) {
      _renderer.srcObject = null;
      await _renderer.dispose();
    }
  }
}
