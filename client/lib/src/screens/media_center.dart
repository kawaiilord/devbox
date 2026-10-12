import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../design.dart';
import '../models.dart';

class MediaCenter extends StatefulWidget {
  const MediaCenter({
    super.key,
    required this.api,
    required this.session,
    required this.onConnect,
    required this.onReconnect,
    required this.onBrowse,
    required this.onRemoved,
  });

  final ApiClient api;
  final Session session;
  final Future<bool> Function(String kind) onConnect;
  final Future<bool> Function(MediaSource source) onReconnect, onBrowse;
  final ValueChanged<MediaSource> onRemoved;

  @override
  State<MediaCenter> createState() => _MediaCenterState();
}

class _MediaCenterState extends State<MediaCenter> {
  final _search = TextEditingController();
  List<MediaSource> _sources = [];
  String? _expanded, _error;
  bool _loading = true, _working = false, _connectedOnly = false;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _reload() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final sources = await widget.api.listMediaSources(widget.session);
      if (mounted) setState(() => _sources = sources);
    } catch (_) {
      if (mounted) setState(() => _error = '暂时无法读取媒体源，请刷新重试。');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  List<MediaSource> _accounts(_MediaTool tool) =>
      _sources.where((source) => tool.types.contains(source.type)).toList();

  bool _matches(_MediaTool tool) {
    final accounts = _accounts(tool);
    if (_connectedOnly && accounts.isEmpty) return false;
    final query = _search.text.trim().toLowerCase();
    return query.isEmpty ||
        '${tool.title} ${tool.subtitle} ${tool.types.join(' ')} ${accounts.map((s) => s.name).join(' ')}'
            .toLowerCase()
            .contains(query);
  }

  Future<void> _perform(Future<void> Function() action) async {
    if (_working || _loading) return;
    setState(() {
      _working = true;
      _error = null;
    });
    try {
      await action();
    } catch (e) {
      if (mounted) {
        setState(() => _error = e is ApiException ? e.message : '操作失败，请重试。');
      }
    } finally {
      if (mounted) setState(() => _working = false);
    }
  }

  Future<void> _connect(_MediaTool tool) => _perform(() async {
    if (await widget.onConnect(tool.id) && mounted) {
      setState(() {
        _expanded = tool.id;
        _connectedOnly = false;
      });
      await _reload();
    }
  });

  Future<void> _remove(MediaSource source) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('移除媒体源'),
        content: Text('移除“${source.name}”后，需要重新连接才能选片。正在使用此来源的房间可能受到影响。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('移除'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    await widget.api.deleteMediaSource(widget.session, source.id);
    widget.onRemoved(source);
    if (mounted) await _reload();
  }

  @override
  Widget build(BuildContext context) {
    final colors = FrameColors.of(context);
    final mobile = MediaQuery.sizeOf(context).width < 700;
    final groups = [
      ..._mediaGroups,
      if (_sources.any((s) => s.type == 'rtmp'))
        const _MediaGroup('房间直播', [
          _MediaTool('rtmp', '房间直播', '管理已生成的房间直播源', Icons.sensors, ['rtmp']),
        ]),
    ];
    final visible = groups
        .map(
          (group) =>
              _MediaGroup(group.title, group.tools.where(_matches).toList()),
        )
        .where((group) => group.tools.isNotEmpty)
        .toList();
    final toolCount = _mediaGroups.fold<int>(
      0,
      (count, group) => count + group.tools.length,
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          padding: EdgeInsets.all(mobile ? 22 : 30),
          decoration: BoxDecoration(
            color: colors.elevated,
            borderRadius: BorderRadius.circular(mobile ? 20 : 24),
          ),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'SAMEFRAME / MEDIA',
                      style: TextStyle(
                        fontSize: 10,
                        letterSpacing: 2,
                        color: colors.muted,
                      ),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      '媒体中心',
                      style: TextStyle(
                        fontSize: mobile ? 28 : 32,
                        fontWeight: FontWeight.w600,
                        letterSpacing: -1,
                        height: 1.2,
                      ),
                    ),
                    const SizedBox(height: 10),
                    Text(
                      '$toolCount 个入口 · ${_mediaGroups.length} 个分类',
                      style: TextStyle(color: colors.muted, fontSize: 13),
                    ),
                  ],
                ),
              ),
              Container(
                width: mobile ? 52 : 64,
                height: mobile ? 52 : 64,
                decoration: BoxDecoration(
                  color: colors.surface,
                  borderRadius: BorderRadius.circular(18),
                ),
                child: Icon(
                  Icons.widgets_outlined,
                  size: mobile ? 25 : 30,
                  color: colors.text,
                ),
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.only(top: 20, bottom: 18),
          child: Text(
            '连接网盘与媒体库，把喜欢的影片带进房间。',
            style: TextStyle(color: colors.muted, fontSize: 13),
          ),
        ),
        LayoutBuilder(
          builder: (context, constraints) {
            final search = TextField(
              controller: _search,
              onChanged: (_) => setState(() {}),
              decoration: InputDecoration(
                hintText: '搜索平台或已添加的账号',
                prefixIcon: const Icon(Icons.search_rounded, size: 20),
                suffixIcon: _search.text.isEmpty
                    ? null
                    : IconButton(
                        tooltip: '清空搜索',
                        onPressed: () => setState(_search.clear),
                        icon: const Icon(Icons.close, size: 18),
                      ),
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 16,
                  vertical: 13,
                ),
              ),
            );
            final filters = Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                ChoiceChip(
                  label: const Text('全部'),
                  selected: !_connectedOnly,
                  showCheckmark: false,
                  onSelected: (_) => setState(() => _connectedOnly = false),
                ),
                const SizedBox(width: 8),
                ChoiceChip(
                  label: Text('已添加 ${_sources.length}'),
                  selected: _connectedOnly,
                  showCheckmark: false,
                  onSelected: (_) => setState(() => _connectedOnly = true),
                ),
                const SizedBox(width: 4),
                IconButton(
                  tooltip: '刷新媒体源',
                  onPressed: _loading || _working ? null : _reload,
                  icon: const Icon(Icons.refresh_rounded, size: 21),
                ),
              ],
            );
            if (constraints.maxWidth < 600) {
              return Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [search, const SizedBox(height: 12), filters],
              );
            }
            return Row(
              children: [
                Expanded(child: search),
                const SizedBox(width: 20),
                filters,
              ],
            );
          },
        ),
        if (_loading)
          const Padding(
            padding: EdgeInsets.only(top: 16),
            child: LinearProgressIndicator(minHeight: 2),
          ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(top: 16),
            child: Semantics(
              liveRegion: true,
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      _error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                  TextButton(
                    onPressed: _loading || _working ? null : _reload,
                    child: const Text('重试'),
                  ),
                ],
              ),
            ),
          ),
        const SizedBox(height: 12),
        if (visible.isEmpty && !_loading)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 52),
            child: Column(
              children: [
                Icon(
                  _connectedOnly
                      ? Icons.cloud_outlined
                      : Icons.search_off_rounded,
                  size: 34,
                  color: colors.muted,
                ),
                const SizedBox(height: 16),
                Text(
                  _connectedOnly && _search.text.isEmpty
                      ? '还没有添加媒体源'
                      : '没有找到匹配的媒体源',
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
                const SizedBox(height: 8),
                Text(
                  _connectedOnly ? '从全部入口中选择一个平台，开始连接。' : '试试其他平台名称或账号名称。',
                  style: TextStyle(color: colors.muted),
                ),
                TextButton(
                  onPressed: () => setState(() {
                    _search.clear();
                    _connectedOnly = false;
                  }),
                  child: const Text('查看全部入口'),
                ),
              ],
            ),
          ),
        for (final group in visible) ...[
          Padding(
            padding: const EdgeInsets.fromLTRB(8, 22, 8, 10),
            child: Row(
              children: [
                Container(
                  width: 4,
                  height: 15,
                  decoration: BoxDecoration(
                    color: colors.text,
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
                const SizedBox(width: 10),
                Text(
                  group.title,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const Spacer(),
                Text(
                  group.tools.length.toString().padLeft(2, '0'),
                  style: TextStyle(color: colors.muted, fontSize: 12),
                ),
              ],
            ),
          ),
          for (final tool in group.tools) _tool(context, tool, mobile),
        ],
        const SizedBox(height: 28),
        Divider(color: colors.border, height: 1),
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 18),
          child: Text(
            'SAMEFRAME  /  让片源与相聚，轻松连接。',
            style: TextStyle(
              fontSize: 10,
              letterSpacing: 1,
              color: colors.muted,
            ),
          ),
        ),
      ],
    );
  }

  Widget _tool(BuildContext context, _MediaTool tool, bool mobile) {
    final colors = FrameColors.of(context);
    final accounts = _accounts(tool);
    final expanded = _expanded == tool.id;
    final disabled = _loading || _working;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Semantics(
          button: true,
          expanded: accounts.isEmpty ? null : expanded,
          child: Material(
            color: expanded ? colors.surface : Colors.transparent,
            borderRadius: BorderRadius.circular(14),
            child: InkWell(
              key: ValueKey('media-tool-${tool.id}'),
              borderRadius: BorderRadius.circular(14),
              onTap: disabled
                  ? null
                  : () {
                      if (accounts.isNotEmpty || tool.id == 'rtmp') {
                        setState(() => _expanded = expanded ? null : tool.id);
                      } else {
                        _connect(tool);
                      }
                    },
              child: Padding(
                padding: EdgeInsets.symmetric(
                  horizontal: mobile ? 6 : 10,
                  vertical: 12,
                ),
                child: Row(
                  children: [
                    Container(
                      width: 44,
                      height: 44,
                      decoration: BoxDecoration(
                        color: colors.elevated,
                        borderRadius: BorderRadius.circular(13),
                      ),
                      child: Icon(tool.icon, size: 22, color: colors.text),
                    ),
                    const SizedBox(width: 14),
                    if (mobile)
                      Expanded(child: _toolLabel(tool, colors))
                    else ...[
                      SizedBox(width: 190, child: _toolLabel(tool, colors)),
                      Expanded(
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 16),
                          child: SizedBox(
                            height: 1,
                            child: CustomPaint(
                              painter: _DashedRule(colors.border),
                            ),
                          ),
                        ),
                      ),
                    ],
                    if (accounts.isNotEmpty) ...[
                      Text(
                        '${accounts.length} 个账号',
                        style: TextStyle(fontSize: 11, color: colors.muted),
                      ),
                      const SizedBox(width: 8),
                    ],
                    Icon(
                      expanded
                          ? Icons.keyboard_arrow_down_rounded
                          : Icons.chevron_right_rounded,
                      size: 22,
                      color: colors.muted,
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
        if (expanded)
          Container(
            margin: EdgeInsets.only(left: mobile ? 0 : 68, top: 4, bottom: 12),
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: colors.surface,
              borderRadius: BorderRadius.circular(14),
              border: Border.all(color: colors.border),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final source in accounts) _account(context, source),
                if (tool.id != 'rtmp')
                  Align(
                    alignment: Alignment.centerLeft,
                    child: TextButton.icon(
                      onPressed: disabled ? null : () => _connect(tool),
                      icon: const Icon(Icons.add_rounded, size: 18),
                      label: const Text('添加另一个账号'),
                    ),
                  ),
              ],
            ),
          ),
      ],
    );
  }

  Widget _toolLabel(_MediaTool tool, FrameColors colors) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        tool.title,
        style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
      ),
      const SizedBox(height: 3),
      Text(
        tool.subtitle,
        style: TextStyle(fontSize: 11, height: 1.5, color: colors.muted),
      ),
    ],
  );

  Widget _account(BuildContext context, MediaSource source) => ListTile(
    contentPadding: const EdgeInsets.symmetric(horizontal: 8),
    title: Text(source.name, maxLines: 2, overflow: TextOverflow.ellipsis),
    subtitle: Text(
      source.disabled
          ? '已停用 · 联系管理员启用'
          : '${mediaSourceLabel(source.type)} · 点击浏览选片',
      style: TextStyle(fontSize: 11, color: FrameColors.of(context).muted),
    ),
    leading: Icon(
      Icons.folder_open_outlined,
      color: FrameColors.of(context).muted,
      size: 20,
    ),
    onTap: _working || _loading || source.disabled
        ? null
        : () => _perform(() async {
            await widget.onBrowse(source);
          }),
    trailing: PopupMenuButton<String>(
      tooltip: '${source.name}的操作',
      enabled: !_working && !_loading,
      onSelected: (action) => _perform(() async {
        if (action == 'remove') {
          await _remove(source);
        } else if (await widget.onReconnect(source) && mounted) {
          await _reload();
        }
      }),
      itemBuilder: (_) => [
        if (source.type == 'quark' ||
            platformSourceTypes.contains(source.type) ||
            nasSourceTypes.contains(source.type))
          const PopupMenuItem(value: 'reconnect', child: Text('更新登录')),
        const PopupMenuItem(value: 'remove', child: Text('移除媒体源')),
      ],
    ),
  );
}

