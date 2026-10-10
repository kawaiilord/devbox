import 'dart:math' as math;

import 'package:flutter/material.dart';

abstract final class FrameColors {
  static const background = Color(0xFF0C0C0D);
  static const sidebar = Color(0xFF111112);
  static const surface = Color(0xFF171719);
  static const elevated = Color(0xFF232326);
  static const input = Color(0xFF121214);
  static const border = Color(0xFF303033);
  static const muted = Color(0xFF9A9A9F);
  static const text = Color(0xFFF4F4F5);
  static const accent = Color(0xFFF4F4F5);
  static const silver = Color(0xFFC5C5CA);
}

ThemeData frameTheme() {
  final scheme = const ColorScheme.dark().copyWith(
    primary: FrameColors.accent,
    onPrimary: FrameColors.background,
    primaryContainer: FrameColors.elevated,
    onPrimaryContainer: FrameColors.text,
    secondary: FrameColors.silver,
    onSecondary: FrameColors.background,
    secondaryContainer: FrameColors.elevated,
    onSecondaryContainer: FrameColors.text,
    tertiary: FrameColors.silver,
    onTertiary: FrameColors.background,
    tertiaryContainer: FrameColors.elevated,
    onTertiaryContainer: FrameColors.text,
    surface: FrameColors.surface,
    onSurface: FrameColors.text,
    onSurfaceVariant: FrameColors.muted,
    surfaceContainerLowest: FrameColors.background,
    surfaceContainerLow: FrameColors.sidebar,
    surfaceContainer: FrameColors.surface,
    surfaceContainerHigh: FrameColors.elevated,
    surfaceContainerHighest: FrameColors.border,
    surfaceTint: Colors.transparent,
    outline: FrameColors.border,
    outlineVariant: FrameColors.border,
    inverseSurface: FrameColors.text,
    onInverseSurface: FrameColors.background,
    inversePrimary: FrameColors.background,
    error: const Color(0xFFF0A5A5),
    onError: const Color(0xFF301818),
    errorContainer: const Color(0xFF301C1C),
    onErrorContainer: const Color(0xFFF0A5A5),
  );
  final base = ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    fontFamilyFallback: const [
      'Noto Sans SC',
      'PingFang SC',
      'Microsoft YaHei',
    ],
  );
  final shape = RoundedRectangleBorder(borderRadius: BorderRadius.circular(12));
  return base.copyWith(
    scaffoldBackgroundColor: FrameColors.background,
    canvasColor: FrameColors.surface,
    hoverColor: Colors.white.withValues(alpha: .05),
    focusColor: Colors.white.withValues(alpha: .10),
    textTheme: base.textTheme.copyWith(
      headlineLarge: const TextStyle(
        color: FrameColors.text,
        fontSize: 34,
        fontWeight: FontWeight.w600,
        height: 1.3,
        letterSpacing: -1,
      ),
      headlineSmall: const TextStyle(
        color: FrameColors.text,
        fontSize: 26,
        fontWeight: FontWeight.w500,
        height: 1.4,
      ),
      titleLarge: const TextStyle(
        color: FrameColors.text,
        fontSize: 18,
        fontWeight: FontWeight.w600,
        height: 1.4,
      ),
      titleMedium: const TextStyle(
        color: FrameColors.text,
        fontSize: 16,
        fontWeight: FontWeight.w600,
        height: 1.4,
      ),
      bodyMedium: const TextStyle(
        fontSize: 14,
        height: 1.6,
        color: FrameColors.text,
      ),
      bodySmall: const TextStyle(
        fontSize: 12,
        height: 1.5,
        color: FrameColors.muted,
      ),
    ),
    appBarTheme: const AppBarTheme(
      backgroundColor: FrameColors.background,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      toolbarHeight: 68,
      shape: Border(bottom: BorderSide(color: FrameColors.border)),
      titleTextStyle: TextStyle(
        fontSize: 17,
        fontWeight: FontWeight.w600,
        color: FrameColors.text,
      ),
    ),
    cardTheme: CardThemeData(
      color: FrameColors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: const BorderSide(color: FrameColors.border),
      ),
    ),
    dividerTheme: const DividerThemeData(
      color: FrameColors.border,
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
        foregroundColor: FrameColors.text,
        side: const BorderSide(color: FrameColors.border),
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
      fillColor: FrameColors.input,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 17),
      labelStyle: const TextStyle(color: FrameColors.muted, fontSize: 14),
      hintStyle: const TextStyle(color: FrameColors.muted, fontSize: 14),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: FrameColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: FrameColors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: FrameColors.accent, width: 1.2),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: FrameColors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: const BorderSide(color: FrameColors.border),
      ),
    ),
    chipTheme: base.chipTheme.copyWith(
      backgroundColor: FrameColors.surface,
      selectedColor: FrameColors.elevated,
      labelStyle: const TextStyle(color: FrameColors.silver, fontSize: 12),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      side: const BorderSide(color: FrameColors.border),
    ),
    sliderTheme: base.sliderTheme.copyWith(
      trackHeight: 3,
      activeTrackColor: FrameColors.accent,
      inactiveTrackColor: FrameColors.border,
      thumbColor: FrameColors.accent,
      thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 6),
      overlayShape: const RoundSliderOverlayShape(overlayRadius: 16),
    ),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) return FrameColors.muted;
        return states.contains(WidgetState.selected)
            ? FrameColors.background
            : FrameColors.silver;
      }),
      trackColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) return FrameColors.elevated;
        return states.contains(WidgetState.selected)
            ? FrameColors.accent
            : FrameColors.elevated;
      }),
      trackOutlineColor: WidgetStateProperty.resolveWith(
        (states) => states.contains(WidgetState.selected)
            ? Colors.transparent
            : FrameColors.border,
      ),
    ),
    popupMenuTheme: PopupMenuThemeData(
      color: FrameColors.elevated,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: FrameColors.border),
      ),
    ),
    bottomSheetTheme: const BottomSheetThemeData(
      backgroundColor: FrameColors.surface,
      surfaceTintColor: Colors.transparent,
      showDragHandle: true,
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: FrameColors.elevated,
      contentTextStyle: const TextStyle(color: FrameColors.text),
      actionTextColor: FrameColors.accent,
      behavior: SnackBarBehavior.floating,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: FrameColors.border),
      ),
    ),
    tooltipTheme: TooltipThemeData(
      decoration: BoxDecoration(
        color: FrameColors.elevated,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: FrameColors.border),
      ),
      textStyle: const TextStyle(color: FrameColors.text, fontSize: 12),
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
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            compact ? 'SameFrame' : 'SameFrame · 同帧',
            style: const TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w600,
              letterSpacing: -.5,
            ),
          ),
          if (compact)
            const Text(
              '同帧 / 共享此刻',
              style: TextStyle(fontSize: 10, color: FrameColors.muted),
            ),
        ],
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
      child: CustomPaint(painter: _FrameMarkPainter()),
    ),
  );
}

