import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../design.dart';
import 'lobby_layout.dart';
import 'media_source_browser.dart';
import 'quark_source_dialog.dart';
import '../secure_session_store.dart';
import 'library_page.dart';
import 'couple_page.dart';
import 'metadata_search_page.dart';
import 'membership_page.dart';
import 'privacy_page.dart';
import 'room_page.dart';
import 'social_page.dart';
import 'update_page.dart';

class LobbyPage extends StatefulWidget {
  const LobbyPage({super.key, required this.api, required this.sessionStore});

  final ApiClient api;
  final SessionStore sessionStore;

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
  void initState() {
    super.initState();
    _restoreSession();
  }

  Future<void> _restoreSession() async {
    final session = await widget.sessionStore.read();
    if (session == null || !mounted) return;
    try {
      await widget.api.listDevices(session);
      if (!mounted) return;
      setState(() {
        _session = session;
        _verifiedOverride = session.user.emailVerified;
      });
      await _showStartupAnnouncement();
    } catch (_) {
      await widget.sessionStore.clear();
    }
  }

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
        await widget.sessionStore.write(session);
        await _showStartupAnnouncement();
      }
    });
  }

  Future<void> _showStartupAnnouncement() async {
    final items = await widget.api.announcements();
    final startup = items.where((item) => item.kind == 'startup').firstOrNull;
    if (startup == null || !mounted) return;
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(startup.title),
        content: Text(startup.body),
        actions: [
          FilledButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('知道了'),
          ),
        ],
      ),
    );
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
      await widget.sessionStore.clear();
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

  void _showMembership() {
    final session = _session;
    if (session == null) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => MembershipPage(api: widget.api, session: session),
      ),
    );
  }

  void _showUpdates() {
    Navigator.of(context)
        .push(MaterialPageRoute<void>(builder: (_) => const UpdatePage()));
  }

  Future<void> _showMediaSources() async {
    final session = _session;
    if (session == null) return;
    var sources = <MediaSource>[];
    String? loadError;
    try {
      sources = await widget.api.listMediaSources(session);
    } catch (_) {
      loadError = '媒体源加载失败，请重试。';
    }
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) {
          Future<void> reload() async {
            try {
              final next = await widget.api.listMediaSources(session);
              if (!dialogContext.mounted) return;
              setDialogState(() {
                sources = next;
                loadError = null;
              });
            } catch (_) {
              if (dialogContext.mounted)
                setDialogState(() => loadError = '媒体源加载失败，请重试。');
            }
          }

          return AlertDialog(
            title: const Text('我的媒体源'),
            content: SizedBox(
              width: 560,
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxHeight: 400),
                child: SingleChildScrollView(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (loadError != null) ...[
                        Text(loadError!),
                        TextButton(onPressed: reload, child: const Text('重试')),
                      ],
                      if (sources.isEmpty && loadError == null)
                        const Padding(
                          padding: EdgeInsets.symmetric(vertical: 24),
                          child: Text('连接你的网盘或媒体库，选一部影片，和好友一起看。'),
                        ),
                      for (final source in sources)
                        ListTile(
                          leading: Icon(
                            source.type == 'emby'
                                ? Icons.dns_rounded
                                : Icons.cloud_rounded,
                          ),
                          title: Text(source.name),
                          subtitle: Text(mediaSourceLabel(source.type)),
                          onTap: () async {
                            final selected = await _browseMediaSource(source);
                            if (selected && dialogContext.mounted)
                              Navigator.of(dialogContext).pop();
                          },
                          trailing: PopupMenuButton<String>(
                            tooltip: '媒体源操作',
                            itemBuilder: (_) => [
                              if (source.type == 'quark')
                                const PopupMenuItem(
                                  value: 'login',
                                  child: Text('更新夸克登录'),
                                ),
                              const PopupMenuItem(
                                value: 'delete',
                                child: Text('移除媒体源'),
                              ),
                            ],
                            onSelected: (action) async {
                              if (action == 'login') {
                                await _promptAddQuark(source: source);
                              } else {
                                try {
                                  await widget.api.deleteMediaSource(
                                    session,
                                    source.id,
                                  );
                                  if (mounted &&
                                      _selectedMediaSourceId == source.id) {
                                    setState(() {
                                      _selectedMediaSourceId = '';
                                      _selectedMediaPath = '';
                                      _source.clear();
                                    });
                                  }
                                } catch (_) {
                                  if (dialogContext.mounted)
                                    setDialogState(
                                      () => loadError = '移除失败，请重试。',
                                    );
                                  return;
                                }
                              }
                              if (dialogContext.mounted) await reload();
                            },
                          ),
                        ),
                    ],
                  ),
                ),
              ),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(dialogContext).pop(),
                child: const Text('关闭'),
              ),
              OutlinedButton(
                onPressed: () async {
                  if (await _promptAddEmby() && dialogContext.mounted)
                    await reload();
                },
                child: const Text('添加 Emby'),
              ),
              OutlinedButton(
                onPressed: () async {
                  if (await _promptAddWebDAV() && dialogContext.mounted)
                    await reload();
                },
                child: const Text('添加 WebDAV'),
              ),
              FilledButton.icon(
                onPressed: () async {
                  if (await _promptAddQuark() && dialogContext.mounted)
                    await reload();
                },
                icon: const Icon(Icons.cloud_outlined),
                label: const Text('添加夸克'),
              ),
            ],
          );
        },
      ),
    );
  }

  Future<bool> _promptAddQuark({MediaSource? source}) async {
    final session = _session;
    if (session == null) return false;
    final result = await showDialog<MediaSource>(
      context: context,
      barrierDismissible: false,
      builder: (_) =>
          QuarkSourceDialog(api: widget.api, session: session, source: source),
    );
    return result != null;
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

  Future<bool> _browseMediaSource(MediaSource source) async {
    final session = _session;
    if (session == null) return false;
    final file = await showDialog<MediaFile>(
      context: context,
      builder: (_) =>
          MediaSourceBrowser(api: widget.api, session: session, source: source),
    );
    if (file == null || !mounted) return false;
    setState(() {
      _selectedMediaSourceId = source.id;
      _selectedMediaPath = file.path;
      _source.text =
          '${mediaSourceLabel(source.type)} · ${source.name} · ${file.name}';
      _roomName.text = file.name.trim().runes.length < 2
          ? '网盘放映室'
          : String.fromCharCodes(file.name.trim().runes.take(64));
    });
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text('已选择 ${file.name}，点击“创建并播放”即可邀请好友。')),
    );
    return true;
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
  Widget build(BuildContext context) => LobbyLayout(
    signedIn: _session != null,
    name: _session?.user.displayName ?? '',
    error: _error,
    onSearch: _showMetadataSearch,
    onMembership: _showMembership,
    onUpdates: _showUpdates,
    onLogout: _logout,
    destinations: [
      LobbyDestination(Icons.video_library_outlined, '收藏与历史', _showLibrary),
      LobbyDestination(Icons.people_outline_rounded, '好友与私聊', _showSocial),
      LobbyDestination(Icons.favorite_border_rounded, '情侣空间', _showCouple),
      LobbyDestination(Icons.folder_open_rounded, '我的媒体源', _showMediaSources),
      LobbyDestination(Icons.devices_rounded, '设备管理', _showDevices),
      LobbyDestination(Icons.shield_outlined, '隐私与屏蔽', _showPrivacy),
    ],
    login: _LoginCard(
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
    ),
    create: _CreateCard(
      roomName: _roomName,
      source: _source,
      busy: _busy,
      onSubmit: _createRoom,
      hasMediaSource: _selectedMediaSourceId.isNotEmpty,
      onPickSource: _showMediaSources,
      onSourceChanged: () => setState(() {
        _selectedMediaSourceId = '';
        _selectedMediaPath = '';
        _source.clear();
      }),
    ),
    join: _JoinCard(roomCode: _roomCode, busy: _busy, onSubmit: _joinRoom),
    verification: _verifiedOverride
        ? null
        : _VerificationCard(
            token: _verificationToken,
            busy: _busy,
            onRequest: _requestVerification,
            onVerify: _verifyEmail,
          ),
  );
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
        subtitle: registerMode ? '从这里开始，收藏属于你们的观影时光。' : '使用邮箱和密码继续进入放映室。',
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
    required this.hasMediaSource,
    required this.onPickSource,
  });
  final TextEditingController roomName;
  final TextEditingController source;
  final bool busy;
  final VoidCallback onSubmit;
  final VoidCallback onSourceChanged;
  final bool hasMediaSource;
  final VoidCallback onPickSource;

  @override
  Widget build(BuildContext context) {
    return _Panel(
      title: '创建房间',
      subtitle: '选一部好片，邀请好友坐在你身边。',
      child: Column(
        children: [
          TextField(
            controller: roomName,
            decoration: const InputDecoration(labelText: '房间名称'),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: source,
            readOnly: hasMediaSource,
            decoration: InputDecoration(
              labelText: hasMediaSource ? '已选影片' : '视频链接',
              suffixIcon: hasMediaSource
                  ? IconButton(
                      tooltip: '清除选片，改用视频链接',
                      onPressed: onSourceChanged,
                      icon: const Icon(Icons.close_rounded),
                    )
                  : null,
            ),
            minLines: 2,
            maxLines: 3,
          ),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton.icon(
              onPressed: busy ? null : onPickSource,
              icon: const Icon(Icons.video_library_outlined),
              label: const Text('从网盘或媒体库选片'),
            ),
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
      subtitle: '输入邮件中的验证码，即可完成验证。',
      child: Wrap(
        spacing: 10,
        runSpacing: 12,
        children: [
          SizedBox(
            width: 270,
            child: TextField(
              controller: token,
              decoration: const InputDecoration(labelText: '邮箱验证码'),
            ),
          ),
          OutlinedButton(
            onPressed: busy ? null : onRequest,
            child: const Text('重新发送'),
          ),
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
      subtitle: '赴一场朋友的邀约，输入房间码即可加入。',
      child: Column(
        children: [
          TextField(
            controller: roomCode,
            style: const TextStyle(
              letterSpacing: 6,
              fontSize: 24,
              fontWeight: FontWeight.w600,
            ),
            textCapitalization: TextCapitalization.characters,
            decoration: const InputDecoration(
              labelText: '房间码',
              hintText: 'ABC123',
              prefixIcon: Icon(Icons.tag_rounded, size: 22),
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
          const SizedBox(height: 24),
          const Divider(height: 1),
          const SizedBox(height: 22),
          const Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Icon(
                Icons.headphones_outlined,
                color: FrameColors.muted,
                size: 24,
              ),
              SizedBox(width: 12),
              Expanded(
                child: Text(
                  '戴上耳机，准备好零食。\n剩下的时间，留给你们的好故事。',
                  style: TextStyle(
                    color: FrameColors.muted,
                    fontSize: 12,
                    height: 1.9,
                  ),
                ),
              ),
            ],
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
    return FrameSection(
      title: title,
      subtitle: subtitle,
      icon: title == '创建房间'
          ? Icons.add_box_outlined
          : title == '加入房间'
          ? Icons.meeting_room_outlined
          : null,
      child: child,
    );
  }
}
