// The terminal of a website: xterm.js in the browser, a WebSocket to the panel, and from there an SSH login as the
// website's own account. Nothing typed or shown is kept by the panel.
(function () {
  var box = document.getElementById('terminal');
  if (!box || typeof Terminal === 'undefined') return;
  // xterm.js makes <style> elements; the page's policy allows exactly the ones that carry this nonce.
  window.MPN_STYLE_NONCE = box.getAttribute('data-nonce');
  var say = function (key) { return box.getAttribute('data-t-' + key) || ''; };
  var state = document.getElementById('terminal-state');
  var show = function (text, cls) { if (state) { state.textContent = text; state.className = cls; } };

  var term = new Terminal({
    cursorBlink: true, scrollback: 5000, fontSize: 14,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
    theme: { background: '#0b1220', foreground: '#e2e8f0', cursor: '#5eead4', selectionBackground: '#334155' }
  });
  var fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(box);
  fit.fit();

  var scheme = location.protocol === 'https:' ? 'wss://' : 'ws://';
  var url = scheme + location.host + box.getAttribute('data-ws') + '?csrf=' + encodeURIComponent(box.getAttribute('data-csrf')) +
    '&cols=' + term.cols + '&rows=' + term.rows;
  var ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';
  var encoder = new TextEncoder();
  show(say('connecting'), 'badge-gray');

  ws.onopen = function () { show(say('connected'), 'badge-teal'); term.focus(); };
  ws.onmessage = function (e) {
    if (typeof e.data === 'string') { // a message from the panel itself, such as why the login failed
      try { var m = JSON.parse(e.data); if (m.error) term.write('\r\n\x1b[31m' + m.error + '\x1b[0m\r\n'); } catch (_) {}
      return;
    }
    term.write(new Uint8Array(e.data));
  };
  ws.onclose = function () { show(say('closed'), 'badge-amber'); term.write('\r\n\x1b[2m' + say('closed') + '\x1b[0m\r\n'); };
  ws.onerror = function () { show(say('failed'), 'badge-rose'); };

  term.onData(function (data) { if (ws.readyState === 1) ws.send(encoder.encode(data)); });
  term.onResize(function (size) {
    if (ws.readyState === 1) ws.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
  });
  var refit = function () { try { fit.fit(); } catch (_) {} };
  window.addEventListener('resize', refit);
  if (typeof ResizeObserver !== 'undefined') new ResizeObserver(refit).observe(box);
})();
