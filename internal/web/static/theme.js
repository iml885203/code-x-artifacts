// Loaded by the shell as a plain script in <head>, ahead of the stylesheet and
// the app, so the theme a person pinned is on <html> before the first paint
// (no flash of the other theme). It is a file because the CSP allows no inline
// script. The rule is web/src/shell/theme.ts; web/src/shell/theme.test.ts runs
// this file against it. Following the system sets nothing: the stylesheet does.
(function () {
  "use strict";
  try {
    var choice = window.localStorage.getItem("pictor-theme");
    if (choice === "light" || choice === "dark") {
      document.documentElement.setAttribute("data-theme", choice);
    }
  } catch (e) {
    // Storage blocked: follow the system.
  }
})();
