'use strict';

const field = (view, id) => view.querySelector(`#${id}`);
const number = value => value == null ? '—' : Number(value).toLocaleString('zh-CN');
const money = value => value == null ? '—' : `¥${(Number(value) / 100).toLocaleString('zh-CN', {minimumFractionDigits: 2, maximumFractionDigits: 2})}`;
const dateTime = value => value ? new Date(value).toLocaleString('zh-CN', {hour12: false}) : '—';
const roles = {'': '普通用户', super_admin: '超级管理员', admin: '管理员', operator: '运营人员', seller: '销售人员'};
const statuses = {pending: '待处理', submitted: '待审核', triaged: '核验中', paid: '已支付', activated: '已激活', expired: '已过期', cancelled: '已取消', actioned: '已处理', dismissed: '已驳回', rejected: '已驳回', approved: '已批准'};
const kinds = {list: '列表公告', startup: '启动弹窗', room: '房间实时'};
function options(values, selected) {
  return Object.entries(values).map(([value, label]) => `<option value="${esc(value)}" ${value === selected ? 'selected' : ''}>${esc(label)}</option>`).join('');
}
function empty(title, description, name = 'layers') {
  return `<div class="empty-state">${icon(name)}<h3>${esc(title)}</h3><p>${esc(description)}</p></div>`;
}
function panelHeader(title, description, name) {
  return `<div class="panel-header"><div><h2>${esc(title)}</h2><p>${esc(description)}</p></div>${name ? icon(name) : ''}</div>`;
}
function listPanel(title, items, render, emptyTitle, emptyDescription, name) {
  return `<section class="panel"><div class="list-meta"><h2>${esc(title)}</h2><span class="badge">本次加载 ${items.length} 条</span></div><div>${items.map(render).join('') || empty(emptyTitle, emptyDescription, name)}</div></section>`;
}
function safeLink(value) {
  try { const url = new URL(value); return ['http:', 'https:'].includes(url.protocol) ? url.href : ''; }
  catch (_) { return ''; }
}

async function dashboard(view) {
  const data = await api('/api/v1/admin/dashboard');
  const metrics = [
    ['累计用户', data.users, 'users', '已注册的用户总数'],
    ['房间总数', data.rooms, 'room', '所有已创建的观影房间'],
    ['有效会员', data.active_vip_users, 'plans', '当前权益有效的用户'],
    ['累计收入', data.revenue_minor, 'money', '已激活订单金额 · 人民币']
  ];
  const stats = [['verified_users', '已验证邮箱'], ['active_rooms', '活跃房间'], ['couples', '情侣关系'], ['activated_orders', '已激活订单'], ['reviews', '已发布影评'], ['together_watchings', '共同观看']];
  const queue = [
    ['reports', '待审举报', '查看用户提交的内容举报', data.pending_reports],
    ['deletions', '注销申请', '核验申请与账号信息', data.pending_deletions],
    ['complaints', '版权投诉', `其中 ${number(data.overdue_copyright_complaints)} 项已超时`, data.open_copyright_complaints]
  ];
  view.innerHTML = `<div class="metric-grid">${metrics.map(([label, value, name, description], index) => `<article class="metric ${index === 0 ? 'featured' : ''}"><div class="metric-top"><span>${label}</span>${icon(name)}</div><strong>${index === 3 ? money(value) : number(value)}</strong><small>${description}</small></article>`).join('')}</div>
    <div class="dashboard-columns"><section class="panel">${panelHeader('运营数据', '每一次连接，都有迹可循。', 'layers')}<div class="stat-grid">${stats.map(([key, label]) => `<div class="stat"><strong>${number(data[key])}</strong><span>${label}</span></div>`).join('')}</div><div class="data-caption">${icon('clock')}<span>数据读取于 ${dateTime(Date.now())}</span></div></section>
    <section class="panel">${panelHeader('待办中心', '需要你关注的审核与申请。', 'shield')}${queue.map(([tab, label, description, count]) => `<button type="button" class="queue-item" data-tab="${tab}"><span class="queue-icon">${icon(tab)}</span><span><strong>${label}</strong><small>${description}</small></span><span class="queue-count">${number(count)}</span>${icon('chevron')}</button>`).join('')}</section></div>
    <section class="panel quick-panel">${panelHeader('常用操作', '从这里开始今天的工作。')}<div class="quick-grid">${[['announcements', '发布公告', '向用户传递重要消息'], ['plans', '管理会员套餐', '调整价格与会员权益'], ['config', '运行配置', '管理服务与功能开关']].map(([tab, label, description]) => `<button type="button" class="quick-link" data-tab="${tab}">${icon(tab)}<span><strong>${label}</strong><small>${description}</small></span>${icon('arrow')}</button>`).join('')}</div></section>`;
}

