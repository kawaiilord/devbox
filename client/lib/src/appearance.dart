import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

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
    final saved = preferences.getString(preferenceKey);
    final mode =
        ThemeMode.values.where((mode) => mode.name == saved).firstOrNull ??
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

class FrameThemeMenu extends StatelessWidget {
  const FrameThemeMenu({super.key});

  @override
  Widget build(BuildContext context) {
    final appearance = FrameAppearanceScope.of(context);
    return PopupMenuButton<ThemeMode>(
      tooltip: '切换外观',
      initialValue: appearance.mode,
      icon: Icon(
        Theme.of(context).brightness == Brightness.light
            ? Icons.light_mode_outlined
            : Icons.dark_mode_outlined,
        size: 20,
      ),
      onSelected: (mode) async {
        try {
          await appearance.setMode(mode);
        } catch (_) {
          if (context.mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(content: Text('外观已切换，但未能保存偏好，请稍后重试。')),
            );
          }
        }
      },
      itemBuilder: (_) => [
        for (final entry in const {
          ThemeMode.light: '浅色模式',
          ThemeMode.dark: '深色模式',
          ThemeMode.system: '跟随系统',
        }.entries)
          CheckedPopupMenuItem(
            value: entry.key,
            checked: appearance.mode == entry.key,
            child: Text(entry.value),
          ),
      ],
    );
  }
}
