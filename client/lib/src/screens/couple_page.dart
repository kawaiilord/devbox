import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class CouplePage extends StatefulWidget {
  const CouplePage({super.key, required this.api, required this.session});
  final ApiClient api;
  final Session session;
  @override
  State<CouplePage> createState() => _CouplePageState();
}

class _CouplePageState extends State<CouplePage> {
  final _partner = TextEditingController();
  final _moment = TextEditingController();
  Couple? _couple;
  List<CoupleRequest> _requests = const [];
  List<CoupleMoment> _moments = const [];
  bool _busy = false;
  String? _error;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _busy = true);
    try {
      final couple = await widget.api.coupleInfo(widget.session);
      final requests = await widget.api.coupleRequests(widget.session);
      final moments = couple == null
          ? <CoupleMoment>[]
          : await widget.api.coupleMoments(widget.session);
      if (mounted) {
        setState(() {
          _couple = couple;
          _requests = requests;
          _moments = moments;
          _error = null;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _run(Future<void> Function() op) async {
    try {
      await op();
      await _load();
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    }
  }

  @override
  void dispose() {
    _partner.dispose();
    _moment.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('情侣空间')),
    body: ListView(
      padding: const EdgeInsets.all(18),
      children: [
        if (_busy) const LinearProgressIndicator(),
        if (_error != null)
          Text(
            _error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        if (_couple == null) ...[
          TextField(
            controller: _partner,
            decoration: const InputDecoration(labelText: '对方用户 ID'),
          ),
          FilledButton(
            onPressed: () => _run(
              () => widget.api.requestCouple(
                widget.session,
                _partner.text.trim(),
              ),
            ),
            child: const Text('发起绑定申请'),
          ),
          for (final request in _requests)
            ListTile(
              title: Text('${request.requester.displayName} 请求绑定'),
              trailing: Wrap(
                children: [
                  TextButton(
                    onPressed: () => _run(
                      () => widget.api.respondCouple(
                        widget.session,
                        request.id,
                        false,
                      ),
                    ),
                    child: const Text('拒绝'),
                  ),
                  FilledButton(
                    onPressed: () => _run(
                      () => widget.api.respondCouple(
                        widget.session,
                        request.id,
                        true,
                      ),
                    ),
                    child: const Text('接受'),
                  ),
                ],
              ),
            ),
        ] else ...[
          Card(
            child: ListTile(
              leading: const Icon(Icons.favorite_rounded, color: Colors.pink),
              title: Text(_couple!.partner.displayName),
              subtitle: Text(
                _couple!.status == 'active'
                    ? '已绑定 · ${DateTime.fromMillisecondsSinceEpoch(_couple!.boundAt).toLocal()}'
                    : '冷静期至 ${DateTime.fromMillisecondsSinceEpoch(_couple!.coolingPeriodEnd).toLocal()}',
              ),
              trailing: _couple!.status == 'active'
                  ? TextButton(
                      onPressed: () =>
                          _run(() => widget.api.separateCouple(widget.session)),
                      child: const Text('解除'),
                    )
                  : FilledButton(
                      onPressed: () =>
                          _run(() => widget.api.restoreCouple(widget.session)),
                      child: const Text('恢复'),
                    ),
            ),
          ),
          if (_couple!.status == 'active') ...[
            Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _moment,
                    maxLength: 1000,
                    decoration: const InputDecoration(labelText: '记录一个共同瞬间'),
                  ),
                ),
                IconButton.filled(
                  onPressed: () => _run(() async {
                    await widget.api.addCoupleMoment(
                      widget.session,
                      _moment.text,
                    );
                    _moment.clear();
                  }),
                  icon: const Icon(Icons.add_rounded),
                ),
              ],
            ),
            for (final moment in _moments)
              ListTile(
                title: Text(moment.body),
                subtitle: Text(
                  '${moment.author.displayName} · ${DateTime.fromMillisecondsSinceEpoch(moment.createdAt).toLocal()}',
                ),
              ),
          ],
        ],
      ],
    ),
  );
}
