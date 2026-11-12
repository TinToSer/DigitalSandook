'use strict';
(() => {
// ============ tiny DOM toolkit (no innerHTML anywhere: XSS-safe by construction) ============
const SVGNS = 'http://www.w3.org/2000/svg';
const ICONS = {
  folder: 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z',
  file: 'M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z M13 2v7h7',
  upload: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M17 8l-5-5-5 5 M12 3v12',
  download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M7 10l5 5 5-5 M12 15V3',
  trash: 'M3 6h18 M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6 M10 11v6 M14 11v6 M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2',
  edit: 'M12 20h9 M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
  copy: 'M9 9h11a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2v-9a2 2 0 0 1 2-2z M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1',
  cut: 'M6 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M20 4L8.1 15.9 M14.5 14.5L20 20 M8.1 8.1L12 12',
  paste: 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2 M9 2h6a1 1 0 0 1 1 1v2a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z',
  share: 'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M8.6 13.5l6.8 4 M15.4 6.5l-6.8 4',
  users: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z M23 21v-2a4 4 0 0 0-3-3.9 M16 3.1a4 4 0 0 1 0 7.8',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z M21 21l-4.3-4.3',
  x: 'M18 6L6 18 M6 6l12 12',
  plus: 'M12 5v14 M5 12h14',
  refresh: 'M23 4v6h-6 M1 20v-6h6 M3.5 9a9 9 0 0 1 14.9-3.4L23 10 M1 14l4.6 4.4A9 9 0 0 0 20.5 15',
  lock: 'M5 11h14a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2z M7 11V7a5 5 0 0 1 10 0v4',
  link: 'M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7 M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7',
  logout: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4 M16 17l5-5-5-5 M21 12H9',
  shield: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z',
  fplus: 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z M12 11v6 M9 14h6',
  up: 'M12 19V5 M5 12l7-7 7 7',
  drive: 'M22 12H2 M5.5 5.1L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.1z M6 16h.01 M10 16h.01',
  check: 'M20 6L9 17l-5-5',
  eye: 'M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  menu: 'M3 12h18 M3 6h18 M3 18h18',
  key: 'M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 1-7.8 7.8 5.5 5.5 0 0 1 7.8-7.8z M15.5 7.5l3 3L22 7l-3-3',
  image: 'M19 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2z M8.5 10a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3z M21 15l-5-5L5 21',
  film: 'M4 4h16a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z M7 4v16 M17 4v16 M3 12h18',
  music: 'M9 18V5l12-2v13 M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M18 19a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  archive: 'M21 8v13H3V8 M1 3h22v5H1z M10 12h4',
  code: 'M16 18l6-6-6-6 M8 6l-6 6 6 6',
  pdf: 'M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z M13 2v7h7 M8 13h8 M8 17h5',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10z M12 1v2 M12 21v2 M4.2 4.2l1.4 1.4 M18.4 18.4l1.4 1.4 M1 12h2 M21 12h2 M4.2 19.8l1.4-1.4 M18.4 5.6l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z',
  monitor: 'M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z M8 21h8 M12 17v4',
  grid: 'M3 3h7v7H3z M14 3h7v7h-7z M14 14h7v7h-7z M3 14h7v7H3z',
  list: 'M8 6h13 M8 12h13 M8 18h13 M3 6h.01 M3 12h.01 M3 18h.01',
  chevL: 'M15 18l-6-6 6-6',
  chevR: 'M9 18l6-6-6-6',
};
function ic(name) {
  const s = document.createElementNS(SVGNS, 'svg');
  s.setAttribute('viewBox', '0 0 24 24'); s.setAttribute('class', 'i');
  for (const d of (ICONS[name] || ICONS.file).split(' M')) {
    const p = document.createElementNS(SVGNS, 'path');
    p.setAttribute('d', d.startsWith('M') ? d : 'M' + d); s.append(p);
  }
  return s;
}
function add(e, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k == null || k === false) continue;
    e.append(k.nodeType ? k : document.createTextNode(String(k)));
  }
}
function h(tag, props, ...kids) {
  const e = document.createElement(tag);
  for (const k in props || {}) {
    const v = props[k];
    if (v == null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else if (k === 'value' || k === 'checked' || k === 'disabled' || k === 'selected') e[k] = v;
    else e.setAttribute(k, v === true ? '' : v);
  }
  add(e, kids);
  return e;
}
const $ = (s, r = document) => r.querySelector(s);
// key events can target the document itself, which has no .matches()
const isField = t => !!(t && t.matches && t.matches('input,select,textarea'));
const btn = (icon, label, onclick, cls = '') => h('button', { class: 'btn ' + cls, onclick, title: label, type: 'button' }, icon ? ic(icon) : null, label ? h('span', { class: 'lbl' }, label) : null);

function fmtSize(n) {
  if (n < 1024) return n + ' B';
  const u = ['KB', 'MB', 'GB', 'TB', 'PB']; let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < u.length - 1);
  return n.toFixed(n >= 100 ? 0 : n >= 10 ? 1 : 2) + ' ' + u[i];
}
function fmtDate(sec) {
  if (!sec || sec < 86400) return '';
  const d = new Date(sec * 1000);
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
function ago(sec) {
  if (!sec || sec < 86400) return 'never';
  const s = Math.max(0, Date.now() / 1000 - sec);
  if (s < 60) return 'just now';
  if (s < 3600) return Math.floor(s / 60) + ' min ago';
  if (s < 86400) return Math.floor(s / 3600) + ' h ago';
  return Math.floor(s / 86400) + ' d ago';
}
const extOf = n => { const i = n.lastIndexOf('.'); return i > 0 ? n.slice(i + 1).toLowerCase() : ''; };
const LEVELS = { 1: 'Viewer', 2: 'Editor', 3: 'Owner' };
const LEVEL_HELP = { 1: 'Viewer - browse, preview, download', 2: 'Editor - also upload, rename, move, delete, share', 3: 'Owner - also manage everyone\'s access below' };
const joinPath = (d, n) => (d === '/' ? '' : d) + '/' + n;
const parentOf = p => { const i = p.lastIndexOf('/'); return i <= 0 ? '/' : p.slice(0, i); };
const baseName = p => p.slice(p.lastIndexOf('/') + 1);

// ============ state & API ============
// localStorage can be unavailable (private mode, blocked storage): never let it break the app
const lsGet = k => { try { return localStorage.getItem(k); } catch (_) { return null; } };
const lsSet = (k, v) => { try { localStorage.setItem(k, v); } catch (_) {} };
const savedSort = (() => { try { const s = JSON.parse(lsGet('dsd.sort')); if (s && ['name', 'size', 'mtime'].includes(s.key) && Math.abs(s.dir) === 1) return s; } catch (_) {} return { key: 'name', dir: 1 }; })();
const S = {
  me: null, csrf: '', roots: [], cur: { mount: '', path: '/' }, level: 0, entries: [], hits: null,
  sel: new Set(), last: null, clip: null, sort: savedSort, limit: 500, loading: false,
  view: lsGet('dsd.view') === 'grid' ? 'grid' : 'list',
};
const THUMBS = new Set(['jpg', 'jpeg', 'png', 'gif']);

// theme: auto (follow the OS) -> light -> dark
const THEMES = { auto: ['monitor', 'Theme: follow system'], light: ['sun', 'Theme: light'], dark: ['moon', 'Theme: dark'] };
function applyTheme(t) {
  if (!THEMES[t]) t = 'auto';
  if (t === 'auto') delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = t;
  return t;
}
let theme = applyTheme(lsGet('dsd.theme'));
function themeBtn() {
  const b = h('button', { class: 'btn ghost', type: 'button', title: THEMES[theme][1] }, ic(THEMES[theme][0]));
  b.addEventListener('click', () => {
    theme = applyTheme({ auto: 'light', light: 'dark', dark: 'auto' }[theme]);
    lsSet('dsd.theme', theme);
    b.replaceChildren(ic(THEMES[theme][0])); b.title = THEMES[theme][1];
  });
  return b;
}
const app = $('#app');

class ApiError extends Error { constructor(m, status, body) { super(m); this.status = status; this.body = body; } }
async function api(method, url, body) {
  let r;
  try {
    r = await fetch(url, {
      method, credentials: 'same-origin',
      headers: Object.assign({ 'X-CSRF-Token': S.csrf }, body !== undefined ? { 'Content-Type': 'application/json' } : {}),
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) { throw new ApiError('Network error - is the server reachable?', 0); }
  if (r.status === 401 && S.me) { S.me = null; showLogin('Your session has expired. Please sign in again.'); }
  if (!r.ok) {
    let j = {}; try { j = await r.json(); } catch (_) {}
    throw new ApiError(j.error || ('Request failed (' + r.status + ')'), r.status, j);
  }
  if (r.status === 204) return null;
  return r.json();
}
const q = o => Object.entries(o).filter(([, v]) => v != null).map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&');

// ============ toast / modal ============
let toastBox;
function toast(msg, kind) {
  if (!toastBox) { toastBox = h('div', { class: 'toasts' }); document.body.append(toastBox); }
  const t = h('div', { class: 'toast ' + (kind || '') }, msg);
  toastBox.append(t);
  setTimeout(() => t.remove(), kind === 'bad' ? 6000 : 2800);
}
const fail = e => toast(e.message || String(e), 'bad');

function modal(title, body, opts = {}) {
  const ov = h('div', { class: 'ov' });
  const close = () => { ov.remove(); document.removeEventListener('keydown', onkey, true); opts.onclose && opts.onclose(); };
  const onkey = ev => { if (ev.key === 'Escape' && !opts.sticky && ov === document.body.lastElementChild) { ev.stopPropagation(); close(); } };
  const m = h('div', { class: 'modal ' + (opts.cls || '') },
    h('div', { class: 'hd' }, h('span', { class: 'grow' }, title), opts.sticky ? null : h('button', { class: 'btn ghost sm', type: 'button', onclick: close, title: 'Close' }, ic('x'))),
    h('div', { class: 'bd' }, body),
    opts.footer ? h('div', { class: 'ft' }, opts.footer(close)) : null);
  ov.append(m);
  if (!opts.sticky) ov.addEventListener('mousedown', ev => { if (ev.target === ov) close(); });
  document.addEventListener('keydown', onkey, true);
  document.body.append(ov);
  const first = m.querySelector('input,select');
  if (first && !opts.nofocus) first.focus();
  return { close, el: m };
}
function confirmBox(title, text, okLabel, danger) {
  return new Promise(res => {
    let done = false;
    const fin = v => { if (!done) { done = true; res(v); } };
    modal(title, h('p', null, text), { onclose: () => fin(false), footer: close => [
      btn(null, 'Cancel', () => close(), ''),
      btn(null, okLabel, () => { fin(true); close(); }, danger ? 'primary danger' : 'primary')] });
  });
}
function promptBox(title, label, value = '', okLabel = 'OK') {
  return new Promise(res => {
    let done = false;
    const fin = v => { if (!done) { done = true; res(v); } };
    const inp = h('input', { type: 'text', value, maxlength: 255, autocomplete: 'off', spellcheck: 'false' });
    let mm;
    const ok = () => { const v = inp.value.trim(); if (v) { fin(v); mm.close(); } };
    inp.addEventListener('keydown', e => { if (e.key === 'Enter') ok(); });
    mm = modal(title, [h('label', { class: 'f' }, label), inp], { onclose: () => fin(null), footer: close => [btn(null, 'Cancel', () => close()), btn(null, okLabel, ok, 'primary')] });
    const dot = value.lastIndexOf('.');
    inp.setSelectionRange(0, dot > 0 ? dot : value.length);
  });
}

// ============ login ============
function showLogin(msg) {
  S.me = null;
  document.querySelectorAll('.ov,.up').forEach(e => e.remove());
  const err = h('div', { class: 'err' }, msg || '');
  const user = h('input', { type: 'text', autocomplete: 'username', autocapitalize: 'none', spellcheck: 'false', placeholder: 'Username' });
  const pass = h('input', { type: 'password', autocomplete: 'current-password', placeholder: 'Password' });
  const go = h('button', { class: 'btn primary', type: 'submit', style: null }, 'Sign in');
  const form = h('form', { onsubmit: async ev => {
    ev.preventDefault(); err.textContent = ''; go.disabled = true;
    try {
      const r = await api('POST', '/api/login', { username: user.value, password: pass.value });
      S.csrf = r.csrf; await boot();
    } catch (e) { err.textContent = e.message; pass.value = ''; pass.focus(); }
    go.disabled = false;
  } },
    h('div', { class: 'brand' }, h('img', { src: '/static/logo.svg', alt: '' }), 'DigitalSanDook'),
    h('p', { class: 'muted' }, 'Sign in to access your files'),
    h('label', { class: 'f' }, 'Username'), user,
    h('label', { class: 'f' }, 'Password'), pass,
    err,
    h('div', { class: 'row' }, go));
  go.classList.add('grow');
  app.replaceChildren(h('div', { class: 'login' }, form));
  user.focus();
}

// ============ bootstrap ============
async function boot() {
  try {
    const r = await api('GET', '/api/me');
    S.me = r.user; S.csrf = r.csrf; S.roots = r.roots; S.publicBase = r.publicBase || '';
  } catch (e) { if (e.status === 401 || e.status === 0) return showLogin(e.status === 0 ? e.message : ''); return fail(e); }
  buildShell();
  if (S.me.mustChange) { await changePassword(true); }
  route();
}

let el = {};
function buildShell() {
  el.search = h('input', { type: 'text', placeholder: 'Search in this folder (Enter)', spellcheck: 'false',
    onkeydown: ev => { if (ev.key === 'Enter') doSearch(); else if (ev.key === 'Escape') { el.search.value = ''; S.hits = null; render(); } } });
  el.side = h('aside');
  el.main = h('main');
  el.input = h('input', { type: 'file', multiple: true, class: 'hide', onchange: ev => { startUploads([...ev.target.files].map(f => ({ file: f, rel: '' }))); ev.target.value = ''; } });
  el.dirInput = h('input', { type: 'file', multiple: true, webkitdirectory: true, class: 'hide', onchange: ev => {
    startUploads([...ev.target.files].map(f => ({ file: f, rel: parentOf('/' + (f.webkitRelativePath || f.name)).slice(1) }))); ev.target.value = ''; } });
  const userBtn = h('button', { class: 'btn', type: 'button', onclick: userMenu }, ic('users'), h('span', { class: 'lbl' }, S.me.name));
  app.replaceChildren(h('div', { id: 'shell' },
    h('header', null,
      h('button', { class: 'btn ghost sidebtn', type: 'button', onclick: () => el.side.classList.toggle('open') }, ic('menu')),
      h('div', { class: 'brand' }, h('img', { src: '/static/logo.svg', alt: '' }), h('span', null, 'DigitalSanDook')),
      h('div', { class: 'search' }, ic('search'), el.search),
      h('div', { class: 'grow' }),
      themeBtn(),
      S.me.admin ? btn('shield', 'Admin', openAdmin) : null,
      userBtn),
    el.side, el.main, el.input, el.dirInput));
  setupDnD();
}

function userMenu(ev) {
  const r = ev.currentTarget.getBoundingClientRect();
  showMenu(r.right - 190, r.bottom + 4, [
    { icon: 'key', label: 'Change password', fn: () => changePassword(false) },
    { icon: 'logout', label: 'Sign out', fn: async () => { try { await api('POST', '/api/logout'); } catch (_) {} S.me = null; location.hash = ''; showLogin(); } },
  ]);
}

function changePassword(force) {
  return new Promise(res => {
    const old = h('input', { type: 'password', autocomplete: 'current-password' });
    const n1 = h('input', { type: 'password', autocomplete: 'new-password' });
    const n2 = h('input', { type: 'password', autocomplete: 'new-password' });
    const err = h('div', { class: 'err' });
    let mm;
    const submit = async () => {
      err.textContent = '';
      if (n1.value !== n2.value) { err.textContent = 'New passwords do not match'; return; }
      try { await api('POST', '/api/password', { old: old.value, new: n1.value }); S.me.mustChange = false; toast('Password changed'); mm.close(); res(); }
      catch (e) { err.textContent = e.message; }
    };
    mm = modal(force ? 'Set a new password' : 'Change password', [
      force ? h('p', { class: 'muted' }, 'You must choose a new password before continuing (minimum 10 characters).') : null,
      h('label', { class: 'f' }, 'Current password'), old,
      h('label', { class: 'f' }, 'New password (min 10 characters)'), n1,
      h('label', { class: 'f' }, 'Repeat new password'), n2, err],
    { sticky: force, onclose: () => res(), footer: close => [force ? null : btn(null, 'Cancel', () => close()), btn(null, 'Change password', submit, 'primary')] });
  });
}

// ============ context menu ============
function showMenu(x, y, items) {
  document.querySelectorAll('.menu').forEach(m => m.remove());
  const m = h('div', { class: 'menu' }, items.map(it => it === '-' ? h('hr') : h('div', { onclick: () => { m.remove(); it.fn(); } }, ic(it.icon), it.label)));
  document.body.append(m);
  const b = m.getBoundingClientRect();
  m.style.left = Math.max(4, Math.min(x, innerWidth - b.width - 4)) + 'px';
  m.style.top = Math.max(4, Math.min(y, innerHeight - b.height - 4)) + 'px';
  const off = ev => { if (!m.contains(ev.target)) { m.remove(); document.removeEventListener('mousedown', off, true); } };
  setTimeout(() => document.addEventListener('mousedown', off, true), 0);
}

// ============ routing ============
function parseHash() {
  const hsh = location.hash.replace(/^#\/?/, '');
  if (!hsh) return null;
  const parts = hsh.split('/').map(s => { try { return decodeURIComponent(s); } catch (_) { return s; } }).filter(Boolean);
  return { mount: parts[0], path: '/' + parts.slice(1).join('/') };
}
function goto(mount, path) {
  const hsh = '#/' + [mount, ...path.split('/').filter(Boolean)].map(encodeURIComponent).join('/');
  if (location.hash === hsh) route(); else location.hash = hsh;
}
window.addEventListener('hashchange', () => { if (S.me) route(); });

async function route() {
  const p = parseHash();
  S.sel.clear(); S.hits = null; S.limit = 500; if (el.search) el.search.value = '';
  el.side && el.side.classList.remove('open');
  if (!p) {
    renderSide();
    if (S.roots.length) return goto(S.roots[0].mount, S.roots[0].path);
    S.cur = { mount: '', path: '/' }; S.entries = []; return render();
  }
  S.cur = p;
  await refresh();
}
async function refresh() {
  if (!S.cur.mount) return render();
  try {
    S.loading = true;
    const d = await api('GET', '/api/list?' + q({ m: S.cur.mount, p: S.cur.path }));
    S.entries = d.entries; S.level = d.level; S.cur.path = d.path;
  } catch (e) { S.entries = []; S.level = 0; if (e.status !== 401) fail(e); }
  S.loading = false;
  for (const n of [...S.sel]) if (!S.entries.some(e => e.n === n)) S.sel.delete(n);
  render();
}
function curRoot() {
  let best = null;
  for (const r of S.roots) if (r.mount === S.cur.mount && (r.path === '/' || S.cur.path === r.path || S.cur.path.startsWith(r.path + '/'))) if (!best || r.path.length > best.path.length) best = r;
  return best;
}

// ============ rendering ============
function renderSide() {
  const cr = curRoot();
  const mine = S.roots.filter(r => !r.by), shared = S.roots.filter(r => r.by);
  const item = r => h('div', { class: 'root' + (cr === r ? ' on' : ''), title: r.label + (r.by ? ' (shared by ' + r.by + ')' : ''), onclick: () => goto(r.mount, r.path) },
    ic(r.by ? 'users' : 'drive'), h('span', null, r.label));
  el.side.replaceChildren();
  add(el.side, [
    h('h4', null, S.me.admin ? 'Shared folders' : 'My folders'), mine.length ? mine.map(item) : h('div', { class: 'muted' }, ' None yet'),
    shared.length ? [h('h4', null, 'Shared with me'), shared.map(item)] : null]);
}

function sortedEntries() {
  const { key, dir } = S.sort;
  const arr = S.entries.slice();
  arr.sort((a, b) => {
    if (a.d !== b.d) return a.d ? -1 : 1;
    let r = 0;
    if (key === 'size') r = a.s - b.s; else if (key === 'mtime') r = a.m - b.m;
    else r = a.n.toLowerCase() < b.n.toLowerCase() ? -1 : a.n.toLowerCase() > b.n.toLowerCase() ? 1 : 0;
    return r * dir || (a.n < b.n ? -1 : 1);
  });
  return arr;
}

function fileIcon(e) {
  if (e.d) return 'folder';
  const x = extOf(e.n);
  if ('png jpg jpeg gif webp bmp avif ico svg heic tiff'.split(' ').includes(x)) return 'image';
  if ('mp4 mkv mov webm avi m4v wmv'.split(' ').includes(x)) return 'film';
  if ('mp3 wav flac ogg m4a aac opus wma'.split(' ').includes(x)) return 'music';
  if ('zip rar 7z tar gz bz2 xz iso'.split(' ').includes(x)) return 'archive';
  if ('js ts go py c h cpp java json xml html css sh rs sql yml yaml toml'.split(' ').includes(x)) return 'code';
  if (x === 'pdf') return 'pdf';
  return 'file';
}

let rowsByName = new Map();
function render() {
  renderSide();
  const m = el.main;
  if (!S.cur.mount) {
    m.replaceChildren(h('div', { class: 'empty' }, ic('drive'), h('h3', null, 'No folders available'),
      h('p', null, S.me.admin ? 'Open the Admin panel and add a local folder to share.' : 'Ask an administrator to share a folder with you.'),
      S.me.admin ? btn('shield', 'Open Admin', openAdmin, 'primary') : null));
    return;
  }
  const canW = S.level >= 2 && !S.hits;
  const root = curRoot();
  // breadcrumbs
  const crumbs = h('div', { class: 'crumbs' });
  if (S.hits) crumbs.append(h('a', { onclick: () => { S.hits = null; el.search.value = ''; render(); } }, '← Back'), h('span', { class: 'sep' }, ' Search results'));
  else if (root) {
    crumbs.append(h('a', { onclick: () => goto(root.mount, root.path) }, root.label));
    const rest = S.cur.path.slice(root.path === '/' ? 1 : root.path.length + 1).split('/').filter(Boolean);
    let acc = root.path;
    for (const seg of rest) { acc = joinPath(acc, seg); const target = acc; crumbs.append(h('span', { class: 'sep' }, '/'), h('a', { onclick: () => goto(S.cur.mount, target) }, seg)); }
  }
  el.btns = {};
  const B = (k, icon, label, fn, cls) => (el.btns[k] = btn(icon, label, fn, cls));
  const bar = h('div', { class: 'bar' }, crumbs,
    canW ? [B('up', 'upload', 'Upload', () => el.input.click(), 'primary'),
      B('upd', 'fplus', 'Folder upload', () => el.dirInput.click()),
      B('mk', 'fplus', 'New folder', newFolder)] : null,
    B('dl', 'download', 'Download', downloadSel),
    canW ? [B('ren', 'edit', 'Rename', renameSel), B('cp', 'copy', 'Copy', () => setClip('copy')), B('cut', 'cut', 'Cut', () => setClip('move')),
      B('paste', 'paste', 'Paste', pasteHere), B('del', 'trash', 'Delete', deleteSel, 'danger')] : null,
    S.level >= 2 && !S.hits ? B('sh', 'share', 'Share', shareSel) : null,
    h('div', { class: 'seg' }, viewBtn('list', 'list', 'List view'), viewBtn('grid', 'grid', 'Grid view with thumbnails')),
    B('rf', 'refresh', '', () => { S.hits ? doSearch() : refresh(); }, 'ghost'));

  // items: table (list view, search results) or cards (grid view)
  const setSort = (key, dir) => { S.sort = { key, dir }; lsSet('dsd.sort', JSON.stringify(S.sort)); render(); };
  const th = (key, label, cls) => h('th', { class: cls || '', onclick: key ? () => setSort(key, S.sort.key === key ? -S.sort.dir : 1) : null },
    label + (S.sort.key === key ? (S.sort.dir > 0 ? ' ▲' : ' ▼') : ''));
  const all = h('input', { type: 'checkbox', title: 'Select all', onchange: ev => { if (ev.target.checked) S.entries.forEach(e => S.sel.add(e.n)); else S.sel.clear(); updateSel(); } });
  el.all = all;
  const grid = S.view === 'grid' && !S.hits;
  const items = grid ? h('div', { class: 'grid' }) : h('tbody');
  const list = S.hits ? S.hits.items : sortedEntries();
  rowsByName = new Map();
  const shown = list.slice(0, S.limit);
  for (const e of shown) items.append(rowFor(e));
  if (list.length > shown.length) {
    const more = h('button', { class: 'btn', type: 'button', onclick: () => { S.limit += 1000; render(); } }, 'Show more (' + (list.length - shown.length) + ' remaining)');
    items.append(grid ? h('div', { class: 'empty more' }, more) : h('tr', null, h('td', { colspan: 4, class: 'empty' }, more)));
  }
  let body;
  if (grid) {
    const sortSel = h('select', { title: 'Sort by', onchange: ev => { const [k, d] = ev.target.value.split(':'); setSort(k, +d); } },
      [['name:1', 'Name A-Z'], ['name:-1', 'Name Z-A'], ['mtime:-1', 'Newest first'], ['mtime:1', 'Oldest first'], ['size:-1', 'Largest first'], ['size:1', 'Smallest first']]
        .map(([v, t]) => h('option', { value: v, selected: v === S.sort.key + ':' + S.sort.dir }, t)));
    body = h('div', null, h('div', { class: 'gridhead' }, h('label', { class: 'row' }, all, 'Select all'), h('div', { class: 'grow' }), sortSel), items);
  } else {
    body = h('table', null, h('thead', null, h('tr', null, h('th', { class: 'ck' }, S.hits ? null : all), th('name', 'Name'), th('size', 'Size', 'col-m'), th('mtime', 'Modified', 'col-m'))), items);
  }
  if (S.loading && !list.length) body = h('div', { class: 'empty' }, 'Loading…');
  else if (!list.length) body = h('div', { class: 'empty' }, ic('folder'), h('p', null, S.hits ? 'No matches found' : (canW ? 'This folder is empty. Drag files here or click Upload.' : 'This folder is empty.')));
  el.list = h('div', { class: 'list', onclick: ev => { if (ev.target === el.list) { S.sel.clear(); updateSel(); } }, oncontextmenu: ev => {
    if (ev.target.closest('.r')) return; ev.preventDefault();
    showMenu(ev.clientX, ev.clientY, [canW && { icon: 'fplus', label: 'New folder', fn: newFolder }, canW && { icon: 'upload', label: 'Upload files', fn: () => el.input.click() }, canW && S.clip && { icon: 'paste', label: 'Paste', fn: pasteHere }, { icon: 'refresh', label: 'Refresh', fn: refresh }].filter(Boolean));
  } }, body);
  // keep the scroll position when the same folder re-renders (e.g. during uploads)
  const where = S.cur.mount + '\n' + S.cur.path + '\n' + S.view + '\n' + !!S.hits;
  const keep = el.where === where && el.listOld ? el.listOld.scrollTop : 0;
  el.status = h('div', { class: 'status' });
  m.replaceChildren(bar, el.list, el.status);
  el.list.scrollTop = keep; el.listOld = el.list; el.where = where;
  updateSel();
}
function viewBtn(v, icon, title) {
  return h('button', { class: 'btn' + (S.view === v ? ' on' : ''), type: 'button', title, onclick: () => { S.view = v; lsSet('dsd.view', v); render(); } }, ic(icon));
}

function rowFor(e) {
  const name = e.n;
  const path = S.hits ? e.p : joinPath(S.cur.path, name);
  const key = S.hits ? e.p : name;
  const cb = h('input', { type: 'checkbox', onclick: ev => { ev.stopPropagation(); toggleSel(key, ev.shiftKey); } });
  const kind = fileIcon(e);
  const draggable = S.level >= 2 && !S.hits ? 'true' : null;
  let tr;
  if (S.view === 'grid' && !S.hits) {
    const thumb = h('div', { class: 'th t-' + kind }, ic(kind));
    if (!e.d && THUMBS.has(extOf(name)) && e.s <= 64 * 1024 * 1024) {
      // the icon stays underneath until the lazily loaded thumbnail covers it
      const img = h('img', { loading: 'lazy', decoding: 'async', alt: '', draggable: 'false', src: '/api/thumb?' + q({ m: S.cur.mount, p: path, v: e.m + '-' + e.s }) });
      img.addEventListener('error', () => img.remove());
      thumb.append(img);
    }
    tr = h('div', { class: 'card r', draggable, title: name + (e.d ? '' : '\n' + fmtSize(e.s) + ' · ' + fmtDate(e.m)) },
      cb, thumb, h('div', { class: 'nm' }, name), h('div', { class: 'sz' }, e.d ? 'Folder' : fmtSize(e.s)));
  } else {
    tr = h('tr', { class: 'r', draggable },
      h('td', { class: 'ck' }, cb),
      h('td', { class: 'nm' }, h('div', { class: 'c' }, h('span', { class: 'fi t-' + kind }, ic(kind)), h('span', { title: S.hits ? e.p : name }, name),
        S.hits ? h('span', { class: 'muted mono' }, parentOf(e.p)) : null)),
      h('td', { class: 'col-m muted' }, e.d ? '—' : fmtSize(e.s)),
      h('td', { class: 'col-m muted' }, fmtDate(e.m)));
  }
  tr._cb = cb; tr._e = e; tr._key = key;
  tr.addEventListener('click', ev => {
    if (ev.ctrlKey || ev.metaKey) toggleSel(key); else if (ev.shiftKey) rangeSel(key);
    else { S.sel.clear(); S.sel.add(key); S.last = key; updateSel(); }
  });
  tr.addEventListener('dblclick', () => openEntry(e, path));
  tr.addEventListener('contextmenu', ev => {
    ev.preventDefault();
    if (!S.sel.has(key)) { S.sel.clear(); S.sel.add(key); updateSel(); }
    const w = S.level >= 2 && !S.hits;
    showMenu(ev.clientX, ev.clientY, [
      { icon: e.d ? 'folder' : 'eye', label: e.d ? 'Open' : 'Open / preview', fn: () => openEntry(e, path) },
      { icon: 'download', label: 'Download', fn: downloadSel },
      w && '-', w && { icon: 'edit', label: 'Rename', fn: renameSel }, w && { icon: 'copy', label: 'Copy', fn: () => setClip('copy') },
      w && { icon: 'cut', label: 'Cut', fn: () => setClip('move') }, w && { icon: 'share', label: 'Share…', fn: shareSel },
      w && '-', w && { icon: 'trash', label: 'Delete', fn: deleteSel }].filter(Boolean));
  });
  if (!S.hits && S.level >= 2) {
    tr.addEventListener('dragstart', ev => {
      if (!S.sel.has(key)) { S.sel.clear(); S.sel.add(key); updateSel(); }
      S.drag = [...S.sel]; ev.dataTransfer.effectAllowed = 'move'; ev.dataTransfer.setData('text/x-dsd', '1');
    });
    if (e.d) {
      tr.addEventListener('dragover', ev => { if (S.drag && !S.drag.includes(name)) { ev.preventDefault(); tr.classList.add('drop'); } });
      tr.addEventListener('dragleave', () => tr.classList.remove('drop'));
      tr.addEventListener('drop', async ev => {
        tr.classList.remove('drop');
        if (!S.drag || S.drag.includes(name)) return;
        ev.preventDefault(); ev.stopPropagation();
        const names = S.drag; S.drag = null;
        try { await api('POST', '/api/paste', { mount: S.cur.mount, from: S.cur.path, to: path, op: 'move', names }); toast('Moved ' + names.length + ' item(s)'); refresh(); } catch (er) { fail(er); }
      });
    }
    tr.addEventListener('dragend', () => { S.drag = null; });
  }
  rowsByName.set(key, tr);
  return tr;
}

function toggleSel(key, range) {
  if (range) return rangeSel(key);
  if (S.sel.has(key)) S.sel.delete(key); else S.sel.add(key);
  S.last = key; updateSel();
}
function rangeSel(key) {
  const keys = [...rowsByName.keys()];
  const a = keys.indexOf(S.last == null ? key : S.last), b = keys.indexOf(key);
  if (a < 0 || b < 0) return;
  if (!event || !(event.ctrlKey || event.metaKey)) S.sel.clear();
  for (let i = Math.min(a, b); i <= Math.max(a, b); i++) S.sel.add(keys[i]);
  updateSel();
}
function updateSel() {
  for (const [k, tr] of rowsByName) { const on = S.sel.has(k); tr.classList.toggle('sel', on); tr._cb.checked = on; }
  const n = S.sel.size;
  if (el.all) { el.all.checked = n > 0 && n === S.entries.length; el.all.indeterminate = n > 0 && n < S.entries.length; }
  const b = el.btns || {};
  const dis = (k, v) => { if (b[k]) b[k].disabled = v; };
  dis('dl', n === 0 && !!S.hits); dis('ren', n !== 1); dis('cp', n === 0); dis('cut', n === 0); dis('del', n === 0);
  dis('paste', !S.clip || S.clip.mount !== S.cur.mount);
  if (b.dl) b.dl.title = n ? 'Download selected' : 'Download this folder as ZIP';
  if (el.status && document.body.contains(el.status)) {
    const all = S.hits ? S.hits.items : S.entries;
    const dirs = all.filter(e => e.d).length, bytes = all.reduce((s, e) => s + (e.d ? 0 : e.s), 0);
    const sel = selectedEntries(), selBytes = sel.reduce((s, e) => s + (e.d ? 0 : e.s), 0);
    const plural = (k, w) => k + ' ' + w + (k === 1 ? '' : 's');
    el.status.replaceChildren(
      h('span', null, plural(dirs, 'folder') + ', ' + plural(all.length - dirs, 'file') + ' · ' + fmtSize(bytes)),
      sel.length ? h('span', null, h('b', null, sel.length + ' selected'), selBytes ? ' · ' + fmtSize(selBytes) : '') : document.createTextNode(''),
      h('span', { class: 'grow' }),
      S.clip ? h('span', { class: 'opt' }, (S.clip.op === 'move' ? 'Cut: ' : 'Copied: ') + plural(S.clip.names.length, 'item')) : document.createTextNode(''),
      S.level ? h('span', { class: 'opt' }, 'Your access: ' + LEVELS[Math.min(S.level, 3)]) : document.createTextNode(''));
  }
}
const selectedEntries = () => [...S.sel].map(k => rowsByName.get(k)?._e).filter(Boolean);

// ============ actions ============
function openEntry(e, path) {
  if (e.d) { S.hits = null; goto(S.cur.mount, path); return; }
  const kind = previewKind(e.n);
  if (kind) preview(e, path, kind); else downloadPaths([path], [e]);
}
function dlLink(path, inline) { return '/api/dl?' + q({ m: S.cur.mount, p: path, inline: inline ? 1 : null }); }
function trigger(url) { const a = h('a', { href: url, download: '' }); document.body.append(a); a.click(); a.remove(); }
function downloadPaths(paths, ents) {
  if (paths.length === 1 && !ents[0].d) return trigger(dlLink(paths[0]));
  if (S.hits) { toast('Open the folder to download several items at once', 'bad'); return; }
  const names = ents.map(e => e.n);
  trigger('/api/zip?' + q({ m: S.cur.mount, p: S.cur.path }) + names.map(n => '&n=' + encodeURIComponent(n)).join(''));
}
function downloadSel() {
  const ents = selectedEntries();
  if (!ents.length) { // whole folder
    trigger('/api/zip?' + q({ m: S.cur.mount, p: S.cur.path })); return;
  }
  downloadPaths(ents.map(e => S.hits ? e.p : joinPath(S.cur.path, e.n)), ents);
}
async function newFolder() {
  const n = await promptBox('New folder', 'Folder name', '', 'Create');
  if (!n) return;
  try { await api('POST', '/api/mkdir', { mount: S.cur.mount, path: S.cur.path, name: n }); refresh(); } catch (e) { fail(e); }
}
async function renameSel() {
  const [e] = selectedEntries(); if (!e) return;
  const n = await promptBox('Rename', 'New name', e.n, 'Rename');
  if (!n || n === e.n) return;
  try { await api('POST', '/api/rename', { mount: S.cur.mount, path: S.cur.path, from: e.n, to: n }); S.sel.clear(); refresh(); } catch (er) { fail(er); }
}
async function deleteSel() {
  const ents = selectedEntries(); if (!ents.length) return;
  const what = ents.length === 1 ? '"' + ents[0].n + '"' : ents.length + ' items';
  if (!await confirmBox('Delete', 'Permanently delete ' + what + '? This cannot be undone.', 'Delete', true)) return;
  try { await api('POST', '/api/delete', { mount: S.cur.mount, path: S.cur.path, names: ents.map(e => e.n) }); S.sel.clear(); toast('Deleted'); refresh(); } catch (e) { fail(e); refresh(); }
}
function setClip(op) {
  const ents = selectedEntries(); if (!ents.length) return;
  S.clip = { op, mount: S.cur.mount, dir: S.cur.path, names: ents.map(e => e.n) };
  toast((op === 'move' ? 'Cut ' : 'Copied ') + ents.length + ' item(s) - open the destination and press Paste');
  updateSel();
}
async function pasteHere() {
  const c = S.clip; if (!c) return;
  if (c.mount !== S.cur.mount) { toast('Copy/move works inside the same shared folder', 'bad'); return; }
  try {
    await api('POST', '/api/paste', { mount: c.mount, from: c.dir, to: S.cur.path, op: c.op, names: c.names });
    if (c.op === 'move') S.clip = null;
    toast('Done'); refresh();
  } catch (e) { fail(e); }
}
function shareSel() {
  const ents = selectedEntries();
  if (ents.length > 1) { toast('Select a single item to share (or none to share this folder)', 'bad'); return; }
  const path = ents.length ? joinPath(S.cur.path, ents[0].n) : S.cur.path;
  shareDialog(S.cur.mount, path, ents.length ? ents[0].n : (curRoot()?.label || S.cur.mount));
}
async function doSearch() {
  const term = el.search.value.trim();
  if (term.length < 2) return toast('Type at least 2 characters');
  try {
    const r = await api('GET', '/api/search?' + q({ m: S.cur.mount, p: S.cur.path, q: term }));
    S.hits = { items: r.hits.map(h => ({ n: h.n, d: h.d, s: h.s, m: h.m, p: h.p })) };
    S.sel.clear(); render();
    if (r.truncated) toast('Showing the first results only - refine your search');
  } catch (e) { fail(e); }
}

// ============ preview ============
function previewKind(name) {
  const x = extOf(name);
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'avif', 'ico'].includes(x)) return 'img';
  if (['mp4', 'webm', 'ogv', 'mov', 'm4v'].includes(x)) return 'video';
  if (['mp3', 'wav', 'ogg', 'flac', 'm4a', 'aac', 'opus'].includes(x)) return 'audio';
  if (x === 'pdf') return 'pdf';
  if (['txt', 'md', 'log', 'csv', 'json', 'xml', 'yml', 'yaml', 'toml', 'ini', 'conf', 'go', 'py', 'js', 'ts', 'c', 'h', 'cpp', 'java', 'sh', 'bat', 'rs', 'sql'].includes(x)) return 'text';
  return null;
}
// Preview with gallery navigation (buttons, arrow keys) across the previewable
// files of the current folder, in the order they are displayed.
function preview(e, path, kind) {
  const items = S.hits ? [{ e, path }] : sortedEntries().filter(x => !x.d && previewKind(x.n)).map(x => ({ e: x, path: joinPath(S.cur.path, x.n) }));
  let idx = Math.max(0, items.findIndex(x => x.path === path));
  if (!items.length) items.push({ e, path });
  const stage = h('div', { class: 'pv' });
  const nav = (dir, icon, label) => h('button', { class: 'nav ' + (dir < 0 ? 'l' : 'r'), type: 'button', title: label, onclick: () => show(idx + dir) }, ic(icon));
  let title;
  const show = i => {
    idx = (i + items.length) % items.length;
    const { e: it, path: p } = items[idx];
    const k = previewKind(it.n), url = dlLink(p, true);
    let node;
    if (k === 'img') node = h('img', { src: url, alt: it.n });
    else if (k === 'video') node = h('video', { src: url, controls: true, autoplay: true });
    else if (k === 'audio') node = h('audio', { src: url, controls: true, autoplay: true });
    else if (k === 'pdf') node = h('iframe', { src: url, title: it.n });
    else {
      node = h('pre', { class: 'mono' }, 'Loading…');
      fetch(url, { credentials: 'same-origin', headers: { Range: 'bytes=0-1048575' } }).then(r => r.text())
        .then(t => { node.textContent = t + (it.s > 1048576 ? '\n\n… (truncated preview)' : ''); }).catch(() => { node.textContent = 'Could not load preview'; });
    }
    stage.replaceChildren(node);
    if (items.length > 1) stage.append(nav(-1, 'chevL', 'Previous (←)'), nav(1, 'chevR', 'Next (→)'));
    if (title) title.textContent = it.n + (items.length > 1 ? '   ' + (idx + 1) + ' / ' + items.length : '');
  };
  const onkey = ev => {
    if (isField(ev.target) || items.length < 2) return;
    if (ev.key === 'ArrowLeft') { ev.preventDefault(); show(idx - 1); } else if (ev.key === 'ArrowRight') { ev.preventDefault(); show(idx + 1); }
  };
  const m = modal(e.n, stage, { cls: 'full', nofocus: true, onclose: () => document.removeEventListener('keydown', onkey),
    footer: close => [btn('download', 'Download', () => trigger(dlLink(items[idx].path))), btn(null, 'Close', () => close(), 'primary')] });
  m.el.querySelector('.bd').style.padding = '0'; m.el.querySelector('.bd').style.display = 'flex';
  title = m.el.querySelector('.hd span');
  document.addEventListener('keydown', onkey);
  show(idx);
}

// ============ drag & drop upload ============
function setupDnD() {
  let depth = 0, ov = null;
  const hasFiles = ev => ev.dataTransfer && [...ev.dataTransfer.types].includes('Files');
  window.addEventListener('dragenter', ev => { if (!hasFiles(ev) || S.level < 2 || S.hits) return; ev.preventDefault(); if (depth++ === 0 && !ov) { ov = h('div', { class: 'drop-ov' }, 'Drop to upload to this folder'); el.main.append(ov); } });
  window.addEventListener('dragover', ev => { if (hasFiles(ev)) ev.preventDefault(); });
  window.addEventListener('dragleave', ev => { if (!hasFiles(ev)) return; if (--depth <= 0) { depth = 0; ov && ov.remove(); ov = null; } });
  window.addEventListener('drop', async ev => {
    if (!hasFiles(ev)) return;
    ev.preventDefault(); depth = 0; ov && ov.remove(); ov = null;
    if (!S.me || S.level < 2 || S.hits) { toast('You cannot upload here', 'bad'); return; }
    const entries = [...ev.dataTransfer.items].map(i => i.webkitGetAsEntry ? i.webkitGetAsEntry() : null);
    const plain = [...ev.dataTransfer.files];
    const out = [];
    const walk = async (en, parent) => {
      if (en.isFile) { const f = await new Promise((ok, no) => en.file(ok, no)); out.push({ file: f, rel: parent }); return; }
      const rel = parent ? parent + '/' + en.name : en.name;
      const rd = en.createReader(); const kids = [];
      for (;;) { const b = await new Promise((ok, no) => rd.readEntries(ok, no)); if (!b.length) break; kids.push(...b); }
      for (const k of kids) await walk(k, rel);
    };
    try {
      if (entries.every(Boolean) && entries.length) for (const en of entries) await walk(en, '');
      else plain.forEach(f => out.push({ file: f, rel: '' }));
    } catch (_) { plain.forEach(f => out.push({ file: f, rel: '' })); }
    startUploads(out);
  });
}

// ============ upload engine: parallel chunks, retries, speed meter ============
const SMALL = 8 * 1024 * 1024, MAXFILES = 3, MAXSLOTS = 6;
const U = { items: [], q: [], qi: 0, running: 0, slots: 0, waiters: [], speed: 0, lastBytes: 0, timer: null, panel: null, rf: null };
const acquire = () => new Promise(res => { if (U.slots < MAXSLOTS) { U.slots++; res(); } else U.waiters.push(res); });
const release = () => { const n = U.waiters.shift(); if (n) n(); else U.slots--; };
const sleep = ms => new Promise(r => setTimeout(r, ms));

function startUploads(list) {
  if (!list.length) return;
  const conflict = lsGet('dsd.overwrite') === '1' ? 'overwrite' : 'rename';
  for (const { file, rel } of list) {
    const it = { file, rel, name: file.name, size: file.size, sent: 0, state: 'queued', mount: S.cur.mount, path: S.cur.path, conflict, xhrs: new Set(), cancel: false, err: '' };
    U.items.push(it); U.q.push(it);
  }
  ensurePanel(); pumpUploads();
  if (!U.timer) U.timer = setInterval(tickUploads, 300);
}
function pumpUploads() {
  while (U.running < MAXFILES) {
    // FIFO cursor instead of scanning the whole list: O(1) per file even for 10k+ files
    while (U.qi < U.q.length && U.q[U.qi].state !== 'queued') U.qi++;
    if (U.qi >= U.q.length) { U.q = []; U.qi = 0; break; }
    const it = U.q[U.qi++];
    U.running++; it.state = 'active';
    runItem(it).finally(() => { U.running--; pumpUploads(); });
  }
}
async function runItem(it) {
  try {
    if (it.size <= SMALL) await putSmall(it); else await putChunked(it);
    it.state = 'done'; it.sent = it.size;
  } catch (e) {
    it.state = it.cancel ? 'cancelled' : 'error'; it.err = e.message || 'failed';
  }
  if (it.state === 'done' && it.mount === S.cur.mount && it.path === S.cur.path && !S.hits) scheduleRefresh();
}
// at most one listing refresh every 1.5 s while a bulk upload is running
function scheduleRefresh() {
  if (U.rf) return;
  U.rf = setTimeout(() => { U.rf = null; refresh(); }, U.running > 0 ? 1500 : 300);
}
function xhrPut(url, body, it, onprog) {
  return new Promise((res, rej) => {
    const x = new XMLHttpRequest();
    x.open('PUT', url);
    x.setRequestHeader('X-CSRF-Token', S.csrf);
    x.upload.onprogress = e => onprog(e.loaded);
    x.onload = () => { it.xhrs.delete(x); if (x.status < 300) return res(x.responseText); let m = 'HTTP ' + x.status; try { m = JSON.parse(x.responseText).error || m; } catch (_) {} const er = new ApiError(m, x.status); rej(er); };
    x.onerror = () => { it.xhrs.delete(x); rej(new ApiError('network error', 0)); };
    x.onabort = () => { it.xhrs.delete(x); rej(new ApiError('cancelled', -1)); };
    it.xhrs.add(x); x.send(body);
  });
}
async function retrying(it, fn) {
  for (let a = 0; ; a++) {
    if (it.cancel) throw new ApiError('cancelled', -1);
    try { return await fn(); } catch (e) {
      const permanent = e.status >= 400 && e.status < 500 && e.status !== 408 && e.status !== 429;
      if (permanent || e.status === -1 || a >= 5) throw e;
      await sleep(400 * 2 ** a);
    }
  }
}
async function putSmall(it) {
  await acquire();
  try {
    await retrying(it, async () => {
      const prev = it.sent; it.sent = 0;
      try {
        await xhrPut('/api/put?' + q({ m: it.mount, p: it.path, rel: it.rel, name: it.name, conflict: it.conflict, mtime: it.file.lastModified }), it.file, it, l => { it.sent = l; });
      } catch (e) { it.sent = prev; throw e; }
    });
  } finally { release(); }
}
async function putChunked(it) {
  const init = await api('POST', '/api/upload', { mount: it.mount, path: it.path, rel: it.rel, name: it.name, size: it.size, mtime: it.file.lastModified, conflict: it.conflict });
  it.uid = init.id;
  const { chunk, n } = init;
  const loaded = new Float64Array(n);
  let next = 0, failure = null;
  const worker = async () => {
    while (!it.cancel && !failure && next < n) {
      const i = next++;
      const start = i * chunk, end = Math.min(it.size, start + chunk);
      await acquire();
      try {
        await retrying(it, async () => {
          it.sent -= loaded[i]; loaded[i] = 0;
          await xhrPut('/api/upload/' + init.id + '/' + i, it.file.slice(start, end), it, l => { it.sent += l - loaded[i]; loaded[i] = l; });
        });
      } catch (e) { failure = e; } finally { release(); }
    }
  };
  await Promise.all(Array.from({ length: Math.min(n, MAXSLOTS) }, worker));
  if (it.cancel) { api('DELETE', '/api/upload/' + init.id).catch(() => {}); throw new ApiError('cancelled', -1); }
  if (failure) { api('DELETE', '/api/upload/' + init.id).catch(() => {}); throw failure; }
  await api('POST', '/api/upload/' + init.id + '/finish');
}
function cancelItem(it) {
  it.cancel = true;
  if (it.state === 'queued') it.state = 'cancelled';
  for (const x of it.xhrs) x.abort();
}
function ensurePanel() {
  if (U.panel && document.body.contains(U.panel)) return;
  U.head = h('span', { class: 'grow' }, 'Uploads');
  U.list = h('div', { class: 'items' });
  const ow = h('input', { type: 'checkbox', checked: lsGet('dsd.overwrite') === '1', onchange: ev => lsSet('dsd.overwrite', ev.target.checked ? '1' : '0') });
  U.panel = h('div', { class: 'up' },
    h('div', { class: 'hd' }, U.head,
      h('button', { class: 'btn sm', type: 'button', onclick: () => { U.items.filter(i => ['queued', 'active'].includes(i.state)).forEach(cancelItem); } }, 'Cancel all'),
      h('button', { class: 'btn sm ghost', type: 'button', title: 'Hide finished', onclick: () => { U.items = U.items.filter(i => ['queued', 'active'].includes(i.state)); if (!U.items.length) U.panel.remove(); else tickUploads(); } }, ic('x'))),
    h('label', { class: 'row muted', style: null }, ow, ' Overwrite files with the same name (otherwise keep both)'),
    U.list);
  document.body.append(U.panel);
}
function tickUploads() {
  if (!U.panel) return;
  const total = U.items.reduce((s, i) => s + i.sent, 0);
  U.speed = U.speed * 0.6 + Math.max(0, (total - U.lastBytes) / 0.3) * 0.4; U.lastBytes = total;
  const active = U.items.filter(i => ['queued', 'active'].includes(i.state));
  U.head.textContent = active.length ? 'Uploading ' + active.length + ' file' + (active.length > 1 ? 's' : '') + ' · ' + fmtSize(U.speed) + '/s' : 'Uploads complete';
  const shown = U.items.slice(-60);
  U.list.replaceChildren(...shown.map(i => {
    const pct = i.size ? Math.min(100, i.sent / i.size * 100) : (i.state === 'done' ? 100 : 0);
    const bar = h('div', { class: 'bar2 ' + (i.state === 'done' ? 'done' : i.state === 'error' ? 'bad' : '') }, h('i'));
    bar.firstChild.style.width = pct + '%';
    const status = i.state === 'done' ? 'Done' : i.state === 'error' ? i.err : i.state === 'cancelled' ? 'Cancelled' : i.state === 'queued' ? 'Waiting' : pct.toFixed(0) + '%';
    return h('div', { class: 'it' }, h('div', { class: 't' }, h('span', { title: (i.rel ? i.rel + '/' : '') + i.name }, (i.rel ? i.rel + '/' : '') + i.name), h('span', { class: 'muted' }, status)), bar);
  }));
  if (!active.length && U.timer) { clearInterval(U.timer); U.timer = null; }
}
window.addEventListener('beforeunload', ev => { if (U.items.some(i => i.state === 'active' || i.state === 'queued')) { ev.preventDefault(); ev.returnValue = ''; } });

// ============ sharing dialog ============
async function shareDialog(mount, path, label) {
  const holder = h('div');
  let tab = 'people';
  const tabs = h('div', { class: 'tabs' });
  const draw = () => {
    tabs.replaceChildren(...[['people', 'People'], ['link', 'Public link']].map(([k, t]) => h('div', { class: 'tab' + (tab === k ? ' on' : ''), onclick: () => { tab = k; draw(); } }, t)));
    holder.replaceChildren(h('div', { class: 'muted' }, 'Loading…'));
    (tab === 'people' ? peopleTab : linkTab)(mount, path).then(n => holder.replaceChildren(n)).catch(e => holder.replaceChildren(h('div', { class: 'err' }, e.message)));
  };
  const m = modal('Share "' + label + '"', [tabs, h('div', { style: null }, holder)], { cls: 'wide' });
  m.el.querySelector('.bd').style.padding = '0'; holder.style.padding = '16px 18px';
  draw();
}
async function peopleTab(mount, path) {
  const [g, users] = await Promise.all([api('GET', '/api/grants?' + q({ m: mount, p: path })), api('GET', '/api/users')]);
  const my = g.level;
  const box = h('div');
  const reload = async () => { try { box.replaceWith(await peopleTab(mount, path)); } catch (e) { fail(e); } };
  const levelSel = (val, onchange) => h('select', { onchange, style: null }, [1, 2, 3].filter(l => l <= my).map(l => h('option', { value: l, selected: l === val }, LEVEL_HELP[l])));
  const rows = g.direct.map(x => h('tr', null, h('td', null, ic('users'), ' ', x.user), h('td', null, levelSel(x.level, async ev => {
    try { await api('POST', '/api/grants', { mount, path, user: x.user, level: +ev.target.value }); toast('Updated'); } catch (e) { fail(e); reload(); } })),
    h('td', { class: 'muted' }, 'by ' + x.by), h('td', null, btn('x', 'Revoke', async () => { try { await api('DELETE', '/api/grants/' + x.id); reload(); } catch (e) { fail(e); } }, 'sm danger'))));
  const who = h('select', null, h('option', { value: '' }, 'Choose a person…'), users.map(u => h('option', { value: u }, u)));
  const lvl = levelSel(1, null); lvl.value = '1';
  const addBtn = btn('plus', 'Share', async () => {
    if (!who.value) return toast('Choose a person first');
    try { await api('POST', '/api/grants', { mount, path, user: who.value, level: +lvl.value }); toast('Shared with ' + who.value); reload(); } catch (e) { fail(e); }
  }, 'primary');
  box.append(
    h('div', { class: 'row' }, h('div', { class: 'grow' }, who), h('div', { class: 'grow' }, lvl), addBtn),
    h('h4', { class: 'muted' }, 'People with access to this item'),
    rows.length ? h('table', { class: 'tbl' }, h('tbody', null, rows)) : h('p', { class: 'muted' }, 'Not shared with anyone directly yet.'),
    g.inherited.length ? [h('h4', { class: 'muted' }, 'Inherited from a parent folder'), h('table', { class: 'tbl' }, h('tbody', null, g.inherited.map(x => h('tr', null,
      h('td', null, x.user), h('td', null, h('span', { class: 'pill' }, LEVELS[x.level])), h('td', { class: 'muted mono' }, x.path)))))] : null,
    h('p', { class: 'muted' }, 'You can only give others the same or lower access than you have yourself.'));
  return box;
}
// Base URL for public links: the Cloudflare tunnel / configured public URL when there is one.
async function linkBase() {
  try { S.publicBase = (await api('GET', '/api/me')).publicBase || ''; } catch (_) {}
  return S.publicBase || location.origin;
}
async function linkTab(mount, path) {
  const base = await linkBase();
  const links = await api('GET', '/api/links?' + q({ m: mount, p: path }));
  const box = h('div');
  const reload = async () => { try { box.replaceWith(await linkTab(mount, path)); } catch (e) { fail(e); } };
  const days = h('select', null, [[1, '1 day'], [7, '7 days'], [30, '30 days'], [90, '90 days'], [0, 'Never expires']].map(([v, t]) => h('option', { value: v, selected: v === 7 }, t)));
  const pw = h('input', { type: 'password', placeholder: 'Optional password', autocomplete: 'new-password' });
  const mk = btn('link', 'Create link', async () => {
    try { const l = await api('POST', '/api/links', { mount, path, days: +days.value, password: pw.value }); try { await navigator.clipboard.writeText(await linkBase() + '/s/' + l.token); toast('Link created and copied'); } catch (_) { toast('Link created'); } reload(); } catch (e) { fail(e); }
  }, 'primary');
  box.append(h('p', { class: 'muted' }, 'Anyone with the link can view and download (read-only), even without an account.' +
      (S.publicBase ? '' : ' These links only work for people who can reach this server directly.')),
    h('div', { class: 'grid2' }, h('div', null, h('label', { class: 'f' }, 'Expires'), days), h('div', null, h('label', { class: 'f' }, 'Password'), pw)),
    h('div', { class: 'row', style: null }, mk),
    h('h4', { class: 'muted' }, 'Active links'),
    links.length ? links.map(l => {
      const url = base + '/s/' + l.token;
      return h('div', { class: 'copybox' }, h('input', { type: 'text', value: url, readonly: true, onfocus: ev => ev.target.select() }),
        h('span', { class: 'badge' }, l.protected ? 'password' : 'open'), h('span', { class: 'badge' }, l.expires ? 'expires ' + new Date(l.expires * 1000).toLocaleDateString() : 'no expiry'),
        btn('copy', 'Copy', async () => { try { await navigator.clipboard.writeText(url); toast('Copied'); } catch (_) { toast('Select the link and copy it', 'bad'); } }, 'sm'),
        btn('trash', '', async () => { try { await api('DELETE', '/api/links/' + l.token); reload(); } catch (e) { fail(e); } }, 'sm danger'));
    }) : h('p', { class: 'muted' }, 'No links yet.'));
  box.querySelectorAll('.row').forEach(r => { r.style.marginTop = '12px'; });
  box.querySelectorAll('.copybox').forEach(r => { r.style.marginBottom = '6px'; });
  return box;
}

// ============ admin panel ============
function openAdmin() {
  let tab = 'users';
  const body = h('div');
  const tabs = h('div', { class: 'tabs' });
  const T = { users: ['Users', adminUsers], mounts: ['Shared folders', adminMounts], access: ['Access', adminAccess], links: ['Public links', adminLinks], tunnel: ['Internet access', adminTunnel], sessions: ['Sessions', adminSessions], audit: ['Audit log', adminAudit] };
  const draw = () => {
    tabs.replaceChildren(...Object.entries(T).map(([k, [t]]) => h('div', { class: 'tab' + (tab === k ? ' on' : ''), onclick: () => { tab = k; draw(); } }, t)));
    body.replaceChildren(h('div', { class: 'muted' }, 'Loading…'));
    T[tab][1]().then(n => body.replaceChildren(n)).catch(e => body.replaceChildren(h('div', { class: 'err' }, e.message)));
  };
  const m = modal('Administration', [tabs, body], { cls: 'wide', nofocus: true, onclose: async () => { try { const r = await api('GET', '/api/me'); S.roots = r.roots; renderSide(); } catch (_) {} } });
  m.el.querySelector('.bd').style.padding = '0'; body.style.padding = '16px 18px';
  draw();
  return m;
}
const tbl = (head, rows) => h('div', { style: null }, h('table', { class: 'tbl' }, h('thead', null, h('tr', null, head.map(t => h('th', null, t)))), h('tbody', null, rows)));
const reloadTab = (fn, node) => fn().then(n => node.replaceWith(n)).catch(fail);

async function adminUsers() {
  const users = await api('GET', '/api/admin/users');
  const box = h('div');
  const reload = () => reloadTab(adminUsers, box);
  const showPw = (name, pw) => modal('Password for ' + name, [h('p', null, 'Share this one-time password securely. The user must change it at first login.'), h('input', { type: 'text', value: pw, readonly: true, class: 'mono', onfocus: ev => ev.target.select() })], { footer: c => [btn(null, 'Done', () => c(), 'primary')] });
  const rows = users.map(u => h('tr', null,
    h('td', null, h('b', null, u.name)), h('td', null, h('span', { class: 'badge ' + (u.admin ? 'warn' : '') }, u.admin ? 'admin' : 'user'), ' ', u.disabled ? h('span', { class: 'badge bad' }, 'disabled') : null),
    h('td', { class: 'muted' }, ago(u.lastLogin)),
    h('td', null, h('div', { class: 'row' },
      btn('key', 'Reset password', async () => { if (!await confirmBox('Reset password', 'Generate a new password for ' + u.name + '? They will be signed out everywhere.', 'Reset')) return; try { const r = await api('PATCH', '/api/admin/users/' + u.name, { generate: true }); showPw(u.name, r.password); } catch (e) { fail(e); } }, 'sm'),
      btn('shield', u.admin ? 'Remove admin' : 'Make admin', async () => { try { await api('PATCH', '/api/admin/users/' + u.name, { admin: !u.admin }); reload(); } catch (e) { fail(e); } }, 'sm'),
      btn('lock', u.disabled ? 'Enable' : 'Disable', async () => { try { await api('PATCH', '/api/admin/users/' + u.name, { disabled: !u.disabled }); reload(); } catch (e) { fail(e); } }, 'sm'),
      btn('trash', '', async () => { if (!await confirmBox('Delete user', 'Delete ' + u.name + ' and all access they were given?', 'Delete', true)) return; try { await api('DELETE', '/api/admin/users/' + u.name); reload(); } catch (e) { fail(e); } }, 'sm danger')))));
  const nm = h('input', { type: 'text', placeholder: 'username', autocomplete: 'off', autocapitalize: 'none' });
  const pw = h('input', { type: 'text', placeholder: 'password (empty = generate)', autocomplete: 'off' });
  const adm = h('input', { type: 'checkbox' });
  box.append(
    h('div', { class: 'row' }, h('div', { class: 'grow' }, nm), h('div', { class: 'grow' }, pw), h('label', { class: 'row' }, adm, ' admin'),
      btn('plus', 'Add user', async () => {
        try { const r = await api('POST', '/api/admin/users', { name: nm.value, password: pw.value, admin: adm.checked }); if (r.password) showPw(r.name, r.password); else toast('User created'); reload(); } catch (e) { fail(e); }
      }, 'primary')),
    h('div', { style: null }, ' '),
    tbl(['User', 'Role', 'Last login', ''], rows));
  return box;
}

async function adminMounts() {
  const ms = await api('GET', '/api/admin/mounts');
  const box = h('div');
  const reload = () => reloadTab(adminMounts, box);
  const rows = ms.map(m => h('tr', null, h('td', null, h('b', null, m.name)), h('td', { class: 'mono wrap' }, m.path),
    h('td', null, m.online ? h('span', { class: 'badge ok' }, 'online') : h('span', { class: 'badge bad' }, 'offline'), ' ', m.readOnly ? h('span', { class: 'badge warn' }, 'read-only') : null),
    h('td', null, h('div', { class: 'row' },
      btn('lock', m.readOnly ? 'Make writable' : 'Make read-only', async () => { try { await api('POST', '/api/admin/mounts', { name: m.name, path: m.path, readOnly: !m.readOnly }); reload(); } catch (e) { fail(e); } }, 'sm'),
      btn('trash', 'Stop sharing', async () => { if (!await confirmBox('Stop sharing', 'Stop sharing "' + m.name + '"? Files stay on disk; all access and links to it are removed.', 'Stop sharing', true)) return; try { await api('DELETE', '/api/admin/mounts/' + m.name); reload(); } catch (e) { fail(e); } }, 'sm danger')))));
  const name = h('input', { type: 'text', placeholder: 'Name (e.g. Projects)', autocomplete: 'off' });
  const path = h('input', { type: 'text', placeholder: 'Local folder path on this server', autocomplete: 'off', class: 'mono' });
  const ro = h('input', { type: 'checkbox' });
  box.append(
    h('p', { class: 'muted' }, 'Choose a folder on the machine running DigitalSanDook. Users never see real paths - only the name below.'),
    h('div', { class: 'row' }, h('div', { style: null, class: 'grow' }, name), h('div', { class: 'grow' }, path), btn('folder', 'Browse…', () => browseDialog(path, name), '')),
    h('div', { class: 'row', style: null }, h('label', { class: 'row' }, ro, ' read-only (nobody can change files here)'), h('div', { class: 'grow' }),
      btn('plus', 'Share this folder', async () => {
        try { await api('POST', '/api/admin/mounts', { name: name.value, path: path.value, readOnly: ro.checked }); toast('Folder shared'); reload(); } catch (e) { fail(e); }
      }, 'primary')),
    tbl(['Name', 'Local path', 'Status', ''], rows));
  box.querySelectorAll('.row').forEach(r => { r.style.marginBottom = '10px'; });
  return box;
}
function browseDialog(pathInput, nameInput) {
  const list = h('div'); const crumb = h('div', { class: 'mono muted' });
  let curPath = pathInput.value || '';
  const load = async p => {
    try {
      const r = await api('GET', '/api/admin/browse?' + q({ path: p }));
      curPath = r.path; crumb.textContent = r.path || 'This computer';
      list.replaceChildren();
      add(list, [
        r.path ? h('div', { class: 'root', onclick: () => load(r.parent) }, ic('up'), h('span', null, '..')) : null,
        r.dirs.map(d => h('div', { class: 'root', onclick: () => load(r.root ? d : (r.path.endsWith('/') || r.path.endsWith('\\') ? r.path : r.path + (r.path.includes('\\') ? '\\' : '/')) + d) }, ic('folder'), h('span', null, d)))]);
    } catch (e) { fail(e); }
  };
  const m = modal('Choose a folder to share', [crumb, h('div', { style: null }, list)], { footer: close => [btn(null, 'Cancel', () => close()), btn('check', 'Use this folder', () => {
    if (!curPath) return toast('Open a folder first', 'bad');
    pathInput.value = curPath;
    if (!nameInput.value) nameInput.value = (curPath.split(/[\\/]/).filter(Boolean).pop() || 'share').replace(/[^A-Za-z0-9_-]/g, '_').slice(0, 32);
    close();
  }, 'primary')] });
  list.parentElement.style.maxHeight = '50vh'; list.parentElement.style.overflow = 'auto';
  load(pathInput.value.trim() ? curPath : '');
}

async function adminAccess() {
  const [gs, ms, us] = await Promise.all([api('GET', '/api/admin/grants'), api('GET', '/api/admin/mounts'), api('GET', '/api/admin/users')]);
  const box = h('div');
  const reload = () => reloadTab(adminAccess, box);
  const rows = gs.map(g => h('tr', null, h('td', null, h('b', null, g.user)), h('td', null, g.mount), h('td', { class: 'mono' }, g.path),
    h('td', null, h('span', { class: 'pill' }, LEVELS[g.level])), h('td', { class: 'muted' }, 'by ' + g.by),
    h('td', null, btn('x', 'Revoke', async () => { try { await api('DELETE', '/api/grants/' + g.id); reload(); } catch (e) { fail(e); } }, 'sm danger'))));
  const who = h('select', null, h('option', { value: '' }, 'User…'), us.filter(u => !u.admin).map(u => h('option', { value: u.name }, u.name)));
  const mnt = h('select', null, h('option', { value: '' }, 'Folder…'), ms.map(m => h('option', { value: m.name }, m.name)));
  const sub = h('input', { type: 'text', placeholder: 'Sub-path (optional, e.g. /Reports)', class: 'mono', autocomplete: 'off' });
  const lvl = h('select', null, [1, 2, 3].map(l => h('option', { value: l, selected: l === 2 }, LEVEL_HELP[l])));
  box.append(
    h('p', { class: 'muted' }, 'Admins always have full access. Give other users access to a whole shared folder or to a sub-folder. Users with Editor access can further share what they have.'),
    h('div', { class: 'grid2' }, who, mnt), h('div', { class: 'grid2', style: null }, sub, lvl),
    h('div', { class: 'row', style: null }, h('div', { class: 'grow' }), btn('plus', 'Grant access', async () => {
      if (!who.value || !mnt.value) return toast('Choose a user and a folder', 'bad');
      try { await api('POST', '/api/grants', { mount: mnt.value, path: sub.value.trim() || '/', user: who.value, level: +lvl.value }); toast('Access granted'); reload(); } catch (e) { fail(e); }
    }, 'primary')),
    tbl(['User', 'Folder', 'Path', 'Level', 'Granted', ''], rows));
  box.querySelectorAll('.grid2,.row').forEach(r => { r.style.marginBottom = '10px'; });
  return box;
}

async function adminLinks() {
  const ls = await api('GET', '/api/admin/links');
  const box = h('div');
  const reload = () => reloadTab(adminLinks, box);
  box.append(tbl(['Owner', 'Folder', 'Path', 'Expires', 'Hits', ''], ls.map(l => h('tr', null, h('td', null, l.owner), h('td', null, l.mount), h('td', { class: 'mono' }, l.path),
    h('td', { class: 'muted' }, l.expires ? new Date(l.expires * 1000).toLocaleString() : 'never'), h('td', null, l.hits),
    h('td', null, btn('trash', 'Revoke', async () => { try { await api('DELETE', '/api/links/' + l.token); reload(); } catch (e) { fail(e); } }, 'sm danger'))))));
  return box;
}

async function adminTunnel() {
  const st = await api('GET', '/api/admin/tunnel');
  const box = h('div');
  const reload = () => reloadTab(adminTunnel, box);
  const mode = h('select', null,
    h('option', { value: '', selected: st.mode === '' }, 'Off - links only work on your own network'),
    h('option', { value: 'quick', selected: st.mode === 'quick' }, 'Quick tunnel - free, no account, temporary random address'),
    h('option', { value: 'token', selected: st.mode === 'token' }, 'Named tunnel - your own domain, permanent address (recommended)'));
  const token = h('input', { type: 'password', autocomplete: 'off', placeholder: st.hasToken ? 'Token saved - leave empty to keep it' : 'Tunnel token from the Cloudflare Zero Trust dashboard' });
  const pub = h('input', { type: 'text', value: st.publicUrl, placeholder: 'https://files.example.com (named tunnels)', autocomplete: 'off', class: 'mono' });
  const port = h('input', { type: 'number', value: st.port, min: 1024, max: 65535 });
  const stateBadge = { running: 'ok', starting: 'warn', error: 'bad', off: '' }[st.state];
  const save = btn('check', 'Save and apply', async () => {
    save.disabled = true;
    try {
      await api('POST', '/api/admin/tunnel', { mode: mode.value, token: token.value, keepToken: !token.value && st.hasToken, publicUrl: pub.value, port: +port.value });
      toast('Saved'); reload();
    } catch (e) { fail(e); save.disabled = false; }
  }, 'primary');
  box.append(
    h('p', { class: 'muted' }, 'Publish your share links to the internet through a Cloudflare Tunnel - no port forwarding, no public IP. Only public share links are exposed; the login page, file manager and admin panel are never reachable through the tunnel.'),
    h('div', { class: 'row' }, h('b', null, 'Status'), h('span', { class: 'badge ' + stateBadge }, st.state),
      st.url ? h('input', { type: 'text', readonly: true, value: st.url, class: 'mono', onfocus: ev => ev.target.select() }) : null),
    st.message ? h('div', { class: 'err' }, st.message) : null,
    h('div', { class: 'row muted' }, 'cloudflared: ', st.binary ? h('span', { class: 'mono' }, st.binary) : h('b', { class: 'err' }, 'not found - install it from developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads or put the executable next to DigitalSanDook')),
    h('label', { class: 'f' }, 'Mode'), mode,
    h('label', { class: 'f' }, 'Named tunnel token'), token,
    h('label', { class: 'f' }, 'Public URL (shown in links; required for named tunnels)'), pub,
    h('label', { class: 'f' }, 'Local tunnel port - in the Cloudflare dashboard point the public hostname at http://localhost:' + st.port), port,
    h('p', { class: 'muted' }, 'Quick tunnels get a new address every time they start, so links created earlier stop working after a restart. Use a named tunnel for permanent links.'),
    h('div', { class: 'row' }, h('div', { class: 'grow' }), btn('refresh', 'Refresh status', reload), save));
  box.querySelectorAll('.row').forEach(r => { r.style.margin = '10px 0'; });
  return box;
}

async function adminSessions() {
  const ss = await api('GET', '/api/admin/sessions');
  const box = h('div');
  const reload = () => reloadTab(adminSessions, box);
  box.append(tbl(['User', 'IP', 'Device', 'Last active', ''], ss.map(s => h('tr', null, h('td', null, h('b', null, s.user), s.current ? ' (you)' : ''), h('td', { class: 'mono' }, s.ip),
    h('td', { class: 'muted wrap' }, s.ua.slice(0, 60)), h('td', { class: 'muted' }, ago(s.last)),
    h('td', null, s.current ? null : btn('x', 'Sign out', async () => { try { await api('DELETE', '/api/admin/sessions/' + s.id); toast('Session ended'); reload(); } catch (e) { fail(e); } }, 'sm danger'))))));
  return box;
}

async function adminAudit() {
  const [log, st] = await Promise.all([api('GET', '/api/admin/audit?limit=400'), api('GET', '/api/admin/status')]);
  const up = st.uptime;
  return h('div', null,
    h('p', { class: 'muted' }, 'DigitalSanDook ' + st.version + ' on ' + st.platform + ' · up ' + Math.floor(up / 3600) + 'h ' + Math.floor(up % 3600 / 60) + 'm · ' + (st.tls ? 'TLS on' : 'plain HTTP') + ' · ' + st.sessions + ' session(s) · ' + st.uploads + ' upload(s) in progress'),
    tbl(['Time', 'User', 'IP', 'Action', 'Detail'], log.map(e => h('tr', null, h('td', { class: 'muted' }, new Date(e.t).toLocaleString()), h('td', null, e.user), h('td', { class: 'mono' }, e.ip),
      h('td', null, h('span', { class: 'badge ' + (e.action.includes('fail') ? 'bad' : '') }, e.action)), h('td', { class: 'wrap mono' }, e.detail)))));
}

// ============ keyboard ============
document.addEventListener('keydown', ev => {
  if (!S.me || document.querySelector('.ov') || isField(ev.target)) return;
  const mod = ev.ctrlKey || ev.metaKey;
  // file operations address items by name inside the current folder; search
  // results come from sub-folders, so (like the toolbar) they are disabled there
  const canW = S.level >= 2 && !S.hits;
  if (mod && ev.key.toLowerCase() === 'a') { ev.preventDefault(); for (const k of rowsByName.keys()) S.sel.add(k); updateSel(); }
  else if (mod && ev.key.toLowerCase() === 'c' && S.sel.size && !S.hits) { setClip('copy'); }
  else if (mod && ev.key.toLowerCase() === 'x' && S.sel.size && canW) { setClip('move'); }
  else if (mod && ev.key.toLowerCase() === 'v' && canW) { pasteHere(); }
  else if (ev.key === 'Delete' && S.sel.size && canW) deleteSel();
  else if (ev.key === 'F2' && S.sel.size === 1 && canW) renameSel();
  else if (ev.key === 'Enter' && S.sel.size === 1) { const [e] = selectedEntries(); if (e) openEntry(e, S.hits ? e.p : joinPath(S.cur.path, e.n)); }
  else if (ev.key === 'Backspace') { const r = curRoot(); if (r && S.cur.path !== r.path) goto(S.cur.mount, parentOf(S.cur.path)); }
  else if (ev.key === 'Escape') { S.sel.clear(); updateSel(); }
});

boot();
})();
