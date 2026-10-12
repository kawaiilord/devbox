// Paint the saved appearance before Flutter starts. Preferences in the app
// are shared with the official website to keep navigation between pages consistent.
(() => {
  const storageKey = 'sameframe.appearance.boot';
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  let mode = 'light';
  try {
    const saved = localStorage.getItem(storageKey);
    if (['light', 'dark', 'system'].includes(saved)) mode = saved;
  } catch (_) {}
  function paint() {
    const dark = mode === 'dark' || (mode === 'system' && system.matches);
    const background = dark ? '#0c0c0d' : '#fafafa';
    const root = document.documentElement;
    root.style.setProperty('--frame-background', background);
    root.style.setProperty('--frame-foreground', dark ? '#f4f4f5' : '#18181b');
    root.style.colorScheme = dark ? 'dark' : 'light';
    root.dataset.theme = dark ? 'dark' : 'light';
    window.dispatchEvent(new CustomEvent('sameframe-theme-change'));
    document.querySelector('meta[name="theme-color"]').content = background;
  }
  window.sameframeAppearance = {
    getMode() {
      try {
        const saved = localStorage.getItem(storageKey);
        return ['light', 'dark', 'system'].includes(saved) ? saved : '';
      } catch (_) { return ''; }
    },
    setMode(next) {
      mode = ['light', 'dark', 'system'].includes(next) ? next : 'light';
      try { localStorage.setItem(storageKey, mode); } catch (_) {}
      paint();
    },
  };
  system.addEventListener('change', paint);
  paint();
})();
