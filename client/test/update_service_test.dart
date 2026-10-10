import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:sameframe_client/src/update_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  PackageInfo.setMockInitialValues(
    appName: 'SameFrame',
    packageName: 'sameframe_client',
    version: '1.0.0',
    buildNumber: '1',
    buildSignature: 'test',
  );

  test('accepts a newer Ed25519 signed update manifest', () async {
    final algorithm = Ed25519();
    final keyPair = await algorithm.newKeyPair();
    final publicKey = await keyPair.extractPublicKey();
    const version = '1.2.0';
    final platform = currentUpdatePlatform;
    const url = 'https://updates.example.test/sameframe.pkg';
    const hash =
        'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
    const size = 1234;
    const published = '2026-10-10T00:00:00Z';
    final payload = '$version\n$platform\n$url\n$hash\n$size\n$published';
    final signature = await algorithm.sign(
      utf8.encode(payload),
      keyPair: keyPair,
    );
    final client = MockClient(
      (_) async => http.Response.bytes(
        utf8.encode(
          jsonEncode({
            'version': version,
            'platform': platform,
            'url': url,
            'sha256': hash,
            'size': size,
            'published_at': published,
            'signature': base64Encode(signature.bytes),
          }),
        ),
        200,
      ),
    );
    final service = UpdateService(
      client: client,
      manifestUrl: 'https://updates.example.test/manifest.json',
      publicKey: base64Encode(publicKey.bytes),
    );
    final manifest = await service.check();
    expect(manifest?.version, version);
    service.close();
  });

  test('rejects a tampered update manifest', () async {
    final algorithm = Ed25519();
    final keyPair = await algorithm.newKeyPair();
    final publicKey = await keyPair.extractPublicKey();
    final signature = await algorithm.sign(
      utf8.encode('different payload'),
      keyPair: keyPair,
    );
    final client = MockClient(
      (_) async => http.Response.bytes(
        utf8.encode(
          jsonEncode({
            'version': '2.0.0',
            'platform': currentUpdatePlatform,
            'url': 'https://updates.example.test/package',
            'sha256': 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
            'size': 10,
            'published_at': '2026-10-10T00:00:00Z',
            'signature': base64Encode(signature.bytes),
          }),
        ),
        200,
      ),
    );
    final service = UpdateService(
      client: client,
      manifestUrl: 'https://updates.example.test/manifest.json',
      publicKey: base64Encode(publicKey.bytes),
    );
    expect(service.check(), throwsA(isA<UpdateException>()));
    service.close();
  });
}
