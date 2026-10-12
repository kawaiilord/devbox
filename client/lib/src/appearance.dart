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
    return Container(
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        color: colors.elevated,
        borderRadius: BorderRadius.circular(11),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        spacing: 2,
        children: [
          for (final (mode, label, icon) in const [
            (ThemeMode.light, '浅色模式', Icons.light_mode_outlined),
            (ThemeMode.dark, '深色模式', Icons.dark_mode_outlined),
            (ThemeMode.system, '跟随系统', Icons.desktop_windows_outlined),
          ])
            Semantics(
              label: label,
              button: true,
              toggled: appearance.mode == mode,
              inMutuallyExclusiveGroup: true,
              onTap: () => _select(context, mode),
              child: Tooltip(
                message: label,
                excludeFromSemantics: true,
                child: Material(
                  color: appearance.mode == mode
                      ? colors.surface
                      : Colors.transparent,
                  elevation: appearance.mode == mode ? 1 : 0,
                  shadowColor: Colors.black.withValues(alpha: .1),
                  borderRadius: BorderRadius.circular(8),
                  child: InkWell(
                    excludeFromSemantics: true,
                    borderRadius: BorderRadius.circular(8),
                    onTap: () => _select(context, mode),
                    child: SizedBox(
                      width: 38,
                      height: 36,
                      child: Icon(
                        icon,
                        size: 19,
                        color: appearance.mode == mode
                            ? colors.text
                            : colors.muted,
                      ),
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
