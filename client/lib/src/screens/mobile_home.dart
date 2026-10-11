import 'package:flutter/material.dart';

import '../design.dart';

class MobileHome extends StatefulWidget {
  const MobileHome({
    super.key,
    required this.name,
    required this.create,
    required this.join,
    required this.onDiscover,
    required this.onSources,
    required this.shortcuts,
    this.verification,
    this.error,
  });

  final String name;
  final Widget create, join;
  final Widget? verification;
  final String? error;
  final VoidCallback onDiscover, onSources;
  final List<({IconData icon, String label, VoidCallback onTap})> shortcuts;

  @override
  State<MobileHome> createState() => _MobileHomeState();
}

class _MobileHomeState extends State<MobileHome> {
  bool _joining = false;

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
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: _QuickEntry(
                icon: Icons.explore_outlined,
                label: '发现房间',
                onTap: widget.onDiscover,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: _QuickEntry(
                icon: Icons.video_library_outlined,
                label: '我的媒体',
                onTap: widget.onSources,
              ),
            ),
          ],
        ),
        const SizedBox(height: 22),
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
        const SizedBox(height: 22),
        Text('你的观影空间', style: TextStyle(color: colors.muted, fontSize: 12)),
        const SizedBox(height: 12),
        Row(
          children: [
            for (var i = 0; i < widget.shortcuts.length; i++) ...[
              if (i > 0) const SizedBox(width: 10),
              Expanded(
                child: Card(
                  clipBehavior: Clip.antiAlias,
                  child: InkWell(
                    onTap: widget.shortcuts[i].onTap,
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 16,
                      ),
                      child: Column(
                        children: [
                          Icon(
                            widget.shortcuts[i].icon,
                            size: 22,
                            color: colors.silver,
                          ),
                          const SizedBox(height: 8),
                          Text(
                            widget.shortcuts[i].label,
                            textAlign: TextAlign.center,
                            style: const TextStyle(fontSize: 12),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ],
        ),
        if (widget.verification != null) ...[
          const SizedBox(height: 14),
          Card(
            child: ExpansionTile(
              tilePadding: const EdgeInsets.symmetric(horizontal: 16),
              shape: const Border(),
              leading: Icon(
                Icons.mark_email_unread_outlined,
                size: 18,
                color: colors.muted,
              ),
              title: const Text('验证邮箱，保护账号', style: TextStyle(fontSize: 12)),
              children: [widget.verification!],
            ),
          ),
        ],
      ],
    );
  }
}

class _QuickEntry extends StatelessWidget {
  const _QuickEntry({
    required this.icon,
    required this.label,
    required this.onTap,
  });
  final IconData icon;
  final String label;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) => Card(
    clipBehavior: Clip.antiAlias,
    child: InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 17),
        child: Row(
          children: [
            Icon(icon, size: 19, color: FrameColors.of(context).silver),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
            Icon(
              Icons.chevron_right_rounded,
              size: 16,
              color: FrameColors.of(context).muted,
            ),
          ],
        ),
      ),
    ),
  );
}
