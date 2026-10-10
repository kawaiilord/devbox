import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'room_page.dart';

class LibraryPage extends StatefulWidget {
  const LibraryPage({super.key, required this.api, required this.session});

  final ApiClient api;
  final Session session;

  @override
  State<LibraryPage> createState() => _LibraryPageState();
}

class _LibraryPageState extends State<LibraryPage> {
  List<Favorite> _favorites = const [];
  List<WatchRecord> _history = const [];
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
        widget.api.favorites(widget.session),
        widget.api.watchHistory(widget.session),
      ]);
      if (!mounted) return;
      setState(() {
        _favorites = values[0] as List<Favorite>;
        _history = values[1] as List<WatchRecord>;
      });
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _openFavorite(Favorite favorite) async {
    await _createRoom(
      name: '${favorite.title} 放映室',
      sourceId: favorite.sourceId,
      mediaPath: favorite.mediaPath,
    );
  }

  Future<void> _resume(WatchRecord record) async {
    if (!record.resumable) return;
    await _createRoom(
      name: '${record.title} · 继续观看',
      sourceId: record.sourceId,
      mediaPath: record.mediaPath,
      startPosition: record.positionSeconds,
    );
  }

  Future<void> _createRoom({
    required String name,
    required String sourceId,
    required String mediaPath,
    double startPosition = 0,
  }) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final room = await widget.api.createRoom(
        session: widget.session,
        name: name,
        mediaSourceId: sourceId,
        mediaPath: mediaPath,
        startPosition: startPosition,
      );
      if (!mounted) return;
      await Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => RoomPage(
            api: widget.api,
            session: widget.session,
            initialRoom: room,
          ),
        ),
      );
      await _load();
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _deleteFavorite(Favorite favorite) async {
    await _mutate(() => widget.api.deleteFavorite(widget.session, favorite.id));
  }

  Future<void> _deleteHistory(WatchRecord record) async {
    await _mutate(
      () => widget.api.deleteWatchRecord(widget.session, record.id),
    );
  }

  Future<void> _mutate(Future<void> Function() operation) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await operation();
      await _load();
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('收藏与观看历史'),
        actions: [
          IconButton(
            tooltip: '刷新',
            onPressed: _busy ? null : _load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          if (_busy) const LinearProgressIndicator(),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ],
          const SizedBox(height: 16),
          _Section(
            title: '收藏',
            emptyText: '还没有收藏影片。可以在媒体源浏览器中点击星标收藏。',
            children: [
              for (final favorite in _favorites)
                ListTile(
                  leading: const Icon(Icons.star_rounded),
                  title: Text(favorite.title),
                  subtitle: Text(
                    '${mediaSourceLabel(favorite.sourceType)} · ${favorite.sourceName} · ${_formatBytes(favorite.size)}',
                  ),
                  onTap: _busy ? null : () => _openFavorite(favorite),
                  trailing: IconButton(
                    tooltip: '取消收藏',
                    onPressed: _busy ? null : () => _deleteFavorite(favorite),
                    icon: const Icon(Icons.delete_outline_rounded),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 16),
          _Section(
            title: '观看历史',
            emptyText: '开始播放后，这里会同步显示观看进度。',
            children: [
              for (final record in _history)
                ListTile(
                  leading: Icon(
                    record.completed
                        ? Icons.check_circle_rounded
                        : Icons.history_rounded,
                  ),
                  title: Text(record.title),
                  subtitle: Text(_historySubtitle(record)),
                  onTap: _busy || !record.resumable
                      ? null
                      : () => _resume(record),
                  trailing: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (record.resumable)
                        IconButton(
                          tooltip: '从进度创建房间',
                          onPressed: _busy ? null : () => _resume(record),
                          icon: const Icon(Icons.play_arrow_rounded),
                        ),
                      IconButton(
                        tooltip: '删除历史',
                        onPressed: _busy ? null : () => _deleteHistory(record),
                        icon: const Icon(Icons.delete_outline_rounded),
                      ),
                    ],
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }

  String _historySubtitle(WatchRecord record) {
    final position = _formatDuration(record.positionSeconds);
    final duration = record.durationSeconds > 0
        ? _formatDuration(record.durationSeconds)
        : '--:--';
    final together = record.companionCount > 0
        ? ' · 与 ${record.companionCount} 人同看'
        : '';
    final resume = record.resumable ? '' : ' · 片源不可继续';
    return '$position / $duration$together$resume';
  }

  String _formatDuration(double seconds) {
    final value = Duration(seconds: seconds.round());
    final hours = value.inHours;
    final minutes = value.inMinutes.remainder(60).toString().padLeft(2, '0');
    final remaining = value.inSeconds.remainder(60).toString().padLeft(2, '0');
    return hours > 0 ? '$hours:$minutes:$remaining' : '$minutes:$remaining';
  }

  String _formatBytes(int bytes) {
    if (bytes >= 1024 * 1024 * 1024) {
      return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(1)} GB';
    }
    if (bytes >= 1024 * 1024) {
      return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    if (bytes >= 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    return '$bytes B';
  }
}

class _Section extends StatelessWidget {
  const _Section({
    required this.title,
    required this.emptyText,
    required this.children,
  });

  final String title;
  final String emptyText;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            if (children.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16),
                child: Text(emptyText),
              )
            else
              ...children,
          ],
        ),
      ),
    );
  }
}
