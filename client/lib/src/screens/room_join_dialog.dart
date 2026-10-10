import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class JoinedRoom {
  const JoinedRoom(this.session, this.room);
  final Session session;
  final Room room;
}

class RoomJoinDialog extends StatefulWidget {
  const RoomJoinDialog({
    super.key,
    required this.api,
    this.session,
    this.code = '',
    this.needsPassword = true,
  });
  final ApiClient api;
  final Session? session;
  final String code;
  final bool needsPassword;
  @override
  State<RoomJoinDialog> createState() => _RoomJoinDialogState();
}

class _RoomJoinDialogState extends State<RoomJoinDialog> {
  late final _code = TextEditingController(text: widget.code);
  final _name = TextEditingController();
  final _password = TextEditingController();
  bool _busy = false;
  String? _error;
  @override
  void dispose() {
    _code.dispose();
    _name.dispose();
    _password.clear();
    _password.dispose();
    super.dispose();
  }

  Future<void> _join() async {
    if (_busy) {
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      JoinedRoom result;
      final session = widget.session;
      if (session == null) {
        final joined = await widget.api.joinAsGuest(
          code: _code.text,
          displayName: _name.text.trim(),
          password: _password.text,
        );
        result = JoinedRoom(joined.session, joined.room);
      } else {
        result = JoinedRoom(
          session,
          await widget.api.joinRoom(
            session: session,
            code: _code.text,
            password: _password.text,
          ),
        );
      }
      if (mounted) {
        Navigator.of(context).pop(result);
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e is ApiException ? e.message : '加入失败，请重试。';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_busy,
    child: AlertDialog(
      scrollable: true,
      title: Text(widget.session == null ? '以访客身份加入' : '加入房间'),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              controller: _code,
              enabled: !_busy && widget.code.isEmpty,
              textCapitalization: TextCapitalization.characters,
              maxLength: 6,
              decoration: const InputDecoration(labelText: '房间码'),
            ),
            if (widget.session == null) ...[
              const SizedBox(height: 12),
              TextField(
                controller: _name,
                enabled: !_busy,
                maxLength: 32,
                decoration: const InputDecoration(labelText: '访客昵称'),
              ),
              const Text('访客会话仅用于此房间，可用功能由房主设置。'),
            ],
            if (widget.needsPassword) ...[
              const SizedBox(height: 12),
              TextField(
                controller: _password,
                enabled: !_busy,
                obscureText: true,
                decoration: const InputDecoration(
                  labelText: '房间密码',
                  helperText: '无密码房间可留空。',
                ),
                onSubmitted: (_) => _join(),
              ),
            ],
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
          onPressed: _busy ? null : _join,
          child: Text(_busy ? '正在加入…' : '进入房间'),
        ),
      ],
    ),
  );
}
