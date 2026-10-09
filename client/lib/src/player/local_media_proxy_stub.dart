class ProxyBackend {
  const ProxyBackend();

  static Future<ProxyBackend> create() async => const ProxyBackend();

  Future<Uri> prepare(Uri upstream, {String? cacheIdentity}) async => upstream;

  Future<void> close() async {}
}
