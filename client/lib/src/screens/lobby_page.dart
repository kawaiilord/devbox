import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'room_page.dart';

class LobbyPage extends StatefulWidget {
  const LobbyPage({super.key, required this.api});

  final ApiClient api;

  @override
  State<LobbyPage> createState() => _LobbyPageState();
}

class _LobbyPageState extends State<LobbyPage> {
  final _name = TextEditingController();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _roomName = TextEditingController(text: '周五放映室');
  final _source = TextEditingController(
    text: 'https://flutter.github.io/assets-for-api-docs/assets/videos/bee.mp4',
  );
  final _roomCode = TextEditingController();
  Session? _session;
  bool _registerMode = true;
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    _password.dispose();
    _roomName.dispose();
    _source.dispose();
    _roomCode.dispose();
    super.dispose();
  }

  Future<void> _signIn() async {
    await _run(() async {
      final session = _registerMode
          ? await widget.api.register(
              email: _email.text,
              displayName: _name.text,
              password: _password.text,
            )
          : await widget.api.login(
              email: _email.text,
              password: _password.text,
            );
      if (mounted) setState(() => _session = session);
    });
  }

  Future<void> _createRoom() async {
    final session = _session;
    if (session == null) return;
    await _run(() async {
      final room = await widget.api.createRoom(
        session: session,
        name: _roomName.text,
        sourceUrl: _source.text,
      );
      _openRoom(session, room);
    });
  }

  Future<void> _joinRoom() async {
    final session = _session;
    if (session == null) return;
    await _run(() async {
      final room = await widget.api.joinRoom(
        session: session,
        code: _roomCode.text,
      );
      _openRoom(session, room);
    });
  }

  Future<void> _logout() async {
    final session = _session;
    if (session == null) return;
    await _run(() async {
      await widget.api.logout(session);
      if (mounted) {
        setState(() {
          _session = null;
          _password.clear();
        });
      }
    });
  }

  Future<void> _run(Future<void> Function() operation) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await operation();
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _openRoom(Session session, Room room) {
    if (!mounted) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) =>
            RoomPage(api: widget.api, session: session, initialRoom: room),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 1040),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _Brand(
                    session: _session,
                    onLogout: _session == null ? null : _logout,
                  ),
                  const SizedBox(height: 40),
                  if (_session == null)
                    _LoginCard(
                      name: _name,
                      email: _email,
                      password: _password,
                      registerMode: _registerMode,
                      busy: _busy,
                      onSubmit: _signIn,
                      onToggleMode: () => setState(() {
                        _registerMode = !_registerMode;
                        _error = null;
                      }),
                    )
                  else
                    LayoutBuilder(
                      builder: (context, constraints) {
                        final cards = [
                          _CreateCard(
                            roomName: _roomName,
                            source: _source,
                            busy: _busy,
                            onSubmit: _createRoom,
                          ),
                          _JoinCard(
                            roomCode: _roomCode,
                            busy: _busy,
                            onSubmit: _joinRoom,
                          ),
                        ];
                        if (constraints.maxWidth < 760) {
                          return Column(
                            children: [
                              cards[0],
                              const SizedBox(height: 16),
                              cards[1],
                            ],
                          );
                        }
                        return Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Expanded(child: cards[0]),
                            const SizedBox(width: 16),
                            Expanded(child: cards[1]),
                          ],
                        );
                      },
                    ),
                  if (_error != null) ...[
                    const SizedBox(height: 18),
                    Text(
                      _error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ],
                  const SizedBox(height: 36),
                  const _ProtocolSummary(),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _Brand extends StatelessWidget {
  const _Brand({required this.session, required this.onLogout});
  final Session? session;
  final VoidCallback? onLogout;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          width: 48,
          height: 48,
          decoration: BoxDecoration(
            color: Theme.of(context).colorScheme.primary,
            borderRadius: BorderRadius.circular(15),
          ),
          child: const Icon(Icons.join_inner_rounded, color: Colors.black),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'SameFrame · 同帧',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              Text(
                session == null
                    ? '让两块屏幕，停在同一秒'
                    : '你好，${session!.user.displayName}',
                style: Theme.of(context).textTheme.bodyMedium
                    ?.copyWith(color: Colors.white60),
              ),
            ],
          ),
        ),
        const _StatusPill(text: 'Clean-room MVP'),
        if (onLogout != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onLogout,
            tooltip: '退出登录',
            icon: const Icon(Icons.logout_rounded),
          ),
        ],
      ],
    );
  }
}

