import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../design.dart';
import '../models.dart';
import '../security/browser_security.dart' as browser;

class AccountSecurityPanel extends StatefulWidget {
  const AccountSecurityPanel({
    super.key,
    required this.api,
    required this.session,
    required this.onBusyChanged,
  });
  final ApiClient api;
  final Session session;
  final ValueChanged<bool> onBusyChanged;
  @override
  State<AccountSecurityPanel> createState() => _AccountSecurityPanelState();
}

class _AccountSecurityPanelState extends State<AccountSecurityPanel> {
  Map<String, dynamic>? _security;
  AppUser? _profile;
  List<Map<String, dynamic>> _providers = [];
  List<UserDevice> _devices = [];
  bool _loading = true, _busy = false, _cancelLink = false;
  String? _error, _notice;
  bool get _totpEnabled => _security?['totp_enabled'] == true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _cancelLink = true;
    browser.closeAuthorization();
    browser.cancelPasskey();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final data = await Future.wait<Object>([
        widget.api.accountSecurity(widget.session),
        widget.api.loginOptions(),
        widget.api.listDevices(widget.session),
        widget.api.accountProfile(widget.session),
      ]);
      if (!mounted) return;
      setState(() {
        _security = data[0] as Map<String, dynamic>;
        _providers =
            ((data[1] as Map<String, dynamic>)['providers'] as List<dynamic>)
                .cast<Map<String, dynamic>>();
        _devices = data[2] as List<UserDevice>;
        _profile = data[3] as AppUser;
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => _error = e is ApiException ? e.message : '安全设置加载失败，请重试。',
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _perform(Future<void> Function() work) async {
    if (_busy || _loading) return;
    widget.onBusyChanged(true);
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
    });
    try {
      await work();
    } catch (e) {
      if (mounted) {
        setState(
          () => _error = e is ApiException ? e.message : '操作未完成，请刷新后重试。',
        );
      }
    } finally {
      if (mounted) {
        setState(() => _busy = false);
        widget.onBusyChanged(false);
      }
    }
  }

  Future<Map<String, dynamic>?> _credentials({
    bool name = false,
    VoidCallback? onGesture,
  }) => showDialog<Map<String, dynamic>>(
    context: context,
    builder: (_) => _SecurityIdentityDialog(
      totp: _totpEnabled,
      askName: name,
      onGesture: onGesture,
    ),
  );

