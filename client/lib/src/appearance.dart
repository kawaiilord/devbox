import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'design.dart';

import 'appearance_platform_stub.dart'
    if (dart.library.js_interop) 'appearance_platform_web.dart';

class FrameAppearance extends ChangeNotifier {
  FrameAppearance() : _mode = ThemeMode.light, _preferences = null;
  FrameAppearance._(this._mode, this._preferences);

  static const preferenceKey = 'appearance.theme';
  final SharedPreferences? _preferences;
  ThemeMode _mode;
  ThemeMode get mode => _mode;

  static Future<FrameAppearance> load() async {
    final preferences = await SharedPreferences.getInstance();
    final browserMode = readBrowserAppearance();
    final saved = browserMode.isNotEmpty
        ? browserMode
        : preferences.getString(preferenceKey);
    final mode =
        const [
          ThemeMode.light,
          ThemeMode.dark,
        ].where((mode) => mode.name == saved).firstOrNull ??
        ThemeMode.light;
    syncBrowserAppearance(mode.name);
    return FrameAppearance._(mode, preferences);
  }

  Future<void> setMode(ThemeMode mode) async {
    if (_mode != mode) {
      _mode = mode;
      syncBrowserAppearance(mode.name);
      notifyListeners();
    }
    final preferences = _preferences;
    if (preferences != null &&
        !await preferences.setString(preferenceKey, mode.name)) {
      throw StateError('Could not save appearance preference');
    }
  }
}

class FrameAppearanceScope extends InheritedNotifier<FrameAppearance> {
  const FrameAppearanceScope({
    super.key,
    required FrameAppearance appearance,
    required super.child,
  }) : super(notifier: appearance);

  static FrameAppearance of(BuildContext context) => context
      .dependOnInheritedWidgetOfExactType<FrameAppearanceScope>()!
      .notifier!;
}

class FrameThemeSwitch extends StatelessWidget {
  const FrameThemeSwitch({super.key});

  Future<void> _select(BuildContext context, ThemeMode mode) async {
    try {
      await FrameAppearanceScope.of(context).setMode(mode);
    } catch (_) {
      if (context.mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('外观已切换，但未能保存偏好，请稍后重试。')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final appearance = FrameAppearanceScope.of(context);
    final colors = FrameColors.of(context);
    final dark = appearance.mode == ThemeMode.dark;
    return IconButton(
      tooltip: dark ? '切换为浅色' : '切换为深色',
      onPressed: () =>
          _select(context, dark ? ThemeMode.light : ThemeMode.dark),
      style: IconButton.styleFrom(
        foregroundColor: colors.muted,
        backgroundColor: Colors.transparent,
        minimumSize: const Size(42, 42),
      ),
      icon: AnimatedSwitcher(
        duration: const Duration(milliseconds: 180),
        child: Icon(
          dark ? Icons.dark_mode_outlined : Icons.light_mode_outlined,
          key: ValueKey(dark),
          size: 21,
        ),
      ),
    );
  }
}
