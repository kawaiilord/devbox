import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/player/local_media_proxy_io.dart';

void main() {
  test('desktop proxy forwards and caches an HTTP range', () async {
    var upstreamRequests = 0;
    final upstream = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final upstreamSubscription = upstream.listen((request) async {
      upstreamRequests++;
      expect(request.headers.value(HttpHeaders.rangeHeader), 'bytes=0-3');
      request.response.statusCode = HttpStatus.partialContent;
      request.response.headers.contentType = ContentType('video', 'mp4');
      request.response.headers.set(
        HttpHeaders.contentRangeHeader,
        'bytes 0-3/10',
      );
      request.response.headers.set(HttpHeaders.acceptRangesHeader, 'bytes');
      request.response.contentLength = 4;
      request.response.add(<int>[1, 2, 3, 4]);
      await request.response.close();
    });
    final cache = await Directory.systemTemp.createTemp(
      'sameframe-proxy-test-',
    );
    final proxy = await ProxyBackend.create(cacheDirectory: cache);
    final local = await proxy.prepare(
      Uri.parse(
        'http://${upstream.address.address}:${upstream.port}/movie.mp4?ticket=one',
      ),
      cacheIdentity: 'source-a:/movie.mp4',
    );

    Future<String?> fetch(Uri target) async {
      final client = HttpClient();
      try {
        final request = await client.getUrl(target);
        request.headers.set(HttpHeaders.rangeHeader, 'bytes=0-3');
        final response = await request.close();
        expect(response.statusCode, HttpStatus.partialContent);
        await response.drain<void>();
        return response.headers.value('X-SameFrame-Cache');
      } finally {
        client.close(force: true);
      }
    }

    expect(await fetch(local), 'MISS');
    final renewedLocal = await proxy.prepare(
      Uri.parse(
        'http://${upstream.address.address}:${upstream.port}/movie.mp4?ticket=two',
      ),
      cacheIdentity: 'source-a:/movie.mp4',
    );
    expect(await fetch(renewedLocal), 'HIT');
    expect(upstreamRequests, 1);

    await proxy.close();
    await upstreamSubscription.cancel();
    await upstream.close(force: true);
    await cache.delete(recursive: true);
  });
}
