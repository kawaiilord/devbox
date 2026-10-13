import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:uuid/uuid.dart';

import 'device_platform_stub.dart'
    if (dart.library.js_interop) 'device_platform_web.dart';

class DeviceIdentity {
  const DeviceIdentity({
    required this.id,
    required this.label,
    required this.platform,
  });

  final String id;
  final String label;
  final String platform;

  static Future<DeviceIdentity> load() async {
    final preferences = await SharedPreferences.getInstance();
    var id = preferences.getString('device.id');
    if (kIsWeb) {
      final shared = browserDeviceID(id ?? '');
      if (id != shared) await preferences.setString('device.id', shared);
      id = shared;
    }
    if (id == null || id.length < 16) {
      id = const Uuid().v4();
      await preferences.setString('device.id', id);
    }
    final platform = kIsWeb ? 'web' : defaultTargetPlatform.name.toLowerCase();
    final label = kIsWeb
        ? 'Web browser'
        : '${defaultTargetPlatform.name} device';
    return DeviceIdentity(id: id, label: label, platform: platform);
  }

  static const test = DeviceIdentity(
    id: 'test-device-0001',
    label: 'Test device',
    platform: 'test',
  );
}
