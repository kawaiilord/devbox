import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:sameframe_client/src/appearance.dart';
import 'package:sameframe_client/src/design.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test(
    'appearance choice survives a fresh controller and tolerates old values',
    () async {
      SharedPreferences.setMockInitialValues({});
      final appearance = await FrameAppearance.load();
      addTearDown(appearance.dispose);
      expect(appearance.mode, ThemeMode.light);

      for (final mode in [ThemeMode.dark, ThemeMode.system, ThemeMode.light]) {
        await appearance.setMode(mode);
        final restored = await FrameAppearance.load();
        expect(restored.mode, mode);
        restored.dispose();
      }

      final preferences = await SharedPreferences.getInstance();
      await preferences.setString('appearance.theme', 'unknown-future-mode');
      final fallback = await FrameAppearance.load();
      expect(fallback.mode, ThemeMode.light);
      fallback.dispose();
    },
  );

  test('normal and secondary text retain readable contrast in both themes', () {
    double contrast(Color a, Color b) {
      final x = a.computeLuminance();
      final y = b.computeLuminance();
      return x > y ? (x + .05) / (y + .05) : (y + .05) / (x + .05);
    }

    for (final brightness in Brightness.values) {
      final theme = frameTheme(brightness: brightness);
      final colors = theme.extension<FrameColors>()!;
      for (final surface in [
        colors.background,
        colors.surface,
        colors.sidebar,
        colors.elevated,
        colors.input,
      ]) {
        expect(contrast(colors.text, surface), greaterThanOrEqualTo(4.5));
        expect(contrast(colors.muted, surface), greaterThanOrEqualTo(4.5));
      }
      expect(
        contrast(theme.colorScheme.primary, theme.colorScheme.onPrimary),
        greaterThanOrEqualTo(4.5),
      );
    }
  });
}
