import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:path_provider/path_provider.dart';
import 'package:uuid/uuid.dart';

const _maxCacheBytes = 512 * 1024 * 1024;
const _maxEntryBytes = 64 * 1024 * 1024;

class ProxyBackend {
  ProxyBackend._(this._server, this._cacheDirectory) {
    _subscription = _server.listen(_handleRequest);
  }

  final HttpServer _server;
  final Directory _cacheDirectory;
  final Map<String, ({Uri uri, String cacheIdentity})> _upstreams =
      <String, ({Uri uri, String cacheIdentity})>{};
  late final StreamSubscription<HttpRequest> _subscription;

  static Future<ProxyBackend> create({Directory? cacheDirectory}) async {
    final support = cacheDirectory == null
        ? await getApplicationSupportDirectory()
        : null;
    final cache =
        cacheDirectory ??
        Directory('${support!.path}${Platform.pathSeparator}media_range_cache');
    await cache.create(recursive: true);
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final backend = ProxyBackend._(server, cache);
    await backend._enforceCacheLimit();
    return backend;
  }

  Future<Uri> prepare(Uri upstream, {String? cacheIdentity}) async {
    if (_upstreams.length >= 256) {
      _upstreams.remove(_upstreams.keys.first);
    }
    final routeToken = const Uuid().v4();
    _upstreams[routeToken] = (
      uri: upstream,
      cacheIdentity: cacheIdentity ?? upstream.toString(),
    );
    return Uri(
      scheme: 'http',
      host: InternetAddress.loopbackIPv4.address,
      port: _server.port,
      path: '/media/$routeToken',
    );
  }

  Future<void> _handleRequest(HttpRequest request) async {
    try {
      if (request.method != 'GET' && request.method != 'HEAD') {
        request.response.statusCode = HttpStatus.methodNotAllowed;
        await request.response.close();
        return;
      }
      final segments = request.uri.pathSegments;
      if (segments.length != 2 || segments.first != 'media') {
        request.response.statusCode = HttpStatus.notFound;
        await request.response.close();
        return;
      }
      final mapping = _upstreams[segments[1]];
      if (mapping == null) {
        request.response.statusCode = HttpStatus.notFound;
        await request.response.close();
        return;
      }
      final range = request.headers.value(HttpHeaders.rangeHeader) ?? '';
      final ifRange = request.headers.value(HttpHeaders.ifRangeHeader) ?? '';
      final cacheKey = sha256
          .convert(utf8.encode('${mapping.cacheIdentity}\n$range\n$ifRange'))
          .toString();
      final bodyFile = File(
        '${_cacheDirectory.path}${Platform.pathSeparator}$cacheKey.body',
      );
      final metaFile = File(
        '${_cacheDirectory.path}${Platform.pathSeparator}$cacheKey.json',
      );
      if (request.method == 'GET' &&
          await bodyFile.exists() &&
          await metaFile.exists()) {
        await _serveCached(request, bodyFile, metaFile);
        return;
      }
      await _fetch(request, mapping.uri, range, ifRange, bodyFile, metaFile);
    } catch (_) {
      try {
        request.response.statusCode = HttpStatus.badGateway;
      } catch (_) {}
      try {
        await request.response.close();
      } catch (_) {}
    }
  }

  Future<void> _serveCached(
    HttpRequest request,
    File bodyFile,
    File metaFile,
  ) async {
    final metadata =
        jsonDecode(await metaFile.readAsString()) as Map<String, dynamic>;
    request.response.statusCode = (metadata['status'] as num).toInt();
    final headers = metadata['headers'] as Map<String, dynamic>;
    for (final entry in headers.entries) {
      if (entry.value.toString().isNotEmpty) {
        request.response.headers.set(entry.key, entry.value.toString());
      }
    }
    request.response.headers.set('X-SameFrame-Cache', 'HIT');
    await request.response.addStream(bodyFile.openRead());
    await request.response.close();
    final now = DateTime.now();
    await bodyFile.setLastModified(now);
    await metaFile.setLastModified(now);
  }

  Future<void> _fetch(
    HttpRequest request,
    Uri upstream,
    String range,
    String ifRange,
    File bodyFile,
    File metaFile,
  ) async {
    final client = HttpClient()
      ..connectionTimeout = const Duration(seconds: 10);
    try {
      final upstreamRequest = await client.openUrl(request.method, upstream);
      upstreamRequest.followRedirects = false;
      if (range.isNotEmpty) {
        upstreamRequest.headers.set(HttpHeaders.rangeHeader, range);
      }
      if (ifRange.isNotEmpty) {
        upstreamRequest.headers.set(HttpHeaders.ifRangeHeader, ifRange);
      }
      final upstreamResponse = await upstreamRequest.close();
      request.response.statusCode = upstreamResponse.statusCode;
      final metadataHeaders = <String, String>{};
      for (final name in <String>[
        HttpHeaders.contentTypeHeader,
        HttpHeaders.contentLengthHeader,
        HttpHeaders.contentRangeHeader,
        HttpHeaders.acceptRangesHeader,
        HttpHeaders.etagHeader,
        HttpHeaders.lastModifiedHeader,
      ]) {
        final value = upstreamResponse.headers.value(name);
        if (value != null) {
          request.response.headers.set(name, value);
          metadataHeaders[name] = value;
        }
      }
      request.response.headers.set('X-SameFrame-Cache', 'MISS');
      if (request.method == 'HEAD') {
        await request.response.close();
        return;
      }
      final cacheable =
          (upstreamResponse.statusCode == HttpStatus.ok ||
              upstreamResponse.statusCode == HttpStatus.partialContent) &&
          upstreamResponse.contentLength >= 0 &&
          upstreamResponse.contentLength <= _maxEntryBytes;
      IOSink? sink;
      File? temporary;
      if (cacheable) {
        temporary = File('${bodyFile.path}.${const Uuid().v4()}.tmp');
        sink = temporary.openWrite();
      }
      await for (final chunk in upstreamResponse) {
        request.response.add(chunk);
        sink?.add(chunk);
      }
      await sink?.close();
      await request.response.close();
      if (temporary != null) {
        if (await bodyFile.exists()) await bodyFile.delete();
        await temporary.rename(bodyFile.path);
        await metaFile.writeAsString(
          jsonEncode({
            'status': upstreamResponse.statusCode,
            'headers': metadataHeaders,
          }),
          flush: true,
        );
        await _enforceCacheLimit();
      }
    } finally {
      client.close(force: true);
    }
  }

  Future<void> _enforceCacheLimit() async {
    final bodies = await _cacheDirectory
        .list()
        .where((entity) => entity is File && entity.path.endsWith('.body'))
        .cast<File>()
        .toList();
    final entries = <({File file, int size, DateTime modified})>[];
    var total = 0;
    for (final file in bodies) {
      final stat = await file.stat();
      total += stat.size;
      entries.add((file: file, size: stat.size, modified: stat.modified));
    }
    entries.sort((a, b) => a.modified.compareTo(b.modified));
    for (final entry in entries) {
      if (total <= _maxCacheBytes) break;
      total -= entry.size;
      await entry.file.delete();
      final metadata = File(
        entry.file.path.replaceFirst(RegExp(r'\.body$'), '.json'),
      );
      if (await metadata.exists()) await metadata.delete();
    }
  }

  Future<void> close() async {
    _upstreams.clear();
    await _subscription.cancel();
    await _server.close(force: true);
  }
}
