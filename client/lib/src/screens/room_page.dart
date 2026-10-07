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
              child: Video(
                controller: controller.player.videoController,
                controls: NoVideoControls,
                pauseUponEnteringBackgroundMode: false,
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
