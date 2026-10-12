import 'package:flutter/material.dart';

import '../update_service.dart';

class UpdatePage extends StatefulWidget {
  const UpdatePage({super.key});
  @override
  State<UpdatePage> createState() => _UpdatePageState();
}

class _UpdatePageState extends State<UpdatePage> {
  final _service = UpdateService();
  UpdateManifest? _manifest;
  bool _busy = false;
  String? _message;

  @override
  void dispose() {
    _service.close();
    super.dispose();
  }

  Future<void> _check() async {
    setState(() {
      _busy = true;
      _message = null;
    });
    try {
      final manifest = await _service.check();
      if (!mounted) return;
      setState(() {
        _manifest = manifest;
        _message = manifest == null
            ? (_service.configured ? '当前已是最新版本。' : '更新服务未配置。')
            : '发现版本 ${manifest.version}，签名有效。';
      });
    } catch (error) {
      if (mounted) setState(() => _message = error.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _download() async {
    final manifest = _manifest;
    if (manifest == null) return;
    setState(() {
      _busy = true;
      _message = '正在下载并验证…';
    });
    try {
      final path = await _service.download(manifest);
      if (mounted) setState(() => _message = '已验证并保存至：$path');
    } catch (error) {
      if (mounted) setState(() => _message = error.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('安全更新')),
    body: ListView(
      padding: const EdgeInsets.all(20),
      children: [
        const Card(
          child: Padding(
            padding: EdgeInsets.all(16),
            child: Text(
              '更新清单使用 Ed25519 验签；安装包必须同时通过平台、大小和 SHA-256 校验，才会保存为 verified 文件。',
            ),
          ),
        ),
        const SizedBox(height: 12),
        FilledButton.icon(
          onPressed: _busy ? null : _check,
          icon: const Icon(Icons.security_update_good),
          label: const Text('检查更新'),
        ),
        if (_manifest != null)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: FilledButton.icon(
              onPressed: _busy ? null : _download,
              icon: const Icon(Icons.download),
              label: Text('下载 ${_manifest!.version}'),
            ),
          ),
        if (_busy)
          const Padding(
            padding: EdgeInsets.all(16),
            child: LinearProgressIndicator(),
          ),
        if (_message != null)
          Padding(
            padding: const EdgeInsets.all(16),
            child: SelectableText(_message!),
          ),
      ],
    ),
  );
}
