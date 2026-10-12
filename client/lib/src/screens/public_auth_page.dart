import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../appearance.dart';
import '../design.dart';

/// The public account entry shares the client's existing authentication flow.
class PublicAuthPage extends StatelessWidget {
  const PublicAuthPage({
    super.key,
    required this.registerMode,
    required this.form,
    required this.onHome,
    this.error,
    this.restoring = false,
  });

  final bool registerMode, restoring;
  final Widget form;
  final String? error;
  final VoidCallback onHome;

  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    return Scaffold(
      body: SafeArea(
        child: LayoutBuilder(
          builder: (context, viewport) {
            final compact = viewport.maxWidth < 760;
            return SingleChildScrollView(
              keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
              child: Column(
                children: [
                  Padding(
                    padding: EdgeInsets.symmetric(
                      horizontal: compact ? 22 : 48,
                      vertical: compact ? 20 : 28,
                    ),
                    child: Row(
                      children: [
                        InkWell(
                          borderRadius: BorderRadius.circular(9),
                          onTap: onHome,
                          child: const Padding(
                            padding: EdgeInsets.all(3),
                            child: Row(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                FrameMark(size: 36),
                                SizedBox(width: 12),
                                Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      '同帧',
                                      style: TextStyle(
                                        fontSize: 21,
                                        fontWeight: FontWeight.w700,
                                      ),
                                    ),
                                    Text(
                                      'SAMEFRAME',
                                      style: TextStyle(
                                        fontSize: 9,
                                        letterSpacing: 2,
                                      ),
                                    ),
                                  ],
                                ),
                              ],
                            ),
                          ),
                        ),
                        const Spacer(),
                        const FrameThemeSwitch(),
                      ],
                    ),
                  ),
                  ConstrainedBox(
                    constraints: BoxConstraints(
                      minHeight: math.max(
                        0,
                        viewport.maxHeight - (compact ? 165 : 185),
                      ),
                    ),
                    child: Center(
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 1200),
                        child: Padding(
                          padding: EdgeInsets.fromLTRB(
                            compact ? 22 : 40,
                            compact ? 14 : 40,
                            compact ? 22 : 40,
                            compact ? 34 : 48,
                          ),
                          child: compact
                              ? Column(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.stretch,
                                  children: [
                                    _story(context, compact: true),
                                    const SizedBox(height: 26),
                                    _card(context),
                                  ],
                                )
                              : Row(
                                  crossAxisAlignment: CrossAxisAlignment.center,
                                  children: [
                                    Expanded(child: _story(context)),
                                    const SizedBox(width: 76),
                                    Expanded(child: _card(context)),
                                  ],
                                ),
                        ),
                      ),
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.fromLTRB(24, 10, 24, 24),
                    child: Wrap(
                      alignment: WrapAlignment.center,
                      spacing: 26,
                      runSpacing: 10,
                      children: [
                        Text(
                          '同帧 · 让每一幕，都有共鸣。',
                          style: TextStyle(fontSize: 11, color: colors.muted),
                        ),
                        InkWell(
                          onTap: onHome,
                          child: Text(
                            '返回官网',
                            style: TextStyle(fontSize: 11, color: colors.muted),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _story(BuildContext context, {bool compact = false}) {
    final colors = FrameColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'SAMEFRAME / ${registerMode ? 'JOIN THE MOMENT' : 'WELCOME BACK'}',
          style: TextStyle(fontSize: 10, letterSpacing: 2, color: colors.muted),
        ),
        SizedBox(height: compact ? 15 : 27),
        Text(
          registerMode ? '下一场好故事，' : '欢迎回来，',
          style: TextStyle(
            fontSize: compact ? 28 : 43,
            fontWeight: FontWeight.w600,
            letterSpacing: -1.2,
            height: 1.5,
          ),
        ),
        Text(
          registerMode ? '从这里开始。' : '继续你们的故事。',
          style: TextStyle(
            fontSize: compact ? 28 : 43,
            fontWeight: FontWeight.w600,
            letterSpacing: -1.2,
            height: 1.5,
            color: colors.muted,
          ),
        ),
        if (!compact) ...[
          const SizedBox(height: 24),
          Text(
            '一个放映室，一部好电影。\n和你想见的人，共享每一个好瞬间。',
            style: TextStyle(fontSize: 14, height: 2, color: colors.muted),
          ),
          const SizedBox(height: 26),
          const SizedBox(
            height: 225,
            width: 390,
            child: CustomPaint(painter: _EntryArtwork()),
          ),
          const SizedBox(height: 16),
          Text(
            'A SHARED MOMENT. YOUR OWN SPACE.',
            style: TextStyle(
              fontSize: 9,
              letterSpacing: 1.8,
              color: colors.muted,
            ),
          ),
        ],
      ],
    );
  }

  Widget _card(BuildContext context) {
    final colors = FrameColors.of(context);
    final compact = MediaQuery.sizeOf(context).width < 760;
    return Container(
      padding: EdgeInsets.all(compact ? 24 : 36),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(24),
        border: Border.all(color: colors.border),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: .025),
            blurRadius: 50,
            offset: const Offset(0, 14),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: colors.input,
                border: Border.all(color: colors.border),
                borderRadius: BorderRadius.circular(13),
              ),
              child: Icon(
                registerMode
                    ? Icons.person_add_alt_1_outlined
                    : Icons.lock_outline_rounded,
                size: 21,
              ),
            ),
          ),
          const SizedBox(height: 22),
          Text(
            registerMode ? '创建同帧账号' : '登录同帧',
            style: const TextStyle(
              fontSize: 24,
              fontWeight: FontWeight.w600,
              letterSpacing: -.5,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            registerMode ? '为下一次相聚，留一个位置。' : '使用你的账号，进入同帧观影空间。',
            style: TextStyle(fontSize: 12, color: colors.muted, height: 1.8),
          ),
          const SizedBox(height: 28),
          if (restoring)
            const Padding(
              padding: EdgeInsets.only(bottom: 16),
              child: LinearProgressIndicator(),
            ),
          form,
          if (error != null)
            Padding(
              padding: const EdgeInsets.only(top: 14),
              child: Semantics(
                liveRegion: true,
                child: Text(
                  error!,
                  style: TextStyle(
                    color: Theme.of(context).colorScheme.error,
                    fontSize: 12,
                    height: 1.7,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _EntryArtwork extends CustomPainter {
  const _EntryArtwork();
  @override
  void paint(Canvas canvas, Size size) {
    final line = Paint()
      ..color = const Color(0x16787885)
      ..strokeWidth = .6;
    for (double x = 25; x < size.width; x += 27) {
      canvas.drawLine(Offset(x, 12), Offset(x, size.height - 12), line);
    }
    for (double y = 12; y < size.height; y += 27) {
      canvas.drawLine(Offset(25, y), Offset(size.width - 15, y), line);
    }
    canvas.save();
    canvas.translate(size.width * .5, size.height * .48);
    canvas.rotate(-.2);
    final back = RRect.fromRectAndRadius(
      const Rect.fromLTWH(-112, -77, 126, 126),
      const Radius.circular(27),
    );
    canvas.drawRRect(back, Paint()..color = const Color(0xFFF7F7FA));
    canvas.drawRRect(
      back,
      Paint()
        ..color = const Color(0xFFCBCBD3)
        ..style = PaintingStyle.stroke
        ..strokeWidth = .8,
    );
    final front = RRect.fromRectAndRadius(
      const Rect.fromLTWH(-45, -25, 130, 130),
      const Radius.circular(28),
    );
    canvas.drawRRect(front, Paint()..color = const Color(0xFF19191C));
    final play = Path()
      ..moveTo(8, 15)
      ..lineTo(32, 30)
      ..lineTo(8, 45)
      ..close();
    canvas.drawPath(
      play,
      Paint()
        ..color = const Color(0xFFF4F4F6)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2.5
        ..strokeJoin = StrokeJoin.round,
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant _EntryArtwork oldDelegate) => false;
}
