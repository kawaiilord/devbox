import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../room_controller.dart';

class RoomSettingsDialog extends StatefulWidget {
  const RoomSettingsDialog({super.key, required this.controller});
  final RoomController controller;
  @override
  State<RoomSettingsDialog> createState() => _RoomSettingsDialogState();
}

class _RoomSettingsDialogState extends State<RoomSettingsDialog> {
  late final Room _baseline = widget.controller.room;
  late final _description = TextEditingController(
    text: _baseline.features?.description ?? '',
  );
  late final _category = TextEditingController(
    text: _baseline.features?.category ?? '',
  );
  late final _tags = TextEditingController(
    text: _baseline.features?.tags.join(', ') ?? '',
  );
  final _password = TextEditingController();
  late bool _public = _baseline.features?.visibility == 'public';
  late bool _guests = _baseline.features?.allowGuests ?? false;
  late bool _autoNext = _baseline.features?.autoNext ?? true;
  bool _clearPassword = false, _busy = false;
  String? _error;
  @override
  void dispose() {
    _description.dispose();
    _category.dispose();
    _tags.dispose();
    _password.clear();
    _password.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    if (_busy) {
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final room = await widget.controller.api.updateRoomSettings(
        widget.controller.session,
        _baseline,
        {
          'visibility': _public ? 'public' : 'private',
          'description': _description.text.trim(),
          'category': _category.text.trim(),
          'tags': _tags.text
              .replaceAll('，', ',')
              .split(',')
              .map((v) => v.trim())
              .where((v) => v.isNotEmpty)
              .toList(),
          'allow_guests': _guests,
          'auto_next': _autoNext,
          if (_clearPassword || _password.text.isNotEmpty)
            'password': _clearPassword ? '' : _password.text,
        },
      );
      await widget.controller.acceptRoom(room);
      if (mounted) {
        Navigator.of(context).pop();
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e is ApiException ? e.message : '保存失败，请重试。';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_busy,
    child: AlertDialog(
      scrollable: true,
      title: const Text('房间设置'),
      content: SizedBox(
        width: 520,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('公开房间'),
              subtitle: const Text('开启后出现在房间发现列表中'),
              value: _public,
              onChanged: _busy ? null : (v) => setState(() => _public = v),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('允许访客加入'),
              subtitle: const Text('访客默认仅观看；房主可单独授予互动权限'),
              value: _guests,
              onChanged: _busy ? null : (v) => setState(() => _guests = v),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('自动播放下一集'),
              value: _autoNext,
              onChanged: _busy ? null : (v) => setState(() => _autoNext = v),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _description,
              enabled: !_busy,
              maxLength: 500,
              minLines: 2,
              maxLines: 3,
              decoration: const InputDecoration(labelText: '房间简介'),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _category,
              enabled: !_busy,
              maxLength: 40,
              decoration: const InputDecoration(
                labelText: '分类',
                hintText: '电影、剧集、直播、音乐或其他',
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _tags,
              enabled: !_busy,
              decoration: const InputDecoration(
                labelText: '房间标签',
                helperText: '逗号分隔，最多 8 个标签',
              ),
            ),
            const SizedBox(height: 18),
            TextField(
              controller: _password,
              enabled: !_busy && !_clearPassword,
              obscureText: true,
              decoration: InputDecoration(
                labelText: '设置新密码',
                helperText: _baseline.features?.passwordProtected == true
                    ? '留空保留原密码。'
                    : '留空不设置密码；密码至少 6 位。',
              ),
            ),
            if (_baseline.features?.passwordProtected == true)
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('移除现有密码'),
                value: _clearPassword,
                onChanged: _busy
                    ? null
                    : (v) => setState(() => _clearPassword = v ?? false),
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
          onPressed: _busy ? null : () => Navigator.of(context).pop(),
          child: const Text('取消'),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(_busy ? '正在保存…' : '保存设置'),
        ),
      ],
    ),
  );
}

class RoomMemberDialog extends StatefulWidget {
  const RoomMemberDialog({
    super.key,
    required this.controller,
    required this.member,
  });
  final RoomController controller;
  final RoomMember member;
  @override
  State<RoomMemberDialog> createState() => _RoomMemberDialogState();
}

class _RoomMemberDialogState extends State<RoomMemberDialog> {
  late final Room _baseline = widget.controller.room;
  late String _role = widget.member.role;
  late Map<String, dynamic> _permissions =
      (_baseline.features?.memberPermissions[widget.member.userId] ??
              const RoomPermissions())
          .toJson();
  bool _busy = false;
  String? _error;
  Future<void> _save({bool remove = false}) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final c = widget.controller;
      final room = remove
          ? await c.api.removeRoomMember(
              c.session,
              _baseline,
              widget.member.userId,
            )
          : await c.api.updateRoomMember(
              c.session,
              _baseline,
              widget.member.userId,
              role: _role,
              permissions: RoomPermissions.fromJson(_permissions),
            );
      await c.acceptRoom(room);
      if (mounted) {
        Navigator.of(context).pop();
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e is ApiException ? e.message : '权限更新失败。';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    scrollable: true,
    title: Text('${widget.member.displayName} · 权限'),
    content: SizedBox(
      width: 440,
      child: Column(
        children: [
          DropdownButtonFormField<String>(
            initialValue: _role,
            decoration: const InputDecoration(labelText: '房间角色'),
            items: (_role == 'guest' ? ['guest'] : ['member', 'moderator'])
                .map(
                  (v) => DropdownMenuItem(
                    value: v,
                    child: Text(
                      v == 'guest'
                          ? '访客'
                          : v == 'moderator'
                          ? '协管员'
                          : '成员',
                    ),
                  ),
                )
                .toList(),
            onChanged: _busy || _role == 'guest'
                ? null
                : (v) {
                    if (v != null) {
                      setState(() {
                        _role = v;
                        _permissions = RoomPermissions(
                          playback: v == 'moderator',
                          playlist: v == 'moderator',
                        ).toJson();
                      });
                    }
                  },
          ),
          const SizedBox(height: 12),
          for (final entry in const {
            'playback': '播放、暂停、拖动与切集',
            'playlist': '添加、删除和排序片单',
            'chat': '房间聊天',
            'danmaku': '观看与发送弹幕',
            'voice': '加入房间语音',
          }.entries)
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(entry.value),
              value: _permissions[entry.key] == true,
              onChanged: _busy
                  ? null
                  : (v) => setState(() => _permissions[entry.key] = v),
            ),
          if (_error != null)
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: _busy ? null : () => _save(remove: true),
        child: const Text('移出并禁止加入'),
      ),
      TextButton(
        onPressed: _busy ? null : () => Navigator.of(context).pop(),
        child: const Text('取消'),
      ),
      FilledButton(
        onPressed: _busy ? null : () => _save(),
        child: const Text('保存权限'),
      ),
    ],
  );
}
