import 'dart:async';

import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../design.dart';

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

  Future<void> _redeem() async {
    final days = int.tryParse(_days.text);
    if (days == null || days < 1) {
      setState(() => _error = '请输入有效的兑换天数');
      return;
    }
    await _run(() async {
      await widget.api.redeemPoints(widget.session, days);
    });
  }

  String _orderStatus(String value) => switch (value) {
    'pending' => '待支付',
    'paid' => '已支付',
    'activated' => '已开通',
    'expired' => '已过期',
    'cancelled' => '已取消',
    _ => value,
  };
  String _transactionType(String value) => switch (value) {
    'check_in' => '每日签到',
    'redeem_vip' => '会员兑换',
    'invite' => '邀请奖励',
    'admin' => '积分调整',
    _ => '积分变动',
  };

  Widget _planCard(VipPlan plan) {
    final accent = plan.popular ? FrameColors.gold : FrameColors.mint;
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: plan.popular ? const Color(0xFF252821) : FrameColors.surface,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: plan.popular
              ? FrameColors.gold.withValues(alpha: .6)
              : FrameColors.border,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                plan.lifetime
                    ? Icons.all_inclusive_rounded
                    : Icons.workspace_premium_outlined,
                color: accent,
                size: 24,
              ),
              const Spacer(),
              if (plan.popular)
                const FramePill('推荐之选', color: FrameColors.gold),
            ],
          ),
          const SizedBox(height: 22),
          Text(
            plan.title,
            style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 6),
          Text(
            plan.lifetime ? '一次开通，长久相伴' : '${plan.durationDays} 天相聚时光',
            style: const TextStyle(color: FrameColors.muted, fontSize: 13),
          ),
          const SizedBox(height: 22),
          Text(
            _money(plan.priceMinor),
            style: TextStyle(
              fontSize: 36,
              letterSpacing: -1.5,
              fontWeight: FontWeight.w700,
              color: accent,
            ),
          ),
          SizedBox(
            height: 24,
            child: plan.originalPriceMinor > plan.priceMinor
                ? Text(
                    '原价 ${_money(plan.originalPriceMinor)}',
                    style: const TextStyle(
                      color: FrameColors.muted,
                      fontSize: 12,
                      decoration: TextDecoration.lineThrough,
                    ),
                  )
                : const Text(
                    '按当前套餐价格开通',
                    style: TextStyle(color: FrameColors.muted, fontSize: 12),
                  ),
          ),
          const Divider(height: 32),
          const Row(
            children: [
              Icon(Icons.check_rounded, size: 16, color: FrameColors.mint),
              SizedBox(width: 8),
              Text('延长房间有效期', style: TextStyle(fontSize: 13)),
            ],
          ),
          const SizedBox(height: 12),
          const Row(
            children: [
              Icon(Icons.check_rounded, size: 16, color: FrameColors.mint),
              SizedBox(width: 8),
              Text('解锁情侣绑定', style: TextStyle(fontSize: 13)),
            ],
          ),
          const SizedBox(height: 24),
          SizedBox(
            width: double.infinity,
            child: FilledButton(
              style: FilledButton.styleFrom(backgroundColor: accent),
              onPressed: _busy || _info?.paymentMethod == 'disabled'
                  ? null
                  : () => _buy(plan),
              child: Text(
                _info?.paymentMethod == 'disabled'
                    ? '购买暂未开放'
                    : '开通${plan.title}',
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _codeSection() => FrameSection(
    title: '已有激活码？',
    subtitle: '输入激活码，让下一场相聚即刻开启。',
    icon: Icons.confirmation_number_outlined,
    child: Column(
      children: [
        TextField(
          controller: _code,
          decoration: const InputDecoration(
            labelText: '输入激活码',
            prefixIcon: Icon(Icons.key_rounded, size: 19),
          ),
        ),
        const SizedBox(height: 14),
        SizedBox(
          width: double.infinity,
          child: OutlinedButton(
            onPressed: _busy
                ? null
                : () => _run(() async {
                    await widget.api.redeemActivationCode(
                      widget.session,
                      _code.text,
                    );
                    _code.clear();
                  }),
            child: const Text('兑换会员'),
          ),
        ),
      ],
    ),
  );

  Widget _pointsSection(CheckInStatus status) => FrameSection(
    title: '每天来，攒一点期待',
    subtitle: '签到积累积分，兑换更多相聚时光。',
    icon: Icons.calendar_today_outlined,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Text(
              '${status.availablePoints}',
              style: const TextStyle(
                fontSize: 36,
                fontWeight: FontWeight.w700,
                color: FrameColors.mint,
              ),
            ),
            const SizedBox(width: 10),
            const Text(
              '可用积分',
              style: TextStyle(color: FrameColors.muted, fontSize: 12),
            ),
            const Spacer(),
            FramePill(
              '连续 ${status.consecutiveDays} 天',
              icon: Icons.local_fire_department_outlined,
            ),
          ],
        ),
        const SizedBox(height: 14),
        Wrap(
          spacing: 12,
          runSpacing: 12,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            FilledButton.icon(
              onPressed: _busy || status.checkedInToday
                  ? null
                  : () => _run(() async {
                      await widget.api.dailyCheckIn(widget.session);
                    }),
              icon: const Icon(Icons.check_circle_outline, size: 18),
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
                decoration: const InputDecoration(labelText: '兑换天数'),
              ),
            ),
            TextButton(
              onPressed: _busy || status.pointsPerVipDay <= 0 ? null : _redeem,
              child: Text(
                status.pointsPerVipDay <= 0
                    ? '兑换暂未开放'
                    : '${status.pointsPerVipDay} 积分 / 天',
              ),
            ),
          ],
        ),
      ],
    ),
  );

  @override
  Widget build(BuildContext context) {
    final info = _info;
    final status = _status;
    final membership = _membership;
    return Scaffold(
      appBar: AppBar(
        title: const Text('会员中心'),
        actions: [
          IconButton(
            onPressed: _busy ? null : _load,
            tooltip: '刷新',
            icon: const Icon(Icons.refresh_rounded),
          ),
          const SizedBox(width: 16),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: SingleChildScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 40),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 1100),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    width: double.infinity,
                    padding: const EdgeInsets.all(30),
                    decoration: BoxDecoration(
                      borderRadius: BorderRadius.circular(24),
                      border: Border.all(
                        color: FrameColors.gold.withValues(alpha: .22),
                      ),
                      gradient: const LinearGradient(
                        colors: [Color(0xFF2D3027), Color(0xFF16252A)],
                      ),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const FramePill(
                          'SAMEFRAME PLUS',
                          color: FrameColors.gold,
                          icon: Icons.workspace_premium_outlined,
                        ),
                        const SizedBox(height: 18),
                        Text(
                          '让每一次相聚，\n都有更多可能。',
                          style: Theme.of(context).textTheme.headlineLarge,
                        ),
                        const SizedBox(height: 18),
                        Wrap(
                          spacing: 16,
                          runSpacing: 8,
                          crossAxisAlignment: WrapCrossAlignment.center,
                          children: [
                            FramePill(
                              membership == null
                                  ? '正在加载会员状态'
                                  : membership.active
                                  ? '会员有效'
                                  : '当前为免费账号',
                              color: membership?.active == true
                                  ? FrameColors.gold
                                  : FrameColors.muted,
                            ),
                            if (membership != null && membership.active)
                              Text(
                                '有效期至 ${DateTime.fromMillisecondsSinceEpoch(membership.vipExpiresAt).toLocal().toString().split(' ').first}',
                                style: const TextStyle(
                                  color: FrameColors.muted,
                                  fontSize: 12,
                                ),
                              ),
                            if (info != null && info.announcement.isNotEmpty)
                              Text(
                                info.announcement,
                                style: const TextStyle(
                                  color: FrameColors.gold,
                                  fontSize: 13,
                                ),
                              ),
                          ],
                        ),
                      ],
                    ),
                  ),
                  if (_busy || info == null && _error == null)
                    const Padding(
                      padding: EdgeInsets.only(top: 16),
                      child: LinearProgressIndicator(),
                    ),
                  if (_error != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 16),
                      child: Text(
                        _error!,
                        style: TextStyle(
                          color: Theme.of(context).colorScheme.error,
                        ),
                      ),
                    ),
                  const SizedBox(height: 32),
                  Text(
                    '选择适合你的相聚方式',
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    '套餐与价格以当前服务为准，开通后权益同步至你的账号。',
                    style: TextStyle(color: FrameColors.muted, fontSize: 13),
                  ),
                  const SizedBox(height: 20),
                  if (info != null)
                    LayoutBuilder(
                      builder: (context, box) {
                        final cards = info.plans.map(_planCard).toList();
                        if (box.maxWidth < 760) {
                          return Column(
                            children: [
                              for (final card in cards)
                                Padding(
                                  padding: const EdgeInsets.only(bottom: 16),
                                  child: card,
                                ),
                            ],
                          );
                        }
                        return Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            for (var i = 0; i < cards.length; i++) ...[
                              if (i > 0) const SizedBox(width: 18),
                              Expanded(child: cards[i]),
                            ],
                          ],
                        );
                      },
                    ),
                  const SizedBox(height: 24),
                  LayoutBuilder(
                    builder: (context, box) {
                      if (status == null) return _codeSection();
                      return box.maxWidth < 760
                          ? Column(
                              children: [
                                _pointsSection(status),
                                const SizedBox(height: 16),
                                _codeSection(),
                              ],
                            )
                          : Row(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Expanded(
                                  flex: 6,
                                  child: _pointsSection(status),
                                ),
                                const SizedBox(width: 20),
                                Expanded(flex: 5, child: _codeSection()),
                              ],
                            );
                    },
                  ),
                  const SizedBox(height: 24),
                  FrameSection(
                    title: '我的订单',
                    icon: Icons.receipt_long_outlined,
                    child: _orders.isEmpty
                        ? const FrameEmpty(
                            icon: Icons.receipt_long_outlined,
                            title: '还没有订单',
                            detail: '你的会员订单会显示在这里',
                          )
                        : Column(
                            children: [
                              for (final order in _orders)
                                ListTile(
                                  contentPadding: EdgeInsets.zero,
                                  title: Text(order.planTitle),
                                  subtitle: Text(order.orderNo),
                                  trailing: Text(
                                    '${_money(order.amountMinor)} · ${_orderStatus(order.status)}',
                                  ),
                                ),
                            ],
                          ),
                  ),
                  const SizedBox(height: 20),
                  FrameSection(
                    title: '积分记录',
                    icon: Icons.toll_outlined,
                    child: _transactions.isEmpty
                        ? const FrameEmpty(
                            icon: Icons.toll_outlined,
                            title: '从第一次签到开始',
                            detail: '每一份积分，都有迹可循',
                          )
                        : Column(
                            children: [
                              for (final item in _transactions)
                                ListTile(
                                  contentPadding: EdgeInsets.zero,
                                  title: Text(_transactionType(item.type)),
                                  subtitle: Text('余额 ${item.balanceAfter}'),
                                  trailing: Text(
                                    item.change > 0
                                        ? '+${item.change}'
                                        : '${item.change}',
                                    style: const TextStyle(
                                      color: FrameColors.mint,
                                    ),
                                  ),
                                ),
                            ],
                          ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
