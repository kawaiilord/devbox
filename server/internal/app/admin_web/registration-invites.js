'use strict';

pages.registration_invites = async function registrationInvites(view) {
  let settings = await api('/api/v1/admin/site-policy');
  let issued = [], issuedBatch = '';
  const filters = filtersFor('registration_invites');
  const statuses = {active:'可使用',used:'已使用',expired:'已过期',revoked:'已撤销'};
  view.innerHTML = `<section class="panel form-panel">${panelHeader('注册访问','为新用户设置注册门槛。已有账号登录不受影响。','shield')}
    <form id="invitePolicyForm">${toggleField('registration_requires_invite','注册必须填写邀请码',settings.policy.registration_requires_invite,'开启后需要有效邀请码；关闭后注册页不显示邀请码输入框。')}
    <div class="form-actions"><button type="submit">保存邀请码设置</button><span class="muted">每个邀请码仅可注册一个账号</span></div></form></section>
    <section class="panel form-panel">${panelHeader('批量创建邀请码','生成后可复制或下载。完整邀请码仅在生成时显示一次。','codes')}
    <form id="registrationInviteForm" class="form-grid">${textField('label','批次备注','','text','maxlength="80" placeholder="例如：首批体验用户"')}${textField('count','生成数量',10,'number','required min="1" max="100" step="1"')}${textField('expires_days','有效天数',30,'number','required min="0" max="365" step="1"')}
    <p class="hint">每批最多 100 个；有效天数设为 0 表示长期有效。邀请码用于注册，不兑换会员。</p><button type="submit">批量创建邀请码</button></form>
    <div id="issuedInvitePanel" hidden><p class="notice" role="status">请及时复制或下载；离开本页面后无法再次查看完整邀请码。</p><pre id="issuedInviteCodes" tabindex="0" class="invite-code-output"></pre><div class="item-actions"><button type="button" class="secondary" id="copyInviteCodes">复制本批次</button><button type="button" class="secondary" id="downloadInviteCodes">下载 TXT</button></div></div></section>
    <section class="panel"><form id="inviteFilter" class="management-toolbar"><label>搜索批次<input name="q" type="search" value="${esc(filters.q)}" placeholder="批次备注、批次 ID 或尾号"></label><label>状态<select name="status">${options({'':'全部状态',...statuses},filters.status||'')}</select></label><button type="submit" class="secondary">筛选</button></form><div id="registrationInviteList" aria-live="polite"></div></section>`;

  async function list() {
    const target = view.querySelector('#registrationInviteList');
    try {
      const data = await api('/api/v1/admin/registration-invites?' + queryFor(filters));
      if (!view.contains(target)) return;
      target.innerHTML = `<div class="list-meta"><h2>邀请码记录</h2><span class="badge">${number(data.total)} 个</span></div><div class="record-list">${data.codes.map((code,i)=>`<article class="item"><div class="item-header"><h3>${esc(code.hint)}</h3><span class="badge">${esc(statuses[code.status])}</span></div><p>${esc(code.label || '未填写批次备注')}</p><div class="record-meta"><span>创建 ${dateTime(code.created_at)}</span><span>到期 ${code.expires_at ? dateTime(code.expires_at) : '长期有效'}</span><span>批次 <code>${esc(code.batch_id)}</code></span></div>${code.used_at ? `<p class="muted">使用时间 ${dateTime(code.used_at)} · 用户 ${esc(code.used_by || '已删除账号')}</p>` : ''}${code.status==='active' ? `<button type="button" class="secondary revoke-registration-invite" data-index="${i}">撤销此邀请码</button>` : ''}</article>`).join('') || empty('暂无邀请码','创建一批邀请码后，可在这里查看状态和使用记录。','codes')}</div>${pagination(data)}`;
      target.querySelector('.previous-page')?.addEventListener('click',()=>{filters.offset=Math.max(0,data.offset-data.limit);list();});
      target.querySelector('.next-page')?.addEventListener('click',()=>{filters.offset=data.offset+data.limit;list();});
      target.querySelectorAll('.revoke-registration-invite').forEach(button=>button.addEventListener('click',()=>{
        const code=data.codes[Number(button.dataset.index)];
        formDialog({title:'撤销邀请码',content:`<p>撤销 ${esc(code.hint)} 后，将不能再用于注册。</p>`,submit:'确认撤销',reload:false,onSubmit:async()=>{await api(`/api/v1/admin/registration-invites/${encodeURIComponent(code.id)}/revoke`,{method:'POST'});await list();}});
      }));
    } catch (error) { target.textContent=error.message;notify(error.message,true); }
  }
  bindForm(view,'#invitePolicyForm',async()=>{
    const toggle=view.querySelector('[name="registration_requires_invite"]');
    try {
      settings=await api('/api/v1/admin/site-policy',{method:'PUT',headers:{'X-Confirm-Dangerous':'update-site-policy'},body:JSON.stringify({version:settings.version,policy:{...settings.policy,registration_requires_invite:toggle.checked}})});
      notify(settings.policy.registration_requires_invite?'已开启邀请码注册':'已关闭邀请码要求');
    } catch(error) { toggle.checked=settings.policy.registration_requires_invite;throw error; }
  });
  bindForm(view,'#registrationInviteForm',async()=>{
    const form=new FormData(view.querySelector('#registrationInviteForm'));
    const data=await api('/api/v1/admin/registration-invites',{method:'POST',body:JSON.stringify({count:Number(form.get('count')),expires_days:Number(form.get('expires_days')),label:String(form.get('label')||'')})});
    issued=data.codes.map(item=>item.code);issuedBatch=data.batch_id;
    view.querySelector('#issuedInviteCodes').textContent=issued.join('\n');
    view.querySelector('#issuedInvitePanel').hidden=false;
    notify(`已生成 ${issued.length} 个邀请码，请及时保存`);filters.offset=0;await list();
  });
  view.querySelector('#copyInviteCodes').onclick=async()=>{
    try { await navigator.clipboard.writeText(issued.join('\n'));notify('邀请码已复制'); }
    catch (_) { const range=document.createRange();range.selectNodeContents(view.querySelector('#issuedInviteCodes'));const selection=getSelection();selection.removeAllRanges();selection.addRange(range);notify('已选中邀请码，请使用复制快捷键'); }
  };
  view.querySelector('#downloadInviteCodes').onclick=()=>{
    const blob=new Blob([issued.join('\n')+'\n'],{type:'text/plain;charset=utf-8'}),url=URL.createObjectURL(blob),link=document.createElement('a');
    link.href=url;link.download=`sameframe-invites-${issuedBatch}.txt`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
  };
  view.querySelector('#inviteFilter').onsubmit=event=>{event.preventDefault();const data=new FormData(event.currentTarget);filters.q=String(data.get('q')||'');filters.status=String(data.get('status')||'');filters.offset=0;list();};
  await list();
};
