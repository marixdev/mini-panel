// Runs in <head> before the page is drawn, so the saved or system theme shows from the first frame.
(function () {
  var saved = null;
  try { saved = localStorage.getItem('mpn-theme'); } catch (_) {}
  var dark = saved ? saved === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches;
  document.documentElement.classList.toggle('dark', dark);
})();