  Future<void> _setupTOTP() => _perform(() async {
    final credentials = await _credentials();
    if (credentials == null || !mounted) return;
    final setup = await widget.api.beginTOTP(
      widget.session,
      credentials['password'] as String,
    );
    if (!mounted) return;
    final code = await showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (_) => _TOTPSetupDialog(secret: setup['secret'] as String),
    );
    if (code == null || !mounted) return;
    final codes = await widget.api.confirmTOTP(
      widget.session,
      setup['challenge'] as String,
      code,
    );
    if (!mounted) return;
    // Reload using the replacement session before presenting the one-time recovery codes.
    await _load();
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        scrollable: true,
        title: const Text('保存恢复码'),
        content: SizedBox(
          width: 440,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text('验证器不可用时，可用恢复码登录。每个恢复码仅能使用一次，关闭后无法再次查看。'),
              const SizedBox(height: 16),
              SelectableText(
                codes.join('\n'),
                style: const TextStyle(fontFamily: 'monospace', height: 1.8),
              ),
              const SizedBox(height: 12),
              OutlinedButton.icon(
                onPressed: () => _copy(codes.join('\n')),
                icon: const Icon(Icons.copy_rounded, size: 18),
                label: const Text('复制恢复码'),
              ),
            ],
          ),
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('我已保存'),
          ),
        ],
      ),
    );
    if (mounted) setState(() => _notice = '动态验证码已启用，其他设备的旧会话已失效。');
  });

  Future<void> _disableTOTP() => _perform(() async {
    final credentials = await _credentials();
    if (credentials == null || !mounted) return;
    await widget.api.disableTOTP(widget.session, credentials);
    if (!mounted) return;
    await _load();
    if (mounted) setState(() => _notice = '动态验证码已关闭。');
  });

  Future<void> _addPasskey() => _perform(() async {
    if (!kIsWeb) {
      await _openBrowserSettings();
      return;
    }
    if (!browser.supportsBrowserSecurity) {
      throw const ApiException('此浏览器暂不支持 Passkey，请换用支持的浏览器。');
    }
    final credentials = await _credentials(name: true);
    if (credentials == null || !mounted) return;
    final start = await widget.api.beginPasskey(widget.session, credentials);
    if (!mounted) return;
    Map<String, dynamic> credential;
    try {
      credential = await browser.createPasskey(
        start['options'] as Map<String, dynamic>,
      );
    } catch (_) {
      throw const ApiException('设备验证已取消或未完成，请重试。');
    }
    if (!mounted) return;
    await widget.api.finishPasskey(
      widget.session,
      start['challenge'] as String,
      credential,
    );
    if (!mounted) return;
    await _load();
    if (mounted) setState(() => _notice = 'Passkey 已添加，当前账号可以继续使用。');
  });

  Future<void> _deletePasskey(Map<String, dynamic> key) => _perform(() async {
    final credentials = await _credentials();
    if (credentials == null || !mounted) return;
    await widget.api.removePasskey(
      widget.session,
      key['id'] as String,
      credentials,
    );
    if (!mounted) return;
    await _load();
    if (mounted) setState(() => _notice = 'Passkey 已移除。');
  });

  Future<void> _linkIdentity(Map<String, dynamic> provider) =>
      _perform(() async {
        if (!kIsWeb) {
          await _openBrowserSettings();
          return;
        }
        final credentials = await _credentials(
          onGesture: () => browser.beginAuthorization(),
        );
        if (credentials == null || !mounted) {
          browser.closeAuthorization();
          return;
        }
        _cancelLink = false;
        try {
          final start = await widget.api.beginIdentityLink(
            widget.session,
            provider['id'] as String,
            credentials,
          );
          if (!mounted || _cancelLink) return;
          browser.navigateAuthorization(start['url'] as String);
          setState(() => _notice = '请在平台授权窗口确认，完成后会自动返回这里。');
          final expires = DateTime.now().add(const Duration(minutes: 5));
          while (mounted && !_cancelLink && DateTime.now().isBefore(expires)) {
            await Future<void>.delayed(const Duration(seconds: 2));
            if (!mounted || _cancelLink) return;
            final result = await widget.api.pollIdentityLink(
              start['poll_token'] as String,
            );
            if (!mounted || _cancelLink) return;
            if (result['linked'] == true) {
              setState(() => _notice = '${provider['name']} 已关联。');
              return;
            }
            if (result['pending'] != true) {
              throw const ApiException('未能完成关联，请重新开始。');
            }
          }
          if (mounted && !_cancelLink) throw const ApiException('授权已超时，请重新开始。');
        } finally {
          browser.closeAuthorization();
        }
      });

  Future<void> _openBrowserSettings() async {
    if (!await launchUrl(
      Uri.parse(widget.api.baseUrl).resolve('/admin/account.html'),
      mode: LaunchMode.externalApplication,
    )) {
      throw const ApiException('无法打开浏览器，请使用网页版完成此操作。');
    }
    if (mounted) setState(() => _notice = '完成浏览器验证后，可返回此页刷新状态。');
  }

  Future<void> _revokeDevice(UserDevice device) => _perform(() async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('退出此设备'),
        content: Text('“${device.label}”将退出当前账号，需要重新登录才能继续使用。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('退出设备'),
          ),
        ],
      ),
    );
    if (confirm != true || !mounted) return;
    await widget.api.revokeDevice(widget.session, device.id);
    if (!mounted) return;
    await _load();
    if (mounted) setState(() => _notice = '该设备已退出。');
  });

  Future<void> _copy(String value) async {
    try {
      await Clipboard.setData(ClipboardData(text: value));
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('已复制，请妥善保管。')));
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('无法自动复制，请长按文字手动复制。')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final profile = _profile ?? widget.session.user;
    final colors = FrameColors.of(context);
    final disabled = _busy || _loading;
    final keys = (_security?['passkeys'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'ACCOUNT / SECURITY',
                    style: TextStyle(
                      fontSize: 10,
                      letterSpacing: 2,
                      color: colors.muted,
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    '账号与安全',
                    style: TextStyle(
                      fontSize: 29,
                      fontWeight: FontWeight.w600,
                      letterSpacing: -.8,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    '管理登录方式与已登录的设备。',
                    style: TextStyle(fontSize: 13, color: colors.muted),
                  ),
                ],
              ),
            ),
            IconButton(
              tooltip: '刷新安全设置',
              onPressed: disabled ? null : _load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        const SizedBox(height: 24),
        if (_loading) const LinearProgressIndicator(minHeight: 2),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 16),
            child: Semantics(
              liveRegion: true,
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      _error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                  if (!disabled)
                    TextButton(onPressed: _load, child: const Text('重试')),
                ],
              ),
            ),
          ),
        if (_notice != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 16),
            child: Semantics(
              liveRegion: true,
              child: Text(_notice!, style: TextStyle(color: colors.muted)),
            ),
          ),
        _section(
          Icons.person_outline_rounded,
          '当前账号',
          Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                profile.displayName,
                style: const TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 6),
              SelectableText(
                profile.email,
                style: TextStyle(color: colors.muted),
              ),
              const SizedBox(height: 12),
              Align(
                alignment: Alignment.centerLeft,
                child: FramePill(
                  profile.emailVerified ? '邮箱已验证' : '邮箱待验证',
                  icon: profile.emailVerified
                      ? Icons.verified_user_outlined
                      : Icons.mail_outline_rounded,
                ),
              ),
            ],
          ),
        ),
        if (_security != null) ...[
          _section(
            Icons.shield_outlined,
            '动态验证码',
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                FramePill(
                  _totpEnabled ? '已启用' : '未启用',
                  icon: _totpEnabled
                      ? Icons.check_rounded
                      : Icons.lock_outline_rounded,
                ),
                const SizedBox(height: 12),
                Text(
                  _totpEnabled
                      ? '密码登录后还需验证动态验证码。剩余恢复码：${_security!['recovery_codes_remaining']} 个。'
                      : '使用身份验证器生成动态验证码，为密码登录增加一次验证。',
                  style: TextStyle(color: colors.muted),
                ),
                const SizedBox(height: 16),
                if (_totpEnabled)
                  OutlinedButton(
                    onPressed: disabled ? null : _disableTOTP,
                    child: const Text('关闭动态验证码'),
                  )
                else if (_security!['totp_enrollment'] == true)
                  FilledButton.icon(
                    onPressed: disabled ? null : _setupTOTP,
                    icon: const Icon(Icons.add_rounded, size: 18),
                    label: const Text('配置身份验证器'),
                  )
                else
                  Text('本站暂未开放配置。', style: TextStyle(color: colors.muted)),
              ],
            ),
          ),
          _section(
            Icons.fingerprint_rounded,
            'Passkey',
            Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  '使用设备的指纹、面容或屏幕锁登录。',
                  style: TextStyle(color: colors.muted),
                ),
                if (keys.isEmpty)
                  const Padding(
                    padding: EdgeInsets.symmetric(vertical: 14),
                    child: Text('尚未添加 Passkey'),
                  ),
                for (final key in keys)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(key['name'] as String),
                    subtitle: Text(_date((key['created_at'] as num).toInt())),
                    trailing: TextButton(
                      onPressed: disabled ? null : () => _deletePasskey(key),
                      child: const Text('移除'),
                    ),
                  ),
                if (_security!['passkeys_enabled'] == true)
                  Align(
                    alignment: Alignment.centerLeft,
                    child: OutlinedButton.icon(
                      onPressed: disabled ? null : _addPasskey,
                      icon: const Icon(Icons.add_rounded, size: 18),
                      label: Text(kIsWeb ? '添加 Passkey' : '在浏览器中添加'),
                    ),
                  )
                else
                  Text(
                    '本站暂未启用 Passkey，可继续使用密码登录。',
                    style: TextStyle(fontSize: 12, color: colors.muted),
                  ),
              ],
            ),
          ),
          _section(
            Icons.link_rounded,
            '第三方账号',
            Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  _providers.isEmpty
                      ? '本站暂未开放第三方账号关联。'
                      : '关联后可使用相应平台登录；仅在授权时打开平台验证窗口。',
                  style: TextStyle(color: colors.muted),
                ),
                if (_providers.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  Wrap(
                    spacing: 12,
                    runSpacing: 10,
                    children: [
                      for (final provider in _providers)
                        OutlinedButton(
                          onPressed: disabled
                              ? null
                              : () => _linkIdentity(provider),
                          child: Text('关联 ${provider['name']}'),
                        ),
                    ],
                  ),
                  if (_busy && _notice?.startsWith('请在平台授权窗口') == true)
                    TextButton(
                      onPressed: () {
                        _cancelLink = true;
                        browser.closeAuthorization();
                        setState(() => _notice = '已取消等待授权。');
                      },
                      child: const Text('取消等待'),
                    ),
                ],
              ],
            ),
          ),
          _section(
            Icons.devices_rounded,
            '登录设备',
            Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  '查看当前账号的登录记录，退出不再使用的设备。',
                  style: TextStyle(color: colors.muted),
                ),
                const SizedBox(height: 8),
                for (final device in _devices)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(
                      device.label,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    subtitle: Text(
                      '${device.platform} · ${_date(device.lastSeen)}',
                      style: const TextStyle(fontSize: 11),
                    ),
                    trailing: device.current
                        ? const FramePill('当前设备')
                        : TextButton(
                            onPressed: disabled
                                ? null
                                : () => _revokeDevice(device),
                            child: const Text('退出'),
                          ),
                  ),
                if (_devices.isEmpty)
                  const Padding(
                    padding: EdgeInsets.only(top: 8),
                    child: Text('暂无登录设备记录。'),
                  ),
              ],
            ),
          ),
        ],
      ],
    );
  }

  Widget _section(IconData icon, String title, Widget content) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: Card(
      child: Padding(
        padding: const EdgeInsets.all(22),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Icon(icon, size: 21),
                const SizedBox(width: 11),
                Text(
                  title,
                  style: const TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 20),
            content,
          ],
        ),
      ),
    ),
  );

  String _date(int milliseconds) =>
      DateTime.fromMillisecondsSinceEpoch(milliseconds)
          .toLocal()
          .toString()
          .split('.')
          .first;
}

