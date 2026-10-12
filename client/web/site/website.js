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
    const dark = document.documentElement.dataset.theme === 'dark';
    document.querySelectorAll('[data-theme-toggle]').forEach(button => {
      const label = dark ? '切换为浅色' : '切换为深色';
      button.setAttribute('aria-label', label);
      button.title = label;
      button.querySelector('use').setAttribute('href', dark ? '#i-moon' : '#i-sun');
    });
  }
  document.querySelectorAll('[data-theme-toggle]').forEach(button => {
    button.addEventListener('click', () => window.sameframeAppearance.setMode(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'));
  });
  window.addEventListener('sameframe-theme-change', renderTheme);
  renderTheme();
})();
