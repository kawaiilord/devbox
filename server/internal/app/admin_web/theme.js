(() => {
  const key = 'sf_admin_theme';
  let mode = 'light';
  try { if (localStorage.getItem(key) === 'dark') mode = 'dark'; } catch (_) {}
  const paths = {
    light: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>',
    dark: '<path d="M21 13A9 9 0 0 1 11 3a9 9 0 1 0 10 10Z"/>'
  };
  function apply() {
    document.documentElement.dataset.theme = mode;
    document.querySelectorAll('[data-theme-toggle]').forEach(button => {
      const label = mode === 'dark' ? '切换为浅色' : '切换为深色';
      button.setAttribute('aria-label', label); button.title = label;
      button.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + paths[mode] + '</svg>';
    });
  }
  window.adminTheme = {
    apply,
    set(value) {
      mode = value === 'dark' ? 'dark' : 'light';
      try { localStorage.setItem(key, mode); } catch (_) {}
      apply();
    }
  };
  document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-theme-toggle]').forEach(button => button.addEventListener('click', () => window.adminTheme.set(mode === 'dark' ? 'light' : 'dark')));
    apply();
  });
  window.addEventListener('storage', event => { if (event.key === key) { mode = event.newValue === 'dark' ? 'dark' : 'light'; apply(); } });
  apply();
})();
