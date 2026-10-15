'use strict';

const accountStatuses = {active: '正常', banned: '已封禁', deleted: '回收站', pending:'待审批', rejected:'未通过'};
const roomStatuses = {active: '开放中', banned: '已封禁', closed: '已关闭', deleted: '回收站', expired: '已过期', pending:'待审批', rejected:'未通过'};
function filtersFor(tab, defaults = {}) { return state.filters[tab] ||= {q: '', offset: 0, limit: 25, ...defaults}; }
function queryFor(filters) { return new URLSearchParams(Object.entries(filters).filter(([,value]) => value !== '' && value != null)).toString(); }
function textField(name, label, value = '', type = 'text', attributes = '') {
  return `<label>${esc(label)}<input name="${name}" type="${type}" value="${esc(value)}" ${attributes}></label>`;
}
function toggleField(name, label, checked, description = '') {
  return `<div class="switch-row"><label for="${name}">${esc(label)}${description ? `<small>${esc(description)}</small>` : ''}</label><input id="${name}" name="${name}" class="toggle" type="checkbox" ${checked ? 'checked' : ''}></div>`;
}
function formDialog({title, content, submit = '保存更改', onSubmit, setup, reload = true, success = '操作已保存'}) {
  const dialog = document.createElement('dialog');
  dialog.className = 'admin-dialog';
  dialog.setAttribute('aria-labelledby', 'dialogTitle');
  dialog.innerHTML = `<div class="dialog-heading"><h2 id="dialogTitle">${esc(title)}</h2><button type="button" class="icon-button dialog-close" aria-label="关闭">${icon('close')}</button></div><form class="dialog-form">${content}<p class="error" role="alert" hidden></p><div class="dialog-actions"><button type="button" class="secondary dialog-close">取消</button><button type="submit" class="dialog-submit">${esc(submit)}</button></div></form>`;
  document.body.append(dialog);
  const form = dialog.querySelector('form');
  const value = name => form.querySelector(`[name="${name}"]`)?.value ?? '';
  const checked = name => Boolean(form.querySelector(`[name="${name}"]`)?.checked);
  const openedTab = state.tab;
  dialog.querySelectorAll('.dialog-close').forEach(button => button.onclick = () => dialog.close());
  dialog.addEventListener('close', () => dialog.remove(), {once: true});
  form.addEventListener('submit', async event => {
    event.preventDefault();
    const button = form.querySelector('.dialog-submit');
    if (button.disabled) return;
    const errorElement = form.querySelector('.error');
    errorElement.hidden = true; button.disabled = true; button.textContent = '处理中…';
    try {
      const result = await onSubmit({value, checked, form, dialog});
      dialog.close();
      if (reload && state.tab === openedTab) await loadTab({keepNotice: true});
      if (success) notify(typeof result === 'string' ? result : success);
    } catch (error) {
      errorElement.textContent = error.message;
      errorElement.hidden = false;
    } finally { button.disabled = false; button.textContent = submit; }
  });
  if (setup) setup({value, checked, form, dialog});
  dialog.showModal();
  return dialog;
}
function pagination(data) {
  return `<div class="pagination"><span>共 ${number(data.total)} 条 · 第 ${Math.floor(data.offset / data.limit) + 1} 页</span><div class="pagination-actions"><button type="button" class="secondary previous-page" ${data.offset <= 0 ? 'disabled' : ''}>上一页</button><button type="button" class="secondary next-page" ${!data.has_more ? 'disabled' : ''}>下一页</button></div></div>`;
}
function bindPagination(view, filters, data) {
  view.querySelector('.previous-page')?.addEventListener('click', () => { filters.offset = Math.max(0, data.offset - data.limit); loadTab(); });
  view.querySelector('.next-page')?.addEventListener('click', () => { filters.offset = data.offset + data.limit; loadTab(); });
}
function batchBar(labels) {
  return `<div class="batch-bar"><label class="record-select"><input type="checkbox" class="select-all">选择本页</label><span class="batch-count" aria-live="polite">已选 0 项</span><select class="batch-action" aria-label="批量操作">${options(labels, Object.keys(labels)[0])}</select><button type="button" class="secondary apply-batch" disabled>执行批量操作</button></div>`;
}
function bindBatch(view, items, key, entity) {
  const boxes = [...view.querySelectorAll('.record-checkbox:not(:disabled)')];
  const all = view.querySelector('.select-all');
  if (!all) return;
  function sync() {
    const selected = boxes.filter(box => box.checked).length;
    view.querySelector('.batch-count').textContent = `已选 ${selected} 项`;
    view.querySelector('.apply-batch').disabled = selected === 0;
    all.checked = boxes.length > 0 && selected === boxes.length;
    all.indeterminate = selected > 0 && selected < boxes.length;
  }
  all.onchange = () => { boxes.forEach(box => { box.checked = all.checked; }); sync(); };
  boxes.forEach(box => box.onchange = sync);
  view.querySelector('.apply-batch').onclick = () => {
    const chosen = boxes.filter(box => box.checked).map(box => items[Number(box.dataset.index)]);
    const action = view.querySelector('.batch-action').value;
    const labels = {ban: '封禁', unban: '解封', delete: '移入回收站', restore: '恢复并启用'};
    formDialog({title: `${labels[action]} ${chosen.length} 个${entity === 'users' ? '账号' : '房间'}`, submit: '确认执行',
      content: `<p class="hint">仅处理当前选中的 ${chosen.length} 项。每项会分别检查权限与记录版本；失败项会列出原因。${action === 'delete' ? '移入回收站后可恢复，历史数据保留。' : ''}</p><label>操作原因<textarea name="reason" maxlength="500" ${['ban','delete'].includes(action) ? 'required' : ''}></textarea></label><label class="check-label"><input type="checkbox" required>我已核对所选记录</label>`,
      onSubmit: async ({value}) => {
        const data = await api(`/api/v1/admin/${entity === 'users' ? 'managed-users' : 'rooms'}/batch`, {method: 'POST', body: JSON.stringify({action, reason: value('reason'), confirm: entity === 'users' ? 'DELETE ACCOUNTS' : 'DELETE ROOMS', targets: chosen.map(item => ({[key]: item[key], version: item.version}))})});
        const failed = data.results.filter(item => !item.ok);
        const count = data.results.length - failed.length;
        if (failed.length) {
          setTimeout(() => formDialog({title: '批量处理结果', submit: '关闭', reload: false, success: '', content: `<p>成功 ${count} 项，失败 ${failed.length} 项。</p><div class="batch-results">${failed.map(item => `<p><code>${esc(item.id)}</code> · ${esc(item.error)}</p>`).join('')}</div>`, onSubmit: async () => {}}), 0);
        }
        return `批量处理完成：成功 ${count} 项，失败 ${failed.length} 项`;
      }
    });
  };
}
function bindFilterForm(view, filters) {
  bindForm(view, '.management-toolbar', async () => {
    filters.q = view.querySelector('[name="q"]').value.trim();
    filters.status = view.querySelector('[name="status"]').value;
    filters.offset = 0; await loadTab();
  });
}
function accountAction(user, action) {
  const labels = {ban:'封禁账号',unban:'解除封禁',delete:'账号移入回收站',restore:'恢复并启用账号'};
  formDialog({title: labels[action], submit: '确认操作', content: `<p>${esc(user.display_name)} · ${esc(user.email)}</p><p class="hint">${action === 'delete' ? '账号将无法登录，历史数据保留，可从回收站恢复。永久注销仍通过注销审核处理。' : '账号状态变更后，旧登录不会自动恢复，需要重新登录。'}</p><label>操作原因<textarea name="reason" maxlength="500" ${['ban','delete'].includes(action) ? 'required' : ''}></textarea></label>`,
    onSubmit: ({value}) => api(`/api/v1/admin/managed-users/${encodeURIComponent(user.id)}`, {method:'PUT',body:JSON.stringify({version:user.version,action,reason:value('reason'),confirm:'DELETE ACCOUNT'})})});
}
async function managedUsers(view) {
  const filters = filtersFor('users', {status: 'active'});
  const data = await api(`/api/v1/admin/managed-users?${queryFor(filters)}`);
  const items = data.users;
  view.innerHTML = `<section class="panel"><form class="management-toolbar"><div class="search-field">${icon('search')}<label for="userQuery">搜索用户</label><input name="q" id="userQuery" type="search" placeholder="搜索昵称、邮箱或用户 ID" value="${esc(filters.q)}"></div><select name="status" aria-label="账号状态">${options({'':'全部账号',active:'正常账号',banned:'已封禁',pending:'待审批',rejected:'未通过',deleted:'回收站'},filters.status)}</select><button type="submit" id="searchUsers" class="secondary">搜索</button>${can('user.manage') ? '<button type="button" id="addManagedUser">创建账号</button>' : ''}</form><div class="list-meta"><h2>用户与访问</h2><span class="badge">${number(data.total)} 位用户</span></div>${can('user.manage') ? batchBar({ban:'批量封禁',unban:'批量解封',delete:'移入回收站',restore:'恢复账号'}) : ''}<div class="record-list">${items.map((user,index) => `<article class="item"><div class="item-header">${can('user.manage') ? `<label class="record-select"><input type="checkbox" class="record-checkbox" data-index="${index}" ${!data.can_manage[user.id] ? 'disabled' : ''} aria-label="选择 ${esc(user.display_name)}"></label>` : ''}<div class="avatar" aria-hidden="true">${esc(Array.from(user.display_name || 'U')[0])}</div><div class="item-identity"><h3>${esc(user.display_name)}</h3><p>${esc(user.email)}</p></div><span class="badge">${esc(accountStatuses[user.status])}</span></div><div class="record-meta"><span>${esc(roles[user.admin_role] || '普通用户')}</span><span>${user.email_verified ? '邮箱已验证' : '邮箱待验证'}</span><span>会员到期 ${dateTime(user.vip_expires_at)}</span><code>${esc(user.id)}</code></div>${user.reason ? `<p>状态原因 · ${esc(user.reason)}</p>` : ''}<div class="item-actions">${can('user.manage') ? `<button class="secondary registration-user" data-index="${index}">注册环境</button>` : ''}${data.can_manage[user.id] ? (user.status === 'deleted' ? `<button class="secondary user-action" data-index="${index}" data-action="restore">恢复账号</button>` : `<button class="secondary edit-user" data-index="${index}">修改昵称</button><button class="secondary password-user" data-index="${index}">重置密码</button>${can('role.manage') ? `<button class="secondary role-user" data-index="${index}">调整角色</button>` : ''}<button class="secondary user-action" data-index="${index}" data-action="${user.status === 'banned' ? 'unban' : 'ban'}">${user.status === 'banned' ? '解除封禁' : '封禁账号'}</button><button class="danger user-action" data-index="${index}" data-action="delete">移入回收站</button>`) : ''}${can('user.vip.manage') && user.status !== 'deleted' ? `<button class="secondary vip-user" data-index="${index}">增加会员</button>` : ''}</div></article>`).join('') || empty('没有匹配的用户','调整搜索内容或账号状态筛选。','users')}</div>${pagination(data)}</section>`;
  bindFilterForm(view, filters); bindPagination(view, filters, data); bindBatch(view, items, 'id', 'users');
  view.querySelector('#addManagedUser')?.addEventListener('click', () => formDialog({title:'创建用户账号',submit:'创建账号',content:`${textField('email','邮箱地址','','email','required maxlength="254" autocomplete="off"')}${textField('display_name','用户昵称','','text','required minlength="2" maxlength="32"')}${textField('password','初始密码','','password','required minlength="10" maxlength="128" autocomplete="new-password"')}<label class="check-label"><input name="verified" type="checkbox">邮箱已人工核验</label><p class="hint">新建账号默认为普通用户。密码请通过安全方式交给账号使用者。</p>`,onSubmit:({value,checked})=>api('/api/v1/admin/managed-users',{method:'POST',body:JSON.stringify({email:value('email'),display_name:value('display_name'),password:value('password'),email_verified:checked('verified')})})}));
  bindActions(view,'.user-action',button=>accountAction(items[button.dataset.index],button.dataset.action));
  bindActions(view,'.registration-user',button=>showRegistration(items[button.dataset.index]));
  bindActions(view,'.edit-user',button=>{const user=items[button.dataset.index];formDialog({title:'修改昵称',content:textField('name','用户昵称',user.display_name,'text','required minlength="2" maxlength="32"'),onSubmit:({value})=>api(`/api/v1/admin/managed-users/${encodeURIComponent(user.id)}`,{method:'PUT',body:JSON.stringify({action:'profile',version:user.version,display_name:value('name')})})});});
  bindActions(view,'.password-user',button=>{const user=items[button.dataset.index];formDialog({title:'重置登录密码',content:`<p class="hint">${esc(user.email)} 的现有登录将失效。</p>${textField('password','新密码','','password','required minlength="10" maxlength="128" autocomplete="new-password"')}`,onSubmit:({value})=>api(`/api/v1/admin/managed-users/${encodeURIComponent(user.id)}`,{method:'PUT',body:JSON.stringify({action:'password',version:user.version,password:value('password')})})});});
  bindActions(view,'.role-user',button=>{const user=items[button.dataset.index];formDialog({title:'调整管理角色',content:`<p class="hint">${esc(user.email)}</p><label>账号角色<select name="role">${options(roles,user.admin_role || '')}</select></label>`,onSubmit:({value})=>api(`/api/v1/admin/managed-users/${encodeURIComponent(user.id)}`,{method:'PUT',body:JSON.stringify({action:'role',version:user.version,role:value('role')})})});});
  bindActions(view,'.vip-user',button=>{const user=items[button.dataset.index];formDialog({title:'增加会员时长',content:`<p class="hint">${esc(user.email)}</p>${textField('days','增加天数',30,'number','required min="1" max="36500" step="1"')}`,onSubmit:({value})=>api(`/api/v1/admin/users/${encodeURIComponent(user.id)}/vip`,{method:'PUT',body:JSON.stringify({days:Number(value('days'))})})});});
}
async function sitePolicy(view) {
  const data=await api('/api/v1/admin/site-policy'), p=data.policy;
  view.innerHTML=`<section class="panel form-panel">${panelHeader('站点访问策略','设置会作用于实际注册、建房、访客访问和聊天请求。','config')}<form id="policyForm"><div class="policy-grid">${toggleField('registration_enabled','开放注册',p.registration_enabled,'关闭后仍可在后台创建账号。')}${toggleField('registration_requires_email_code','注册邮箱验证',p.registration_requires_email_code,'开启时注册需验证码，关闭时前台隐藏。请先配置可用的邮件服务。')}${toggleField('registration_requires_invite','注册需要邀请码',p.registration_requires_invite,'邀请码在“注册邀请码”中批量生成。')}${toggleField('registration_requires_review','注册需审批',p.registration_requires_review,'通过审批后，用户才能登录。')}${toggleField('room_creation_requires_review','建房需审批',p.room_creation_requires_review,'普通用户提交后，由管理人员审核。')}${toggleField('room_creation_enabled','允许用户建房',p.room_creation_enabled,'关闭后管理人员仍可创建房间。')}${toggleField('guests_enabled','开放访客访问',p.guests_enabled,'关闭后，现有访客连接也会退出。')}${toggleField('chat_enabled','允许房间聊天',p.chat_enabled,'控制新的房间聊天消息。')}</div><div class="policy-numbers">${textField('registration_ip_limit','每个 IP 每 24 小时注册额度',p.registration_ip_limit,'number','required min="0" max="1000" step="1"')}${textField('registration_device_limit','每个设备 / 浏览器每 24 小时注册额度',p.registration_device_limit,'number','required min="0" max="1000" step="1"')}${textField('max_rooms_per_user','每人最多开放房间',p.max_rooms_per_user,'number','required min="0" max="1000" step="1"')}${textField('default_max_members','新房间默认人数',p.default_max_members,'number','required min="2" max="100" step="1"')}${textField('max_room_members','房间人数上限',p.max_room_members,'number','required min="2" max="100" step="1"')}${textField('chat_history_limit','每次最多读取聊天记录',p.chat_history_limit,'number','required min="1" max="100" step="1"')}</div><p class="hint">注册额度按首次成功注册起算的 24 小时周期累计，设为 0 表示不限制；IPv6 同一 /64 网段共用 IP 额度。设备与浏览器分别计数，管理员创建账号不占额度。每人房间数设为 0 表示不限制。降低人数上限不自动移除现有成员。</p><div class="form-actions"><button type="submit" id="savePolicy">确认并更新策略</button><span class="muted">版本 ${data.version} · 修改会写入操作审计</span></div></form></section>`;
  bindForm(view,'#policyForm',async()=>{
    if(!confirm('站点策略会立即影响用户操作，确认更新？'))return;
    const policy={}; for(const key of ['registration_enabled','registration_requires_email_code','registration_requires_invite','registration_requires_review','room_creation_requires_review','room_creation_enabled','guests_enabled','chat_enabled'])policy[key]=view.querySelector(`[name="${key}"]`).checked;
    for(const key of ['registration_ip_limit','registration_device_limit','max_rooms_per_user','default_max_members','max_room_members','chat_history_limit'])policy[key]=Number(view.querySelector(`[name="${key}"]`).value);
    await mutate('/api/v1/admin/site-policy','PUT',{version:data.version,policy},true,{'X-Confirm-Dangerous':'update-site-policy'});
  });
}
async function taxonomyPage(view) {
  const data=await api('/api/v1/admin/taxonomy');const entries=data.entries || [];
  const categories=Object.fromEntries(entries.filter(item=>item.kind==='category').map(item=>[item.key,item.name]));
  view.innerHTML=`<section class="panel">${panelHeader('分类与标签','为房间设置提供统一选项。停用分类后，其关联标签也会停止公开展示。','layers')}<div class="item-actions"><button type="button" id="addCategory">添加分类</button><button type="button" id="addTag" class="secondary">添加标签</button></div></section>${['category','tag'].map(kind=>`<section class="panel"><div class="list-meta"><h2>${kind==='category'?'房间分类':'房间标签'}</h2><span class="badge">${entries.filter(item=>item.kind===kind).length} 项</span></div><div class="record-list">${entries.map((item,index)=>item.kind!==kind?'':`<article class="item"><div class="item-header"><h3>${esc(item.name)}</h3><span class="badge">${item.enabled?'已启用':'已停用'}</span></div><div class="record-meta"><code>${esc(item.key)}</code><span>排序 ${item.sort_order}</span>${item.category?`<span>归属 ${esc(categories[item.category] || item.category)}</span>`:''}</div><p>${esc(item.description)}</p><div class="item-actions"><button class="secondary edit-taxonomy" data-index="${index}" type="button">编辑</button><button class="secondary toggle-taxonomy" data-index="${index}" type="button">${item.enabled?'停用':'启用'}</button><button class="danger delete-taxonomy" data-index="${index}" type="button">删除</button></div></article>`).join('')||empty('暂无条目','添加分类或标签后，可在房间设置中选用。','layers')}</div></section>`).join('')}`;
  async function save(next){await api('/api/v1/admin/taxonomy',{method:'PUT',body:JSON.stringify({version:data.version,entries:next})});}
  function edit(kind,index){const item=index==null?{key:'',kind,name:'',description:'',category:'',sort_order:0,enabled:true}:entries[index];formDialog({title:index==null?'添加条目':'编辑条目',content:`${textField('key','标识（房间保存的值）',item.key,'text',`required maxlength="30" ${index==null?'':'readonly'}`)}${textField('name','显示名称',item.name,'text','required maxlength="40"')}${kind==='tag'?`<label>所属分类<select name="category">${options({'':'不指定分类',...categories},item.category)}</select></label>`:''}${textField('sort','排序值',item.sort_order,'number','required min="0" max="10000" step="1"')}<label>说明<textarea name="description" maxlength="300">${esc(item.description)}</textarea></label><label class="check-label"><input type="checkbox" name="enabled" ${item.enabled?'checked':''}>启用此条目</label>`,onSubmit:({value,checked})=>{const next=entries.map(entry=>({...entry})),updated={...item,key:value('key').trim(),name:value('name').trim(),description:value('description'),category:value('category'),sort_order:Number(value('sort')),enabled:checked('enabled')};if(index==null)next.push(updated);else next[index]=updated;return save(next);}});}
  view.querySelector('#addCategory').onclick=()=>edit('category');view.querySelector('#addTag').onclick=()=>edit('tag');
  bindActions(view,'.edit-taxonomy',button=>{const index=Number(button.dataset.index);edit(entries[index].kind,index);});
  bindActions(view,'.toggle-taxonomy',async button=>{const next=entries.map((item,index)=>index===Number(button.dataset.index)?{...item,enabled:!item.enabled}:item);await save(next);await loadTab();notify('分类与标签已更新');});
  bindActions(view,'.delete-taxonomy',button=>{const index=Number(button.dataset.index),item=entries[index];formDialog({title:'删除分类或标签',submit:'确认删除',content:`<p>${esc(item.name)}</p><p class="hint">${item.kind==='category'?'关联的标签字典也会移除。':''}已有房间中保存的分类、标签文本会保留。</p><label class="check-label"><input type="checkbox" required>确认删除此条目</label>`,onSubmit:()=>save(entries.filter((entry,i)=>i!==index&&!(item.kind==='category'&&entry.category===item.key)))});});
}
async function mediaServices(view) {
  const filters=filtersFor('sources',{status:''});const data=await api(`/api/v1/admin/media-sources?${queryFor(filters)}`);const items=data.sources;
  view.innerHTML=`<div class="status-strip"><span class="badge">媒体服务${data.media_service_ready?'已配置':'未配置'}</span><span class="badge">平台解析器${data.platform_parser_ready?'可用':'未就绪'}</span></div><section class="panel"><form class="management-toolbar"><div class="search-field">${icon('search')}<label for="sourceQuery">搜索媒体来源</label><input id="sourceQuery" name="q" type="search" value="${esc(filters.q)}" placeholder="搜索名称、平台或用户邮箱"></div><select name="status" aria-label="媒体来源状态">${options({'':'全部来源',enabled:'已启用',disabled:'已停用'},filters.status)}</select><button type="submit" class="secondary">搜索</button></form><div class="list-meta"><h2>全站媒体来源</h2><span class="badge">${data.total} 项</span></div><p class="hint">用户在客户端绑定的媒体来源会显示在这里。账号密钥不会显示在后台。</p><div class="record-list">${items.map((item,index)=>`<article class="item"><div class="item-header"><h3>${esc(item.name)}</h3><span class="badge">${item.disabled?'已停用':'已启用'}</span></div><div class="record-meta"><span>${esc(item.type)}</span><span>${esc(item.owner_name)} · ${esc(item.owner_email)}</span><code>${esc(item.id)}</code></div><p>${esc(item.base_url)}</p><div class="item-actions"><button class="secondary source-toggle" type="button" data-index="${index}">${item.disabled?'启用':'停用'}</button><button class="secondary source-probe" type="button" data-index="${index}" ${item.disabled||!data.media_service_ready?'disabled':''}>检测连接</button><button class="secondary source-rename" type="button" data-index="${index}">重命名</button><button class="danger source-delete" type="button" data-index="${index}">删除</button></div></article>`).join('')||empty('暂无媒体来源','用户绑定网盘、媒体库或视频平台后，可在此管理。','layers')}</div>${pagination(data)}</section>`;
  bindFilterForm(view,filters);bindPagination(view,filters,data);
  const path=item=>`/api/v1/admin/media-sources/${encodeURIComponent(item.id)}`;
  bindActions(view,'.source-toggle',button=>{const item=items[button.dataset.index];formDialog({title:item.disabled?'启用媒体来源':'停用媒体来源',content:`<p>${esc(item.name)}</p><p class="hint">${item.disabled?'启用后，用户可重新访问此来源。':'停用后，新目录请求、播放授权和代理请求会被拒绝。'}</p>`,onSubmit:()=>api(path(item),{method:'PUT',body:JSON.stringify({version:item.admin_version,action:item.disabled?'enable':'disable'})})});});
  bindActions(view,'.source-rename',button=>{const item=items[button.dataset.index];formDialog({title:'重命名媒体来源',content:textField('name','来源名称',item.name,'text','required maxlength="64"'),onSubmit:({value})=>api(path(item),{method:'PUT',body:JSON.stringify({version:item.admin_version,action:'rename',name:value('name')})})});});
  bindActions(view,'.source-delete',button=>{const item=items[button.dataset.index];formDialog({title:'删除媒体来源',submit:'确认删除',content:`<p>${esc(item.name)}</p><p class="hint">删除后需要重新绑定，关联房间的播放可能中断。</p><label class="check-label"><input type="checkbox" required>确认删除此媒体来源</label>`,onSubmit:()=>api(path(item),{method:'PUT',body:JSON.stringify({version:item.admin_version,action:'delete',confirm:'DELETE SOURCE'})})});});
  bindActions(view,'.source-probe',button=>{const item=items[button.dataset.index],platform=!['webdav','emby','quark','synology','qnap','fnos','nextcloud','seafile','truenas'].includes(item.type);formDialog({title:'检测媒体连接',submit:'开始检测',reload:false,content:`<p class="hint">${platform?'使用该来源解析一个视频链接。':'读取来源根目录，验证当前网络与凭据。'}检测最多等待 15 秒。</p>${platform?textField('url','该平台的视频链接','','url','required'):''}`,onSubmit:async({value})=>{const result=await api(path(item)+'/probe',{method:'POST',body:JSON.stringify({url:value('url')})});if(!result.success)throw new Error(result.message);return `连接成功 · ${result.elapsed_ms} ms · 返回 ${result.items} 项`;}});});
}
pages.users = managedUsers;
pages.policy = sitePolicy;
pages.taxonomy = taxonomyPage;
pages.sources = mediaServices;
