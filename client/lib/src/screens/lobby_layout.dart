import 'package:flutter/material.dart';

import '../design.dart';

class LobbyDestination {
  const LobbyDestination(this.icon, this.label, this.onTap);
  final IconData icon;
  final String label;
  final VoidCallback onTap;
}

class LobbyLayout extends StatelessWidget {
  const LobbyLayout({
    super.key,
    required this.signedIn,
    required this.name,
    required this.login,
    required this.create,
    required this.join,
    required this.verification,
    required this.destinations,
    required this.onSearch,
    required this.onDiscover,
    required this.onGuestJoin,
    required this.onMembership,
    required this.onUpdates,
    required this.onLogout,
    this.error,
  });
  final bool signedIn;
  final String name;
  final Widget login, create, join;
  final Widget? verification;
  final List<LobbyDestination> destinations;
  final VoidCallback onSearch, onMembership, onUpdates, onLogout;
  final VoidCallback onDiscover, onGuestJoin;
  final String? error;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final desktop = constraints.maxWidth >= 1080;
      return Scaffold(
        appBar: desktop
            ? null
            : AppBar(
                title: FrameBrand(compact: constraints.maxWidth < 420),
                actions: [
                  IconButton(
                    onPressed: signedIn ? onSearch : onUpdates,
                    tooltip: signedIn ? '影视搜索' : '检查更新',
                    icon: Icon(
                      signedIn
                          ? Icons.search_rounded
                          : Icons.system_update_alt_rounded,
                    ),
                  ),
                ],
              ),
        drawer: desktop
            ? null
            : Drawer(
                backgroundColor: FrameColors.sidebar,
                child: SafeArea(child: _navigation(context, closeDrawer: true)),
              ),
        body: SafeArea(
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (desktop) SizedBox(width: 232, child: _navigation(context)),
              Expanded(
                child: Column(
                  children: [
                    if (desktop) _toolbar(),
                    Expanded(
                      child: SingleChildScrollView(
                        padding: EdgeInsets.fromLTRB(
                          desktop ? 40 : 20,
                          desktop ? 36 : 24,
                          desktop ? 40 : 20,
                          28,
                        ),
                        child: Center(
                          child: ConstrainedBox(
                            constraints: const BoxConstraints(maxWidth: 1180),
                            child: signedIn
                                ? _workspace(context)
                                : _welcome(context),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      );
    },
  );

  Widget _toolbar() => Container(
    height: 68,
    padding: const EdgeInsets.symmetric(horizontal: 32),
    decoration: const BoxDecoration(
      border: Border(bottom: BorderSide(color: FrameColors.border)),
    ),
    child: Row(
      children: [
        const Icon(
          Icons.space_dashboard_outlined,
          size: 17,
          color: FrameColors.muted,
        ),
        const SizedBox(width: 12),
        const Text(
          '我的空间',
          style: TextStyle(fontSize: 13, color: FrameColors.muted),
        ),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 12),
          child: Text('/', style: TextStyle(color: FrameColors.border)),
        ),
        Text(signedIn ? '观影大厅' : '欢迎', style: const TextStyle(fontSize: 13)),
        const Spacer(),
        TextButton.icon(
          onPressed: onDiscover,
          icon: const Icon(Icons.explore_outlined, size: 17),
          label: const Text('发现公开房间'),
        ),
        const SizedBox(width: 8),
        IconButton(
          onPressed: onUpdates,
          tooltip: '检查更新',
          icon: const Icon(Icons.system_update_alt_rounded, size: 19),
        ),
      ],
    ),
  );

  Widget _welcome(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      const _Eyebrow('SAMEFRAME  /  SHARED CINEMA'),
      const SizedBox(height: 36),
      LayoutBuilder(
        builder: (context, box) {
          final wide = box.maxWidth >= 800;
          final intro = Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '此刻，\n我们同帧。',
                style: TextStyle(
                  fontSize: wide ? 58 : 40,
                  height: 1.22,
                  fontWeight: FontWeight.w500,
                  letterSpacing: -2,
                ),
              ),
              const SizedBox(height: 20),
              const Text(
                '把距离留在屏幕之外。\n一个放映室，一部好电影，和你想见的人。',
                style: TextStyle(
                  color: FrameColors.muted,
                  fontSize: 14,
                  height: 1.9,
                ),
              ),
              const SizedBox(height: 24),
              Wrap(
                spacing: 10,
                runSpacing: 10,
                children: [
                  OutlinedButton.icon(
                    onPressed: onDiscover,
                    icon: const Icon(Icons.explore_outlined, size: 18),
                    label: const Text('发现公开房间'),
                  ),
                  TextButton.icon(
                    onPressed: onGuestJoin,
                    icon: const Icon(Icons.north_east_rounded, size: 17),
                    label: const Text('访客加入'),
                  ),
                ],
              ),
              if (wide) ...[const SizedBox(height: 32), const _ApertureCard()],
            ],
          );
          return wide
              ? Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    Expanded(child: intro),
                    const SizedBox(width: 52),
                    Expanded(child: login),
                  ],
                )
              : Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [intro, const SizedBox(height: 32), login],
                );
        },
      ),
      _error(context),
      const SizedBox(height: 40),
      const Divider(height: 1),
      const SizedBox(height: 22),
      const Wrap(
        spacing: 28,
        runSpacing: 14,
        children: [
          _Benefit(Icons.sync_rounded, '同一进度，同一瞬间'),
          _Benefit(Icons.chat_bubble_outline_rounded, '边看边聊'),
          _Benefit(Icons.devices_outlined, '在你喜欢的屏幕相见'),
        ],
      ),
      const SizedBox(height: 28),
      const _Footer(),
    ],
  );

  Widget _workspace(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const _Eyebrow('YOUR PERSONAL CINEMA'),
                const SizedBox(height: 10),
                Text(
                  '今晚，一起看点什么？',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
              ],
            ),
          ),
          if (MediaQuery.sizeOf(context).width >= 700)
            OutlinedButton.icon(
              onPressed: onSearch,
              icon: const Icon(Icons.search_rounded, size: 18),
              label: const Text('搜索电影 / 剧集'),
            ),
        ],
      ),
      const SizedBox(height: 28),
      const _CinemaHero(),
      const SizedBox(height: 30),
      Row(
        children: [
          const _Eyebrow('开始一场放映'),
          const Spacer(),
          TextButton.icon(
            onPressed: onDiscover,
            icon: const Icon(Icons.arrow_outward_rounded, size: 16),
            label: const Text('发现房间'),
          ),
        ],
      ),
      const SizedBox(height: 12),
      LayoutBuilder(
        builder: (context, box) => box.maxWidth < 720
            ? Column(children: [create, const SizedBox(height: 16), join])
            : Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(flex: 6, child: create),
                  const SizedBox(width: 20),
                  Expanded(flex: 5, child: join),
                ],
              ),
      ),
      _error(context),
      const SizedBox(height: 24),
      LayoutBuilder(
        builder: (context, box) {
          final items = [
            _Entry(destinations[0], '接着上次的故事'),
            _Entry(destinations[1], '与朋友分享此刻'),
            _Entry(destinations[2], '珍藏共同的观影时光'),
          ];
          return box.maxWidth < 700
              ? Column(
                  children: [
                    for (final item in items)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 10),
                        child: item,
                      ),
                  ],
                )
              : Row(
                  children: [
                    for (var i = 0; i < items.length; i++) ...[
                      if (i > 0) const SizedBox(width: 12),
                      Expanded(child: items[i]),
                    ],
                  ],
                );
        },
      ),
      if (verification != null) ...[
        const SizedBox(height: 20),
        Card(
          child: ExpansionTile(
            shape: const Border(),
            leading: const Icon(
              Icons.mark_email_unread_outlined,
              color: FrameColors.muted,
              size: 20,
            ),
            title: const Text('验证邮箱，保护你的账号', style: TextStyle(fontSize: 13)),
            subtitle: const Text(
              '展开完成邮箱验证',
              style: TextStyle(fontSize: 12, color: FrameColors.muted),
            ),
            children: [verification!],
          ),
        ),
      ],
      const SizedBox(height: 28),
      const _Footer(),
    ],
  );

  Widget _error(BuildContext context) => error == null
      ? const SizedBox.shrink()
      : Padding(
          padding: const EdgeInsets.only(top: 16),
          child: Text(
            error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        );

  void _navigate(BuildContext context, VoidCallback action, bool closeDrawer) {
    if (closeDrawer) Navigator.of(context).pop();
    action();
  }

  Widget _navigation(
    BuildContext context, {
    bool closeDrawer = false,
  }) => Container(
    decoration: const BoxDecoration(
      color: FrameColors.sidebar,
      border: Border(right: BorderSide(color: FrameColors.border)),
    ),
    child: Column(
      children: [
        const Padding(
          padding: EdgeInsets.fromLTRB(22, 27, 22, 28),
          child: FrameBrand(compact: true),
        ),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.symmetric(horizontal: 14),
            children: [
              _NavItem(
                icon: Icons.space_dashboard_outlined,
                label: '观影大厅',
                selected: true,
                onTap: closeDrawer ? () => Navigator.of(context).pop() : null,
              ),
              const SizedBox(height: 5),
              _NavItem(
                icon: Icons.explore_outlined,
                label: '发现房间',
                onTap: () => _navigate(context, onDiscover, closeDrawer),
              ),
              _NavItem(
                icon: Icons.meeting_room_outlined,
                label: '访客加入',
                onTap: () => _navigate(context, onGuestJoin, closeDrawer),
              ),
              const Padding(
                padding: EdgeInsets.fromLTRB(12, 28, 0, 10),
                child: _Eyebrow('资料库'),
              ),
              for (var i = 0; i < destinations.length; i++) ...[
                if (i == 3)
                  const Padding(
                    padding: EdgeInsets.fromLTRB(12, 28, 0, 10),
                    child: _Eyebrow('连接与设置'),
                  ),
                _NavItem(
                  icon: destinations[i].icon,
                  label: destinations[i].label,
                  onTap: signedIn
                      ? () => _navigate(
                          context,
                          destinations[i].onTap,
                          closeDrawer,
                        )
                      : null,
                ),
              ],
              _NavItem(
                icon: Icons.system_update_alt_rounded,
                label: '安全更新',
                onTap: () => _navigate(context, onUpdates, closeDrawer),
              ),
              const SizedBox(height: 30),
              if (signedIn)
                Card(
                  child: InkWell(
                    borderRadius: BorderRadius.circular(16),
                    onTap: () => _navigate(context, onMembership, closeDrawer),
                    child: const Padding(
                      padding: EdgeInsets.all(16),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Text(
                                'SameFrame',
                                style: TextStyle(fontWeight: FontWeight.w500),
                              ),
                              SizedBox(width: 6),
                              FramePill('PLUS'),
                              Spacer(),
                              Icon(Icons.arrow_outward_rounded, size: 15),
                            ],
                          ),
                          SizedBox(height: 10),
                          Text(
                            '探索更多相聚的可能',
                            style: TextStyle(
                              fontSize: 12,
                              color: FrameColors.muted,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                )
              else
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 12),
                  child: Text(
                    '登录后开启\n你的专属观影空间。',
                    style: TextStyle(
                      color: FrameColors.muted,
                      fontSize: 12,
                      height: 1.8,
                    ),
                  ),
                ),
            ],
          ),
        ),
        Container(
          margin: const EdgeInsets.all(16),
          padding: const EdgeInsets.only(top: 16),
          decoration: const BoxDecoration(
            border: Border(top: BorderSide(color: FrameColors.border)),
          ),
          child: Row(
            children: [
              CircleAvatar(
                radius: 17,
                backgroundColor: FrameColors.elevated,
                foregroundColor: FrameColors.text,
                child: signedIn
                    ? Text(
                        name.isEmpty ? 'S' : name.characters.first,
                        style: const TextStyle(fontSize: 12),
                      )
                    : const Icon(Icons.person_outline_rounded, size: 19),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      signedIn ? name : '欢迎来到同帧',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 12),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      signedIn ? '个人空间' : '共享每一个好故事',
                      style: const TextStyle(
                        fontSize: 10,
                        color: FrameColors.muted,
                      ),
                    ),
                  ],
                ),
              ),
              if (signedIn)
                IconButton(
                  onPressed: () => _navigate(context, onLogout, closeDrawer),
                  tooltip: '退出登录',
                  icon: const Icon(
                    Icons.logout_rounded,
                    size: 17,
                    color: FrameColors.muted,
                  ),
                ),
            ],
          ),
        ),
      ],
    ),
  );
}

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.icon,
    required this.label,
    this.onTap,
    this.selected = false,
  });
  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final bool selected;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 2),
    child: Material(
      color: selected ? FrameColors.elevated : Colors.transparent,
      borderRadius: BorderRadius.circular(9),
      child: ListTile(
        enabled: onTap != null || selected,
        dense: true,
        contentPadding: const EdgeInsets.symmetric(horizontal: 12),
        minLeadingWidth: 20,
        horizontalTitleGap: 10,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(9)),
        leading: Icon(
          icon,
          size: 19,
          color: selected ? FrameColors.text : FrameColors.muted,
        ),
        title: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            color: selected || onTap != null
                ? FrameColors.text
                : FrameColors.muted,
          ),
        ),
        trailing: selected
            ? Container(
                width: 4,
                height: 4,
                decoration: const BoxDecoration(
                  color: FrameColors.accent,
                  shape: BoxShape.circle,
                ),
              )
            : null,
        onTap: onTap,
      ),
    ),
  );
}

