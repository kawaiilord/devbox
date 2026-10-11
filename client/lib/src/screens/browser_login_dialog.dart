import 'dart:async';

import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../models.dart';

class BrowserLoginDialog extends StatefulWidget {
  const BrowserLoginDialog({super.key, required this.api});
  final ApiClient api;
  @override
  State<BrowserLoginDialog> createState() => _BrowserLoginDialogState();
}

class _BrowserLoginDialogState extends State<BrowserLoginDialog> {
  Timer? _timer;
  Uri? _uri;
  String? _error, _token;
  bool _polling = false;
  int _attempts = 0;
  @override
  void initState() {
    super.initState();
    _start();
  }

  Future<void> _start() async {
    try {
      final data = await widget.api.startBrowserLogin();
      if (!mounted) return;
      setState(() {
        _uri = Uri.parse(widget.api.baseUrl).resolve(data['url'] as String);
        _token = data['poll_token'] as String;
      });
      _timer = Timer.periodic(const Duration(seconds: 2), (_) => _poll());
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    }
  }

  Future<void> _poll() async {
    if (_polling || _token == null) return;
    if (++_attempts > 150) {
      _timer?.cancel();
      if (mounted) setState(() => _error = '登录请求已过期，请关闭后重试。');
      return;
    }
    _polling = true;
    try {
      final session = await widget.api.exchangeBrowserLogin(_token!);
      if (session != null && mounted) {
        _timer?.cancel();
        Navigator.of(context).pop(session);
      }
    } catch (e) {
      _timer?.cancel();
      if (mounted) setState(() => _error = '$e');
    } finally {
      _polling = false;
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: const Text('安全登录'),
    content: SizedBox(
      width: 420,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('在安全中心使用邮箱验证码、第三方账号或 Passkey 登录，然后选择“使用此账号登录客户端”。'),
          const SizedBox(height: 20),
          if (_uri == null && _error == null) const LinearProgressIndicator(),
          if (_error != null)
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          if (_uri != null && _error == null) ...[
            FilledButton.icon(
              onPressed: () async {
                try {
                  if (!await launchUrl(
                    _uri!,
                    mode: LaunchMode.externalApplication,
                  )) {
                    if (mounted) setState(() => _error = '无法打开浏览器，请检查浏览器设置。');
                  }
                } catch (e) {
                  if (mounted) setState(() => _error = '$e');
                }
              },
              icon: const Icon(Icons.open_in_new),
              label: const Text('打开安全中心'),
            ),
            const SizedBox(height: 16),
            const Text('等待浏览器完成登录…'),
          ],
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.of(context).pop(),
        child: const Text('取消'),
      ),
    ],
  );
}
