import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import 'approvals_page.dart';
import 'devices_page.dart';
import 'workspace_navigation.dart';
import 'workspace_section.dart';
import 'public_auth_page.dart';
import '../browser_entry.dart';
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

enum _WorkspacePanel {
  discover,
  library,
  profile,
  media,
  security,
  approvals,
  devices,
  privacy,
  search,
  social,
  couple,
  membership,
  updates,
}

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
  final _registrationCode = TextEditingController();
  final _inviteCode = TextEditingController();
  bool _requiresInvite = false, _requiresEmailCode = false;
  bool _registrationAllowed = true, _registrationRulesReady = false;
  bool _loadingRegistrationRules = false;

  String _registrationChallenge = '', _registrationEmail = '';
  String? _registrationNotice;
  bool _sendingCode = false;
  int _resendSeconds = 0;
  Timer? _resendTimer;

  final _roomName = TextEditingController(text: '周五放映室');
  final _source = TextEditingController(
    text: 'https://flutter.github.io/assets-for-api-docs/assets/videos/bee.mp4',
  );
  final _roomCode = TextEditingController();
  final _verificationToken = TextEditingController();
  Session? _session;
  MembershipStatus? _membership;
  Timer? _membershipRefresh;
  bool _refreshingMembership = false;
  int _mobileTab = 0;
  _WorkspacePanel? _workspacePanel;
  bool _securityBusy = false;
  int? _returnTabAfterLogin;
  bool _preferJoin = false;
  final _workspaceNavigator = GlobalKey<NavigatorState>(
    debugLabel: 'workspace',
  );
  late final _workspaceObserver = WorkspaceObserver(_navigationChanged);
  bool _workspaceCanPop = false;
  bool _registerMode = true;
  bool _publicAuthEntry = false;
  bool _restoringSession = true;
  bool _verifiedOverride = false;
  bool _busy = false;
  String? _error;
  String _selectedMediaSourceId = '';
  String _selectedMediaPath = '';

  @override
  void initState() {
    super.initState();
    if (kIsWeb) {
      final entry = Uri.base.path.split('/').last;
      _publicAuthEntry = true;
      _registerMode = entry == 'register.html' || entry == 'register';
      if ((entry == 'app.html' || entry == 'app') &&
          Uri.base.queryParameters['discover'] == '1') {
        _workspacePanel = _WorkspacePanel.discover;
        _mobileTab = 1;
      }
    }
    _loadRegistrationRules();
    _restoreSession();
  }

  Future<void> _loadRegistrationRules({bool preserveError = false}) async {
    if (_loadingRegistrationRules) return;
    setState(() {
      _loadingRegistrationRules = true;
    });
    try {
      final options = await widget.api.loginOptions();
      if (!mounted) return;
      setState(() {
        _registrationAllowed = options['registration_enabled'] != false;
        _requiresInvite = options['registration_invite_required'] == true;
        _requiresEmailCode =
            options['registration_email_code_required'] == true;
        _registrationRulesReady = true;
        if (!preserveError) _error = null;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _registrationRulesReady = false;
          if (!preserveError) _error = '暂时无法获取注册设置，请点击重试。';
        });
      }
    } finally {
      if (mounted) setState(() => _loadingRegistrationRules = false);
    }
  }

  Future<void> _restoreSession() async {
    try {
      Session? session;
      if (kIsWeb) {
        session = await widget.api.claimWebEntry();
        if (session != null) await widget.sessionStore.write(session);
      }
      session ??= await widget.sessionStore.read();
      if (!mounted) return;
      if (session == null || session.user.isGuest) {
        if (session != null) await widget.sessionStore.clear();
        if (kIsWeb) openAccountEntry(_registerMode ? 'register' : 'login');
        return;
      }
      await widget.api.listDevices(session);
      if (!mounted) return;
      setState(() {
        _session = session;
        _verifiedOverride = session.user.emailVerified;
      });
      if (kIsWeb && _publicAuthEntry) setBrowserEntry('app');
      await _refreshMembership();
      await _showStartupAnnouncement();
    } catch (_) {
      await widget.sessionStore.clear();
      if (kIsWeb) openAccountEntry('login');
    } finally {
      if (mounted) setState(() => _restoringSession = false);
    }
  }

  @override
  void dispose() {
    _membershipRefresh?.cancel();
    _name.dispose();
    _email.dispose();
    _password.dispose();
    _registrationCode.dispose();
    _inviteCode.dispose();
    _resendTimer?.cancel();
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
        _workspacePanel = _panelForTab(_mobileTab);
        _returnTabAfterLogin = null;
      }
      _verifiedOverride = session.user.emailVerified;
    });
    _password.clear();
    if (kIsWeb) setBrowserEntry('app');
    await widget.sessionStore.write(session);
    await _refreshMembership();
    await _showStartupAnnouncement();
  }

  Future<void> _signIn() async {
    if (_busy || (_publicAuthEntry && _restoringSession)) return;
    if (_registerMode &&
        (!_registrationRulesReady || _loadingRegistrationRules)) {
      return;
    }
    if (_registerMode && !_registrationAllowed) {
      setState(() => _error = '站点当前暂停注册，请联系管理员');
      return;
    }
    if (_registerMode && _requiresInvite && _inviteCode.text.trim().isEmpty) {
      setState(() => _error = '请填写邀请码');
      return;
    }
    if (_registerMode &&
        _requiresEmailCode &&
        (_registrationChallenge.isEmpty ||
            _registrationEmail != _email.text.trim().toLowerCase() ||
            !RegExp(r'^\d{6}$').hasMatch(_registrationCode.text.trim()))) {
      setState(() => _error = '请先获取并填写该邮箱的 6 位验证码');
      return;
    }
    await _run(() async {
      final session = _registerMode
          ? await widget.api.register(
              email: _email.text,
              displayName: _name.text,
              password: _password.text,
              challenge: _registrationChallenge,
              code: _registrationCode.text.trim(),
              inviteCode: _requiresInvite ? _inviteCode.text.trim() : '',
            )
          : await _passwordLogin();
      await _acceptSession(session);
    });
    if (mounted && _session == null && _registerMode) {
      await _loadRegistrationRules(preserveError: true);
    }
  }

  Future<void> _sendRegistrationCode() async {
    if (!_requiresEmailCode || _sendingCode || _resendSeconds > 0 || _busy) {
      return;
    }
    final email = _email.text.trim().toLowerCase();
    setState(() {
      _sendingCode = true;
      _error = null;
      _registrationNotice = null;
    });
    try {
      final result = await widget.api.requestRegistrationCode(email);
      if (!mounted) return;
      setState(() {
        _registrationChallenge = result['challenge'] as String;
        _registrationEmail = email;
        _registrationCode.clear();
        _resendSeconds = (result['retry_after'] as num?)?.toInt() ?? 60;
        _registrationNotice = '验证码已发送，10 分钟内有效，请查看邮箱。';
      });
      _resendTimer?.cancel();
      _resendTimer = Timer.periodic(const Duration(seconds: 1), (timer) {
        if (!mounted) {
          timer.cancel();
          return;
        }
        setState(() => _resendSeconds--);
        if (_resendSeconds <= 0) timer.cancel();
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => _error = e is ApiException ? e.message : '验证码发送失败，请稍后重试',
        );
      }
    } finally {
      if (mounted) setState(() => _sendingCode = false);
    }
  }

  void _setMembership(MembershipStatus status) {
    if (!mounted || _session == null) return;
    setState(() => _membership = status);
    _membershipRefresh?.cancel();
    if (!status.isCurrent) return;
    final remaining =
        status.vipExpiresAt - DateTime.now().millisecondsSinceEpoch;
    final delay = status.isCurrent && remaining < 60000
        ? remaining + 100
        : 60000;
    _membershipRefresh = Timer(
      Duration(milliseconds: delay),
      _refreshMembership,
    );
  }

  Future<void> _refreshMembership() async {
    final session = _session;
    if (session == null || _refreshingMembership) return;
    _refreshingMembership = true;
    try {
      final status = await widget.api.membership(session);
      if (mounted && identical(_session, session)) _setMembership(status);
    } catch (_) {
      if (mounted && identical(_session, session)) {
        setState(() {});
        _membershipRefresh?.cancel();
        if (_membership?.isCurrent == true) {
          _membershipRefresh = Timer(
            const Duration(minutes: 1),
            _refreshMembership,
          );
        }
      }
    } finally {
      _refreshingMembership = false;
    }
  }

  void _showAccountSecurity() => _showPanel(_WorkspacePanel.security);

  Widget _accountSecurity() => AccountSecurityPanel(
    key: ValueKey('account-security-${_session!.user.id}'),
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

  void _showPublicRooms() => _selectMobileTab(1);

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
        if (kIsWeb) openAccountEntry('login');
        setState(() {
          _session = null;
          _publicAuthEntry = kIsWeb;
          _registerMode = false;
          _membership = null;
          _membershipRefresh?.cancel();
          _workspacePanel = null;
          _mobileTab = 0;
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

  void _showApprovals() => _showPanel(_WorkspacePanel.approvals);
  void _showDevices() => _showPanel(_WorkspacePanel.devices);
  void _showPrivacy() => _showPanel(_WorkspacePanel.privacy);
  void _showLibrary() => _selectMobileTab(2);
  void _showMetadataSearch() => _showPanel(_WorkspacePanel.search);
  void _showSocial() => _showPanel(_WorkspacePanel.social);
  void _showCouple() => _showPanel(_WorkspacePanel.couple);
  void _showMembership() => _showPanel(_WorkspacePanel.membership);
  void _showUpdates() {
    if (!kIsWeb) _showPanel(_WorkspacePanel.updates);
  }

  void _showMediaSources() => _showPanel(_WorkspacePanel.media);

  void _showPanel(_WorkspacePanel panel) {
    if (_securityBusy ||
        (kIsWeb && panel == _WorkspacePanel.updates) ||
        (_session == null && panel != _WorkspacePanel.updates)) {
      return;
    }
    FocusScope.of(context).unfocus();
    if (_workspacePanel == panel) {
      _workspaceNavigator.currentState?.popUntil((route) => route.isFirst);
    }
    setState(() => _workspacePanel = panel);
    unawaited(_refreshMembership());
  }

  void _closeWorkspacePanel() {
    if (_securityBusy) return;
    final mobile = MediaQuery.sizeOf(context).width < 700;
    final primary = _panelForTab(_mobileTab);
    if (mobile && _workspacePanel != primary) {
      setState(() => _workspacePanel = primary);
    } else {
      _selectMobileTab(0);
    }
  }

  Future<void> _backWorkspace() async {
    if (_securityBusy) return;
    if (await _workspaceNavigator.currentState!.maybePop()) return;
    if (mounted) _closeWorkspacePanel();
  }

  void _navigationChanged() {
    if (!mounted) return;
    final canPop = _workspaceNavigator.currentState?.canPop() ?? false;
    if (canPop != _workspaceCanPop) {
      setState(() => _workspaceCanPop = canPop);
    }
  }

  _WorkspacePanel? _panelForTab(int tab) => switch (tab) {
    1 => _WorkspacePanel.discover,
    2 => _WorkspacePanel.library,
    3 => _WorkspacePanel.profile,
    _ => null,
  };

  String? get _workspaceTitle => switch (_workspacePanel) {
    _WorkspacePanel.discover => '发现房间',
    _WorkspacePanel.library => '收藏与历史',
    _WorkspacePanel.profile => '我的账号',
    _WorkspacePanel.media => '我的媒体源',
    _WorkspacePanel.security => '账号与安全',
    _WorkspacePanel.approvals => '我的申请',
    _WorkspacePanel.devices => '设备管理',
    _WorkspacePanel.privacy => '隐私与屏蔽',
    _WorkspacePanel.search => '影视搜索',
    _WorkspacePanel.social => '好友与私聊',
    _WorkspacePanel.couple => '情侣空间',
    _WorkspacePanel.membership => '会员中心',
    _WorkspacePanel.updates => '安全更新',
    null => null,
  };

  (String, String) get _sectionInfo => switch (_workspacePanel) {
    _WorkspacePanel.discover => ('DISCOVER', '找到正在放映的房间，和大家一起看。'),
    _WorkspacePanel.library => ('LIBRARY', '收藏喜欢的影片，继续上次的观看进度。'),
    _WorkspacePanel.profile => ('MY ACCOUNT', '管理你的账号与观影空间。'),
    _WorkspacePanel.media => ('MEDIA SOURCES', '连接网盘、视频平台和私人媒体库。'),
    _WorkspacePanel.security => ('SECURITY', '管理登录方式，保护你的账号。'),
    _WorkspacePanel.approvals => ('APPLICATIONS', '查看注册、建房和加入房间申请的进度。'),
    _WorkspacePanel.devices => ('DEVICES', '查看登录设备，管理设备访问。'),
    _WorkspacePanel.privacy => ('PRIVACY', '设置聊天、资料和观看活动的可见范围。'),
    _WorkspacePanel.search => ('SEARCH', '查找影片资料，分享你的观影感受。'),
    _WorkspacePanel.social => ('FRIENDS', '找到朋友，分享每一个观影时刻。'),
    _WorkspacePanel.couple => ('TOGETHER', '珍藏属于你们的共同观影时光。'),
    _WorkspacePanel.membership => ('MEMBERSHIP', '查看会员权益、积分与订单。'),
    _WorkspacePanel.updates => ('UPDATES', '检查版本更新，保持应用最新。'),
    null => ('HOME', '开启一场共同放映。'),
  };

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

  Widget _workspace(Widget home) {
    final mobile = MediaQuery.sizeOf(context).width < 700;
    final session = _session;
    final title = _workspaceTitle;
    final panelContent = switch (_workspacePanel) {
      _WorkspacePanel.security => _accountSecurity(),
      _WorkspacePanel.media => _mediaCenter(),
      _ => null,
    };
    final page = switch (_workspacePanel) {
      null => home,
      _WorkspacePanel.discover => PublicRoomsPage(
        api: widget.api,
        session: session,
        embedded: true,
        onJoined: _openRoom,
        onLoginRequired: _loginForRoom,
      ),
      _WorkspacePanel.library =>
        session == null
            ? MobileLibrarySignIn(onLogin: () => _requestLogin(2))
            : LibraryPage(api: widget.api, session: session, embedded: mobile),
      _WorkspacePanel.profile => _profile(),
      _WorkspacePanel.media || _WorkspacePanel.security => WorkspaceSection(
        title: Text(title!),
        body: SingleChildScrollView(
          key: PageStorageKey(_workspacePanel),
          keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
          padding: EdgeInsets.all(mobile ? 16 : 20),
          child: panelContent,
        ),
      ),
      _WorkspacePanel.approvals => ApprovalsPage(
        api: widget.api,
        session: session!,
      ),
      _WorkspacePanel.devices => DevicesPage(
        api: widget.api,
        session: session!,
      ),
      _WorkspacePanel.privacy => PrivacyPage(
        api: widget.api,
        session: session!,
      ),
      _WorkspacePanel.search => MetadataSearchPage(
        api: widget.api,
        session: session!,
      ),
      _WorkspacePanel.social => SocialPage(api: widget.api, session: session!),
      _WorkspacePanel.couple => CouplePage(api: widget.api, session: session!),
      _WorkspacePanel.membership => MembershipPage(
        api: widget.api,
        session: session!,
        onMembershipChanged: _setMembership,
      ),
      _WorkspacePanel.updates =>
        kIsWeb ? const SizedBox.shrink() : const UpdatePage(),
    };
    // A route's modal barrier must not hide the shell's navigation from
    // accessibility tools. Keep its semantics inside the content region.
    final (eyebrow, description) = _sectionInfo;
    return WorkspaceSectionInfo(
      eyebrow: eyebrow,
      description: description,
      child: Semantics(
        container: true,
        explicitChildNodes: true,
        child: Navigator(
          key: _workspaceNavigator,
          observers: [_workspaceObserver],
          pages: [
            WorkspacePage(
              key: ValueKey(
                '${session?.user.id}:${_workspacePanel?.name ?? 'home'}',
              ),
              child: page,
            ),
          ],
          onDidRemovePage: (_) {},
        ),
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
    _workspaceNavigator.currentState!.push(
      MaterialPageRoute<void>(
        builder: (_) =>
            RoomPage(api: widget.api, session: session, initialRoom: room),
      ),
    );
  }

  void _selectMobileTab(int tab) {
    if (_securityBusy) return;
    FocusScope.of(context).unfocus();
    if (_workspacePanel == _panelForTab(tab)) {
      _workspaceNavigator.currentState?.popUntil((route) => route.isFirst);
    }
    setState(() {
      _workspacePanel = _panelForTab(tab);
      _mobileTab = tab;
    });
    unawaited(_refreshMembership());
  }

  void _requestLogin(int returnTab) {
    if (kIsWeb) setBrowserEntry('login');
    setState(() {
      _publicAuthEntry = kIsWeb;
      _error = null;
      _workspacePanel = null;
      _mobileTab = 0;
      _returnTabAfterLogin = returnTab;
      _registerMode = false;
    });
  }

  void _loginForRoom(String code) {
    if (!mounted) return;
    if (kIsWeb) setBrowserEntry('login');
    setState(() {
      _publicAuthEntry = kIsWeb;
      _error = null;
      _workspacePanel = null;
      _roomCode.text = code;
      _preferJoin = true;
      _registerMode = false;
      _mobileTab = 0;
      _returnTabAfterLogin = 0;
    });
  }

  Widget _profile() {
    final session = _session;
    return MobileProfile(
      name: session?.user.displayName ?? '',
      email: session?.user.email ?? '',
      signedIn: session != null,
      member: _membership?.isCurrent ?? false,
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
        if (!kIsWeb)
          (
            icon: Icons.system_update_alt_rounded,
            label: '安全更新',
            onTap: _showUpdates,
          ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) =>
      kIsWeb && _restoringSession && _session == null
      ? const Scaffold(
          body: Center(
            child: SizedBox(
              width: 22,
              height: 22,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
          ),
        )
      : _session == null
      ? PublicAuthPage(
          registerMode: _registerMode,
          restoring: _restoringSession,
          form: _loginCard(framed: false),
          error: _error,
          onHome: openOfficialWebsite,
        )
      : PopScope(
          canPop:
              !_securityBusy &&
              !_workspaceCanPop &&
              _workspacePanel == null &&
              _mobileTab == 0,
          onPopInvokedWithResult: (didPop, result) {
            if (!didPop) _backWorkspace();
          },
          child: _buildLobby(),
        );

  int get _dockIndex => switch (_workspacePanel) {
    null => 0,
    _WorkspacePanel.discover => 1,
    _WorkspacePanel.library => 2,
    _WorkspacePanel.profile => 3,
    _ => 4,
  };

  Widget _buildLobby() => LobbyLayout(
    selectedTab: _dockIndex,
    onSelectTab: _selectMobileTab,
    navigationLocked: _securityBusy,
    workspaceBuilder: _workspace,
    activeDestination: _workspaceTitle,
    onHome: () => _selectMobileTab(0),
    preferJoin: _preferJoin,
    signedIn: _session != null,
    member: _membership?.isCurrent ?? false,
    name: _session?.user.displayName ?? '',
    error: _error,
    onSearch: _showMetadataSearch,
    onDiscover: _showPublicRooms,
    onMembership: _showMembership,
    onUpdates: kIsWeb ? null : _showUpdates,
    onOfficialSite: kIsWeb ? openOfficialWebsite : null,
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
      LobbyDestination(
        Icons.person_outline_rounded,
        '我的账号',
        () => _selectMobileTab(3),
      ),
      LobbyDestination(Icons.search_rounded, '影视搜索', _showMetadataSearch),
      LobbyDestination(
        Icons.workspace_premium_outlined,
        '会员中心',
        _showMembership,
      ),
    ],
    login: _loginCard(),
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
  Widget _loginCard({bool framed = true}) => _LoginCard(
    name: _name,
    email: _email,
    password: _password,
    registerMode: _registerMode,
    busy: _busy || _restoringSession,
    framed: framed,
    onSubmit: _signIn,
    registrationCode: _registrationCode,
    inviteCode: _inviteCode,
    requiresInvite: _requiresInvite,
    requiresEmailCode: _requiresEmailCode,
    registrationAllowed: _registrationAllowed,
    rulesReady: _registrationRulesReady,
    rulesLoading: _loadingRegistrationRules,
    onReloadRules: _loadRegistrationRules,
    onSendCode: _sendRegistrationCode,
    sendingCode: _sendingCode,
    resendSeconds: _resendSeconds,
    notice: _registrationNotice,
    onForgotPassword: _showPasswordReset,
    onToggleMode: () {
      setState(() {
        _registerMode = !_registerMode;
        _error = null;
      });
      if (_registerMode) _loadRegistrationRules();
      if (kIsWeb && _publicAuthEntry) {
        setBrowserEntry(_registerMode ? 'register' : 'login');
      }
    },
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
    required this.registrationCode,
    required this.inviteCode,
    required this.requiresInvite,
    required this.requiresEmailCode,
    required this.registrationAllowed,
    required this.rulesReady,
    required this.rulesLoading,
    required this.onReloadRules,
    required this.onSendCode,
    required this.sendingCode,
    required this.resendSeconds,
    this.notice,
    required this.onToggleMode,
    required this.onForgotPassword,
    this.framed = true,
  });
  final TextEditingController name;
  final TextEditingController email;
  final TextEditingController password;
  final bool registerMode;
  final bool busy, framed;
  final VoidCallback onSubmit;
  final TextEditingController registrationCode, inviteCode;
  final bool requiresInvite, requiresEmailCode, registrationAllowed;
  final bool rulesReady, rulesLoading;
  final VoidCallback onReloadRules;
  final VoidCallback onSendCode;
  final bool sendingCode;
  final int resendSeconds;
  final String? notice;
  final VoidCallback onToggleMode;
  final VoidCallback onForgotPassword;

  @override
  Widget build(BuildContext context) {
    final fields = AutofillGroup(
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
            autofocus: !registerMode && MediaQuery.sizeOf(context).width >= 700,
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
            autofillHints: [
              registerMode ? AutofillHints.newPassword : AutofillHints.password,
            ],
            textInputAction: TextInputAction.done,
            decoration: const InputDecoration(
              labelText: '密码',
              prefixIcon: Icon(Icons.lock_outline_rounded, size: 19),
              helperText: '至少 10 个字符',
            ),
            onSubmitted: (_) {
              if (!busy) onSubmit();
            },
          ),
          if (registerMode && requiresInvite) ...[
            const SizedBox(height: 12),
            TextField(
              controller: inviteCode,
              textCapitalization: TextCapitalization.characters,
              autocorrect: false,
              enableSuggestions: false,
              textInputAction: TextInputAction.done,
              decoration: const InputDecoration(
                labelText: '邀请码',
                helperText: '请填写管理员提供的邀请码',
                prefixIcon: Icon(Icons.vpn_key_outlined, size: 19),
              ),
              onSubmitted: (_) {
                if (!busy) onSubmit();
              },
            ),
          ],
          if (registerMode && requiresEmailCode) ...[
            const SizedBox(height: 12),
            TextField(
              controller: registrationCode,
              keyboardType: TextInputType.number,
              autofillHints: const [AutofillHints.oneTimeCode],
              maxLength: 6,
              decoration: InputDecoration(
                labelText: '邮箱验证码',
                counterText: '',
                prefixIcon: const Icon(
                  Icons.mark_email_read_outlined,
                  size: 19,
                ),
                suffixIcon: Padding(
                  padding: const EdgeInsets.only(right: 6),
                  child: TextButton(
                    onPressed: busy || sendingCode || resendSeconds > 0
                        ? null
                        : onSendCode,
                    style: TextButton.styleFrom(
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                    ),
                    child: Text(
                      sendingCode
                          ? '发送中…'
                          : resendSeconds > 0
                          ? '${resendSeconds}s'
                          : '获取验证码',
                      style: const TextStyle(fontSize: 12),
                    ),
                  ),
                ),
              ),
              onSubmitted: (_) {
                if (!busy) onSubmit();
              },
            ),
            if (notice != null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  notice!,
                  style: TextStyle(
                    fontSize: 12,
                    color: FrameColors.of(context).muted,
                  ),
                ),
              ),
          ],
          if (registerMode && rulesLoading)
            const Padding(
              padding: EdgeInsets.only(top: 12),
              child: LinearProgressIndicator(),
            ),
          if (registerMode && !rulesLoading && !rulesReady)
            TextButton.icon(
              onPressed: onReloadRules,
              icon: const Icon(Icons.refresh),
              label: const Text('重试获取注册设置'),
            ),
          if (registerMode && rulesReady && !registrationAllowed)
            const Padding(
              padding: EdgeInsets.only(top: 12),
              child: Text('站点当前暂停注册，请联系管理员。'),
            ),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: FilledButton.icon(
              onPressed:
                  busy ||
                      (registerMode &&
                          (!rulesReady || rulesLoading || !registrationAllowed))
                  ? null
                  : onSubmit,
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
    );
    if (!framed) return fields;
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 520),
      child: _Panel(
        title: registerMode ? '创建账号' : '登录 SameFrame',
        subtitle: registerMode ? '为下一次相聚，留一个位置。' : '欢迎回来，继续你们的故事。',
        child: fields,
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