class _FrameMarkPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    canvas.scale(size.width / 36, size.height / 36);
    final paint = Paint()
      ..color = FrameColors.accent
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
      Paint()..color = FrameColors.accent,
    );
  }

  @override
  bool shouldRepaint(_FrameMarkPainter oldDelegate) => false;
}

class FramePill extends StatelessWidget {
  const FramePill(
    this.text, {
    super.key,
    this.icon,
    this.color = FrameColors.accent,
  });
  final String text;
  final IconData? icon;
  final Color color;
  @override
  Widget build(BuildContext context) => Container(
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

class FrameSection extends StatelessWidget {
  const FrameSection({
    super.key,
    required this.title,
    this.subtitle,
    required this.child,
    this.icon,
    this.trailing,
  });
  final String title;
  final String? subtitle;
  final Widget child;
  final IconData? icon;
  final Widget? trailing;
  @override
  Widget build(BuildContext context) => Card(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              if (icon != null) ...[
                Icon(icon, color: FrameColors.silver, size: 20),
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
          if (subtitle != null) ...[
            const SizedBox(height: 8),
            Text(
              subtitle!,
              style: const TextStyle(
                color: FrameColors.muted,
                fontSize: 13,
                height: 1.6,
              ),
            ),
          ],
          const SizedBox(height: 24),
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
          Icon(icon, size: 30, color: FrameColors.muted.withValues(alpha: .6)),
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
  Widget build(BuildContext context) =>
      ExcludeSemantics(child: CustomPaint(painter: _AperturePainter()));
}

class _AperturePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.clipRect(Offset.zero & size);
    final grid = Paint()
      ..color = Colors.white.withValues(alpha: .035)
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
          ..color = Colors.white.withValues(alpha: .10 + .30 * (1 - t)),
      );
      canvas.restore();
    }
    canvas.restore();
    final marker = Paint()
      ..color = FrameColors.silver.withValues(alpha: .4)
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
  bool shouldRepaint(_AperturePainter oldDelegate) => false;
}
