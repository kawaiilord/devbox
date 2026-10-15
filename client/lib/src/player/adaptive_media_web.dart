import 'dart:js_interop';
import 'dart:convert';

@JS('sameframeAdaptive.info')
external JSString _info(JSNumber id);
@JS('sameframeAdaptive.action')
external JSPromise<JSAny?> _action(JSNumber id, JSString name, JSString value);
Future<Map<String, dynamic>> adaptiveInfo(int id) async =>
    jsonDecode(_info(id.toJS).toDart) as Map<String, dynamic>;
Future<void> adaptiveAction(int id, String name, [String value = '']) async {
  await _action(id.toJS, name.toJS, value.toJS).toDart;
}

@JS('sameframeAdaptive.prepare')
external JSPromise<JSAny?> _prepare(
  JSNumber id,
  JSString url,
  JSBoolean live,
  JSFunction onError,
  JSNumber startPosition,
);
@JS('sameframeAdaptive.ready')
external JSPromise<JSAny?> _ready(JSNumber id);
@JS('sameframeAdaptive.dispose')
external JSPromise<JSAny?> _dispose(JSNumber id);
@JS('sameframeAdaptive.buffered')
external JSBoolean _buffered(JSNumber id);
Future<bool> adaptiveBuffered(int id) async => _buffered(id.toJS).toDart;
Future<void> prepareAdaptive(
  int id,
  String url,
  bool live,
  void Function(String) onError, {
  Duration initialPosition = Duration.zero,
}) async {
  await _prepare(
    id.toJS,
    url.toJS,
    live.toJS,
    ((JSString message) => onError(message.toDart)).toJS,
    (initialPosition.inMilliseconds / 1000).toJS,
  ).toDart;
}

Future<void> readyAdaptive(int id) async {
  await _ready(id.toJS).toDart;
}

Future<void> disposeAdaptive(int id) async {
  await _dispose(id.toJS).toDart;
}
