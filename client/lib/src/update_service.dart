import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:package_info_plus/package_info_plus.dart';

import 'update_package_stub.dart'
    if (dart.library.io) 'update_package_io.dart'
    as package_download;

class UpdateManifest {
  const UpdateManifest({
    required this.version,
    required this.platform,
    required this.url,
    required this.sha256,
    required this.size,
    required this.publishedAt,
    required this.signature,
  });
  final String version;
  final String platform;
  final Uri url;
  final String sha256;
  final int size;
  final String publishedAt;
  final String signature;
  String get signedPayload =>
      '$version\n$platform\n$url\n$sha256\n$size\n$publishedAt';
  factory UpdateManifest.fromJson(Map<String, dynamic> json) => UpdateManifest(
    version: json['version'] as String,
    platform: json['platform'] as String,
    url: Uri.parse(json['url'] as String),
    sha256: json['sha256'] as String,
    size: (json['size'] as num).toInt(),
    publishedAt: json['published_at'] as String,
    signature: json['signature'] as String,
  );
}

class UpdateService {
  UpdateService({http.Client? client, String? manifestUrl, String? publicKey})
    : _client = client ?? http.Client(),
      manifestUrl =
          manifestUrl ?? const String.fromEnvironment('UPDATE_MANIFEST_URL'),
      publicKey =
          publicKey ?? const String.fromEnvironment('UPDATE_PUBLIC_KEY');
  final http.Client _client;
  final String manifestUrl;
  final String publicKey;

  bool get configured => manifestUrl.isNotEmpty && publicKey.isNotEmpty;

  Future<UpdateManifest?> check() async {
    if (!configured) {
      return null;
    }
    final manifestUri = Uri.parse(manifestUrl);
    if (manifestUri.scheme != 'https') {
      throw const UpdateException('更新清单必须使用 HTTPS');
    }
    final response = await _client
        .get(manifestUri)
        .timeout(const Duration(seconds: 15));
    if (response.statusCode != 200 || response.bodyBytes.length > 64 * 1024) {
      throw const UpdateException('无法读取更新清单');
    }
    final manifest = UpdateManifest.fromJson(
      jsonDecode(utf8.decode(response.bodyBytes)) as Map<String, dynamic>,
    );
    if (manifest.platform != currentUpdatePlatform ||
        manifest.url.scheme != 'https' ||
        manifest.size < 1 ||
        manifest.size > 2 * 1024 * 1024 * 1024 ||
        !RegExp(r'^[0-9a-f]{64}$').hasMatch(manifest.sha256)) {
      throw const UpdateException('更新清单字段无效');
    }
    final keyBytes = base64Decode(publicKey);
    final signatureBytes = base64Decode(manifest.signature);
    if (keyBytes.length != 32 || signatureBytes.length != 64) {
      throw const UpdateException('更新签名格式无效');
    }
    final valid = await Ed25519().verify(
      utf8.encode(manifest.signedPayload),
      signature: Signature(
        signatureBytes,
        publicKey: SimplePublicKey(keyBytes, type: KeyPairType.ed25519),
      ),
    );
    if (!valid) throw const UpdateException('更新签名验证失败');
    final current = (await PackageInfo.fromPlatform()).version;
    return _compareVersions(manifest.version, current) > 0 ? manifest : null;
  }

  Future<String> download(UpdateManifest manifest) =>
      package_download.downloadVerifiedPackage(manifest);
  void close() => _client.close();
}

String get currentUpdatePlatform {
  if (kIsWeb) return 'web';
  return switch (defaultTargetPlatform) {
    TargetPlatform.windows => 'windows',
    TargetPlatform.linux => 'linux',
    TargetPlatform.macOS => 'macos',
    TargetPlatform.android => 'android',
    TargetPlatform.iOS => 'ios',
    _ => 'unknown',
  };
}

int _compareVersions(String left, String right) {
  List<int> parse(String value) => value
      .split('-')
      .first
      .split('.')
      .map((part) => int.tryParse(part) ?? 0)
      .toList();
  final a = parse(left), b = parse(right);
  for (var i = 0; i < 3; i++) {
    final delta = (i < a.length ? a[i] : 0) - (i < b.length ? b[i] : 0);
    if (delta != 0) return delta;
  }
  return 0;
}

class UpdateException implements Exception {
  const UpdateException(this.message);
  final String message;
  @override
  String toString() => message;
}
