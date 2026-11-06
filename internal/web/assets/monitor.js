// The hover layer of the charts on the monitoring page. Each chart is a <figure data-chart="..."> holding an SVG and a tooltip.
// The data is in the attribute: the time of every point, where it is in the picture, and each series' values already written.
// Texts go in with textContent, never as HTML.
(function () {
  'use strict';
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }
  document.querySelectorAll('figure[data-chart]').forEach(function (fig) {
    var data;
    try { data = JSON.parse(fig.getAttribute('data-chart')); } catch (e) { return; }
    if (!data || !data.x || !data.x.length) return;
    var svg = fig.querySelector('svg');
    // On a narrow screen the chart is wider than its card and scrolls. The newest values are at the right edge: start there.
    var scroller = fig.closest('.viz-scroll');
    if (scroller) scroller.scrollLeft = scroller.scrollWidth;
    var cross = fig.querySelector('.viz-cross');
    var tip = fig.querySelector('.viz-tip');
    var current = -1;

    function show(i) {
      if (i < 0 || i >= data.x.length) return;
      current = i;
      cross.setAttribute('x1', data.x[i]);
      cross.setAttribute('x2', data.x[i]);
      cross.removeAttribute('hidden');
      tip.textContent = '';
      tip.appendChild(el('div', 'viz-tip-time', data.t[i]));
      data.s.forEach(function (s) {
        var row = el('div', 'viz-tip-row');
        row.appendChild(el('span', 'viz-key viz-key-' + s.c));
        row.appendChild(el('strong', 'viz-tip-val', s.v[i]));
        row.appendChild(el('span', 'viz-tip-name', s.n));
        tip.appendChild(row);
      });
      tip.removeAttribute('hidden');
      var rect = svg.getBoundingClientRect();
      var px = data.x[i] / data.w * rect.width;
      var left = px + 12;
      if (left + tip.offsetWidth > rect.width) left = px - tip.offsetWidth - 12;
      tip.style.left = Math.max(0, left) + 'px';
      tip.style.top = '8px';
    }
    function hide() {
      cross.setAttribute('hidden', '');
      tip.setAttribute('hidden', '');
      current = -1;
    }
    function nearest(clientX) {
      var rect = svg.getBoundingClientRect();
      var x = (clientX - rect.left) / rect.width * data.w;
      var best = 0, bd = Infinity;
      for (var i = 0; i < data.x.length; i++) {
        var d = Math.abs(data.x[i] - x);
        if (d < bd) { bd = d; best = i; }
      }
      return best;
    }
    fig.addEventListener('pointermove', function (e) { show(nearest(e.clientX)); });
    fig.addEventListener('pointerleave', hide);
    fig.addEventListener('blur', hide);
    fig.addEventListener('focus', function () { show(data.x.length - 1); });
    fig.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowLeft') { show(Math.max(0, (current < 0 ? data.x.length : current) - 1)); e.preventDefault(); }
      else if (e.key === 'ArrowRight') { show(Math.min(data.x.length - 1, current + 1)); e.preventDefault(); }
      else if (e.key === 'Escape') { hide(); }
    });
  });
})();