class _LoginCard extends StatelessWidget {
  const _LoginCard({
    required this.name,
    required this.email,
    required this.password,
    required this.registerMode,
    required this.busy,
    required this.onSubmit,
    required this.onToggleMode,
  });
  final TextEditingController name;
  final TextEditingController email;
  final TextEditingController password;
  final bool registerMode;
  final bool busy;
  final VoidCallback onSubmit;
  final VoidCallback onToggleMode;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 520),
      child: _Panel(
        title: registerMode ? '创建账号' : '登录 SameFrame',
        subtitle: registerMode
            ? '密码使用 Argon2id 处理，登录态支持安全轮换。'
            : '使用邮箱和密码继续进入放映室。',
        child: Column(
          children: [
            if (registerMode) ...[
              TextField(
                controller: name,
                autofocus: true,
                decoration: const InputDecoration(labelText: '你的昵称'),
              ),
              const SizedBox(height: 12),
            ],
            TextField(
              controller: email,
              autofocus: !registerMode,
              keyboardType: TextInputType.emailAddress,
              decoration: const InputDecoration(labelText: '邮箱'),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: password,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: '密码',
                helperText: '至少 10 个字符',
              ),
              onSubmitted: (_) => onSubmit(),
            ),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              child: FilledButton.icon(
                onPressed: busy ? null : onSubmit,
                icon: const Icon(Icons.arrow_forward_rounded),
                label: Text(busy ? '连接中…' : (registerMode ? '注册并进入' : '登录')),
              ),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: busy ? null : onToggleMode,
              child: Text(registerMode ? '已有账号？直接登录' : '没有账号？立即注册'),
            ),
          ],
        ),
      ),
    );
  }
}

class _CreateCard extends StatelessWidget {
  const _CreateCard({
    required this.roomName,
    required this.source,
    required this.busy,
    required this.onSubmit,
  });
  final TextEditingController roomName;
  final TextEditingController source;
  final bool busy;
  final VoidCallback onSubmit;

  @override
  Widget build(BuildContext context) {
    return _Panel(
      title: '创建房间',
      subtitle: '房主是唯一共享播放控制源。',
      child: Column(
        children: [
          TextField(
            controller: roomName,
            decoration: const InputDecoration(labelText: '房间名称'),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: source,
            decoration: const InputDecoration(labelText: 'HTTP(S) 直链'),
            minLines: 2,
            maxLines: 3,
          ),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: FilledButton.icon(
              onPressed: busy ? null : onSubmit,
              icon: const Icon(Icons.add_rounded),
              label: const Text('创建并播放'),
            ),
          ),
        ],
      ),
    );
  }
}

class _JoinCard extends StatelessWidget {
  const _JoinCard({
    required this.roomCode,
    required this.busy,
    required this.onSubmit,
  });
  final TextEditingController roomCode;
  final bool busy;
  final VoidCallback onSubmit;

  @override
  Widget build(BuildContext context) {
    return _Panel(
      title: '加入房间',
      subtitle: '输入六位房间码，自动拉取权威状态。',
      child: Column(
        children: [
          TextField(
            controller: roomCode,
            textCapitalization: TextCapitalization.characters,
            decoration: const InputDecoration(
              labelText: '房间码',
              hintText: 'ABC123',
            ),
            onSubmitted: (_) => onSubmit(),
          ),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: OutlinedButton.icon(
              onPressed: busy ? null : onSubmit,
              icon: const Icon(Icons.login_rounded),
              label: const Text('加入房间'),
            ),
          ),
        ],
      ),
    );
  }
}

class _Panel extends StatelessWidget {
  const _Panel({
    required this.title,
    required this.subtitle,
    required this.child,
  });
  final String title;
  final String subtitle;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 6),
            Text(subtitle, style: const TextStyle(color: Colors.white54)),
            const SizedBox(height: 22),
            child,
          ],
        ),
      ),
    );
  }
}

class _ProtocolSummary extends StatelessWidget {
  const _ProtocolSummary();

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 10,
      runSpacing: 10,
      children: const [
        _StatusPill(text: '3s 权威快照'),
        _StatusPill(text: '0.3s 软追帧'),
        _StatusPill(text: '1.5s 硬对齐'),
        _StatusPill(text: '断线全量恢复'),
        _StatusPill(text: '凭据零下发'),
      ],
    );
  }
}

class _StatusPill extends StatelessWidget {
  const _StatusPill({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.06),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: Colors.white12),
      ),
      child: Text(text, style: Theme.of(context).textTheme.labelMedium),
    );
  }
}
