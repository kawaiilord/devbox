'use strict';
(() => {
  const byId = id => document.getElementById(id);
  const mode = document.body.dataset.accountMode;
  const registering = mode === 'register', resetting = mode === 'forgot-password';
  const form = byId('accountForm'), submit = byId('submitAccount');
  const initialLabel = submit.querySelector('span').textContent;
  const state = {busy:false, sending:false, rules:null, challenge:'', codeEmail:'', codeUntil:0, mfa:null};
  const paths = {
    light:'<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>',
    dark:'<path d="M21 13A9 9 0 0 1 11 3a9 9 0 1 0 10 10Z"/>',
  };
  function theme() {
    const dark=document.documentElement.dataset.theme==='dark',button=byId('themeToggle');
    button.setAttribute('aria-label',dark?'切换为浅色':'切换为深色');button.title=button.getAttribute('aria-label');
    button.querySelector('svg').innerHTML=paths[dark?'dark':'light'];
  }
  byId('themeToggle').addEventListener('click',()=>window.sameframeAppearance.setMode(document.documentElement.dataset.theme==='dark'?'light':'dark'));
  window.addEventListener('sameframe-theme-change',theme);theme();
  byId('passwordToggle')?.addEventListener('click',()=>{
    const input=byId('password'),show=input.type==='password';input.type=show?'text':'password';
    byId('passwordToggle').setAttribute('aria-label',show?'隐藏密码':'显示密码');
    byId('passwordToggle').setAttribute('aria-pressed',String(show));
  });
  function message(text,error=false,id='formMessage') {
    const element=byId(id);element.textContent=text;element.hidden=!text;
    element.classList.toggle('error',error);
  }
  function displayError(error,id='formMessage') {
    const translations={'invalid credentials':'邮箱或密码不正确，请重新输入。','unauthorized':'登录状态已失效，请重新登录。','invalid email or password':'邮箱或密码不正确，请重新输入。'};
    message(translations[error.message]||error.message||'请求未完成，请稍后重试。',true,id);
  }
  function updateButtons() {
    submit.disabled=state.busy || (registering && (!state.rules || !state.rules.registration_enabled));
    submit.querySelector('span').textContent=state.busy?(resetting?'正在提交…':registering?'正在创建账号…':'正在登录…'):initialLabel;
    const codeButton=byId('sendCode');
    if (codeButton) {
      const remaining=Math.max(0,Math.ceil((state.codeUntil-Date.now())/1000));
      codeButton.disabled=state.busy||state.sending||remaining>0||!state.rules?.registration_email_code_required;
      codeButton.textContent=state.sending?'发送中…':remaining?`${remaining}s 后重发`:'获取验证码';
    }
  }
  async function api(path,body,session) {
    let device;
    try { device=window.sameframeAccountDevice.id(); } catch (_) { throw new Error('请允许本站使用浏览器存储后重试。'); }
    const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),30000);
    try {
      const response=await fetch(path,{method:body===undefined?'GET':'POST',credentials:'same-origin',cache:'no-store',signal:controller.signal,
        headers:{'Content-Type':'application/json','X-Device-ID':device,'X-Device-Name':'Web browser','X-Device-Platform':'web',...(registering ? globalThis.sameframeRegistrationContext?.() || {} : {}),...(session?{Authorization:`Bearer ${session.access_token}`}:{})},
        ...(body===undefined?{}:{body:JSON.stringify(body)})});
      const data=await response.json();
      if (response.status===202 && data.data) return {...data.data,_message:data.msg};
      if (!response.ok||data.code!==0) { const error=new Error(data.msg||'请求失败，请稍后重试。');error.status=response.status;error.data=data.data||{};throw error; }
      return data.data||{};
    } catch(error) {
      if(error.name==='AbortError')throw new Error('连接超时，请检查网络后重试。');
      if(error instanceof TypeError)throw new Error('暂时无法连接，请检查网络后重试。');
      throw error;
    } finally { clearTimeout(timeout); }
  }
  function conditionalField(id,visible) {
    const field=byId(id),input=field.querySelector('input');field.hidden=!visible;input.disabled=!visible;input.required=visible;
  }
  let rulesPromise=null;
  async function loadRules(preserveMessage=false) {
    if (!registering || rulesPromise) return rulesPromise;
    rulesPromise=(async()=>{
      try {
        const rules=await api('/api/v1/auth/options');
        state.rules=rules;
        conditionalField('inviteField',!!rules.registration_invite_required);
        conditionalField('codeField',!!rules.registration_email_code_required);
        if (!rules.registration_email_code_required) { state.challenge='';state.codeEmail='';state.codeUntil=0;byId('emailCode').value=''; }
        byId('retryRules').hidden=true;
        byId('registrationStatus').hidden=!!rules.registration_enabled;
        byId('rulesText').textContent=rules.registration_enabled?'':'站点当前暂停注册，请联系管理员。';
        if(!preserveMessage)message('');
      } catch(error) {
        state.rules=null;conditionalField('inviteField',false);conditionalField('codeField',false);
        byId('registrationStatus').hidden=false;byId('retryRules').hidden=false;
        byId('rulesText').textContent='暂时无法获取注册设置。';
      } finally { updateButtons();rulesPromise=null; }
    })();
    return rulesPromise;
  }
  byId('retryRules').addEventListener('click',()=>loadRules());
  byId('email').addEventListener('input',()=>{
    if (byId('email').value.trim().toLowerCase()!==state.codeEmail) {
      state.challenge='';if(byId('emailCode'))byId('emailCode').value='';
      if(byId('codeHint'))byId('codeHint').textContent='';
    }
  });
  byId('sendCode')?.addEventListener('click',async()=>{
    if (state.sending||state.busy||Date.now()<state.codeUntil) return;
    const email=byId('email');if(!email.reportValidity())return;
    state.sending=true;message('');updateButtons();
    try {
      const normalized=email.value.trim().toLowerCase();
      const result=await api('/api/v1/auth/register/code',{email:normalized});
      state.challenge=result.challenge;state.codeEmail=normalized;
      state.codeUntil=Date.now()+(result.retry_after||60)*1000;
      byId('codeHint').textContent='验证码已发送，10 分钟内有效。';byId('emailCode').focus();
    } catch(error) { displayError(error);await loadRules(true); }
    finally { state.sending=false;updateButtons(); }
  });
  async function enterWorkspace(session) {
    if (!session.access_token || !session.refresh_token) throw new Error('登录未完成，请重新尝试。');
    await api('/api/v1/auth/web-session',{refresh_token:session.refresh_token},session);
    location.replace('/app');
  }
  async function finish(result) {
    if(result.pending_approval) {
      message(result._message||'注册申请已提交，请等待管理员审核后登录。');
      byId('password').value='';form.querySelectorAll('input,button').forEach(el=>el.disabled=true);
      submit.hidden=true;return;
    }
    if(result.mfa_required) {
      state.mfa=result.challenge;form.hidden=true;byId('mfaForm').hidden=false;
      byId('accountTitle').textContent='验证你的身份';byId('accountIntro').textContent='输入身份验证器的动态验证码或恢复码。';
      byId('mfaCode').focus();return;
    }
    await enterWorkspace(result);
  }
  form.addEventListener('submit',async event=>{
    event.preventDefault();if(state.busy||!form.reportValidity())return;
    if(registering&&(!state.rules||!state.rules.registration_enabled))return;
    state.busy=true;message('');updateButtons();
    try {
      const email=byId('email').value.trim().toLowerCase();
      if(resetting) {
        await api('/api/v1/auth/password/request',{email});
        message('如果该邮箱已注册且邮件服务可用，请查看密码重置邮件。');
      } else if(registering) {
        if(state.rules.registration_email_code_required&&(!state.challenge||state.codeEmail!==email))throw new Error('请先获取此邮箱的验证码。');
        await finish(await api('/api/v1/auth/register',{email,display_name:byId('displayName').value.trim(),password:byId('password').value,
          invite_code:state.rules.registration_invite_required?byId('inviteCode').value.trim():'',
          challenge:state.rules.registration_email_code_required?state.challenge:'',code:state.rules.registration_email_code_required?byId('emailCode').value.trim():''}));
      } else {
        await finish(await api('/api/v1/auth/login',{email,password:byId('password').value}));
      }
    } catch(error) { displayError(error);if(registering)await loadRules(true); }
    finally { state.busy=false;updateButtons(); }
  });
  byId('mfaForm').addEventListener('submit',async event=>{
    event.preventDefault();if(state.busy||!event.currentTarget.reportValidity())return;
    state.busy=true;byId('submitMfa').disabled=true;message('',false,'mfaMessage');
    try { await finish(await api('/api/v1/auth/mfa',{challenge:state.mfa,code:byId('mfaCode').value.trim()})); }
    catch(error) { displayError(error,'mfaMessage'); }
    finally { state.busy=false;byId('submitMfa').disabled=false; }
  });
  byId('cancelMfa').addEventListener('click',()=>{ location.replace('/login'); });
  if(registering) { loadRules();setInterval(updateButtons,1000);window.addEventListener('focus',()=>{if(!state.busy)loadRules(true);}); }
})();
