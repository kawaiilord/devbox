import 'package:flutter/material.dart';

import 'design.dart';

class MemberBadge extends StatelessWidget {
  const MemberBadge({super.key, this.compact = false});
  final bool compact;
  @override
  Widget build(BuildContext context) => Semantics(
    label: 'PLUS 会员',
    child: Container(
      padding: EdgeInsets.symmetric(horizontal: compact ? 5 : 8, vertical: 3),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          colors: [Color(0xFF39393F), Color(0xFF151518)],
        ),
        border: Border.all(color: const Color(0xFF85858F)),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.workspace_premium_outlined,
            size: compact ? 11 : 14,
            color: const Color(0xFFEAEAF0),
          ),
          const SizedBox(width: 4),
          Text(
            'PLUS',
            style: TextStyle(
              color: const Color(0xFFF5F5F8),
              fontSize: compact ? 9 : 10,
              letterSpacing: 1.2,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    ),
  );
}

class MemberAvatar extends StatelessWidget {
  const MemberAvatar({
    super.key,
    required this.name,
    required this.member,
    this.radius = 22,
  });
  final String name;
  final bool member;
  final double radius;
  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    final avatar = CircleAvatar(
      radius: radius,
      backgroundColor: colors.elevated,
      foregroundColor: colors.text,
      child: Text(
        name.isEmpty ? 'S' : name.characters.first.toUpperCase(),
        style: TextStyle(fontSize: radius * .7, fontWeight: FontWeight.w600),
      ),
    );
    if (!member) return avatar;
    return Container(
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            Color(0xFFB8B8C4),
            Color(0xFFF0F0F5),
            Color(0xFF6E6E79),
            Color(0xFFD2D2DA),
          ],
        ),
      ),
      child: Container(
        padding: const EdgeInsets.all(2),
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: colors.background,
        ),
        child: avatar,
      ),
    );
  }
}

class MemberName extends StatelessWidget {
  const MemberName({
    super.key,
    required this.name,
    required this.member,
    this.style,
  });
  final String name;
  final bool member;
  final TextStyle? style;
  @override
  Widget build(BuildContext context) => Row(
    children: [
      Flexible(
        child: Text(
          name,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: style,
        ),
      ),
      if (member) ...[
        const SizedBox(width: 7),
        const MemberBadge(compact: true),
      ],
    ],
  );
}

class MembershipCard extends StatelessWidget {
  const MembershipCard({
    super.key,
    required this.name,
    required this.active,
    required this.detail,
  });
  final String name, detail;
  final bool active;
  @override
  Widget build(BuildContext context) => Container(
    width: double.infinity,
    clipBehavior: Clip.antiAlias,
    decoration: BoxDecoration(
      borderRadius: BorderRadius.circular(22),
      gradient: const LinearGradient(
        begin: Alignment.topLeft,
        end: Alignment.bottomRight,
        colors: [Color(0xFF303034), Color(0xFF141416), Color(0xFF232327)],
      ),
      border: Border.all(color: const Color(0xFF64646E)),
    ),
    child: Stack(
      children: [
        const Positioned.fill(
          child: IgnorePointer(
            child: CustomPaint(painter: _MembershipArtwork()),
          ),
        ),
        Padding(
          padding: const EdgeInsets.all(26),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const Text(
                    'SAMEFRAME',
                    style: TextStyle(
                      color: Color(0xFFC8C8D1),
                      fontSize: 11,
                      letterSpacing: 2.5,
                    ),
                  ),
                  const Spacer(),
                  if (active)
                    const MemberBadge()
                  else
                    const Icon(
                      Icons.workspace_premium_outlined,
                      color: Color(0xFFE7E7EE),
                    ),
                ],
              ),
              const SizedBox(height: 30),
              const Text(
                'PLUS',
                style: TextStyle(
                  fontSize: 44,
                  fontWeight: FontWeight.w500,
                  letterSpacing: 7,
                  color: Color(0xFFF4F4F7),
                ),
              ),
              const SizedBox(height: 4),
              Text(
                active ? '尊享会员 · 让相聚长久一点' : '每日签到，兑换你的会员时光',
                style: const TextStyle(
                  color: Color(0xFFCCCCD5),
                  fontSize: 13,
                  height: 1.7,
                ),
              ),
              const SizedBox(height: 28),
              Wrap(
                spacing: 14,
                runSpacing: 6,
                children: [
                  Text(
                    name,
                    style: const TextStyle(
                      color: Color(0xFFF4F4F7),
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                  Text(
                    detail,
                    style: const TextStyle(
                      color: Color(0xFFBFBFC9),
                      fontSize: 12,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ],
    ),
  );
}

class _MembershipArtwork extends CustomPainter {
  const _MembershipArtwork();
  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width * .88, size.height * .46);
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = .7
      ..color = const Color(0x28FFFFFF);
    for (var i = 0; i < 8; i++) {
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromCenter(
            center: center,
            width: 110.0 + i * 15,
            height: 110.0 + i * 15,
          ),
          const Radius.circular(28),
        ),
        paint,
      );
    }
    final gem = Path()
      ..moveTo(center.dx, center.dy - 48)
      ..lineTo(center.dx + 38, center.dy)
      ..lineTo(center.dx, center.dy + 48)
      ..lineTo(center.dx - 38, center.dy)
      ..close();
    canvas.drawPath(gem, paint..color = const Color(0x66FFFFFF));
    canvas.drawLine(
      Offset(center.dx, center.dy - 48),
      Offset(center.dx, center.dy + 48),
      paint,
    );
  }

  @override
  bool shouldRepaint(covariant _MembershipArtwork oldDelegate) => false;
}
