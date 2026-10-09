import 'dart:async';

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../sync/room_socket.dart';

class SocialPage extends StatefulWidget {
  const SocialPage({super.key, required this.api, required this.session});
  final ApiClient api;
  final Session session;
  @override
  State<SocialPage> createState() => _SocialPageState();
}

class _SocialPageState extends State<SocialPage> {
  final _query = TextEditingController();
  List<SocialProfile> _users = const [];
  List<Conversation> _conversations = const [];
  RoomSocket? _socket;
  StreamSubscription<RoomEnvelope>? _events;
  bool _busy = false;
  String? _error;
  int _unread = 0;
  @override
  void initState() {
    super.initState();
    _load();
    _connect();
  }

  Future<void> _connect() async {
    final socket = RoomSocket(() => widget.api.socialSocketUri(widget.session));
    _socket = socket;
    _events = socket.events.listen((event) {
      if (event.type == 'social.message' || event.type == 'social.unread') {
        _load();
      }
    });
    try {
      await socket.connect();
    } catch (_) {}
  }

  Future<void> _load() async {
    try {
      final values = await Future.wait<Object>([
        widget.api.conversations(widget.session),
        widget.api.directUnread(widget.session),
      ]);
      if (mounted) {
        setState(() {
          _conversations = values[0] as List<Conversation>;
          _unread = values[1] as int;
        });
      }
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    }
  }

  Future<void> _search() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final users = await widget.api.searchSocialUsers(
        widget.session,
        _query.text,
      );
      if (mounted) setState(() => _users = users);
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _toggleFollow(SocialProfile user) async {
    if (user.following) {
      await widget.api.unfollowUser(widget.session, user.id);
    } else {
      await widget.api.followUser(widget.session, user.id);
    }
    await _search();
  }

  Future<void> _startChat(SocialProfile user) async {
    final conversation = await widget.api.createConversation(
      widget.session,
      user.id,
    );
    await _openChat(conversation);
    await _load();
  }

  Future<void> _openChat(Conversation conversation) async {
    var messages = await widget.api.directMessages(
      widget.session,
      conversation.id,
    );
    if (!mounted) return;
    final input = TextEditingController();
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: Text(conversation.peer.displayName),
          content: SizedBox(
            width: 520,
            height: 440,
            child: Column(
              children: [
                Expanded(
                  child: ListView(
                    children: [
                      for (final message in messages)
                        Align(
                          alignment: message.senderId == widget.session.user.id
                              ? Alignment.centerRight
                              : Alignment.centerLeft,
                          child: Card(
                            child: Padding(
                              padding: const EdgeInsets.all(10),
                              child: Text(message.body),
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: input,
                        maxLength: 2000,
                        maxLines: 2,
                        decoration: const InputDecoration(
                          labelText: '私聊消息',
                          counterText: '',
                        ),
                      ),
                    ),
                    IconButton.filled(
                      onPressed: () async {
                        final body = input.text.trim();
                        if (body.isEmpty) return;
                        await widget.api.sendDirectMessage(
                          widget.session,
                          conversation.id,
                          body,
                        );
                        input.clear();
                        messages = await widget.api.directMessages(
                          widget.session,
                          conversation.id,
                        );
                        setDialogState(() {});
                      },
                      icon: const Icon(Icons.send_rounded),
                    ),
                  ],
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('关闭'),
            ),
          ],
        ),
      ),
    );
    input.dispose();
  }

  @override
  void dispose() {
    _events?.cancel();
    _socket?.close();
    _query.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text('社交与私聊${_unread > 0 ? ' ($_unread)' : ''}')),
    body: ListView(
      padding: const EdgeInsets.all(18),
      children: [
        Row(
          children: [
            Expanded(
              child: TextField(
                controller: _query,
                decoration: const InputDecoration(labelText: '搜索用户'),
                onSubmitted: (_) => _search(),
              ),
            ),
            const SizedBox(width: 8),
            FilledButton(
              onPressed: _busy ? null : _search,
              child: const Text('搜索'),
            ),
          ],
        ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.all(12),
            child: Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ),
        for (final user in _users)
          ListTile(
            leading: CircleAvatar(
              child: Text(user.displayName.characters.first),
            ),
            title: Text(user.displayName),
            subtitle: Text(
              '${user.followerCount} 粉丝${user.signature.isEmpty ? '' : ' · ${user.signature}'}',
            ),
            trailing: Wrap(
              children: [
                TextButton(
                  onPressed: () => _toggleFollow(user),
                  child: Text(user.following ? '取消关注' : '关注'),
                ),
                IconButton(
                  onPressed: () => _startChat(user),
                  icon: const Icon(Icons.chat_outlined),
                ),
              ],
            ),
          ),
        const Divider(),
        Text('私聊', style: Theme.of(context).textTheme.titleLarge),
        for (final conversation in _conversations)
          ListTile(
            leading: CircleAvatar(
              child: Text(conversation.peer.displayName.characters.first),
            ),
            title: Text(conversation.peer.displayName),
            subtitle: Text(
              conversation.lastMessage.isEmpty
                  ? '还没有消息'
                  : conversation.lastMessage,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            trailing: conversation.unreadCount > 0
                ? Badge(label: Text('${conversation.unreadCount}'))
                : null,
            onTap: () => _openChat(conversation),
          ),
      ],
    ),
  );
}
