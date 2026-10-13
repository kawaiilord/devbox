'use strict';
(() => {
  const key = 'sameframe.browser.device';
  const valid = value => typeof value === 'string' && /^[a-zA-Z0-9_-]{16,128}$/.test(value);
  window.sameframeAccountDevice = {
    id(preferred = '') {
      let legacy = '';
      try { legacy = JSON.parse(localStorage.getItem('flutter.device.id') || 'null'); } catch (_) {}
      const saved = localStorage.getItem(key);
      const id = valid(preferred) ? preferred : valid(legacy) ? legacy : valid(saved) ? saved : crypto.randomUUID();
      localStorage.setItem(key, id);
      return id;
    },
  };
})();
