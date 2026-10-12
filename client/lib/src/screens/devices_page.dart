import 'package:flutter/material.dart';

import 'workspace_section.dart';

import '../api/api_client.dart';
import '../models.dart';

class DevicesPage extends StatefulWidget {
  const DevicesPage({super.key, required this.api, required this.session});
  final ApiClient api;
  final Session session;

  @override
  State<DevicesPage> createState() => _DevicesPageState();
}

class _DevicesPageState extends State<DevicesPage> {
  List<UserDevice> _devices = [];
  bool _busy = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final devices = await widget.api.listDevices(widget.session);
      if (mounted) setState(() => _devices = devices);
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _revoke(UserDevice device) async {
    if (_busy || device.current) return;
    final accepted = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('撤销设备登录'),
        content: Text('“${device.label}”将退出登录，再次使用时需要重新验证。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('撤销登录'),
          ),
        ],
      ),
    );
    if (!mounted || accepted != true) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.revokeDevice(widget.session, device.id);
      if (mounted) await _load();
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => WorkspaceSection(
    title: const Text('设备管理'),
    actions: [
      IconButton(
        tooltip: '刷新',
        onPressed: _busy ? null : _load,
        icon: const Icon(Icons.refresh_rounded),
      ),
    ],
    body: Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 1180),
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            Text('登录设备', style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 8),
            const Text('查看最近使用的设备，撤销不再使用的登录。'),
            const SizedBox(height: 20),
            if (_busy) const LinearProgressIndicator(),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            if (!_busy && _devices.isEmpty && _error == null)
              const Text('没有可管理的设备。'),
            for (final device in _devices)
              Card(
                child: ListTile(
                  leading: Icon(
                    device.current
                        ? Icons.devices_rounded
                        : Icons.computer_rounded,
                  ),
                  title: Text(device.label),
                  subtitle: Text(
                    '${device.platform} · 最近使用 '
                    '${DateTime.fromMillisecondsSinceEpoch(device.lastSeen).toLocal()}',
                  ),
                  trailing: device.current
                      ? const Chip(label: Text('当前设备'))
                      : IconButton(
                          tooltip: '撤销设备',
                          onPressed: _busy ? null : () => _revoke(device),
                          icon: const Icon(Icons.link_off_rounded),
                        ),
                ),
              ),
          ],
        ),
      ),
    ),
  );
}
