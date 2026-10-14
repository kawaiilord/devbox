import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../catalog_models.dart';
import '../design.dart';
import '../models.dart';
import '../short_drama_models.dart';
import 'catalog_browser.dart';
import 'workspace_section.dart';

const _genres = [
  ('short_play', '真人剧', Icons.movie_outlined),
  ('comic_series', '漫剧', Icons.auto_stories_outlined),
  ('ai_series', 'AI 剧', Icons.auto_awesome_outlined),
  ('hot', '热播榜', Icons.leaderboard_outlined),
];
const _boards = [
  ('hot', '总热播榜'),
  ('real', '真人剧榜'),
  ('comic', '漫剧榜'),
  ('ai', 'AI 剧榜'),
];

class ShortDramaPage extends StatefulWidget {
  const ShortDramaPage({
    super.key,
    required this.api,
    required this.session,
    required this.onOpenRoom,
  });
  final ApiClient api;
  final Session session;
  final ValueChanged<Room> onOpenRoom;
  @override
  State<ShortDramaPage> createState() => _ShortDramaPageState();
}

class _ShortDramaPageState extends State<ShortDramaPage> {
  final _query = TextEditingController();
  final _scroll = ScrollController();
  String _genre = 'short_play', _board = 'hot', _search = '';
  List<CatalogFilm> _items = [];
  ShortDramaFeed? _feed;
  CatalogFilm? _selected;
  bool _loading = true, _loadingMore = false;
  String? _error;
  int _request = 0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _request++;
    _query.dispose();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load({bool more = false, bool reset = false}) async {
    if (more && (_loading || _loadingMore || _feed?.hasMore != true)) return;
    final request = ++_request;
    setState(() {
      _error = null;
      _loading = !more;
      _loadingMore = more;
      if (reset) {
        _items = [];
        _feed = null;
      }
    });
    try {
      final feed = await widget.api.shortDramas(widget.session, {
        'section': _search.isNotEmpty
            ? 'search'
            : (_genre == 'hot' ? _board : _genre),
        'query': _search,
        'offset': more ? _feed?.offset ?? 0 : 0,
        'session': more ? _feed?.session ?? '' : '',
        'seen': more
            ? _items
                  .skip(math.max(0, _items.length - 540))
                  .map((i) => i.id)
                  .toList()
            : <String>[],
        'page': more ? (_feed?.page ?? 0) + 1 : 1,
      });
      if (!mounted || request != _request) return;
      setState(() {
        final seen = more ? _items.map((f) => f.id).toSet() : <String>{};
        _items = [
          if (more) ..._items,
          ...feed.items.where((i) => seen.add(i.id)),
        ];
        _feed = feed;
      });
    } catch (e) {
      if (mounted && request == _request)
        setState(
          () => _error = e is ApiException ? e.message : '短剧暂时连接失败，请重试。',
        );
    } finally {
      if (mounted && request == _request)
        setState(() {
          _loading = false;
          _loadingMore = false;
        });
    }
  }

  void _choose(String genre) {
    setState(() {
      _genre = genre;
      _search = '';
      _query.clear();
    });
    if (_scroll.hasClients) _scroll.jumpTo(0);
    _load(reset: true);
  }

  void _submit() {
    final text = _query.text.trim();
    if (text == _search && _loading) return;
    setState(() => _search = text);
    if (_scroll.hasClients) _scroll.jumpTo(0);
    _load(reset: true);
  }

