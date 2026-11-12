// The file manager page: choosing several files, sorting and filtering the list, the dialogs for each action, the menu
// of the right mouse button, and uploads with a progress bar (by choosing files or dropping them on the page). The page
// also works as plain links without this script; the script only adds the dialogs, the menu and the upload.
(function () {
  var root = document.getElementById('fm');
  if (!root) return;
  var site = root.getAttribute('data-site'), dir = root.getAttribute('data-dir'), csrf = root.getAttribute('data-csrf');
  var say = function (k) { return root.getAttribute('data-t-' + k) || ''; };
  var $ = function (id) { return document.getElementById(id); };
  var body = $('fm-body'), all = $('fm-all'), sel = $('fm-sel'), count = $('fm-count');
  var rows = body ? Array.prototype.slice.call(body.querySelectorAll('tr')) : [];
  var base = '/sites/' + encodeURIComponent(site) + '/files';
  var rel = function (name) { return (dir ? dir + '/' : '') + name; };
  var rowOf = function (name) { return rows.filter(function (r) { return r.getAttribute('data-name') === name; })[0]; };
  var boxOf = function (r) { return r.querySelector('input[name=item]'); };

  // ---- selection
  function chosen() { return rows.filter(function (r) { return boxOf(r).checked; }).map(function (r) { return r.getAttribute('data-name'); }); }
  function refresh() {
    var names = chosen();
    rows.forEach(function (r) { r.setAttribute('data-selected', boxOf(r).checked ? '1' : '0'); });
    sel.hidden = names.length === 0;
    count.textContent = say('selected').replace('%d', names.length);
    if (all) {
      var vis = rows.filter(function (r) { return !r.hidden; }).map(boxOf);
      all.checked = vis.length > 0 && vis.every(function (c) { return c.checked; });
      all.indeterminate = !all.checked && vis.some(function (c) { return c.checked; });
    }
  }
  function only(name) { rows.forEach(function (r) { boxOf(r).checked = r.getAttribute('data-name') === name; }); refresh(); }
  if (body) body.addEventListener('change', function (e) { if (e.target.name === 'item') refresh(); });
  if (all) all.addEventListener('change', function () {
    rows.forEach(function (r) { if (!r.hidden) boxOf(r).checked = all.checked; });
    refresh();
  });

  // ---- filter
  var filter = $('fm-filter');
  if (filter) filter.addEventListener('input', function () {
    var q = filter.value.trim().toLowerCase(), shown = 0;
    rows.forEach(function (r) {
      var hit = !q || r.getAttribute('data-name').toLowerCase().indexOf(q) >= 0;
      r.hidden = !hit;
      if (hit) shown++;
    });
    if ($('fm-nomatch')) $('fm-nomatch').hidden = shown > 0;
    refresh();
  });

  // ---- sort: folders always stay first
  var sortKey = 'name', sortDir = 1;
  document.querySelectorAll('[data-sort]').forEach(function (b) {
    b.addEventListener('click', function () {
      var key = b.getAttribute('data-sort');
      sortDir = key === sortKey ? -sortDir : 1;
      sortKey = key;
      rows.sort(function (a, c) {
        var da = a.getAttribute('data-dir'), dc = c.getAttribute('data-dir');
        if (da !== dc) return da === '1' ? -1 : 1;
        var x, y;
        if (key === 'name') { x = a.getAttribute('data-name').toLowerCase(); y = c.getAttribute('data-name').toLowerCase(); }
        else { x = Number(a.getAttribute('data-' + key)); y = Number(c.getAttribute('data-' + key)); }
        return (x < y ? -1 : x > y ? 1 : 0) * sortDir;
      });
      rows.forEach(function (r) { body.appendChild(r); });
      document.querySelectorAll('[data-arrow]').forEach(function (s) { s.textContent = ''; });
      b.querySelector('[data-arrow]').textContent = sortDir === 1 ? '▲' : '▼';
    });
  });

  // ---- dialogs
  function open(id) { var d = $(id); if (d && d.showModal) { d.showModal(); return d; } return null; }
  document.querySelectorAll('dialog').forEach(function (d) {
    d.addEventListener('click', function (e) { if (e.target === d || e.target.closest('[data-close]')) d.close(); });
  });
  function nameDialog(title, action, button, value, path) {
    var d = open('dlg-name');
    if (!d) return false;
    $('dlg-name-title').textContent = title;
    $('dlg-name-form').setAttribute('action', base + '/' + action);
    $('dlg-name-ok').textContent = button;
    $('dlg-name-path').value = path || '';
    var input = $('dlg-name-input');
    input.value = value || '';
    input.focus();
    var dot = (value || '').lastIndexOf('.');
    input.setSelectionRange(0, dot > 0 ? dot : (value || '').length); // renaming keeps the extension unselected
    return true;
  }
  function deleteDialog(names) {
    if (!names.length || !open('dlg-delete')) return false;
    $('dlg-delete-title').textContent = names.length === 1 ? say('delete-one').replace('%s', names[0]) : say('delete-many').replace('%d', names.length);
    $('dlg-delete-items').value = names.join('\n');
    var list = $('dlg-delete-list');
    list.textContent = '';
    names.forEach(function (n) { var li = document.createElement('li'); li.textContent = n; li.className = 'break-all'; list.appendChild(li); });
    return true;
  }
  function itemsInto(form, names) {
    form.querySelectorAll('input[name=item]').forEach(function (i) { i.remove(); });
    names.forEach(function (n) { var i = document.createElement('input'); i.type = 'hidden'; i.name = 'item'; i.value = n; form.appendChild(i); });
  }
  function zipDialog(names) {
    if (!names.length || !open('dlg-zip')) return false;
    itemsInto($('dlg-zip-form'), names);
    $('dlg-zip-list').textContent = names.join(', ');
    $('dlg-zip-name').value = (names.length === 1 ? names[0].replace(/\.[^.]*$/, '') || names[0] : (dir.split('/').pop() || 'archive')) + '.zip';
    $('dlg-zip-name').select();
    return true;
  }

  // ---- permissions: nine boxes and the octal number always agree
  var bits = Array.prototype.slice.call(document.querySelectorAll('#dlg-chmod [data-bit]'));
  var octal = $('chmod-octal');
  function fromOctal() {
    if (!/^[0-7]{3}$/.test(octal.value)) return;
    var v = parseInt(octal.value, 8);
    bits.forEach(function (b) { b.checked = (v & parseInt(b.getAttribute('data-bit'), 8)) !== 0; });
  }
  function fromBoxes() {
    var v = 0;
    bits.forEach(function (b) { if (b.checked) v |= parseInt(b.getAttribute('data-bit'), 8); });
    octal.value = ('000' + v.toString(8)).slice(-3);
  }
  if (octal) {
    octal.addEventListener('input', fromOctal);
    bits.forEach(function (b) { b.addEventListener('change', fromBoxes); });
    document.querySelectorAll('[data-preset]').forEach(function (b) {
      b.addEventListener('click', function () { octal.value = b.getAttribute('data-preset'); fromOctal(); });
    });
    $('chmod-rec-box').addEventListener('change', function () { $('chmod-scope').disabled = !this.checked; });
  }
  function chmodDialog(names) {
    if (!names.length || !open('dlg-chmod')) return false;
    itemsInto($('dlg-chmod-form'), names);
    $('dlg-chmod-list').textContent = names.join(', ');
    var first = rowOf(names[0]);
    octal.value = /^[0-7]{3}$/.test(first.getAttribute('data-mode')) ? first.getAttribute('data-mode') : '644';
    fromOctal();
    var anyDir = names.some(function (n) { return rowOf(n).getAttribute('data-dir') === '1'; });
    $('chmod-rec').hidden = !anyDir;
    $('chmod-rec-box').checked = false;
    $('chmod-scope').disabled = true;
    return true;
  }

  // ---- cut, copy and paste. The items are remembered for this site in this tab, so the person can open another folder
  // and paste there. Nothing changes on the server until Paste is chosen.
  var clipKey = 'mpn-fm-clip:' + site;
  function getClip() {
    try { var c = JSON.parse(sessionStorage.getItem(clipKey) || 'null'); return c && c.names && c.names.length ? c : null; } catch (_) { return null; }
  }
  function setClip(c) {
    try { if (c) sessionStorage.setItem(clipKey, JSON.stringify(c)); else sessionStorage.removeItem(clipKey); } catch (_) {}
    clipShow(c);
  }
  var memClip = null; // used when the browser keeps nothing in sessionStorage
  function clip() { return getClip() || memClip; }
  function clipShow(c) {
    var bar = $('fm-clip');
    if (!bar) return;
    bar.hidden = !c;
    rows.forEach(function (r) { r.removeAttribute('data-cut'); });
    if (!c) return;
    $('fm-clip-text').textContent = say(c.mode === 'cut' ? 'clip-cut' : 'clip-copy').replace('%d', c.names.length);
    $('fm-clip-paste').textContent = say('paste-here').replace('%d', c.names.length);
    if (c.mode === 'cut' && c.dir === dir) c.names.forEach(function (n) { var r = rowOf(n); if (r) r.setAttribute('data-cut', '1'); });
  }
  function remember(mode, names) {
    if (!names.length) return false;
    memClip = { mode: mode, dir: dir, names: names };
    setClip(memClip);
    return true;
  }
  function paste(target) {
    var c = clip(), form = $('fm-paste-form');
    if (!c || !form) return false;
    form.querySelector('[name=dir]').value = target;
    form.querySelector('[name=from]').value = c.dir;
    form.querySelector('[name=mode]').value = c.mode;
    itemsInto(form, c.names);
    if (c.mode === 'cut') { memClip = null; setClip(null); } // a cut can be pasted once; a copy as often as wanted
    form.submit();
    return true;
  }

  // ---- one place that carries out an action, whether it came from a button, the menu or the keyboard
  function run(act, names) {
    var one = names[0];
    switch (act) {
      case 'cut': return remember('cut', names);
      case 'copy': return remember('copy', names);
      // From the menu on a folder, the items go into that folder; otherwise into the open one.
      case 'paste': return paste(one && rowOf(one) && rowOf(one).getAttribute('data-dir') === '1' ? rel(one) : dir);
      case 'clipclear': memClip = null; setClip(null); return true;
      case 'upload': return openUpload();
      case 'newfile': return nameDialog(say('newfile'), 'newfile', say('create'), '');
      case 'newdir': return nameDialog(say('newdir'), 'mkdir', say('create'), '');
      case 'rename': return one && nameDialog(say('rename'), 'rename', say('rename'), one, rel(one));
      case 'delete': return deleteDialog(names);
      case 'zip': return zipDialog(names);
      case 'chmod': return chmodDialog(names);
      case 'unzip':
        if (!one || !open('dlg-unzip')) return false;
        $('dlg-unzip-zip').value = one; $('dlg-unzip-name').textContent = one;
        return true;
      case 'open': location.href = base + '?path=' + encodeURIComponent(rel(one)); return true;
      case 'edit': location.href = base + '/edit?path=' + encodeURIComponent(rel(one)); return true;
      case 'download': location.href = base + '/download?path=' + encodeURIComponent(rel(one)); return true;
      case 'refresh': location.reload(); return true;
      case 'selectall': if (all) { all.checked = true; all.dispatchEvent(new Event('change')); } return true;
      case 'clear': rows.forEach(function (r) { boxOf(r).checked = false; }); refresh(); return true;
    }
    return false;
  }
  root.addEventListener('click', function (e) {
    var b = e.target.closest('[data-fm]');
    if (!b) return;
    var act = b.getAttribute('data-fm'), name = b.getAttribute('data-name');
    if (act === 'delete-selected') act = 'delete';
    if (run(act, name ? [name] : act === 'paste' ? [] : chosen())) e.preventDefault();
  });

  // ---- the menu of the right mouse button
  var menu = $('fm-menu');
  var items = menu ? Array.prototype.slice.call(menu.querySelectorAll('.fm-mi')) : [];
  var openedAt = 0;
  function closeMenu() { if (menu && !menu.hidden) { menu.hidden = true; } }
  // A window event (resize, blur, scroll) that arrives while the menu is only just opening is a side effect of opening it or
  // of the page settling, not the person leaving; closing on it would make the menu vanish as it appears.
  function closeOnWindow() { if (Date.now() - openedAt > 300) closeMenu(); }
  function showMenu(x, y, names) {
    var one = names.length === 1 ? rowOf(names[0]) : null, isDir = one && one.getAttribute('data-dir') === '1';
    var allowed = !names.length ? ['upload', 'newfile', 'newdir', 'refresh', 'selectall'] :
      names.length > 1 ? ['cut', 'copy', 'zip', 'chmod', 'delete'] :
      isDir ? ['open', 'cut', 'copy', 'rename', 'chmod', 'zip', 'delete'] :
      ['edit', 'download', 'cut', 'copy', 'rename', 'chmod', 'zip', 'unzip', 'delete'].filter(function (a) {
        return (a !== 'edit' || one.getAttribute('data-edit') === '1') && (a !== 'unzip' || one.getAttribute('data-zip') === '1');
      });
    // Paste goes into the open folder, or into the folder that was right-clicked.
    if (clip() && (!names.length || isDir)) allowed.push('paste');
    items.forEach(function (i) { i.hidden = allowed.indexOf(i.getAttribute('data-m')) < 0; });
    menu.querySelectorAll('.fm-sep').forEach(function (s) { s.hidden = !names.length; });
    menu.setAttribute('data-names', JSON.stringify(names));
    menu.hidden = false;
    openedAt = Date.now();
    var w = menu.offsetWidth, h = menu.offsetHeight;
    menu.style.left = Math.max(4, Math.min(x, window.innerWidth - w - 4)) + 'px';
    menu.style.top = Math.max(4, Math.min(y, window.innerHeight - h - 4)) + 'px';
    menu.tabIndex = -1;
    menu.focus({ preventScroll: true });
  }
  if (menu) {
    root.addEventListener('contextmenu', function (e) {
      if (e.target.closest('input[type=search], input[type=text], textarea, a[href]:not([href*="/files"])')) return; // leave the browser's own menu there
      var tr = e.target.closest('#fm-body tr');
      e.preventDefault();
      if (tr) {
        var name = tr.getAttribute('data-name');
        if (!boxOf(tr).checked) only(name); // like a desktop: a click outside the chosen items chooses just this one
        showMenu(e.clientX, e.clientY, chosen());
      } else {
        showMenu(e.clientX, e.clientY, []);
      }
    });
    menu.addEventListener('click', function (e) {
      var b = e.target.closest('.fm-mi');
      if (!b) return;
      var names = JSON.parse(menu.getAttribute('data-names') || '[]');
      closeMenu();
      run(b.getAttribute('data-m'), names);
    });
    document.addEventListener('click', function (e) { if (!menu.contains(e.target)) closeMenu(); });
    document.addEventListener('contextmenu', function (e) { if (!root.contains(e.target)) closeMenu(); });
    // "scroll" does not bubble, so it is caught on the way down. "blur" must not be: with capture it would also fire when the
    // menu takes the focus from the element that had it, and the menu would close at the moment it opens.
    window.addEventListener('scroll', closeOnWindow, true);
    window.addEventListener('resize', closeOnWindow);
    window.addEventListener('blur', closeOnWindow);
    menu.addEventListener('keydown', function (e) {
      var vis = items.filter(function (i) { return !i.hidden; }), at = vis.indexOf(document.activeElement);
      if (e.key === 'ArrowDown') { vis[(at + 1) % vis.length].focus(); e.preventDefault(); }
      else if (e.key === 'ArrowUp') { vis[(at <= 0 ? vis.length : at) - 1].focus(); e.preventDefault(); }
      else if (e.key === 'Home') { vis[0].focus(); e.preventDefault(); }
      else if (e.key === 'End') { vis[vis.length - 1].focus(); e.preventDefault(); }
      else if (e.key === 'Escape' || e.key === 'Tab') { closeMenu(); e.preventDefault(); }
    });
  }
  // Keys, when nothing is being typed: F2 renames the chosen item, Delete deletes the chosen items, Escape clears the choice.
  document.addEventListener('keydown', function (e) {
    if (e.defaultPrevented || e.target.closest('input, textarea, select, dialog') || document.querySelector('dialog[open]')) return;
    var names = chosen();
    if (e.key === 'Escape') { closeMenu(); if (names.length) run('clear', []); }
    else if (e.key === 'F2' && names.length === 1) { e.preventDefault(); run('rename', names); }
    else if (e.key === 'Delete' && names.length) { e.preventDefault(); run('delete', names); }
    else if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && window.getSelection && String(window.getSelection()) === '') {
      // Only when no text is selected on the page, so copying text still works as usual.
      var k = e.key.toLowerCase();
      if (k === 'x' && names.length) { e.preventDefault(); run('cut', names); }
      else if (k === 'c' && names.length) { e.preventDefault(); run('copy', names); }
      else if (k === 'v' && clip()) { e.preventDefault(); run('paste', []); }
    }
  });

  // ---- upload
  var upDialog = $('dlg-upload'), upInput = $('up-input'), zone = $('up-zone'), busy = false, changed = false;
  function openUpload() {
    var d = open('dlg-upload');
    if (!d) return false;
    $('up-run').hidden = true;
    $('up-errors').hidden = true;
    return true;
  }
  function send(files) {
    if (!files.length || busy) return;
    if (!upDialog.open) openUpload();
    var fd = new FormData();
    if ($('up-over').checked) fd.append('overwrite', 'on'); // the panel reads this before the files
    for (var i = 0; i < files.length; i++) fd.append('files', files[i], files[i].name);
    var xhr = new XMLHttpRequest(), bar = $('up-progress'), status = $('up-status'), errs = $('up-errors');
    xhr.open('POST', base + '/upload?dir=' + encodeURIComponent(dir) + '&csrf=' + encodeURIComponent(csrf));
    xhr.setRequestHeader('Accept', 'application/json');
    errs.hidden = true; errs.textContent = '';
    $('up-run').hidden = false; bar.value = 0;
    status.textContent = say('uploading');
    busy = true;
    xhr.upload.onprogress = function (e) {
      if (e.lengthComputable) { bar.value = Math.round(e.loaded * 100 / e.total); status.textContent = say('uploading') + ' ' + bar.value + '%'; }
    };
    xhr.onload = function () {
      busy = false;
      var res = null;
      try { res = JSON.parse(xhr.responseText); } catch (_) {}
      if (!res) { status.textContent = say('failed') + ' (' + xhr.status + ')'; return; }
      bar.value = 100; status.textContent = res.title; changed = changed || res.done > 0;
      if (res.errors && res.errors.length) {
        errs.hidden = false;
        res.errors.forEach(function (m) { var li = document.createElement('li'); li.textContent = m; errs.appendChild(li); });
      } else {
        setTimeout(function () { location.reload(); }, 500);
      }
    };
    xhr.onerror = function () { busy = false; status.textContent = say('failed'); };
    xhr.send(fd);
  }
  if (upInput) upInput.addEventListener('change', function () { send(upInput.files); upInput.value = ''; });
  if (upDialog) upDialog.addEventListener('close', function () { if (changed && !busy) location.reload(); });

  // ---- dropping files anywhere on the page
  var veil = $('fm-drop'), depth = 0;
  function hasFiles(e) { return e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], 'Files') >= 0; }
  document.addEventListener('dragenter', function (e) { if (hasFiles(e)) { depth++; veil.hidden = false; e.preventDefault(); } });
  document.addEventListener('dragover', function (e) { if (hasFiles(e)) e.preventDefault(); });
  document.addEventListener('dragleave', function (e) { if (hasFiles(e) && --depth <= 0) { depth = 0; veil.hidden = true; } });
  document.addEventListener('drop', function (e) {
    if (!hasFiles(e)) return;
    e.preventDefault(); depth = 0; veil.hidden = true;
    send(Array.prototype.filter.call(e.dataTransfer.files, function (f) { return f.size > 0 || f.type; })); // a folder has neither
  });
  if (zone) {
    zone.addEventListener('dragover', function () { zone.setAttribute('data-over', '1'); });
    ['dragleave', 'drop'].forEach(function (n) { zone.addEventListener(n, function () { zone.removeAttribute('data-over'); }); });
  }
  clipShow(clip());
  refresh();
})();