class _Eyebrow extends StatelessWidget {
  const _Eyebrow(this.text);
  final String text;
  @override
  Widget build(BuildContext context) => Text(
    text,
    style: const TextStyle(
      fontSize: 10,
      letterSpacing: 1.5,
      color: FrameColors.muted,
      fontWeight: FontWeight.w500,
    ),
  );
}

class _ApertureCard extends StatelessWidget {
  const _ApertureCard();
  @override
  Widget build(BuildContext context) => Container(
    height: 160,
    clipBehavior: Clip.antiAlias,
    decoration: BoxDecoration(
      color: FrameColors.sidebar,
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: FrameColors.border),
    ),
    child: const Stack(
      fit: StackFit.expand,
      children: [
        CinemaArtwork(),
        Positioned(
          left: 20,
          bottom: 16,
          child: _Eyebrow('CONNECTED BY THE FRAME'),
        ),
      ],
    ),
  );
}

class _CinemaHero extends StatelessWidget {
  const _CinemaHero();
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      final compact = box.maxWidth < 600;
      return Container(
        height: compact ? 214 : 226,
        clipBehavior: Clip.antiAlias,
        decoration: BoxDecoration(
          color: FrameColors.surface,
          borderRadius: BorderRadius.circular(18),
          border: Border.all(color: FrameColors.border),
        ),
        child: Stack(
          fit: StackFit.expand,
          children: [
            Positioned(
              right: 0,
              top: 0,
              bottom: 0,
              width: box.maxWidth * (compact ? .75 : .55),
              child: Opacity(
                opacity: compact ? .35 : 1,
                child: const CinemaArtwork(),
              ),
            ),
            Padding(
              padding: EdgeInsets.all(compact ? 24 : 30),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Row(
                    children: [
                      Icon(
                        Icons.blur_on_rounded,
                        size: 18,
                        color: FrameColors.silver,
                      ),
                      SizedBox(width: 9),
                      _Eyebrow('SHARED MOMENTS. ONE FRAME.'),
                    ],
                  ),
                  const Spacer(),
                  Text(
                    '好故事，值得一起看。',
                    style: TextStyle(
                      fontSize: compact ? 24 : 32,
                      fontWeight: FontWeight.w500,
                      letterSpacing: -1,
                      height: 1.3,
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    '同步播放  /  实时交流  /  共享此刻',
                    style: TextStyle(
                      fontSize: 12,
                      color: FrameColors.muted,
                      letterSpacing: .5,
                    ),
                  ),
                  const SizedBox(height: 6),
                ],
              ),
            ),
            if (!compact)
              const Positioned(
                right: 22,
                bottom: 18,
                child: _Eyebrow('01 — SAMEFRAME'),
              ),
          ],
        ),
      );
    },
  );
}

