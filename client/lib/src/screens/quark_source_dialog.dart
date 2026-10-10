import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../models.dart';

class QuarkSourceDialog extends StatefulWidget {
  const QuarkSourceDialog({
    super.key,
    required this.api,
    required this.session,
    this.source,
  });

  final ApiClient api;
  final Session session;
  final MediaSource? source;

  @override
  State<QuarkSourceDialog> createState() => _QuarkSourceDialogState();
}

class _QuarkSourceDialogState extends State<QuarkSourceDialog> {
  final _name = TextEditingController(text: '我的夸克网盘');
  final _cookie = TextEditingController();
  bool _saving = false;
  bool _showCookie = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _name.text = widget.source?.name ?? '我的夸克网盘';
  }

  @override
  void dispose() {
    _cookie.clear();
    _cookie.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _openLogin() async {
    try {
      if (!await launchUrl(
        Uri.parse('https://pan.quark.cn/'),
        mode: LaunchMode.externalApplication,
      )) {
        throw StateError('open failed');
      }
    } catch (_) {
      if (mounted) {
        setState(() => _error = '请在浏览器打开 pan.quark.cn 登录夸克网盘。');
      }
    }
  }

  Future<void> _save() async {
    if (_saving) return;
    if (_name.text.trim().isEmpty || _cookie.text.trim().isEmpty) {
      setState(() => _error = '请填写名称并粘贴登录 Cookie。');
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final source = await widget.api.saveQuarkSource(
        session: widget.session,
        name: _name.text.trim(),
        cookie: _cookie.text.trim(),
        sourceId: widget.source?.id,
      );
      if (!mounted) return;
      _cookie.clear();
      Navigator.of(context).pop(source);
    } catch (exception) {
      if (mounted) {
        setState(() {
          _saving = false;
          _error = exception is ApiException
              ? exception.message
              : '连接失败，请检查网络后重试。';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_saving,
    child: AlertDialog(
      scrollable: true,
      title: Text(widget.source == null ? '连接夸克网盘' : '更新夸克登录'),
      content: SizedBox(
        width: 480,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text('选一部网盘里的电影，邀请好友来房间一起看。'),
            const SizedBox(height: 16),
            OutlinedButton.icon(
              onPressed: _saving ? null : _openLogin,
              icon: const Icon(Icons.open_in_new_rounded),
              label: const Text('打开夸克网页登录'),
            ),
            const SizedBox(height: 8),
            const ExpansionTile(
              tilePadding: EdgeInsets.zero,
              title: Text('如何获取登录 Cookie？'),
              children: [
                Padding(
                  padding: EdgeInsets.only(bottom: 16),
                  child: Text(
                    '1. 在电脑浏览器登录夸克网盘。\n'
                    '2. 按 F12 打开开发者工具，选择“网络”，刷新网盘列表。\n'
                    '3. 选中 file/sort 请求，复制“请求标头”里的 Cookie 值，粘贴到下方。\n\n'
                    'Cookie 等同于登录凭据，只在此连接窗口填写。好友无需提供夸克账号。',
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _name,
              enabled: !_saving && widget.source == null,
              maxLength: 64,
              decoration: const InputDecoration(labelText: '网盘名称'),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _cookie,
              enabled: !_saving,
              obscureText: !_showCookie,
              autocorrect: false,
              enableSuggestions: false,
              maxLength: 16384,
              decoration: InputDecoration(
                labelText: '登录 Cookie',
                hintText: '粘贴完整的 Cookie 值',
                counterText: '',
                suffixIcon: IconButton(
                  onPressed: () => setState(() => _showCookie = !_showCookie),
                  tooltip: _showCookie ? '隐藏登录凭据' : '显示登录凭据',
                  icon: Icon(
                    _showCookie ? Icons.visibility_off : Icons.visibility,
                  ),
                ),
              ),
              onSubmitted: (_) => _save(),
            ),
            const SizedBox(height: 14),
            Text(
              '登录凭据加密保存，不会分享给房间成员。连接失效时可在这里重新登录。',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
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
        FilledButton.icon(
          onPressed: _saving ? null : _save,
          icon: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.cloud_done_outlined),
          label: Text(_saving ? '正在验证登录…' : '验证并连接'),
        ),
      ],
    ),
  );
}
