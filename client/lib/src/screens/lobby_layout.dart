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
  final String? error;

  Widget _error(BuildContext context) => error == null
      ? const SizedBox.shrink()
      : Padding(
          padding: const EdgeInsets.only(top: 16),
          child: Text(
            error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        );

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final desktop = constraints.maxWidth >= 1100;
      if (!signedIn) {
        final wide = constraints.maxWidth >= 940;
        return Scaffold(
          body: SafeArea(
            child: SingleChildScrollView(
              padding: EdgeInsets.symmetric(
                horizontal: wide ? 48 : 20,
                vertical: 28,
              ),
              child: Center(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 1180),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(
                            child: FrameBrand(
                              compact: constraints.maxWidth < 420,
                            ),
                          ),
                          TextButton.icon(
                            onPressed: onUpdates,
                            icon: const Icon(
                              Icons.system_update_alt_rounded,
                              size: 16,
                            ),
                            label: const Text('检查更新'),
                          ),
                        ],
                      ),
                      SizedBox(height: wide ? 72 : 32),
                      if (wide)
                        Row(
                          crossAxisAlignment: CrossAxisAlignment.center,
                          children: [
                            const Expanded(flex: 6, child: _Welcome()),
                            const SizedBox(width: 72),
                            Expanded(
                              flex: 4,
                              child: Column(children: [login, _error(context)]),
                            ),
                          ],
                        )
                      else ...[
                        const _Welcome(compact: true),
                        const SizedBox(height: 28),
                        login,
                        _error(context),
                      ],
                      const SizedBox(height: 48),
                      const Divider(),
                      const Wrap(
                        spacing: 32,
                        runSpacing: 12,
                        children: [
                          _Benefit(Icons.sync_rounded, '同步每一幕'),
                          _Benefit(Icons.chat_bubble_outline_rounded, '分享每份感受'),
                          _Benefit(Icons.devices_rounded, '随时，多端相见'),
                        ],
                      ),
                      const SizedBox(height: 24),
                      const Text(
                        'SAMEFRAME  /  A LITTLE CLOSER, FRAME BY FRAME.',
                        style: TextStyle(
                          fontSize: 10,
                          letterSpacing: 1.6,
                          color: FrameColors.muted,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        );
      }
      return Scaffold(
        appBar: desktop
            ? null
            : AppBar(
                title: const FrameBrand(compact: true),
                actions: [
                  IconButton(
                    onPressed: onSearch,
                    tooltip: '影视搜索',
                    icon: const Icon(Icons.search_rounded),
                  ),
                ],
              ),
        drawer: desktop
            ? null
            : Drawer(
                backgroundColor: FrameColors.background,
                child: SafeArea(child: _navigation(context, closeDrawer: true)),
              ),
        body: SafeArea(
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (desktop) SizedBox(width: 228, child: _navigation(context)),
              Expanded(
                child: SingleChildScrollView(
                  padding: EdgeInsets.all(desktop ? 36 : 20),
                  child: Center(
                    child: ConstrainedBox(
                      constraints: const BoxConstraints(maxWidth: 1240),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          LayoutBuilder(
                            builder: (context, box) => Row(
                              children: [
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      const Text(
                                        'YOUR SHARED CINEMA',
                                        style: TextStyle(
                                          fontSize: 10,
                                          letterSpacing: 2.2,
                                          color: FrameColors.muted,
                                        ),
                                      ),
                                      const SizedBox(height: 6),
                                      Text(
                                        '今晚，一起看点什么？',
                                        style: Theme.of(context)
                                            .textTheme
                                            .headlineSmall,
                                      ),
                                    ],
                                  ),
                                ),
                                if (box.maxWidth > 520)
                                  OutlinedButton.icon(
                                    onPressed: onSearch,
                                    icon: const Icon(Icons.search, size: 18),
                                    label: const Text('搜索电影 / 剧集'),
                                  ),
                              ],
                            ),
                          ),
                          const SizedBox(height: 24),
                          const _CinemaHero(),
                          const SizedBox(height: 28),
                          Text(
                            '开启一场共同放映',
                            style: Theme.of(context).textTheme.titleLarge,
                          ),
                          const SizedBox(height: 16),
                          LayoutBuilder(
                            builder: (context, box) => box.maxWidth < 700
                                ? Column(
                                    children: [
                                      create,
                                      const SizedBox(height: 16),
                                      join,
                                    ],
                                  )
                                : Row(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
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
                                _Entry(destinations[0], '收藏喜欢的故事，接着上次看'),
                                _Entry(destinations[1], '聊聊电影，也聊聊今天'),
                                _Entry(destinations[2], '把一起看的时光，留在这里'),
                              ];
                              return box.maxWidth < 700
                                  ? Column(
                                      children: [
                                        for (final item in items)
                                          Padding(
                                            padding: const EdgeInsets.only(
                                              bottom: 12,
                                            ),
                                            child: item,
                                          ),
                                      ],
                                    )
                                  : Row(
                                      children: [
                                        for (
                                          var i = 0;
                                          i < items.length;
                                          i++
                                        ) ...[
                                          if (i > 0) const SizedBox(width: 14),
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
                                  color: FrameColors.gold,
                                  size: 20,
                                ),
                                title: const Text(
                                  '验证邮箱，保护你的账号',
                                  style: TextStyle(fontSize: 14),
                                ),
                                subtitle: const Text(
                                  '展开完成邮箱验证',
                                  style: TextStyle(
                                    fontSize: 12,
                                    color: FrameColors.muted,
                                  ),
                                ),
                                children: [verification!],
                              ),
                            ),
                          ],
                          const SizedBox(height: 28),
                          const Text(
                            'SAMEFRAME  /  好故事，值得一起看。',
                            style: TextStyle(
                              fontSize: 11,
                              color: FrameColors.muted,
                              letterSpacing: 1,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      );
    },
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
      color: Color(0xFF101720),
      border: Border(right: BorderSide(color: FrameColors.border)),
    ),
    child: Column(
      children: [
        const Padding(
          padding: EdgeInsets.fromLTRB(22, 30, 22, 30),
          child: FrameBrand(compact: true),
        ),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.symmetric(horizontal: 14),
            children: [
              const Padding(
                padding: EdgeInsets.fromLTRB(16, 0, 0, 12),
                child: Text(
                  '我的空间',
                  style: TextStyle(
                    fontSize: 10,
                    color: FrameColors.muted,
                    letterSpacing: 1,
                  ),
                ),
              ),
              Container(
                decoration: BoxDecoration(
                  color: FrameColors.mint.withValues(alpha: .09),
                  borderRadius: BorderRadius.circular(10),
                ),
                child: const ListTile(
                  dense: true,
                  leading: Icon(
                    Icons.grid_view_rounded,
                    size: 19,
                    color: FrameColors.mint,
                  ),
                  title: Text(
                    '观影大厅',
                    style: TextStyle(fontSize: 13, color: FrameColors.mint),
                  ),
                ),
              ),
              const SizedBox(height: 5),
              for (var i = 0; i < destinations.length; i++) ...[
                if (i == 3) ...[
                  const Divider(),
                  const Padding(
                    padding: EdgeInsets.fromLTRB(16, 0, 0, 12),
                    child: Text(
                      '管理与设置',
                      style: TextStyle(fontSize: 10, color: FrameColors.muted),
                    ),
                  ),
                ],
                ListTile(
                  dense: true,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(10),
                  ),
                  leading: Icon(
                    destinations[i].icon,
                    size: 19,
                    color: FrameColors.muted,
                  ),
                  title: Text(
                    destinations[i].label,
                    style: const TextStyle(fontSize: 13),
                  ),
                  onTap: () =>
                      _navigate(context, destinations[i].onTap, closeDrawer),
                ),
              ],
              ListTile(
                dense: true,
                leading: const Icon(
                  Icons.system_update_alt,
                  size: 19,
                  color: FrameColors.muted,
                ),
                title: const Text('安全更新', style: TextStyle(fontSize: 13)),
                onTap: () => _navigate(context, onUpdates, closeDrawer),
              ),
              const SizedBox(height: 24),
              Card(
                color: const Color(0xFF25271F),
                child: InkWell(
                  borderRadius: BorderRadius.circular(20),
                  onTap: () => _navigate(context, onMembership, closeDrawer),
                  child: const Padding(
                    padding: EdgeInsets.all(18),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        FramePill('SAMEFRAME PLUS', color: FrameColors.gold),
                        SizedBox(height: 16),
                        Text(
                          '让相聚，更长久',
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        SizedBox(height: 6),
                        Text(
                          '查看会员与积分权益',
                          style: TextStyle(
                            fontSize: 11,
                            color: FrameColors.muted,
                          ),
                        ),
                        SizedBox(height: 16),
                        Row(
                          children: [
                            Text(
                              '会员中心',
                              style: TextStyle(
                                color: FrameColors.gold,
                                fontSize: 12,
                              ),
                            ),
                            Spacer(),
                            Icon(
                              Icons.arrow_forward_rounded,
                              size: 16,
                              color: FrameColors.gold,
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.all(18),
          child: Row(
            children: [
              CircleAvatar(
                radius: 17,
                backgroundColor: FrameColors.elevated,
                child: Text(
                  name.isEmpty ? 'S' : name.characters.first,
                  style: const TextStyle(fontSize: 12, color: FrameColors.mint),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 12),
                ),
              ),
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

class _Welcome extends StatelessWidget {
  const _Welcome({this.compact = false});
  final bool compact;
  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      const FramePill('一起看，让距离更近', icon: Icons.play_circle_outline_rounded),
      const SizedBox(height: 24),
      Text(
        '相隔千里，\n也在同一幕。',
        style: TextStyle(
          fontSize: compact ? 36 : 54,
          fontWeight: FontWeight.w700,
          height: 1.22,
          letterSpacing: -2,
        ),
      ),
      const SizedBox(height: 20),
      const Text(
        '一部好电影，一个熟悉的人。\n创建专属放映室，让每次相聚都有好故事。',
        style: TextStyle(fontSize: 15, color: FrameColors.muted, height: 1.9),
      ),
      if (!compact) ...[
        const SizedBox(height: 32),
        ClipRRect(
          borderRadius: BorderRadius.circular(20),
          child: SizedBox(
            height: 210,
            width: double.infinity,
            child: Stack(
              fit: StackFit.expand,
              children: [
                const CinemaArtwork(),
                Container(
                  decoration: const BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topCenter,
                      end: Alignment.bottomCenter,
                      colors: [Colors.transparent, Color(0xA6081522)],
                    ),
                  ),
                ),
                const Positioned(
                  left: 22,
                  top: 18,
                  child: Text(
                    'THE EVENING EDITION',
                    style: TextStyle(
                      fontSize: 9,
                      letterSpacing: 2.3,
                      color: Colors.white70,
                    ),
                  ),
                ),
                const Positioned(
                  left: 22,
                  bottom: 20,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '把今晚，留给彼此。',
                        style: TextStyle(
                          fontSize: 22,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      SizedBox(height: 6),
                      Text(
                        'YOUR PRIVATE CINEMA',
                        style: TextStyle(
                          fontSize: 9,
                          letterSpacing: 2,
                          color: Colors.white60,
                        ),
                      ),
                    ],
                  ),
                ),
                const Positioned(
                  right: 22,
                  bottom: 24,
                  child: Icon(
                    Icons.slow_motion_video_rounded,
                    size: 36,
                    color: Colors.white70,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    ],
  );
}

class _CinemaHero extends StatelessWidget {
  const _CinemaHero();
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      final compact = box.maxWidth < 650;
      return ClipRRect(
        borderRadius: BorderRadius.circular(24),
        child: SizedBox(
          height: compact ? 230 : 258,
          child: Stack(
            fit: StackFit.expand,
            children: [
              const CinemaArtwork(),
              Container(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    colors: [
                      const Color(0xFF152A32),
                      const Color(0xFF152A32)
                          .withValues(alpha: compact ? .65 : .35),
                      Colors.transparent,
                    ],
                    stops: const [0, .45, 1],
                  ),
                ),
              ),
              Padding(
                padding: EdgeInsets.all(compact ? 24 : 32),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const FramePill(
                      'SAMEFRAME CINEMA',
                      icon: Icons.movie_outlined,
                    ),
                    const Spacer(),
                    Text(
                      '好故事，\n和重要的人一起看。',
                      style: TextStyle(
                        fontSize: compact ? 28 : 34,
                        fontWeight: FontWeight.w700,
                        height: 1.35,
                        letterSpacing: -1,
                      ),
                    ),
                    const SizedBox(height: 12),
                    const Text(
                      '同步播放 · 房间聊天 · 共享此刻',
                      style: TextStyle(
                        fontSize: 12,
                        color: Colors.white70,
                        letterSpacing: .6,
                      ),
                    ),
                  ],
                ),
              ),
              if (!compact)
                const Positioned(
                  right: 28,
                  bottom: 28,
                  child: Text(
                    '01  /  TOGETHER TONIGHT',
                    style: TextStyle(
                      fontSize: 10,
                      color: Colors.white60,
                      letterSpacing: 2,
                    ),
                  ),
                ),
            ],
          ),
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
      Icon(icon, size: 17, color: FrameColors.mint),
      const SizedBox(width: 10),
      Text(
        text,
        style: const TextStyle(fontSize: 13, color: FrameColors.muted),
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
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(destination.icon, color: FrameColors.mint),
                const Spacer(),
                const Icon(
                  Icons.north_east_rounded,
                  color: FrameColors.muted,
                  size: 17,
                ),
              ],
            ),
            const SizedBox(height: 20),
            Text(
              destination.label,
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 4),
            Text(detail, style: Theme.of(context).textTheme.bodySmall),
          ],
        ),
      ),
    ),
  );
}
