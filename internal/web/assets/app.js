// The only script of the panel: theme switch, copy buttons, and the helpers of the "add a website" form.
// Nothing is fetched and nothing is sent. Without the script the form still works: the server then picks the account
// name from the domain and makes a random password itself.

var SECOND_LEVEL = ['co', 'com', 'net', 'org', 'gov', 'edu', 'ac'];

// The same rule as siteUserFrom in siteuser.go, so the form shows what the server would choose:
// the top-level domain and a leading "www." go, so "datvu.io" gives "datvu" and "blog.example.com" gives "blog-example".
function deriveUser(value, reserved) {
  var host = (value.trim().split(/[\s,]+/)[0] || '').toLowerCase().replace(/^www\./, '');
  var labels = host.split('.');
  if (labels.length >= 2) {
    var tld = labels.pop();
    if (tld.length === 2 && labels.length >= 2 && SECOND_LEVEL.indexOf(labels[labels.length - 1]) >= 0) labels.pop();
  }
  var u = labels.join('-').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  if (!u) return 'site';
  if (/^[0-9]/.test(u)) u = 'u' + u;
  if (u.length > 32) u = u.slice(0, 32).replace(/-+$/, '');
  if (reserved.indexOf(u) >= 0 || u.indexOf('systemd-') === 0) u = u.slice(0, 28).replace(/-+$/, '') + '-web';
  return u;
}

function randomPassword() {
  var alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789', out = '';
  var bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  for (var i = 0; i < bytes.length; i++) out += alphabet[bytes[i] % alphabet.length];
  return out;
}

// Typing a domain fills in the account name until the person edits that field themselves.
document.addEventListener('input', function (e) {
  var user = document.querySelector('[data-user]');
  if (!user) return;
  if (e.target.matches('[data-hosts]') && !user.dataset.touched) {
    user.value = deriveUser(e.target.value, (user.getAttribute('data-reserved') || '').split(','));
  } else if (e.target === user) {
    user.dataset.touched = '1';
  }
});

// The details of WordPress are asked only when WordPress is chosen. They are required only while shown, because a browser refuses to
// send a form that has a required field it is hiding.
function syncApp(sel) {
  var form = sel.closest('form'), box = form && form.querySelector('[data-app-wp]');
  if (!box) return;
  var wp = sel.value === 'wordpress';
  box.hidden = !wp;
  box.querySelectorAll('[data-app-req]').forEach(function (f) { f.required = wp; });
}
document.addEventListener('change', function (e) {
  if (e.target.matches('[data-app-select]')) syncApp(e.target);
});
document.querySelectorAll('[data-app-select]').forEach(syncApp);

// A random password is put in the field when the form opens, and again each time the link is used.
document.querySelectorAll('[data-password]').forEach(function (field) {
  if (!field.value) field.value = randomPassword();
});

// Copies text to the clipboard. The panel is served over HTTPS, where the clipboard API works; a page opened in a way that
// hides it falls back to selecting a hidden box and asking the browser to copy.
function copyText(text) {
  if (navigator.clipboard && window.isSecureContext) return navigator.clipboard.writeText(text);
  return new Promise(function (ok, fail) {
    var box = document.createElement('textarea');
    box.value = text;
    box.setAttribute('readonly', '');
    box.className = 'sr-only';
    document.body.appendChild(box);
    box.select();
    var done = false;
    try { done = document.execCommand('copy'); } catch (_) {}
    document.body.removeChild(box);
    if (done) ok(); else fail();
  });
}

// The menu under the name of the signed-in person: the button opens it, a press anywhere else or Escape closes it, and the
// arrow keys move between its entries.
function setMenu(menu, open) {
  var panel = menu.querySelector('[data-menu-panel]'), button = menu.querySelector('[data-menu-button]');
  panel.hidden = !open;
  button.setAttribute('aria-expanded', open ? 'true' : 'false');
  if (open) { var first = panel.querySelector('[role^=menuitem]'); if (first) first.focus(); }
}
document.addEventListener('click', function (e) {
  var button = e.target.closest('[data-menu-button]');
  document.querySelectorAll('[data-menu]').forEach(function (menu) {
    if (button && menu.contains(button)) setMenu(menu, menu.querySelector('[data-menu-panel]').hidden);
    else if (!menu.contains(e.target)) setMenu(menu, false);
  });
});
document.addEventListener('keydown', function (e) {
  var menu = e.target.closest && e.target.closest('[data-menu]');
  if (!menu) return;
  var panel = menu.querySelector('[data-menu-panel]');
  if (e.key === 'Escape' && !panel.hidden) {
    setMenu(menu, false);
    menu.querySelector('[data-menu-button]').focus();
  } else if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && !panel.hidden) {
    var items = Array.prototype.slice.call(panel.querySelectorAll('[role^=menuitem]'));
    var at = items.indexOf(document.activeElement);
    items[(at + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length].focus();
    e.preventDefault();
  }
});

document.addEventListener('click', function (e) {
  var gen = e.target.closest('[data-gen-password]');
  if (gen) {
    document.getElementById(gen.getAttribute('data-gen-password')).value = randomPassword();
    return;
  }
  var eye = e.target.closest('[data-toggle-pw]');
  if (eye) { // the show/hide button of a password field: the field's id is in the attribute
    var field = document.getElementById(eye.getAttribute('data-toggle-pw'));
    var show = field.type === 'password';
    field.type = show ? 'text' : 'password';
    eye.textContent = eye.getAttribute(show ? 'data-hide' : 'data-show');
    eye.setAttribute('aria-pressed', show ? 'true' : 'false');
    return;
  }
  if (e.target.closest('[data-history-back]')) { // "back" on the page for a missing address; without history it leaves the link to "/"
    if (history.length > 1) { history.back(); e.preventDefault(); }
    return;
  }
  var toggle = e.target.closest('[data-theme-toggle]');
  if (toggle) {
    var dark = !document.documentElement.classList.contains('dark');
    document.documentElement.classList.toggle('dark', dark);
    try { localStorage.setItem('mpn-theme', dark ? 'dark' : 'light'); } catch (_) {}
    return;
  }
  var copy = e.target.closest('[data-copy]');
  if (copy) {
    copyText(copy.getAttribute('data-copy')).then(function () {
      var status = copy.querySelector('[data-copy-status]');
      if (status) { // a chip: the check mark shows for a moment, and a screen reader hears the words
        copy.classList.add('is-copied');
        status.textContent = copy.getAttribute('data-copied') || 'OK';
        setTimeout(function () { copy.classList.remove('is-copied'); status.textContent = ''; }, 1500);
        return;
      }
      var before = copy.textContent; // a plain button: its own words change for a moment
      copy.textContent = copy.getAttribute('data-copied') || 'OK';
      setTimeout(function () { copy.textContent = before; }, 1500);
    }, function () {});
  }
});

// The page that opens the database browser has one link; it is followed at once. The link stays on the page for a browser
// that does not run this script, or for somebody who wants to open it again.
(function () {
  var go = document.querySelector('[data-autonav]');
  if (go) setTimeout(function () { location.href = go.href; }, 50);
})();
