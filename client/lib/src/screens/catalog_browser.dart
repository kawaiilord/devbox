import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../catalog_models.dart';
import '../design.dart';
import '../models.dart';
import '../player/catalog_resolver.dart';
import 'watch_page.dart';

class CatalogBrowser extends StatefulWidget {
  const CatalogBrowser({
    super.key,
    required this.api,
    required this.session,
    required this.onOpenRoom,
    this.initialQuery = '',
  });
  final ApiClient api;
  final Session session;
  final ValueChanged<Room> onOpenRoom;
  final String initialQuery;
  @override
  State<CatalogBrowser> createState() => _CatalogBrowserState();
}

class _CatalogBrowserState extends State<CatalogBrowser> {
  late final TextEditingController _query;
  List<CatalogSource> _sources = [];
  final List<CatalogFilm> _films = [];
  final Map<String, int> _pages = {};
  final Set<String> _more = {};
  final Map<String, String> _failures = {};
  bool _searching = false;
  int _request = 0, _done = 0;
  String _searchQuery = '';
  String? _error;

  @override
  void initState() {
    super.initState();
    _query = TextEditingController(text: widget.initialQuery);
    _search();
  }

  @override
  void dispose() {
    _request++;
    _query.dispose();
    super.dispose();
  }

  String _message(Object error) =>
      error is ApiException ? error.message : '暂时连接失败，请重试。';

  Future<void> _search({bool more = false}) async {
    final request = ++_request;
    setState(() {
      _error = null;
      _searching = true;
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
    } catch (e) {
      if (mounted && request == _request) setState(() => _error = _message(e));
    } finally {
      if (mounted && request == _request) setState(() => _searching = false);
    }
  }

  Future<void> _detail(List<CatalogFilm> matches) async {
    final room = await openWatchPage(
      context,
      api: widget.api,
      session: widget.session,
      target: WatchTarget.film(matches.first),
      films: matches,
    );
    if (mounted && room != null) widget.onOpenRoom(room);
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
            ],
          ),
        ),
        if (_searching) const LinearProgressIndicator(minHeight: 2),
        Expanded(
          child: CustomScrollView(
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
                            style: TextStyle(color: c.muted, fontSize: 13),
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
                      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
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
                              crossAxisAlignment: CrossAxisAlignment.start,
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
}
