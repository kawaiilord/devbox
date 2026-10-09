import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'library_page.dart';
import 'couple_page.dart';
import 'metadata_search_page.dart';
import 'privacy_page.dart';
import 'room_page.dart';
import 'social_page.dart';

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
  final _verificationToken = TextEditingController();
  Session? _session;
  bool _registerMode = true;
  bool _verifiedOverride = false;
  bool _busy = false;
  String? _error;
  String _selectedMediaSourceId = '';
  String _selectedMediaPath = '';

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    _password.dispose();
    _roomName.dispose();
    _source.dispose();
    _roomCode.dispose();
    _verificationToken.dispose();
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
      if (mounted) {
        setState(() {
          _session = session;
          _verifiedOverride = session.user.emailVerified;
        });
      }
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
        mediaSourceId: _selectedMediaSourceId,
        mediaPath: _selectedMediaPath,
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

  Future<void> _requestVerification() async {
    final session = _session;
    if (session == null) return;
    await _run(() => widget.api.requestEmailVerification(session));
  }

  Future<void> _verifyEmail() async {
    await _run(() async {
      await widget.api.verifyEmail(_verificationToken.text);
      if (mounted) setState(() => _verifiedOverride = true);
    });
  }

  Future<void> _showDevices() async {
    final session = _session;
    if (session == null) return;
    await _run(() async {
      final devices = await widget.api.listDevices(session);
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (dialogContext) => AlertDialog(
          title: const Text('登录设备'),
          content: SizedBox(
            width: 480,
            child: devices.isEmpty
                ? const Text('没有可管理的设备。')
                : Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      for (final device in devices)
                        ListTile(
                          leading: Icon(
                            device.current
                                ? Icons.devices_rounded
                                : Icons.computer_rounded,
                          ),
                          title: Text(device.label),
                          subtitle: Text(
                            '${device.platform} · ${DateTime.fromMillisecondsSinceEpoch(device.lastSeen).toLocal()}',
                          ),
                          trailing: device.current
                              ? const Chip(label: Text('当前'))
                              : IconButton(
                                  tooltip: '撤销设备',
                                  icon: const Icon(Icons.link_off_rounded),
                                  onPressed: () async {
                                    await widget.api.revokeDevice(
                                      session,
                                      device.id,
                                    );
                                    if (dialogContext.mounted) {
                                      Navigator.of(dialogContext).pop();
                                    }
                                  },
                                ),
                        ),
                    ],
                  ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('关闭'),
            ),
          ],
        ),
      );
    });
  }

  void _showPrivacy() {
    final session = _session;
    if (session == null) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PrivacyPage(api: widget.api, session: session),
      ),
    );
  }

  void _showLibrary() {
    final session = _session;
    if (session == null) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => LibraryPage(api: widget.api, session: session),
      ),
    );
  }

  void _showMetadataSearch() {
    final session = _session;
    if (session == null) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => MetadataSearchPage(api: widget.api, session: session),
      ),
    );
  }

  void _showSocial() {
    final session = _session;
    if (session == null) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => SocialPage(api: widget.api, session: session),
      ),
    );
  }

  void _showCouple() {
    final session = _session;
    if (session == null) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => CouplePage(api: widget.api, session: session),
      ),
    );
  }

  Future<void> _showMediaSources() async {
    final session = _session;
    if (session == null) return;
    var sources = await widget.api.listMediaSources(session);
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: const Text('媒体源'),
          content: SizedBox(
            width: 560,
            child: sources.isEmpty
                ? const Text('还没有媒体源。凭据只会加密保存在服务端。')
                : Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      for (final source in sources)
                        ListTile(
                          leading: Icon(
                            source.type == 'emby'
                                ? Icons.dns_rounded
                                : Icons.cloud_rounded,
                          ),
                          title: Text(source.name),
                          subtitle: Text(
                            '${source.type == 'emby' ? 'Emby' : 'WebDAV'} · ${source.baseUrl}',
                          ),
                          onTap: () => _browseMediaSource(source),
                          trailing: IconButton(
                            tooltip: '删除媒体源',
                            icon: const Icon(Icons.delete_outline_rounded),
                            onPressed: () async {
                              await widget.api.deleteMediaSource(
                                session,
                                source.id,
                              );
                              sources = await widget.api.listMediaSources(
                                session,
                              );
                              setDialogState(() {});
                            },
                          ),
                        ),
                    ],
                  ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('关闭'),
            ),
            OutlinedButton.icon(
              onPressed: () async {
                final created = await _promptAddEmby();
                if (created && dialogContext.mounted) {
                  sources = await widget.api.listMediaSources(session);
                  setDialogState(() {});
                }
              },
              icon: const Icon(Icons.dns_rounded),
              label: const Text('添加 Emby'),
            ),
            FilledButton.icon(
              onPressed: () async {
                final created = await _promptAddWebDAV();
                if (created && dialogContext.mounted) {
                  sources = await widget.api.listMediaSources(session);
                  setDialogState(() {});
                }
              },
              icon: const Icon(Icons.add_rounded),
              label: const Text('添加 WebDAV'),
            ),
          ],
        ),
      ),
    );
  }

  Future<bool> _promptAddWebDAV() async {
    final session = _session;
    if (session == null) return false;
    final name = TextEditingController();
    final baseUrl = TextEditingController();
    final username = TextEditingController();
    final password = TextEditingController();
    var created = false;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('添加 WebDAV'),
        content: SizedBox(
          width: 460,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: name,
                decoration: const InputDecoration(labelText: '名称'),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: baseUrl,
                decoration: const InputDecoration(labelText: 'HTTPS WebDAV 地址'),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: username,
                decoration: const InputDecoration(labelText: '用户名'),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: password,
                obscureText: true,
                decoration: const InputDecoration(labelText: '密码'),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () async {
              await widget.api.createWebDAVSource(
                session: session,
                name: name.text,
                baseUrl: baseUrl.text,
                username: username.text,
                password: password.text,
              );
              created = true;
              if (dialogContext.mounted) Navigator.of(dialogContext).pop();
            },
            child: const Text('保存'),
          ),
        ],
      ),
    );
    name.dispose();
    baseUrl.dispose();
    username.dispose();
    password.dispose();
    return created;
  }

  Future<bool> _promptAddEmby() async {
    final session = _session;
    if (session == null) return false;
    final name = TextEditingController();
    final baseUrl = TextEditingController();
    final username = TextEditingController();
    final password = TextEditingController();
    var created = false;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('添加 Emby'),
        content: SizedBox(
          width: 460,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: name,
                decoration: const InputDecoration(labelText: '名称'),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: baseUrl,
                decoration: const InputDecoration(
                  labelText: 'HTTPS Emby API 地址',
                  hintText: 'https://emby.example.com/emby/',
                ),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: username,
                decoration: const InputDecoration(labelText: 'Emby 用户名'),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: password,
                obscureText: true,
                decoration: const InputDecoration(labelText: 'Emby 密码'),
              ),
              const SizedBox(height: 12),
              const Text(
                '密码仅用于本次登录；服务端只加密保存 Emby 会话令牌。',
                style: TextStyle(color: Colors.white60),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () async {
              await widget.api.createEmbySource(
                session: session,
                name: name.text,
                baseUrl: baseUrl.text,
                username: username.text,
                password: password.text,
              );
              created = true;
              if (dialogContext.mounted) Navigator.of(dialogContext).pop();
            },
            child: const Text('登录并保存'),
          ),
        ],
      ),
    );
    name.dispose();
    baseUrl.dispose();
    username.dispose();
    password.dispose();
    return created;
  }

  Future<void> _browseMediaSource(MediaSource source) async {
    final session = _session;
    if (session == null) return;
    var currentPath = '/';
    var files = await widget.api.browseMediaSource(
      session,
      source.id,
      currentPath,
    );
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: Text('${source.name} · $currentPath'),
          content: SizedBox(
            width: 560,
            height: 420,
            child: ListView(
              children: [
                if (currentPath != '/')
                  ListTile(
                    leading: const Icon(Icons.arrow_upward_rounded),
                    title: const Text('返回上级'),
                    onTap: () async {
                      final segments = currentPath.split('/')
                        ..removeWhere((value) => value.isEmpty);
                      if (segments.isNotEmpty) segments.removeLast();
                      currentPath = '/${segments.join('/')}';
                      if (currentPath != '/') currentPath += '/';
                      files = await widget.api.browseMediaSource(
                        session,
                        source.id,
                        currentPath,
                      );
                      setDialogState(() {});
                    },
                  ),
                for (final file in files)
                  ListTile(
                    leading: Icon(
                      file.isDirectory
                          ? Icons.folder_rounded
                          : Icons.movie_outlined,
                    ),
                    title: Text(file.name),
                    subtitle: file.isDirectory
                        ? null
                        : Text(_formatBytes(file.size)),
                    trailing: file.isDirectory
                        ? null
                        : IconButton(
                            tooltip: '收藏',
                            icon: const Icon(Icons.star_border_rounded),
                            onPressed: () async {
                              try {
                                await widget.api.addFavorite(
                                  session: session,
                                  sourceId: source.id,
                                  file: file,
                                );
                                if (mounted) {
                                  ScaffoldMessenger.of(
                                    this.context,
                                  ).showSnackBar(
                                    SnackBar(content: Text('已收藏 ${file.name}')),
                                  );
                                }
                              } catch (exception) {
                                if (mounted) {
                                  ScaffoldMessenger.of(
                                    this.context,
                                  ).showSnackBar(
                                    SnackBar(content: Text('收藏失败：$exception')),
                                  );
                                }
                              }
                            },
                          ),
                    onTap: () async {
                      if (file.isDirectory) {
                        currentPath = file.path.endsWith('/')
                            ? file.path
                            : '${file.path}/';
                        files = await widget.api.browseMediaSource(
                          session,
                          source.id,
                          currentPath,
                        );
                        setDialogState(() {});
                        return;
                      }
                      _selectedMediaSourceId = source.id;
                      _selectedMediaPath = file.path;
                      final sourceType = source.type == 'emby'
                          ? 'Emby'
                          : 'WebDAV';
                      _source.text =
                          '$sourceType · ${source.name} · ${file.name}';
                      if (dialogContext.mounted) {
                        Navigator.of(dialogContext).pop();
                      }
                      if (mounted) {
                        ScaffoldMessenger.of(this.context).showSnackBar(
                          SnackBar(content: Text('已选择 ${file.name}，现在可以创建房间。')),
                        );
                      }
                    },
                  ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('关闭'),
            ),
          ],
        ),
      ),
    );
  }

  String _formatBytes(int bytes) {
    if (bytes >= 1024 * 1024 * 1024) {
      return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(1)} GB';
    }
    if (bytes >= 1024 * 1024) {
      return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    if (bytes >= 1024) {
      return '${(bytes / 1024).toStringAsFixed(1)} KB';
    }
    return '$bytes B';
  }

  Future<void> _showPasswordReset() async {
    final token = TextEditingController();
    final password = TextEditingController();
    var requested = false;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: const Text('找回密码'),
          content: SizedBox(
            width: 430,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: _email,
                  decoration: const InputDecoration(labelText: '账号邮箱'),
                ),
                if (requested) ...[
                  const SizedBox(height: 12),
                  TextField(
                    controller: token,
                    decoration: const InputDecoration(labelText: '邮件中的重置令牌'),
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: password,
                    obscureText: true,
                    decoration: const InputDecoration(labelText: '新密码'),
                  ),
                ],
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () async {
                if (!requested) {
                  await widget.api.requestPasswordReset(_email.text);
                  setDialogState(() => requested = true);
                  return;
                }
                await widget.api.resetPassword(
                  token: token.text,
                  password: password.text,
                );
                if (dialogContext.mounted) {
                  Navigator.of(dialogContext).pop();
                }
              },
              child: Text(requested ? '更新密码' : '发送重置邮件'),
            ),
          ],
        ),
      ),
    );
    token.dispose();
    password.dispose();
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
                    onDevices: _session == null ? null : _showDevices,
                    onSources: _session == null ? null : _showMediaSources,
                    onPrivacy: _session == null ? null : _showPrivacy,
                    onLibrary: _session == null ? null : _showLibrary,
                    onMetadata: _session == null ? null : _showMetadataSearch,
                    onSocial: _session == null ? null : _showSocial,
                    onCouple: _session == null ? null : _showCouple,
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
                      onForgotPassword: _showPasswordReset,
                      onToggleMode: () => setState(() {
                        _registerMode = !_registerMode;
                        _error = null;
                      }),
                    )
                  else
                    Column(
                      children: [
                        if (!_verifiedOverride) ...[
                          _VerificationCard(
                            token: _verificationToken,
                            busy: _busy,
                            onRequest: _requestVerification,
                            onVerify: _verifyEmail,
                          ),
                          const SizedBox(height: 16),
                        ],
                        LayoutBuilder(
                          builder: (context, constraints) {
                            final cards = [
                              _CreateCard(
                                roomName: _roomName,
                                source: _source,
                                busy: _busy,
                                onSubmit: _createRoom,
                                onSourceChanged: () {
                                  _selectedMediaSourceId = '';
                                  _selectedMediaPath = '';
                                },
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
                      ],
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
  const _Brand({
    required this.session,
    required this.onLogout,
    required this.onDevices,
    required this.onSources,
    required this.onPrivacy,
    required this.onLibrary,
    required this.onMetadata,
    required this.onSocial,
    required this.onCouple,
  });
  final Session? session;
  final VoidCallback? onLogout;
  final VoidCallback? onDevices;
  final VoidCallback? onSources;
  final VoidCallback? onPrivacy;
  final VoidCallback? onLibrary;
  final VoidCallback? onMetadata;
  final VoidCallback? onSocial;
  final VoidCallback? onCouple;

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
        if (onCouple != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onCouple,
            tooltip: '情侣空间',
            icon: const Icon(Icons.favorite_border_rounded),
          ),
        ],
        if (onSocial != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onSocial,
            tooltip: '社交与私聊',
            icon: const Icon(Icons.people_outline_rounded),
          ),
        ],
        if (onMetadata != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onMetadata,
            tooltip: '影视搜索',
            icon: const Icon(Icons.manage_search_rounded),
          ),
        ],
        if (onLibrary != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onLibrary,
            tooltip: '收藏与历史',
            icon: const Icon(Icons.video_library_outlined),
          ),
        ],
        if (onPrivacy != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onPrivacy,
            tooltip: '隐私与屏蔽',
            icon: const Icon(Icons.privacy_tip_outlined),
          ),
        ],
        if (onSources != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onSources,
            tooltip: '媒体源',
            icon: const Icon(Icons.video_library_rounded),
          ),
        ],
        if (onDevices != null) ...[
          const SizedBox(width: 8),
          IconButton(
            onPressed: onDevices,
            tooltip: '设备管理',
            icon: const Icon(Icons.devices_rounded),
          ),
        ],
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
    required this.onForgotPassword,
  });
  final TextEditingController name;
  final TextEditingController email;
  final TextEditingController password;
  final bool registerMode;
  final bool busy;
  final VoidCallback onSubmit;
  final VoidCallback onToggleMode;
  final VoidCallback onForgotPassword;

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
            if (!registerMode)
              TextButton(
                onPressed: busy ? null : onForgotPassword,
                child: const Text('忘记密码'),
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
    required this.onSourceChanged,
  });
  final TextEditingController roomName;
  final TextEditingController source;
  final bool busy;
  final VoidCallback onSubmit;
  final VoidCallback onSourceChanged;

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
            onChanged: (_) => onSourceChanged(),
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

class _VerificationCard extends StatelessWidget {
  const _VerificationCard({
    required this.token,
    required this.busy,
    required this.onRequest,
    required this.onVerify,
  });

  final TextEditingController token;
  final bool busy;
  final VoidCallback onRequest;
  final VoidCallback onVerify;

  @override
  Widget build(BuildContext context) {
    return _Panel(
      title: '验证邮箱',
      subtitle: '验证令牌 24 小时有效且只能使用一次。',
      child: Row(
        children: [
          Expanded(
            child: TextField(
              controller: token,
              decoration: const InputDecoration(labelText: '邮件中的验证令牌'),
            ),
          ),
          const SizedBox(width: 10),
          OutlinedButton(
            onPressed: busy ? null : onRequest,
            child: const Text('重新发送'),
          ),
          const SizedBox(width: 8),
          FilledButton(
            onPressed: busy ? null : onVerify,
            child: const Text('完成验证'),
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
