'use strict';

async function showRegistration(user) {
  const data = await api(`/api/v1/admin/managed-users/${encodeURIComponent(user.id)}/registration`);
  const row = (label, value) => `<div class="registration-row"><dt>${esc(label)}</dt><dd>${esc(value || '未提供')}</dd></div>`;
  const r = data.record;
  let content = '<p>此账号创建时尚未采集注册环境，历史注册 IP 无记录。</p>';
  if (r) {
    const client = r.client || {};
    const method = r.method === 'admin' ? '管理员创建（以下是操作管理员的环境）' : r.method === 'password' ? '注册页' : r.method;
    const sources = {trusted_proxy:'可信反向代理',direct:'服务器连接地址',unverified_proxy:'代理来源未确认'};
    const context = {not_reported:'客户端未上报',invalid:'客户端上报无效',client_reported:'客户端上报'};
    const reasons = {ip:'同 IP',network:'同网络段',device:'同设备标识',browser:'同浏览器标识',environment:'相同环境信息'};
    content = `<p class="hint">${esc(user.display_name)} · ${esc(user.email)}</p><dl class="registration-details">${row('注册时间',dateTime(r.registered_at))}${row('注册方式',method)}${row('访问出口 IP',r.source_ip)}${row('IP 来源',sources[r.ip_source])}${row('代理连接地址',r.peer_ip)}${row('计数网络',r.network)}${row('设备名称 / 平台',[r.device_name,r.device_platform].filter(Boolean).join(' / '))}${row('浏览器 / 系统',r.user_agent)}${row('浏览器提示',[r.client_hints_ua,r.client_hints_os,r.client_mobile].filter(Boolean).join(' · '))}${row('请求语言',r.accept_language)}${row('环境信息来源',context[r.context_status])}${row('时区',client.timezone)}${row('界面语言 / 系统',[client.language,client.platform].filter(Boolean).join(' / '))}${row('屏幕',client.screen_width ? `${client.screen_width} × ${client.screen_height} · 像素比 ${client.pixel_ratio}` : '')}${row('自动化标志',r.context_status === 'client_reported' ? (client.webdriver ? '客户端报告自动化环境' : '客户端未报告自动化环境') : '')}${row('设备标识摘要',r.device_hash)}${row('浏览器标识摘要',r.browser_hash)}${row('环境摘要',r.environment_hash)}${r.created_by ? row('创建管理员',r.created_by) : ''}</dl><p class="hint">IP 为访问本站的网络出口。设备及浏览器字段由客户端提供；相同环境信息可能来自不同用户，仅作为关联线索。</p><h3>关联账号 · ${number(data.related_total)}</h3>${data.related.length ? `<div class="record-list">${data.related.map(u=>`<div class="item"><strong>${esc(u.display_name)}</strong><code>${esc(u.user_id)}</code><p>${u.reasons.map(k=>esc(reasons[k] || k)).join(' · ')}</p></div>`).join('')}</div>${data.related_total > data.related.length ? '<p class="hint">仅展示前 25 个关联账号。</p>' : ''}` : '<p>暂未发现关联账号。</p>'}`;
  }
  const dialog = document.createElement('dialog');
  dialog.className = 'admin-dialog';
  dialog.setAttribute('aria-label','注册来源与环境');
  dialog.innerHTML = `<div class="dialog-heading"><h2>注册来源与环境</h2><button type="button" class="icon-button dialog-close" aria-label="关闭">${icon('close')}</button></div><div class="dialog-form">${content}<div class="dialog-actions"><button type="button" class="secondary dialog-close">关闭</button></div></div>`;
  document.body.append(dialog);
  dialog.querySelectorAll('.dialog-close').forEach(b=>b.onclick=()=>dialog.close());
  dialog.addEventListener('close',()=>dialog.remove(),{once:true});
  dialog.showModal();
}
