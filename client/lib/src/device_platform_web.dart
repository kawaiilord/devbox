import 'dart:js_interop';

@JS('sameframeAccountDevice.id')
external String _browserDeviceID(String fallback);

String browserDeviceID(String fallback) {
  try {
    return _browserDeviceID(fallback);
  } catch (_) {
    return fallback;
  }
}
