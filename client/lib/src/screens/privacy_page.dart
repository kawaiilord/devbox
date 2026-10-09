import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class PrivacyPage extends StatefulWidget {
  const PrivacyPage({super.key, required this.api, required this.session});

  final ApiClient api;
  final Session session;

  @override
  State<PrivacyPage> createState() => _PrivacyPageState();
}

class _PrivacyPageState extends State<PrivacyPage> {
  PrivacySettings? _settings;
  List<AppUser> _blockedUsers = const [];
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
      final values = await Future.wait<Object>([
        widget.api.privacy(widget.session),
        widget.api.blockedUsers(widget.session),
      ]);
      if (!mounted) return;
      setState(() {
        _settings = values[0] as PrivacySettings;
        _blockedUsers = values[1] as List<AppUser>;
      });
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() async {
    final settings = _settings;
    if (settings == null) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final saved = await widget.api.updatePrivacy(widget.session, settings);
      if (!mounted) return;
      setState(() => _settings = saved);
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('隐私设置已保存。')));
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _unblock(AppUser user) async {
    setState(() => _busy = true);
    try {
      await widget.api.unblockUser(widget.session, user.id);
      if (mounted) {
        setState(() {
          _blockedUsers = _blockedUsers
              .where((candidate) => candidate.id != user.id)
              .toList(growable: false);
        });
      }
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _replace({
    bool? allowRoomChat,
    bool? allowProfileFind,
    bool? showWatchActivity,
  }) {
    final current = _settings;
    if (current == null) return;
    setState(() {
      _settings = PrivacySettings(
        allowRoomChat: allowRoomChat ?? current.allowRoomChat,
        allowProfileFind: allowProfileFind ?? current.allowProfileFind,
        showWatchActivity: showWatchActivity ?? current.showWatchActivity,
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final settings = _settings;
    return Scaffold(
      appBar: AppBar(title: const Text('隐私与屏蔽')),
      body: _busy && settings == null
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(20),
              children: [
                ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 720),
                  child: Card(
                    child: Padding(
                      padding: const EdgeInsets.all(18),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            '隐私设置',
                            style: Theme.of(context).textTheme.titleLarge,
                          ),
                          const SizedBox(height: 8),
                          if (settings != null) ...[
                            SwitchListTile(
                              contentPadding: EdgeInsets.zero,
                              title: const Text('参与房间聊天'),
                              subtitle: const Text('关闭后不发送、接收或显示房间消息。'),
                              value: settings.allowRoomChat,
                              onChanged: _busy
                                  ? null
                                  : (value) => _replace(allowRoomChat: value),
                            ),
                            SwitchListTile(
                              contentPadding: EdgeInsets.zero,
                              title: const Text('允许通过资料找到我'),
                              value: settings.allowProfileFind,
                              onChanged: _busy
                                  ? null
                                  : (value) =>
                                        _replace(allowProfileFind: value),
                            ),
                            SwitchListTile(
                              contentPadding: EdgeInsets.zero,
                              title: const Text('显示观看活动'),
                              value: settings.showWatchActivity,
                              onChanged: _busy
                                  ? null
                                  : (value) =>
                                        _replace(showWatchActivity: value),
                            ),
                            const SizedBox(height: 8),
                            FilledButton.icon(
                              onPressed: _busy ? null : _save,
                              icon: const Icon(Icons.save_rounded),
                              label: const Text('保存'),
                            ),
                          ],
                        ],
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 16),
                ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 720),
                  child: Card(
                    child: Padding(
                      padding: const EdgeInsets.all(18),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            '已屏蔽用户',
                            style: Theme.of(context).textTheme.titleLarge,
                          ),
                          const SizedBox(height: 8),
                          if (_blockedUsers.isEmpty)
                            const Padding(
                              padding: EdgeInsets.symmetric(vertical: 14),
                              child: Text('暂无已屏蔽用户。'),
                            ),
                          for (final user in _blockedUsers)
                            ListTile(
                              contentPadding: EdgeInsets.zero,
                              leading: const Icon(Icons.block_rounded),
                              title: Text(user.displayName),
                              trailing: TextButton(
                                onPressed: _busy ? null : () => _unblock(user),
                                child: const Text('取消屏蔽'),
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
                ),
                if (_error != null) ...[
                  const SizedBox(height: 16),
                  Text(
                    _error!,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ],
              ],
            ),
    );
  }
}
