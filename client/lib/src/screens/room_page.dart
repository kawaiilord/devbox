import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../room_controller.dart';

class RoomPage extends StatefulWidget {
  const RoomPage({
    super.key,
    required this.api,
    required this.session,
    required this.initialRoom,
  });

  final ApiClient api;
  final Session session;
  final Room initialRoom;

  @override
  State<RoomPage> createState() => _RoomPageState();
}

class _RoomPageState extends State<RoomPage> {
  late final RoomController controller = RoomController(
    api: widget.api,
    session: widget.session,
    room: widget.initialRoom,
  );

  @override
  void initState() {
    super.initState();
    controller.initialize();
  }

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  Future<void> _showChat() async {
    final input = TextEditingController();
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('房间聊天'),
        content: SizedBox(
          width: 520,
          height: 480,
          child: AnimatedBuilder(
            animation: controller,
            builder: (context, _) => Column(
              children: [
                Expanded(
                  child: controller.messages.isEmpty
                      ? const Center(child: Text('还没有消息。'))
                      : ListView.builder(
                          itemCount: controller.messages.length,
                          itemBuilder: (context, index) {
                            final message = controller.messages[index];
                            final mine =
                                message.userId == controller.session.user.id;
                            return Align(
                              alignment: mine
                                  ? Alignment.centerRight
                                  : Alignment.centerLeft,
                              child: Container(
                                constraints: const BoxConstraints(
                                  maxWidth: 360,
                                ),
                                margin: const EdgeInsets.symmetric(vertical: 4),
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 12,
                                  vertical: 9,
                                ),
                                decoration: BoxDecoration(
                                  color: mine
                                      ? Theme.of(context)
                                            .colorScheme
                                            .primaryContainer
                                      : Colors.white.withValues(alpha: 0.06),
                                  borderRadius: BorderRadius.circular(14),
                                ),
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Row(
                                      children: [
                                        Expanded(
                                          child: Text(
                                            message.displayName,
                                            style: Theme.of(context)
                                                .textTheme
                                                .labelSmall,
                                          ),
                                        ),
                                        PopupMenuButton<String>(
                                          tooltip: '消息操作',
                                          padding: EdgeInsets.zero,
                                          iconSize: 18,
                                          onSelected: (action) =>
                                              _handleMessageAction(
                                                action,
                                                message,
                                              ),
                                          itemBuilder: (_) => [
                                            const PopupMenuItem(
                                              value: 'report',
                                              child: Text('举报消息'),
                                            ),
                                            if (!mine)
                                              const PopupMenuItem(
                                                value: 'block',
                                                child: Text('屏蔽该用户'),
                                              ),
                                          ],
                                        ),
                                      ],
                                    ),
                                    const SizedBox(height: 3),
                                    Text(message.body),
                                  ],
                                ),
                              ),
                            );
                          },
                        ),
                ),
                const SizedBox(height: 12),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: input,
                        maxLength: 500,
                        decoration: const InputDecoration(
                          labelText: '消息',
                          counterText: '',
                        ),
                        onSubmitted: (_) {
                          controller.sendChat(input.text);
                          input.clear();
                        },
                      ),
                    ),
                    const SizedBox(width: 8),
                    IconButton.filled(
                      onPressed: () {
                        controller.sendChat(input.text);
                        input.clear();
                      },
                      icon: const Icon(Icons.send_rounded),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('关闭'),
          ),
        ],
      ),
    );
    input.dispose();
  }

  Future<void> _showDanmaku() async {
    final input = TextEditingController();
    final keyword = TextEditingController();
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('弹幕'),
        content: SizedBox(
          width: 520,
          height: 440,
          child: AnimatedBuilder(
            animation: controller,
            builder: (context, _) => Column(
              children: [
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('显示弹幕'),
                  value: controller.danmakuEnabled,
                  onChanged: controller.setDanmakuEnabled,
                ),
                Expanded(
                  child: ListView(
                    children: [
                      for (final message
                          in controller.danmakuMessages.reversed.take(100))
                        ListTile(
                          dense: true,
                          title: Text(message.body),
                          subtitle: Text(
                            '${message.displayName} · ${message.positionSeconds.toStringAsFixed(1)} 秒',
                          ),
                        ),
                    ],
                  ),
                ),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: input,
                        maxLength: 100,
                        decoration: const InputDecoration(
                          labelText: '在当前时间发送弹幕',
                          counterText: '',
                        ),
                        onSubmitted: (_) {
                          controller.sendDanmaku(input.text);
                          input.clear();
                        },
                      ),
                    ),
                    const SizedBox(width: 8),
                    IconButton.filled(
                      onPressed: controller.danmakuEnabled
                          ? () {
                              controller.sendDanmaku(input.text);
                              input.clear();
                            }
                          : null,
                      icon: const Icon(Icons.send_rounded),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: keyword,
                        decoration: const InputDecoration(labelText: '添加本地屏蔽词'),
                      ),
                    ),
                    TextButton(
                      onPressed: () {
                        controller.blockDanmakuKeyword(keyword.text);
                        keyword.clear();
                      },
                      child: const Text('屏蔽'),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('关闭'),
          ),
        ],
      ),
    );
    input.dispose();
    keyword.dispose();
  }

  Future<void> _handleMessageAction(String action, ChatMessage message) async {
    if (action == 'block') {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (dialogContext) => AlertDialog(
          title: Text('屏蔽 ${message.displayName}？'),
          content: const Text('双方的历史消息和后续实时消息都会对你隐藏。'),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(dialogContext).pop(true),
              child: const Text('屏蔽'),
            ),
          ],
        ),
      );
      if (confirmed != true) return;
      try {
        await controller.blockUser(message.userId);
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text('已屏蔽 ${message.displayName}')));
        }
      } catch (exception) {
        _showActionError(exception);
      }
      return;
    }
    await _showReport(message: message);
  }

  Future<void> _showReport({ChatMessage? message}) async {
    final details = TextEditingController();
    var reason = 'harassment';
    var submitted = false;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: Text(message == null ? '举报房间' : '举报消息'),
          content: SizedBox(
            width: 440,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                DropdownButtonFormField<String>(
                  initialValue: reason,
                  decoration: const InputDecoration(labelText: '原因'),
                  items: const [
                    DropdownMenuItem(value: 'spam', child: Text('垃圾信息')),
                    DropdownMenuItem(value: 'harassment', child: Text('骚扰或攻击')),
                    DropdownMenuItem(value: 'illegal', child: Text('违法内容')),
                    DropdownMenuItem(value: 'copyright', child: Text('版权问题')),
                    DropdownMenuItem(value: 'other', child: Text('其他')),
                  ],
                  onChanged: submitted
                      ? null
                      : (value) => setDialogState(() => reason = value!),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: details,
                  enabled: !submitted,
                  maxLength: 1000,
                  maxLines: 4,
                  decoration: const InputDecoration(labelText: '补充说明（可选）'),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: submitted
                  ? null
                  : () => Navigator.of(dialogContext).pop(),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: submitted
                  ? null
                  : () async {
                      setDialogState(() => submitted = true);
                      try {
                        if (message == null) {
                          await controller.reportRoom(
                            reason: reason,
                            details: details.text,
                          );
                        } else {
                          await controller.reportMessage(
                            message,
                            reason: reason,
                            details: details.text,
                          );
                        }
                        if (dialogContext.mounted) {
                          Navigator.of(dialogContext).pop();
                        }
                        if (mounted) {
                          ScaffoldMessenger.of(this.context).showSnackBar(
                            const SnackBar(content: Text('举报已提交，管理员会进行审核。')),
                          );
                        }
                      } catch (exception) {
                        setDialogState(() => submitted = false);
                        _showActionError(exception);
                      }
                    },
              child: const Text('提交'),
            ),
          ],
        ),
      ),
    );
    details.dispose();
  }

  void _showActionError(Object exception) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text('操作失败：$exception')));
  }

  Future<void> _showSubtitles() async {
    final files = await controller.availableSubtitles();
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('外挂字幕'),
        content: SizedBox(
          width: 460,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              ListTile(
                leading: const Icon(Icons.subtitles_off_rounded),
                title: const Text('关闭外挂字幕'),
                onTap: () async {
                  await controller.disableSubtitles();
                  if (dialogContext.mounted) {
                    Navigator.of(dialogContext).pop();
                  }
                },
              ),
              if (files.isEmpty)
                const Padding(
                  padding: EdgeInsets.all(18),
                  child: Text('视频所在目录没有 SRT、VTT、ASS 或 SSA 字幕。'),
                ),
              for (final file in files)
                ListTile(
                  leading: const Icon(Icons.subtitles_rounded),
                  title: Text(file.name),
                  selected: controller.selectedSubtitlePath == file.path,
                  onTap: () async {
                    await controller.selectSubtitle(file);
                    if (dialogContext.mounted) {
                      Navigator.of(dialogContext).pop();
                    }
                  },
                ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        return Scaffold(
          appBar: AppBar(
            title: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(controller.room.name),
                Text(
                  '房间 ${controller.room.code}',
                  style: Theme.of(context).textTheme.labelMedium
                      ?.copyWith(color: Colors.white54),
                ),
              ],
            ),
            actions: [
              IconButton(
                tooltip: '举报房间',
                onPressed: _showReport,
                icon: const Icon(Icons.flag_outlined),
              ),
              if (controller.room.mediaSourceId.isNotEmpty)
                IconButton(
                  tooltip: '外挂字幕',
                  onPressed: _showSubtitles,
                  icon: const Icon(Icons.subtitles_rounded),
                ),
              IconButton(
                tooltip: '弹幕',
                onPressed: controller.room.closed ? null : _showDanmaku,
                icon: const Icon(Icons.slow_motion_video_rounded),
              ),
              IconButton(
                tooltip: '房间聊天',
                onPressed: controller.room.closed ? null : _showChat,
                icon: const Icon(Icons.chat_bubble_outline_rounded),
              ),
              Padding(
                padding: const EdgeInsets.only(right: 18),
                child: _ConnectionBadge(connected: controller.connected),
              ),
            ],
          ),
          body: controller.loading
              ? const Center(child: CircularProgressIndicator())
              : LayoutBuilder(
                  builder: (context, constraints) {
                    final player = _PlayerPanel(controller: controller);
                    final members = _MembersPanel(controller: controller);
                    if (constraints.maxWidth < 900) {
                      return ListView(
                        padding: const EdgeInsets.all(16),
                        children: [player, const SizedBox(height: 16), members],
                      );
                    }
                    return Padding(
                      padding: const EdgeInsets.all(20),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Expanded(flex: 4, child: player),
                          const SizedBox(width: 18),
                          SizedBox(width: 290, child: members),
                        ],
                      ),
                    );
                  },
                ),
        );
      },
    );
  }
}

