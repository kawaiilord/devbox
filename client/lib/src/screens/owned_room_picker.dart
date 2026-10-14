import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../catalog_models.dart';
import '../models.dart';

class OwnedRoomPicker extends StatefulWidget {
  const OwnedRoomPicker({super.key, required this.api, required this.session});
  final ApiClient api;
  final Session session;
  @override
  State<OwnedRoomPicker> createState() => _OwnedRoomPickerState();
}

class _OwnedRoomPickerState extends State<OwnedRoomPicker> {
  final List<OwnedRoom> _rooms = [];
  bool _loading = true, _more = false;
  int _offset = 0;
  String? _error;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final page = await widget.api.ownedRooms(widget.session, offset: _offset);
      if (!mounted) return;
      setState(() {
        final ids = _rooms.map((r) => r.code).toSet();
        _rooms.addAll(page.rooms.where((r) => ids.add(r.code)));
        _more = page.hasMore;
        _offset = page.nextOffset;
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => _error = e is ApiException ? e.message : '暂时无法读取房间，请重试。',
        );
      }
    } finally {
      if (mounted) {
        setState(() => _loading = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: const Text('加入我的房间'),
    content: SizedBox(
      width: 460,
      height: 360,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('选择你创建的房间，带着当前影片和进度一起看。', style: TextStyle(fontSize: 12)),
          const SizedBox(height: 14),
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          if (_error != null)
            TextButton(onPressed: _load, child: Text('$_error 点击重试')),
          Expanded(
            child: _rooms.isEmpty && !_loading
                ? const Center(
                    child: Text(
                      '还没有可加入的房间\n关闭后可选择“创建房间”。',
                      textAlign: TextAlign.center,
                    ),
                  )
                : ListView(
                    children: [
                      for (final room in _rooms)
                        ListTile(
                          leading: const Icon(Icons.meeting_room_outlined),
                          title: Text(
                            room.name,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                          subtitle: Text(
                            '${room.code} · ${room.memberCount} 位成员',
                          ),
                          trailing: const Icon(Icons.chevron_right_rounded),
                          onTap: () => Navigator.pop(context, room.code),
                        ),
                      if (_more)
                        TextButton(
                          onPressed: _loading ? null : _load,
                          child: const Text('加载更多房间'),
                        ),
                    ],
                  ),
          ),
        ],
      ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('关闭'),
      ),
    ],
  );
}
