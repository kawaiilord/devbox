'use strict';
(() => {
  // Existing installed shortcuts may still start at '/'. Keep them in the app.
  if (matchMedia('(display-mode: standalone)').matches || navigator.standalone) {
    location.replace(new URL('app.html', location.href));
    return;
  }
  const menu = document.getElementById('mobileMenu');
  const toggle = document.getElementById('menuToggle');
  function closeMenu() {
    menu.hidden = true;
    toggle.setAttribute('aria-expanded', 'false');
    toggle.setAttribute('aria-label', '打开导航');
  }
  toggle.addEventListener('click', () => {
    const open = menu.hidden;
    menu.hidden = !open;
    toggle.setAttribute('aria-expanded', String(open));
    toggle.setAttribute('aria-label', open ? '关闭导航' : '打开导航');
  });
  menu.querySelectorAll('a').forEach(link => link.addEventListener('click', closeMenu));
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && !menu.hidden) { closeMenu(); toggle.focus(); }
  });
  function renderTheme() {
    const current = window.sameframeAppearance.getMode() || 'light';
    document.querySelectorAll('[data-theme-choice]').forEach(button => {
      button.setAttribute('aria-pressed', String(button.dataset.themeChoice === current));
    });
  }
  document.querySelectorAll('[data-theme-choice]').forEach(button => {
    button.addEventListener('click', () => window.sameframeAppearance.setMode(button.dataset.themeChoice));
  });
  window.addEventListener('sameframe-theme-change', renderTheme);
  renderTheme();
})();