  @override
  Widget build(BuildContext context) {
    final c = FrameColors.of(context);
    if (_selected != null) {
      return WorkspaceSection(
        title: const Text('果果短剧'),
        body: CatalogBrowser(
          key: ValueKey('short-drama-${_selected!.id}'),
          api: widget.api,
          session: widget.session,
          initialFilm: _selected,
          onCloseDetail: () => setState(() => _selected = null),
          onOpenRoom: widget.onOpenRoom,
        ),
      );
    }
    return WorkspaceSection(
      title: const Text('果果短剧'),
      actions: [
        IconButton(
          tooltip: '刷新短剧',
          onPressed: _loading ? null : () => _load(),
          icon: const Icon(Icons.refresh_rounded),
        ),
      ],
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 2, 20, 18),
            child: TextField(
              controller: _query,
              maxLength: 80,
              textInputAction: TextInputAction.search,
              onSubmitted: (_) => _submit(),
              decoration: InputDecoration(
                counterText: '',
                hintText: '搜索短剧名称，发现下一个好故事',
                prefixIcon: const Icon(Icons.search_rounded),
                suffixIcon: IconButton(
                  tooltip: '搜索短剧',
                  onPressed: _submit,
                  icon: const Icon(Icons.arrow_forward_rounded),
                ),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                for (final genre in _genres)
                  ChoiceChip(
                    label: Text(genre.$2),
                    avatar: Icon(genre.$3, size: 17),
                    selected: _search.isEmpty && _genre == genre.$1,
                    onSelected: (_) => _choose(genre.$1),
                  ),
                if (_search.isNotEmpty)
                  InputChip(
                    label: Text('“$_search”'),
                    onDeleted: () => _choose(_genre),
                  ),
              ],
            ),
          ),
          if (_genre == 'hot' && _search.isEmpty)
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
              child: Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final board in _boards)
                    ChoiceChip(
                      label: Text(board.$2),
                      selected: _board == board.$1,
                      onSelected: (_) {
                        setState(() => _board = board.$1);
                        _load(reset: true);
                      },
                    ),
                ],
              ),
            ),
          const SizedBox(height: 16),
          if (_loading || _loadingMore)
            const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: CustomScrollView(
              controller: _scroll,
              key: const PageStorageKey('guoguo-posters'),
              slivers: [
                SliverToBoxAdapter(
                  child: Padding(
                    padding: const EdgeInsets.fromLTRB(20, 4, 20, 18),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          _loading && _items.isEmpty
                              ? '正在寻找好故事…'
                              : '已展示 ${_items.length} 部短剧 · 选集后即可一起看',
                          style: TextStyle(color: c.muted, fontSize: 12),
                        ),
                        if (_feed?.warning.isNotEmpty == true)
                          Padding(
                            padding: const EdgeInsets.only(top: 10),
                            child: Text(
                              _feed!.warning,
                              style: TextStyle(color: c.muted),
                            ),
                          ),
                        if (_error != null)
                          Padding(
                            padding: const EdgeInsets.only(top: 14),
                            child: Row(
                              children: [
                                const Icon(
                                  Icons.info_outline_rounded,
                                  size: 18,
                                ),
                                const SizedBox(width: 10),
                                Expanded(child: Text(_error!)),
                                TextButton(
                                  onPressed: () => _load(
                                    more:
                                        _items.isNotEmpty &&
                                        _feed?.hasMore == true,
                                  ),
                                  child: const Text('重试'),
                                ),
                              ],
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
                if (_items.isEmpty && !_loading)
                  SliverFillRemaining(
                    hasScrollBody: false,
                    child: Center(
                      child: Padding(
                        padding: const EdgeInsets.all(32),
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.video_library_outlined,
                              size: 42,
                              color: c.muted,
                            ),
                            const SizedBox(height: 18),
                            Text(
                              _error == null ? '暂时没有找到短剧' : '短剧暂不可用',
                              style: const TextStyle(
                                fontSize: 20,
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            const SizedBox(height: 10),
                            Text(
                              _search.isEmpty
                                  ? '稍后刷新试试，或搜索你感兴趣的剧名。'
                                  : '试试更短的剧名或其他关键词。',
                              style: TextStyle(color: c.muted),
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
                        (constraints.crossAxisExtent / 190).floor(),
                      );
                      final width =
                          (constraints.crossAxisExtent - (columns - 1) * 18) /
                          columns;
                      return SliverGrid.builder(
                        itemCount: _items.length,
                        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                          crossAxisCount: columns,
                          crossAxisSpacing: 18,
                          mainAxisSpacing: 22,
                          mainAxisExtent: width * 1.4 + 68,
                        ),
                        itemBuilder: (context, index) =>
                            _card(_items[index], width),
                      );
                    },
                  ),
                ),
                SliverToBoxAdapter(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Center(
                      child: _feed?.hasMore == true
                          ? OutlinedButton.icon(
                              onPressed: _loading || _loadingMore
                                  ? null
                                  : () => _load(more: true),
                              icon: const Icon(Icons.add_rounded, size: 18),
                              label: Text(_loadingMore ? '正在加载…' : '发现更多短剧'),
                            )
                          : _items.isNotEmpty
                          ? Text(
                              '当前内容已展示完',
                              style: TextStyle(color: c.muted, fontSize: 12),
                            )
                          : const SizedBox.shrink(),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _card(CatalogFilm film, double width) {
    final c = FrameColors.of(context);
    final fallback = ColoredBox(
      color: c.elevated,
      child: Center(
        child: Icon(Icons.movie_creation_outlined, color: c.muted, size: 32),
      ),
    );
    return Semantics(
      button: true,
      label: '${film.title}，查看分集',
      child: Material(
        color: Colors.transparent,
        child: InkWell(
          borderRadius: BorderRadius.circular(12),
          onTap: () => setState(() => _selected = film),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ClipRRect(
                borderRadius: BorderRadius.circular(12),
                child: SizedBox(
                  height: width * 1.4,
                  width: width,
                  child: Stack(
                    fit: StackFit.expand,
                    children: [
                      film.poster.isEmpty
                          ? fallback
                          : Image.network(
                              Uri.parse(widget.api.baseUrl)
                                  .resolve(film.poster)
                                  .toString(),
                              fit: BoxFit.cover,
                              cacheWidth: 480,
                              errorBuilder: (_, e, stack) => fallback,
                            ),
                      Positioned(
                        right: 10,
                        bottom: 10,
                        child: Container(
                          padding: const EdgeInsets.all(8),
                          decoration: BoxDecoration(
                            color: Colors.black.withValues(alpha: .7),
                            shape: BoxShape.circle,
                          ),
                          child: const Icon(
                            Icons.play_arrow_rounded,
                            size: 20,
                            color: Colors.white,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 11),
              Text(
                film.title,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 14,
                ),
              ),
              const SizedBox(height: 5),
              Text(
                film.remarks.isEmpty ? '短剧 · 点击选集' : film.remarks,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(fontSize: 11, color: c.muted),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
