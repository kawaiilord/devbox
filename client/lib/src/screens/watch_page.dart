import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../appearance.dart';
import '../catalog_models.dart';
import '../design.dart';
import '../models.dart';
import '../player/catalog_resolver.dart';
import '../player/watch_history.dart';
import 'catalog_player.dart';
import 'owned_room_picker.dart';

Future<Room?> openWatchPage(
  BuildContext context, {
  required ApiClient api,
  required Session session,
  required WatchTarget target,
  List<CatalogFilm> films = const [],
  VoidCallback? onMetadata,
}) => Navigator.of(context).push<Room>(
  MaterialPageRoute(
    builder: (_) => WatchPage(
      api: api,
      session: session,
      target: target,
      films: films,
      onMetadata: onMetadata,
    ),
  ),
);

class WatchPage extends StatefulWidget {
  const WatchPage({
    super.key,
    required this.api,
    required this.session,
    required this.target,
    this.films = const [],
    this.onMetadata,
  });
  final ApiClient api;
  final Session session;
  final WatchTarget target;
  final List<CatalogFilm> films;
  final VoidCallback? onMetadata;
  @override
  State<WatchPage> createState() => _WatchPageState();
}

class _WatchPageState extends State<WatchPage> {
  final _player = GlobalKey<CatalogPlayerState>();
  late final CatalogResolver _resolver;
  late final WatchHistory _history;
  CatalogLine? _line;
  int _episode = 0, _episodePage = 0, _intro = 0, _outro = 0;
  double _start = 0, _rate = 1;
  bool _startPlaying = true,
      _busy = false,
      _wide = false,
      _sourcesTab = false,
      _descending = false,
      _autoNext = true,
      _allEpisodes = true,
      _failed = false;
  String? _notice, _roomError;
  final Set<String> _attempted = {};
  Map<String, dynamic>? _resume;

  String get _identity =>
      _line == null ? '' : '${_line!.id}:${_line!.group.episodes[_episode].id}';
  List<CatalogEpisode> get _episodes => _line?.group.episodes ?? [];
  @override
  void initState() {
    super.initState();
    _resolver = CatalogResolver(widget.api, widget.session, widget.target)
      ..addListener(_resolved);
    _history = WatchHistory(widget.session.user.id, widget.target.key);
    unawaited(_startResolve());
  }

  Future<void> _startResolve() async {
    _resume = await _history.read();
    if (!mounted) return;
    _rate = ((_resume?['rate'] as num?)?.toDouble() ?? 1).clamp(.5, 3);
    _intro = ((_resume?['intro'] as num?)?.toInt() ?? 0).clamp(0, 600);
    _outro = ((_resume?['outro'] as num?)?.toInt() ?? 0).clamp(0, 600);
    await _resolver.resolve(seeds: widget.films);
  }

  @override
  void dispose() {
    _resolver.removeListener(_resolved);
    _resolver.dispose();
    super.dispose();
  }

  void _resolved() {
    if (!mounted) return;
    setState(() {
      if (_line == null && _resolver.lines.isNotEmpty) {
        _line =
            _resolver.lines
                .where((l) => l.id == _resume?['line'])
                .firstOrNull ??
            _resolver.lines.first;
        final savedName = _resume?['episode_name'] as String? ?? '';
        final index = _episodes.indexWhere(
          (e) =>
              e.name == savedName ||
              (episodeNumber(savedName) != null &&
                  episodeNumber(e.name) == episodeNumber(savedName)),
        );
        _episode = index >= 0 ? index : 0;
        _episodePage = _episode ~/ 50;
        if (index >= 0) {
          final pos = (_resume?['position'] as num?)?.toDouble() ?? 0;
          final duration = (_resume?['duration'] as num?)?.toDouble() ?? 0;
          _start =
              pos.isFinite && pos > 0 && (duration <= 0 || pos < duration - 8)
              ? pos
              : 0;
          if (_start > 0) {
            _notice = '继续上次观看 · ${_episodes[_episode].name} · ${_time(_start)}';
          }
        }
        _allEpisodes = _episodes.length <= 500;
        _attempted.add(_line!.id);
      }
    });
    if (_failed) _tryFallback();
  }