class _PlayerPanel extends StatelessWidget {
  const _PlayerPanel({required this.controller});
  final RoomController controller;

  @override
  Widget build(BuildContext context) {
    return Card(
      clipBehavior: Clip.antiAlias,
      child: Column(
        children: [
          AspectRatio(
            aspectRatio: 16 / 9,
            child: ColoredBox(
              color: Colors.black,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  Video(
                    controller: controller.player.videoController,
                    controls: NoVideoControls,
                    pauseUponEnteringBackgroundMode: false,
                  ),
                  _DanmakuOverlay(controller: controller),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(18, 12, 18, 18),
            child: Column(
              children: [
                _Timeline(controller: controller),
                const SizedBox(height: 8),
                Row(
                  children: [
                    StreamBuilder<bool>(
                      stream: controller.player.playingStream,
                      initialData: controller.player.playing,
                      builder: (context, snapshot) => IconButton.filled(
                        tooltip: controller.isOwner ? '播放/暂停' : '仅房主可控制',
                        onPressed: controller.isOwner
                            ? () => snapshot.data == true
                                  ? controller.pause()
                                  : controller.play()
                            : null,
                        icon: Icon(
                          snapshot.data == true
                              ? Icons.pause_rounded
                              : Icons.play_arrow_rounded,
                        ),
                      ),
                    ),
                    const SizedBox(width: 10),
                    PopupMenuButton<double>(
                      enabled: controller.isOwner,
                      tooltip: '播放速度',
                      onSelected: controller.setSpeed,
                      itemBuilder: (_) => const [0.75, 1.0, 1.25, 1.5, 2.0]
                          .map(
                            (speed) => PopupMenuItem(
                              value: speed,
                              child: Text('${speed}x'),
                            ),
                          )
                          .toList(),
                      child: const Padding(
                        padding: EdgeInsets.symmetric(
                          horizontal: 10,
                          vertical: 8,
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(Icons.speed_rounded, size: 20),
                            SizedBox(width: 6),
                            Text('倍速'),
                          ],
                        ),
                      ),
                    ),
                    const Spacer(),
                    Icon(
                      controller.isOwner
                          ? Icons.admin_panel_settings_rounded
                          : Icons.visibility_rounded,
                      size: 18,
                    ),
                    const SizedBox(width: 6),
                    Text(controller.isOwner ? '房主控制' : '跟随房主'),
                  ],
                ),
                if (controller.error != null) ...[
                  const SizedBox(height: 10),
                  Align(
                    alignment: Alignment.centerLeft,
                    child: Text(
                      controller.error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _DanmakuOverlay extends StatelessWidget {
  const _DanmakuOverlay({required this.controller});
  final RoomController controller;

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: StreamBuilder<Duration>(
        stream: controller.player.positionStream,
        initialData: controller.player.position,
        builder: (context, snapshot) {
          if (!controller.danmakuEnabled) return const SizedBox.shrink();
          final position =
              (snapshot.data ?? Duration.zero).inMilliseconds / 1000;
          final visible = controller.danmakuMessages
              .where(
                (message) =>
                    controller.danmakuVisible(message) &&
                    message.positionSeconds <= position + 0.25 &&
                    message.positionSeconds >= position - 6,
              )
              .toList(growable: false);
          return LayoutBuilder(
            builder: (context, constraints) => Stack(
              clipBehavior: Clip.none,
              children: [
                for (final message in visible.take(24))
                  Positioned(
                    top: message.mode == 'bottom'
                        ? constraints.maxHeight - 42 - (message.id % 2) * 28
                        : 12 + (message.id % 6) * 30,
                    left: 0,
                    right: 0,
                    child: message.mode == 'scroll'
                        ? TweenAnimationBuilder<double>(
                            key: ValueKey(message.id),
                            tween: Tween(
                              begin: constraints.maxWidth,
                              end: -constraints.maxWidth,
                            ),
                            duration: const Duration(seconds: 6),
                            builder: (context, offset, child) =>
                                Transform.translate(
                                  offset: Offset(offset, 0),
                                  child: child,
                                ),
                            child: _DanmakuText(message: message),
                          )
                        : Align(
                            alignment: Alignment.center,
                            child: _DanmakuText(message: message),
                          ),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _DanmakuText extends StatelessWidget {
  const _DanmakuText({required this.message});
  final DanmakuMessage message;

  @override
  Widget build(BuildContext context) {
    return Text(
      message.body,
      maxLines: 1,
      style: TextStyle(
        color: Color(0xff000000 | message.color),
        fontSize: 17,
        fontWeight: FontWeight.w600,
        shadows: const [
          Shadow(color: Colors.black, blurRadius: 3),
          Shadow(color: Colors.black, offset: Offset(1, 1)),
        ],
      ),
    );
  }
}

class _Timeline extends StatelessWidget {
  const _Timeline({required this.controller});
  final RoomController controller;

  @override
  Widget build(BuildContext context) {
    return StreamBuilder<Duration>(
      stream: controller.player.durationStream,
      initialData: controller.player.duration,
      builder: (context, durationSnapshot) {
        return StreamBuilder<Duration>(
          stream: controller.player.positionStream,
          initialData: controller.player.position,
          builder: (context, positionSnapshot) {
            final duration = durationSnapshot.data ?? Duration.zero;
            final position = positionSnapshot.data ?? Duration.zero;
            final maxMilliseconds = math.max(1, duration.inMilliseconds);
            final value = position.inMilliseconds
                .clamp(0, maxMilliseconds)
                .toDouble();
            return Column(
              children: [
                Slider(
                  value: value,
                  max: maxMilliseconds.toDouble(),
                  onChanged: controller.isOwner ? (_) {} : null,
                  onChangeEnd: controller.isOwner
                      ? (next) => controller.seek(
                          Duration(milliseconds: next.round()),
                        )
                      : null,
                ),
                Row(
                  children: [
                    Text(_format(position)),
                    const Spacer(),
                    Text(_format(duration)),
                  ],
                ),
              ],
            );
          },
        );
      },
    );
  }

  String _format(Duration value) {
    final hours = value.inHours;
    final minutes = value.inMinutes.remainder(60).toString().padLeft(2, '0');
    final seconds = value.inSeconds.remainder(60).toString().padLeft(2, '0');
    return hours > 0 ? '$hours:$minutes:$seconds' : '$minutes:$seconds';
  }
}

class _MembersPanel extends StatelessWidget {
  const _MembersPanel({required this.controller});
  final RoomController controller;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text('房间成员', style: Theme.of(context).textTheme.titleMedium),
                const Spacer(),
                Text(
                  '${controller.onlineUserIds.length} 在线 · ${controller.room.members.length}/${controller.room.maxMembers}',
                ),
              ],
            ),
            const SizedBox(height: 14),
            for (final member in controller.room.members)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: CircleAvatar(
                  child: Text(
                    member.displayName.characters.first.toUpperCase(),
                  ),
                ),
                title: Text(member.displayName),
                subtitle: Text(
                  member.userId == controller.session.user.id
                      ? '当前设备'
                      : (controller.onlineUserIds.contains(member.userId)
                            ? '在线'
                            : '离线'),
                ),
                trailing: member.userId == controller.room.ownerId
                    ? const Icon(Icons.star_rounded, color: Color(0xFFFFC857))
                    : null,
              ),
            const Divider(height: 28),
            Text('同步状态', style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 10),
            _InfoRow(label: '服务端', value: controller.connected ? '已连接' : '重连中'),
            _InfoRow(
              label: '最近校准',
              value: controller.lastAlignment?.name ?? '等待快照',
            ),
            const _InfoRow(label: '快照周期', value: '3 秒'),
            const _InfoRow(label: '控制模型', value: '房主唯一控制'),
          ],
        ),
      ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        children: [
          Text(label, style: const TextStyle(color: Colors.white54)),
          const Spacer(),
          Flexible(child: Text(value, textAlign: TextAlign.end)),
        ],
      ),
    );
  }
}

class _ConnectionBadge extends StatelessWidget {
  const _ConnectionBadge({required this.connected});
  final bool connected;

  @override
  Widget build(BuildContext context) {
    final color = connected ? const Color(0xFF42D3B1) : const Color(0xFFFFC857);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: color.withValues(alpha: 0.45)),
      ),
      child: Row(
        children: [
          Icon(Icons.circle, size: 9, color: color),
          const SizedBox(width: 6),
          Text(connected ? '已同步' : '连接中'),
        ],
      ),
    );
  }
}
