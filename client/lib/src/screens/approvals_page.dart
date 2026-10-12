import 'package:flutter/material.dart';

import 'workspace_section.dart';

import '../api/api_client.dart';
import '../models.dart';

class ApprovalsPage extends StatefulWidget {
  const ApprovalsPage({
    super.key,
    required this.api,
    required this.session,
    this.roomCode,
  });
  final ApiClient api;
  final Session session;
  final String? roomCode;
  @override
  State<ApprovalsPage> createState() => _ApprovalsPageState();
}

class _ApprovalsPageState extends State<ApprovalsPage> {
  final List<Map<String, dynamic>> _items = [];
  bool _busy = false, _more = true;
  String? _error;
  static const _kinds = {
    'registration': '注册申请',
    'room_creation': '建房申请',
    'room_join': '加房申请',
  };
  static const _statuses = {
    'pending': '待审核',
    'approved': '已通过',
    'rejected': '已拒绝',
    'cancelled': '已取消',
  };
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool reset = false}) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final data = await widget.api.listApprovals(
        widget.session,
        roomCode: widget.roomCode,
        offset: reset ? 0 : _items.length,
      );
      if (!mounted) return;
      setState(() {
        if (reset) _items.clear();
        _items.addAll(
          (data['requests'] as List<dynamic>).cast<Map<String, dynamic>>(),
        );
        _more = data['has_more'] == true;
      });
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _resolve(Map<String, dynamic> item, bool approve) async {
    final reason = TextEditingController();
    final accepted = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(approve ? '通过加房申请' : '拒绝加房申请'),
        content: TextField(
          controller: reason,
          maxLength: 500,
          decoration: InputDecoration(
            labelText: approve ? '审核说明（可选）' : '拒绝原因（必填）',
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('确认'),
          ),
        ],
      ),
    );
    final text = reason.text.trim();
    reason.dispose();
    if (accepted != true || !mounted) return;
    if (!approve && text.isEmpty) {
      setState(() => _error = '拒绝申请时请填写原因');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.resolveApproval(widget.session, item, approve, text);
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = '$e';
        });
      }
      return;
    }
    if (!mounted) return;
    setState(() => _busy = false);
    await _load(reset: true);
  }

  @override
  Widget build(BuildContext context) => WorkspaceSection(
    title: Text(widget.roomCode == null ? '我的申请' : '加房审批'),
    actions: [
      IconButton(
        tooltip: '刷新',
        onPressed: _busy ? null : () => _load(reset: true),
        icon: const Icon(Icons.refresh),
      ),
    ],
    body: Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 760),
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (_busy) const LinearProgressIndicator(),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.all(12),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            if (_items.isEmpty && !_busy && _error == null)
              const Padding(padding: EdgeInsets.all(32), child: Text('暂无申请记录')),
            ..._items.map(
              (item) => Card(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '${_kinds[item['kind']] ?? item['kind']} · ${_statuses[item['status']] ?? item['status']}',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      const SizedBox(height: 8),
                      Text(
                        '${item['display_name'] ?? ''}  ${item['room_name'] ?? ''}  ${item['room_code'] ?? ''}',
                      ),
                      Text(
                        DateTime.fromMillisecondsSinceEpoch(
                          (item['created_at'] as num).toInt(),
                        ).toLocal().toString().split('.').first,
                      ),
                      if ((item['reason'] ?? '').toString().isNotEmpty)
                        Text('审核说明：${item['reason']}'),
                      if (widget.roomCode != null &&
                          item['status'] == 'pending')
                        Wrap(
                          spacing: 12,
                          children: [
                            TextButton(
                              onPressed: _busy
                                  ? null
                                  : () => _resolve(item, false),
                              child: const Text('拒绝'),
                            ),
                            FilledButton(
                              onPressed: _busy
                                  ? null
                                  : () => _resolve(item, true),
                              child: const Text('通过'),
                            ),
                          ],
                        ),
                    ],
                  ),
                ),
              ),
            ),
            if (_more && _items.isNotEmpty)
              TextButton(
                onPressed: _busy ? null : _load,
                child: const Text('加载更多'),
              ),
          ],
        ),
      ),
    ),
  );
}
