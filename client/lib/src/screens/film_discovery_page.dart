import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../design.dart';
import '../discovery_models.dart';
import '../models.dart';
import 'workspace_section.dart';

const _sections = [
  ('movies', '热门电影'),
  ('series', '剧集分类'),
  ('top', '高分榜'),
  ('anime', '新番放送'),
];
const _series = [
  ('all', '热门'),
  ('cn', '国产剧'),
  ('us', '美剧'),
  ('uk', '英剧'),
  ('jp', '日剧'),
  ('kr', '韩剧'),
  ('hk', '港剧'),
  ('anime', '日本动画'),
  ('variety', '综艺'),
  ('documentary', '纪录片'),
];
const _weekdays = ['今日', '周一', '周二', '周三', '周四', '周五', '周六', '周日'];

class FilmDiscoveryPage extends StatefulWidget {
  const FilmDiscoveryPage({
    super.key,
    required this.api,
    required this.session,
  });
  final ApiClient api;
  final Session session;
  @override
  State<FilmDiscoveryPage> createState() => _FilmDiscoveryPageState();
}

class _FilmDiscoveryPageState extends State<FilmDiscoveryPage> {
  final _scroll = ScrollController();
  String _section = 'movies', _category = 'all';
  int _weekday = 0, _request = 0;
  DiscoveryFeed? _feed;
  List<DiscoveryItem> _items = [];
  bool _loading = true, _loadingMore = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _request++;
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load({bool more = false}) async {
    if (more && (_loading || _loadingMore || _feed?.hasMore != true)) return;
    final request = ++_request;
    setState(() {
      _error = null;
      if (more) {
        _loadingMore = true;
      } else {
        _loading = true;
        _loadingMore = false;
      }
    });
    try {
      final feed = await widget.api.discoverFilms(
        widget.session,
        section: _section,
        category: _category,
        page: more ? (_feed?.page ?? 0) + 1 : 1,
        weekday: _weekday,
      );
      if (!mounted || request != _request) return;
      setState(() {
        final known = more ? _items.map((i) => i.id).toSet() : <String>{};
        _items = [
          if (more) ..._items,
          ...feed.items.where((i) => known.add(i.id)),
        ];
        _feed = feed;
      });
    } catch (error) {
      if (mounted && request == _request) {
        setState(
          () => _error = error is ApiException
              ? error.message
              : '暂时无法加载影视资料，请稍后重试。',
        );
      }
    } finally {
      if (mounted && request == _request) {
        setState(() {
          _loading = false;
          _loadingMore = false;
        });
      }
    }
  }

  void _selectSection(String section) {
    if (section == _section) return;
    setState(() {
      _section = section;
      _category = section == 'top' ? 'movie' : 'all';
      _weekday = 0;
      _items = [];
      _feed = null;
    });
    if (_scroll.hasClients) _scroll.jumpTo(0);
    _load();
  }

  void _selectCategory(String category) {
    if (category == _category) return;
    setState(() {
      _category = category;
      _items = [];
      _feed = null;
    });
    if (_scroll.hasClients) _scroll.jumpTo(0);
    _load();
  }

  void _selectWeekday(int day) {
    if (day == _weekday) return;
    setState(() {
      _weekday = day;
      _items = [];
      _feed = null;
    });
    if (_scroll.hasClients) _scroll.jumpTo(0);
    _load();
  }

  String _poster(DiscoveryItem item) => item.poster.isEmpty
      ? ''
      : Uri.parse(widget.api.baseUrl).resolve(item.poster).toString();

