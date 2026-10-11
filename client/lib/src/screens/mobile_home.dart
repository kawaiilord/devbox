import 'package:flutter/material.dart';

import '../design.dart';

class MobileHome extends StatefulWidget {
  const MobileHome({
    super.key,
    required this.name,
    required this.create,
    required this.join,
    this.initiallyJoining = false,
    this.error,
  });

  final String name;
  final Widget create, join;
  final String? error;
  final bool initiallyJoining;

  @override
  State<MobileHome> createState() => _MobileHomeState();
}

class _MobileHomeState extends State<MobileHome> {
  late bool _joining = widget.initiallyJoining;

  @override
  void didUpdateWidget(covariant MobileHome oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!oldWidget.initiallyJoining && widget.initiallyJoining) _joining = true;
  }

  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          clipBehavior: Clip.antiAlias,
          decoration: BoxDecoration(
            color: colors.surface,
            borderRadius: BorderRadius.circular(18),
            border: Border.all(color: colors.border),
          ),
          child: Stack(
            children: [
              const Positioned(
                right: -14,
                top: -4,
                bottom: -4,
                width: 164,
                child: Opacity(opacity: .65, child: CinemaArtwork()),
              ),
              Padding(
                padding: const EdgeInsets.all(20),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      widget.name.isEmpty ? '欢迎回来' : '嗨，${widget.name}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(color: colors.muted, fontSize: 12),
                    ),
                    const SizedBox(height: 8),
                    const Text(
                      '好故事，一起看。',
                      style: TextStyle(
                        fontSize: 24,
                        fontWeight: FontWeight.w600,
                        letterSpacing: -.8,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      '同步每一幕，分享每一刻',
                      style: TextStyle(color: colors.muted, fontSize: 12),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 20),
        SegmentedButton<bool>(
          showSelectedIcon: false,
          segments: const [
            ButtonSegment(
              value: false,
              icon: Icon(Icons.add_rounded, size: 18),
              label: Text('创建房间'),
            ),
            ButtonSegment(
              value: true,
              icon: Icon(Icons.login_rounded, size: 18),
              label: Text('加入房间'),
            ),
          ],
          selected: {_joining},
          onSelectionChanged: (value) =>
              setState(() => _joining = value.single),
          style: ButtonStyle(
            minimumSize: const WidgetStatePropertyAll(Size(0, 48)),
            visualDensity: VisualDensity.standard,
            backgroundColor: WidgetStateProperty.resolveWith(
              (states) => states.contains(WidgetState.selected)
                  ? colors.accent
                  : colors.surface,
            ),
            foregroundColor: WidgetStateProperty.resolveWith(
              (states) => states.contains(WidgetState.selected)
                  ? colors.background
                  : colors.muted,
            ),
            side: WidgetStatePropertyAll(BorderSide(color: colors.border)),
            textStyle: const WidgetStatePropertyAll(
              TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
            ),
          ),
        ),
        const SizedBox(height: 12),
        _joining ? widget.join : widget.create,
        if (widget.error != null) ...[
          const SizedBox(height: 12),
          Text(
            widget.error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ],
        const SizedBox(height: 18),
        Text(
          '一部好电影，一次新的相聚。',
          textAlign: TextAlign.center,
          style: TextStyle(fontSize: 12, color: colors.muted),
        ),
      ],
    );
  }
}
