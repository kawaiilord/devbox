import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../catalog_models.dart';
import '../design.dart';
import '../models.dart';

class CatalogBrowser extends StatefulWidget {
  const CatalogBrowser({
    super.key,
    required this.api,
    required this.session,
    required this.onOpenRoom,
    this.initialQuery = '',
    this.onMetadata,
    this.initialFilm,
    this.onCloseDetail,
  });
  final ApiClient api;
  final Session session;
  final ValueChanged<Room> onOpenRoom;
  final String initialQuery;
  final VoidCallback? onMetadata;
  final CatalogFilm? initialFilm;
  final VoidCallback? onCloseDetail;
  @override
  State<CatalogBrowser> createState() => _CatalogBrowserState();
}

class _CatalogBrowserState extends State<CatalogBrowser> {
  late final TextEditingController _query;
  final _roomCode = TextEditingController();
  List<CatalogSource> _sources = [];
  final List<CatalogFilm> _films = [];
  final Map<String, int> _pages = {};
  final Set<String> _more = {};
  final Map<String, String> _failures = {};
  List<CatalogFilm> _details = [];
  List<(CatalogFilm, CatalogGroup)> _lines = [];
  bool _searching = false,
      _detailing = false,
      _busy = false,
      _allEpisodes = true;
  int _request = 0, _done = 0, _line = 0, _episodePage = 0;
  String _episode = '', _searchQuery = '';
  String? _error;
  bool _showDetail = false;

  @override
  void initState() {
    super.initState();
    _query = TextEditingController(text: widget.initialQuery);
    if (widget.initialFilm != null) {
      _detail([widget.initialFilm!]);
    } else {
      _search();
    }
  }

  @override
  void dispose() {
    _request++;
    _query.dispose();
    _roomCode.dispose();
    super.dispose();
  }

  String _message(Object error) =>
      error is ApiException ? error.message : '暂时连接失败，请重试。';

  Future<void> _search({bool more = false}) async {
    final request = ++_request;
    setState(() {
      _error = null;
      _searching = true;
      _showDetail = false;
      _done = 0;
      if (!more) {
        _films.clear();
        _pages.clear();
        _more.clear();
        _failures.clear();
        _searchQuery = _query.text.trim();
      }
    });
    try {
      if (!more) _sources = await widget.api.catalogSources(widget.session);
      if (!mounted || request != _request) return;
      final pending = _sources
          .where((s) => !more || _more.contains(s.id))
          .toList();
      var cursor = 0;
      Future<void> worker() async {
        while (cursor < pending.length && mounted && request == _request) {
          final source = pending[cursor++];
          try {
            final page = (_pages[source.id] ?? 0) + 1;
            final result = await widget.api.searchCatalog(
              widget.session,
              source.id,
              _searchQuery,
              page: page,
            );
            if (!mounted || request != _request) return;
            setState(() {
              _films.addAll(result.items);
              _pages[source.id] = page;
              _failures.remove(source.id);
              if (result.hasMore) {
                _more.add(source.id);
              } else {
                _more.remove(source.id);
              }
            });
          } catch (e) {
            if (!mounted || request != _request) return;
            setState(() => _failures[source.id] = _message(e));
          } finally {
            if (mounted && request == _request) setState(() => _done++);
          }
        }
      }

      await Future.wait(
        List.generate(math.min(4, pending.length), (_) => worker()),
      );
      if (!mounted || request != _request) return;
      if (!more &&
          widget.initialQuery.isNotEmpty &&
          _searchQuery == widget.initialQuery) {
        final exact = groupCatalogFilms(_films)
            .where((g) => g.first.title.trim() == _searchQuery)
            .toList();
        if (exact.length == 1) await _detail(exact.single);
      }
    } catch (e) {
      if (mounted && request == _request) setState(() => _error = _message(e));
    } finally {
      if (mounted && request == _request) setState(() => _searching = false);
    }
  }