async function config(view) {
  const data = await api('/api/v1/admin/config');
  view.innerHTML = `<section class="panel form-panel">${panelHeader('服务与功能', '配置保存后立即生效。', 'config')}<form id="configForm">
    <div class="switch-row"><label for="maintenance">维护模式<small>启用后暂停普通用户访问，管理入口仍可使用。</small></label><input class="toggle" id="maintenance" type="checkbox" ${data.maintenance.enabled ? 'checked' : ''}></div>
    <label>维护提示<input id="maintenanceMessage" value="${esc(data.maintenance.message)}" placeholder="告诉用户预计恢复时间"></label>
    <label>全局公告<textarea id="globalAnnouncement" placeholder="显示在客户端的全局公告">${esc(data.branding.global_announcement)}</textarea></label>
    <label>功能开关（JSON）<textarea class="code-input" id="features" spellcheck="false">${esc(JSON.stringify(data.features, null, 2))}</textarea></label>
    <div class="form-actions"><button id="saveConfig" type="submit">${icon('check')}确认并更新配置</button><span class="muted">此操作会影响所有用户</span></div></form></section>`;
  bindForm(view, '#configForm', async () => {
    const features = JSON.parse(field(view, 'features').value);
    if (!confirm('配置会立即影响所有用户，确认更新？')) return;
    await mutate('/api/v1/admin/config', 'PUT', {
      maintenance: {enabled: field(view, 'maintenance').checked, message: field(view, 'maintenanceMessage').value},
      branding: {global_announcement: field(view, 'globalAnnouncement').value, startup_announcement_id: data.branding.startup_announcement_id || 0}, features
    }, true, {'X-Confirm-Dangerous': 'update-runtime-config'});
  });
}
async function announcements(view) {
  const data = await api('/api/v1/admin/announcements');
  view.innerHTML = `<section class="panel">${panelHeader('发布新公告', '选择展示位置，再填写公告内容。', 'announcements')}<form id="announcementForm"><div class="row"><label>公告标题<input id="aTitle" required placeholder="输入标题"></label><label>展示位置<select id="aKind">${options(kinds, 'list')}</select></label></div><label>公告内容<textarea id="aBody" required placeholder="你想与用户分享什么？"></textarea></label><div class="form-actions"><button id="createAnnouncement" type="submit">${icon('announcements')}发布公告</button></div></form></section>
    ${listPanel('已发布公告', data.announcements || [], item => `<article class="item"><div class="item-header"><h3>${esc(item.title)}</h3><span class="badge">${esc(kinds[item.kind] || item.kind)}</span></div><p>${esc(item.body)}</p><button type="button" class="danger deleteA" data-id="${esc(item.id)}">${icon('deletions')}删除公告</button></article>`, '还没有公告', '发布第一条公告，与用户保持联系。', 'announcements')}`;
  bindForm(view, '#announcementForm', () => mutate('/api/v1/admin/announcements', 'POST', {title: field(view, 'aTitle').value, body: field(view, 'aBody').value, kind: field(view, 'aKind').value, active: true}));
  bindActions(view, '.deleteA', button => mutate(`/api/v1/admin/announcements/${encodeURIComponent(button.dataset.id)}`, 'DELETE', null));
}
async function users(view) {
  const query = new URLSearchParams(location.hash.slice(1)).get('q') || '';
  const data = await api(`/api/v1/admin/users?q=${encodeURIComponent(query)}`);
  const users = data.users || [];
  view.innerHTML = `<section class="panel"><form id="userSearch" class="search-row"><div class="search-field">${icon('search')}<label for="userQuery">搜索用户</label><input id="userQuery" value="${esc(query)}" placeholder="搜索昵称或邮箱" type="search"></div><button id="searchUsers" type="submit">搜索</button></form><div class="list-meta"><h2>用户列表</h2><span class="badge">本次加载 ${users.length} 位</span></div><div>${users.map((user, index) => `<article class="item"><div class="item-header"><div class="avatar" aria-hidden="true">${esc(Array.from(user.display_name || 'U')[0])}</div><div class="item-identity"><h3>${esc(user.display_name)}</h3><p>${esc(user.email)}</p></div><span class="badge">${icon(user.email_verified ? 'check' : 'clock')}${user.email_verified ? '邮箱已验证' : '邮箱待验证'}</span></div><form class="row userForm" data-index="${index}"><label>账号角色<select class="role">${options(roles, user.admin_role || '')}</select></label><label>增加会员天数<input class="vipDays" type="number" min="1" step="1" placeholder="不调整请留空"></label><button class="saveUser" type="submit">保存更改</button></form></article>`).join('') || empty('没有找到用户', '尝试使用其他昵称或邮箱搜索。', 'users')}</div></section>`;
  bindForm(view, '#userSearch', async () => {
    history.replaceState(null, '', `#q=${encodeURIComponent(field(view, 'userQuery').value)}`);
    await loadTab();
  });
  view.querySelectorAll('.userForm').forEach(form => form.addEventListener('submit', event => {
    event.preventDefault();
    perform(event.submitter, async () => {
      const user = users[Number(form.dataset.index)], role = form.querySelector('.role').value;
      const days = Number(form.querySelector('.vipDays').value || 0);
      const tab = state.tab;
      if (role !== (user.admin_role || '')) await api(`/api/v1/admin/users/${encodeURIComponent(user.id)}/role`, {method: 'PUT', body: JSON.stringify({role})});
      if (days > 0) await api(`/api/v1/admin/users/${encodeURIComponent(user.id)}/vip`, {method: 'PUT', body: JSON.stringify({days})});
      if (state.tab === tab) await loadTab({keepNotice: true});
      notify('用户信息已保存');
    });
  }));
}
async function reports(view) {
  const data = await api('/api/v1/admin/reports?status=pending');
  view.innerHTML = listPanel('待审举报', data.reports || [], item => `<article class="item"><h3>${esc(item.reason)}</h3><p>${esc(item.target_type)} · ${esc(item.target_id)}</p><p>${esc(item.details)}</p><div class="item-actions"><button type="button" class="resolve" data-id="${esc(item.id)}" data-status="actioned">标记已处理</button><button type="button" class="resolve secondary" data-id="${esc(item.id)}" data-status="dismissed">驳回举报</button></div></article>`, '当前没有待审举报', '新的用户举报将显示在这里。', 'shield');
  bindActions(view, '.resolve', button => mutate(`/api/v1/admin/reports/${encodeURIComponent(button.dataset.id)}/resolve`, 'POST', {status: button.dataset.status, resolution: '管理后台处理'}));
}
async function deletions(view) {
  const data = await api('/api/v1/admin/account-deletions?status=pending');
  view.innerHTML = listPanel('待审注销申请', data.requests || [], item => `<article class="item"><div class="item-header"><h3>申请 #${esc(item.id)}</h3><span class="badge">待核验</span></div><p>${esc(item.reason)}</p><p>提交时间 · ${dateTime(item.requested_at)}</p><div class="item-actions"><button type="button" class="approveDeletion danger" data-id="${esc(item.id)}">${icon('deletions')}批准并永久注销</button><button type="button" class="rejectDeletion secondary" data-id="${esc(item.id)}">驳回申请</button></div></article>`, '没有待审注销申请', '用户提交注销申请后，可在此核验并处理。', 'users');
  bindActions(view, '.approveDeletion', button => {
    if (confirm('将不可逆删除账号及关联数据，确认？')) return mutate(`/api/v1/admin/account-deletions/${encodeURIComponent(button.dataset.id)}/resolve`, 'POST', {approve: true, resolution: '身份及申请已核验', confirm: 'IRREVERSIBLY DELETE ACCOUNT'});
  });
  bindActions(view, '.rejectDeletion', button => mutate(`/api/v1/admin/account-deletions/${encodeURIComponent(button.dataset.id)}/resolve`, 'POST', {approve: false, resolution: '申请未通过核验'}));
}
async function complaints(view) {
  const data = await api('/api/v1/admin/copyright-complaints');
  view.innerHTML = listPanel('版权投诉记录', data.complaints || [], item => `<article class="item"><div class="item-header"><h3>投诉 #${esc(item.id)}</h3><span class="badge">${esc(statuses[item.status] || item.status)}</span></div><p><strong>${esc(item.claimant_name)}</strong> · ${esc(item.claimant_email)}</p><p>处理截止 · ${dateTime(item.due_at)}</p><p>${esc(item.rights_basis)}</p><p>${safeLink(item.infringement_url) ? `<a href="${esc(safeLink(item.infringement_url))}" target="_blank" rel="noopener noreferrer">查看被投诉链接 ↗</a>` : '链接不可用'} · 房间 ${esc(item.room_code || '—')}</p><details><summary>查看投诉证据</summary><pre>${esc((item.evidence || []).join('\n'))}</pre></details><div class="item-actions"><button type="button" class="actionComplaint danger" data-id="${esc(item.id)}" data-room="${esc(item.room_code || '')}">下架处理</button><button type="button" class="rejectComplaint secondary" data-id="${esc(item.id)}">驳回投诉</button></div></article>`, '暂无版权投诉', '收到的权利人投诉与证据会显示在这里。', 'complaints');
  bindActions(view, '.actionComplaint', button => mutate(`/api/v1/admin/copyright-complaints/${encodeURIComponent(button.dataset.id)}/resolve`, 'POST', {status: 'actioned', resolution: '权属核验后下架', close_room: Boolean(button.dataset.room)}));
  bindActions(view, '.rejectComplaint', button => mutate(`/api/v1/admin/copyright-complaints/${encodeURIComponent(button.dataset.id)}/resolve`, 'POST', {status: 'rejected', resolution: '证据不足', close_room: false}));
}
async function devices(view) {
  const data = await api('/api/v1/admin/device-bans');
  view.innerHTML = listPanel('已封禁设备', data.bans || [], item => `<article class="item"><h3><code>${esc(item.device_hash)}</code></h3><p>${esc(item.reason)}</p><button type="button" class="unban secondary" data-hash="${esc(item.device_hash)}">解除封禁</button></article>`, '没有已封禁的设备', '设备封禁记录与原因将显示在这里。', 'devices');
  bindActions(view, '.unban', button => mutate(`/api/v1/admin/device-bans/${encodeURIComponent(button.dataset.hash)}`, 'DELETE', null));
}
async function orders(view) {
  const data = await api('/api/v1/admin/orders');
  view.innerHTML = listPanel('订单记录', data.orders || [], item => `<article class="item"><div class="item-header"><h3>${esc(item.order_no)}</h3><span class="badge">${esc(statuses[item.status] || item.status)}</span></div><p>${esc(item.plan_title)} · ${money(item.amount_minor)}</p>${item.status !== 'activated' ? `<button type="button" class="activate secondary" data-id="${esc(item.order_no)}">人工补单</button>` : ''}</article>`, '还没有订单', '会员订单生成后，可在此查看和处理。', 'orders');
  bindActions(view, '.activate', button => mutate(`/api/v1/admin/orders/${encodeURIComponent(button.dataset.id)}/activate`, 'POST', {}));
}
async function plans(view) {
  const data = await api('/api/v1/admin/plans');
  const plans = data.plans || [];
  view.innerHTML = listPanel('套餐设置', plans, (plan, index) => `<article class="item"><div class="item-header"><div><h3>${esc(plan.title)}</h3><p>套餐 ID · ${esc(plan.id)}</p></div><span class="badge">${plan.enabled ? '已上架' : '已下架'}</span></div><form class="row planForm" data-index="${index}"><label>价格（分）<input class="price" type="number" min="0" step="1" required value="${esc(plan.price_minor)}"></label><label>有效天数<input class="duration" type="number" min="0" step="1" required value="${esc(plan.duration_days)}"></label><label class="check-label"><input class="enabled toggle" type="checkbox" ${plan.enabled ? 'checked' : ''}>上架</label><button type="submit" class="savePlan">保存套餐</button></form></article>`, '暂无会员套餐', '套餐配置后会显示在这里。', 'plans');
  view.querySelectorAll('.planForm').forEach(form => form.addEventListener('submit', event => {
    event.preventDefault();
    perform(event.submitter, () => {
      const plan = {...plans[Number(form.dataset.index)], price_minor: Number(form.querySelector('.price').value), duration_days: Number(form.querySelector('.duration').value), enabled: form.querySelector('.enabled').checked};
      return mutate(`/api/v1/admin/plans/${encodeURIComponent(plan.id)}`, 'PUT', plan);
    });
  }));
}
async function codes(view) {
  view.innerHTML = `<section class="panel form-panel">${panelHeader('生成激活码', '选择套餐并设置本批次的数量。', 'codes')}<form id="codeForm" class="row"><label>套餐 ID<input id="codePlan" value="monthly" required></label><label>生成数量<input id="codeCount" type="number" min="1" max="100" step="1" value="10" required></label><button id="createCodes" type="submit">生成激活码</button></form><p class="hint">每批最多 100 个；明文仅在生成后显示一次，请及时安全保存。</p><pre id="codeOutput" aria-live="polite">生成的激活码将显示在这里。</pre></section>`;
  bindForm(view, '#codeForm', async () => {
    const data = await api('/api/v1/admin/activation-codes', {method: 'POST', body: JSON.stringify({plan_id: field(view, 'codePlan').value, count: Number(field(view, 'codeCount').value)})});
    field(view, 'codeOutput').textContent = data.codes.map(code => code.code).join('\n');
    notify(`已生成 ${data.codes.length} 个激活码，请及时保存`);
  });
}
async function bot(view) {
  const data = await api('/api/v1/admin/room-bot');
  view.innerHTML = `<section class="panel form-panel">${panelHeader('助手配置', '设置房间 Bot 的展示身份与调用策略。', 'bot')}<form id="botForm"><div class="switch-row"><label for="botEnabled">启用房间 Bot<small>管理助手的可用状态</small></label><input id="botEnabled" class="toggle" type="checkbox" ${data.enabled ? 'checked' : ''}></div><label>助手名称<input id="botName" value="${esc(data.display_name)}"></label><div class="row"><label>召唤策略<select id="summon">${options({admin: '仅管理员', vip: '会员', allowlist: '允许名单', all: '所有用户'}, data.summon_policy)}</select></label><label>回复策略<select id="reply">${options({mention: '被提及时回复', all: '所有消息', off: '关闭回复'}, data.reply_policy)}</select></label></div><label>提供商 HTTPS 地址<input id="botUrl" type="url" placeholder="https://…" value="${esc(data.provider_base_url)}"></label><label>模型名称<input id="botModel" value="${esc(data.model)}"></label><label>更新密钥<input id="botCredential" type="password" autocomplete="new-password" placeholder="${data.has_credential ? '已配置，留空保留现有密钥' : '尚未配置'}"></label><div class="form-actions"><button id="saveBot" type="submit">保存配置</button><span class="muted">密钥留空时保持原有配置</span></div></form></section>`;
  bindForm(view, '#botForm', () => mutate('/api/v1/admin/room-bot', 'PUT', {enabled: field(view, 'botEnabled').checked, display_name: field(view, 'botName').value, summon_policy: field(view, 'summon').value, reply_policy: field(view, 'reply').value, provider_base_url: field(view, 'botUrl').value, model: field(view, 'botModel').value, credential: field(view, 'botCredential').value}));
}
async function audit(view) {
  const data = await api('/api/v1/admin/audit');
  const events = data.events || [];
  view.innerHTML = `<section class="panel"><div class="audit-summary">${icon(data.chain_valid ? 'shield' : 'alert')}<h2>操作记录</h2><span class="badge">审计链${data.chain_valid ? '有效' : '异常'}</span></div><div class="audit-list">${events.map(item => `<article class="item"><div class="item-header"><h3>${esc(item.action)}</h3><span class="badge">#${esc(item.id)}</span></div><p>${esc(item.target_type)} · ${esc(item.target_id)}<br>${dateTime(item.created_at)}</p><details><summary>查看完整记录</summary><pre>${esc(JSON.stringify(item, null, 2))}</pre></details></article>`).join('') || empty('暂无操作记录', '管理操作发生后，可在此追踪变更。', 'audit')}</div><details><summary>查看原始审计数据</summary><pre>${esc(JSON.stringify(events, null, 2))}</pre></details></section>`;
}
const pages = {dashboard, config, announcements, users, reports, deletions, complaints, devices, orders, plans, codes, bot, audit};
