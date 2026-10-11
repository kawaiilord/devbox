'use strict';

const $ = id => document.getElementById(id);
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const icons = {
  dashboard: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  users: '<circle cx="9" cy="8" r="3"/><path d="M3 21v-2a6 6 0 0 1 12 0v2M16 5a3 3 0 0 1 0 6m2 4a5 5 0 0 1 3 4v2"/>',
  announcements: '<path d="m3 9 14-5v16L3 15zm3 7 2 5h3l-2-4M21 9v6"/>',
  reports: '<path d="M5 21V4m0 0c5-5 9 5 14 0v10c-5 5-9-5-14 0"/>',
  deletions: '<path d="M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14M10 11v6m4-6v6"/>',
  complaints: '<circle cx="12" cy="12" r="9"/><path d="M15 9a4 4 0 1 0 0 6"/>',
  devices: '<rect x="6" y="2" width="12" height="20" rx="3"/><path d="M10 18h4"/>',
  orders: '<path d="M5 3h14v18l-3-2-4 2-4-2-3 2zM9 8h6m-6 4h6"/>',
  plans: '<path d="m3 6 5 4 4-7 4 7 5-4-3 13H6zM8 22h8"/>',
  codes: '<rect x="3" y="5" width="18" height="14" rx="3"/><path d="M8 9v6m4-6v6m4-6v6"/>',
  config: '<path d="M4 7h8m4 0h4M4 17h3m4 0h9"/><circle cx="14" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>',
  bot: '<rect x="4" y="7" width="16" height="14" rx="4"/><path d="M12 7V3M2 12v5m20-5v5M9 17h6m-7-5h1m6 0h1"/>',
  audit: '<path d="M14 3H5v18h14V8zm0 0v5h5M9 12h6m-6 4h6"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>',
  moon: '<path d="M21 13A9 9 0 0 1 11 3a9 9 0 1 0 10 10Z"/>',
  monitor: '<rect x="3" y="3" width="18" height="13" rx="2"/><path d="M8 21h8m-4-5v5"/>',
  arrow: '<path d="M4 12h16m-6-6 6 6-6 6"/>',
  chevron: '<path d="m9 5 7 7-7 7"/>',
  refresh: '<path d="M20 7v5h-5M4 17v-5h5m-5 0a8 8 0 0 1 13-6l3 3M4 15l3 3a8 8 0 0 0 13-6"/>',
  close: '<path d="m6 6 12 12M6 18 18 6"/>',
  logout: '<path d="M10 4H4v16h6m-1-8h12m-4-4 4 4-4 4"/>',
  grid: '<circle cx="6" cy="6" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="6" cy="18" r="2"/><circle cx="18" cy="18" r="2"/>',
  layers: '<path d="m12 3 10 5-10 5L2 8zm-9 10 9 5 9-5M3 18l9 5 9-5"/>',
  play: '<path d="m9 5 11 7-11 7z"/>',
  room: '<rect x="2" y="5" width="15" height="14" rx="3"/><path d="m17 10 5-3v10l-5-3"/>',
  lock: '<rect x="5" y="10" width="14" height="11" rx="3"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 5v2"/>',
  shield: '<path d="m12 3 8 3v6c0 4-4 7-8 9-4-2-8-5-8-9V6zm-4 9 3 3 5-6"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
  alert: '<path d="m12 3 10 18H2zM12 9v5m0 3v.1"/>',
  money: '<rect x="2" y="5" width="20" height="14" rx="3"/><circle cx="12" cy="12" r="3"/><path d="M6 12h.01M18 12h.01"/>'
};
function icon(name) {
  return `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${icons[name] || icons.dashboard}</svg>`;
}
const tabs = [
  ['dashboard', '数据概览', 'OVERVIEW', '用户、房间与运营待办，一目了然。', '工作空间'],
  ['rooms', '房间管理', 'ROOMS', '管理全站房间、成员权限与同步播放。', '工作空间'],
  ['users', '用户管理', 'PEOPLE', '查看用户信息，管理角色与会员权益。', '工作空间'],
  ['taxonomy', '分类与标签', 'TAXONOMY', '统一管理房间分类、标签与展示顺序。', '工作空间'],
  ['announcements', '公告管理', 'ANNOUNCEMENTS', '把重要的消息，传递给每一位用户。', '工作空间'],
  ['approvals', '申请审批', 'APPROVALS', '处理注册、建房和加入房间申请。', '内容与安全'],
  ['reports', '举报审核', 'MODERATION', '集中处理用户举报，维护良好的观影环境。', '内容与安全'],
  ['deletions', '注销申请', 'ACCOUNT REQUESTS', '核验账号注销申请与处理结果。', '内容与安全'],
  ['complaints', '版权投诉', 'COPYRIGHT', '审查权利证明、投诉证据与处理时限。', '内容与安全'],
  ['devices', '设备管理', 'DEVICES', '查看设备封禁记录，管理访问限制。', '内容与安全'],
  ['orders', '订单管理', 'ORDERS', '查看会员订单与支付激活状态。', '商业化'],
  ['plans', '会员套餐', 'MEMBERSHIP', '管理套餐价格、有效期与上架状态。', '商业化'],
  ['codes', '激活卡密', 'ACTIVATION CODES', '按批次生成会员激活码。', '商业化'],
  ['config', '运行配置', 'CONFIGURATION', '管理维护模式、全局公告与功能开关。', '系统'],
  ['policy', '站点策略', 'SITE POLICIES', '配置注册、建房、访客和聊天策略，即时生效。', '系统'],
  ['sources', '媒体服务', 'MEDIA SERVICES', '管理全站已绑定的媒体来源与连接状态。', '系统'],
  ['remote_providers', '远程 Provider', 'REMOTE PROVIDERS', '配置服务端点、TLS 和共享密钥，检测连接状态。', '系统'],
  ['slice_cache', '分片缓存', 'SLICE CACHE', '管理各节点容量、命中情况与清理任务。', '系统'],
  ['rtmp', '直播推流', 'LIVE STREAMS', '管理推流策略、活动连接和房间密钥。', '系统'],
  ['smtp', '邮件服务', 'MAIL', '配置 SMTP 投递、邮件模板与发送测试。', '系统'],
  ['login_security', '登录与安全', 'AUTHENTICATION', '管理第三方登录、Passkey 和动态验证码。', '系统'],
  ['bot', '房间 Bot', 'ROOM ASSISTANT', '配置房间助手的身份、策略与服务连接。', '系统'],
  ['audit', '操作审计', 'AUDIT LOG', '追踪管理操作，查看审计链的完整性。', '系统']
];
icons.remote_providers=icons.layers;
icons.smtp = icons.announcements;
icons.login_security = icons.shield;
icons.approvals = icons.check;
icons.rooms = icons.room;
icons.taxonomy = icons.layers;
icons.sources = icons.layers;
icons.policy = icons.config;
const tabPermissions = {slice_cache:'config.write',rtmp:'config.write',remote_providers:'config.write',smtp:'config.write',login_security:'config.write',approvals:'review.manage',dashboard:'observability.view', rooms:'room.view', users:'user.view', taxonomy:'taxonomy.manage', announcements:'announcement.manage', reports:'report.review', deletions:'compliance.review', complaints:'compliance.review', devices:'device.ban', orders:'order.view', plans:'config.write', codes:'activation_code.manage', config:'config.read', policy:'config.write', sources:'source.manage', bot:'bot.manage', audit:'audit.view'};
const state = {
  permissions: [],
  filters: {},
  roomCode: '',
  token: sessionStorage.getItem('sf_admin_token') || '',
  device: localStorage.getItem('sf_admin_device') || crypto.randomUUID(),
  tab: sessionStorage.getItem('sf_admin_tab') || 'dashboard',
  request: 0
};
const can = permission => state.permissions.includes('*') || state.permissions.includes(permission);
if (!tabs.some(([id]) => id === state.tab)) state.tab = 'dashboard';
localStorage.setItem('sf_admin_device', state.device);
const mobile = matchMedia('(max-width: 820px)');

