import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../room_controller.dart';

class RTMPDialog extends StatefulWidget {
  const RTMPDialog({super.key, required this.controller});
  final RoomController controller;
  @override
  State<RTMPDialog> createState() => _RTMPDialogState();
}

class _RTMPDialogState extends State<RTMPDialog> {
  Map<String, dynamic>? _data;
  bool _busy = false;
  String? _error;
  @override
  void initState() {
    super.initState();
    _request('GET');
  }

  Future<void> _request(String method) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final c = widget.controller;
      final data = await c.api.roomRTMP(c.session, c.room.code, method);
      if (mounted) setState(() => _data = data);
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    scrollable: true,
    title: const Text('房间直播'),
    content: SizedBox(
      width: 500,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('用 OBS 或其他推流软件向房间直播。生成密钥后，直播源会自动加入当前片单。'),
          const SizedBox(height: 16),
          if (_busy) const LinearProgressIndicator(),
          if (_error != null)
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          if (_data?['publish_url'] != null) ...[
            const Text('推流服务器'),
            SelectableText(_data!['publish_url'] as String),
            const SizedBox(height: 12),
            const Text('推流密钥（仅本次显示，请勿分享）'),
            SelectableText(_data!['stream_key'] as String),
            TextButton.icon(
              onPressed: () => Clipboard.setData(
                ClipboardData(text: _data!['stream_key'] as String),
              ),
              icon: const Icon(Icons.copy),
              label: const Text('复制密钥'),
            ),
          ],
          if ((_data?['expires_at'] as num? ?? 0) > 0)
            Text(
              '到期：${DateTime.fromMillisecondsSinceEpoch((_data!['expires_at'] as num).toInt()).toLocal()}',
            ),
          if (_data?['revoked'] == true) const Text('推流密钥已撤销。'),
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: _busy ? null : () => Navigator.pop(context),
        child: const Text('关闭'),
      ),
      TextButton(
        onPressed: _busy ? null : () => _request('DELETE'),
        child: const Text('撤销密钥'),
      ),
      FilledButton(
        onPressed: _busy ? null : () => _request('POST'),
        child: const Text('生成新密钥'),
      ),
    ],
  );
}
