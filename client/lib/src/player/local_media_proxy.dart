import 'local_media_proxy_stub.dart'
    if (dart.library.io) 'local_media_proxy_io.dart'
    as implementation;

class LocalMediaProxy {
  LocalMediaProxy._(this._backend);

  final implementation.ProxyBackend _backend;

  static Future<LocalMediaProxy> create() async {
    return LocalMediaProxy._(await implementation.ProxyBackend.create());
  }

  Future<Uri> prepare(Uri upstream, {String? cacheIdentity}) =>
      _backend.prepare(upstream, cacheIdentity: cacheIdentity);

  Future<void> close() => _backend.close();
}
