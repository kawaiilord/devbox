import 'package:flutter/material.dart';

import '../appearance.dart';
import '../design.dart';

class MobileProfile extends StatelessWidget {
  const MobileProfile({
    super.key,
    required this.name,
    required this.email,
    required this.signedIn,
    required this.onLogin,
    required this.onLogout,
    required this.items,
    this.verification,
  });
  final String name, email;
  final bool signedIn;
  final VoidCallback onLogin, onLogout;
  final Widget? verification;
  final List<({IconData icon, String label, VoidCallback? onTap})> items;

  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    return Scaffold(
      appBar: AppBar(
        automaticallyImplyLeading: false,
        toolbarHeight: 60,
        title: const Text('我的'),
        actions: const [FrameThemeMenu()],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Row(
                    children: [
                      CircleAvatar(
                        radius: 28,
                        backgroundColor: colors.elevated,
                        foregroundColor: colors.text,
                        child: signedIn
                            ? Text(
                                name.isEmpty ? 'S' : name.characters.first,
                                style: const TextStyle(fontSize: 22),
                              )
                            : const Icon(
                                Icons.person_outline_rounded,
                                size: 28,
                              ),
                      ),
                      const SizedBox(width: 16),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              signedIn ? name : '欢迎来到同帧',
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: const TextStyle(
                                fontSize: 20,
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            const SizedBox(height: 6),
                            Text(
                              signedIn ? email : '登录后管理你的观影空间',
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                color: colors.muted,
                                fontSize: 12,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  if (!signedIn) ...[
                    const SizedBox(height: 20),
                    FilledButton(
                      onPressed: onLogin,
                      child: const Text('登录或注册'),
                    ),
                  ],
                ],
              ),
            ),
          ),
          const SizedBox(height: 18),
          Card(
            clipBehavior: Clip.antiAlias,
            child: Column(
              children: [
                for (var i = 0; i < items.length; i++) ...[
                  if (i > 0) const Divider(height: 1, indent: 54),
                  ListTile(
                    enabled: items[i].onTap != null,
                    minLeadingWidth: 24,
                    leading: Icon(
                      items[i].icon,
                      size: 21,
                      color: colors.silver,
                    ),
                    title: Text(
                      items[i].label,
                      style: const TextStyle(fontSize: 14),
                    ),
                    trailing: Icon(
                      Icons.chevron_right_rounded,
                      size: 18,
                      color: colors.muted,
                    ),
                    onTap: items[i].onTap,
                  ),
                ],
              ],
            ),
          ),
          if (verification != null) ...[
            const SizedBox(height: 16),
            Card(
              child: ExpansionTile(
                shape: const Border(),
                leading: Icon(
                  Icons.mark_email_unread_outlined,
                  size: 20,
                  color: colors.muted,
                ),
                title: const Text('验证邮箱，保护账号', style: TextStyle(fontSize: 13)),
                children: [verification!],
              ),
            ),
          ],
          if (signedIn) ...[
            const SizedBox(height: 20),
            OutlinedButton.icon(
              onPressed: onLogout,
              icon: const Icon(Icons.logout_rounded, size: 18),
              label: const Text('退出登录'),
            ),
          ],
          const SizedBox(height: 16),
          Text(
            'SAMEFRAME  /  共享此刻',
            textAlign: TextAlign.center,
            style: TextStyle(
              color: colors.muted,
              fontSize: 10,
              letterSpacing: 1,
            ),
          ),
        ],
      ),
    );
  }
}

class MobileLibrarySignIn extends StatelessWidget {
  const MobileLibrarySignIn({super.key, required this.onLogin});
  final VoidCallback onLogin;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      automaticallyImplyLeading: false,
      toolbarHeight: 60,
      title: const Text('片库'),
      actions: const [FrameThemeMenu()],
    ),
    body: Center(
      child: Padding(
        padding: const EdgeInsets.all(28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const FrameEmpty(
              icon: Icons.video_library_outlined,
              title: '把喜欢的故事留在这里',
              detail: '登录后查看收藏，接着上次的进度继续看。',
            ),
            FilledButton(onPressed: onLogin, child: const Text('登录查看片库')),
          ],
        ),
      ),
    ),
  );
}
