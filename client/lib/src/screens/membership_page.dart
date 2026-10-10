import 'dart:async';

import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../models.dart';

class MembershipPage extends StatefulWidget {
  const MembershipPage({super.key, required this.api, required this.session});
  final ApiClient api;
  final Session session;
  @override
  State<MembershipPage> createState() => _MembershipPageState();
}

class _MembershipPageState extends State<MembershipPage> {
  final _code = TextEditingController();
  final _days = TextEditingController(text: '1');
  VipInfo? _info;
  MembershipStatus? _membership;
  CheckInStatus? _status;
  List<VipOrder> _orders = const [];
  List<PointsTransaction> _transactions = const [];
  bool _busy = false;
  String? _error;
  Timer? _poll;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _poll?.cancel();
    _code.dispose();
    _days.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final values = await Future.wait<Object>([
        widget.api.vipInfo(),
        widget.api.membership(widget.session),
        widget.api.checkInStatus(widget.session),
        widget.api.orders(widget.session),
        widget.api.pointsTransactions(widget.session),
      ]);
      if (!mounted) return;
      setState(() {
        _info = values[0] as VipInfo;
        _membership = values[1] as MembershipStatus;
        _status = values[2] as CheckInStatus;
        _orders = values[3] as List<VipOrder>;
        _transactions = values[4] as List<PointsTransaction>;
        _error = null;
      });
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    }
  }

  Future<void> _run(Future<void> Function() action) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await action();
      await _load();
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _buy(VipPlan plan) => _run(() async {
    final order = await widget.api.createOrder(widget.session, plan.id);
    if (order.checkoutUrl.isNotEmpty) {
      await launchUrl(
        Uri.parse(order.checkoutUrl),
        mode: LaunchMode.externalApplication,
      );
    }
    _poll?.cancel();
    _poll = Timer.periodic(const Duration(seconds: 3), (_) async {
      if (!mounted) return;
      final current = await widget.api.order(widget.session, order.orderNo);
      if (current.status == 'activated' ||
          current.status == 'expired' ||
          current.status == 'cancelled') {
        _poll?.cancel();
      }
      await _load();
    });
  });

  String _money(int minor) => '¥${(minor / 100).toStringAsFixed(2)}';

  @override
  Widget build(BuildContext context) {
    final info = _info;
    final status = _status;
    return Scaffold(
      appBar: AppBar(title: const Text('会员中心')),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            if (_busy) const LinearProgressIndicator(),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.all(8),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            if (info?.announcement.isNotEmpty == true)
              Card(
                child: ListTile(
                  leading: const Icon(Icons.campaign_outlined),
                  title: Text(info!.announcement),
                ),
                Card(
                  child: ListTile(
                    leading: Icon(
                      _membership?.active == true
                          ? Icons.verified_rounded
                          : Icons.lock_clock_outlined,
                    ),
                    title: Text(
                      _membership?.active == true ? '会员有效' : '当前为免费用户',
                    ),
                    subtitle: _membership?.vipExpiresAt == 0
                        ? null
                        : Text(
                            '有效期至 ${DateTime.fromMillisecondsSinceEpoch(_membership!.vipExpiresAt, isUtc: true).toLocal()}',
                          ),
                  ),
                ),
              ),
            Text('会员套餐', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            if (info != null)
              for (final plan in info.plans)
                Card(
                  child: ListTile(
                    leading: Icon(
                      plan.popular
                          ? Icons.workspace_premium
                          : Icons.star_outline,
                    ),
                    title: Text(plan.title),
                    subtitle: Text(
                      plan.lifetime ? '终身有效' : '${plan.durationDays} 天',
                    ),
                    trailing: FilledButton(
                      onPressed: _busy || info.paymentMethod == 'disabled'
                          ? null
                          : () => _buy(plan),
                      child: Text(_money(plan.priceMinor)),
                    ),
                  ),
                ),
            const SizedBox(height: 18),
            Text('卡密激活', style: Theme.of(context).textTheme.titleLarge),
            Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _code,
                    decoration: const InputDecoration(labelText: '激活码'),
                  ),
                ),
                const SizedBox(width: 8),
                FilledButton(
                  onPressed: _busy
                      ? null
                      : () => _run(() async {
                          await widget.api.redeemActivationCode(
                            widget.session,
                            _code.text,
                          );
                          _code.clear();
                        }),
                  child: const Text('激活'),
                ),
              ],
            ),
            const SizedBox(height: 18),
            Text('签到与积分', style: Theme.of(context).textTheme.titleLarge),
            if (status != null)
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '可用积分 ${status.availablePoints} · 连续 ${status.consecutiveDays} 天',
                      ),
                      Text(
                        '累计签到 ${status.totalCheckIns} 次 · 累计获得 ${status.totalPoints} · 已使用 ${status.usedPoints}',
                      ),
                      const SizedBox(height: 8),
                      Wrap(
                        spacing: 8,
                        children: [
                          FilledButton.icon(
                            onPressed: _busy || status.checkedInToday
                                ? null
                                : () => _run(() async {
                                    await widget.api.dailyCheckIn(
                                      widget.session,
                                    );
                                  }),
                            icon: const Icon(Icons.event_available),
                            label: Text(
                              status.checkedInToday
                                  ? '今日已签到'
                                  : '签到 +${status.pointsPerCheckIn}',
                            ),
                          ),
                          SizedBox(
                            width: 100,
                            child: TextField(
                              controller: _days,
                              keyboardType: TextInputType.number,
                              decoration: const InputDecoration(
                                labelText: '兑换天数',
                              ),
                            ),
                          ),
                          OutlinedButton(
                            onPressed: _busy || status.pointsPerVipDay <= 0
                                ? null
                                : () => _run(() async {
                                    await widget.api.redeemPoints(
                                      widget.session,
                                      int.parse(_days.text),
                                    );
                                  }),
                            child: Text(
                              status.pointsPerVipDay <= 0
                                  ? '兑换未开放'
                                  : '${status.pointsPerVipDay} 分/天',
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
            const SizedBox(height: 18),
            Text('订单', style: Theme.of(context).textTheme.titleLarge),
            if (_orders.isEmpty) const Text('暂无订单'),
            for (final order in _orders)
              ListTile(
                title: Text(order.planTitle),
                subtitle: Text(order.orderNo),
                trailing: Text(
                  '${_money(order.amountMinor)} · ${order.status}',
                ),
              ),
            const SizedBox(height: 18),
            Text('积分流水', style: Theme.of(context).textTheme.titleLarge),
            if (_transactions.isEmpty) const Text('暂无流水'),
            for (final item in _transactions)
              ListTile(
                title: Text(item.type),
                subtitle: Text('余额 ${item.balanceAfter}'),
                trailing: Text(
                  item.change > 0 ? '+${item.change}' : '${item.change}',
                ),
              ),
          ],
        ),
      ),
    );
  }
}
