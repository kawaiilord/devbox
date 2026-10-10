import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class ProviderSourceDialog extends StatefulWidget {
  const ProviderSourceDialog({
    super.key,
    required this.api,
    required this.session,
    this.source,
  });
  final ApiClient api;
  final Session session;
  final MediaSource? source;
  @override
  State<ProviderSourceDialog> createState() => _ProviderSourceDialogState();
}

class _ProviderSourceDialogState extends State<ProviderSourceDialog> {
  final _name = TextEditingController();
  final _base = TextEditingController();
  final _username = TextEditingController();
  final _password = TextEditingController();
  final _cookie = TextEditingController();
  final _otp = TextEditingController();
  final _dav = TextEditingController();
  List<ProviderDescriptor> _providers = [];
  String _provider = 'bilibili';
  String? _error;
  bool _loading = true, _saving = false;
  bool get _isPlatform => platformSourceTypes.contains(_provider);

  @override
  void initState() {
    super.initState();
    _provider = widget.source?.type ?? 'bilibili';
    _name.text = widget.source?.name ?? mediaSourceLabel(_provider);
    _base.text = widget.source?.baseUrl ?? '';
    _load();
  }

  Future<void> _load() async {
    try {
      final providers = await widget.api.providerCatalog();
      if (!mounted) {
        return;
      }
      setState(() {
        _providers = providers;
        _loading = false;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = '连接类型加载失败，请重试。';
        });
      }
    }
  }

  @override
  void dispose() {
    for (final controller in [
      _name,
      _base,
      _username,
      _password,
      _cookie,
      _otp,
      _dav,
    ]) {
      controller.clear();
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    if (_saving) {
      return;
    }
    if (_name.text.trim().isEmpty ||
        (!_isPlatform && _base.text.trim().isEmpty)) {
      setState(() => _error = '请填写名称和服务地址。');
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final values = <String, dynamic>{
        'provider': _provider,
        'name': _name.text.trim(),
        if (_isPlatform) 'cookie': _cookie.text.trim(),
        if (!_isPlatform) ...{
          'base_url': _base.text.trim(),
          'username': _username.text.trim(),
          'password': _provider == 'truenas' ? '' : _password.text,
          'token': _provider == 'truenas' ? _password.text.trim() : '',
          'otp': _otp.text.trim(),
          'webdav_url': _dav.text.trim(),
        },
      };
      final source = await widget.api.saveProviderSource(
        widget.session,
        values,
        sourceId: widget.source?.id,
      );
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(source);
    } catch (e) {
      if (mounted) {
        setState(() {
          _saving = false;
          _error = e is ApiException ? e.message : '连接失败，请检查网络后重试。';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final available =
        _providers.where((p) => p.id == _provider).firstOrNull?.available ??
        false;
    return PopScope(
      canPop: !_saving,
      child: AlertDialog(
        scrollable: true,
        title: Text(widget.source == null ? '连接平台或 NAS' : '更新媒体源连接'),
        content: SizedBox(
          width: 500,
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (_providers.isNotEmpty)
                      DropdownButtonFormField<String>(
                        initialValue: _provider,
                        isExpanded: true,
                        decoration: const InputDecoration(labelText: '媒体类型'),
                        items: _providers
                            .map(
                              (p) => DropdownMenuItem(
                                value: p.id,
                                child: Text(p.name),
                              ),
                            )
                            .toList(),
                        onChanged: _saving || widget.source != null
                            ? null
                            : (value) {
                                if (value != null) {
                                  setState(() {
                                    _provider = value;
                                    _name.text = mediaSourceLabel(value);
                                    _error = null;
                                    _cookie.clear();
                                    _password.clear();
                                    _otp.clear();
                                  });
                                }
                              },
                      ),
                    const SizedBox(height: 14),
                    TextField(
                      controller: _name,
                      enabled: !_saving,
                      decoration: const InputDecoration(labelText: '连接名称'),
                      maxLength: 64,
                    ),
                    if (_isPlatform) ...[
                      const Text('连接后可粘贴视频或播放列表链接，选择影片加入房间。'),
                      const SizedBox(height: 14),
                      TextField(
                        controller: _cookie,
                        obscureText: true,
                        enabled: !_saving,
                        autocorrect: false,
                        enableSuggestions: false,
                        decoration: const InputDecoration(
                          labelText: '登录 Cookie（可选）',
                          helperText: '留空以未登录身份访问公开视频。',
                          helperMaxLines: 2,
                        ),
                      ),
                      const SizedBox(height: 10),
                      const Text(
                        '需要登录的视频，请先在平台官网登录，再从浏览器请求标头复制 Cookie。账号权限和平台验证仍由平台决定。',
                      ),
                    ] else ...[
                      TextField(
                        controller: _base,
                        enabled: !_saving && widget.source == null,
                        decoration: const InputDecoration(
                          labelText: 'NAS / 服务地址',
                          hintText: 'https://nas.example.com:5001/',
                        ),
                      ),
                      if (_provider != 'truenas') ...[
                        const SizedBox(height: 12),
                        TextField(
                          controller: _username,
                          enabled: !_saving,
                          decoration: const InputDecoration(labelText: '用户名'),
                        ),
                      ],
                      const SizedBox(height: 12),
                      TextField(
                        controller: _password,
                        enabled: !_saving,
                        obscureText: true,
                        autocorrect: false,
                        enableSuggestions: false,
                        decoration: InputDecoration(
                          labelText: _provider == 'truenas'
                              ? 'API Key'
                              : _provider == 'nextcloud'
                              ? '应用密码'
                              : '密码',
                        ),
                      ),
                      if (_provider == 'synology' || _provider == 'fnos') ...[
                        const SizedBox(height: 12),
                        TextField(
                          controller: _otp,
                          enabled: !_saving,
                          keyboardType: TextInputType.number,
                          decoration: const InputDecoration(
                            labelText: '验证器验证码（如已启用）',
                          ),
                        ),
                      ],
                      if (_provider == 'fnos') ...[
                        const SizedBox(height: 12),
                        TextField(
                          controller: _dav,
                          enabled: !_saving,
                          decoration: const InputDecoration(
                            labelText: 'WebDAV 地址（可选）',
                            helperText: '自动发现失败时填写；请先在飞牛启用文件服务。',
                            helperMaxLines: 2,
                          ),
                        ),
                      ],
                      if (_provider == 'truenas') ...[
                        const SizedBox(height: 10),
                        const Text(
                          '连接支持 REST API v2 的 TrueNAS 服务，仅浏览 /mnt 下的存储。',
                        ),
                      ],
                    ],
                    const SizedBox(height: 14),
                    Text(
                      '登录凭据加密保存，不会发给房间成员。',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                    if (!available) ...[
                      const SizedBox(height: 12),
                      const Text('当前服务器尚未启用此连接，请联系管理员配置媒体服务。'),
                    ],
                    if (_error != null) ...[
                      const SizedBox(height: 12),
                      Text(
                        _error!,
                        style: TextStyle(
                          color: Theme.of(context).colorScheme.error,
                        ),
                      ),
                    ],
                  ],
                ),
        ),
        actions: [
          TextButton(
            onPressed: _saving ? null : () => Navigator.of(context).pop(),
            child: const Text('取消'),
          ),
          if (_providers.isEmpty && !_loading)
            TextButton(onPressed: _load, child: const Text('重试')),
          FilledButton(
            onPressed: _saving || _loading || !available ? null : _save,
            child: Text(
              _saving
                  ? '正在连接…'
                  : _isPlatform
                  ? '保存连接'
                  : '验证并连接',
            ),
          ),
        ],
      ),
    );
  }
}
