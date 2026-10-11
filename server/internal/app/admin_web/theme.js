(() => {
  const key = 'sf_admin_theme';
  const choices = ['light', 'dark', 'system'];
  const media = window.matchMedia('(prefers-color-scheme: dark)');
  let preference = 'light';
  try {
    const stored = localStorage.getItem(key);
    if (choices.includes(stored)) preference = stored;
  } catch (_) { /* The theme still works when browser storage is unavailable. */ }
  function apply() {
    document.documentElement.dataset.theme = preference === 'system'
      ? (media.matches ? 'dark' : 'light') : preference;
    document.querySelectorAll('[data-theme-choice]').forEach(button => {
      button.setAttribute('aria-pressed', String(button.dataset.themeChoice === preference));
    });
  }
  window.adminTheme = {
    apply,
    set(value) {
      if (!choices.includes(value)) return;
      preference = value;
      try { localStorage.setItem(key, value); } catch (_) { /* Session-only choice. */ }
      apply();
    }
  };
  media.addEventListener('change', apply);
  window.addEventListener('storage', event => {
    if (event.key !== key) return;
    preference = choices.includes(event.newValue) ? event.newValue : 'light';
    apply();
  });
  apply();
})();