  void _progress(
    String identity,
    double position,
    double duration,
    double rate,
  ) {
    final line = _line;
    if (line == null || identity != _identity) return;
    unawaited(
      _history.save({
        'line': line.id,
        'episode_name': _episodes[_episode].name,
        'position': position,
        'duration': duration,
        'rate': rate,
        'intro': _intro,
        'outro': _outro,
      }),
    );
  }

  void _failure() {
    _failed = true;
    // Player errors can arrive while Flutter updates the player subtree.
    scheduleMicrotask(() {
      if (mounted) _tryFallback();
    });
  }

  void _tryFallback() {
    if (_line == null || _busy || _attempted.length >= 4) return;
    for (final next in _resolver.lines) {
      if (_attempted.contains(next.id)) continue;
      if (matchingEpisode(_line!.group, _episode, next.group) < 0) continue;
      _switch(next, automatic: true);
      return;
    }
  }

  void _switch(CatalogLine line, {bool automatic = false}) {
    if (_busy || _line == null || _line!.id == line.id) return;
    final index = matchingEpisode(_line!.group, _episode, line.group);
    if (index < 0) {
      setState(() => _notice = '这个来源缺少当前集，已保留原来的播放。');
      return;
    }
    final player = _player.currentState;
    setState(() {
      _start = player?.positionSeconds ?? _start;
      _startPlaying = player?.playing ?? true;
      _rate = player?.rate ?? _rate;
      _line = line;
      _episode = index;
      _episodePage = index ~/ 50;
      _allEpisodes = _allEpisodes && _episodes.length <= 500;
      _failed = false;
      if (!automatic) _attempted.clear();
      _attempted.add(line.id);
      _notice =
          '${automatic ? '当前线路不可用，已切换至' : '已切换至'} ${line.label} · ${_episodes[index].name}${_start > 0 ? ' · 续播 ${_time(_start)}' : ''}';
    });
  }

  void _selectEpisode(int index) {
    if (_busy || index < 0 || index >= _episodes.length || index == _episode) {
      return;
    }
    final rate = _player.currentState?.rate ?? _rate;
    setState(() {
      _episode = index;
      _episodePage = index ~/ 50;
      _start = 0;
      _rate = rate;
      _startPlaying = true;
      _failed = false;
      _attempted.clear();
      _attempted.add(_line!.id);
      _notice = null;
    });
  }

  Map<String, dynamic> _selection() => {
    'refs':
        [
              _line!,
              ..._resolver.lines.where(
                (l) =>
                    l.id != _line!.id &&
                    matchingEpisode(_line!.group, _episode, l.group) >= 0,
              ),
            ]
            .take(8)
            .map(
              (l) => {
                'source_id': l.film.sourceId,
                'film_id': l.film.id,
                'group_id': l.group.id,
              },
            )
            .toList(),
    'episode_id': _episodes[_episode].id,
    'all_episodes': _allEpisodes,
  };

