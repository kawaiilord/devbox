import 'dart:math' as math;

import 'package:flutter/material.dart';

abstract final class FrameColors {
  static const background = Color(0xFF0C1119);
  static const surface = Color(0xFF151D28);
  static const elevated = Color(0xFF1C2633);
  static const border = Color(0xFF293442);
  static const muted = Color(0xFF94A2B3);
  static const text = Color(0xFFF1F5F9);
  static const mint = Color(0xFF7CE4C3);
  static const gold = Color(0xFFEAC58C);
}

ThemeData frameTheme() {
  final scheme =
      ColorScheme.fromSeed(
        seedColor: FrameColors.mint,
        brightness: Brightness.dark,
        surface: FrameColors.surface,
      ).copyWith(
        primary: FrameColors.mint,
        onPrimary: const Color(0xFF102C23),
        secondary: FrameColors.gold,
        onSurface: FrameColors.text,
        outline: FrameColors.border,
      );
  final base = ThemeData(useMaterial3: true, colorScheme: scheme);
  final shape = RoundedRectangleBorder(borderRadius: BorderRadius.circular(12));
  return base.copyWith(
    scaffoldBackgroundColor: FrameColors.background,
    textTheme: base.textTheme.copyWith(
      headlineLarge: const TextStyle(
        fontSize: 34,
        fontWeight: FontWeight.w700,
        height: 1.3,
        letterSpacing: -1,
      ),
      headlineSmall: const TextStyle(
        fontSize: 25,
        fontWeight: FontWeight.w700,
        height: 1.4,
      ),
      titleLarge: const TextStyle(
        fontSize: 20,
        fontWeight: FontWeight.w600,
        height: 1.4,
      ),
      titleMedium: const TextStyle(
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
      toolbarHeight: 76,
      titleTextStyle: TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
    ),
    cardTheme: CardThemeData(
      color: FrameColors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
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
      fillColor: const Color(0xFF101722),
      contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 18),
      labelStyle: const TextStyle(color: FrameColors.muted, fontSize: 14),
      hintStyle: const TextStyle(color: Color(0xFF67768A)),
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
        borderSide: const BorderSide(color: FrameColors.mint, width: 1.4),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: FrameColors.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
    ),
    chipTheme: base.chipTheme.copyWith(
      side: const BorderSide(color: FrameColors.border),
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
      Container(
        width: 38,
        height: 38,
        decoration: BoxDecoration(
          color: FrameColors.mint,
          borderRadius: BorderRadius.circular(12),
        ),
        child: const Icon(
          Icons.join_inner_rounded,
          color: Color(0xFF163B30),
          size: 26,
        ),
      ),
      const SizedBox(width: 11),
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            compact ? '同帧' : 'SameFrame · 同帧',
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w700,
              letterSpacing: -.5,
            ),
          ),
          if (compact)
            const Text(
              'S A M E F R A M E',
              style: TextStyle(fontSize: 8, color: FrameColors.muted),
            ),
        ],
      ),
    ],
  );
}

class FramePill extends StatelessWidget {
  const FramePill(
    this.text, {
    super.key,
    this.icon,
    this.color = FrameColors.mint,
  });
  final String text;
  final IconData? icon;
  final Color color;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 6),
    decoration: BoxDecoration(
      color: color.withValues(alpha: .08),
      borderRadius: BorderRadius.circular(8),
      border: Border.all(color: color.withValues(alpha: .2)),
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
                Icon(icon, color: FrameColors.mint, size: 22),
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

/// Original vector landscape, painted locally at any screen density.
class CinemaArtwork extends StatelessWidget {
  const CinemaArtwork({super.key});
  @override
  Widget build(BuildContext context) =>
      CustomPaint(painter: _LandscapePainter());
}

class _LandscapePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final rect = Offset.zero & size;
    canvas.drawRect(
      rect,
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [Color(0xFF172E38), Color(0xFF6A6660), Color(0xFFCE9671)],
        ).createShader(rect),
    );
    final sun = Offset(size.width * .70, size.height * .37);
    canvas.drawCircle(
      sun,
      size.height * .21,
      Paint()
        ..shader = RadialGradient(
          colors: [
            const Color(0xFFECC3A0).withValues(alpha: .35),
            Colors.transparent,
          ],
        ).createShader(Rect.fromCircle(center: sun, radius: size.height * .21)),
    );
    canvas.drawCircle(
      sun,
      size.height * .13,
      Paint()..color = const Color(0xFFF0C9A5),
    );
    for (var layer = 0; layer < 4; layer++) {
      final path = Path()..moveTo(0, size.height);
      for (var i = 0; i <= 90; i++) {
        final x = size.width * i / 90;
        final y =
            size.height * (.50 + layer * .11) +
            math.sin(i * .055 + layer * 1.6) * size.height * .09 +
            math.sin(i * .14 + layer) * size.height * .036;
        path.lineTo(x, y);
      }
      path.lineTo(size.width, size.height);
      path.close();
      canvas.drawPath(
        path,
        Paint()
          ..color = const [
            Color(0xFF54747A),
            Color(0xFF335761),
            Color(0xFF24414D),
            Color(0xFF152C38),
          ][layer],
      );
    }
    final random = math.Random(24);
    for (var i = 0; i < 30; i++) {
      canvas.drawCircle(
        Offset(
          random.nextDouble() * size.width,
          random.nextDouble() * size.height * .28,
        ),
        .5 + random.nextDouble() * .6,
        Paint()
          ..color = Colors.white.withValues(
            alpha: .18 + random.nextDouble() * .2,
          ),
      );
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
