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
  final _roomName = TextEditingController(text: '周五放映室');
  final _source = TextEditingController(
    text: 'https://flutter.github.io/assets-for-api-docs/assets/videos/bee.mp4',
  );
  final _roomCode = TextEditingController();
  Session? _session;
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    _roomName.dispose();
    _source.dispose();
    _roomCode.dispose();
    super.dispose();
  }

  Future<void> _signIn() async {
    await _run(() async {
      final session = await widget.api.createDemoSession(_name.text);
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
                  _Brand(session: _session),
                  const SizedBox(height: 40),
                  if (_session == null)
                    _LoginCard(
                      controller: _name,
                      busy: _busy,
                      onSubmit: _signIn,
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
  const _Brand({required this.session});
  final Session? session;

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
      ],
    );
  }
}

class _LoginCard extends StatelessWidget {
  const _LoginCard({
    required this.controller,
    required this.busy,
    required this.onSubmit,
  });
  final TextEditingController controller;
  final bool busy;
  final VoidCallback onSubmit;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 520),
      child: _Panel(
        title: '进入测试环境',
        subtitle: '当前里程碑使用临时会话，刷新服务端后数据会清空。',
        child: Column(
          children: [
            TextField(
              controller: controller,
              autofocus: true,
              decoration: const InputDecoration(labelText: '你的昵称'),
              onSubmitted: (_) => onSubmit(),
            ),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              child: FilledButton.icon(
                onPressed: busy ? null : onSubmit,
                icon: const Icon(Icons.arrow_forward_rounded),
                label: Text(busy ? '连接中…' : '进入大厅'),
              ),
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