  Future<void> _room({bool existing = false}) async {
    if (_busy || _line == null) return;
    String? code;
    if (existing) {
      code = await showDialog<String>(
        context: context,
        builder: (_) =>
            OwnedRoomPicker(api: widget.api, session: widget.session),
      );
      if (!mounted || code == null) return;
    }
    setState(() {
      _busy = true;
      _roomError = null;
    });
    final player = _player.currentState;
    final wasPlaying = player?.playing ?? false;
    final position = player?.positionSeconds ?? _start;
    await player?.pause();
    try {
      Room room;
      if (existing) {
        room = await widget.api.joinRoom(session: widget.session, code: code!);
        if (room.features?.permissions.playlist != true) {
          throw const ApiException('你没有这个房间的片单编辑权限。');
        }
        room = await widget.api.watchCatalogInRoom(
          widget.session,
          room,
          _selection(),
          startPosition: position,
        );
      } else {
        room = await widget.api.createRoom(
          session: widget.session,
          name: '${_line!.film.title} · 一起看',
          catalog: _selection(),
          startPosition: position,
        );
      }
      if (mounted) Navigator.of(context).pop(room);
    } catch (e) {
      if (mounted) {
        setState(
          () => _roomError = e is ApiException ? e.message : '暂时无法进入房间，请重试。',
        );
        if (wasPlaying) await player?.resume();
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _skipSettings() async {
    final intro = TextEditingController(text: '$_intro');
    final outro = TextEditingController(text: '$_outro');
    final values = await showDialog<(int, int)>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('跳过片头片尾'),
        content: SizedBox(
          width: 300,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: intro,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(labelText: '片头秒数（0 为关闭）'),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: outro,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(labelText: '片尾秒数（自动连播时生效）'),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, (
              (int.tryParse(intro.text) ?? 0).clamp(0, 600),
              (int.tryParse(outro.text) ?? 0).clamp(0, 600),
            )),
            child: const Text('保存'),
          ),
        ],
      ),
    );
    // Dialog exit animation may still reference its controllers.
    await Future<void>.delayed(const Duration(milliseconds: 300));
    intro.dispose();
    outro.dispose();
    if (!mounted || values == null) return;
    setState(() {
      _intro = values.$1;
      _outro = values.$2;
    });
  }

  String _time(double seconds) =>
      '${seconds ~/ 60}:${(seconds.toInt() % 60).toString().padLeft(2, '0')}';

  Widget _panel(double height) {
    final c = FrameColors.of(context);
    final indices = List.generate(_episodes.length, (i) => i);
    final ordered = _descending ? indices.reversed.toList() : indices;
    final shown = ordered.skip(_episodePage * 50).take(50);
    return Container(
      height: height,
      decoration: BoxDecoration(
        color: c.surface,
        border: Border.all(color: c.border),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.all(16),
            child: SegmentedButton<bool>(
              segments: [
                const ButtonSegment(
                  value: false,
                  label: Text('选集'),
                  icon: Icon(Icons.grid_view_rounded, size: 16),
                ),
                ButtonSegment(
                  value: true,
                  label: Text('换源 ${_resolver.lines.length}'),
                  icon: const Icon(Icons.swap_horiz_rounded, size: 18),
                ),
              ],
              selected: {_sourcesTab},
              onSelectionChanged: (value) =>
                  setState(() => _sourcesTab = value.single),
            ),
          ),
          if (_resolver.loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: _sourcesTab
                ? _sourceList()
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 16),
                        child: Row(
                          children: [
                            Expanded(
                              child: Text(
                                '选择剧集 · 共 ${_episodes.length} 集',
                                style: TextStyle(fontSize: 12, color: c.muted),
                              ),
                            ),
                            IconButton(
                              tooltip: _descending ? '正序排列' : '倒序排列',
                              icon: const Icon(
                                Icons.swap_vert_rounded,
                                size: 18,
                              ),
                              onPressed: () => setState(() {
                                _descending = !_descending;
                                _episodePage = 0;
                              }),
                            ),
                          ],
                        ),
                      ),
                      Expanded(
                        child: SingleChildScrollView(
                          padding: const EdgeInsets.fromLTRB(16, 6, 16, 12),
                          child: Wrap(
                            spacing: 8,
                            runSpacing: 8,
                            children: [
                              for (final i in shown)
                                ChoiceChip(
                                  label: Text(
                                    _episodes[i].name,
                                    overflow: TextOverflow.ellipsis,
                                  ),
                                  selected: i == _episode,
                                  showCheckmark: false,
                                  onSelected: _busy
                                      ? null
                                      : (_) => _selectEpisode(i),
                                ),
                              if (_episodes.isEmpty)
                                Padding(
                                  padding: const EdgeInsets.symmetric(
                                    vertical: 30,
                                  ),
                                  child: Text(
                                    _resolver.loading ? '正在载入影片…' : '暂无可播放剧集',
                                    style: TextStyle(color: c.muted),
                                  ),
                                ),
                            ],
                          ),
                        ),
                      ),
                      if (_episodes.length > 50)
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 8),
                          child: Row(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              IconButton(
                                tooltip: '上一组剧集',
                                onPressed: _episodePage > 0
                                    ? () => setState(() => _episodePage--)
                                    : null,
                                icon: const Icon(Icons.chevron_left_rounded),
                              ),
                              Text(
                                '${_episodePage + 1} / ${(_episodes.length / 50).ceil()}',
                                style: const TextStyle(fontSize: 12),
                              ),
                              IconButton(
                                tooltip: '下一组剧集',
                                onPressed:
                                    (_episodePage + 1) * 50 < _episodes.length
                                    ? () => setState(() => _episodePage++)
                                    : null,
                                icon: const Icon(Icons.chevron_right_rounded),
                              ),
                            ],
                          ),
                        ),
                      if (_episodes.length > 1)
                        SwitchListTile.adaptive(
                          dense: true,
                          title: const Text(
                            '自动连播',
                            style: TextStyle(fontSize: 12),
                          ),
                          value: _autoNext,
                          onChanged: (v) => setState(() => _autoNext = v),
                        ),
                    ],
                  ),
          ),
          Padding(
            padding: const EdgeInsets.all(16),
            child: Text(
              _resolver.loading
                  ? '正在补充其他来源 · ${_resolver.done}/${_resolver.total}'
                  : '${_resolver.lines.length} 条播放线路${_resolver.failures.isEmpty ? '' : ' · ${_resolver.failures.length} 个片源暂不可用'}',
              style: TextStyle(fontSize: 11, color: c.muted),
            ),
          ),
        ],
      ),
    );
  }

  Widget _sourceList() {
    final c = FrameColors.of(context);
    return ListView(
      padding: const EdgeInsets.symmetric(horizontal: 12),
      children: [
        const Padding(
          padding: EdgeInsets.fromLTRB(4, 0, 4, 12),
          child: Text('切换来源，继续当前集与进度', style: TextStyle(fontSize: 12)),
        ),
        for (final line in _resolver.lines)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Material(
              color: line.id == _line?.id ? c.elevated : c.surface,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(10),
                side: BorderSide(color: c.border),
              ),
              child: ListTile(
                dense: true,
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(10),
                ),
                title: Text(
                  line.film.sourceName,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  ),
                ),
                subtitle: Text(
                  '${line.group.name} · ${line.group.episodes.length} 集${line.film.year.isEmpty ? '' : ' · ${line.film.year}'}',
                  style: const TextStyle(fontSize: 11),
                ),
                trailing: Icon(
                  line.id == _line?.id
                      ? Icons.check_circle_rounded
                      : Icons.play_circle_outline_rounded,
                  size: 19,
                ),
                selected: line.id == _line?.id,
                onTap: _busy ? null : () => _switch(line),
              ),
            ),
          ),
        if (_resolver.lines.isEmpty && !_resolver.loading)
          const Padding(
            padding: EdgeInsets.all(16),
            child: Text('还没有匹配的播放来源', style: TextStyle(fontSize: 13)),
          ),
      ],
    );
  }

  Widget _placeholder(double height) => Container(
    height: height,
    decoration: BoxDecoration(
      color: Colors.black,
      borderRadius: BorderRadius.circular(14),
    ),
    child: Center(
      child: Padding(
        padding: const EdgeInsets.all(28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (_resolver.loading)
              const CircularProgressIndicator(
                strokeWidth: 2,
                color: Colors.white,
              )
            else
              const Icon(Icons.movie_outlined, size: 42, color: Colors.white70),
            const SizedBox(height: 20),
            Text(
              _resolver.loading
                  ? '正在为你准备 ${widget.target.title}'
                  : _resolver.error ?? '暂时没有可播放版本',
              style: const TextStyle(color: Colors.white, fontSize: 15),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 10),
            Text(
              _resolver.loading ? '匹配到片源后自动播放' : '片源更新后可在这里重试',
              style: const TextStyle(color: Colors.white54, fontSize: 12),
            ),
            if (!_resolver.loading)
              Padding(
                padding: const EdgeInsets.only(top: 20),
                child: OutlinedButton.icon(
                  style: OutlinedButton.styleFrom(
                    foregroundColor: Colors.white,
                    side: const BorderSide(color: Colors.white54),
                  ),
                  onPressed: () => _resolver.resolve(seeds: widget.films),
                  icon: const Icon(Icons.refresh_rounded),
                  label: const Text('重试片源'),
                ),
              ),
          ],
        ),
      ),
    ),
  );

  @override
  Widget build(BuildContext context) {
    final c = FrameColors.of(context);
    final film = _line?.film;
    return Scaffold(
      appBar: AppBar(
        toolbarHeight: 64,
        elevation: 0,
        scrolledUnderElevation: 0,
        leading: IconButton(
          tooltip: '返回片库',
          onPressed: _busy ? null : () => Navigator.of(context).pop(),
          icon: const Icon(Icons.arrow_back_rounded, size: 21),
        ),
        title: const Text(
          '同帧 / 放映厅',
          style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
        ),
        actions: [
          if (widget.onMetadata != null)
            IconButton(
              tooltip: '影片资料',
              onPressed: widget.onMetadata,
              icon: const Icon(Icons.info_outline_rounded, size: 20),
            ),
          if (context
                  .dependOnInheritedWidgetOfExactType<FrameAppearanceScope>() !=
              null)
            const FrameThemeSwitch(),
          const SizedBox(width: 16),
        ],
      ),
      body: LayoutBuilder(
        builder: (context, box) {
          final desktop = box.maxWidth >= 1050;
          final padding = box.maxWidth >= 700 ? 32.0 : 16.0;
          final width = math.min(1600.0, box.maxWidth - padding * 2);
          final videoWidth = desktop && !_wide ? width - 336 : width;
          final height = (videoWidth * 9 / 16).clamp(
            210.0,
            _wide ? 780.0 : 620.0,
          );
          final panel = _panel(desktop ? height + 68 : 410);
          final player = _line == null
              ? _placeholder(height)
              : CatalogPlayer(
                  key: _player,
                  api: widget.api,
                  session: widget.session,
                  selection: _selection(),
                  identity: _identity,
                  startPosition: _start,
                  startPlaying: _startPlaying,
                  startRate: _rate,
                  height: height,
                  skipIntro: _intro,
                  skipOutro: _autoNext ? _outro : 0,
                  autoAdvance: _autoNext,
                  onProgress: _progress,
                  onFailure: _failure,
                  onPrevious: _episode > 0 && !_busy
                      ? () => _selectEpisode(_episode - 1)
                      : null,
                  onNext: _episode + 1 < _episodes.length && !_busy
                      ? () => _selectEpisode(_episode + 1)
                      : null,
                );
          final roomActions = Container(
            margin: const EdgeInsets.only(top: 12),
            padding: const EdgeInsets.all(18),
            decoration: BoxDecoration(
              color: c.surface,
              border: Border.all(color: c.border),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Wrap(
                  spacing: 12,
                  runSpacing: 12,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    FilledButton.icon(
                      onPressed: _busy || _line == null ? null : () => _room(),
                      icon: const Icon(Icons.add_rounded, size: 18),
                      label: Text(_busy ? '正在进入房间…' : '创建房间'),
                    ),
                    OutlinedButton.icon(
                      onPressed: _busy || _line == null
                          ? null
                          : () => _room(existing: true),
                      icon: const Icon(Icons.meeting_room_outlined, size: 18),
                      label: const Text('加入房间'),
                    ),
                    Text(
                      '一个人看，也可以邀请朋友一起。',
                      style: TextStyle(color: c.muted, fontSize: 12),
                    ),
                  ],
                ),
                if (_episodes.length > 1)
                  CheckboxListTile(
                    contentPadding: EdgeInsets.zero,
                    dense: true,
                    controlAffinity: ListTileControlAffinity.leading,
                    title: Text(
                      _episodes.length > 500
                          ? '加入房间时只带入当前集（片单最多 500 集）'
                          : '加入房间时带上整部剧集',
                      style: const TextStyle(fontSize: 12),
                    ),
                    value: _allEpisodes,
                    onChanged: _busy || _episodes.length > 500
                        ? null
                        : (v) => setState(() => _allEpisodes = v ?? false),
                  ),
                if (_roomError != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(
                      _roomError!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                        fontSize: 13,
                      ),
                    ),
                  ),
              ],
            ),
          );
          return SingleChildScrollView(
            child: Center(
              child: SizedBox(
                width: width,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const SizedBox(height: 22),
                    Row(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                widget.target.title,
                                style: TextStyle(
                                  fontSize: desktop ? 28 : 23,
                                  fontWeight: FontWeight.w600,
                                  letterSpacing: -.6,
                                ),
                              ),
                              const SizedBox(height: 8),
                              Text(
                                [
                                  if (film?.year.isNotEmpty == true) film!.year,
                                  film?.kindLabel ?? '观影',
                                  if (_episodes.isNotEmpty)
                                    _episodes[_episode].name,
                                  '单人观看',
                                ].join(' · '),
                                style: TextStyle(color: c.muted, fontSize: 12),
                              ),
                            ],
                          ),
                        ),
                        if (desktop)
                          TextButton.icon(
                            onPressed: () => setState(() => _wide = !_wide),
                            icon: Icon(
                              _wide
                                  ? Icons.view_sidebar_outlined
                                  : Icons.crop_landscape_rounded,
                              size: 19,
                            ),
                            label: Text(_wide ? '显示选集' : '宽屏模式'),
                          ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    // Keep the player in a stable element position when toggling widescreen.
                    Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              player,
                              if (_notice != null)
                                Padding(
                                  padding: const EdgeInsets.symmetric(
                                    vertical: 8,
                                  ),
                                  child: Text(
                                    _notice!,
                                    style: TextStyle(
                                      fontSize: 12,
                                      color: c.muted,
                                    ),
                                  ),
                                ),
                              if (desktop) roomActions,
                            ],
                          ),
                        ),
                        if (desktop && !_wide) ...[
                          const SizedBox(width: 24),
                          SizedBox(width: 312, child: panel),
                        ],
                      ],
                    ),
                    if (!desktop || _wide) ...[
                      if (!desktop) roomActions,
                      const SizedBox(height: 20),
                      panel,
                    ],
                    const SizedBox(height: 26),
                    Wrap(
                      spacing: 16,
                      runSpacing: 12,
                      children: [
                        TextButton.icon(
                          onPressed: _skipSettings,
                          icon: const Icon(
                            Icons.fast_forward_outlined,
                            size: 19,
                          ),
                          label: Text(
                            _intro + _outro == 0
                                ? '跳过片头片尾'
                                : '片头 $_intro 秒 · 片尾 $_outro 秒',
                          ),
                        ),
                        if (_line != null)
                          Padding(
                            padding: const EdgeInsets.symmetric(vertical: 12),
                            child: Text(
                              '当前来源 · ${_line!.label}',
                              style: TextStyle(fontSize: 12, color: c.muted),
                            ),
                          ),
                      ],
                    ),
                    const SizedBox(height: 18),
                    Text(
                      '剧情简介',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                        color: c.text,
                      ),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      film?.summary.isNotEmpty == true
                          ? film!.summary
                          : widget.target.summary.isNotEmpty
                          ? widget.target.summary
                          : '暂无剧情介绍。',
                      style: TextStyle(
                        color: c.muted,
                        height: 1.8,
                        fontSize: 13,
                      ),
                    ),
                    if (film?.actors.isNotEmpty == true)
                      Padding(
                        padding: const EdgeInsets.only(top: 14),
                        child: Text(
                          '主演 · ${film!.actors}',
                          style: TextStyle(color: c.muted, fontSize: 12),
                        ),
                      ),
                    const SizedBox(height: 40),
                  ],
                ),
              ),
            ),
          );
        },
      ),
    );
  }
}