async function api(path, options = {}) {
  const headers = {'Content-Type': 'application/json', 'X-Device-ID': state.device,
    'X-Device-Name': 'Admin Web', 'X-Device-Platform': 'web-admin', ...options.headers};
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  const response = await fetch(path, {...options, headers});
  const body = await response.json().catch(() => ({msg: '服务响应异常，请稍后重试'}));
  if (!response.ok || body.code !== 0) {
    const error = new Error(body.msg || `HTTP ${response.status}`);
    error.status = response.status;
    error.data = body.data || {};
    throw error;
  }
  return body.data || {};
}
function notify(message, error = false) {
  const notice = $('notice');
  notice.hidden = !message;
  notice.className = error ? 'error' : 'notice';
  notice.setAttribute('role', error ? 'alert' : 'status');
  notice.innerHTML = message ? `${icon(error ? 'alert' : 'check')}<span>${esc(message)}</span>` : '';
}
async function login(event) {
  event.preventDefault();
  $('loginButton').disabled = true;
  $('loginError').hidden = true;
  try {
    let data;
    try { data = await api('/api/v1/auth/login', {method: 'POST', body: JSON.stringify({email: $('email').value, password: $('password').value})}); } catch (error) {
      if (!error.data?.mfa_required) throw error;
      const code = await requestMFACode();
      if (!code) throw new Error('登录已取消');
      data = await api('/api/v1/auth/mfa',{method:'POST',body:JSON.stringify({challenge:error.data.challenge,code})});
    }
    state.token = data.access_token;
    sessionStorage.setItem('sf_admin_token', state.token);
    $('password').value = '';
    await start();
  } catch (error) {
    $('loginError').textContent = error.message;
    $('loginError').hidden = false;
  } finally { $('loginButton').disabled = false; }
}
function logout() {
  sessionStorage.removeItem('sf_admin_token');
  sessionStorage.removeItem('sf_admin_tab');
  location.reload();
}
async function start() {
  const session = await api('/api/v1/admin/session');
  state.permissions = session.permissions;
  state.user = session.user;
  if (!can(tabPermissions[state.tab])) state.tab = tabs.find(([id]) => can(tabPermissions[id]))?.[0] || 'dashboard';
  document.querySelector('.account-copy strong').textContent = session.user.display_name;
  document.querySelector('.account-copy span').textContent = roles[session.role] || session.role;
  document.querySelectorAll('#mobileDock [data-tab]').forEach(button => { button.hidden = !can(tabPermissions[button.dataset.tab]); });
  $('login').hidden = true;
  $('console').hidden = false;
  renderTabs();
  setMenu(false, false);
  await loadTab();
}
function renderTabs() {
  const visibleTabs = tabs.filter(([id]) => can(tabPermissions[id]));
  const groups = [...new Set(visibleTabs.map(tab => tab[4]))];
  $('tabs').innerHTML = groups.map(group => `<div class="nav-group"><p class="nav-label">${group}</p>${visibleTabs.filter(tab => tab[4] === group).map(([id, label]) => `<button type="button" class="nav-button" data-tab="${id}">${icon(id)}<span>${label}</span></button>`).join('')}</div>`).join('');
  bindNavigation($('tabs'));
}
function bindNavigation(root) {
  root.querySelectorAll('[data-tab]').forEach(button => button.addEventListener('click', () => navigate(button.dataset.tab)));
}
function updateNavigation() {
  document.querySelectorAll('#tabs [data-tab], #mobileDock [data-tab]').forEach(button => {
    const active = button.dataset.tab === state.tab;
    button.classList.toggle('active', active);
    if (active) button.setAttribute('aria-current', 'page');
    else button.removeAttribute('aria-current');
  });
  $('openMenu').classList.toggle('active', !['dashboard', 'rooms', 'users', 'orders'].includes(state.tab));
}
function navigate(tab) {
  if (!tabs.some(([id]) => id === tab)) return;
  if (!can(tabPermissions[tab])) { notify('当前账号没有此项权限', true); return; }
  state.tab = tab;
  sessionStorage.setItem('sf_admin_tab', tab);
  setMenu(false, false);
  window.scrollTo({top: 0, behavior: 'instant'});
  loadTab().then(() => $('content').focus({preventScroll: true}));
}
async function loadTab({keepNotice = false} = {}) {
  const request = ++state.request;
  const tab = state.tab;
  const [, title, eyebrow, description] = tabs.find(([id]) => id === tab);
  $('pageTitle').textContent = title;
  $('breadcrumbTitle').textContent = title;
  $('pageEyebrow').textContent = eyebrow;
  $('pageDescription').textContent = description;
  document.title = `${title} · 同帧管理控制台`;
  updateNavigation();
  if (!keepNotice) notify('');
  $('refresh').disabled = true;
  $('content').setAttribute('aria-busy', 'true');
  $('content').innerHTML = '<div class="panel loading-state" role="status"><span class="spinner" aria-hidden="true"></span><p>正在读取数据…</p></div>';
  // Build each response offscreen; a slow previous tab cannot replace the current tab.
  const view = document.createElement('div');
  view.dataset.page = tab;
  try {
    await pages[tab](view);
    if (request !== state.request) return;
    $('content').replaceChildren(view);
    bindNavigation(view);
  } catch (error) {
    if (request !== state.request) return;
    $('content').innerHTML = `<div class="panel empty-state" role="alert">${icon('alert')}<h3>${error.status === 403 ? '当前账号没有此项权限' : error.status === 401 ? '登录已过期' : '暂时无法读取数据'}</h3><p>${esc(error.message)}</p><button id="retryPage" type="button" class="secondary">${error.status === 401 ? '重新登录' : '重新加载'}</button></div>`;
    $('retryPage').onclick = error.status === 401 ? logout : () => loadTab();
  } finally {
    if (request === state.request) {
      $('content').setAttribute('aria-busy', 'false');
      $('refresh').disabled = false;
    }
  }
}
function setMenu(open, restoreFocus = true) {
  open = Boolean(open && mobile.matches);
  document.body.classList.toggle('menu-open', open);
  $('drawerBackdrop').hidden = !open;
  $('openMenu').setAttribute('aria-expanded', String(open));
  $('sidebar').inert = mobile.matches && !open;
  $('workspace').inert = open;
  $('mobileDock').inert = open;
  if (open) {
    $('sidebar').setAttribute('role', 'dialog');
    $('sidebar').setAttribute('aria-modal', 'true');
    $('closeMenu').focus();
  } else {
    $('sidebar').removeAttribute('role');
    $('sidebar').removeAttribute('aria-modal');
    if (restoreFocus && mobile.matches) $('openMenu').focus();
  }
}
async function perform(button, action) {
  if (button?.disabled) return;
  if (button) button.disabled = true;
  try { await action(); } catch (error) { notify(error.message, true); }
  finally { if (button) button.disabled = false; }
}
function bindForm(view, selector, action) {
  view.querySelector(selector).addEventListener('submit', event => {
    event.preventDefault();
    perform(event.submitter, action);
  });
}
function bindActions(view, selector, action) {
  view.querySelectorAll(selector).forEach(button => button.addEventListener('click', () => perform(button, () => action(button))));
}
async function mutate(path, method, body, reload = true, headers = {}) {
  const tab = state.tab;
  await api(path, {method, headers, body: body === null ? undefined : JSON.stringify(body)});
  if (reload && state.tab === tab) await loadTab({keepNotice: true});
  notify('操作已保存');
}

