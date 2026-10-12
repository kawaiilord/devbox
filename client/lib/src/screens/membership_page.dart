import 'dart:async';

import 'package:flutter/material.dart';

import 'workspace_section.dart';

import 'package:url_launcher/url_launcher.dart';
import 'package:uuid/uuid.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../design.dart';
import '../membership_visuals.dart';

class MembershipPage extends StatefulWidget {
  const MembershipPage({
    super.key,
    required this.api,
    required this.session,
    this.onMembershipChanged,
  });
  final ApiClient api;
  final Session session;
  final ValueChanged<MembershipStatus>? onMembershipChanged;
  @override
  State<MembershipPage> createState() => _MembershipPageState();
}

class _MembershipPageState extends State<MembershipPage> {
  final _code = TextEditingController();
  String? _redemptionRequestId;
  String? _notice;
  int _loadGeneration = 0;
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
    super.dispose();
  }

  Future<void> _load() async {
    final generation = ++_loadGeneration;
    try {
      final values = await Future.wait<Object>([
        widget.api.vipInfo(),
        widget.api.membership(widget.session),
        widget.api.checkInStatus(widget.session),
        widget.api.orders(widget.session),
        widget.api.pointsTransactions(widget.session),
      ]);
      if (!mounted || generation != _loadGeneration) return;
      setState(() {
        _info = values[0] as VipInfo;
        _membership = values[1] as MembershipStatus;
        _status = values[2] as CheckInStatus;
        _orders = values[3] as List<VipOrder>;
        _transactions = values[4] as List<PointsTransaction>;
        _error = null;
      });
      widget.onMembershipChanged?.call(_membership!);
    } catch (exception) {
      if (mounted && generation == _loadGeneration) {
        setState(() => _error = exception.toString());
      }
    }
  }

  Future<void> _run(Future<void> Function() action) async {
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
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
    if (!mounted) return;
    if (order.checkoutUrl.isNotEmpty) {
      await launchUrl(
        Uri.parse(order.checkoutUrl),
        mode: LaunchMode.externalApplication,
      );
    }
    _poll?.cancel();
    _poll = Timer.periodic(const Duration(seconds: 3), (_) async {
      if (!mounted) {
        _poll?.cancel();
        return;
      }
      try {
        final current = await widget.api.order(widget.session, order.orderNo);
        if (current.status == 'activated' ||
            current.status == 'expired' ||
            current.status == 'cancelled') {
          _poll?.cancel();
        }
        if (mounted) await _load();
      } catch (_) {
        _poll?.cancel();
        if (mounted) setState(() => _error = '订单状态暂未同步，请点击刷新重试。');
      }
    });
  });

  String _money(int minor) => '¥${(minor / 100).toStringAsFixed(2)}';

  Future<void> _redeem() async {
    final reward = _status?.monthlyReward;
    if (_busy || reward == null || (_membership?.isLifetime ?? false)) return;
    final accepted = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('兑换 PLUS 会员月卡'),
        content: Text(
          '本次消耗 ${reward.points} 积分，获得 ${reward.days} 天会员。'
          '${_membership?.isCurrent == true ? '有效期将从当前到期时间延长。' : '兑换成功后立即生效。'}',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: Text('确认兑换 · ${reward.points} 积分'),
          ),
        ],
      ),
    );
    if (!mounted || accepted != true) return;
    _redemptionRequestId ??= const Uuid().v4();
    await _run(() async {
      final receipt = await widget.api.redeemMonthlyMembership(
        widget.session,
        _redemptionRequestId!,
      );
      _redemptionRequestId = null;
      if (mounted) {
        setState(() => _notice = '兑换成功，${receipt.durationDays} 天 PLUS 会员已到账。');
      }
    });
  }

  String _date(int milliseconds) {
    final d = DateTime.fromMillisecondsSinceEpoch(milliseconds).toLocal();
    return '${d.year}.${d.month.toString().padLeft(2, '0')}.${d.day.toString().padLeft(2, '0')}';
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
    final accent = plan.popular
        ? FrameColors.of(context).silver
        : FrameColors.of(context).accent;
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: plan.popular
            ? FrameColors.of(context).elevated
            : FrameColors.of(context).surface,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: plan.popular
              ? FrameColors.of(context).silver.withValues(alpha: .6)
              : FrameColors.of(context).border,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            height: 32,
            child: Row(
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
                  FramePill('推荐之选', color: FrameColors.of(context).silver),
              ],
            ),
          ),
          const SizedBox(height: 22),
          Text(
            plan.title,
            style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 6),
          Text(
            plan.lifetime ? '一次开通，长久相伴' : '${plan.durationDays} 天相聚时光',
            style: TextStyle(
              color: FrameColors.of(context).muted,
              fontSize: 13,
            ),
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
                    style: TextStyle(
                      color: FrameColors.of(context).muted,
                      fontSize: 12,
                      decoration: TextDecoration.lineThrough,
                    ),
                  )
                : Text(
                    '按当前套餐价格开通',
                    style: TextStyle(
                      color: FrameColors.of(context).muted,
                      fontSize: 12,
                    ),
                  ),
          ),
          const Divider(height: 32),
          Row(
            children: [
              Icon(
                Icons.check_rounded,
                size: 16,
                color: FrameColors.of(context).accent,
              ),
              const SizedBox(width: 8),
              const Text('延长房间有效期', style: TextStyle(fontSize: 13)),
            ],
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Icon(
                Icons.check_rounded,
                size: 16,
                color: FrameColors.of(context).accent,
              ),
              const SizedBox(width: 8),
              const Text('解锁情侣绑定', style: TextStyle(fontSize: 13)),
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

  Widget _checkInCard(CheckInStatus status) => FrameSection(
    title: '每日签到',
    subtitle: '每次签到 +${status.pointsPerCheckIn} 积分，北京时间每天 08:00 刷新。',
    icon: Icons.calendar_today_outlined,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Text(
              '${status.availablePoints}',
              style: const TextStyle(
                fontSize: 42,
                fontWeight: FontWeight.w600,
                letterSpacing: -1.5,
              ),
            ),
            const Padding(
              padding: EdgeInsets.only(left: 8, bottom: 8),
              child: Text('可用积分'),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          '累计获得 ${status.totalPoints} · 已使用 ${status.usedPoints}',
          style: TextStyle(fontSize: 12, color: FrameColors.of(context).muted),
        ),
        const SizedBox(height: 20),
        _checkInWeek(status),
        const SizedBox(height: 18),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            FilledButton.icon(
              onPressed: _busy || status.checkedInToday
                  ? null
                  : () => _run(() async {
                      final result = await widget.api.dailyCheckIn(
                        widget.session,
                      );
                      if (mounted) {
                        setState(() {
                          _status = result;
                          _notice = '签到成功，今日积分已领取。';
                        });
                      }
                    }),
              icon: Icon(
                status.checkedInToday
                    ? Icons.check_circle_rounded
                    : Icons.add_rounded,
                size: 18,
              ),
              label: Text(
                status.checkedInToday
                    ? '今日已签到'
                    : '签到领取 ${status.pointsPerCheckIn} 积分',
              ),
            ),
            Text(
              '连续 ${status.consecutiveDays} 天',
              style: TextStyle(
                color: FrameColors.of(context).muted,
                fontSize: 12,
              ),
            ),
          ],
        ),
      ],
    ),
  );

  Widget _checkInWeek(CheckInStatus status) {
    final today =
        DateTime.tryParse('${status.checkInDate}T00:00:00Z') ??
        DateTime.now().toUtc();
    final colors = FrameColors.of(context);
    return Row(
      children: [
        for (var i = 6; i >= 0; i--) ...[
          if (i < 6) const SizedBox(width: 5),
          Expanded(
            child: Builder(
              builder: (context) {
                final date = today.subtract(Duration(days: i));
                final key =
                    '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')}';
                final checked =
                    status.recentCheckIns.contains(key) ||
                    (i == 0 && status.checkedInToday);
                return Column(
                  children: [
                    Container(
                      height: 34,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: checked ? colors.text : colors.elevated,
                        borderRadius: BorderRadius.circular(9),
                      ),
                      child: checked
                          ? Icon(
                              Icons.check_rounded,
                              color: colors.background,
                              size: 17,
                            )
                          : Text(
                              '${date.day}',
                              style: TextStyle(
                                fontSize: 12,
                                color: colors.muted,
                              ),
                            ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      i == 0 ? '今天' : '${date.month}/${date.day}',
                      style: TextStyle(fontSize: 9, color: colors.muted),
                    ),
                  ],
                );
              },
            ),
          ),
        ],
      ],
    );
  }

  Widget _rewardCard(CheckInStatus status) {
    final reward = status.monthlyReward;
    final cost = reward?.points ?? 30;
    final missing = (cost - status.availablePoints).clamp(0, cost);
    final lifetime = _membership?.isLifetime ?? false;
    return FrameSection(
      title: '会员月卡',
      subtitle: '每张月卡增加 ${reward?.days ?? 30} 天，有效会员可累加续期。',
      icon: Icons.workspace_premium_outlined,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Text(
                '$cost',
                style: const TextStyle(
                  fontSize: 42,
                  fontWeight: FontWeight.w600,
                  letterSpacing: -1.5,
                ),
              ),
              const Padding(
                padding: EdgeInsets.only(left: 8, bottom: 8),
                child: Text('积分 / 月'),
              ),
              const Spacer(),
              const MemberBadge(),
            ],
          ),
          const SizedBox(height: 16),
          ClipRRect(
            borderRadius: BorderRadius.circular(10),
            child: LinearProgressIndicator(
              value: (status.availablePoints / cost).clamp(0, 1).toDouble(),
              minHeight: 6,
            ),
          ),
          const SizedBox(height: 10),
          Text(
            lifetime
                ? '你已享有长期会员，无需兑换月卡。'
                : missing == 0
                ? '积分已足够，可以兑换会员月卡。'
                : '还差 $missing 积分即可兑换。',
            style: TextStyle(
              color: FrameColors.of(context).muted,
              fontSize: 12,
            ),
          ),
          const SizedBox(height: 20),
          SizedBox(
            width: double.infinity,
            child: FilledButton.icon(
              onPressed: _busy || reward == null || missing > 0 || lifetime
                  ? null
                  : _redeem,
              icon: const Icon(Icons.diamond_outlined, size: 18),
              label: Text(
                lifetime
                    ? '已享长期会员'
                    : reward == null
                    ? '兑换服务暂不可用'
                    : '兑换月卡 · $cost 积分',
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _pointsSection(CheckInStatus status) => LayoutBuilder(
    builder: (context, box) {
      final items = [_checkInCard(status), _rewardCard(status)];
      return box.maxWidth < 760
          ? Column(children: [items[0], const SizedBox(height: 16), items[1]])
          : Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(child: items[0]),
                const SizedBox(width: 20),
                Expanded(child: items[1]),
              ],
            );
    },
  );

  Widget _benefits() => FrameSection(
    title: '你的 PLUS 身份',
    icon: Icons.diamond_outlined,
    child: Column(
      children: const [
        ListTile(
          contentPadding: EdgeInsets.zero,
          leading: Icon(Icons.workspace_premium_outlined),
          title: Text('铂金会员标识'),
          subtitle: Text('账号、好友列表与房间中展示专属身份。'),
        ),
        ListTile(
          contentPadding: EdgeInsets.zero,
          leading: Icon(Icons.account_circle_outlined),
          title: Text('专属头像边框'),
          subtitle: Text('铂金描边与 PLUS 铭牌，让你的身份清晰可见。'),
        ),
        ListTile(
          contentPadding: EdgeInsets.zero,
          leading: Icon(Icons.meeting_room_outlined),
          title: Text('更长的房间有效期'),
          subtitle: Text('会员期间新建的房间按会员有效期保留。'),
        ),
      ],
    ),
  );

  Widget _purchaseOptions(VipInfo info) => Card(
    child: ExpansionTile(
      title: const Text('其他开通方式'),
      subtitle: const Text('会员套餐与激活码'),
      childrenPadding: const EdgeInsets.all(16),
      children: [
        if (info.paymentMethod != 'disabled') ...[
          LayoutBuilder(
            builder: (context, box) => box.maxWidth < 760
                ? Column(
                    children: [
                      for (final plan in info.plans)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 12),
                          child: _planCard(plan),
                        ),
                    ],
                  )
                : Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      for (final plan in info.plans)
                        Expanded(
                          child: Padding(
                            padding: const EdgeInsets.all(6),
                            child: _planCard(plan),
                          ),
                        ),
                    ],
                  ),
          ),
          const SizedBox(height: 16),
        ],
        _codeSection(),
      ],
    ),
  );

  @override
  Widget build(BuildContext context) {
    final membership = _membership;
    final active = membership?.isCurrent ?? false;
    final detail = membership == null
        ? '正在同步会员状态'
        : membership.isLifetime
        ? '长期有效'
        : active
        ? '有效期至 ${_date(membership.vipExpiresAt)}'
        : membership.vipExpiresAt > 0
        ? '会员已到期 · 可使用积分续期'
        : '普通账号 · 从今天开始积攒';
    return WorkspaceSection(
      title: const Text('会员中心'),
      actions: [
        IconButton(
          onPressed: _busy ? null : _load,
          tooltip: '刷新',
          icon: const Icon(Icons.refresh_rounded),
        ),
      ],
      body: RefreshIndicator(
        onRefresh: _load,
        child: SingleChildScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 40),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 1100),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  MembershipCard(
                    name: widget.session.user.displayName,
                    active: active,
                    detail: detail,
                  ),
                  if (_busy || _status == null && _error == null)
                    const Padding(
                      padding: EdgeInsets.only(top: 16),
                      child: LinearProgressIndicator(),
                    ),
                  if (_notice != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 16),
                      child: Card(
                        child: Padding(
                          padding: const EdgeInsets.all(16),
                          child: Row(
                            children: [
                              const Icon(Icons.check_circle_outline_rounded),
                              const SizedBox(width: 10),
                              Expanded(child: Text(_notice!)),
                            ],
                          ),
                        ),
                      ),
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
                  const SizedBox(height: 24),
                  if (_status != null) _pointsSection(_status!),
                  const SizedBox(height: 24),
                  _benefits(),
                  const SizedBox(height: 20),
                  FrameSection(
                    title: '积分记录',
                    icon: Icons.toll_outlined,
                    child: _transactions.isEmpty
                        ? const FrameEmpty(
                            icon: Icons.toll_outlined,
                            title: '从第一次签到开始',
                            detail: '积分领取与兑换记录会显示在这里。',
                          )
                        : Column(
                            children: [
                              for (final item in _transactions)
                                ListTile(
                                  contentPadding: EdgeInsets.zero,
                                  leading: Icon(
                                    item.change > 0
                                        ? Icons.add_circle_outline
                                        : Icons.workspace_premium_outlined,
                                  ),
                                  title: Text(_transactionType(item.type)),
                                  subtitle: Text(
                                    '${_date(item.createdAt)} · 余额 ${item.balanceAfter}${item.refId.startsWith('monthly:') ? ' · 月卡 30 天' : ''}',
                                  ),
                                  trailing: Text(
                                    item.change > 0
                                        ? '+${item.change}'
                                        : '${item.change}',
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                                ),
                            ],
                          ),
                  ),
                  if (_info != null) ...[
                    const SizedBox(height: 20),
                    _purchaseOptions(_info!),
                  ],
                  if (_orders.isNotEmpty) ...[
                    const SizedBox(height: 20),
                    FrameSection(
                      title: '我的订单',
                      icon: Icons.receipt_long_outlined,
                      child: Column(
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
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