class _MediaGroup {
  const _MediaGroup(this.title, this.tools);
  final String title;
  final List<_MediaTool> tools;
}

class _MediaTool {
  const _MediaTool(this.id, this.title, this.subtitle, this.icon, this.types);
  final String id, title, subtitle;
  final IconData icon;
  final List<String> types;
}

const _mediaGroups = [
  _MediaGroup('云存储与媒体库', [
    _MediaTool('quark', '夸克网盘', '连接网盘，浏览你的影片', Icons.cloud_outlined, ['quark']),
    _MediaTool(
      'webdav',
      'WebDAV',
      '连接支持 WebDAV 的文件服务',
      Icons.cloud_queue_rounded,
      ['webdav'],
    ),
    _MediaTool('emby', 'Emby', '电影、剧集与私人媒体库', Icons.movie_outlined, ['emby']),
  ]),
  _MediaGroup('视频平台', [
    _MediaTool('bilibili', '哔哩哔哩', '视频、分集与播放列表', Icons.smart_display_outlined, [
      'bilibili',
    ]),
    _MediaTool(
      'youtube',
      'YouTube',
      '粘贴视频或播放列表链接',
      Icons.play_circle_outline_rounded,
      ['youtube'],
    ),
    _MediaTool('douyin', '抖音', '连接账号，解析视频链接', Icons.music_video_outlined, [
      'douyin',
    ]),
    _MediaTool(
      'more-platforms',
      '更多视频平台',
      'TikTok、Twitch、虎牙、斗鱼等',
      Icons.live_tv_outlined,
      ['tiktok', 'twitch', 'huya', 'douyu', 'acfun', 'cctv'],
    ),
  ]),
  _MediaGroup('家庭存储', [
    _MediaTool(
      'synology',
      '群晖 Synology',
      '浏览 NAS 中的影片',
      Icons.storage_outlined,
      ['synology'],
    ),
    _MediaTool('fnos', '飞牛 fnOS', '连接飞牛文件服务', Icons.dns_outlined, ['fnos']),
    _MediaTool('qnap', 'QNAP', '连接威联通媒体存储', Icons.inventory_2_outlined, [
      'qnap',
    ]),
    _MediaTool(
      'more-nas',
      '更多存储服务',
      'Nextcloud、Seafile、TrueNAS',
      Icons.folder_shared_outlined,
      ['nextcloud', 'seafile', 'truenas'],
    ),
  ]),
  _MediaGroup('扩展服务', [
    _MediaTool('remote', '远程媒体服务', '使用管理员配置的 Provider', Icons.hub_outlined, [
      'remote-alist',
      'remote-emby',
      'remote-bilibili',
    ]),
  ]),
];

class _DashedRule extends CustomPainter {
  const _DashedRule(this.color);
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    for (double x = 0; x < size.width; x += 7) {
      canvas.drawLine(
        Offset(x, .5),
        Offset((x + 3).clamp(0.0, size.width).toDouble(), .5),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(_DashedRule oldDelegate) => oldDelegate.color != color;
}
