import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';

import 'src/app.dart';
import 'src/appearance.dart';
import 'src/device_identity.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  MediaKit.ensureInitialized();
  final device = await DeviceIdentity.load();
  final appearance = await FrameAppearance.load();
  runApp(SameFrameApp(device: device, appearance: appearance));
}
