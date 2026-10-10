import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../design.dart';
import 'room_join_dialog.dart';

class PublicRoomsPage extends StatefulWidget {
  const PublicRoomsPage({
    super.key,
    required this.api,
    required this.session,
    required this.onJoined,
    required this.onLoginRequired,
  });
  final ApiClient api;
  final Session? session;
  final void Function(Session, Room) onJoined;
  final void Function(String) onLoginRequired;
  @override
  State<PublicRoomsPage> createState() => _PublicRoomsPageState();
}

class _PublicRoomsPageState extends State<PublicRoomsPage> {
  final _query = TextEditingController();
  final _tag = TextEditingController();
  String _category = '';
  List<PublicRoom> _rooms = [];
  bool _loading = true, _hasMore = false;
  int _offset = 0, _generation = 0;
  String? _error;
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _query.dispose();
    _tag.dispose();
    super.dispose();
  }

  Future<void> _load({bool more = false}) async {
    final generation = ++_generation;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final result = await widget.api.discoverRooms(
        query: _query.text.trim(),
        category: _category,
        tag: _tag.text.trim(),
        offset: more ? _offset : 0,
      );
      if (!mounted || generation != _generation) {
        return;
      }
      setState(() {
        _rooms = more ? [..._rooms, ...result.rooms] : result.rooms;
        _offset = result.nextOffset;
        _hasMore = result.hasMore;
        _loading = false;
      });
    } catch (e) {
      if (mounted && generation == _generation) {
        setState(() {
          _loading = false;
          _error = e is ApiException ? e.message : '房间列表加载失败。';
        });
      }
    }
  }

  Future<void> _join(PublicRoom room) async {
    if (widget.session == null && !room.allowGuests) {
      Navigator.of(context).pop();
      widget.onLoginRequired(room.code);
      return;
    }
    final result = await showDialog<JoinedRoom>(
      context: context,
      barrierDismissible: false,
      builder: (_) => RoomJoinDialog(
        api: widget.api,
        session: widget.session,
        code: room.code,
        needsPassword: room.passwordProtected,
      ),
    );
    if (result != null && mounted) {
      Navigator.of(context).pop();
      widget.onJoined(result.session, result.room);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('发现一起看的房间'),
      actions: [
        IconButton(
          tooltip: '刷新',
          onPressed: _loading ? null : () => _load(),
          icon: const Icon(Icons.refresh),
        ),
      ],
    ),
    body: Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 1100),
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                children: [
                  TextField(
                    controller: _query,
                    decoration: InputDecoration(
                      labelText: '搜索房间',
                      prefixIcon: const Icon(Icons.search),
                      suffixIcon: IconButton(
                        onPressed: () => _load(),
                        icon: const Icon(Icons.arrow_forward),
                      ),
                    ),
                    onSubmitted: (_) => _load(),
                  ),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(
                        child: DropdownButtonFormField<String>(
                          initialValue: _category,
                          isExpanded: true,
                          decoration: const InputDecoration(labelText: '分类'),
                          items: const ['', '电影', '剧集', '直播', '音乐', '其他']
                              .map(
                                (v) => DropdownMenuItem(
                                  value: v,
                                  child: Text(v.isEmpty ? '全部分类' : v),
                                ),
                              )
                              .toList(),
                          onChanged: (value) {
                            setState(() => _category = value ?? '');
                            _load();
                          },
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: TextField(
                          controller: _tag,
                          decoration: const InputDecoration(labelText: '标签筛选'),
                          onSubmitted: (_) => _load(),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.all(12),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            if (_loading) const LinearProgressIndicator(),
            Expanded(
              child: _rooms.isEmpty && !_loading
                  ? const Center(
                      child: Text(
                        '还没有符合条件的公开房间。\n创建房间后，可在房间设置中公开。',
                        textAlign: TextAlign.center,
                      ),
                    )
                  : ListView.separated(
                      padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
                      itemCount: _rooms.length + (_hasMore ? 1 : 0),
                      separatorBuilder: (_, _) => const SizedBox(height: 12),
                      itemBuilder: (context, index) {
                        if (index == _rooms.length) {
                          return TextButton(
                            onPressed: _loading
                                ? null
                                : () => _load(more: true),
                            child: const Text('加载更多房间'),
                          );
                        }
                        final room = _rooms[index];
                        return Container(
                          padding: const EdgeInsets.all(20),
                          decoration: BoxDecoration(
                            color: FrameColors.surface,
                            border: Border.all(color: FrameColors.border),
                            borderRadius: BorderRadius.circular(20),
                          ),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Row(
                                children: [
                                  Icon(
                                    room.passwordProtected
                                        ? Icons.lock_outline
                                        : Icons.live_tv_rounded,
                                    color: FrameColors.mint,
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: Text(
                                      room.name,
                                      style: Theme.of(context)
                                          .textTheme
                                          .titleLarge,
                                      maxLines: 2,
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                  ),
                                ],
                              ),
                              if (room.description.isNotEmpty) ...[
                                const SizedBox(height: 10),
                                Text(room.description),
                              ],
                              const SizedBox(height: 12),
                              Wrap(
                                spacing: 8,
                                runSpacing: 6,
                                children: [
                                  if (room.category.isNotEmpty)
                                    Chip(label: Text(room.category)),
                                  for (final tag in room.tags)
                                    Chip(label: Text(tag)),
                                  if (room.allowGuests)
                                    const Chip(label: Text('可访客加入')),
                                ],
                              ),
                              const SizedBox(height: 12),
                              Row(
                                children: [
                                  Expanded(
                                    child: Text(
                                      '房间 ${room.code} · 成员 ${room.members}/${room.maxMembers}',
                                      style: Theme.of(context)
                                          .textTheme
                                          .bodySmall,
                                    ),
                                  ),
                                  FilledButton(
                                    onPressed: room.members >= room.maxMembers
                                        ? null
                                        : () => _join(room),
                                    child: Text(
                                      room.members >= room.maxMembers
                                          ? '房间已满'
                                          : widget.session == null &&
                                                !room.allowGuests
                                          ? '先登录'
                                          : '加入房间',
                                    ),
                                  ),
                                ],
                              ),
                            ],
                          ),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    ),
  );
}
