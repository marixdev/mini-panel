// The editing page of the file manager. The page works with a plain text box; when the code editor has loaded it takes
// the text box's place, and what is saved is always read back into the text box first.
(function () {
  var form = document.getElementById('edit-form');
  var area = document.getElementById('edit-content');
  if (!form || !area) return;
  var host = document.getElementById('edit-host');
  var state = document.getElementById('edit-state');
  var dirty = false, view = null;
  var say = function (k) { return form.getAttribute('data-t-' + k) || ''; };
  var mark = function () { dirty = true; if (state) state.textContent = say('dirty'); };

  area.addEventListener('input', mark);
  form.addEventListener('submit', function () {
    if (view) area.value = view.state.doc.toString();
    dirty = false;
  });
  window.addEventListener('beforeunload', function (e) { if (dirty) { e.preventDefault(); e.returnValue = ''; } });

  // The editor is a module with the languages split off; it is fetched only on this page.
  if (!host) return;
  if (state) state.textContent = say('loading');
  import(host.getAttribute('data-src')).then(function (m) {
    view = m.create(host, { doc: area.value, name: host.getAttribute('data-name'), nonce: host.getAttribute('data-nonce'), onChange: mark });
    area.hidden = true;
    host.hidden = false;
    if (state) state.textContent = '';
    view.focus();
    form.addEventListener('keydown', function (e) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); form.requestSubmit(); }
    });
  }).catch(function () { if (state) state.textContent = say('plain'); });
})();
