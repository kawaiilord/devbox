import 'dart:js_interop';

@JS('sameframeAppearance.setMode')
external void syncBrowserAppearance(String mode);

@JS('window.localStorage.getItem')
external String? _readStorage(String key);

String readBrowserAppearance() {
  try {
    final value = _readStorage('sameframe.appearance.boot');
    return const {'light', 'dark', 'system'}.contains(value) ? value! : '';
  } catch (_) {
    return '';
  }
}
