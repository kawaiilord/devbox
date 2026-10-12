bool get supportsBrowserSecurity => false;
Future<Map<String, dynamic>> createPasskey(Map<String, dynamic> options) =>
    Future.error(UnsupportedError('请在网页版添加 Passkey'));
void beginAuthorization() => throw UnsupportedError('请在浏览器中关联账号');
void navigateAuthorization(String url) => throw UnsupportedError('请在浏览器中关联账号');
void closeAuthorization() {}
void cancelPasskey() {}
