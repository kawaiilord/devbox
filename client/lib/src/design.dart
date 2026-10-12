import 'dart:math' as math;

import 'package:flutter/material.dart';

@immutable
class FrameColors extends ThemeExtension<FrameColors> {
  const FrameColors({
    required this.background,
    required this.sidebar,
    required this.surface,
    required this.elevated,
    required this.input,
    required this.border,
    required this.muted,
    required this.text,
    required this.accent,
    required this.silver,
  });

  final Color background;
  final Color sidebar;
  final Color surface;
  final Color elevated;
  final Color input;
  final Color border;
  final Color muted;
  final Color text;
  final Color accent;
  final Color silver;

  static const light = FrameColors(
    background: Color(0xFFFAFAFA),
    sidebar: Color(0xFFF3F3F4),
    surface: Color(0xFFFFFFFF),
    elevated: Color(0xFFEEEEF0),
    input: Color(0xFFFAFAFB),
    border: Color(0xFFD9D9DE),
    muted: Color(0xFF67676E),
    text: Color(0xFF18181B),
    accent: Color(0xFF18181B),
    silver: Color(0xFF4F4F57),
  );

  static const dark = FrameColors(
    background: Color(0xFF0C0C0D),
    sidebar: Color(0xFF111112),
    surface: Color(0xFF171719),
    elevated: Color(0xFF232326),
    input: Color(0xFF121214),
    border: Color(0xFF303033),
    muted: Color(0xFF9A9A9F),
    text: Color(0xFFF4F4F5),
    accent: Color(0xFFF4F4F5),
    silver: Color(0xFFC5C5CA),
  );

  static FrameColors of(BuildContext context) =>
      Theme.of(context).extension<FrameColors>() ??
      (Theme.of(context).brightness == Brightness.dark ? dark : light);

  @override
  FrameColors copyWith({
    Color? background,
    Color? sidebar,
    Color? surface,
    Color? elevated,
    Color? input,
    Color? border,
    Color? muted,
    Color? text,
    Color? accent,
    Color? silver,
  }) => FrameColors(
    background: background ?? this.background,
    sidebar: sidebar ?? this.sidebar,
    surface: surface ?? this.surface,
    elevated: elevated ?? this.elevated,
    input: input ?? this.input,
    border: border ?? this.border,
    muted: muted ?? this.muted,
    text: text ?? this.text,
    accent: accent ?? this.accent,
    silver: silver ?? this.silver,
  );

  @override
  FrameColors lerp(covariant FrameColors? other, double t) {
    if (other == null) return this;
    return FrameColors(
      background: Color.lerp(background, other.background, t)!,
      sidebar: Color.lerp(sidebar, other.sidebar, t)!,
      surface: Color.lerp(surface, other.surface, t)!,
      elevated: Color.lerp(elevated, other.elevated, t)!,
      input: Color.lerp(input, other.input, t)!,
      border: Color.lerp(border, other.border, t)!,
      muted: Color.lerp(muted, other.muted, t)!,
      text: Color.lerp(text, other.text, t)!,
      accent: Color.lerp(accent, other.accent, t)!,
      silver: Color.lerp(silver, other.silver, t)!,
    );
  }
}

