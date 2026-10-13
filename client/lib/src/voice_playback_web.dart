import 'dart:js_interop';

import 'package:flutter_webrtc/flutter_webrtc.dart';

@JS('document.getElementById')
external JSObject? _elementById(String id);

extension type _AudioElement(JSObject _) implements JSObject {
  external JSPromise<JSAny?> play();
}

Future<bool> resumeVoicePlayback(RTCVideoRenderer renderer) async {
  final element = _elementById('audio_RTCVideoRenderer-${renderer.textureId}');
  if (element == null) return false;
  try {
    await _AudioElement(element)
        .play()
        .toDart
        .timeout(const Duration(seconds: 5));
    return true;
  } catch (_) {
    // Browsers may require another explicit tap to allow remote audio.
    return false;
  }
}