document.querySelectorAll('[data-icon]').forEach(element => { element.innerHTML = icon(element.dataset.icon); });
window.adminTheme.apply();
document.querySelectorAll('[data-theme-choice]').forEach(button => button.addEventListener('click', () => window.adminTheme.set(button.dataset.themeChoice)));
$('loginForm').addEventListener('submit', login);
$('logout').onclick = logout;
$('refresh').onclick = () => loadTab();
$('openMenu').onclick = () => setMenu(true);
$('closeMenu').onclick = () => setMenu(false);
$('drawerBackdrop').onclick = () => setMenu(false);
bindNavigation($('mobileDock'));
mobile.addEventListener('change', () => setMenu(false, false));
document.addEventListener('keydown', event => {
  if (!document.body.classList.contains('menu-open')) return;
  if (event.key === 'Escape') { event.preventDefault(); setMenu(false); }
  if (event.key === 'Tab') {
    const focusable = [...$('sidebar').querySelectorAll('button:not([disabled])')];
    const first = focusable[0], last = focusable.at(-1);
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }
});
if (state.token) start().catch(error => {
  sessionStorage.removeItem('sf_admin_token');
  state.token = '';
  $('loginError').textContent = error.message;
  $('loginError').hidden = false;
});

function requestMFACode(){
 return new Promise(resolve=>{const dialog=document.createElement('dialog');dialog.className='admin-dialog';dialog.innerHTML='<form method="dialog"><h2>二次验证</h2><label>动态验证码或恢复码<input name="code" autocomplete="one-time-code" required autofocus></label><div class="item-actions"><button value="ok">验证</button><button value="cancel" class="secondary" formnovalidate>取消</button></div></form>';document.body.append(dialog);dialog.addEventListener('close',()=>{resolve(dialog.returnValue==='ok'?dialog.querySelector('input').value.trim():null);dialog.remove();},{once:true});dialog.showModal();});
}
