import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class LibraryConnectionDialog extends StatefulWidget {
  const LibraryConnectionDialog({
    super.key,
    required this.api,
    required this.session,
    required this.emby,
  });
  final ApiClient api;
  final Session session;
  final bool emby;
  @override
  State<LibraryConnectionDialog> createState() =>
      _LibraryConnectionDialogState();
}

class _LibraryConnectionDialogState extends State<LibraryConnectionDialog> {
  final _form = GlobalKey<FormState>();
  late final _name = TextEditingController(
    text: widget.emby ? '我的 Emby' : '我的 WebDAV',
  );
  final _base = TextEditingController(),
      _username = TextEditingController(),
      _password = TextEditingController();
  String? _error;
  bool _busy = false;
  @override
  void dispose() {
    for (final field in [_name, _base, _username, _password]) {
      field.clear();
      field.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    if (_busy || !_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final source = widget.emby
          ? await widget.api.createEmbySource(
              session: widget.session,
              name: _name.text.trim(),
              baseUrl: _base.text.trim(),
              username: _username.text,
              password: _password.text,
            )
          : await widget.api.createWebDAVSource(
              session: widget.session,
              name: _name.text.trim(),
              baseUrl: _base.text.trim(),
              username: _username.text,
              password: _password.text,
            );
      if (mounted) Navigator.pop(context, source);
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e is ApiException ? e.message : '连接失败，请检查服务地址和账号。';
          _busy = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_busy,
    child: AlertDialog(
      scrollable: true,
      insetPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
      title: Text(widget.emby ? '连接 Emby' : '连接 WebDAV'),
      content: SizedBox(
        width: 460,
        child: Form(
          key: _form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextFormField(
                controller: _name,
                enabled: !_busy,
                maxLength: 64,
                decoration: const InputDecoration(labelText: '连接名称'),
                validator: (v) =>
                    v == null || v.trim().isEmpty ? '请填写名称' : null,
              ),
              const SizedBox(height: 12),
              TextFormField(
                controller: _base,
                enabled: !_busy,
                keyboardType: TextInputType.url,
                decoration: InputDecoration(
                  labelText: widget.emby ? 'Emby 服务地址' : 'WebDAV 地址',
                  hintText: widget.emby
                      ? 'https://emby.example.com/emby/'
                      : 'https://dav.example.com/',
                ),
                validator: (v) {
                  final u = Uri.tryParse(v?.trim() ?? '');
                  return u == null ||
                          u.host.isEmpty ||
                          !['http', 'https'].contains(u.scheme)
                      ? '请填写完整的服务地址'
                      : null;
                },
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _username,
                enabled: !_busy,
                decoration: const InputDecoration(labelText: '用户名'),
                autofillHints: const [AutofillHints.username],
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _password,
                enabled: !_busy,
                obscureText: true,
                autocorrect: false,
                enableSuggestions: false,
                decoration: const InputDecoration(labelText: '密码'),
                onFieldSubmitted: (_) => _save(),
              ),
              const SizedBox(height: 14),
              Text(
                widget.emby
                    ? '密码仅用于本次登录，连接成功后保存加密的会话令牌。'
                    : '账号信息加密保存，用于浏览和播放你的文件。',
                style: Theme.of(context).textTheme.bodySmall,
              ),
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.only(top: 14),
                  child: Semantics(
                    liveRegion: true,
                    child: Text(
                      _error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(_busy ? '连接中…' : '连接并保存'),
        ),
      ],
    ),
  );
}