  Future<void> _detail(List<CatalogFilm> matches) async {
    setState(() {
      _showDetail = true;
      _detailing = true;
      _error = null;
      _details = [];
      _lines = [];
      _line = 0;
      _episodePage = 0;
    });
    final request = _request;
    var cursor = 0;
    final candidates = matches.take(8).toList();
    final details = <CatalogFilm>[];
    Future<void> worker() async {
      while (cursor < candidates.length) {
        final candidate = candidates[cursor++];
        try {
          final film = await widget.api.catalogDetail(
            widget.session,
            candidate,
          );
          if (mounted && request == _request) details.add(film);
        } catch (_) {
          /* Other sources remain available. */
        }
      }
    }

    await Future.wait(
      List.generate(math.min(4, candidates.length), (_) => worker()),
    );
    if (!mounted || request != _request) return;
    details.sort(
      (a, b) => matches
          .indexWhere((m) => m.id == a.id && m.sourceId == a.sourceId)
          .compareTo(
            matches.indexWhere((m) => m.id == b.id && m.sourceId == b.sourceId),
          ),
    );
    setState(() {
      _details = details;
      _lines = [
        for (final film in details)
          for (final group in film.groups) (film, group),
      ];
      _episode = _lines.isEmpty ? '' : _lines.first.$2.episodes.first.id;
      _detailing = false;
      if (_lines.isEmpty) _error = '暂未找到可直连的播放线路，可选择其他片源或稍后重试。';
    });
  }

  Map<String, dynamic> _selection() {
    final chosen = _lines[_line];
    final refs = [chosen, ..._lines.where((l) => l != chosen)].take(8);
    return {
      'refs': refs
          .map(
            (line) => {
              'source_id': line.$1.sourceId,
              'film_id': line.$1.id,
              'group_id': line.$2.id,
            },
          )
          .toList(),
      'episode_id': _episode,
      'all_episodes': _allEpisodes,
    };
  }

