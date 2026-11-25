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
    // The editor draws only the lines on screen. When the browser copies by itself (Ctrl+A pressed outside the editor, a menu, an
    // extension), it copies those 70 or so lines, not the file. So Ctrl+A anywhere on the page but a form field selects the whole
    // file in the editor, and a copy or cut the editor did not answer, made while its selection is not empty, takes that selection.
    var inField = function (el) { return el && !host.contains(el) && /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName); };
    document.addEventListener('keydown', function (e) {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === 'a' && !host.contains(e.target) && !inField(e.target)) {
        e.preventDefault();
        view.focus();
        view.dispatch({ selection: { anchor: 0, head: view.state.doc.length } });
      }
    });
    var take = function (e) {
      if (e.defaultPrevented || !e.clipboardData || inField(document.activeElement)) return;
      var sel = window.getSelection();
      var touches = sel && sel.rangeCount > 0 && (host.contains(sel.anchorNode) || host.contains(sel.focusNode) || sel.containsNode(host, true));
      if (!touches) return;
      var parts = view.state.selection.ranges.filter(function (r) { return !r.empty; })
        .map(function (r) { return view.state.sliceDoc(r.from, r.to); });
      if (!parts.length) return;
      e.preventDefault();
      e.clipboardData.setData('text/plain', parts.join(view.state.lineBreak));
      if (e.type === 'cut') view.dispatch(view.state.replaceSelection(''));
    };
    document.addEventListener('copy', take);
    document.addEventListener('cut', take);
  }).catch(function () { if (state) state.textContent = say('plain'); });
})();
