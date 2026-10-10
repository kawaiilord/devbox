import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:path_provider/path_provider.dart';

import 'update_service.dart';

Future<String> downloadVerifiedPackage(UpdateManifest manifest) async {
  final directory = await getApplicationSupportDirectory();
  final updateDirectory = Directory(
    '${directory.path}${Platform.pathSeparator}updates',
  );
  await updateDirectory.create(recursive: true);
  final partial = File(
    '${updateDirectory.path}${Platform.pathSeparator}sameframe-${manifest.version}.${manifest.platform}.partial',
  );
  final ready = File(
    '${updateDirectory.path}${Platform.pathSeparator}sameframe-${manifest.version}.${manifest.platform}.verified',
  );
  final client = HttpClient();
  final request = await client.getUrl(manifest.url);
  final response = await request.close().timeout(const Duration(seconds: 30));
  if (response.statusCode != HttpStatus.ok ||
      (response.contentLength >= 0 && response.contentLength != manifest.size))
    throw const UpdateException('更新包响应无效');
  final output = partial.openWrite();
  final digestSink = _DigestSink();
  final hashSink = sha256.startChunkedConversion(digestSink);
  var received = 0;
  try {
    await for (final chunk in response.timeout(const Duration(seconds: 30))) {
      received += chunk.length;
      if (received > manifest.size) throw const UpdateException('更新包超过清单大小');
      hashSink.add(chunk);
      output.add(chunk);
    }
    hashSink.close();
    await output.close();
    if (received != manifest.size ||
        digestSink.value.toString() != manifest.sha256)
      throw const UpdateException('更新包完整性验证失败');
    if (await ready.exists()) await ready.delete();
    await partial.rename(ready.path);
    return ready.path;
  } catch (_) {
    await output.close();
    if (await partial.exists()) await partial.delete();
    rethrow;
  } finally {
    client.close(force: true);
  }
}

class _DigestSink implements Sink<Digest> {
  Digest? _value;
  Digest get value => _value ?? (throw StateError('digest is unavailable'));
  @override
  void add(Digest data) => _value = data;
  @override
  void close() {}
}
