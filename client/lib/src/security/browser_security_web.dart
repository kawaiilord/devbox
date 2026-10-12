import 'dart:convert';
import 'dart:js_interop';

@JS('sameframeSecurity.supported')
external bool get _supported;
@JS('sameframeSecurity.createPasskey')
external JSPromise<JSString> _createPasskey(JSString options);
@JS('sameframeSecurity.beginAuthorization')
external void beginAuthorization();
@JS('sameframeSecurity.navigateAuthorization')
external void navigateAuthorization(String url);
@JS('sameframeSecurity.closeAuthorization')
external void closeAuthorization();
@JS('sameframeSecurity.cancelPasskey')
external void cancelPasskey();

bool get supportsBrowserSecurity => _supported;
Future<Map<String, dynamic>> createPasskey(Map<String, dynamic> options) async {
  final result = await _createPasskey(jsonEncode(options).toJS).toDart;
  return jsonDecode(result.toDart) as Map<String, dynamic>;
}