ThemeData frameTheme({Brightness brightness = Brightness.dark}) {
  final isDark = brightness == Brightness.dark;
  final colors = isDark ? FrameColors.dark : FrameColors.light;
  final scheme = (isDark ? const ColorScheme.dark() : const ColorScheme.light())
      .copyWith(
        primary: colors.accent,
        onPrimary: colors.background,
        primaryContainer: colors.elevated,
        onPrimaryContainer: colors.text,
        secondary: colors.silver,
        onSecondary: colors.background,
        secondaryContainer: colors.elevated,
        onSecondaryContainer: colors.text,
        tertiary: colors.silver,
        onTertiary: colors.background,
        tertiaryContainer: colors.elevated,
        onTertiaryContainer: colors.text,
        surface: colors.surface,
        onSurface: colors.text,
        onSurfaceVariant: colors.muted,
        surfaceContainerLowest: colors.background,
        surfaceContainerLow: colors.sidebar,
        surfaceContainer: colors.surface,
        surfaceContainerHigh: colors.elevated,
        surfaceContainerHighest: colors.border,
        surfaceTint: Colors.transparent,
        outline: colors.border,
        outlineVariant: colors.border,
        inverseSurface: colors.text,
        onInverseSurface: colors.background,
        inversePrimary: colors.background,
        error: isDark ? const Color(0xFFF0A5A5) : const Color(0xFFB42332),
        onError: isDark ? const Color(0xFF301818) : Colors.white,
        errorContainer: isDark
            ? const Color(0xFF301C1C)
            : const Color(0xFFFFEDEE),
        onErrorContainer: isDark
            ? const Color(0xFFF0A5A5)
            : const Color(0xFF871923),
      );
  final base = ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    extensions: [colors],
    fontFamilyFallback: const [
      'Noto Sans SC',
      'PingFang SC',
      'Microsoft YaHei',
    ],
  );
  final shape = RoundedRectangleBorder(borderRadius: BorderRadius.circular(12));
  return base.copyWith(
    scaffoldBackgroundColor: colors.background,
    canvasColor: colors.surface,
    hoverColor: colors.text.withValues(alpha: .05),
    focusColor: colors.text.withValues(alpha: .10),
    textTheme: base.textTheme.copyWith(
      headlineLarge: TextStyle(
        color: colors.text,
        fontSize: 34,
        fontWeight: FontWeight.w600,
        height: 1.3,
        letterSpacing: -1,
      ),
      headlineSmall: TextStyle(
        color: colors.text,
        fontSize: 26,
        fontWeight: FontWeight.w500,
        height: 1.4,
      ),
      titleLarge: TextStyle(
        color: colors.text,
        fontSize: 18,
        fontWeight: FontWeight.w600,
        height: 1.4,
      ),
      titleMedium: TextStyle(
        color: colors.text,
        fontSize: 16,
        fontWeight: FontWeight.w600,
        height: 1.4,
      ),
      bodyMedium: TextStyle(fontSize: 14, height: 1.6, color: colors.text),
      bodySmall: TextStyle(fontSize: 12, height: 1.5, color: colors.muted),
    ),
    appBarTheme: AppBarTheme(
      backgroundColor: colors.background,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      toolbarHeight: 68,
      shape: Border(bottom: BorderSide(color: colors.border)),
      titleTextStyle: TextStyle(
        fontSize: 17,
        fontWeight: FontWeight.w600,
        color: colors.text,
      ),
    ),
    cardTheme: CardThemeData(
      color: colors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: colors.border),
      ),
    ),
    dividerTheme: DividerThemeData(
      color: colors.border,
      thickness: 1,
      space: 32,
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(0, 48),
        padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 16),
        shape: shape,
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: colors.text,
        side: BorderSide(color: colors.border),
        minimumSize: const Size(0, 48),
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
        shape: shape,
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        shape: shape,
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: colors.input,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 17),
      labelStyle: TextStyle(color: colors.muted, fontSize: 14),
      hintStyle: TextStyle(color: colors.muted, fontSize: 14),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide(color: colors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide(color: colors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide(color: colors.accent, width: 1.2),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: colors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: BorderSide(color: colors.border),
      ),
    ),
    chipTheme: base.chipTheme.copyWith(
      backgroundColor: colors.surface,
      selectedColor: colors.elevated,
      labelStyle: TextStyle(color: colors.silver, fontSize: 12),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      side: BorderSide(color: colors.border),
    ),
    sliderTheme: base.sliderTheme.copyWith(
      trackHeight: 3,
      activeTrackColor: colors.accent,
      inactiveTrackColor: colors.border,
      thumbColor: colors.accent,
      thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 6),
      overlayShape: const RoundSliderOverlayShape(overlayRadius: 16),
    ),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) return colors.muted;
        return states.contains(WidgetState.selected)
            ? colors.background
            : colors.silver;
      }),
      trackColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) return colors.elevated;
        return states.contains(WidgetState.selected)
            ? colors.accent
            : colors.elevated;
      }),
      trackOutlineColor: WidgetStateProperty.resolveWith(
        (states) => states.contains(WidgetState.selected)
            ? Colors.transparent
            : colors.border,
      ),
    ),
    popupMenuTheme: PopupMenuThemeData(
      color: colors.elevated,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: colors.border),
      ),
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: colors.surface,
      surfaceTintColor: Colors.transparent,
      showDragHandle: true,
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: colors.elevated,
      contentTextStyle: TextStyle(color: colors.text),
      actionTextColor: colors.accent,
      behavior: SnackBarBehavior.floating,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: colors.border),
      ),
    ),
    tooltipTheme: TooltipThemeData(
      decoration: BoxDecoration(
        color: colors.elevated,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: colors.border),
      ),
      textStyle: TextStyle(color: colors.text, fontSize: 12),
    ),
  );
}

class FrameBrand extends StatelessWidget {
  const FrameBrand({super.key, this.compact = false});
  final bool compact;
  @override
  Widget build(BuildContext context) => Row(
    mainAxisSize: MainAxisSize.min,
    children: [
      const FrameMark(),
      const SizedBox(width: 11),
      Flexible(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              compact ? 'SameFrame' : 'SameFrame · 同帧',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 17,
                fontWeight: FontWeight.w600,
                letterSpacing: -.5,
              ),
            ),
            if (compact)
              Text(
                '同帧 / 共享此刻',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 10,
                  color: FrameColors.of(context).muted,
                ),
              ),
          ],
        ),
      ),
    ],
  );
}

/// Two frames sharing one view; also used by the web app's vector icon.
class FrameMark extends StatelessWidget {
  const FrameMark({super.key, this.size = 36});
  final double size;

  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox.square(
      dimension: size,
      child: CustomPaint(
        painter: _FrameMarkPainter(FrameColors.of(context).accent),
      ),
    ),
  );
}

