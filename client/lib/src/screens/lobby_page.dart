import 'package:flutter/material.dart';

import 'approvals_page.dart';
import 'browser_login_dialog.dart';
import 'remote_source_dialog.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../design.dart';
import 'lobby_layout.dart';
import 'mobile_profile.dart';
import 'media_source_browser.dart';
import 'media_center.dart';
import 'account_security_panel.dart';
import 'library_connection_dialog.dart';
import '../appearance.dart';
import 'quark_source_dialog.dart';
import 'provider_source_dialog.dart';
import 'public_rooms_page.dart';
import 'room_join_dialog.dart';
import '../secure_session_store.dart';
import 'library_page.dart';
import 'couple_page.dart';
import 'metadata_search_page.dart';
import 'membership_page.dart';
import 'privacy_page.dart';
import 'room_page.dart';
import 'social_page.dart';
import 'update_page.dart';

enum _WorkspacePanel { media, security }

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
  int _mobileTab = 0;
  _WorkspacePanel? _workspacePanel;
  bool _securityBusy = false;
  final _accountSecurityKey = GlobalKey(debugLabel: 'account-security');
  int? _returnTabAfterLogin;
  bool _preferJoin = false;
  final _visitedTabs = <int>{0};
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

  Future<Session> _passwordLogin() async {
    try {
      return await widget.api.login(
        email: _email.text,
        password: _password.text,
      );
    } on ApiException catch (e) {
      if (e.data['mfa_required'] != true || !mounted) rethrow;
      final code = TextEditingController();
      final accepted = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('身份验证'),
          content: TextField(
            controller: code,
            autofocus: true,
            autofillHints: const [AutofillHints.oneTimeCode],
            decoration: const InputDecoration(labelText: '动态验证码或恢复码'),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('验证'),
            ),
          ],
        ),
      );
      final value = code.text.trim();
      code.dispose();
      if (accepted != true) throw const ApiException('登录已取消');
      return widget.api.finishMFA(e.data['challenge'] as String, value);
    }
  }

  Future<void> _acceptSession(Session session) async {
    if (!mounted) return;
    setState(() {
      _session = session;
      if (_returnTabAfterLogin != null) {
        _mobileTab = _returnTabAfterLogin!;
        _visitedTabs.add(_mobileTab);
        _returnTabAfterLogin = null;
      }
      _verifiedOverride = session.user.emailVerified;
    });
    _password.clear();
    await widget.sessionStore.write(session);
    await _showStartupAnnouncement();
  }

  Future<void> _signIn() async {
    await _run(() async {
      final session = _registerMode
          ? await widget.api.register(
              email: _email.text,
              displayName: _name.text,
              password: _password.text,
            )
          : await _passwordLogin();
      await _acceptSession(session);
    });
  }

  Future<void> _browserSignIn() async {
    await _run(() async {
      final session = await showDialog<Session>(
        context: context,
        builder: (_) => BrowserLoginDialog(api: widget.api),
      );
      if (session != null) await _acceptSession(session);
    });
  }

  void _showAccountSecurity() {
    if (_session == null) return;
    FocusScope.of(context).unfocus();
    setState(() => _workspacePanel = _WorkspacePanel.security);
  }

  Widget _accountSecurity() => AccountSecurityPanel(
    key: _accountSecurityKey,
    onBusyChanged: (busy) {
      if (mounted) setState(() => _securityBusy = busy);
    },
    api: widget.api,
    session: _session!,
  );

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
    FocusScope.of(context).unfocus();
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
    FocusScope.of(context).unfocus();
    final joined = await showDialog<JoinedRoom>(
      context: context,
      barrierDismissible: false,
      builder: (_) => RoomJoinDialog(
        api: widget.api,
        session: session,
        code: _roomCode.text.trim().toUpperCase(),
      ),
    );
    if (joined != null && mounted) {
      _openRoom(joined.session, joined.room);
    }
  }

  void _showPublicRooms() {
    if (MediaQuery.sizeOf(context).width < 700) {
      _selectMobileTab(1);
      return;
    }
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PublicRoomsPage(
          api: widget.api,
          session: _session,
          onJoined: _openRoom,
          onLoginRequired: _loginForRoom,
        ),
      ),
    );
  }

  Future<void> _joinGuest() async {
    final joined = await showDialog<JoinedRoom>(
      context: context,
      barrierDismissible: false,
      builder: (_) => RoomJoinDialog(api: widget.api),
    );
    if (joined != null && mounted) {
      _openRoom(joined.session, joined.room);
    }
  }

  Future<bool> _promptAddRemote() async {
    final session = _session;
    if (session == null) return false;
    final source = await showDialog<MediaSource>(
      context: context,
      builder: (_) => RemoteSourceDialog(api: widget.api, session: session),
    );
    return source != null;
  }

  Future<bool> _promptAddProvider({
    MediaSource? source,
    String? initialProvider,
    Set<String>? allowedProviders,
  }) async {
    final session = _session;
    if (session == null) {
      return false;
    }
    final result = await showDialog<MediaSource>(
      context: context,
      barrierDismissible: false,
      builder: (_) => ProviderSourceDialog(
        api: widget.api,
        session: session,
        source: source,
        initialProvider: initialProvider,
        allowedProviders: allowedProviders,
      ),
    );
    return result != null;
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
          _workspacePanel = null;
          _mobileTab = 0;
          _visitedTabs
            ..clear()
            ..add(0);
          _returnTabAfterLogin = null;
          _preferJoin = false;
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

  Future<void> _showApprovals() async {
    final session = _session;
    if (session == null) return;
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => ApprovalsPage(api: widget.api, session: session),
      ),
    );
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
    if (MediaQuery.sizeOf(context).width < 700) {
      _selectMobileTab(2);
      return;
    }
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

  void _showMediaSources() {
    if (_session == null) return;
    FocusScope.of(context).unfocus();
    setState(() => _workspacePanel = _WorkspacePanel.media);
  }

  void _closeWorkspacePanel() {
    if (_securityBusy) return;
    setState(() => _workspacePanel = null);
  }

  Future<bool> _connectMediaKind(String kind) {
    switch (kind) {
      case 'quark':
        return _promptAddQuark();
      case 'emby':
        return _promptAddLibrary(emby: true);
      case 'webdav':
        return _promptAddLibrary(emby: false);
      case 'remote':
        return _promptAddRemote();
      case 'more-platforms':
        return _promptAddProvider(
          initialProvider: 'twitch',
          allowedProviders: const {
            'tiktok',
            'twitch',
            'huya',
            'douyu',
            'acfun',
            'cctv',
          },
        );
      case 'more-nas':
        return _promptAddProvider(
          initialProvider: 'nextcloud',
          allowedProviders: const {'nextcloud', 'seafile', 'truenas'},
        );
      default:
        return _promptAddProvider(
          initialProvider: kind,
          allowedProviders: {kind},
        );
    }
  }

  Widget _mediaCenter() => MediaCenter(
    key: ValueKey('media-center-${_session!.user.id}'),
    api: widget.api,
    session: _session!,
    onConnect: _connectMediaKind,
    onReconnect: (source) => source.type == 'quark'
        ? _promptAddQuark(source: source)
        : _promptAddProvider(source: source),
    onBrowse: _browseMediaSource,
    onRemoved: (source) {
      if (_selectedMediaSourceId == source.id) {
        setState(() {
          _selectedMediaSourceId = '';
          _selectedMediaPath = '';
          _source.clear();
        });
      }
    },
  );

  Widget _mobileWorkspacePanel() => Scaffold(
    appBar: AppBar(
      title: Text(
        _workspacePanel == _WorkspacePanel.security ? '账号与安全' : '我的媒体源',
      ),
      leading: IconButton(
        tooltip: '返回',
        onPressed: _securityBusy ? null : _closeWorkspacePanel,
        icon: const Icon(Icons.arrow_back_rounded),
      ),
      actions: const [FrameThemeMenu()],
    ),
    body: SafeArea(
      child: SingleChildScrollView(
        key: PageStorageKey('mobile-${_workspacePanel?.name}'),
        keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
        padding: const EdgeInsets.fromLTRB(16, 20, 16, 20),
        child: _workspacePanel == _WorkspacePanel.security
            ? _accountSecurity()
            : _mediaCenter(),
      ),
    ),
  );

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

  Future<bool> _promptAddLibrary({required bool emby}) async {
    final session = _session;
    if (session == null) return false;
    final result = await showDialog<MediaSource>(
      context: context,
      barrierDismissible: false,
      builder: (_) => LibraryConnectionDialog(
        api: widget.api,
        session: session,
        emby: emby,
      ),
    );
    return result != null;
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
      _workspacePanel = null;
      _mobileTab = 0;
      _visitedTabs.add(0);
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

  void _selectMobileTab(int tab) {
    if (_securityBusy) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _workspacePanel = null;
      _mobileTab = tab;
      _visitedTabs.add(tab);
    });
  }

  void _requestLogin(int returnTab) {
    setState(() {
      _mobileTab = 0;
      _returnTabAfterLogin = returnTab;
      _registerMode = false;
    });
  }

  void _loginForRoom(String code) {
    if (!mounted) return;
    setState(() {
      _roomCode.text = code;
      _preferJoin = true;
      _registerMode = false;
      _mobileTab = 0;
      _returnTabAfterLogin = 0;
    });
  }

  @override
  Widget build(BuildContext context) {
    if (MediaQuery.sizeOf(context).width >= 700) {
      return PopScope(
        canPop: _workspacePanel == null,
        onPopInvokedWithResult: (didPop, result) {
          if (!didPop) _closeWorkspacePanel();
        },
        child: _buildLobby(),
      );
    }
    final colors = FrameColors.of(context);
    final session = _session;
    return PopScope(
      canPop: _mobileTab == 0 && _workspacePanel == null,
      onPopInvokedWithResult: (didPop, result) {
        if (!didPop) {
          if (_workspacePanel != null) {
            _closeWorkspacePanel();
          } else {
            _selectMobileTab(0);
          }
        }
      },
      child: Scaffold(
        body: PrimaryScrollController.none(
          child: Stack(
            children: [
              Offstage(
                offstage: _workspacePanel != null,
                child: IndexedStack(
                  index: _mobileTab,
                  children: [
                    _buildLobby(mobileTabs: true),
                    if (_visitedTabs.contains(1))
                      PublicRoomsPage(
                        api: widget.api,
                        session: session,
                        embedded: true,
                        onJoined: _openRoom,
                        onLoginRequired: _loginForRoom,
                      )
                    else
                      const SizedBox.shrink(),
                    if (!_visitedTabs.contains(2))
                      const SizedBox.shrink()
                    else if (session != null)
                      LibraryPage(
                        key: ValueKey(session.user.id),
                        api: widget.api,
                        session: session,
                        embedded: true,
                      )
                    else
                      MobileLibrarySignIn(onLogin: () => _requestLogin(2)),
                    MobileProfile(
                      name: session?.user.displayName ?? '',
                      email: session?.user.email ?? '',
                      signedIn: session != null,
                      onLogin: () => _requestLogin(3),
                      onLogout: _logout,
                      verification: session == null || _verifiedOverride
                          ? null
                          : _VerificationCard(
                              token: _verificationToken,
                              busy: _busy,
                              onRequest: _requestVerification,
                              onVerify: _verifyEmail,
                            ),
                      items: [
                        (
                          icon: Icons.folder_open_rounded,
                          label: '我的媒体源',
                          onTap: session == null ? null : _showMediaSources,
                        ),
                        (
                          icon: Icons.people_outline_rounded,
                          label: '好友与私聊',
                          onTap: session == null ? null : _showSocial,
                        ),
                        (
                          icon: Icons.favorite_border_rounded,
                          label: '情侣空间',
                          onTap: session == null ? null : _showCouple,
                        ),
                        (
                          icon: Icons.security_outlined,
                          label: '账号与安全',
                          onTap: session == null ? null : _showAccountSecurity,
                        ),
                        (
                          icon: Icons.fact_check_outlined,
                          label: '我的申请',
                          onTap: session == null ? null : _showApprovals,
                        ),
                        (
                          icon: Icons.devices_rounded,
                          label: '设备管理',
                          onTap: session == null ? null : _showDevices,
                        ),
                        (
                          icon: Icons.shield_outlined,
                          label: '隐私与屏蔽',
                          onTap: session == null ? null : _showPrivacy,
                        ),
                        (
                          icon: Icons.workspace_premium_outlined,
                          label: '会员中心',
                          onTap: session == null ? null : _showMembership,
                        ),
                        (
                          icon: Icons.system_update_alt_rounded,
                          label: '安全更新',
                          onTap: _showUpdates,
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              if (_workspacePanel != null && session != null)
                _mobileWorkspacePanel(),
            ],
          ),
        ),
        bottomNavigationBar: MediaQuery.viewInsetsOf(context).bottom > 0
            ? null
            : DecoratedBox(
                decoration: BoxDecoration(
                  border: Border(top: BorderSide(color: colors.border)),
                ),
                child: NavigationBarTheme(
                  data: NavigationBarThemeData(
                    backgroundColor: colors.surface,
                    surfaceTintColor: Colors.transparent,
                    indicatorColor: Colors.transparent,
                    labelTextStyle: WidgetStateProperty.resolveWith(
                      (states) => TextStyle(
                        fontSize: 11,
                        fontWeight: states.contains(WidgetState.selected)
                            ? FontWeight.w600
                            : FontWeight.w400,
                        color: states.contains(WidgetState.selected)
                            ? colors.accent
                            : colors.muted,
                      ),
                    ),
                    iconTheme: WidgetStateProperty.resolveWith(
                      (states) => IconThemeData(
                        size: 24,
                        color: states.contains(WidgetState.selected)
                            ? colors.accent
                            : colors.muted,
                      ),
                    ),
                  ),
                  child: NavigationBar(
                    height: 72,
                    elevation: 0,
                    selectedIndex: _mobileTab,
                    onDestinationSelected: _securityBusy
                        ? null
                        : _selectMobileTab,
                    labelBehavior:
                        NavigationDestinationLabelBehavior.alwaysShow,
                    destinations: const [
                      NavigationDestination(
                        icon: Icon(Icons.meeting_room_outlined),
                        selectedIcon: Icon(Icons.meeting_room_rounded),
                        label: '房间',
                      ),
                      NavigationDestination(
                        icon: Icon(Icons.explore_outlined),
                        selectedIcon: Icon(Icons.explore_rounded),
                        label: '发现',
                      ),
                      NavigationDestination(
                        icon: Icon(Icons.video_library_outlined),
                        selectedIcon: Icon(Icons.video_library_rounded),
                        label: '片库',
                      ),
                      NavigationDestination(
                        icon: Icon(Icons.person_outline_rounded),
                        selectedIcon: Icon(Icons.person_rounded),
                        label: '我的',
                      ),
                    ],
                  ),
                ),
              ),
      ),
    );
  }

  Widget _buildLobby({bool mobileTabs = false}) => LobbyLayout(
    mobileTabs: mobileTabs,
    navigationLocked: _securityBusy,
    content: _session == null || mobileTabs
        ? null
        : switch (_workspacePanel) {
            _WorkspacePanel.media => _mediaCenter(),
            _WorkspacePanel.security => _accountSecurity(),
            null => null,
          },
    activeDestination: switch (_workspacePanel) {
      _WorkspacePanel.media => '我的媒体源',
      _WorkspacePanel.security => '账号与安全',
      null => null,
    },
    onHome: _closeWorkspacePanel,
    preferJoin: _preferJoin,
    signedIn: _session != null,
    name: _session?.user.displayName ?? '',
    error: _error,
    onSearch: _showMetadataSearch,
    onDiscover: _showPublicRooms,
    onGuestJoin: _joinGuest,
    onMembership: _showMembership,
    onUpdates: _showUpdates,
    onLogout: _logout,
    destinations: [
      LobbyDestination(Icons.video_library_outlined, '收藏与历史', _showLibrary),
      LobbyDestination(Icons.people_outline_rounded, '好友与私聊', _showSocial),
      LobbyDestination(Icons.favorite_border_rounded, '情侣空间', _showCouple),
      LobbyDestination(Icons.folder_open_rounded, '我的媒体源', _showMediaSources),
      LobbyDestination(Icons.security_outlined, '账号与安全', _showAccountSecurity),
      LobbyDestination(Icons.fact_check_outlined, '我的申请', _showApprovals),
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
      onBrowserLogin: _browserSignIn,
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
    required this.onBrowserLogin,
    required this.onToggleMode,
    required this.onForgotPassword,
  });
  final TextEditingController name;
  final TextEditingController email;
  final TextEditingController password;
  final bool registerMode;
  final bool busy;
  final VoidCallback onSubmit;
  final VoidCallback onBrowserLogin;
  final VoidCallback onToggleMode;
  final VoidCallback onForgotPassword;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 520),
      child: _Panel(
        title: registerMode ? '创建账号' : '登录 SameFrame',
        subtitle: registerMode ? '为下一次相聚，留一个位置。' : '欢迎回来，继续你们的故事。',
        child: Column(
          children: [
            if (registerMode) ...[
              TextField(
                controller: name,
                autofocus: MediaQuery.sizeOf(context).width >= 700,
                textInputAction: TextInputAction.next,
                autofillHints: const [AutofillHints.nickname],
                decoration: const InputDecoration(
                  labelText: '你的昵称',
                  prefixIcon: Icon(Icons.person_outline_rounded, size: 19),
                ),
              ),
              const SizedBox(height: 12),
            ],
            TextField(
              controller: email,
              autofocus:
                  !registerMode && MediaQuery.sizeOf(context).width >= 700,
              textInputAction: TextInputAction.next,
              autofillHints: const [AutofillHints.email],
              keyboardType: TextInputType.emailAddress,
              decoration: const InputDecoration(
                labelText: '邮箱',
                prefixIcon: Icon(Icons.alternate_email_rounded, size: 19),
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: password,
              obscureText: true,
              textInputAction: TextInputAction.done,
              decoration: const InputDecoration(
                labelText: '密码',
                prefixIcon: Icon(Icons.lock_outline_rounded, size: 19),
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
            OutlinedButton.icon(
              onPressed: busy ? null : onBrowserLogin,
              icon: const Icon(Icons.fingerprint),
              label: const Text('邮箱验证码 / 第三方 / Passkey'),
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
    final mobile = MediaQuery.sizeOf(context).width < 700;
    return _Panel(
      title: '创建房间',
      subtitle: '从一个链接，开始你们的共同放映。',
      compact: mobile,
      child: Column(
        children: [
          TextField(
            controller: roomName,
            textInputAction: TextInputAction.next,
            decoration: const InputDecoration(
              labelText: '房间名称',
              prefixIcon: Icon(Icons.meeting_room_outlined, size: 19),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: source,
            readOnly: hasMediaSource,
            keyboardType: TextInputType.url,
            textInputAction: TextInputAction.done,
            decoration: InputDecoration(
              labelText: hasMediaSource ? '已选影片' : '视频链接',
              hintText: '粘贴视频直链，或从媒体库选片',
              suffixIcon: hasMediaSource
                  ? IconButton(
                      tooltip: '清除选片，改用视频链接',
                      onPressed: onSourceChanged,
                      icon: const Icon(Icons.close_rounded),
                    )
                  : null,
            ),
            minLines: mobile ? 1 : 2,
            maxLines: mobile ? 1 : 3,
          ),
          if (mobile) ...[
            const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              child: OutlinedButton.icon(
                onPressed: busy ? null : onPickSource,
                icon: const Icon(Icons.video_library_outlined, size: 18),
                label: const Text('网盘 / 平台选片'),
              ),
            ),
          ] else
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
              style: mobile
                  ? FilledButton.styleFrom(minimumSize: const Size(0, 50))
                  : null,
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
      child: LayoutBuilder(
        builder: (context, constraints) => Wrap(
          spacing: 10,
          runSpacing: 12,
          children: [
            SizedBox(
              width: constraints.maxWidth < 270 ? constraints.maxWidth : 270,
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
    final mobile = MediaQuery.sizeOf(context).width < 700;
    return _Panel(
      title: '加入房间',
      subtitle: '赴一场朋友的邀约，输入房间码即可加入。',
      compact: mobile,
      child: Column(
        children: [
          TextField(
            controller: roomCode,
            textInputAction: TextInputAction.done,
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
            child: mobile
                ? FilledButton.icon(
                    onPressed: busy ? null : onSubmit,
                    style: FilledButton.styleFrom(
                      minimumSize: const Size(0, 50),
                    ),
                    icon: const Icon(Icons.login_rounded),
                    label: const Text('进入房间'),
                  )
                : OutlinedButton.icon(
                    onPressed: busy ? null : onSubmit,
                    icon: const Icon(Icons.login_rounded),
                    label: const Text('加入房间'),
                  ),
          ),
          if (!mobile) ...[
            const SizedBox(height: 24),
            const Divider(height: 1),
            const SizedBox(height: 22),
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  Icons.headphones_outlined,
                  color: FrameColors.of(context).muted,
                  size: 24,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    '戴上耳机，准备好零食。\n剩下的时间，留给你们的好故事。',
                    style: TextStyle(
                      color: FrameColors.of(context).muted,
                      fontSize: 12,
                      height: 1.9,
                    ),
                  ),
                ),
              ],
            ),
          ],
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
    this.compact = false,
  });
  final String title;
  final String subtitle;
  final Widget child;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    return FrameSection(
      showHeading: !compact,
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
