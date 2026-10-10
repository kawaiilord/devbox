import 'dart:js_interop';

@JS('sameframeAdaptive.prepare')
external JSPromise<JSAny?> _prepare(
  JSNumber id,
  JSString url,
  JSBoolean live,
  JSFunction onError,
);
@JS('sameframeAdaptive.ready')
external JSPromise<JSAny?> _ready(JSNumber id);
@JS('sameframeAdaptive.dispose')
external JSPromise<JSAny?> _dispose(JSNumber id);
Future<void> prepareAdaptive(
  int id,
  String url,
  bool live,
  void Function(String) onError,
) async {
  await _prepare(
    id.toJS,
    url.toJS,
    live.toJS,
    ((JSString message) => onError(message.toDart)).toJS,
  ).toDart;
}

Future<void> readyAdaptive(int id) async {
  await _ready(id.toJS).toDart;
}

Future<void> disposeAdaptive(int id) async {
  await _dispose(id.toJS).toDart;
}
