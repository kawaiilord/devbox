import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../room_controller.dart';
import 'media_source_browser.dart';
import 'provider_source_dialog.dart';

class PlaylistDialog extends StatefulWidget {
  const PlaylistDialog({super.key, required this.controller});
  final RoomController controller;
  @override
  State<PlaylistDialog> createState() => _PlaylistDialogState();
}

class _PlaylistDialogState extends State<PlaylistDialog> {
  bool _busy = false;
  String? _error;
  RoomController get c => widget.controller;
  Future<void> _act(Future<Room> Function(Room) action) async {
    if (_busy) {
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await c.acceptRoom(await action(c.room));
    } catch (e) {
      try {
        await c.refreshRoom();
      } catch (_) {}
      if (mounted) {
        setState(() => _error = e is ApiException ? e.message : '片单更新失败，请重试。');
      }
    } finally {
      if (mounted) {
        setState(() => _busy = false);
      }
    }
  }

  Future<void> _addDirect({PlaylistEntry? alternativeFor}) async {
    final title = TextEditingController(
      text: alternativeFor == null ? '' : '备用来源',
    );
    final url = TextEditingController();
    final result = await showDialog<PlaylistEntry>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        scrollable: true,
        title: Text(alternativeFor == null ? '添加视频直链' : '添加备用来源'),
        content: SizedBox(
          width: 470,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: title,
                decoration: InputDecoration(
                  labelText: alternativeFor == null ? '影片或剧集名称' : '来源名称',
                ),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: url,
                decoration: const InputDecoration(labelText: 'HTTP(S) 视频直链'),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () {
              if (title.text.trim().isNotEmpty && url.text.trim().isNotEmpty) {
                Navigator.of(dialogContext).pop(
                  PlaylistEntry(
                    title: title.text.trim(),
                    sources: [
                      PlaylistSource(
                        label: alternativeFor == null
                            ? '直链'
                            : title.text.trim(),
                        url: url.text.trim(),
                      ),
                    ],
                  ),
                );
              }
            },
            child: const Text('添加'),
          ),
        ],
      ),
    );
    title.dispose();
    url.dispose();
    if (result == null || !mounted) {
      return;
    }
    await _act(
      (room) => alternativeFor == null
          ? c.api.addPlaylistItems(c.session, room, [result])
          : c.api.addPlaylistSource(
              c.session,
              room,
              alternativeFor.id,
              result.sources.single,
            ),
    );
  }

  Future<void> _addFromSource({PlaylistEntry? alternativeFor}) async {
    final result = await showDialog<List<PlaylistEntry>>(
      context: context,
      builder: (_) => _SourcePlaylistPicker(
        api: c.api,
        session: c.session,
        multiple: alternativeFor == null,
      ),
    );
    if (result == null || result.isEmpty || !mounted) {
      return;
    }
    await _act(
      (room) => alternativeFor == null
          ? c.api.addPlaylistItems(c.session, room, result)
          : c.api.addPlaylistSource(
              c.session,
              room,
              alternativeFor.id,
              result.first.sources.first,
            ),
    );
  }

  Future<void> _quality(PlaylistEntry item) async {
    try {
      final variants = await c.api.roomMediaVariants(
        c.session,
        c.room,
        itemId: item.id,
      );
      if (!mounted) {
        return;
      }
      final chosen = await showDialog<MediaVariant>(
        context: context,
        builder: (context) => SimpleDialog(
          title: const Text('选择画质'),
          children: [
            for (final variant in variants)
              SimpleDialogOption(
                onPressed: () => Navigator.of(context).pop(variant),
                child: Text(variant.label),
              ),
          ],
        ),
      );
      if (chosen != null && mounted) {
        await _act(
          (room) => c.api.selectPlaylistMedia(
            c.session,
            room,
            itemId: item.id,
            variantId: chosen.id,
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        setState(() => _error = e is ApiException ? e.message : '画质列表加载失败。');
      }
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) {
      final features = c.room.features;
      final items = features?.playlist ?? <PlaylistEntry>[];
      return AlertDialog(
        title: const Text('房间片单'),
        content: SizedBox(
          width: 700,
          height: 520,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Wrap(
                spacing: 8,
                runSpacing: 6,
                children: [
                  if (c.canManagePlaylist)
                    OutlinedButton.icon(
                      onPressed: _busy ? null : () => _addDirect(),
                      icon: const Icon(Icons.add_link),
                      label: const Text('添加直链'),
                    ),
                  if (c.canManagePlaylist && !c.session.user.isGuest)
                    FilledButton.icon(
                      onPressed: _busy ? null : () => _addFromSource(),
                      icon: const Icon(Icons.video_library_outlined),
                      label: const Text('平台 / 媒体库选片'),
                    ),
                  Text(
                    '${items.length} 集${features?.autoNext == true ? ' · 自动连播' : ''}',
                  ),
                ],
              ),
              if (_busy) const LinearProgressIndicator(),
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Text(
                    _error!,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
              const Divider(),
              Expanded(
                child: items.isEmpty
                    ? const Center(child: Text('片单为空，添加影片开始一起看。'))
                    : ReorderableListView.builder(
                        buildDefaultDragHandles: c.canManagePlaylist && !_busy,
                        itemCount: items.length,
                        onReorder: (oldIndex, newIndex) {
                          if (!c.canManagePlaylist || _busy) {
                            return;
                          }
                          final order = items.map((i) => i.id).toList();
                          if (newIndex > oldIndex) {
                            newIndex--;
                          }
                          final id = order.removeAt(oldIndex);
                          order.insert(newIndex, id);
                          _act(
                            (room) =>
                                c.api.reorderPlaylist(c.session, room, order),
                          );
                        },
                        itemBuilder: (context, index) {
                          final item = items[index];
                          final active = item.id == features?.activeItemId;
                          return ListTile(
                            key: ValueKey(item.id),
                            selected: active,
                            leading: Icon(
                              active
                                  ? Icons.play_circle_fill
                                  : Icons.movie_outlined,
                            ),
                            title: Text(
                              '${index + 1}. ${item.title}',
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                            ),
                            subtitle: Text(
                              '${item.selectedSource?.label ?? '片源'} · ${item.sources.length} 个来源',
                            ),
                            onTap: !c.canControl || _busy
                                ? null
                                : () => _act(
                                    (room) => c.api.selectPlaylistMedia(
                                      c.session,
                                      room,
                                      itemId: item.id,
                                    ),
                                  ),
                            trailing: !c.canControl && !c.canManagePlaylist
                                ? null
                                : PopupMenuButton<String>(
                                    enabled: !_busy,
                                    tooltip: '影片操作',
                                    onSelected: (action) {
                                      if (action == 'quality') {
                                        _quality(item);
                                      } else if (action == 'link') {
                                        _addDirect(alternativeFor: item);
                                      } else if (action == 'library') {
                                        _addFromSource(alternativeFor: item);
                                      } else if (action == 'delete') {
                                        _act(
                                          (room) => c.api.removePlaylistItem(
                                            c.session,
                                            room,
                                            item.id,
                                          ),
                                        );
                                      } else if (action.startsWith('source:')) {
                                        _act(
                                          (room) => c.api.selectPlaylistMedia(
                                            c.session,
                                            room,
                                            itemId: item.id,
                                            sourceId: action.substring(7),
                                          ),
                                        );
                                      }
                                    },
                                    itemBuilder: (_) => [
                                      if (c.canControl)
                                        const PopupMenuItem(
                                          value: 'quality',
                                          child: Text('选择画质'),
                                        ),
                                      if (c.canControl)
                                        for (final source in item.sources)
                                          PopupMenuItem(
                                            value: 'source:${source.id}',
                                            child: Text('切换至 ${source.label}'),
                                          ),
                                      if (c.canManagePlaylist)
                                        const PopupMenuItem(
                                          value: 'link',
                                          child: Text('添加备用直链'),
                                        ),
                                      if (c.canManagePlaylist &&
                                          !c.session.user.isGuest)
                                        const PopupMenuItem(
                                          value: 'library',
                                          child: Text('添加备用媒体源'),
                                        ),
                                      if (c.canManagePlaylist)
                                        const PopupMenuItem(
                                          value: 'delete',
                                          child: Text('从片单移除'),
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
        actions: [
          if (c.canControl)
            TextButton(
              onPressed: _busy
                  ? null
                  : () => _act(
                      (room) => c.api.selectPlaylistMedia(
                        c.session,
                        room,
                        direction: 'previous',
                      ),
                    ),
              child: const Text('上一集'),
            ),
          if (c.canControl)
            TextButton(
              onPressed: _busy
                  ? null
                  : () => _act(
                      (room) => c.api.selectPlaylistMedia(
                        c.session,
                        room,
                        direction: 'next',
                      ),
                    ),
              child: const Text('下一集'),
            ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('关闭'),
          ),
        ],
      );
    },
  );
}

class _SourcePlaylistPicker extends StatefulWidget {
  const _SourcePlaylistPicker({
    required this.api,
    required this.session,
    required this.multiple,
  });
  final ApiClient api;
  final Session session;
  final bool multiple;
  @override
  State<_SourcePlaylistPicker> createState() => _SourcePlaylistPickerState();
}

class _SourcePlaylistPickerState extends State<_SourcePlaylistPicker> {
  List<MediaSource> _sources = [];
  bool _loading = true;
  String? _error;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final sources = await widget.api.listMediaSources(widget.session);
      if (mounted) {
        setState(() {
          _sources = sources;
          _loading = false;
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = '媒体源加载失败。';
        });
      }
    }
  }

  Future<void> _pick(MediaSource source) async {
    List<MediaFile>? files;
    if (widget.multiple) {
      files = await showDialog<List<MediaFile>>(
        context: context,
        builder: (_) => MediaSourceBrowser(
          api: widget.api,
          session: widget.session,
          source: source,
          multiple: true,
        ),
      );
    } else {
      final file = await showDialog<MediaFile>(
        context: context,
        builder: (_) => MediaSourceBrowser(
          api: widget.api,
          session: widget.session,
          source: source,
        ),
      );
      if (file != null) {
        files = [file];
      }
    }
    if (files != null && files.isNotEmpty && mounted) {
      Navigator.of(context).pop(
        files
            .map(
              (f) => PlaylistEntry(
                title: f.name,
                sources: [
                  PlaylistSource(
                    label: '${mediaSourceLabel(source.type)} · ${source.name}',
                    mediaSourceId: source.id,
                    mediaPath: f.path,
                  ),
                ],
              ),
            )
            .toList(),
      );
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: const Text('选择媒体源'),
    content: SizedBox(
      width: 540,
      height: 360,
      child: _loading
          ? const Center(child: CircularProgressIndicator())
          : _sources.isEmpty
          ? Center(child: Text(_error ?? '还没有媒体源，请先连接平台或 NAS。'))
          : ListView(
              children: [
                for (final source in _sources)
                  ListTile(
                    leading: const Icon(Icons.cloud_outlined),
                    title: Text(source.name),
                    subtitle: Text(mediaSourceLabel(source.type)),
                    onTap: () => _pick(source),
                  ),
              ],
            ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.of(context).pop(),
        child: const Text('关闭'),
      ),
      FilledButton(
        onPressed: () async {
          final source = await showDialog<MediaSource>(
            context: context,
            barrierDismissible: false,
            builder: (_) =>
                ProviderSourceDialog(api: widget.api, session: widget.session),
          );
          if (source != null && mounted) {
            await _load();
          }
        },
        child: const Text('添加平台 / NAS'),
      ),
    ],
  );
}