class _SecurityIdentityDialog extends StatefulWidget {
  const _SecurityIdentityDialog({
    required this.totp,
    this.askName = false,
    this.onGesture,
  });
  final bool totp, askName;
  final VoidCallback? onGesture;
  @override
  State<_SecurityIdentityDialog> createState() =>
      _SecurityIdentityDialogState();
}

class _SecurityIdentityDialogState extends State<_SecurityIdentityDialog> {
  final _password = TextEditingController(),
      _code = TextEditingController(),
      _name = TextEditingController();
  String? _error;
  @override
  void dispose() {
    for (final c in [_password, _code, _name]) {
      c.clear();
      c.dispose();
    }
    super.dispose();
  }

  void _submit() {
    if (_password.text.isEmpty ||
        (widget.totp && _code.text.trim().isEmpty) ||
        (widget.askName && _name.text.trim().isEmpty)) {
      setState(() => _error = '请填写完整的验证信息。');
      return;
    }
    try {
      widget.onGesture?.call();
    } catch (_) {
      setState(() => _error = '无法打开授权窗口，请允许本站弹出窗口后重试。');
      return;
    }
    Navigator.pop(context, <String, dynamic>{
      'password': _password.text,
      if (widget.totp) 'code': _code.text.trim(),
      if (widget.askName) 'name': _name.text.trim(),
    });
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    scrollable: true,
    title: const Text('验证账号身份'),
    content: SizedBox(
      width: 430,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('修改安全设置前，请验证当前账号。'),
          const SizedBox(height: 16),
          TextField(
            controller: _password,
            autofocus: true,
            obscureText: true,
            autocorrect: false,
            enableSuggestions: false,
            autofillHints: const [AutofillHints.password],
            decoration: const InputDecoration(labelText: '当前密码'),
          ),
          if (widget.totp) ...[
            const SizedBox(height: 14),
            TextField(
              controller: _code,
              autofillHints: const [AutofillHints.oneTimeCode],
              decoration: const InputDecoration(labelText: '动态验证码或恢复码'),
            ),
          ],
          if (widget.askName) ...[
            const SizedBox(height: 14),
            TextField(
              controller: _name,
              maxLength: 80,
              decoration: const InputDecoration(labelText: '设备名称'),
            ),
          ],
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('取消'),
      ),
      FilledButton(onPressed: _submit, child: const Text('继续')),
    ],
  );
}