  Future<void> _detail(DiscoveryItem item) async {
    final source = _feed?.source ?? '';
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      backgroundColor: FrameColors.of(context).surface,
      constraints: const BoxConstraints(maxWidth: 680),
      builder: (sheet) => DraggableScrollableSheet(
        initialChildSize: item.summary.isEmpty ? .62 : .78,
        minChildSize: .45,
        maxChildSize: .94,
        expand: false,
        builder: (context, controller) => SingleChildScrollView(
          controller: controller,
          padding: const EdgeInsets.fromLTRB(22, 0, 22, 28),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  SizedBox(
                    width: 112,
                    child: AspectRatio(
                      aspectRatio: 2 / 3,
                      child: ClipRRect(
                        borderRadius: BorderRadius.circular(10),
                        child: _Poster(url: _poster(item)),
                      ),
                    ),
                  ),
                  const SizedBox(width: 18),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          item.title,
                          style: const TextStyle(
                            fontSize: 22,
                            height: 1.4,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        if (item.original.isNotEmpty &&
                            item.original != item.title) ...[
                          const SizedBox(height: 7),
                          Text(
                            item.original,
                            style: TextStyle(
                              fontSize: 12,
                              color: FrameColors.of(context).muted,
                            ),
                          ),
                        ],
                        const SizedBox(height: 12),
                        Text(
                          '$source · ${item.kindLabel}',
                          style: TextStyle(
                            fontSize: 12,
                            color: FrameColors.of(context).muted,
                          ),
                        ),
                        const SizedBox(height: 12),
                        Text(
                          item.rating > 0
                              ? '${item.rating.toStringAsFixed(1)} 分'
                              : '暂无评分',
                          style: const TextStyle(
                            fontSize: 25,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        if (item.votes > 0)
                          Text(
                            '${item.votes} 人评分',
                            style: TextStyle(
                              fontSize: 11,
                              color: FrameColors.of(context).muted,
                            ),
                          ),
                      ],
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 24),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  _InfoPill(item.kindLabel),
                  if (item.date.isNotEmpty) _InfoPill('${item.date} 开播'),
                  if (item.weekday > 0)
                    _InfoPill('${_weekdays[item.weekday]}放送'),
                  if (item.episodes.isNotEmpty) _InfoPill(item.episodes),
                  if (item.isNew) const _InfoPill('新上榜'),
                ],
              ),
              const SizedBox(height: 22),
              Text(
                item.summary.isEmpty ? '前往资料来源，了解更多剧情与演职员信息。' : item.summary,
                style: TextStyle(
                  fontSize: 14,
                  height: 1.9,
                  color: FrameColors.of(context).muted,
                ),
              ),
              const SizedBox(height: 28),
              Row(
                children: [
                  Expanded(
                    child: OutlinedButton.icon(
                      onPressed: () async {
                        await Clipboard.setData(
                          ClipboardData(text: item.title),
                        );
                        if (sheet.mounted) {
                          ScaffoldMessenger.of(sheet).showSnackBar(
                            const SnackBar(content: Text('片名已复制')),
                          );
                        }
                      },
                      icon: const Icon(Icons.copy_outlined, size: 16),
                      label: const Text('复制片名'),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: FilledButton.icon(
                      onPressed: () async {
                        final opened = await launchUrl(
                          Uri.parse(item.url),
                          mode: LaunchMode.externalApplication,
                        );
                        if (!opened && sheet.mounted) {
                          ScaffoldMessenger.of(sheet).showSnackBar(
                            const SnackBar(content: Text('暂时无法打开资料页')),
                          );
                        }
                      },
                      icon: const Icon(Icons.open_in_new_rounded, size: 16),
                      label: const Text('查看资料'),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 16),
              Text(
                '资料与评分来自 $source',
                style: TextStyle(
                  fontSize: 11,
                  color: FrameColors.of(context).muted,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    final filters = switch (_section) {
      'movies' => [('all', '正在热门'), ('new', '最近新片')],
      'series' => _series,
      'top' => [('movie', '高分电影'), ('tv', '高分剧集')],
      _ => <(String, String)>[],
    };
    return WorkspaceSection(
      title: const Text('影视发现'),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  for (final (id, label) in _sections)
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: _DiscoveryTab(
                        label: label,
                        selected: _section == id,
                        onTap: () => _selectSection(id),
                      ),
                    ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 18),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Row(
              children: [
                Expanded(
                  child: SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    child: Row(
                      children: [
                        if (_section == 'anime')
                          for (var day = 0; day < 8; day++)
                            Padding(
                              padding: const EdgeInsets.only(right: 7),
                              child: _Filter(
                                label: _weekdays[day],
                                selected: _weekday == day,
                                onTap: () => _selectWeekday(day),
                              ),
                            ),
                        for (final (id, label) in filters)
                          Padding(
                            padding: const EdgeInsets.only(right: 7),
                            child: _Filter(
                              label: label,
                              selected: _category == id,
                              onTap: () => _selectCategory(id),
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(width: 6),
                IconButton(
                  onPressed: _loading ? null : () => _load(),
                  tooltip: '刷新影视内容',
                  icon: const Icon(Icons.refresh_rounded, size: 20),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 10, 20, 12),
            child: Row(
              children: [
                Icon(
                  _section == 'anime'
                      ? Icons.calendar_month_outlined
                      : Icons.local_movies_outlined,
                  size: 15,
                  color: colors.muted,
                ),
                const SizedBox(width: 7),
                Expanded(
                  child: Text(
                    _section == 'anime'
                        ? '每周新番，按放送日查看。'
                        : _section == 'top'
                        ? '从当前片单精选 8 分以上作品，按评分排序。'
                        : '挑一张海报，发现下一场好故事。',
                    style: TextStyle(fontSize: 12, color: colors.muted),
                  ),
                ),
                if (_feed != null)
                  Text(
                    _feed!.source,
                    style: TextStyle(fontSize: 10, color: colors.muted),
                  ),
              ],
            ),
          ),
          if (_feed?.stale == true)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 6),
              child: Text(
                '暂时无法更新，正在展示最近一次获取的内容。',
                style: TextStyle(fontSize: 12, color: colors.muted),
              ),
            ),
          Expanded(
            child: _loading
                ? _skeleton()
                : _items.isEmpty
                ? _empty()
                : _grid(),
          ),
        ],
      ),
    );
  }

  Widget _empty() => Center(
    child: Padding(
      padding: const EdgeInsets.all(26),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            _error != null
                ? Icons.cloud_off_outlined
                : Icons.movie_filter_outlined,
            size: 38,
            color: FrameColors.of(context).muted,
          ),
          const SizedBox(height: 16),
          Text(
            _error ?? (_section == 'anime' ? '这一天暂无放送条目。' : '这个分类暂时没有内容。'),
            textAlign: TextAlign.center,
            style: TextStyle(height: 1.7, color: FrameColors.of(context).muted),
          ),
          const SizedBox(height: 14),
          OutlinedButton.icon(
            onPressed: () => _load(),
            icon: const Icon(Icons.refresh_rounded, size: 17),
            label: const Text('重新加载'),
          ),
        ],
      ),
    ),
  );

  Widget _skeleton() => LayoutBuilder(
    builder: (context, box) {
      final columns = (box.maxWidth / 185).floor().clamp(2, 6).toInt();
      return GridView.builder(
        padding: const EdgeInsets.fromLTRB(20, 5, 20, 20),
        itemCount: columns * 2,
        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: columns,
          crossAxisSpacing: 14,
          mainAxisSpacing: 20,
          childAspectRatio: .58,
        ),
        itemBuilder: (_, index) => DecoratedBox(
          decoration: BoxDecoration(
            color: FrameColors.of(context).elevated,
            borderRadius: BorderRadius.circular(13),
          ),
          child: index == 0
              ? const Center(
                  child: SizedBox(
                    width: 22,
                    height: 22,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                )
              : const SizedBox.expand(),
        ),
      );
    },
  );

  Widget _grid() => LayoutBuilder(
    builder: (context, box) {
      final columns = (box.maxWidth / 185).floor().clamp(2, 6).toInt();
      final width = (box.maxWidth - 40 - 14 * (columns - 1)) / columns;
      return RefreshIndicator(
        onRefresh: _load,
        child: CustomScrollView(
          controller: _scroll,
          physics: const AlwaysScrollableScrollPhysics(),
          slivers: [
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 5, 20, 0),
              sliver: SliverGrid.builder(
                itemCount: _items.length,
                gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: columns,
                  crossAxisSpacing: 14,
                  mainAxisSpacing: 22,
                  mainAxisExtent: width * 1.5 + 68,
                ),
                itemBuilder: (context, index) {
                  final item = _items[index];
                  return _PosterCard(
                    item: item,
                    poster: _poster(item),
                    onTap: () => _detail(item),
                  );
                },
              ),
            ),
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 28),
                child: Column(
                  children: [
                    if (_error != null) ...[
                      Text(
                        _error!,
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          color: Theme.of(context).colorScheme.error,
                        ),
                      ),
                      const SizedBox(height: 10),
                    ],
                    if (_feed?.hasMore == true)
                      OutlinedButton(
                        onPressed: _loadingMore
                            ? null
                            : () => _load(more: true),
                        child: _loadingMore
                            ? const SizedBox(
                                width: 18,
                                height: 18,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                ),
                              )
                            : Text(_error == null ? '加载更多' : '重试加载'),
                      )
                    else
                      Text(
                        _section == 'anime' ? '放送安排以平台实际更新为准' : '已显示当前分类的全部内容',
                        style: TextStyle(
                          fontSize: 11,
                          color: FrameColors.of(context).muted,
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      );
    },
  );
}

class _DiscoveryTab extends StatelessWidget {
  const _DiscoveryTab({
    required this.label,
    required this.selected,
    required this.onTap,
  });
  final String label;
  final bool selected;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final c = FrameColors.of(context);
    return Semantics(
      button: true,
      selected: selected,
      child: Material(
        color: selected ? c.text : c.surface,
        borderRadius: BorderRadius.circular(10),
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(10),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: selected ? c.background : c.muted,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _Filter extends StatelessWidget {
  const _Filter({
    required this.label,
    required this.selected,
    required this.onTap,
  });
  final String label;
  final bool selected;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    return Semantics(
      selected: selected,
      child: OutlinedButton(
        onPressed: onTap,
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(48, 36),
          padding: const EdgeInsets.symmetric(horizontal: 13),
          foregroundColor: selected ? colors.text : colors.muted,
          backgroundColor: selected ? colors.elevated : colors.background,
          side: BorderSide(color: selected ? colors.text : colors.border),
          shape: const StadiumBorder(),
          textStyle: const TextStyle(fontSize: 12, height: 1.3),
        ),
        child: Text(label, softWrap: false),
      ),
    );
  }
}

class _PosterCard extends StatelessWidget {
  const _PosterCard({
    required this.item,
    required this.poster,
    required this.onTap,
  });
  final DiscoveryItem item;
  final String poster;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final c = FrameColors.of(context);
    return Semantics(
      button: true,
      label: '查看 ${item.title} 的资料',
      onTap: onTap,
      excludeSemantics: true,
      child: Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              AspectRatio(
                aspectRatio: 2 / 3,
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(12),
                  child: Stack(
                    fit: StackFit.expand,
                    children: [
                      _Poster(url: poster),
                      if (item.rating > 0)
                        Positioned(
                          top: 9,
                          right: 9,
                          child: _PosterBadge(item.rating.toStringAsFixed(1)),
                        ),
                      if (item.rank > 0)
                        Positioned(
                          top: 9,
                          left: 9,
                          child: _PosterBadge(
                            '#${item.rank.toString().padLeft(2, '0')}',
                          ),
                        )
                      else if (item.isNew)
                        const Positioned(
                          top: 9,
                          left: 9,
                          child: _PosterBadge('新'),
                        ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 10),
              Text(
                item.title,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 5),
              Text(
                item.episodes.isNotEmpty
                    ? item.episodes
                    : item.date.isNotEmpty
                    ? '${item.date} · ${item.kindLabel}'
                    : item.kindLabel,
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

class _Poster extends StatelessWidget {
  const _Poster({required this.url});
  final String url;
  @override
  Widget build(BuildContext context) {
    final c = FrameColors.of(context);
    Widget fallback() => ColoredBox(
      color: c.elevated,
      child: Center(
        child: Icon(Icons.movie_creation_outlined, size: 32, color: c.muted),
      ),
    );
    return url.isEmpty
        ? fallback()
        : Image.network(
            url,
            fit: BoxFit.cover,
            cacheWidth: 440,
            excludeFromSemantics: true,
            frameBuilder: (_, child, frame, synchronous) =>
                frame == null && !synchronous ? fallback() : child,
            errorBuilder: (_, error, stack) => fallback(),
          );
  }
}

class _PosterBadge extends StatelessWidget {
  const _PosterBadge(this.text);
  final String text;
  @override
  Widget build(BuildContext context) => DecoratedBox(
    decoration: BoxDecoration(
      color: Colors.black.withValues(alpha: .78),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Padding(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 4),
      child: Text(
        text,
        style: const TextStyle(
          color: Colors.white,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    ),
  );
}

class _InfoPill extends StatelessWidget {
  const _InfoPill(this.text);
  final String text;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
    decoration: BoxDecoration(
      color: FrameColors.of(context).elevated,
      borderRadius: BorderRadius.circular(6),
    ),
    child: Text(text, style: const TextStyle(fontSize: 11)),
  );
}
