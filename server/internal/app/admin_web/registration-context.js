'use strict';
(() => {
  // First-party registration context only. The server labels these values as
  // client claims and does not use a shared environment alone to deny signup.
  function registrationContext() {
    try {
      const data = {
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || '',
        timezone_offset: -new Date().getTimezoneOffset(),
        language: navigator.language || '',
        platform: navigator.userAgentData?.platform || navigator.platform || '',
        screen_width: Math.round(screen.width),
        screen_height: Math.round(screen.height),
        pixel_ratio: Number.isFinite(devicePixelRatio) ? devicePixelRatio : 0,
        webdriver: navigator.webdriver === true,
      };
      return {'X-Client-Context': btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify(data))))};
    } catch (_) { return {}; }
  }
  globalThis.sameframeRegistrationContext = registrationContext;
})();