class _TOTPSetupDialog extends StatefulWidget {
  const _TOTPSetupDialog({required this.secret});
  final String secret;
  @override
  State<_TOTPSetupDialog> createState() => _TOTPSetupDialogState();
}

class _TOTPSetupDialogState extends State<_TOTPSetupDialog> {
  final _code = TextEditingController();
  String? _error;
  @override
  void dispose() {
    _code.clear();
    _code.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    scrollable: true,
    title: const Text('配置身份验证器'),
    content: SizedBox(
      width: 440,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('在身份验证器中选择手动输入密钥，类型选择“基于时间”。'),
          const SizedBox(height: 16),
          SelectableText(
            widget.secret,
            style: const TextStyle(fontFamily: 'monospace', fontSize: 17),
          ),
          const SizedBox(height: 18),
          TextField(
            controller: _code,
            keyboardType: TextInputType.number,
            maxLength: 6,
            autofillHints: const [AutofillHints.oneTimeCode],
            decoration: const InputDecoration(labelText: '六位动态验证码'),
          ),
          if (_error != null)
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('取消'),
      ),
      FilledButton(
        onPressed: () {
          final code = _code.text.trim();
          if (!RegExp(r'^\d{6}$').hasMatch(code)) {
            setState(() => _error = '请输入六位动态验证码。');
            return;
          }
          Navigator.pop(context, code);
        },
        child: const Text('完成配置'),
      ),
    ],
  );
}
