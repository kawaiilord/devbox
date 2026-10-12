import 'package:flutter/material.dart';

import 'workspace_navigation.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../design.dart';
import '../appearance.dart';
import 'room_join_dialog.dart';

class PublicRoomsPage extends StatefulWidget {
  const PublicRoomsPage({
    super.key,
    required this.api,
    required this.session,
    required this.onJoined,
    required this.onLoginRequired,
    this.embedded = false,
  });
  final ApiClient api;
  final Session? session;
  final void Function(Session, Room) onJoined;
  final void Function(String) onLoginRequired;
  final bool embedded;
  @override
  State<PublicRoomsPage> createState() => _PublicRoomsPageState();
}

class _PublicRoomsPageState extends State<PublicRoomsPage> {
  final _query = TextEditingController();
  final _tag = TextEditingController();
  String _category = '';
  List<PublicRoom> _rooms = [];
  bool _loading = true, _hasMore = false, _showTagFilter = false;
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
      if (!widget.embedded) Navigator.of(context).pop();
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
      if (!widget.embedded) Navigator.of(context).pop();
      widget.onJoined(result.session, result.room);
    }
  }

  @override
  Widget build(BuildContext context) {
    final mobile = MediaQuery.sizeOf(context).width < 700;
    return Scaffold(
      appBar: AppBar(
        leading: WorkspaceNavigation.leading(context),
        automaticallyImplyLeading: !widget.embedded,
        title: const Text('发现一起看的房间'),
        toolbarHeight: mobile ? 60 : 68,
        actions: [
          const FrameThemeMenu(),
          IconButton(
            tooltip: '刷新',
            onPressed: _loading ? null : () => _load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 1080),
          child: CustomScrollView(
            keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
            slivers: [
              SliverPadding(
                padding: mobile
                    ? const EdgeInsets.fromLTRB(16, 20, 16, 16)
                    : const EdgeInsets.fromLTRB(24, 32, 24, 24),
                sliver: SliverToBoxAdapter(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      if (!mobile) ...[
                        const FramePill(
                          'DISCOVER / 共同放映',
                          icon: Icons.explore_outlined,
                        ),
                        const SizedBox(height: 20),
                      ],
                      Text(
                        mobile ? '发现同频的人' : '好故事，总有人同频。',
                        style: Theme.of(context).textTheme.headlineSmall,
                      ),
                      const SizedBox(height: 10),
                      Text(
                        mobile ? '下一场放映，等你一起。' : '发现正在相聚的人，找到属于你的下一场放映。',
                        style: TextStyle(
                          color: FrameColors.of(context).muted,
                          fontSize: 13,
                          height: 1.7,
                        ),
                      ),
                      SizedBox(height: mobile ? 18 : 28),
                      TextField(
                        controller: _query,
                        textInputAction: TextInputAction.search,
                        decoration: InputDecoration(
                          labelText: '搜索房间',
                          hintText: '房间名称、感兴趣的故事…',
                          prefixIcon: const Icon(
                            Icons.search_rounded,
                            size: 20,
                          ),
                          suffixIcon: IconButton(
                            tooltip: '搜索',
                            onPressed: () => _load(),
                            icon: const Icon(
                              Icons.arrow_forward_rounded,
                              size: 20,
                            ),
                          ),
                        ),
                        onSubmitted: (_) => _load(),
                      ),
                      const SizedBox(height: 12),
                      if (mobile) ...[
                        Row(
                          children: [
                            Expanded(
                              child: SingleChildScrollView(
                                scrollDirection: Axis.horizontal,
                                child: Row(
                                  children: [
                                    for (final category in const [
                                      '',
                                      '电影',
                                      '剧集',
                                      '直播',
                                      '音乐',
                                      '其他',
                                    ])
                                      Padding(
                                        padding: const EdgeInsets.only(
                                          right: 8,
                                        ),
                                        child: ChoiceChip(
                                          label: Text(
                                            category.isEmpty ? '全部' : category,
                                          ),
                                          selected: _category == category,
                                          showCheckmark: false,
                                          onSelected: (_) {
                                            setState(
                                              () => _category = category,
                                            );
                                            _load();
                                          },
                                        ),
                                      ),
                                  ],
                                ),
                              ),
                            ),
                            IconButton(
                              tooltip: '标签筛选',
                              onPressed: () => setState(
                                () => _showTagFilter = !_showTagFilter,
                              ),
                              icon: Icon(
                                _showTagFilter || _tag.text.isNotEmpty
                                    ? Icons.filter_alt
                                    : Icons.tune_rounded,
                              ),
                            ),
                          ],
                        ),
                        if (_showTagFilter) ...[
                          const SizedBox(height: 8),
                          TextField(
                            controller: _tag,
                            textInputAction: TextInputAction.search,
                            decoration: InputDecoration(
                              labelText: '标签筛选',
                              suffixIcon: IconButton(
                                tooltip: '清除标签',
                                onPressed: () {
                                  _tag.clear();
                                  _load();
                                },
                                icon: const Icon(Icons.close_rounded),
                              ),
                            ),
                            onSubmitted: (_) => _load(),
                          ),
                        ],
                      ] else
                        Row(
                          children: [
                            Expanded(
                              child: DropdownButtonFormField<String>(
                                initialValue: _category,
                                isExpanded: true,
                                decoration: const InputDecoration(
                                  labelText: '分类',
                                ),
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
                                decoration: const InputDecoration(
                                  labelText: '标签筛选',
                                ),
                                onSubmitted: (_) => _load(),
                              ),
                            ),
                          ],
                        ),
                      SizedBox(height: mobile ? 18 : 26),
                      Row(
                        children: [
                          const Text(
                            '公开放映室',
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w500,
                            ),
                          ),
                          const Spacer(),
                          Text(
                            '${_rooms.length}${_hasMore ? '+' : ''} 个房间',
                            style: TextStyle(
                              fontSize: 12,
                              color: FrameColors.of(context).muted,
                            ),
                          ),
                        ],
                      ),
                      if (_loading)
                        const Padding(
                          padding: EdgeInsets.only(top: 18),
                          child: LinearProgressIndicator(minHeight: 2),
                        ),
                      if (_error != null)
                        Padding(
                          padding: const EdgeInsets.only(top: 16),
                          child: Text(
                            _error!,
                            style: TextStyle(
                              color: Theme.of(context).colorScheme.error,
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
              ),
              if (!_loading && _rooms.isEmpty && _error == null)
                const SliverToBoxAdapter(
                  child: FrameEmpty(
                    icon: Icons.explore_outlined,
                    title: '暂时没有匹配的房间',
                    detail: '试试其他关键词，或创建你的公开放映室。',
                  ),
                ),
              SliverPadding(
                padding: mobile
                    ? const EdgeInsets.fromLTRB(16, 0, 16, 24)
                    : const EdgeInsets.fromLTRB(24, 0, 24, 24),
                sliver: SliverList.separated(
                  itemCount: _rooms.length + (_hasMore ? 1 : 0),
                  separatorBuilder: (_, _) => const SizedBox(height: 12),
                  itemBuilder: (context, index) {
                    if (index == _rooms.length) {
                      return OutlinedButton(
                        onPressed: _loading ? null : () => _load(more: true),
                        child: const Text('加载更多房间'),
                      );
                    }
                    return _roomCard(_rooms[index]);
                  },
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _roomCard(PublicRoom room) => Card(
    child: Padding(
      padding: EdgeInsets.all(MediaQuery.sizeOf(context).width < 700 ? 18 : 22),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: FrameColors.of(context).elevated,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: FrameColors.of(context).border),
                ),
                child: Icon(
                  room.passwordProtected
                      ? Icons.lock_outline_rounded
                      : Icons.movie_outlined,
                  size: 20,
                  color: FrameColors.of(context).silver,
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Text(
                  room.name,
                  style: Theme.of(context).textTheme.titleLarge,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          if (room.description.isNotEmpty) ...[
            const SizedBox(height: 14),
            Text(
              room.description,
              style: TextStyle(
                color: FrameColors.of(context).muted,
                fontSize: 13,
              ),
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
            ),
          ],
          const SizedBox(height: 16),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              if (room.category.isNotEmpty)
                FramePill(room.category, color: FrameColors.of(context).silver),
              for (final tag in room.tags)
                FramePill(tag, color: FrameColors.of(context).muted),
              if (room.allowGuests)
                FramePill(
                  '可访客加入',
                  icon: Icons.person_outline_rounded,
                  color: FrameColors.of(context).silver,
                ),
            ],
          ),
          const SizedBox(height: 18),
          const Divider(height: 1),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: Text(
                  '房间 ${room.code} · ${room.members}/${room.maxMembers} 人',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
              const SizedBox(width: 10),
              FilledButton(
                onPressed: room.members >= room.maxMembers
                    ? null
                    : () => _join(room),
                child: Text(
                  room.members >= room.maxMembers
                      ? '房间已满'
                      : widget.session == null && !room.allowGuests
                      ? '先登录'
                      : '加入房间',
                ),
              ),
            ],
          ),
        ],
      ),
    ),
  );
}