class _FrameMarkPainter extends CustomPainter {
  _FrameMarkPainter(this.color);
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.scale(size.width / 36, size.height / 36);
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.8;
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(3, 4, 23, 23),
        const Radius.circular(7),
      ),
      paint,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(10, 11, 23, 23),
        const Radius.circular(7),
      ),
      paint,
    );
    canvas.drawPath(
      Path()
        ..moveTo(16, 16)
        ..lineTo(23, 20)
        ..lineTo(16, 24)
        ..close(),
      Paint()..color = color,
    );
  }

  @override
  bool shouldRepaint(_FrameMarkPainter oldDelegate) =>
      oldDelegate.color != color;
}

class FramePill extends StatelessWidget {
  const FramePill(this.text, {super.key, this.icon, this.color});
  final String text;
  final IconData? icon;
  final Color? color;
  @override
  Widget build(BuildContext context) {
    final color = this.color ?? FrameColors.of(context).accent;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .04),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: .14)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (icon != null) ...[
            Icon(icon, size: 13, color: color),
            const SizedBox(width: 6),
          ],
          Text(
            text,
            style: TextStyle(
              color: color,
              fontSize: 11,
              fontWeight: FontWeight.w600,
              letterSpacing: .4,
            ),
          ),
        ],
      ),
    );
  }
}

class FrameSection extends StatelessWidget {
  const FrameSection({
    super.key,
    required this.title,
    this.subtitle,
    required this.child,
    this.icon,
    this.trailing,
    this.showHeading = true,
  });
  final String title;
  final String? subtitle;
  final Widget child;
  final IconData? icon;
  final Widget? trailing;
  final bool showHeading;
  @override
  Widget build(BuildContext context) => Card(
    child: Padding(
      padding: EdgeInsets.all(MediaQuery.sizeOf(context).width < 700 ? 20 : 24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (showHeading)
            Row(
              children: [
                if (icon != null) ...[
                  Icon(icon, color: FrameColors.of(context).silver, size: 20),
                  const SizedBox(width: 10),
                ],
                Expanded(
                  child: Text(
                    title,
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                ),
                ?trailing,
              ],
            ),
          if (showHeading && subtitle != null) ...[
            const SizedBox(height: 8),
            Text(
              subtitle!,
              style: TextStyle(
                color: FrameColors.of(context).muted,
                fontSize: 13,
                height: 1.6,
              ),
            ),
          ],
          if (showHeading) const SizedBox(height: 24),
          child,
        ],
      ),
    ),
  );
}

class FrameEmpty extends StatelessWidget {
  const FrameEmpty({
    super.key,
    required this.icon,
    required this.title,
    required this.detail,
  });
  final IconData icon;
  final String title;
  final String detail;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 22),
    child: Center(
      child: Column(
        children: [
          Icon(
            icon,
            size: 30,
            color: FrameColors.of(context).muted.withValues(alpha: .6),
          ),
          const SizedBox(height: 12),
          Text(title, style: const TextStyle(fontWeight: FontWeight.w500)),
          const SizedBox(height: 4),
          Text(
            detail,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    ),
  );
}

/// A static wireframe aperture: no images, network requests or animation.
class CinemaArtwork extends StatelessWidget {
  const CinemaArtwork({super.key});
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: CustomPaint(painter: _AperturePainter(FrameColors.of(context))),
  );
}

class _AperturePainter extends CustomPainter {
  _AperturePainter(this.colors);
  final FrameColors colors;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.clipRect(Offset.zero & size);
    final grid = Paint()
      ..color = colors.text.withValues(alpha: .035)
      ..strokeWidth = .7;
    for (double x = 0; x < size.width; x += 40) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), grid);
    }
    for (double y = 0; y < size.height; y += 40) {
      canvas.drawLine(Offset(0, y), Offset(size.width, y), grid);
    }
    final center = Offset(size.width * .64, size.height * .5);
    canvas.translate(center.dx, center.dy);
    canvas.rotate(-.32);
    for (var i = 0; i < 22; i++) {
      final t = i / 21;
      final radius = size.height * (.14 + .54 * t);
      canvas.save();
      canvas.rotate(t * .65);
      canvas.drawOval(
        Rect.fromCenter(
          center: Offset((t - .5) * size.width * .10, 0),
          width: radius * 1.8,
          height: radius * (1 + .25 * math.sin(t * math.pi)),
        ),
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = .85
          ..color = colors.text.withValues(alpha: .10 + .30 * (1 - t)),
      );
      canvas.restore();
    }
    canvas.restore();
    final marker = Paint()
      ..color = colors.silver.withValues(alpha: .4)
      ..strokeWidth = 1;
    for (final p in [
      Offset(20, 20),
      Offset(size.width - 20, size.height - 20),
    ]) {
      canvas.drawLine(p - const Offset(4, 0), p + const Offset(4, 0), marker);
      canvas.drawLine(p - const Offset(0, 4), p + const Offset(0, 4), marker);
    }
  }

  @override
  bool shouldRepaint(_AperturePainter oldDelegate) =>
      oldDelegate.colors != colors;
}
