import 'dart:js_interop';

@JS('window.history.replaceState')
external void _replaceState(JSAny? data, String unused, String url);
@JS('window.location.assign')
external void _assign(String url);

void setBrowserEntry(String page) => _replaceState(null, '', '/$page');
void openAccountEntry(String page) => _assign('/$page');
void openOfficialWebsite() => _assign('/');