class _Benefit extends StatelessWidget {
  const _Benefit(this.icon, this.text);
  final IconData icon;
  final String text;
  @override
  Widget build(BuildContext context) => Row(
    mainAxisSize: MainAxisSize.min,
    children: [
      Icon(icon, size: 16, color: FrameColors.silver),
      const SizedBox(width: 9),
      Text(
        text,
        style: const TextStyle(fontSize: 12, color: FrameColors.muted),
      ),
    ],
  );
}

class _Entry extends StatelessWidget {
  const _Entry(this.destination, this.detail);
  final LobbyDestination destination;
  final String detail;
  @override
  Widget build(BuildContext context) => Card(
    clipBehavior: Clip.antiAlias,
    child: InkWell(
      onTap: destination.onTap,
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Row(
          children: [
            Icon(destination.icon, color: FrameColors.silver, size: 21),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    destination.label,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    detail,
                    style: const TextStyle(
                      fontSize: 11,
                      color: FrameColors.muted,
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 6),
            const Icon(
              Icons.north_east_rounded,
              color: FrameColors.muted,
              size: 15,
            ),
          ],
        ),
      ),
    ),
  );
}

class _Footer extends StatelessWidget {
  const _Footer();
  @override
  Widget build(BuildContext context) => const Row(
    children: [
      Icon(Icons.join_inner_rounded, size: 15, color: FrameColors.muted),
      SizedBox(width: 8),
      Expanded(
        child: Text(
          'SAMEFRAME  /  让每一幕，都有共鸣。',
          style: TextStyle(
            fontSize: 10,
            color: FrameColors.muted,
            letterSpacing: 1,
          ),
        ),
      ),
    ],
  );
}