  Future<void> _play({bool existing = false}) async {
    if (_busy || _lines.isEmpty || _episode.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      Room room;
      if (existing) {
        final code = _roomCode.text.trim().toUpperCase();
        if (!RegExp(r'^[A-Z0-9]{6}$').hasMatch(code)) {
          setState(() => _error = '填写已加入房间的 6 位房间码。');
          return;
        }
        room = await widget.api.getRoom(widget.session, code);
        if (room.features?.permissions.playlist != true) {
          setState(() => _error = '你没有这个房间的片单编辑权限。');
          return;
        }
        room = await widget.api.appendCatalogPlaylist(
          widget.session,
          room,
          _selection(),
        );
      } else {
        room = await widget.api.createRoom(
          session: widget.session,
          name: '${_lines[_line].$1.title} · 一起看',
          catalog: _selection(),
        );
      }
      if (mounted) widget.onOpenRoom(room);
    } catch (e) {
      if (mounted) setState(() => _error = _message(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Widget _poster(String raw, {double width = 90}) {
    final c = FrameColors.of(context);
    final fallback = ColoredBox(
      color: c.elevated,
      child: Center(child: Icon(Icons.movie_creation_outlined, color: c.muted)),
    );
    return ClipRRect(
      borderRadius: BorderRadius.circular(10),
      child: SizedBox(
        width: width,
        height: width * 1.5,
        child: raw.isEmpty
            ? fallback
            : Image.network(
                Uri.parse(widget.api.baseUrl).resolve(raw).toString(),
                fit: BoxFit.cover,
                cacheWidth: 400,
                errorBuilder: (_, error, stack) => fallback,
              ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = FrameColors.of(context);
    if (widget.initialFilm != null) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_detailing) const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _buildDetail()),
        ],
      );
    }
    final groups = groupCatalogFilms(_films);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _query,
                  textInputAction: TextInputAction.search,
                  onSubmitted: (_) => _search(),
                  decoration: InputDecoration(
                    hintText: '搜索电影、剧集或频道',
                    prefixIcon: const Icon(Icons.search_rounded),
                    suffixIcon: IconButton(
                      onPressed: () => _search(),
                      tooltip: '搜索片源',
                      icon: const Icon(Icons.arrow_forward_rounded),
                    ),
                  ),
                ),
              ),
              if (widget.onMetadata != null)
                IconButton(
                  onPressed: widget.onMetadata,
                  tooltip: '影片资料',
                  icon: const Icon(Icons.info_outline_rounded),
                ),
            ],
          ),
        ),
        if (_searching || _detailing)
          const LinearProgressIndicator(minHeight: 2),
        Expanded(
          child: _showDetail
              ? _buildDetail()
              : CustomScrollView(
                  slivers: [
                    SliverToBoxAdapter(
                      child: Padding(
                        padding: const EdgeInsets.fromLTRB(20, 4, 20, 12),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              _searching
                                  ? '已完成 $_done / ${_sources.length} 个片源 · 结果陆续呈现'
                                  : '${_sources.length} 个片源 · ${groups.length} 部影片 / 频道',
                              style: TextStyle(color: c.muted, fontSize: 12),
                            ),
                            if (_error != null)
                              Padding(
                                padding: const EdgeInsets.only(top: 12),
                                child: Text(_error!),
                              ),
                            if (_failures.isNotEmpty)
                              ExpansionTile(
                                tilePadding: EdgeInsets.zero,
                                title: Text(
                                  '${_failures.length} 个片源暂不可用',
                                  style: const TextStyle(fontSize: 12),
                                ),
                                children: [
                                  for (final source in _sources.where(
                                    (s) => _failures.containsKey(s.id),
                                  ))
                                    ListTile(
                                      dense: true,
                                      title: Text(source.name),
                                      subtitle: Text(_failures[source.id]!),
                                    ),
                                ],
                              ),
                          ],
                        ),
                      ),
                    ),
                    if (groups.isEmpty && !_searching)
                      SliverFillRemaining(
                        hasScrollBody: false,
                        child: Center(
                          child: Padding(
                            padding: const EdgeInsets.all(28),
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Icon(
                                  Icons.video_library_outlined,
                                  size: 36,
                                  color: c.muted,
                                ),
                                const SizedBox(height: 16),
                                Text(
                                  _sources.isEmpty ? '片库等待上架' : '没有找到匹配影片',
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w600,
                                    fontSize: 17,
                                  ),
                                ),
                                const SizedBox(height: 8),
                                Text(
                                  _sources.isEmpty
                                      ? '管理员导入媒体订阅后，便可在这里搜索、选集与一起观看。'
                                      : '试试其他片名，或检查片源是否提供这部影片。',
                                  textAlign: TextAlign.center,
                                  style: TextStyle(
                                    color: c.muted,
                                    fontSize: 13,
                                  ),
                                ),
                                const SizedBox(height: 12),
                                TextButton(
                                  onPressed: () => _search(),
                                  child: const Text('刷新片源'),
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    SliverPadding(
                      padding: const EdgeInsets.symmetric(horizontal: 20),
                      sliver: SliverLayoutBuilder(
                        builder: (context, constraints) {
                          final columns = math.max(
                            2,
                            (constraints.crossAxisExtent / 155).floor(),
                          );
                          return SliverGrid.builder(
                            itemCount: groups.length,
                            gridDelegate:
                                SliverGridDelegateWithFixedCrossAxisCount(
                                  crossAxisCount: columns,
                                  crossAxisSpacing: 14,
                                  mainAxisSpacing: 20,
                                  childAspectRatio: .56,
                                ),
                            itemBuilder: (context, index) {
                              final matches = groups[index];
                              final f = matches.first;
                              return Semantics(
                                button: true,
                                label: '播放 ${f.title}',
                                excludeSemantics: true,
                                onTap: () => _detail(matches),
                                child: InkWell(
                                  borderRadius: BorderRadius.circular(12),
                                  onTap: () => _detail(matches),
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Expanded(
                                        child: SizedBox(
                                          width: double.infinity,
                                          child: _poster(f.poster, width: 180),
                                        ),
                                      ),
                                      const SizedBox(height: 9),
                                      Text(
                                        f.title,
                                        maxLines: 1,
                                        overflow: TextOverflow.ellipsis,
                                        style: const TextStyle(
                                          fontWeight: FontWeight.w600,
                                          fontSize: 13,
                                        ),
                                      ),
                                      const SizedBox(height: 5),
                                      Text(
                                        [
                                          if (f.year.isNotEmpty) f.year,
                                          f.kindLabel,
                                          '${matches.length} 个来源',
                                        ].join(' · '),
                                        maxLines: 1,
                                        overflow: TextOverflow.ellipsis,
                                        style: TextStyle(
                                          color: c.muted,
                                          fontSize: 11,
                                        ),
                                      ),
                                      if (f.remarks.isNotEmpty)
                                        Text(
                                          f.remarks,
                                          maxLines: 1,
                                          overflow: TextOverflow.ellipsis,
                                          style: TextStyle(
                                            color: c.muted,
                                            fontSize: 10,
                                          ),
                                        ),
                                    ],
                                  ),
                                ),
                              );
                            },
                          );
                        },
                      ),
                    ),
                    SliverToBoxAdapter(
                      child: Padding(
                        padding: const EdgeInsets.all(20),
                        child: _more.isNotEmpty
                            ? OutlinedButton(
                                onPressed: _searching
                                    ? null
                                    : () => _search(more: true),
                                child: const Text('加载更多片源结果'),
                              )
                            : const SizedBox(height: 20),
                      ),
                    ),
                  ],
                ),
        ),
      ],
    );
  }

  Widget _buildDetail() {
    final c = FrameColors.of(context);
    final film = _details.isEmpty ? null : _details.first;
    final episodes = _lines.isEmpty
        ? <CatalogEpisode>[]
        : _lines[_line].$2.episodes;
    final start = _episodePage * 50;
    final shown = episodes.skip(start).take(50);
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(20, 0, 20, 28),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              TextButton(
                onPressed: _busy
                    ? null
                    : widget.onCloseDetail ??
                          () => setState(() {
                            _showDetail = false;
                            _error = null;
                          }),
                child: Text(widget.initialFilm == null ? '全部搜索结果' : '短剧列表'),
              ),
              const Spacer(),
              if (_details.length > 1)
                Text(
                  '${_details.length} 个同片来源',
                  style: TextStyle(color: c.muted, fontSize: 12),
                ),
            ],
          ),
          if (film != null) ...[
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _poster(film.poster),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        film.title,
                        style: const TextStyle(
                          fontSize: 21,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        [
                          film.year,
                          film.kindLabel,
                          film.remarks,
                        ].where((s) => s.isNotEmpty).join(' · '),
                        style: TextStyle(color: c.muted, fontSize: 12),
                      ),
                      if (film.actors.isNotEmpty) ...[
                        const SizedBox(height: 10),
                        Text(
                          film.actors,
                          maxLines: 3,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(color: c.muted, fontSize: 12),
                        ),
                      ],
                    ],
                  ),
                ),
              ],
            ),
            if (film.summary.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 14),
                child: Text(
                  film.summary,
                  maxLines: 4,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: c.muted, fontSize: 12, height: 1.6),
                ),
              ),
          ],
          if (_error != null)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 16),
              child: Text(_error!, style: TextStyle(color: c.text)),
            ),
          if (widget.initialFilm != null && !_detailing && _lines.isEmpty)
            TextButton(
              onPressed: () => _detail([widget.initialFilm!]),
              child: const Text('重新加载分集'),
            ),
          if (_lines.isNotEmpty) ...[
            const SizedBox(height: 20),
            DropdownButtonFormField<int>(
              key: ValueKey(
                _lines.map((v) => '${v.$1.sourceId}:${v.$2.id}').join(','),
              ),
              initialValue: _line,
              isExpanded: true,
              decoration: const InputDecoration(labelText: '播放线路'),
              items: [
                for (var i = 0; i < _lines.length; i++)
                  DropdownMenuItem(
                    value: i,
                    child: Text(
                      '${_lines[i].$1.sourceName} · ${_lines[i].$2.name}',
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
              ],
              onChanged: _busy
                  ? null
                  : (i) {
                      if (i == null) return;
                      setState(() {
                        _line = i;
                        _episode = _lines[i].$2.episodes.first.id;
                        _episodePage = 0;
                      });
                    },
            ),
            const SizedBox(height: 14),
            Text(
              '选择剧集 · 共 ${episodes.length} 集',
              style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
            ),
            const SizedBox(height: 10),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final ep in shown)
                  ChoiceChip(
                    label: Text(ep.name),
                    selected: _episode == ep.id,
                    onSelected: _busy
                        ? null
                        : (_) => setState(() => _episode = ep.id),
                  ),
              ],
            ),
            if (episodes.length > 50)
              Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  IconButton(
                    onPressed: _episodePage > 0
                        ? () => setState(() => _episodePage--)
                        : null,
                    icon: const Icon(Icons.chevron_left_rounded),
                  ),
                  Text(
                    '${_episodePage + 1} / ${(episodes.length / 50).ceil()}',
                  ),
                  IconButton(
                    onPressed: start + 50 < episodes.length
                        ? () => setState(() => _episodePage++)
                        : null,
                    icon: const Icon(Icons.chevron_right_rounded),
                  ),
                ],
              ),
            if (episodes.length > 1)
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                dense: true,
                title: const Text('将全部剧集加入播放列表'),
                subtitle: const Text('同集可用线路将合并，换源保留播放位置'),
                value: _allEpisodes,
                onChanged: _busy
                    ? null
                    : (v) => setState(() => _allEpisodes = v ?? true),
              ),
            const SizedBox(height: 18),
            FilledButton.icon(
              onPressed: _busy ? null : () => _play(),
              icon: _busy
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.play_arrow_rounded),
              label: Text(_busy ? '正在准备播放…' : '创建房间 · 立即观看'),
            ),
            const SizedBox(height: 12),
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              title: const Text('加入已有房间的片单', style: TextStyle(fontSize: 13)),
              children: [
                TextField(
                  controller: _roomCode,
                  maxLength: 6,
                  textCapitalization: TextCapitalization.characters,
                  decoration: const InputDecoration(labelText: '已加入的房间码'),
                ),
                OutlinedButton(
                  onPressed: _busy ? null : () => _play(existing: true),
                  child: const Text('加入片单并进入房间'),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}
