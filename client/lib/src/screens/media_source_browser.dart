import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'quark_source_dialog.dart';
import 'provider_source_dialog.dart';

class MediaSourceBrowser extends StatefulWidget {
  const MediaSourceBrowser({
    super.key,
    required this.api,
    required this.session,
    required this.source,
    this.multiple = false,
  });
  final ApiClient api;
  final Session session;
  final MediaSource source;
  final bool multiple;
  @override
  State<MediaSourceBrowser> createState() => _MediaSourceBrowserState();
}

class _MediaSourceBrowserState extends State<MediaSourceBrowser> {
  List<({String path, String name})> _folders = [(path: '/', name: '全部文件')];
  List<MediaFile> _files = [];
  final _url = TextEditingController();
  final _filter = TextEditingController();
  final Map<String, MediaFile> _selected = {};
  String _resolvedUrl = '';
  int _offset = 0;
  bool _loading = true, _hasMore = false;
  String? _error;
  bool get _platform => platformSourceTypes.contains(widget.source.type);
  @override
  void initState() {
    super.initState();
    if (_platform) {
      _loading = false;
    } else {
      _load(_folders);
    }
  }

  @override
  void dispose() {
    _url.dispose();
    _filter.dispose();
    super.dispose();
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
      if (!mounted) {
        return;
      }
      setState(() {
        _folders = folders;
        _files = files;
        _loading = false;
        _filter.clear();
      });
    } catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = e is ApiException ? e.message : '文件列表加载失败，请重试。';
        });
      }
    }
  }

  Future<void> _resolve({bool more = false}) async {
    final target = more ? _resolvedUrl : _url.text.trim();
    if (target.isEmpty) {
      setState(() => _error = '请粘贴视频或播放列表链接。');
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final files = await widget.api.resolvePlatformSource(
        widget.session,
        widget.source.id,
        target,
        offset: more ? _offset : 0,
      );
      if (!mounted) {
        return;
      }
      setState(() {
        _files = more ? [..._files, ...files] : files;
        _offset = (more ? _offset : 0) + 50;
        _hasMore = files.length >= 50;
        _resolvedUrl = target;
        _loading = false;
      });
    } catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = e is ApiException ? e.message : '链接解析失败，请重试。';
        });
      }
    }
  }

  Future<void> _reconnect() async {
    final result = await showDialog<MediaSource>(
      context: context,
      barrierDismissible: false,
      builder: (_) => widget.source.type == 'quark'
          ? QuarkSourceDialog(
              api: widget.api,
              session: widget.session,
              source: widget.source,
            )
          : ProviderSourceDialog(
              api: widget.api,
              session: widget.session,
              source: widget.source,
            ),
    );
    if (result != null && mounted) {
      if (_platform) {
        if (_url.text.trim().isNotEmpty) {
          await _resolve();
        }
      } else {
        await _load(_folders);
      }
    }
  }

  void _pick(MediaFile file) {
    if (widget.multiple) {
      setState(() {
        if (_selected.containsKey(file.path)) {
          _selected.remove(file.path);
        } else if (_selected.length < 50) {
          _selected[file.path] = file;
        } else {
          _error = '每批最多选择 50 集，可分批继续添加。';
        }
      });
    } else {
      Navigator.of(context).pop(file);
    }
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
  Widget build(BuildContext context) {
    final visible = _files
        .where(
          (f) =>
              f.name.toLowerCase().contains(_filter.text.toLowerCase()) &&
              !RegExp(
                r'\.(srt|vtt|ass|ssa)$',
                caseSensitive: false,
              ).hasMatch(f.name),
        )
        .toList();
    return AlertDialog(
      title: Text(widget.source.name),
      content: SizedBox(
        width: 600,
        height: 500,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (_platform) ...[
              TextField(
                controller: _url,
                enabled: !_loading,
                decoration: const InputDecoration(labelText: '视频或播放列表链接'),
                onSubmitted: (_) => _resolve(),
              ),
              Align(
                alignment: Alignment.centerRight,
                child: TextButton.icon(
                  onPressed: _loading ? null : () => _resolve(),
                  icon: const Icon(Icons.link_rounded),
                  label: const Text('解析链接'),
                ),
              ),
            ] else
              Row(
                children: [
                  if (_folders.length > 1)
                    IconButton(
                      tooltip: '返回上级',
                      onPressed: _loading
                          ? null
                          : () =>
                                _load(_folders.sublist(0, _folders.length - 1)),
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
            if (_files.isNotEmpty) ...[
              TextField(
                controller: _filter,
                decoration: const InputDecoration(
                  labelText: '筛选当前列表',
                  prefixIcon: Icon(Icons.search),
                ),
                onChanged: (_) => setState(() {}),
              ),
              if (widget.multiple)
                Row(
                  children: [
                    Text('已选 ${_selected.length} 集'),
                    const Spacer(),
                    TextButton(
                      onPressed: () {
                        setState(() {
                          for (final f in visible.where(
                            (f) => !f.isDirectory,
                          )) {
                            if (_selected.length >= 50) {
                              break;
                            }
                            _selected[f.path] = f;
                          }
                        });
                      },
                      child: const Text('选择当前列表'),
                    ),
                    TextButton(
                      onPressed: () => setState(_selected.clear),
                      child: const Text('清空'),
                    ),
                  ],
                ),
            ],
            if (_error != null)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            const Divider(),
            Expanded(
              child: _loading
                  ? const Center(child: CircularProgressIndicator())
                  : visible.isEmpty
                  ? Center(
                      child: Text(
                        _platform ? '粘贴链接并解析，选择视频或剧集加入房间。' : '这里还没有可选影片。',
                        textAlign: TextAlign.center,
                      ),
                    )
                  : ListView.builder(
                      itemCount: visible.length + (_hasMore ? 1 : 0),
                      itemBuilder: (context, index) {
                        if (index == visible.length) {
                          return TextButton(
                            onPressed: () => _resolve(more: true),
                            child: const Text('加载后续剧集'),
                          );
                        }
                        final f = visible[index];
                        return ListTile(
                          leading: Icon(
                            f.isDirectory
                                ? Icons.folder_rounded
                                : Icons.movie_outlined,
                          ),
                          title: Text(
                            f.name,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                          ),
                          subtitle: f.isDirectory
                              ? null
                              : Text(
                                  _platform
                                      ? mediaSourceLabel(widget.source.type)
                                      : '${(f.size / (1024 * 1024)).toStringAsFixed(1)} MB',
                                ),
                          trailing: f.isDirectory
                              ? const Icon(Icons.chevron_right)
                              : widget.multiple
                              ? Checkbox(
                                  value: _selected.containsKey(f.path),
                                  onChanged: (_) => _pick(f),
                                )
                              : IconButton(
                                  tooltip: '收藏',
                                  onPressed: () => _favorite(f),
                                  icon: const Icon(Icons.star_border),
                                ),
                          onTap: () {
                            if (f.isDirectory) {
                              _load([
                                ..._folders,
                                (path: f.path, name: f.name),
                              ]);
                            } else {
                              _pick(f);
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
        if (widget.source.type == 'quark' ||
            _platform ||
            nasSourceTypes.contains(widget.source.type))
          TextButton(
            onPressed: _loading ? null : _reconnect,
            child: const Text('更新登录'),
          ),
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('关闭'),
        ),
        if (widget.multiple)
          FilledButton(
            onPressed: _selected.isEmpty
                ? null
                : () => Navigator.of(context).pop(_selected.values.toList()),
            child: Text('添加 ${_selected.length} 集'),
          ),
      ],
    );
  }
}
