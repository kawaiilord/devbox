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

  Future<void> _requestDeletion() async {
    final reason = TextEditingController();
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('申请注销账号'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text('管理员审核通过后，账号、房间、关系和凭据将不可恢复；订单仅保留匿名对账记录。'),
            const SizedBox(height: 12),
            TextField(
              controller: reason,
              maxLength: 1000,
              maxLines: 4,
              decoration: const InputDecoration(labelText: '注销原因'),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('确认申请'),
          ),
        ],
      ),
    );
    if (confirmed == true && reason.text.trim().isNotEmpty) {
      try {
        await widget.api.requestAccountDeletion(
          widget.session,
          reason.text.trim(),
        );
        if (mounted) {
          ScaffoldMessenger.of(context)
              .showSnackBar(const SnackBar(content: Text('注销申请已提交，可在审核前撤回。')));
        }
      } catch (error) {
        if (mounted) setState(() => _error = error.toString());
      }
    }
    reason.dispose();
  }

  Future<void> _copyrightComplaint() async {
    final name = TextEditingController(text: widget.session.user.displayName),
        email = TextEditingController(text: widget.session.user.email),
        basis = TextEditingController(),
        url = TextEditingController(),
        room = TextEditingController(),
        evidence = TextEditingController(),
        signature = TextEditingController();
    final submit = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('版权侵权投诉'),
        content: SizedBox(
          width: 560,
          child: SingleChildScrollView(
            child: Column(
              children: [
                TextField(
                  controller: name,
                  decoration: const InputDecoration(labelText: '权利人姓名'),
                ),
                const SizedBox(height: 8),
                TextField(
                  controller: email,
                  decoration: const InputDecoration(labelText: '联系邮箱'),
                ),
                const SizedBox(height: 8),
                TextField(
                  controller: basis,
                  maxLines: 4,
                  decoration: const InputDecoration(labelText: '权属依据'),
                ),
                const SizedBox(height: 8),
                TextField(
                  controller: url,
                  decoration: const InputDecoration(labelText: '侵权链接'),
                ),
                const SizedBox(height: 8),
                TextField(
                  controller: room,
                  decoration: const InputDecoration(labelText: '房间码（可选）'),
                ),
                const SizedBox(height: 8),
                TextField(
                  controller: evidence,
                  maxLines: 3,
                  decoration: const InputDecoration(
                    labelText: 'HTTPS 证据链接（每行一个）',
                  ),
                ),
                const SizedBox(height: 8),
                TextField(
                  controller: signature,
                  decoration: const InputDecoration(labelText: '电子签名姓名'),
                ),
                const Padding(
                  padding: EdgeInsets.only(top: 10),
                  child: Text('提交即确认信息真实准确。平台目标响应时限为 24 小时。'),
                ),
              ],
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('提交'),
          ),
        ],
      ),
    );
    if (submit == true) {
      try {
        await widget.api.submitCopyrightComplaint(
          session: widget.session,
          claimantName: name.text,
          claimantEmail: email.text,
          rightsBasis: basis.text,
          infringementUrl: url.text,
          roomCode: room.text,
          evidence: evidence.text
              .split('\n')
              .map((v) => v.trim())
              .where((v) => v.isNotEmpty)
              .toList(),
          signatureName: signature.text,
        );
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('投诉已提交，将在 24 小时目标时限内处理。')),
          );
        }
      } catch (error) {
        if (mounted) setState(() => _error = error.toString());
      }
    }
    for (final controller in [
      name,
      email,
      basis,
      url,
      room,
      evidence,
      signature,
    ]) {
      controller.dispose();
    }
  }

  void _replace({
    bool? allowRoomChat,
    bool? allowPrivateChat,
    bool? allowProfileFind,
    bool? showWatchActivity,
  }) {
    final current = _settings;
    if (current == null) return;
    setState(() {
      _settings = PrivacySettings(
        allowRoomChat: allowRoomChat ?? current.allowRoomChat,
        allowPrivateChat: allowPrivateChat ?? current.allowPrivateChat,
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
                              title: const Text('允许私聊'),
                              subtitle: const Text('关闭后其他用户不能创建新的私聊会话。'),
                              value: settings.allowPrivateChat,
                              onChanged: _busy
                                  ? null
                                  : (value) =>
                                        _replace(allowPrivateChat: value),
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
                            '合规与账号',
                            style: Theme.of(context).textTheme.titleLarge,
                          ),
                          const SizedBox(height: 8),
                          Wrap(
                            spacing: 8,
                            runSpacing: 8,
                            children: [
                              OutlinedButton.icon(
                                onPressed: _busy ? null : _copyrightComplaint,
                                icon: const Icon(Icons.copyright),
                                label: const Text('版权投诉'),
                              ),
                              FilledButton.icon(
                                style: FilledButton.styleFrom(
                                  backgroundColor: Theme.of(context)
                                      .colorScheme
                                      .error,
                                ),
                                onPressed: _busy ? null : _requestDeletion,
                                icon: const Icon(Icons.person_remove),
                                label: const Text('申请注销账号'),
                              ),
                            ],
                          ),
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
