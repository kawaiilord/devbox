import 'dart:js_interop';

@JS('sameframeAccountDevice.id')
external String _browserDeviceID(String fallback);

String browserDeviceID(String fallback) => _browserDeviceID(fallback);
