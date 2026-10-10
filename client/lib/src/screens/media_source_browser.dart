import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'quark_source_dialog.dart';

class MediaSourceBrowser extends StatefulWidget {
  const MediaSourceBrowser({
    super.key,
    required this.api,
    required this.session,
    required this.source,
  });

  final ApiClient api;
  final Session session;
  final MediaSource source;

  @override
  State<MediaSourceBrowser> createState() => _MediaSourceBrowserState();
}

class _MediaSourceBrowserState extends State<MediaSourceBrowser> {
  List<({String path, String name})> _folders = [(path: '/', name: '全部文件')];
  List<MediaFile> _files = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load(_folders);
  }

  Future<void> _load(List<({String path, String name})> folders) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final files = await widget.api.browseMediaSource(
        widget.session,
        widget.source.id,
        folders.last.path,
      );
      if (!mounted) return;
      setState(() {
        _folders = folders;
        _files = files;
        _loading = false;
      });
    } catch (exception) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = exception is ApiException
            ? exception.message
            : '文件列表加载失败，请重试。';
      });
    }
  }

  Future<void> _reconnect() async {
    final result = await showDialog<MediaSource>(
      context: context,
      barrierDismissible: false,
      builder: (_) => QuarkSourceDialog(
        api: widget.api,
        session: widget.session,
        source: widget.source,
      ),
    );
    if (result != null && mounted) await _load(_folders);
  }

  Future<void> _favorite(MediaFile file) async {
    try {
      await widget.api.addFavorite(
        session: widget.session,
        sourceId: widget.source.id,
        file: file,
      );
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('已收藏 ${file.name}')));
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('收藏失败，请重试。')));
      }
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: Text(widget.source.name),
    content: SizedBox(
      width: 560,
      height: 420,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              if (_folders.length > 1)
                IconButton(
                  tooltip: '返回上级',
                  onPressed: _loading
                      ? null
                      : () => _load(_folders.sublist(0, _folders.length - 1)),
                  icon: const Icon(Icons.arrow_back_rounded),
                ),
              Expanded(
                child: Text(
                  _folders.last.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              IconButton(
                tooltip: '刷新',
                onPressed: _loading ? null : () => _load(_folders),
                icon: const Icon(Icons.refresh_rounded),
              ),
            ],
          ),
          const Divider(),
          Expanded(
            child: _loading
                ? const Center(child: CircularProgressIndicator())
                : _error != null
                ? Center(
                    child: SingleChildScrollView(
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          const Icon(Icons.cloud_off_outlined, size: 36),
                          const SizedBox(height: 12),
                          Text(_error!, textAlign: TextAlign.center),
                          const SizedBox(height: 12),
                          OutlinedButton(
                            onPressed: () => _load(_folders),
                            child: const Text('重试'),
                          ),
                          if (widget.source.type == 'quark')
                            TextButton(
                              onPressed: _reconnect,
                              child: const Text('更新夸克登录'),
                            ),
                        ],
                      ),
                    ),
                  )
                : _files.isEmpty
                ? const Center(
                    child: Text(
                      '这里还没有可选影片。\n可以返回上级，或先在网盘中添加视频。',
                      textAlign: TextAlign.center,
                    ),
                  )
                : ListView.builder(
                    itemCount: _files.length,
                    itemBuilder: (context, index) {
                      final file = _files[index];
                      return ListTile(
                        leading: Icon(
                          file.isDirectory
                              ? Icons.folder_rounded
                              : Icons.movie_outlined,
                        ),
                        title: Text(
                          file.name,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        subtitle: file.isDirectory
                            ? null
                            : Text(
                                '${(file.size / (1024 * 1024)).toStringAsFixed(1)} MB · 选为房间影片',
                              ),
                        trailing: file.isDirectory
                            ? const Icon(Icons.chevron_right_rounded)
                            : IconButton(
                                tooltip: '收藏',
                                onPressed: () => _favorite(file),
                                icon: const Icon(Icons.star_border_rounded),
                              ),
                        onTap: () {
                          if (file.isDirectory) {
                            _load([
                              ..._folders,
                              (path: file.path, name: file.name),
                            ]);
                          } else {
                            Navigator.of(context).pop(file);
                          }
                        },
                      );
                    },
                  ),
          ),
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.of(context).pop(),
        child: const Text('关闭'),
      ),
    ],
  );
}
