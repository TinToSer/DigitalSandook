'use strict';
(() => {
const token = location.pathname.split('/')[2] || '';
const base = '/api/public/' + encodeURIComponent(token);
const app = document.getElementById('app');
try { const t = localStorage.getItem('dsd.theme'); if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t; } catch (_) {}
function h(tag, props, ...kids) {
  const e = document.createElement(tag);
  for (const k in props || {}) {
    const v = props[k];
    if (v == null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else if (k === 'value') e.value = v;
    else e.setAttribute(k, v === true ? '' : v);
  }
  for (const c of kids.flat(Infinity)) if (c != null && c !== false) e.append(c.nodeType ? c : document.createTextNode(String(c)));
  return e;
}
const q = o => Object.entries(o).filter(([, v]) => v != null).map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&');
function fmtSize(n) {
  if (n < 1024) return n + ' B';
  const u = ['KB', 'MB', 'GB', 'TB']; let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < u.length - 1);
  return n.toFixed(n >= 100 ? 0 : n >= 10 ? 1 : 2) + ' ' + u[i];
}
async function get(url, opts) {
  const r = await fetch(url, Object.assign({ credentials: 'same-origin' }, opts));
  let j = {}; try { j = await r.json(); } catch (_) {}
  if (!r.ok) { const e = new Error(j.error || 'Request failed'); e.status = r.status; e.body = j; throw e; }
  return j;
}
const shell = (...kids) => app.replaceChildren(h('div', { class: 'login' }, h('div', { class: 'modal', style: null }, h('div', { class: 'hd' }, h('img', { src: '/static/logo.svg', width: 26, height: 26, alt: '' }), h('span', { class: 'grow' }, 'DigitalSanDook'), ' shared files'), h('div', { class: 'bd' }, kids))));

async function main() {
  let info;
  try { info = await get(base + '/info'); } catch (e) { return shell(h('p', null, e.message)); }
  if (info.needsPassword) return askPassword(info);
  if (!info.isDir) return fileView(info);
  browse(info, '/');
}
function askPassword(info) {
  const pw = h('input', { type: 'password', placeholder: 'Password', autocomplete: 'off' });
  const err = h('div', { class: 'err' });
  const go = async ev => {
    ev.preventDefault(); err.textContent = '';
    try { await get(base + '/unlock', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password: pw.value }) }); main(); }
    catch (e) { err.textContent = e.message; }
  };
  shell(h('form', { onsubmit: go }, h('p', null, h('b', null, info.name), ' is password protected.'), pw, err, h('div', { class: 'row' }, h('button', { class: 'btn primary', type: 'submit' }, 'Unlock'))));
  pw.focus();
}
function fileView(info) {
  shell(h('p', null, h('b', null, info.name)), h('a', { class: 'btn primary', href: base + '/dl?p=/', download: '' }, 'Download'));
}
async function browse(info, path) {
  let d;
  try { d = await get(base + '/list?' + q({ p: path })); } catch (e) { if (e.body && e.body.needsPassword) return main(); return shell(h('p', null, e.message)); }
  const parts = path.split('/').filter(Boolean);
  const crumbs = h('div', { class: 'crumbs' }, h('a', { onclick: () => browse(info, '/') }, info.name));
  let acc = '';
  for (const p of parts) { acc += '/' + p; const t = acc; crumbs.append(h('span', { class: 'sep' }, '/'), h('a', { onclick: () => browse(info, t) }, p)); }
  const rows = d.entries.map(e => {
    const p = (path === '/' ? '' : path) + '/' + e.n;
    return h('tr', null,
      h('td', null, e.d ? h('a', { onclick: () => browse(info, p), class: 'fi d' }, '📁 ' + e.n) : h('span', null, e.n)),
      h('td', { class: 'muted' }, e.d ? '' : fmtSize(e.s)),
      h('td', null, e.d ? h('a', { class: 'btn sm', href: base + '/zip?' + q({ p }), download: '' }, 'ZIP') : h('a', { class: 'btn sm', href: base + '/dl?' + q({ p }), download: '' }, 'Download')));
  });
  shell(crumbs,
    h('p', null, h('a', { class: 'btn primary', href: base + '/zip?' + q({ p: path }), download: '' }, 'Download this folder (ZIP)')),
    rows.length ? h('table', { class: 'tbl' }, h('tbody', null, rows)) : h('p', { class: 'muted' }, 'This folder is empty.'));
}
main();
})();
