(() => {
  'use strict';
  let authorizationWindow = null;
  let passkeyAbort = null;
  const decode = value => Uint8Array.from(atob(value.replace(/-/g, '+').replace(/_/g, '/')), c => c.charCodeAt(0));
  const encode = value => btoa(String.fromCharCode(...new Uint8Array(value))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  window.sameframeSecurity = {
    get supported() { return window.isSecureContext && !!window.PublicKeyCredential && !!navigator.credentials; },
    async createPasskey(raw) {
      if (!window.sameframeSecurity.supported) throw new Error('此浏览器暂不支持 Passkey');
      window.sameframeSecurity.cancelPasskey();
      const controller = new AbortController();
      passkeyAbort = controller;
      try {
        const payload = JSON.parse(raw);
        const options = payload.publicKey;
        options.challenge = decode(options.challenge);
        options.user.id = decode(options.user.id);
        options.excludeCredentials = (options.excludeCredentials || []).map(c => ({...c, id: decode(c.id)}));
        const credential = await navigator.credentials.create({publicKey: options, signal: controller.signal});
        if (!credential) throw new Error('设备验证未完成');
        return JSON.stringify({id: credential.id, rawId: encode(credential.rawId), type: credential.type,
          response: {clientDataJSON: encode(credential.response.clientDataJSON), attestationObject: encode(credential.response.attestationObject), transports: credential.response.getTransports?.() || []},
          clientExtensionResults: credential.getClientExtensionResults(), authenticatorAttachment: credential.authenticatorAttachment});
      } finally { if (passkeyAbort === controller) passkeyAbort = null; }
    },
    cancelPasskey() { passkeyAbort?.abort(); passkeyAbort = null; },
    beginAuthorization() {
      window.sameframeSecurity.closeAuthorization();
      const popup = window.open('about:blank', '_blank', 'popup,width=620,height=740');
      if (!popup) throw new Error('请允许本站打开授权窗口后重试');
      // The new window has its own sessionStorage, initially copied from the opener.
      // Keep only a UI return marker; credentials remain in the application.
      popup.sessionStorage.clear();
      popup.sessionStorage.setItem('sf_in_app_link', '1');
      popup.opener = null;
      popup.document.title = '同帧 · 账号授权';
      popup.document.body.textContent = '正在连接授权平台…';
      authorizationWindow = popup;
    },
    navigateAuthorization(raw) {
      const target = new URL(raw);
      if (target.protocol !== 'https:' && !(target.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(target.hostname))) throw new Error('授权地址无效');
      if (!authorizationWindow || authorizationWindow.closed) throw new Error('授权窗口已关闭，请重试');
      authorizationWindow.location.href = target.href;
    },
    closeAuthorization() { authorizationWindow?.close(); authorizationWindow = null; },
  };
})();
