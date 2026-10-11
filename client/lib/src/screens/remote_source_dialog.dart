import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class RemoteSourceDialog extends StatefulWidget {
  const RemoteSourceDialog({
    super.key,
    required this.api,
    required this.session,
  });
  final ApiClient api;
  final Session session;
  @override
  State<RemoteSourceDialog> createState() => _RemoteSourceDialogState();
}

class _RemoteSourceDialogState extends State<RemoteSourceDialog> {
  final _name = TextEditingController(text: '远程媒体库'),
      _host = TextEditingController(),
      _user = TextEditingController(),
      _password = TextEditingController(),
      _cookie = TextEditingController();
  List<Map<String, dynamic>> _providers = [];
  String? _instance, _kind, _error;
  bool _loading = true, _busy = false;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final data = await widget.api.remoteProviders(widget.session);
      if (mounted) {
        setState(() {
          _providers = (data['providers'] as List<dynamic>)
              .cast<Map<String, dynamic>>();
          if (_providers.isNotEmpty) {
            _instance = _providers.first['id'] as String;
            _kind =
                (_providers.first['types'] as List<dynamic>).first as String;
          }
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  void dispose() {
    for (final c in [_name, _host, _user, _password, _cookie]) {
      c.clear();
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final source = await widget.api.createRemoteSource(widget.session, {
        'instance': _instance,
        'kind': _kind,
        'name': _name.text.trim(),
        'base_url': _host.text.trim(),
        'username': _user.text,
        'password': _password.text,
        'cookie': _cookie.text,
      });
      if (mounted) Navigator.pop(context, source);
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final types =
        _providers.where((p) => p['id'] == _instance).firstOrNull?['types']
            as List<dynamic>? ??
        [];
    return AlertDialog(
      scrollable: true,
      title: const Text('远程媒体服务'),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (_loading) const LinearProgressIndicator(),
            if (!_loading && _providers.isEmpty)
              const Text('管理员尚未配置远程 Provider。'),
            if (_providers.isNotEmpty) ...[
              DropdownButtonFormField<String>(
                initialValue: _instance,
                decoration: const InputDecoration(labelText: '远程服务'),
                items: _providers
                    .map(
                      (p) => DropdownMenuItem(
                        value: p['id'] as String,
                        child: Text(p['name'] as String),
                      ),
                    )
                    .toList(),
                onChanged: _busy
                    ? null
                    : (v) => setState(() {
                        _instance = v;
                        _kind =
                            (_providers.firstWhere((p) => p['id'] == v)['types']
                                        as List<dynamic>)
                                    .first
                                as String;
                      }),
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<String>(
                key: ValueKey(_instance),
                initialValue: _kind,
                decoration: const InputDecoration(labelText: '媒体类型'),
                items: types
                    .map(
                      (t) =>
                          DropdownMenuItem(value: t as String, child: Text(t)),
                    )
                    .toList(),
                onChanged: _busy ? null : (v) => setState(() => _kind = v),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _name,
                decoration: const InputDecoration(labelText: '来源名称'),
              ),
              if (_kind == 'bilibili')
                TextField(
                  controller: _cookie,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: 'B站 Cookie（可选）'),
                )
              else ...[
                TextField(
                  controller: _host,
                  decoration: const InputDecoration(
                    labelText: 'Alist / Emby 服务地址',
                  ),
                ),
                TextField(
                  controller: _user,
                  decoration: const InputDecoration(labelText: '账号'),
                ),
                TextField(
                  controller: _password,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: '密码'),
                ),
              ],
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
          onPressed: _busy ? null : () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        FilledButton(
          onPressed: _busy || _instance == null ? null : _save,
          child: Text(_busy ? '连接中…' : '连接'),
        ),
      ],
    );
  }
}
