'use strict';
(() => {

// ═══════════════════════════════════════════════════════════════════════════
//  utilities
// ═══════════════════════════════════════════════════════════════════════════

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ESC[c]);
const reEsc = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const clamp = (v, a = 0, b = 100) => Math.max(a, Math.min(b, v || 0));
const store = {
  get(k, d) { try { const v = localStorage.getItem('iseall.' + k); return v === null ? d : JSON.parse(v); } catch { return d; } },
  set(k, v) { try { localStorage.setItem('iseall.' + k, JSON.stringify(v)); } catch { } },
};

function fmtB(b, d = 1) {
  b = +b || 0;
  if (b < 1024) return Math.round(b) + ' B';
  const u = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let i = -1;
  do { b /= 1024; i++; } while (b >= 1024 && i < u.length - 1);
  return (b >= 100 ? b.toFixed(0) : b.toFixed(d)) + ' ' + u[i];
}
const fmtRate = b => fmtB(b) + '/s';
function fmtDur(s) {
  s = Math.max(0, Math.floor(s));
  const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600), m = Math.floor(s % 3600 / 60), x = s % 60;
  if (d) return `${d}d ${h}h`;
  if (h) return `${h}h ${m}m`;
  if (m) return `${m}m ${x}s`;
  return `${x}s`;
}
const ago = ts => ts ? fmtDur(Date.now() / 1000 - ts) + ' ago' : '—';
const pad2 = n => String(n).padStart(2, '0');
const clock = ms => { const d = new Date(ms); return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`; };
const lvl = (p, w = 70, c = 90) => p >= c ? 'crit' : p >= w ? 'warn' : '';
const bar = (p, cls = '', extra = '') => `<div class="bar ${cls} ${extra}"><i style="width:${clamp(p).toFixed(1)}%"></i></div>`;
const plural = (n, w) => `${n} ${w}${n === 1 ? '' : 's'}`;
const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };
const download = (name, text) => { const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([text], { type: 'text/plain' })); a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(a.href), 1000); };

// lucide-style 24px stroke icons
const IC = {
  grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  cpu: '<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3"/>',
  box: '<path d="M21 16V8a2 2 0 0 0-1-1.7l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.7l7 4a2 2 0 0 0 2 0l7-4a2 2 0 0 0 1-1.7z"/><path d="M3.3 7 12 12l8.7-5M12 22V12"/>',
  layers: '<path d="M12 2 2 7l10 5 10-5-10-5z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>',
  power: '<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.8 0"/>',
  plug: '<path d="M9 2v6M15 2v6"/><path d="M6 8h12v4a6 6 0 0 1-12 0z"/><path d="M12 18v4"/>',
  wifi: '<path d="M2 9a15 15 0 0 1 20 0"/><path d="M5 12.5a10 10 0 0 1 14 0"/><path d="M8.5 16a5 5 0 0 1 7 0"/><circle cx="12" cy="19.5" r="1"/>',
  disk: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5"/><path d="M3 12c0 1.7 4 3 9 3s9-1.3 9-3"/>',
  list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
  term: '<rect x="2" y="3" width="20" height="18" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/>',
  shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
  bell: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>',
  moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
  play: '<path d="M7 4.5v15l12-7.5z"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="2"/>',
  restart: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/>',
  pause: '<rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>',
  trash: '<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
  more: '<circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
  ext: '<path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  info: '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/>',
  alert: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  okc: '<circle cx="12" cy="12" r="10"/><path d="m8 12 3 3 5-6"/>',
  zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
  clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
  down: '<path d="M12 5v14M19 12l-7 7-7-7"/>',
  up: '<path d="M12 19V5M5 12l7-7 7 7"/>',
  download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
  menu: '<path d="M3 6h18M3 12h18M3 18h18"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
  globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20"/>',
  lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  cal: '<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>',
  rocket: '<path d="M4.5 16.5c-1.5 1.3-2 5-2 5s3.7-.5 5-2c.7-.8.7-2.1-.1-2.9a2.2 2.2 0 0 0-2.9-.1z"/><path d="m12 15-3-3a22 22 0 0 1 2-3.9A12.9 12.9 0 0 1 22 2c0 2.7-.8 7.5-6 11a22.4 22.4 0 0 1-4 2z"/><path d="M9 12H4s.6-3 2-4c1.6-1.1 5 0 5 0M12 15v5s3-.6 4-2c1.1-1.6 0-5 0-5"/>',
  activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>',
  pkg: '<path d="m7.5 4.3 9 5.2M21 8v8l-9 5-9-5V8l9-5z"/><path d="M3.3 7 12 12l8.7-5M12 22V12"/>',
};
const ic = (n, cls = '') => `<svg class="i ${cls}" viewBox="0 0 24 24">${IC[n] || ''}</svg>`;

// ═══════════════════════════════════════════════════════════════════════════
//  api
// ═══════════════════════════════════════════════════════════════════════════

async function api(path, opts = {}) {
  const o = { method: opts.method || 'GET', headers: { 'X-Iseall': '1' }, credentials: 'same-origin' };
  if (opts.body !== undefined) {
    o.method = opts.method || 'POST';
    o.headers['Content-Type'] = 'application/json';
    o.body = JSON.stringify(opts.body);
  }
  const r = await fetch(path, o);
  if (r.status === 401 && !path.startsWith('/api/auth')) { showLogin(); throw new Error('Session expired'); }
  const ct = r.headers.get('content-type') || '';
  const data = ct.includes('json') ? await r.json() : await r.text();
  if (!r.ok) throw new Error((data && data.error) || r.statusText || 'Request failed');
  return data;
}
const post = (p, body = {}) => api(p, { method: 'POST', body });

async function act(fn, okMsg) {
  try {
    const r = await fn();
    if (okMsg) toast(okMsg, 'ok');
    return r;
  } catch (e) {
    toast(e.message, 'crit', 6000);
    throw e;
  }
}

// streaming text response (docker compose output) → callback per chunk
async function streamText(path, body, onChunk) {
  const r = await fetch(path, { method: 'POST', headers: { 'X-Iseall': '1', 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!r.ok) { const j = await r.json().catch(() => ({})); throw new Error(j.error || r.statusText); }
  const rd = r.body.getReader();
  const dec = new TextDecoder();
  for (;;) {
    const { value, done } = await rd.read();
    if (done) break;
    onChunk(dec.decode(value, { stream: true }));
  }
}

// ═══════════════════════════════════════════════════════════════════════════
//  overlays: toast, modal, drawer, menu, confirm
// ═══════════════════════════════════════════════════════════════════════════

function toast(msg, kind = 'ok', ms = 3500) {
  const t = document.createElement('div');
  t.className = 'toast';
  t.innerHTML = `<span class="dot ${kind}"></span><div>${esc(msg)}</div>`;
  $('#toasts').appendChild(t);
  const kill = () => { t.classList.add('out'); setTimeout(() => t.remove(), 260); };
  setTimeout(kill, ms);
  t.onclick = kill;
}

const overlays = [];
function openOverlay(cls, html, onClose) {
  const scrim = document.createElement('div');
  scrim.className = 'scrim';
  const el = document.createElement('div');
  el.className = cls;
  el.innerHTML = html;
  document.body.append(scrim, el);
  document.documentElement.classList.add('locked');
  const o = {
    el, closed: false,
    close() {
      if (o.closed) return;
      o.closed = true;
      scrim.remove(); el.remove();
      overlays.splice(overlays.indexOf(o), 1);
      if (!overlays.length) document.documentElement.classList.remove('locked');
      onClose && onClose();
    },
  };
  scrim.onclick = () => o.close();
  overlays.push(o);
  return o;
}

function drawer({ title, sub = '', head = '', body = '', onClose }) {
  const o = openOverlay('drawer', `
    <div class="drawer-h"><div class="grow" style="min-width:0"><h2>${title}</h2><div class="sub">${sub}</div></div>
      <div class="row">${head}<button class="ib" data-x title="Close (Esc)">${ic('x')}</button></div></div>
    <div class="drawer-b">${body}</div>`, onClose);
  $('[data-x]', o.el).onclick = () => o.close();
  o.body = $('.drawer-b', o.el);
  return o;
}

// modal resolves with onOk's result (true by default) or false when dismissed
function modal({ title, body = '', ok = 'OK', okCls = 'pri', cancel = 'Cancel', wide = false, onOk, onOpen }) {
  return new Promise(res => {
    let settled = false;
    const finish = v => { if (!settled) { settled = true; res(v); } };
    const o = openOverlay('modal' + (wide ? ' wide' : ''), `
      <div class="modal-b"><h2>${title}</h2>${body}</div>
      <div class="modal-f">${cancel ? `<button class="btn ghost" data-c>${esc(cancel)}</button>` : ''}${ok ? `<button class="btn ${okCls}" data-ok>${esc(ok)}</button>` : ''}</div>`,
      () => finish(false));
    const okBtn = $('[data-ok]', o.el);
    const cBtn = $('[data-c]', o.el);
    if (cBtn) cBtn.onclick = () => o.close();
    const submit = async () => {
      if (!okBtn || okBtn.disabled) return;
      if (!onOk) { finish(true); o.close(); return; }
      okBtn.disabled = true;
      const orig = okBtn.innerHTML;
      okBtn.innerHTML = `<span class="spin"></span>${orig}`;
      try {
        const v = await onOk(o.el);
        if (v === false) { okBtn.disabled = false; okBtn.innerHTML = orig; return; }
        finish(v === undefined ? true : v);
        o.close();
      } catch (e) {
        toast(e.message, 'crit', 6000);
        okBtn.disabled = false; okBtn.innerHTML = orig;
      }
    };
    if (okBtn) okBtn.onclick = submit;
    o.el.addEventListener('keydown', e => { if (e.key === 'Enter' && e.target.tagName === 'INPUT') { e.preventDefault(); submit(); } });
    onOpen && onOpen(o.el);
    setTimeout(() => { const f = $('input:not([type=checkbox]),select,textarea', o.el); f && f.focus(); }, 30);
  });
}

function confirmBox(title, msg, { ok = 'Confirm', danger = false, typed = '' } = {}) {
  const body = `<p>${msg}</p>` + (typed ? `<div class="field"><label>Type <b class="mono">${esc(typed)}</b> to confirm</label><input class="in mono" data-typed autocomplete="off"></div>` : '');
  return modal({
    title, body, ok, okCls: danger ? 'danger' : 'pri',
    onOk: el => {
      const t = $('[data-typed]', el);
      if (typed && t.value.trim() !== typed) { t.classList.remove('shake'); void t.offsetWidth; t.classList.add('shake'); return false; }
      return true;
    },
  });
}

let menuEl = null;
function closeMenu() { if (menuEl) { menuEl.remove(); menuEl = null; } }
function showMenu(anchor, items) {
  closeMenu();
  const m = document.createElement('div');
  m.className = 'menu';
  m.innerHTML = items.map((it, i) => it === '-' ? '<hr>' : `<button data-i="${i}" class="${it.cls || ''}">${ic(it.icon || 'check')}${esc(it.label)}</button>`).join('');
  document.body.appendChild(m);
  const r = anchor.getBoundingClientRect();
  const mw = m.offsetWidth, mh = m.offsetHeight;
  m.style.left = Math.max(8, Math.min(r.right - mw, innerWidth - mw - 8)) + 'px';
  m.style.top = (r.bottom + mh + 8 > innerHeight ? Math.max(8, r.top - mh - 6) : r.bottom + 6) + 'px';
  m.onclick = e => { const b = e.target.closest('[data-i]'); if (b) { e.stopPropagation(); closeMenu(); items[+b.dataset.i].fn(); } };
  menuEl = m;
  setTimeout(() => document.addEventListener('click', closeMenu, { once: true }), 0);
}

// ═══════════════════════════════════════════════════════════════════════════
//  charts — tiny canvas renderer, repaints only when data arrives
// ═══════════════════════════════════════════════════════════════════════════

const COL = {};
function readColors() {
  const cs = getComputedStyle(document.documentElement);
  for (const k of ['--acc', '--acc-2', '--acc-3', '--ok', '--warn', '--crit', '--info', '--line', '--mute', '--dim', '--text', '--panel']) COL[k] = cs.getPropertyValue(k).trim();
  COL.font = '10.5px ' + cs.getPropertyValue('--font');
}
function rgba(hex, a) {
  if (!hex || hex[0] !== '#') return hex;
  let h = hex.slice(1);
  if (h.length === 3) h = h.split('').map(c => c + c).join('');
  const n = parseInt(h, 16);
  return `rgba(${n >> 16 & 255},${n >> 8 & 255},${n & 255},${a})`;
}
function niceMax(v) {
  if (v <= 0) return 1;
  const e = Math.pow(10, Math.floor(Math.log10(v)));
  const f = v / e;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * e;
}

const charts = new Set();
class LineChart {
  constructor(el, opts) {
    this.o = Object.assign({ max: 100, window: 300e3, fmt: v => v.toFixed(1) + '%', fmtAxis: v => Math.round(v) + '%', axis: true }, opts);
    this.el = el;
    el.classList.add('chart');
    el.innerHTML = '<canvas></canvas><div class="tip"></div>';
    this.cv = $('canvas', el);
    this.tip = $('.tip', el);
    this.ctx = this.cv.getContext('2d');
    this.t = []; this.ys = [];
    this.hover = -1;
    this.ro = new ResizeObserver(() => this.draw());
    this.ro.observe(el);
    el.addEventListener('mousemove', e => this.onMove(e));
    el.addEventListener('mouseleave', () => { this.hover = -1; this.tip.classList.remove('on'); this.draw(); });
    charts.add(this);
  }
  set(t, ys) { this.t = t; this.ys = ys; this.draw(); }
  destroy() { this.ro.disconnect(); charts.delete(this); }
  draw() {
    if (this.raf) return;
    this.raf = requestAnimationFrame(() => { this.raf = 0; this.paint(); });
  }
  geom() {
    const W = this.el.clientWidth, H = this.el.clientHeight;
    const padT = 6, padB = this.o.axis ? 18 : 2;
    const tEnd = this.t.length ? this.t[this.t.length - 1] : Date.now();
    return { W, H, padT, padB, ch: H - padT - padB, tEnd, tStart: tEnd - this.o.window };
  }
  onMove(e) {
    if (!this.t.length) return;
    const g = this.geom();
    const r = this.el.getBoundingClientRect();
    const tAt = g.tStart + (e.clientX - r.left) / g.W * this.o.window;
    let best = 0, bd = Infinity;
    for (let i = 0; i < this.t.length; i++) { const d = Math.abs(this.t[i] - tAt); if (d < bd) { bd = d; best = i; } }
    this.hover = best;
    this.tip.innerHTML = `<div class="mute" style="margin-bottom:3px">${clock(this.t[best])}</div>` +
      this.o.series.map((se, k) => `<div><i style="background:${COL[se.c] || se.c}"></i>${esc(se.label)} <b>${(se.fmt || this.o.fmt)((this.ys[k] || [])[best] ?? 0)}</b></div>`).join('');
    this.tip.classList.add('on');
    const px = (this.t[best] - g.tStart) / this.o.window * g.W;
    const tw = this.tip.offsetWidth;
    this.tip.style.left = (px + 14 + tw > g.W ? px - tw - 14 : px + 14) + 'px';
    this.draw();
  }
  paint() {
    const { W, H, padT, ch, tStart } = this.geom();
    if (!W || !H) return;
    const dpr = Math.min(devicePixelRatio || 1, 2);
    if (this.cv.width !== Math.round(W * dpr) || this.cv.height !== Math.round(H * dpr)) {
      this.cv.width = Math.round(W * dpr); this.cv.height = Math.round(H * dpr);
    }
    const c = this.ctx;
    c.setTransform(dpr, 0, 0, dpr, 0, 0);
    c.clearRect(0, 0, W, H);
    let max = this.o.max;
    if (!max) {
      let m = 0;
      for (const y of this.ys) for (const v of y) if (v > m) m = v;
      max = niceMax(Math.max(m * 1.15, this.o.min || 1));
    }
    const win = this.o.window;
    const X = t => (t - tStart) / win * W;
    const Y = v => padT + ch * (1 - Math.min(Math.max(v, 0), max) / max);
    c.strokeStyle = COL['--line']; c.lineWidth = 1; c.setLineDash([2, 4]);
    c.font = COL.font; c.fillStyle = COL['--dim'];
    for (let i = 0; i <= 3; i++) {
      const y = Math.round(padT + ch * i / 4) + .5;
      c.beginPath(); c.moveTo(0, y); c.lineTo(W, y); c.stroke();
      if (this.o.axis && i < 3) { c.textAlign = 'right'; c.fillText(this.o.fmtAxis(max * (1 - i / 4)), W - 4, y + 12); }
    }
    c.setLineDash([]);
    if (this.o.axis) {
      c.textAlign = 'left'; c.fillText(`-${Math.round(win / 60e3)}m`, 4, H - 4);
      c.textAlign = 'right'; c.fillText('now', W - 4, H - 4);
    }
    if (!this.t.length) return;
    this.o.series.forEach((se, k) => {
      const col = COL[se.c] || se.c;
      const ys = this.ys[k] || [];
      const segs = [];
      let seg = [];
      for (let i = 0; i < this.t.length; i++) {
        if (this.t[i] < tStart - 5000) continue;
        if (seg.length && this.t[i] - this.t[i - 1] > 15000) { segs.push(seg); seg = []; } // gap: idle sampling or outage
        seg.push([X(this.t[i]), Y(ys[i] || 0)]);
      }
      if (seg.length) segs.push(seg);
      for (const s of segs) {
        if (s.length > 1) {
          const gr = c.createLinearGradient(0, padT, 0, padT + ch);
          gr.addColorStop(0, rgba(col, .26)); gr.addColorStop(1, rgba(col, 0));
          c.beginPath(); c.moveTo(s[0][0], padT + ch);
          for (const [x, y] of s) c.lineTo(x, y);
          c.lineTo(s[s.length - 1][0], padT + ch); c.closePath();
          c.fillStyle = gr; c.fill();
        }
        c.beginPath();
        s.forEach(([x, y], i) => i ? c.lineTo(x, y) : c.moveTo(x, y));
        c.strokeStyle = col; c.lineWidth = 1.7; c.lineJoin = 'round';
        c.stroke();
      }
      const last = segs.length && segs[segs.length - 1];
      if (last && last.length) { // glowing live head
        const [x, y] = last[last.length - 1];
        c.beginPath(); c.arc(x - 2, y, 6.5, 0, 7); c.fillStyle = rgba(col, .18); c.fill();
        c.beginPath(); c.arc(x - 2, y, 3, 0, 7); c.fillStyle = col; c.fill();
      }
    });
    if (this.hover >= 0 && this.hover < this.t.length) {
      const x = Math.round(X(this.t[this.hover])) + .5;
      c.strokeStyle = COL['--mute']; c.globalAlpha = .5; c.beginPath(); c.moveTo(x, padT); c.lineTo(x, padT + ch); c.stroke(); c.globalAlpha = 1;
      this.o.series.forEach((se, k) => {
        c.beginPath(); c.arc(x, Y((this.ys[k] || [])[this.hover] || 0), 3.5, 0, 7);
        c.fillStyle = COL[se.c] || se.c; c.fill(); c.strokeStyle = COL['--panel']; c.lineWidth = 2; c.stroke();
      });
    }
  }
}

function spark(cv, vals, colVar = '--acc', max = 0) {
  if (!cv) return;
  const W = cv.clientWidth, H = cv.clientHeight;
  if (!W) return;
  const dpr = Math.min(devicePixelRatio || 1, 2);
  cv.width = W * dpr; cv.height = H * dpr;
  const c = cv.getContext('2d');
  c.setTransform(dpr, 0, 0, dpr, 0, 0);
  if (!vals || vals.length < 2) return;
  const m = max || Math.max(...vals, 1e-9) * 1.15;
  const col = COL[colVar] || colVar;
  const n = vals.length;
  const pts = vals.map((v, i) => [i / (n - 1) * W, H - 1 - Math.min(v, m) / m * (H - 3)]);
  const g = c.createLinearGradient(0, 0, 0, H);
  g.addColorStop(0, rgba(col, .3)); g.addColorStop(1, rgba(col, 0));
  c.beginPath(); c.moveTo(0, H);
  pts.forEach(([x, y]) => c.lineTo(x, y));
  c.lineTo(W, H); c.fillStyle = g; c.fill();
  c.beginPath(); pts.forEach(([x, y], i) => i ? c.lineTo(x, y) : c.moveTo(x, y));
  c.strokeStyle = col; c.lineWidth = 1.5; c.stroke();
}

// radial gauge (270° arc)
const G_R = 42, G_C = 2 * Math.PI * G_R, G_ARC = G_C * .75;
function gaugeSVG(id) {
  return `<div class="gauge" id="${id}"><svg viewBox="0 0 100 100">
    <defs><linearGradient id="${id}-g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="var(--acc)"/><stop offset="1" stop-color="var(--acc-2)"/></linearGradient></defs>
    <circle class="trk" cx="50" cy="50" r="${G_R}" stroke-dasharray="${G_ARC} ${G_C}"/>
    <circle class="val" cx="50" cy="50" r="${G_R}" stroke="url(#${id}-g)" stroke-dasharray="${G_ARC} ${G_C}" stroke-dashoffset="${G_ARC}"/>
  </svg><div class="ctr"><div><b>—</b><span></span></div></div></div>`;
}
function setGauge(id, frac, text, sub, level) {
  const g = document.getElementById(id);
  if (!g) return;
  const v = $('.val', g);
  v.setAttribute('stroke-dashoffset', (G_ARC * (1 - clamp(frac, 0, 1))).toFixed(1));
  v.setAttribute('stroke', level ? `var(--${level})` : `url(#${id}-g)`);
  if (!document.body.classList.contains('lite')) v.style.filter = `drop-shadow(0 0 6px ${rgba(level ? COL['--' + level] : COL['--acc'], .45)})`;
  const b = $('.ctr b', g);
  if (b.dataset.v !== text) { b.dataset.v = text; b.innerHTML = text; }
  $('.ctr span', g).textContent = sub;
}

// ═══════════════════════════════════════════════════════════════════════════
//  live state — one SSE stream feeds every page
// ═══════════════════════════════════════════════════════════════════════════

const H = { t: [], cpu: [], mem: [], temp: [], rx: [], tx: [], dr: [], dw: [] };
const HK = ['t', 'cpu', 'mem', 'temp', 'rx', 'tx', 'dr', 'dw'];
const S = { info: null, s: null, slow: null, net: {}, alertKeys: null, cache: {} };
let es = null;

function pushHist(row) { HK.forEach((k, i) => { H[k].push(row[i]); if (H[k].length > 300) H[k].shift(); }); }
function pushMap(map, key, v, n = 90) { const a = map[key] || (map[key] = []); a.push(v); if (a.length > n) a.shift(); }

function connect() {
  if (es) es.close();
  es = new EventSource('/api/stream');
  es.addEventListener('init', e => {
    const d = JSON.parse(e.data);
    S.info = d.info;
    HK.forEach(k => { H[k].length = 0; });
    (d.hist || []).forEach(pushHist);
    setConn(true);
    renderShellInfo();
    page && page.init && page.init();
  });
  es.addEventListener('s', e => onSample(JSON.parse(e.data)));
  es.addEventListener('slow', e => onSlow(JSON.parse(e.data)));
  es.onerror = async () => {
    setConn(false);
    if (es.readyState === 2) {
      try { const a = await api('/api/auth'); if (!a.authed) return showLogin(); } catch { }
      setTimeout(connect, 3000);
    }
  };
}

function onSample(s) {
  S.s = s;
  const mp = s.memT ? s.memU / s.memT * 100 : 0;
  pushHist([s.t, s.cpu, +mp.toFixed(1), s.temp, s.rx, s.tx, s.dr, s.dw]);
  (s.net || []).forEach(n => pushMap(S.net, n.n, n.rx + n.tx));
  setConn(true);
  const mini = (id, p, txt) => { const el = document.getElementById(id); if (el) { el.firstChild.style.width = clamp(p) + '%'; el.className = 'bar thin ' + lvl(p); el.nextElementSibling.textContent = txt; } };
  mini('m-cpu', s.cpu, Math.round(s.cpu) + '%');
  mini('m-mem', mp, Math.round(mp) + '%');
  if (s.temp) mini('m-tmp', s.temp / 85 * 100, Math.round(s.temp) + '°');
  // the living eye: pupil dilates with CPU load and the iris pulses on every sample
  const eye = $('#eye');
  if (eye && !document.body.classList.contains('lite')) {
    $('.pupil', eye).style.transform = `scale(${(.6 + s.cpu / 100 * .9).toFixed(2)})`;
    eye.classList.remove('beat'); void eye.offsetWidth; eye.classList.add('beat');
  }
  page && page.sample && page.sample(s);
}

function onSlow(sl) {
  S.slow = sl;
  const al = sl.alerts || [];
  const crit = al.some(a => a.level === 'crit');
  const eye = $('#eye');
  if (eye) { eye.classList.toggle('crit', crit); eye.classList.toggle('warn', !crit && al.length > 0); }
  const n = $('#bell-n');
  if (n) n.textContent = al.length || '';
  const keys = new Set(al.map(a => a.key));
  if (S.alertKeys) al.filter(a => a.level === 'crit' && !S.alertKeys.has(a.key)).forEach(a => toast(a.msg, 'crit', 7000));
  S.alertKeys = keys;
  const setCnt = (id, v, cls = '') => { const el = document.getElementById(id); if (el) { el.textContent = v; el.className = 'cnt ' + cls; } };
  if (sl.docker) setCnt('n-docker', `${sl.docker.running}/${sl.docker.total}`, sl.docker.bad.length ? 'crit' : '');
  setCnt('n-services', sl.failed && sl.failed.length ? sl.failed.length + ' failed' : '', 'crit');
  if (bellOpen) renderBell();
  page && page.slow && page.slow(sl);
}

function setConn(on) {
  const l = $('#live');
  if (!l || l.dataset.on === String(on)) return;
  l.dataset.on = on;
  l.innerHTML = on ? `<span class="dot ok pulse"></span><span class="lbl">Live</span>` : `<span class="dot crit"></span><span class="lbl">Reconnecting…</span>`;
}

// ═══════════════════════════════════════════════════════════════════════════
//  shell & router
// ═══════════════════════════════════════════════════════════════════════════

const PAGES = [
  { id: 'overview', t: 'Overview', i: 'grid', sec: 'Monitor', k: 'o' },
  { id: 'processes', t: 'Processes', i: 'cpu', k: 'p' },
  { id: 'network', t: 'Network', i: 'wifi', k: 'n' },
  { id: 'storage', t: 'Storage', i: 'disk', k: 'f' },
  { id: 'docker', t: 'Docker', i: 'box', sec: 'Manage', k: 'd', cnt: 'n-docker' },
  { id: 'services', t: 'Services', i: 'layers', k: 's', cnt: 'n-services' },
  { id: 'startup', t: 'Startup', i: 'rocket', k: 'b' },
  { id: 'ports', t: 'Ports', i: 'plug', k: 'x' },
  { id: 'logs', t: 'Logs', i: 'list', sec: 'Tools', k: 'l' },
  { id: 'terminal', t: 'Terminal', i: 'term', k: 't' },
  { id: 'security', t: 'Security', i: 'shield', k: 'y' },
  { id: 'settings', t: 'Settings', i: 'gear', k: 'c' },
];
const P = {};
let page = null, pageId = null;

const eyeSVG = (id = '') => `<div class="eye" ${id ? `id="${id}"` : ''}><svg viewBox="0 0 32 32">
  <defs><linearGradient id="eyeG" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="var(--acc)"/><stop offset=".6" stop-color="var(--acc-2)"/><stop offset="1" stop-color="var(--acc-3)"/></linearGradient></defs>
  <circle class="glow" cx="16" cy="16" r="7"/>
  <g class="lids"><path class="lid" d="M2 16C6 8.5 10.5 5 16 5s10 3.5 14 11c-4 7.5-8.5 11-14 11S6 23.5 2 16z"/>
  <circle class="iris" cx="16" cy="16" r="6"/><circle class="pupil" cx="16" cy="16" r="2.6"/></g></svg></div>`;

function renderShell() {
  let navHTML = '';
  for (const p of PAGES) {
    if (p.sec) navHTML += `<div class="nav-sec">${p.sec}</div>`;
    navHTML += `<a href="#/${p.id}" data-p="${p.id}">${ic(p.i)}<span>${p.t}</span>${p.cnt ? `<span class="cnt" id="${p.cnt}"></span>` : ''}</a>`;
  }
  $('#root').innerHTML = `
  <div class="app">
    <aside class="side" id="side">
      <div class="brand">${eyeSVG('eye')}<div><b>iseall</b><small id="b-host">connecting…</small></div></div>
      <nav class="nav">${navHTML}</nav>
      <div class="side-foot">
        <div class="mini-stat"><span>CPU</span><div class="bar thin" id="m-cpu"><i></i></div><span>—</span></div>
        <div class="mini-stat"><span>MEM</span><div class="bar thin" id="m-mem"><i></i></div><span>—</span></div>
        <div class="mini-stat"><span>TEMP</span><div class="bar thin" id="m-tmp"><i></i></div><span>—</span></div>
      </div>
    </aside>
    <main class="main">
      <header class="top">
        <button class="ib burger" id="burger">${ic('menu')}</button>
        <h1 id="title"></h1>
        <button class="search-trigger" id="cmdk">${ic('search')}<span class="lbl">Search or run a command…</span><kbd>Ctrl K</kbd></button>
        <div class="live" id="live"></div>
        <div class="bell"><button class="ib" id="bell" title="Alerts">${ic('bell')}</button><span class="n" id="bell-n"></span></div>
        <button class="ib" id="theme" title="Toggle theme">${ic('sun')}</button>
      </header>
      <div class="content" id="content"></div>
    </main>
  </div>`;
  $('#cmdk').onclick = openPalette;
  $('#theme').onclick = toggleTheme;
  $('#bell').onclick = e => { e.stopPropagation(); toggleBell(); };
  $('#burger').onclick = e => { e.stopPropagation(); $('#side').classList.toggle('open'); };
  $('#side').addEventListener('click', e => { if (e.target.closest('a')) $('#side').classList.remove('open'); });
  updateThemeIcon();
  setConn(false);
}

function renderShellInfo() {
  if (!S.info) return;
  const h = $('#b-host');
  if (h) h.textContent = S.info.hostname;
  document.title = `${S.info.hostname} · iseall`;
  const c = $('#title .crumb');
  if (c) c.textContent = '· ' + S.info.hostname;
}

function parseHash() {
  const h = location.hash.replace(/^#\/?/, '');
  const [id, qs] = h.split('?');
  return { id: PAGES.some(p => p.id === id) ? id : 'overview', q: new URLSearchParams(qs || '') };
}
function navigate(id, q = '') {
  const h = '#/' + id + (q ? '?' + q : '');
  if (location.hash === h) route(); else location.hash = h;
}

let pollGen = 0, pollTimer = 0;
function startPoll(fn, ms) {
  const g = ++pollGen;
  const tick = async () => {
    if (g !== pollGen) return;
    if (!document.hidden) { try { await fn(); if (g !== pollGen) return; } catch (e) { if (e.message !== 'Session expired') console.warn(e); } }
    if (g === pollGen) pollTimer = setTimeout(tick, ms);
  };
  tick();
}
function stopPoll() { pollGen++; clearTimeout(pollTimer); }

function route() {
  if (!$('#root .app')) return;
  const { id, q } = parseHash();
  while (overlays.length) overlays[overlays.length - 1].close();
  closeMenu();
  if (page && page.unmount) page.unmount();
  stopPoll();
  charts.forEach(c => c.destroy());
  pageId = id;
  const meta = PAGES.find(p => p.id === id);
  $$('.nav a').forEach(a => a.classList.toggle('on', a.dataset.p === id));
  $('#title').innerHTML = `${esc(meta.t)}<span class="crumb">${S.info ? '· ' + esc(S.info.hostname) : ''}</span>`;
  scrollTo(0, 0);
  $('#content').innerHTML = '<div class="page" id="pg"></div>';
  page = P[id];
  page.mount($('#pg'), q);
  if (page.poll) startPoll(() => page.poll(), page.every || 3000);
}

// ═══════════════════════════════════════════════════════════════════════════
//  page: overview
// ═══════════════════════════════════════════════════════════════════════════

P.overview = {
  mount(el) {
    this.el = el;
    el.innerHTML = `
    <div class="hero">
      <div class="card host hl">
        <h2 id="o-host">—</h2>
        <div class="model" id="o-model"></div>
        <div class="row wrap" style="margin-top:12px;gap:8px 14px">
          <span class="label">UPTIME</span><span class="uptime" id="o-up">—</span>
          <span class="label">BOOTED</span><span class="mono mute" id="o-boot">—</span>
        </div>
        <div class="facts" id="o-facts"></div>
      </div>
      <div class="card">
        <div class="card-h"><h3>${ic('activity')} Health</h3><div class="r" id="o-hb"></div></div>
        <div class="pills">
          <div class="pill" data-go="docker"><div class="label">${ic('box')} Containers</div><b id="o-p-docker">—</b></div>
          <div class="pill" data-go="services"><div class="label">${ic('layers')} Failed units</div><b id="o-p-failed">—</b></div>
          <div class="pill" data-go="processes"><div class="label">${ic('cpu')} Tasks</div><b id="o-p-procs">—</b></div>
          <div class="pill" data-go="processes"><div class="label">${ic('zap')} Load (1m)</div><b id="o-p-load">—</b></div>
        </div>
        <div id="o-alerts" style="margin-top:12px"></div>
        <div id="o-apps"></div>
      </div>
    </div>
    <div class="grid g4">
      <div class="card gauge-card" data-go="processes">${gaugeSVG('g-cpu')}<div class="gauge-meta" id="gm-cpu"></div></div>
      <div class="card gauge-card" data-go="processes">${gaugeSVG('g-mem')}<div class="gauge-meta" id="gm-mem"></div></div>
      <div class="card gauge-card">${gaugeSVG('g-tmp')}<div class="gauge-meta" id="gm-tmp"></div></div>
      <div class="card gauge-card" data-go="storage">${gaugeSVG('g-dsk')}<div class="gauge-meta" id="gm-dsk"><div class="skel"></div><div class="skel"></div></div></div>
    </div>
    <div class="grid g2 mt">
      <div class="card"><div class="card-h"><h3>${ic('activity')} CPU &amp; memory</h3><div class="r legend" id="l-cm"></div></div><div id="c-cm"></div></div>
      <div class="card"><div class="card-h"><h3>${ic('wifi')} Network</h3><div class="r legend" id="l-net"></div></div><div id="c-net"></div></div>
    </div>
    <div class="grid g3 mt">
      <div class="card"><div class="card-h"><h3>${ic('cpu')} Cores</h3><div class="r mute num" id="o-freq"></div></div><div class="cores" id="o-cores"></div>
        <div class="row" style="margin-top:16px;gap:18px" id="o-load"></div></div>
      <div class="card"><div class="card-h"><h3>${ic('disk')} Disk I/O</h3><div class="r legend" id="l-io"></div></div><div id="c-io" class="chart sm"></div></div>
      <div class="card"><div class="card-h"><h3>${ic('disk')} Filesystems</h3><div class="r"><button class="btn sm ghost" data-go="storage">Details</button></div></div><div class="col" id="o-mounts"><div class="skel"></div><div class="skel"></div></div></div>
    </div>`;
    el.onclick = e => { const a = e.target.closest('[data-nav]'); if (a) return navigate(a.dataset.nav); const g = e.target.closest('[data-go]'); if (g) navigate(g.dataset.go); };
    const rateAxis = { max: 0, min: 1024, fmt: fmtRate, fmtAxis: v => fmtB(v) };
    this.cm = new LineChart($('#c-cm', el), { series: [{ c: '--acc', label: 'CPU' }, { c: '--acc-2', label: 'Memory' }] });
    this.net = new LineChart($('#c-net', el), { ...rateAxis, series: [{ c: '--acc', label: 'Down' }, { c: '--acc-3', label: 'Up' }] });
    this.io = new LineChart($('#c-io', el), { ...rateAxis, series: [{ c: '--info', label: 'Read' }, { c: '--acc-3', label: 'Write' }] });
    this.init();
    if (S.s) this.sample(S.s);
    if (S.slow) this.slow(S.slow);
    this.upT = setInterval(() => this.tickUp(), 1000);
    this.apps();
  },
  unmount() { clearInterval(this.upT); },
  // quick-launch for every web UI discovered on this box
  async apps(retry = 2) {
    const p = (retry === 2 && S.cache.ports) || await api('/api/ports').catch(() => null);
    if (!p || pageId !== 'overview') return;
    S.cache.ports = p;
    const web = p.listen.filter(l => l.web && l.scope !== 'local' && l.web.status < 400 && l.proc !== 'iseall');
    const el = $('#o-apps');
    if (!el) return;
    // HTTP probes run in the background on the server; give them a moment on a cold start
    if (!web.length) { if (retry) setTimeout(() => this.apps(retry - 1), 2500); return; }
    el.innerHTML = `<div class="label" style="margin:14px 0 8px;letter-spacing:.08em;text-transform:uppercase;font-weight:650">Web apps on this machine</div><div class="chips">` +
      web.slice(0, 12).map(l => `<a class="chip" href="${l.web.scheme}://${esc(location.hostname)}:${l.port}/" target="_blank" rel="noopener" title="${esc(l.ctr || l.proc || '')}">${ic('globe')}${esc(l.web.title || l.ctr || l.proc || 'port')}<span class="dim">:${l.port}</span></a>`).join('') + '</div>';
  },
  init() {
    const i = S.info;
    if (!i || !$('#o-host')) return;
    $('#o-host').textContent = i.hostname;
    $('#o-model').textContent = i.model || i.cpuModel || '';
    $('#o-boot').textContent = new Date(i.bootTime * 1000).toLocaleString();
    $('#o-facts').innerHTML = [
      i.os && `<span class="chip">${esc(i.os)}</span>`,
      `<span class="chip">kernel ${esc(i.kernel)}</span>`,
      `<span class="chip">${esc(i.arch)} · ${i.cores} cores</span>`,
      i.cpuModel && `<span class="chip">${esc(i.cpuModel)}</span>`,
      ...(i.ips || []).map(ip => `<span class="chip">${ic('globe')}${esc(ip)}</span>`),
    ].filter(Boolean).join('');
    this.redrawCharts();
    this.tickUp();
  },
  tickUp() { const el = $('#o-up'); if (el && S.info) el.textContent = fmtDur(Date.now() / 1000 - S.info.bootTime); },
  redrawCharts() {
    this.cm.set(H.t, [H.cpu, H.mem]);
    this.net.set(H.t, [H.rx, H.tx]);
    this.io.set(H.t, [H.dr, H.dw]);
  },
  sample(s) {
    if (!$('#g-cpu')) return;
    const mp = s.memT ? s.memU / s.memT * 100 : 0;
    setGauge('g-cpu', s.cpu / 100, `${Math.round(s.cpu)}<small>%</small>`, 'CPU', lvl(s.cpu, 75, 90));
    $('#gm-cpu').innerHTML = `
      <div class="kv"><span>Busy</span><span>${s.cpu.toFixed(1)}%</span></div>
      <div class="kv"><span>I/O wait</span><span class="${s.iow > 10 ? 'warn-t' : ''}">${s.iow.toFixed(1)}%</span></div>
      <div class="kv"><span>Clock</span><span>${s.freq ? (s.freq >= 1000 ? (s.freq / 1000).toFixed(2) + ' GHz' : s.freq + ' MHz') : '—'}</span></div>
      <div class="kv"><span>Running</span><span>${s.run} / ${s.procs}</span></div>`;
    setGauge('g-mem', mp / 100, `${Math.round(mp)}<small>%</small>`, 'Memory', lvl(mp, 80, 92));
    $('#gm-mem').innerHTML = `
      <div class="kv"><span>Used</span><span>${fmtB(s.memU)}</span></div>
      <div class="kv"><span>Cache</span><span>${fmtB(s.memC)}</span></div>
      <div class="kv"><span>Total</span><span>${fmtB(s.memT)}</span></div>
      <div class="kv"><span>Swap</span><span class="${s.swapT && s.swapU / s.swapT > .5 ? 'warn-t' : ''}">${s.swapT ? fmtB(s.swapU) + ' / ' + fmtB(s.swapT) : 'off'}</span></div>`;
    if (s.temp) setGauge('g-tmp', s.temp / 90, `${Math.round(s.temp)}<small>°C</small>`, 'Temp', lvl(s.temp, 70, 80));
    else setGauge('g-tmp', 0, '—', 'Temp', '');
    $('#gm-tmp').innerHTML = this.thermal(s);
    this.redrawCharts();
    const lg = (id, items) => { $(id).innerHTML = items.map(([c, l, v]) => `<span><i style="background:${COL[c]}"></i>${l} <b>${v}</b></span>`).join(''); };
    lg('#l-cm', [['--acc', 'CPU', s.cpu.toFixed(0) + '%'], ['--acc-2', 'Mem', mp.toFixed(0) + '%']]);
    lg('#l-net', [['--acc', '↓', fmtRate(s.rx)], ['--acc-3', '↑', fmtRate(s.tx)]]);
    lg('#l-io', [['--info', 'R', fmtRate(s.dr)], ['--acc-3', 'W', fmtRate(s.dw)]]);
    const cores = s.cores || [];
    const ce = $('#o-cores');
    if (ce.children.length !== cores.length) ce.innerHTML = cores.map((_, k) => `<div class="core"><i></i><span>#${k}</span><b>0%</b></div>`).join('');
    cores.forEach((v, k) => {
      const c = ce.children[k];
      c.firstChild.style.height = clamp(v) + '%';
      c.lastChild.textContent = Math.round(v) + '%';
      c.classList.toggle('hot', v >= 85);
    });
    $('#o-freq').textContent = s.freq ? `${s.freq} MHz` : '';
    const nc = (S.info && S.info.cores) || cores.length || 1;
    $('#o-load').innerHTML = ['1m', '5m', '15m'].map((l, k) => `<div class="grow"><div class="label" style="display:flex;justify-content:space-between;margin-bottom:5px"><span>Load ${l}</span><b class="num" style="color:var(--text)">${s.load[k].toFixed(2)}</b></div>${bar(s.load[k] / nc * 100, lvl(s.load[k] / nc * 100, 80, 100), 'thin')}</div>`).join('');
    $('#o-p-procs').textContent = s.procs;
    $('#o-p-load').innerHTML = `${s.load[0].toFixed(2)} <small>/ ${nc}</small>`;
  },
  thermal(s) {
    const t = s.thr;
    let flags = '';
    if (t >= 0 && S.info && S.info.isPi) {
      const f = [[0x1, 'Under-voltage'], [0x2, 'Freq capped'], [0x4, 'Throttled'], [0x8, 'Soft temp limit']];
      const now = f.filter(([b]) => t & b).map(([, n]) => `<span class="badge crit">${n}</span>`);
      const past = f.filter(([b]) => (t & (b << 16)) && !(t & b)).map(([, n]) => `<span class="badge warn">${n} earlier</span>`);
      flags = now.concat(past).join('') || `<span class="badge ok">${ic('check')}Power &amp; thermals OK</span>`;
    }
    return `
      <div class="kv"><span>Sensor</span><span>${s.temp ? s.temp.toFixed(1) + ' °C' : 'n/a'}</span></div>
      <div class="kv"><span>Peak (5m)</span><span>${H.temp.length ? Math.max(...H.temp).toFixed(1) + ' °C' : '—'}</span></div>
      ${flags ? `<div class="flags">${flags}</div>` : '<div class="kv"><span>Throttling</span><span class="mute">n/a</span></div>'}`;
  },
  slow(sl) {
    if (!$('#g-dsk')) return;
    const ms = sl.mounts || [];
    const root = ms.find(m => m.path === '/') || ms[0];
    if (root && root.total) {
      const p = root.used / root.total * 100;
      setGauge('g-dsk', p / 100, `${Math.round(p)}<small>%</small>`, 'Disk /', lvl(p, 80, 90));
      $('#gm-dsk').innerHTML = `
        <div class="kv"><span>Used</span><span>${fmtB(root.used)}</span></div>
        <div class="kv"><span>Free</span><span>${fmtB(root.free)}</span></div>
        <div class="kv"><span>Size</span><span>${fmtB(root.total)}</span></div>
        <div class="kv"><span>Device</span><span class="mono">${esc(root.dev.replace('/dev/', ''))}</span></div>`;
    }
    $('#o-mounts').innerHTML = ms.slice(0, 6).map(m => {
      const p = m.total ? m.used / m.total * 100 : 0;
      return `<div><div class="row" style="justify-content:space-between;font-size:12.5px;margin-bottom:5px"><span class="mono trunc">${esc(m.path)}</span><span class="mute num" style="white-space:nowrap">${m.err ? '<span class="crit-t">not responding</span>' : `${fmtB(m.used)} / ${fmtB(m.total)}`}</span></div>${bar(p, lvl(p, 80, 90))}</div>`;
    }).join('') || '<div class="mute">No filesystems found</div>';
    const d = sl.docker;
    $('#o-p-docker').innerHTML = d ? `${d.running}<small> / ${d.total}</small>` : '<small>n/a</small>';
    const f = (sl.failed || []).length;
    $('#o-p-failed').innerHTML = f ? `<span class="crit-t">${f}</span>` : '<span class="ok-t">0</span>';
    const al = sl.alerts || [];
    $('#o-hb').innerHTML = al.length ? `<span class="badge ${al.some(a => a.level === 'crit') ? 'crit' : 'warn'}">${plural(al.length, 'issue')}</span>` : `<span class="badge ok">${ic('check')}All systems nominal</span>`;
    $('#o-alerts').innerHTML = al.slice(0, 4).map(a => `<div class="alert-li ${a.level}" data-nav="${esc(a.nav || 'overview')}">${ic(a.level === 'crit' ? 'alert' : 'info')}<span class="grow">${esc(a.msg)}</span>${ic('ext', 'dim')}</div>`).join('') +
      (al.length > 4 ? `<div class="mute" style="font-size:12px">+${al.length - 4} more under the bell</div>` : '');
  },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: processes
// ═══════════════════════════════════════════════════════════════════════════

P.processes = {
  every: 2000,
  mount(el) {
    this.el = el;
    this.sort = store.get('psort', { k: 'cpu', asc: false });
    this.q = ''; this.kern = false; this.filt = 'all'; this.data = null;
    el.innerHTML = `
      <div class="toolbar">
        <div class="search">${ic('search')}<input class="in" id="ps-q" placeholder="Name, user, command, container…  ( / )"></div>
        <div class="btn-group" id="ps-f"><button data-f="all" class="on">All</button><button data-f="ctr">Containers</button><button data-f="svc">Services</button><button data-f="usr">User</button></div>
        <label class="check"><span class="switch"><input type="checkbox" id="ps-k"><i></i></span>Kernel threads</label>
        <div class="r mute num" id="ps-n"></div>
      </div>
      <div class="card flush"><div class="tbl-wrap" style="max-height:calc(100vh - 200px)"><table class="tbl">
        <thead><tr>
          <th data-sort="pid" class="r">PID</th><th data-sort="name">Process</th><th data-sort="user" class="hide-sm">User</th>
          <th data-sort="cpu">CPU</th><th data-sort="rss">Memory</th><th data-sort="thr" class="r hide-sm">Thr</th>
          <th data-sort="start" class="hide-sm">Age</th><th class="hide-sm">Command</th><th></th></tr></thead>
        <tbody id="ps-b"><tr><td colspan="9"><div class="skel"></div><div class="skel"></div><div class="skel"></div></td></tr></tbody></table></div></div>`;
    const q = $('#ps-q', el);
    q.oninput = debounce(() => { this.q = q.value.toLowerCase(); this.render(); }, 120);
    $('#ps-k', el).onchange = e => { this.kern = e.target.checked; this.render(); };
    $('#ps-f', el).onclick = e => { const b = e.target.closest('[data-f]'); if (!b) return; this.filt = b.dataset.f; $$('#ps-f button').forEach(x => x.classList.toggle('on', x === b)); this.render(); };
    $('thead', el).onclick = e => {
      const th = e.target.closest('[data-sort]'); if (!th) return;
      const k = th.dataset.sort;
      this.sort = this.sort.k === k ? { k, asc: !this.sort.asc } : { k, asc: ['name', 'user', 'pid'].includes(k) };
      store.set('psort', this.sort);
      this.render();
    };
    $('#ps-b', el).onclick = e => {
      const tr = e.target.closest('tr[data-pid]');
      if (!tr || !this.data) return;
      const p = this.data.procs.find(x => x.pid === +tr.dataset.pid);
      if (!p) return;
      const a = e.target.closest('[data-a]');
      if (a) { e.stopPropagation(); procMenu(a, p); return; }
      procDrawer(p);
    };
  },
  async poll() { const d = await api('/api/procs'); if (pageId !== 'processes') return; this.data = d; this.render(); },
  render() {
    if (!this.data) return;
    const d = this.data;
    const { k, asc } = this.sort;
    let list = d.procs.filter(p => this.kern || !p.k);
    if (this.filt === 'ctr') list = list.filter(p => p.ctr);
    if (this.filt === 'svc') list = list.filter(p => p.unit);
    if (this.filt === 'usr') list = list.filter(p => !p.unit && !p.ctr && !p.k);
    if (this.q) list = list.filter(p => `${p.name} ${p.user} ${p.cmd} ${p.ctr || ''} ${p.unit || ''} ${p.pid}`.toLowerCase().includes(this.q));
    list.sort((a, b) => { const x = a[k], y = b[k]; const r = typeof x === 'string' ? x.localeCompare(y) : x - y; return asc ? r : -r; });
    $$('thead th', this.el).forEach(th => { th.classList.toggle('sorted', th.dataset.sort === k); th.classList.toggle('asc', th.dataset.sort === k && asc); });
    const tot = d.procs.reduce((s, p) => s + p.cpu, 0);
    $('#ps-n').textContent = `${list.length} shown · ${d.procs.length} total · ΣCPU ${tot.toFixed(0)}%`;
    const maxRss = Math.max(...d.procs.map(p => p.rss), 1);
    const now = Date.now() / 1000;
    $('#ps-b').innerHTML = list.slice(0, 400).map(p => {
      const mp = p.rss / maxRss * 100;
      const tags = (p.ctr ? `<span class="badge acc">${ic('box')}${esc(p.ctr)}</span>` : '') + (p.unit ? `<span class="badge vio">${esc(p.unit.replace(/\.service$/, ''))}</span>` : '');
      const st = p.st === 'Z' ? '<span class="badge crit">zombie</span>' : p.st === 'D' ? '<span class="badge warn">disk wait</span>' : p.st === 'T' ? '<span class="badge warn">stopped</span>' : '';
      return `<tr class="click" data-pid="${p.pid}">
        <td class="r mono mute">${p.pid}</td>
        <td><div class="row" style="gap:7px"><b style="font-weight:600">${esc(p.name)}</b>${tags}${st}</div></td>
        <td class="hide-sm mute">${esc(p.user)}</td>
        <td><div class="cellbar" title="% of one core">${bar(p.cpu, lvl(p.cpu, 60, 90), 'thin')}<span>${p.cpu.toFixed(1)}%</span></div></td>
        <td><div class="cellbar" title="${d.memTotal ? (p.rss / d.memTotal * 100).toFixed(1) + '% of RAM' : ''}">${bar(mp, '', 'thin')}<span>${fmtB(p.rss)}</span></div></td>
        <td class="r hide-sm num mute">${p.thr}</td>
        <td class="hide-sm mute num" style="white-space:nowrap">${fmtDur(now - p.start)}</td>
        <td class="hide-sm"><div class="cmd" title="${esc(p.cmd)}">${esc(p.cmd || '[' + p.name + ']')}</div></td>
        <td class="acts"><button class="ib" data-a="m" title="Actions">${ic('more')}</button></td></tr>`;
    }).join('') || `<tr><td colspan="9"><div class="empty">${ic('search')}<b>No matching processes</b></div></td></tr>`;
  },
};

async function signal(p, sig) {
  if (sig === 'KILL' || sig === 'TERM') {
    const ok = await confirmBox(`${sig === 'KILL' ? 'Force kill' : 'Terminate'} ${esc(p.name)}?`,
      `PID <b class="mono">${p.pid}</b>${p.user ? ' · ' + esc(p.user) : ''}<br><span class="mute mono" style="font-size:12px">${esc((p.cmd || '').slice(0, 200))}</span>`,
      { ok: sig === 'KILL' ? 'Kill' : 'Terminate', danger: true });
    if (!ok) return false;
  }
  await act(() => post('/api/procs/signal', { pid: p.pid, sig }), `Sent SIG${sig} to ${p.name} (${p.pid})`);
  if (pageId === 'processes') page.poll().catch(() => { });
  return true;
}
function procMenu(anchor, p) {
  showMenu(anchor, [
    { label: 'Details', icon: 'info', fn: () => procDrawer(p) },
    '-',
    { label: 'Terminate (TERM)', icon: 'stop', fn: () => signal(p, 'TERM') },
    { label: 'Reload (HUP)', icon: 'restart', fn: () => signal(p, 'HUP') },
    p.st === 'T' ? { label: 'Resume (CONT)', icon: 'play', fn: () => signal(p, 'CONT') } : { label: 'Pause (STOP)', icon: 'pause', fn: () => signal(p, 'STOP') },
    { label: 'Lower priority (nice 10)', icon: 'down', fn: () => act(() => post('/api/procs/signal', { pid: p.pid, nice: 10 }), 'Priority lowered') },
    '-',
    { label: 'Force kill (KILL)', icon: 'x', cls: 'crit', fn: () => signal(p, 'KILL') },
  ]);
}
async function procDrawer(p) {
  const d = drawer({
    title: `${esc(p.name)} <span class="mute mono" style="font-weight:400">#${p.pid}</span>`,
    sub: p.start ? `${esc(p.user)} · started ${ago(p.start)}` : '',
    head: `<button class="btn sm" data-s="TERM">${ic('stop')}Terminate</button><button class="btn sm danger" data-s="KILL">${ic('x')}Kill</button>`,
    body: '<div class="skel"></div><div class="skel"></div>',
  });
  d.el.addEventListener('click', async e => {
    const b = e.target.closest('[data-s]');
    if (b && await signal(p, b.dataset.s)) d.close();
    const pp = e.target.closest('[data-cpid]');
    if (pp) { d.close(); procDrawer({ pid: +pp.dataset.cpid, name: 'pid ' + pp.dataset.cpid }); }
  });
  try {
    const x = await api('/api/procs/' + p.pid);
    d.body.innerHTML = `
      <div><h4>Command line</h4><pre class="out">${esc((x.argv || []).join(' ') || '—')}</pre></div>
      <dl class="kvs">
        <dt>Executable</dt><dd class="mono">${esc(x.exe || '—')}</dd>
        <dt>Working dir</dt><dd class="mono">${esc(x.cwd || '—')}</dd>
        ${p.ctr ? `<dt>Container</dt><dd><span class="badge acc">${esc(p.ctr)}</span></dd>` : ''}
        ${p.unit ? `<dt>Service</dt><dd><span class="badge vio">${esc(p.unit)}</span></dd>` : ''}
        ${p.cpu !== undefined ? `<dt>CPU · memory</dt><dd>${p.cpu.toFixed(1)}% · ${fmtB(p.rss)}</dd>` : ''}
        <dt>Peak virtual</dt><dd>${esc(x.VmPeak || '—')}</dd>
        <dt>Swapped</dt><dd>${esc(x.VmSwap || '—')}</dd>
        <dt>Open files</dt><dd>${x.fds ?? '—'}</dd>
        <dt>Disk read / write</dt><dd>${x.read_bytes !== undefined ? fmtB(x.read_bytes) + ' / ' + fmtB(x.write_bytes) : '—'}</dd>
        <dt>Context switches</dt><dd>${esc(x.voluntary_ctxt_switches || '—')} voluntary · ${esc(x.nonvoluntary_ctxt_switches || '—')} forced</dd>
        ${p.thr !== undefined ? `<dt>Threads · nice</dt><dd>${p.thr} · ${p.ni}</dd><dt>Parent</dt><dd class="mono">${p.ppid}</dd>` : ''}
        <dt>Children</dt><dd>${(x.children || []).map(c => `<a class="chip" href="javascript:void 0" data-cpid="${c}">${c}</a>`).join(' ') || '—'}</dd>
      </dl>`;
  } catch (e) { d.body.innerHTML = `<div class="empty">${ic('alert')}<b>Process is gone</b>${esc(e.message)}</div>`; }
}

// ═══════════════════════════════════════════════════════════════════════════
//  page: docker
// ═══════════════════════════════════════════════════════════════════════════

const ctrHist = {};
const POLICIES = ['no', 'always', 'unless-stopped', 'on-failure'];
const policySelect = (id, cur) => `<select class="mini" data-policy="${esc(id)}">${POLICIES.map(p => `<option ${p === cur ? 'selected' : ''}>${p}</option>`).join('')}</select>`;

P.docker = {
  every: 3000,
  mount(el, q) {
    this.el = el;
    this.tab = q.get('tab') || 'containers';
    this.q = ''; this.state = 'all'; this.d = null; this.img = null;
    el.innerHTML = `
      <div class="tabs" id="dk-tabs">
        <button data-t="containers">${ic('box')}Containers <span class="cnt" id="dk-n1"></span></button>
        <button data-t="images">${ic('layers')}Images <span class="cnt" id="dk-n2"></span></button>
        <button data-t="volumes">${ic('disk')}Volumes <span class="cnt" id="dk-n3"></span></button>
        <button data-t="networks">${ic('wifi')}Networks <span class="cnt" id="dk-n4"></span></button>
      </div>
      <div class="toolbar">
        <div class="search">${ic('search')}<input class="in" id="dk-q" placeholder="Filter…  ( / )"></div>
        <div class="btn-group" id="dk-st"><button data-s="all" class="on">All</button><button data-s="running">Running</button><button data-s="stopped">Stopped</button></div>
        <div class="r"><button class="btn" id="dk-prune">${ic('trash')}Clean up</button></div>
      </div>
      <div id="dk-body"><div class="ctrs">${'<div class="card"><div class="skel"></div><div class="skel"></div><div class="skel"></div></div>'.repeat(3)}</div></div>`;
    $('#dk-tabs', el).onclick = e => { const b = e.target.closest('[data-t]'); if (b) { this.tab = b.dataset.t; this.syncTabs(); this.render(); this.poll().catch(() => { }); } };
    $('#dk-q', el).oninput = debounce(e => { this.q = e.target.value.toLowerCase(); this.render(); }, 120);
    $('#dk-st', el).onclick = e => { const b = e.target.closest('[data-s]'); if (!b) return; this.state = b.dataset.s; $$('#dk-st button').forEach(x => x.classList.toggle('on', x === b)); this.render(); };
    $('#dk-prune', el).onclick = e => { e.stopPropagation(); this.pruneMenu(e.currentTarget); };
    $('#dk-body', el).onclick = e => this.click(e);
    $('#dk-body', el).onchange = e => {
      const s = e.target.closest('select[data-policy]');
      if (s) act(() => post(`/api/docker/c/${s.dataset.policy}/policy`, { policy: s.value }), `Restart policy → ${s.value}`).then(() => this.poll()).catch(() => { });
    };
    this.syncTabs();
  },
  syncTabs() {
    $$('#dk-tabs button').forEach(b => b.classList.toggle('on', b.dataset.t === this.tab));
    $('#dk-st').classList.toggle('hide', this.tab !== 'containers');
  },
  async poll() {
    if (this.tab === 'containers') {
      const d = await api('/api/docker');
      if (pageId !== 'docker') return;
      this.d = d;
      if (this.d.available) this.d.containers.forEach(c => { if (c.state === 'running') pushMap(ctrHist, c.id, c.cpu, 40); });
    } else {
      const img = await api('/api/docker/images');
      if (pageId !== 'docker') return;
      this.img = img;
    }
    this.render();
  },
  render() {
    const body = $('#dk-body');
    if (!body) return;
    const ae = document.activeElement;
    if (ae && body.contains(ae) && ae.tagName === 'SELECT') return; // don't yank an open dropdown
    if (this.d && !this.d.available) {
      body.innerHTML = `<div class="card empty">${ic('box')}<b>Docker isn't reachable</b>${esc(this.d.error || '')}<br><span class="mute">Is Docker installed, and is iseall running as root?</span></div>`;
      return;
    }
    if (this.d) $('#dk-n1').textContent = this.d.containers.length;
    if (this.img) { $('#dk-n2').textContent = this.img.images.length; $('#dk-n3').textContent = (this.img.volumes || []).length; $('#dk-n4').textContent = (this.img.networks || []).length; }
    if (this.tab === 'containers') return this.renderCtrs(body);
    if (!this.img) { body.innerHTML = '<div class="card"><div class="skel"></div><div class="skel"></div></div>'; return; }
    const q = this.q;
    if (this.tab === 'images') {
      const imgs = this.img.images.filter(i => !q || (i.tags || []).join(' ').toLowerCase().includes(q) || i.id.includes(q)).sort((a, b) => b.size - a.size);
      const total = imgs.reduce((s, i) => s + i.size, 0);
      body.innerHTML = `<div class="card flush"><div class="card-h"><h3>${plural(imgs.length, 'image')} · ${fmtB(total)}</h3></div><div class="tbl-wrap"><table class="tbl"><thead><tr><th>Repository : tag</th><th>ID</th><th class="r">Size</th><th>Created</th><th>In use</th><th></th></tr></thead><tbody>
        ${imgs.map(i => `<tr><td>${(i.tags || ['<none>']).map(t => `<div class="mono">${esc(t)}</div>`).join('')}</td><td class="mono mute">${esc(i.id.replace('sha256:', '').slice(0, 12))}</td>
          <td class="r num">${fmtB(i.size)}</td><td class="mute">${ago(i.created)}</td>
          <td>${i.used ? `<span class="badge ok">${plural(i.used, 'container')}</span>` : '<span class="badge mute">unused</span>'}</td>
          <td class="acts"><button class="ib crit" data-a="rmi" data-id="${esc(i.id)}" data-n="${esc((i.tags || [i.id])[0])}" title="Remove image">${ic('trash')}</button></td></tr>`).join('') || '<tr><td colspan="6" class="empty">No images</td></tr>'}
      </tbody></table></div></div>`;
    } else if (this.tab === 'volumes') {
      const v = (this.img.volumes || []).filter(x => !q || x.Name.toLowerCase().includes(q));
      body.innerHTML = `<div class="card flush"><div class="tbl-wrap"><table class="tbl"><thead><tr><th>Name</th><th>Driver</th><th>Mountpoint</th><th>Created</th></tr></thead><tbody>
        ${v.map(x => `<tr><td class="mono" title="${esc(x.Name)}">${esc(x.Name.length > 40 ? x.Name.slice(0, 12) + '…' : x.Name)}</td><td>${esc(x.Driver)}</td><td class="mono mute">${esc(x.Mountpoint)}</td><td class="mute">${esc((x.CreatedAt || '').slice(0, 10))}</td></tr>`).join('') || '<tr><td colspan="4" class="empty">No volumes</td></tr>'}
      </tbody></table></div></div>`;
    } else {
      const n = (this.img.networks || []).filter(x => !q || x.Name.toLowerCase().includes(q));
      body.innerHTML = `<div class="card flush"><div class="tbl-wrap"><table class="tbl"><thead><tr><th>Name</th><th>Driver</th><th>Scope</th><th>Subnet</th><th>ID</th></tr></thead><tbody>
        ${n.map(x => `<tr><td><b>${esc(x.Name)}</b></td><td>${esc(x.Driver)}</td><td class="mute">${esc(x.Scope)}</td><td class="mono">${esc(((x.IPAM || {}).Config || []).map(c => c.Subnet).join(', ') || '—')}</td><td class="mono mute">${esc((x.Id || '').slice(0, 12))}</td></tr>`).join('')}
      </tbody></table></div></div>`;
    }
  },
  renderCtrs(body) {
    if (!this.d) return;
    const q = this.q;
    let cs = this.d.containers;
    if (this.state === 'running') cs = cs.filter(c => c.state === 'running');
    if (this.state === 'stopped') cs = cs.filter(c => c.state !== 'running');
    if (q) cs = cs.filter(c => `${c.name} ${c.image} ${c.project || ''} ${(c.ports || []).join(' ')}`.toLowerCase().includes(q));
    if (!cs.length) { body.innerHTML = `<div class="card empty">${ic('box')}<b>No containers</b>${this.d.containers.length ? 'Nothing matches the filter.' : 'Start something with docker run or docker compose.'}</div>`; return; }
    const groups = new Map();
    cs.forEach(c => { const k = c.project || ''; if (!groups.has(k)) groups.set(k, []); groups.get(k).push(c); });
    const host = location.hostname;
    let html = '';
    for (const [proj, list] of groups) {
      const run = list.filter(c => c.state === 'running').length;
      html += `<div class="stack"><div class="stack-h">
        <h4>${proj ? ic('layers') + esc(proj) : ic('box') + 'Standalone'}</h4><span class="badge ${run === list.length ? 'ok' : run ? 'warn' : 'mute'}">${run}/${list.length} running</span>
        ${proj && this.d.cli ? `<div class="row" style="margin-left:auto;gap:6px">
          <button class="btn sm" data-stack="update" data-p="${esc(proj)}" title="docker compose pull && up -d">${ic('download')}Update</button>
          <button class="btn sm" data-stack="restart" data-p="${esc(proj)}">${ic('restart')}Restart</button>
          <button class="btn sm" data-stack="${run ? 'stop' : 'up'}" data-p="${esc(proj)}">${ic(run ? 'stop' : 'play')}${run ? 'Stop' : 'Up'}</button>
          <button class="ib" data-stackmenu data-p="${esc(proj)}">${ic('more')}</button></div>` : ''}
      </div><div class="ctrs">`;
      for (const c of list) {
        const on = c.state === 'running';
        const dotc = c.health === 'unhealthy' || c.state === 'restarting' ? 'crit' : on ? (c.health === 'starting' ? 'warn' : 'ok') : c.state === 'paused' ? 'warn' : '';
        const memT = S.s ? S.s.memT : 0;
        const mp = c.memLimit ? c.mem / c.memLimit * 100 : memT ? c.mem / memT * 100 : 0;
        const ports = (c.ports || []).map(p => {
          const m = p.match(/^(\d+)→\d+\/tcp$/);
          return m ? `<a class="chip" href="http://${esc(host)}:${m[1]}" target="_blank" rel="noopener" title="Open in a new tab">${esc(p)}${ic('ext')}</a>` : `<span class="chip">${esc(p)}</span>`;
        }).join('');
        html += `<div class="card ctr ${on ? '' : 'off'}" data-id="${esc(c.id)}">
          <div class="row" style="align-items:flex-start">
            <span class="dot ${dotc} ${on ? 'pulse' : ''}" style="margin-top:7px"></span>
            <div class="grow"><div class="nm trunc">${esc(c.name)}</div><div class="img trunc" title="${esc(c.image)}">${esc(c.image)}</div></div>
            ${c.health ? `<span class="badge ${c.health === 'healthy' ? 'ok' : c.health === 'unhealthy' ? 'crit' : 'warn'}">${esc(c.health)}</span>` : ''}
          </div>
          <div class="mute" style="font-size:12px">${esc(c.status)}</div>
          ${on ? `<div class="stats">
            <div><div class="label"><span>CPU</span><b class="num" style="color:var(--text)">${c.cpu.toFixed(1)}%</b></div><canvas class="spark" data-spark="${esc(c.id)}"></canvas></div>
            <div><div class="label"><span>Memory</span><b class="num" style="color:var(--text)">${fmtB(c.mem)}</b></div>${bar(mp, lvl(mp, 75, 90))}<div class="label" style="margin-top:5px">${c.memLimit ? 'of ' + fmtB(c.memLimit) + ' limit' : 'no limit'}</div></div>
          </div>` : ''}
          ${ports ? `<div class="chips">${ports}</div>` : ''}
          <div class="foot">
            ${on ? `<button class="ib warn" data-a="stop" title="Stop">${ic('stop')}</button>` : `<button class="ib ok" data-a="start" title="Start">${ic('play')}</button>`}
            <button class="ib" data-a="restart" title="Restart">${ic('restart')}</button>
            <button class="ib" data-a="logs" title="Logs">${ic('list')}</button>
            <button class="ib" data-a="shell" title="Shell" ${on ? '' : 'disabled'}>${ic('term')}</button>
            <button class="ib" data-a="inspect" title="Inspect">${ic('info')}</button>
            <button class="ib" data-a="more" title="More">${ic('more')}</button>
            <label class="bootsel" title="Restart policy — whether this container comes back after a reboot">${ic('power')}${policySelect(c.id, c.policy)}</label>
          </div></div>`;
      }
      html += '</div></div>';
    }
    body.innerHTML = html;
    $$('canvas[data-spark]', body).forEach(cv => spark(cv, ctrHist[cv.dataset.spark], '--acc'));
  },
  async click(e) {
    const st = e.target.closest('[data-stack]');
    if (st) return this.stack(st.dataset.p, st.dataset.stack);
    const sm = e.target.closest('[data-stackmenu]');
    if (sm) {
      e.stopPropagation();
      return showMenu(sm, [
        { label: 'Up (create & start)', icon: 'play', fn: () => this.stack(sm.dataset.p, 'up') },
        { label: 'Start', icon: 'play', fn: () => this.stack(sm.dataset.p, 'start') },
        { label: 'Stop', icon: 'stop', fn: () => this.stack(sm.dataset.p, 'stop') },
        '-',
        { label: 'Down (remove containers)', icon: 'trash', cls: 'crit', fn: () => this.stack(sm.dataset.p, 'down') },
      ]);
    }
    const b = e.target.closest('[data-a]');
    if (!b) return;
    if (b.dataset.a === 'rmi') {
      if (await confirmBox('Remove image?', `<span class="mono">${esc(b.dataset.n)}</span>`, { ok: 'Remove', danger: true }))
        act(() => post('/api/docker/images/remove', { id: b.dataset.id }), 'Image removed').then(() => this.poll()).catch(() => { });
      return;
    }
    const card = b.closest('[data-id]');
    const c = card && this.d && this.d.containers.find(x => x.id === card.dataset.id);
    if (!c) return;
    if (b.dataset.a === 'more') e.stopPropagation();
    ctrAction(c, b.dataset.a, b, () => this.poll().catch(() => { }));
  },
  async stack(proj, action) {
    if (action === 'down' && !(await confirmBox(`Take down “${esc(proj)}”?`, 'Every container in the stack is stopped and <b>removed</b>. Volumes are kept.', { ok: 'Down', danger: true }))) return;
    const o = drawer({ title: `${esc(proj)} <span class="mute" style="font-weight:400">compose ${esc(action)}</span>`, sub: 'Streaming output…', body: '<pre class="out" style="max-height:none;min-height:60vh"></pre>' });
    const pre = $('pre', o.body);
    try {
      await streamText('/api/docker/stack', { project: proj, action }, t => { pre.textContent += t; pre.scrollTop = pre.scrollHeight; });
      $('.drawer-h .sub', o.el).textContent = 'Finished';
      toast(`compose ${action} finished for ${proj}`, 'ok');
    } catch (err) { pre.textContent += '\n' + err.message; toast(err.message, 'crit'); }
    this.poll().catch(() => { });
  },
  pruneMenu(anchor) {
    const run = async (what, label) => {
      if (!(await confirmBox(`Remove ${label}?`, 'This cannot be undone.', { ok: 'Clean up', danger: true }))) return;
      const r = await act(() => post('/api/docker/prune', { what }));
      toast(`Reclaimed ${fmtB(r.reclaimed)}`, 'ok');
      this.poll().catch(() => { });
    };
    showMenu(anchor, [
      { label: 'Stopped containers', icon: 'box', fn: () => run('containers', 'all stopped containers') },
      { label: 'Dangling images', icon: 'layers', fn: () => run('dangling', 'dangling images') },
      { label: 'All unused images', icon: 'layers', fn: () => run('images', 'every image not used by a container') },
      { label: 'Build cache', icon: 'pkg', fn: () => run('build', 'the build cache') },
      { label: 'Unused networks', icon: 'wifi', fn: () => run('networks', 'unused networks') },
      '-',
      { label: 'Unused volumes', icon: 'disk', cls: 'crit', fn: () => run('volumes', 'unused volumes — their data will be lost') },
    ]);
  },
};

function ctrAction(c, a, anchor, after) {
  const call = (action, msg) => act(() => post(`/api/docker/c/${c.id}/${action}`), msg).then(after, () => { });
  switch (a) {
    case 'start': return call('start', `${c.name} started`);
    case 'stop': return call('stop', `${c.name} stopped`);
    case 'restart': return call('restart', `${c.name} restarted`);
    case 'logs': return ctrLogs(c);
    case 'shell': return openTerminal({ ctr: c.id, name: c.name });
    case 'inspect': return ctrInspect(c);
    case 'more': return showMenu(anchor, [
      c.state === 'paused' ? { label: 'Unpause', icon: 'play', fn: () => call('unpause', 'Unpaused') } : { label: 'Pause', icon: 'pause', fn: () => call('pause', 'Paused') },
      { label: 'Kill', icon: 'zap', fn: () => call('kill', 'Killed') },
      '-',
      { label: 'Remove container', icon: 'trash', cls: 'crit', fn: async () => { if (await confirmBox(`Remove ${esc(c.name)}?`, 'The container is force-removed. Named volumes are kept.', { ok: 'Remove', danger: true })) call('remove', 'Removed'); } },
    ]);
  }
}

// generic live log viewer used by drawers; returns a stop() function
function logView(host, url, fmtLine, height = '') {
  host.innerHTML = `<div class="toolbar" style="margin:0 0 10px">
      <div class="search">${ic('search')}<input class="in" data-lq placeholder="Filter lines…"></div>
      <label class="check"><span class="switch"><input type="checkbox" data-lf checked><i></i></span>Follow</label>
      <div class="r"><span class="mute num" data-ln></span><button class="ib" data-ldl title="Download">${ic('download')}</button></div></div>
    <div class="logview short" data-lv ${height ? `style="height:${height}"` : ''}></div>`;
  const lv = $('[data-lv]', host);
  const lines = [];
  let q = '', follow = true;
  const match = l => !q || JSON.stringify(l).toLowerCase().includes(q);
  const fmt = l => { const h = fmtLine(l); return q ? h.replace(new RegExp(`(>[^<]*?)(${reEsc(esc(q))})`, 'gi'), '$1<mark>$2</mark>') : h; };
  const rerender = () => { lv.innerHTML = lines.filter(match).slice(-3000).map(fmt).join(''); if (follow) lv.scrollTop = lv.scrollHeight; };
  $('[data-lq]', host).oninput = debounce(e => { q = e.target.value.toLowerCase(); rerender(); }, 150);
  $('[data-lf]', host).onchange = e => { follow = e.target.checked; if (follow) lv.scrollTop = lv.scrollHeight; };
  $('[data-ldl]', host).onclick = () => download('logs.txt', lines.map(l => Array.isArray(l) ? l.join(' ') : l).join('\n'));
  lv.addEventListener('scroll', () => {
    const atEnd = lv.scrollHeight - lv.scrollTop - lv.clientHeight < 30;
    if (!atEnd && follow) { follow = false; $('[data-lf]', host).checked = false; }
  });
  const src = new EventSource(url);
  src.addEventListener('l', e => {
    const batch = JSON.parse(e.data);
    lines.push(...batch);
    if (lines.length > 5000) lines.splice(0, lines.length - 5000);
    lv.insertAdjacentHTML('beforeend', batch.filter(match).map(fmt).join(''));
    while (lv.childElementCount > 3000) lv.firstElementChild.remove();
    $('[data-ln]', host).textContent = plural(lines.length, 'line');
    if (follow) lv.scrollTop = lv.scrollHeight;
  });
  src.addEventListener('end', () => src.close());
  src.onerror = () => { if (src.readyState === 2) lv.insertAdjacentHTML('beforeend', '<div class="ll mute">— stream closed —</div>'); };
  return () => src.close();
}

function ctrLogs(c) {
  let stop;
  const o = drawer({ title: `${esc(c.name)} <span class="mute" style="font-weight:400">logs</span>`, sub: esc(c.image), onClose: () => stop && stop() });
  stop = logView(o.body, `/api/docker/c/${encodeURIComponent(c.id)}/logs?tail=500`,
    l => `<div class="ll ${l[1] === 2 ? 'err' : ''}"><span class="ts">${esc((l[0] || '').slice(11, 19))}</span><span class="msg">${esc(l[2])}</span></div>`, 'calc(100vh - 170px)');
}

async function ctrInspect(c) {
  const o = drawer({ title: esc(c.name), sub: `<span class="mono">${esc(c.id.slice(0, 12))}</span> · ${esc(c.image)}`, body: '<div class="skel"></div><div class="skel"></div>' });
  try {
    const x = await api('/api/docker/c/' + c.id);
    const cfg = x.Config || {}, st = x.State || {}, hc = x.HostConfig || {};
    const env = (cfg.Env || []).map(e => { const i = e.indexOf('='); return [e.slice(0, i), e.slice(i + 1)]; });
    const secretish = k => /pass|secret|token|key|auth|cred/i.test(k);
    o.body.innerHTML = `
      <dl class="kvs">
        <dt>State</dt><dd>${esc(st.Status)} ${st.Running ? `· since ${esc(new Date(st.StartedAt).toLocaleString())}` : st.FinishedAt && !st.FinishedAt.startsWith('0001') ? `· exit ${esc(st.ExitCode)} at ${esc(new Date(st.FinishedAt).toLocaleString())}` : ''}</dd>
        <dt>Restarts</dt><dd>${x.RestartCount ?? 0} · policy <b>${esc((hc.RestartPolicy || {}).Name || 'no')}</b></dd>
        <dt>Command</dt><dd class="mono">${esc([...(cfg.Entrypoint || []), ...(cfg.Cmd || [])].join(' ') || '—')}</dd>
        <dt>Working dir</dt><dd class="mono">${esc(cfg.WorkingDir || '/')}</dd>
        <dt>User</dt><dd>${esc(cfg.User || 'root (image default)')}</dd>
        <dt>Network mode</dt><dd>${esc(hc.NetworkMode)}</dd>
        <dt>Privileged</dt><dd>${hc.Privileged ? '<span class="badge warn">yes</span>' : 'no'}</dd>
        <dt>Created</dt><dd>${esc(new Date(x.Created).toLocaleString())}</dd>
      </dl>
      <div><h4>Networks</h4>${Object.entries((x.NetworkSettings || {}).Networks || {}).map(([n, v]) => `<div class="li"><b class="grow">${esc(n)}</b><span class="mono">${esc(v.IPAddress || '—')}</span></div>`).join('') || '<span class="mute">none</span>'}</div>
      <div><h4>Mounts</h4>${(x.Mounts || []).map(m => `<div class="li"><span class="badge mute">${esc(m.Type)}</span><span class="mono grow trunc" title="${esc(m.Source)}">${esc(m.Source || m.Name)}</span><span class="dim">→</span><span class="mono">${esc(m.Destination)}</span>${m.RW ? '' : '<span class="badge info">ro</span>'}</div>`).join('') || '<span class="mute">none</span>'}</div>
      <div><h4>Environment</h4><div class="col" style="gap:4px">${env.map(([k, v]) => `<div class="row mono" style="font-size:12px;gap:4px"><b>${esc(k)}</b>=${secretish(k) ? `<span class="grow trunc" data-secret="${esc(v)}">•••••••• <a href="javascript:void 0" data-reveal>reveal</a></span>` : `<span class="grow trunc" title="${esc(v)}">${esc(v)}</span>`}</div>`).join('') || '<span class="mute">none</span>'}</div></div>
      <div><h4>Labels</h4><pre class="out">${esc(Object.entries(cfg.Labels || {}).map(([k, v]) => `${k}=${v}`).join('\n') || '—')}</pre></div>`;
    o.body.onclick = e => { const r = e.target.closest('[data-reveal]'); if (r) { const s = r.parentElement; s.textContent = s.dataset.secret; } };
  } catch (e) { o.body.innerHTML = `<div class="empty">${ic('alert')}<b>${esc(e.message)}</b></div>`; }
}

// ═══════════════════════════════════════════════════════════════════════════
//  page: services
// ═══════════════════════════════════════════════════════════════════════════

const CRITICAL_UNITS = /^(ssh|sshd|docker|containerd|networking|NetworkManager|dhcpcd|systemd-.*|dbus|iseall|wpa_supplicant|polkit)\.service$/;
P.services = {
  every: 4000,
  mount(el, q) {
    this.el = el;
    this.q = '';
    this.f = q.get('f') || 'running';
    el.innerHTML = `
      <div class="toolbar">
        <div class="search">${ic('search')}<input class="in" id="sv-q" placeholder="Filter services…  ( / )"></div>
        <div class="btn-group" id="sv-f">${[['running', 'Running'], ['failed', 'Failed'], ['enabled', 'Start at boot'], ['disabled', 'Disabled'], ['mine', 'Startup apps'], ['all', 'All']].map(([k, l]) => `<button data-f="${k}">${l}</button>`).join('')}</div>
        <div class="r mute num" id="sv-n"></div>
      </div>
      <div class="card flush"><div class="tbl-wrap" style="max-height:calc(100vh - 200px)"><table class="tbl"><thead><tr>
        <th>Service</th><th>State</th><th title="Start at boot">Boot</th><th class="hide-sm">CPU</th><th class="hide-sm r">Memory</th><th class="hide-sm r">Tasks</th><th></th></tr></thead>
        <tbody id="sv-b"><tr><td colspan="7"><div class="skel"></div><div class="skel"></div></td></tr></tbody></table></div></div>`;
    $('#sv-q', el).oninput = debounce(e => { this.q = e.target.value.toLowerCase(); this.render(); }, 120);
    $('#sv-f', el).onclick = e => { const b = e.target.closest('[data-f]'); if (b) { this.f = b.dataset.f; this.render(); } };
    $('#sv-b', el).onclick = e => {
      const tr = e.target.closest('tr[data-u]'); if (!tr || !this.d) return;
      const s = this.d.find(x => x.unit === tr.dataset.u); if (!s) return;
      const b = e.target.closest('[data-a]');
      if (b) { e.stopPropagation(); svcAction(s, b.dataset.a, b, () => this.poll().catch(() => { })); return; }
      if (e.target.closest('.switch')) return;
      svcDrawer(s.unit);
    };
    $('#sv-b', el).onchange = e => {
      const t = e.target.closest('[data-boot]'); if (!t) return;
      const s = this.d.find(x => x.unit === t.dataset.boot);
      svcAction(s, t.checked ? 'enable' : 'disable', null, () => this.poll().catch(() => { }));
    };
  },
  async poll() { const d = await api('/api/services'); if (pageId !== 'services') return; this.d = d; this.render(); },
  render() {
    $$('#sv-f button').forEach(b => b.classList.toggle('on', b.dataset.f === this.f));
    if (!this.d) return;
    const f = this.f, q = this.q;
    let list = this.d.filter(s => {
      if (f === 'running') return ['active', 'activating', 'reloading'].includes(s.active);
      if (f === 'failed') return s.active === 'failed';
      if (f === 'enabled') return s.file === 'enabled';
      if (f === 'disabled') return s.file === 'disabled';
      if (f === 'mine') return s.mine;
      return true;
    });
    if (q) list = list.filter(s => (s.unit + ' ' + s.desc).toLowerCase().includes(q));
    const failed = this.d.filter(s => s.active === 'failed').length;
    $('#sv-n').innerHTML = `${list.length} shown · ${this.d.filter(s => s.active === 'active').length} active${failed ? ` · <span class="crit-t">${failed} failed</span>` : ''}`;
    $('#sv-b').innerHTML = list.map(s => {
      const on = s.active === 'active';
      const dot = s.active === 'failed' ? 'crit' : on ? 'ok' : s.active === 'activating' ? 'warn' : '';
      const boot = s.file === 'enabled' || s.file === 'disabled'
        ? `<label class="switch" title="${s.file === 'enabled' ? 'Starts at boot' : 'Does not start at boot'}"><input type="checkbox" data-boot="${esc(s.unit)}" ${s.file === 'enabled' ? 'checked' : ''}><i></i></label>`
        : `<span class="badge mute">${esc(s.file || '—')}</span>`;
      return `<tr class="click" data-u="${esc(s.unit)}">
        <td><div class="row" style="gap:10px"><span class="dot ${dot}"></span><div style="min-width:0"><div style="font-weight:600" class="trunc">${esc(s.unit.replace(/\.service$/, ''))}${s.mine ? ' <span class="badge acc">startup app</span>' : ''}</div><div class="sub trunc" style="max-width:440px">${esc(s.desc)}</div></div></div></td>
        <td><span class="badge ${s.active === 'failed' ? 'crit' : on ? 'ok' : s.active === 'activating' ? 'warn' : 'mute'}">${esc(s.sub)}</span></td>
        <td>${boot}</td>
        <td class="hide-sm">${on ? `<span class="num">${s.cpu.toFixed(1)}%</span>` : '<span class="dim">—</span>'}</td>
        <td class="hide-sm r num">${s.mem ? fmtB(s.mem) : '<span class="dim">—</span>'}</td>
        <td class="hide-sm r num mute">${s.tasks || ''}</td>
        <td class="acts">
          ${on ? `<button class="ib warn" data-a="stop" title="Stop">${ic('stop')}</button><button class="ib" data-a="restart" title="Restart">${ic('restart')}</button>`
              : `<button class="ib ok" data-a="start" title="Start">${ic('play')}</button>`}
          <button class="ib" data-a="logs" title="Logs">${ic('list')}</button>
          <button class="ib" data-a="more" title="More">${ic('more')}</button></td></tr>`;
    }).join('') || `<tr><td colspan="7"><div class="empty">${ic(f === 'failed' ? 'okc' : 'search')}<b>${f === 'failed' ? 'Nothing has failed' : 'No services match'}</b></div></td></tr>`;
  },
};

async function svcAction(s, a, anchor, after = () => { }) {
  const u = s.unit;
  const name = u.replace(/\.service$/, '');
  if (['stop', 'restart', 'disable', 'mask', 'disable-now'].includes(a) && CRITICAL_UNITS.test(u)) {
    if (!(await confirmBox(`${a[0].toUpperCase() + a.slice(1)} ${esc(name)}?`, `<b>${esc(name)}</b> is a core system service. This may cut off remote access or break the machine.`, { ok: a, danger: true }))) { after(); return; }
  }
  const call = (action, msg) => act(() => post(`/api/services/${encodeURIComponent(u)}/${action}`), msg).then(after, after);
  switch (a) {
    case 'start': return call('start', `${name} started`);
    case 'stop': return call('stop', `${name} stopped`);
    case 'restart': return call('restart', `${name} restarted`);
    case 'enable': return call('enable', `${name} will start at boot`);
    case 'disable': return call('disable', `${name} won't start at boot`);
    case 'mask': return call('mask', `${name} masked`);
    case 'logs': return navigate('logs', 'unit=' + encodeURIComponent(u));
    case 'more': return showMenu(anchor, [
      { label: 'Details', icon: 'info', fn: () => svcDrawer(u) },
      { label: 'Reload config', icon: 'restart', fn: () => call('reload', 'Reloaded') },
      { label: 'Enable & start now', icon: 'power', fn: () => call('enable-now', 'Enabled and started') },
      { label: 'Disable & stop now', icon: 'stop', fn: () => svcAction(s, 'disable-now', null, after) },
      s.active === 'failed' ? { label: 'Clear failed state', icon: 'check', fn: () => call('reset-failed', 'Cleared') } : null,
      '-',
      s.file === 'masked' ? { label: 'Unmask', icon: 'eye', fn: () => call('unmask', 'Unmasked') } : { label: 'Mask (block every start)', icon: 'lock', cls: 'crit', fn: () => svcAction(s, 'mask', null, after) },
    ].filter(Boolean));
    case 'disable-now': return call('disable-now', 'Disabled and stopped');
  }
}

const journalLine = l => `<div class="ll p${l[1]}"><span class="ts">${clock(l[0])}</span><span class="id">${esc(l[2])}</span><span class="msg">${esc(l[3])}</span></div>`;

async function svcDrawer(unit) {
  let stop;
  const o = drawer({ title: esc(unit), sub: 'loading…', onClose: () => stop && stop(), body: '<div class="skel"></div>' });
  const load = async () => {
    stop && stop();
    const x = await api('/api/services/' + encodeURIComponent(unit));
    if (o.closed) return;
    const p = x.props;
    const on = p.ActiveState === 'active';
    $('.drawer-h .sub', o.el).innerHTML = `<span class="badge ${p.ActiveState === 'failed' ? 'crit' : on ? 'ok' : 'mute'}">${esc(p.ActiveState)} · ${esc(p.SubState)}</span>&nbsp; ${esc(p.Description)}`;
    o.body.innerHTML = `
      <div class="row wrap">
        ${on ? `<button class="btn" data-a="stop">${ic('stop')}Stop</button><button class="btn" data-a="restart">${ic('restart')}Restart</button>` : `<button class="btn pri" data-a="start">${ic('play')}Start</button>`}
        ${p.UnitFileState === 'enabled' ? `<button class="btn" data-a="disable">${ic('power')}Don't start at boot</button>` : p.UnitFileState === 'disabled' ? `<button class="btn" data-a="enable">${ic('power')}Start at boot</button>` : ''}
        <button class="btn ghost" data-a="logs">${ic('ext')}Full logs</button>
      </div>
      <dl class="kvs">
        <dt>Boot</dt><dd>${esc(p.UnitFileState || '—')}</dd>
        <dt>Main PID</dt><dd class="mono">${p.MainPID && p.MainPID !== '0' ? esc(p.MainPID) : '—'}</dd>
        <dt>Active since</dt><dd>${esc(p.ActiveEnterTimestamp || '—')}</dd>
        <dt>Memory</dt><dd>${/^\d+$/.test(p.MemoryCurrent || '') ? fmtB(+p.MemoryCurrent) : '—'}</dd>
        <dt>CPU time</dt><dd>${/^\d+$/.test(p.CPUUsageNSec || '') ? fmtDur(+p.CPUUsageNSec / 1e9) : '—'}</dd>
        <dt>Restarts</dt><dd>${esc(p.NRestarts || '0')} · policy ${esc(p.Restart || '—')}</dd>
        <dt>User</dt><dd>${esc(p.User || 'root')}</dd>
        <dt>Unit file</dt><dd class="mono">${esc(p.FragmentPath || '—')}</dd>
        ${p.Result && p.Result !== 'success' ? `<dt>Last result</dt><dd><span class="badge crit">${esc(p.Result)}</span></dd>` : ''}
      </dl>
      <div><h4>Live log</h4><div data-logs></div></div>
      <div><h4>Unit definition</h4><pre class="out">${esc(x.file)}</pre></div>`;
    o.body.onclick = e => {
      const b = e.target.closest('[data-a]'); if (!b) return;
      if (b.dataset.a === 'logs') { o.close(); return navigate('logs', 'unit=' + encodeURIComponent(unit)); }
      svcAction({ unit, file: p.UnitFileState, active: p.ActiveState }, b.dataset.a, b, () => { load().catch(() => { }); if (pageId === 'services') page.poll().catch(() => { }); });
    };
    stop = logView($('[data-logs]', o.body), `/api/logs?unit=${encodeURIComponent(unit)}&n=150`, journalLine, '34vh');
  };
  load().catch(e => { o.body.innerHTML = `<div class="empty">${ic('alert')}<b>${esc(e.message)}</b></div>`; });
}

// ═══════════════════════════════════════════════════════════════════════════
//  page: startup — everything that runs at boot, in one place
// ═══════════════════════════════════════════════════════════════════════════

P.startup = {
  mount(el) {
    this.el = el;
    el.innerHTML = `<div class="summary">${'<div class="card"><div class="skel"></div><div class="skel"></div></div>'.repeat(4)}</div><div class="card"><div class="skel"></div><div class="skel"></div></div>`;
    el.onclick = e => this.click(e);
    el.onchange = e => {
      const s = e.target.closest('select[data-policy]');
      if (s) act(() => post(`/api/docker/c/${s.dataset.policy}/policy`, { policy: s.value }), `Restart policy → ${s.value}`).then(() => this.load()).catch(() => { });
      const t = e.target.closest('[data-app-en]');
      if (t) act(() => post(`/api/services/${encodeURIComponent(t.dataset.appEn)}/${t.checked ? 'enable' : 'disable'}`), t.checked ? 'Will start at boot' : 'Won\'t start at boot').then(() => this.load()).catch(() => { });
    };
    this.load().catch(e => { el.innerHTML = `<div class="card empty">${ic('alert')}<b>${esc(e.message)}</b></div>`; });
  },
  async load() {
    const [b, sch] = await Promise.all([api('/api/boot'), api('/api/schedule').catch(() => ({}))]);
    if (pageId !== 'startup') return;
    this.b = b;
    this.render(b, sch);
  },
  render(b, sch) {
    const blame = b.blame || [];
    const secs = s => { let t = 0; s.replace(/([\d.]+)(min|ms|s|us)/g, (_, n, u) => { t += +n * { min: 60, s: 1, ms: .001, us: 1e-6 }[u]; }); return t; };
    const maxB = Math.max(...blame.map(x => secs(x[0])), .001);
    const total = ((b.bootTime || '').match(/=\s*(.+)$/) || [])[1] || '—';
    const apps = b.apps || [];
    const ctrs = b.containers || [];
    const ctrBoot = ctrs.filter(c => c.policy && c.policy !== 'no').length;
    const autos = [
      ...(b.cronReboot || []).map(j => ({ kind: '@reboot cron', name: j.user, exec: j.cmd, src: j.source })),
      ...(b.autostart || []).map(a => ({ kind: a.kind, name: a.name, exec: a.exec, src: a.source })),
      ...(b.rcLocal || []).map(l => ({ kind: 'rc.local', name: '', exec: l, src: '/etc/rc.local' })),
    ];
    this.el.innerHTML = `
      <div class="summary">
        <div class="card hl"><div class="label">Last boot took</div><div class="big">${esc(total)}</div></div>
        <div class="card"><div class="label">Your startup apps</div><div class="big">${apps.length}</div></div>
        <div class="card"><div class="label">Containers that auto-start</div><div class="big">${ctrBoot}<small class="mute" style="font-size:14px"> / ${ctrs.length}</small></div></div>
        <div class="card"><div class="label">Services enabled at boot</div><div class="big">${(b.enabled || []).length}</div></div>
      </div>

      <div class="card">
        <div class="card-h"><h3>${ic('rocket')} Startup apps</h3><div class="r"><button class="btn pri" data-a="new">${ic('plus')}New startup app</button></div></div>
        <p class="mute" style="margin:-4px 0 14px;font-size:13px">Programs and scripts that launch at power-on. Each one is a real systemd service: supervised, restarted on crash, and logged.</p>
        <div class="grid g3">${apps.map(a => {
          const on = a.active === 'active';
          const n = esc(a.name);
          return `<div class="card app-card" style="background:var(--raise)">
            <div class="row"><span class="dot ${a.active === 'failed' ? 'crit' : on ? 'ok pulse' : ''}"></span><b class="grow trunc">${n}</b>
              <label class="switch" title="Start at boot"><input type="checkbox" data-app-en="iseall-${n}.service" ${a.enabled === 'enabled' ? 'checked' : ''}><i></i></label></div>
            <div class="cmd" title="${esc(a.command)}">${esc(a.command)}</div>
            <div class="chips"><span class="badge ${a.active === 'failed' ? 'crit' : on ? 'ok' : 'mute'}">${esc(a.sub || a.active || 'unknown')}</span>
              ${a.user ? `<span class="chip">${ic('user')}${esc(a.user)}</span>` : ''}<span class="chip">restart: ${esc(a.restart)}</span>${a.delay ? `<span class="chip">${ic('clock')}${a.delay}s delay</span>` : ''}${a.network ? `<span class="chip">${ic('globe')}after network</span>` : ''}</div>
            <div class="row" style="gap:2px;margin:0 -6px -6px">
              ${on ? `<button class="ib warn" data-a="app" data-op="stop" data-n="${n}" title="Stop">${ic('stop')}</button>` : `<button class="ib ok" data-a="app" data-op="start" data-n="${n}" title="Start">${ic('play')}</button>`}
              <button class="ib" data-a="app" data-op="restart" data-n="${n}" title="Restart">${ic('restart')}</button>
              <button class="ib" data-a="app" data-op="logs" data-n="${n}" title="Logs">${ic('list')}</button>
              <button class="ib" data-a="edit" data-n="${n}" title="Edit">${ic('edit')}</button>
              <button class="ib crit" data-a="del" data-n="${n}" title="Delete" style="margin-left:auto">${ic('trash')}</button></div>
          </div>`;
        }).join('') || `<div class="empty" style="grid-column:1/-1">${ic('rocket')}<b>No startup apps yet</b>Add a script, binary or command to run every time the machine powers on.</div>`}</div>
      </div>

      <div class="grid g2 mt">
        <div class="card flush"><div class="card-h"><h3>${ic('box')} Containers at boot</h3><div class="r mute" style="font-size:12px">restart policy</div></div>
          <div class="tbl-wrap" style="max-height:420px"><table class="tbl"><tbody>${ctrs.map(c => `<tr>
            <td><div class="row"><span class="dot ${c.state === 'running' ? 'ok' : ''}"></span><b>${esc(c.name)}</b></div></td>
            <td class="r">${c.policy === 'no' ? '<span class="badge mute">manual</span>' : '<span class="badge ok">auto-starts</span>'}</td>
            <td class="r" style="width:150px">${policySelect(c.id, c.policy)}</td></tr>`).join('') || '<tr><td class="mute" style="padding:18px">No containers</td></tr>'}</tbody></table></div></div>
        <div class="card"><div class="card-h"><h3>${ic('clock')} Boot timeline</h3><div class="r mute" style="font-size:12px">slowest units</div></div>
          <div class="blame">${blame.slice(0, 12).map(([t, u]) => { const v = secs(t); return `<div class="blame-r"><span class="trunc mono" title="${esc(u)}">${esc(u)}</span>${bar(v / maxB * 100, v > 10 ? 'crit' : v > 3 ? 'warn' : '')}<span class="num mute" style="text-align:right">${esc(t)}</span></div>`; }).join('') || '<span class="mute">systemd-analyze unavailable</span>'}</div>
        </div>
      </div>

      <div class="card mt"><div class="card-h"><h3>${ic('layers')} Services enabled at boot</h3><div class="r"><button class="btn sm ghost" data-go="services" data-f="enabled">Manage ${ic('ext')}</button></div></div>
        <div class="chips">${(b.enabled || []).map(u => `<a class="chip" href="javascript:void 0" data-svc="${esc(u)}">${esc(u.replace(/\.service$/, ''))}</a>`).join('')}</div></div>

      <div class="grid g2 mt">
        <div class="card flush"><div class="card-h"><h3>${ic('cal')} Timers</h3></div><div class="tbl-wrap" style="max-height:380px"><table class="tbl"><thead><tr><th>Timer</th><th>Next run</th><th>Last run</th></tr></thead><tbody>
          ${(sch.timers || []).filter(t => t.active === 'active').map(t => `<tr><td><b>${esc(t.unit.replace(/\.timer$/, ''))}</b><div class="sub">${esc(t.desc)}</div></td><td class="mute" style="font-size:12px">${esc(t.next || '—')}</td><td class="mute" style="font-size:12px">${esc(t.last || '—')}</td></tr>`).join('') || '<tr><td colspan="3" class="mute">No active timers</td></tr>'}
        </tbody></table></div></div>
        <div class="card flush"><div class="card-h"><h3>${ic('clock')} Cron jobs</h3></div><div class="tbl-wrap" style="max-height:380px"><table class="tbl"><thead><tr><th>Schedule</th><th>User</th><th>Command</th></tr></thead><tbody>
          ${(sch.cron || []).map(j => `<tr><td class="mono" style="white-space:nowrap">${esc(j.sched)}</td><td>${esc(j.user)}</td><td><div class="cmd" style="max-width:340px" title="${esc(j.cmd)}">${esc(j.cmd)}</div><div class="sub">${esc(j.source)}</div></td></tr>`).join('') || '<tr><td colspan="3" class="mute">No cron jobs</td></tr>'}
        </tbody></table></div></div>
      </div>

      ${autos.length ? `<div class="card flush mt"><div class="card-h"><h3>${ic('power')} Other things that run at boot</h3></div><div class="tbl-wrap"><table class="tbl"><tbody>
        ${autos.map(a => `<tr><td style="width:130px"><span class="badge vio">${esc(a.kind)}</span></td><td><b>${esc(a.name)}</b><div class="cmd" style="max-width:600px">${esc(a.exec)}</div></td><td class="mono mute hide-sm" style="font-size:11.5px">${esc(a.src)}</td></tr>`).join('')}
      </tbody></table></div></div>` : ''}`;
  },
  async click(e) {
    const g = e.target.closest('[data-go]');
    if (g) return navigate(g.dataset.go, g.dataset.f ? 'f=' + g.dataset.f : '');
    const sv = e.target.closest('[data-svc]');
    if (sv) return svcDrawer(sv.dataset.svc);
    const b = e.target.closest('[data-a]');
    if (!b) return;
    const a = ((this.b && this.b.apps) || []).find(x => x.name === b.dataset.n);
    const reload = () => this.load().catch(() => { });
    switch (b.dataset.a) {
      case 'new': return appForm(null, reload);
      case 'edit': return appForm(a, reload);
      case 'del':
        if (await confirmBox(`Delete “${esc(a.name)}”?`, 'The program is stopped and removed from startup.', { ok: 'Delete', danger: true }))
          act(() => post(`/api/startup/${encodeURIComponent(a.name)}/delete`), 'Startup app deleted').then(reload, () => { });
        return;
      case 'app': {
        const unit = `iseall-${a.name}.service`;
        if (b.dataset.op === 'logs') return navigate('logs', 'unit=' + encodeURIComponent(unit));
        return act(() => post(`/api/services/${encodeURIComponent(unit)}/${b.dataset.op}`), `${a.name}: ${b.dataset.op}`).then(() => setTimeout(reload, 600), () => { });
      }
    }
  },
};

const APP_TEMPLATES = [
  ['Python script', { command: 'python3 main.py', workdir: '/home/pi/app', user: 'pi', restart: 'on-failure', network: true }],
  ['Node app', { command: 'node server.js', workdir: '/home/pi/app', user: 'pi', restart: 'always', network: true, env: 'NODE_ENV=production' }],
  ['Shell script', { command: '/home/pi/startup.sh', user: 'pi', restart: 'no', network: false }],
  ['Compose stack', { command: 'docker compose up', workdir: '/home/pi/stack', user: '', restart: 'always', network: true, delay: 10 }],
];

function appForm(a, done) {
  const v = a ? { ...a, env: (a.env || []).join('\n') } : { name: '', command: '', workdir: '', user: '', restart: 'on-failure', delay: 0, network: true, env: '' };
  const body = `
    ${a ? '' : `<div class="chips"><span class="mute" style="font-size:12px;align-self:center">Start from:</span>${APP_TEMPLATES.map(([n], i) => `<button class="btn sm" data-tpl="${i}">${esc(n)}</button>`).join('')}</div>`}
    <div class="field"><label>Name</label><input class="in mono" name="name" value="${esc(v.name)}" placeholder="my-bot" autocomplete="off"><div class="hint">lowercase, digits and dashes — becomes <span class="mono">iseall-name.service</span></div></div>
    <div class="field"><label>Command</label><input class="in mono" name="command" value="${esc(v.command)}" placeholder="python3 bot.py --verbose" autocomplete="off"><div class="hint">Runs through /bin/sh, so pipes, &amp;&amp; and $VARS work.</div></div>
    <div class="field-row">
      <div class="field"><label>Working directory</label><input class="in mono" name="workdir" value="${esc(v.workdir)}" placeholder="/home/pi/app"></div>
      <div class="field"><label>Run as user</label><input class="in mono" name="user" value="${esc(v.user)}" placeholder="root"></div>
    </div>
    <div class="field-row">
      <div class="field"><label>When it exits</label><select class="in" name="restart">
        <option value="on-failure" ${v.restart === 'on-failure' ? 'selected' : ''}>Restart if it crashes</option>
        <option value="always" ${v.restart === 'always' ? 'selected' : ''}>Always restart</option>
        <option value="no" ${v.restart === 'no' ? 'selected' : ''}>Run once</option></select></div>
      <div class="field"><label>Start delay (seconds)</label><input class="in" type="number" min="0" max="3600" name="delay" value="${v.delay || 0}"></div>
    </div>
    <div class="field"><label>Environment variables</label><textarea class="in mono" name="env" placeholder="KEY=value&#10;ANOTHER=thing">${esc(v.env)}</textarea></div>
    <label class="check"><span class="switch"><input type="checkbox" name="network" ${v.network ? 'checked' : ''}><i></i></span>Wait for the network before starting</label>
    <label class="check"><span class="switch"><input type="checkbox" name="startNow" checked><i></i></span>Start it right now as well</label>`;
  modal({
    title: a ? `Edit ${esc(a.name)}` : 'New startup app', body, ok: a ? 'Save' : 'Create', wide: true,
    onOpen: el => el.addEventListener('click', e => {
      const t = e.target.closest('[data-tpl]'); if (!t) return;
      const tpl = APP_TEMPLATES[+t.dataset.tpl][1];
      for (const [k, val] of Object.entries(tpl)) { const inp = el.querySelector(`[name="${k}"]`); if (!inp) continue; if (inp.type === 'checkbox') inp.checked = !!val; else inp.value = val; }
      el.querySelector('[name="name"]').focus();
    }),
    onOk: async el => {
      const f = n => el.querySelector(`[name="${n}"]`);
      const req = {
        original: a ? a.name : '', name: f('name').value.trim(), command: f('command').value.trim(), workdir: f('workdir').value.trim(),
        user: f('user').value.trim(), restart: f('restart').value, delay: +f('delay').value || 0, network: f('network').checked,
        env: f('env').value.split('\n').map(s => s.trim()).filter(Boolean), startNow: f('startNow').checked,
      };
      await post('/api/startup', req);
      toast(a ? 'Startup app saved' : `${req.name} will now start at boot`, 'ok');
    },
  }).then(r => r && done());
}

// ═══════════════════════════════════════════════════════════════════════════
//  page: ports
// ═══════════════════════════════════════════════════════════════════════════

P.ports = {
  every: 5000,
  mount(el) {
    this.el = el;
    this.tab = 'listen'; this.q = ''; this.f = 'all'; this.d = null;
    el.innerHTML = `
      <div class="summary" id="pt-sum">${'<div class="card"><div class="skel"></div><div class="skel"></div></div>'.repeat(5)}</div>
      <div class="tabs" id="pt-tabs"><button data-t="listen" class="on">${ic('plug')}Listening</button><button data-t="conns">${ic('link')}Connections</button></div>
      <div class="toolbar">
        <div class="search">${ic('search')}<input class="in" id="pt-q" placeholder="Port, process, container…  ( / )"></div>
        <div class="btn-group" id="pt-f"><button data-f="all" class="on">All</button><button data-f="public">Exposed</button><button data-f="local">Local only</button><button data-f="web">Web UIs</button><button data-f="tcp">TCP</button><button data-f="udp">UDP</button></div>
      </div>
      <div id="pt-b"></div>`;
    $('#pt-tabs', el).onclick = e => { const b = e.target.closest('[data-t]'); if (!b) return; this.tab = b.dataset.t; $$('#pt-tabs button').forEach(x => x.classList.toggle('on', x === b)); $('#pt-f').classList.toggle('hide', this.tab !== 'listen'); this.render(); };
    $('#pt-f', el).onclick = e => { const b = e.target.closest('[data-f]'); if (!b) return; this.f = b.dataset.f; $$('#pt-f button').forEach(x => x.classList.toggle('on', x === b)); this.render(); };
    $('#pt-q', el).oninput = debounce(e => { this.q = e.target.value.toLowerCase(); this.render(); }, 120);
    $('#pt-b', el).onclick = e => {
      const p = e.target.closest('[data-pid]');
      if (p) procDrawer({ pid: +p.dataset.pid, name: p.dataset.name });
    };
  },
  async poll() { const d = await api('/api/ports'); S.cache.ports = d; if (pageId !== 'ports') return; this.d = d; this.render(); },
  render() {
    const d = this.d;
    if (!d) return;
    const L = d.listen;
    const pub = L.filter(l => l.scope === 'public');
    const tcp = L.filter(l => l.proto === 'tcp').length;
    $('#pt-sum').innerHTML = [
      ['Listening', L.length, `${tcp} TCP · ${L.length - tcp} UDP`, ''],
      ['Exposed to network', pub.length, 'bound to every interface', pub.length ? 'warn-t' : 'ok-t'],
      ['Local only', L.filter(l => l.scope === 'local').length, 'loopback', ''],
      ['Web interfaces', L.filter(l => l.web).length, 'answer HTTP(S)', ''],
      ['Established', d.states.ESTABLISHED || 0, `${d.states.TIME_WAIT || 0} in time-wait`, ''],
    ].map(([l, v, s, c]) => `<div class="card"><div class="label">${l}</div><div class="big ${c}">${v}</div><div class="label">${s}</div></div>`).join('');
    const q = this.q;
    const host = location.hostname;
    if (this.tab === 'listen') {
      let list = L;
      if (this.f === 'public') list = list.filter(l => l.scope === 'public');
      if (this.f === 'local') list = list.filter(l => l.scope === 'local');
      if (this.f === 'web') list = list.filter(l => l.web);
      if (this.f === 'tcp' || this.f === 'udp') list = list.filter(l => l.proto === this.f);
      if (q) list = list.filter(l => `${l.port} ${l.proc} ${l.ctr || ''} ${l.unit || ''} ${l.svc || ''} ${(l.web || {}).title || ''}`.toLowerCase().includes(q));
      $('#pt-b').innerHTML = list.length ? `<div class="ports">${list.map(l => {
        const scope = l.scope === 'public' ? `<span class="badge warn" title="Reachable from the network">${ic('globe')}exposed</span>` : l.scope === 'lan' ? `<span class="badge info">${esc(l.addrs[0])}</span>` : `<span class="badge ok">${ic('lock')}local</span>`;
        const url = l.web && l.scope !== 'local' ? `${l.web.scheme}://${host}:${l.port}/` : null;
        return `<div class="card port">
          <div class="pn"><span>${l.port}</span><small>${l.proto}${l.svc ? ' · ' + esc(l.svc) : ''}</small></div>
          <div class="meta">
            <div class="row wrap" style="gap:6px">${scope}${l.conns ? `<span class="badge vio">${plural(l.conns, 'client')}</span>` : ''}</div>
            ${l.web ? `<div class="title trunc">${esc(l.web.title || (l.web.scheme.toUpperCase() + ' ' + l.web.status))}</div>` : ''}
            <div class="row wrap" style="gap:6px;font-size:12.5px">
              ${l.pid ? `<a href="javascript:void 0" class="chip" data-pid="${l.pid}" data-name="${esc(l.proc)}">${ic('cpu')}${esc(l.proc)} · ${l.pid}</a>` : `<span class="chip" title="Owner not visible — run iseall as root">${esc(l.proc || 'unknown owner')}</span>`}
              ${l.ctr ? `<span class="badge acc">${ic('box')}${esc(l.ctr)}</span>` : ''}${l.unit ? `<span class="badge vio">${esc(l.unit.replace(/\.service$/, ''))}</span>` : ''}
            </div>
            <div class="mono dim" style="font-size:11px">${l.addrs.slice(0, 3).map(esc).join('  ')}${l.addrs.length > 3 ? ` +${l.addrs.length - 3}` : ''}</div>
          </div>
          ${url ? `<a class="btn sm" href="${esc(url)}" target="_blank" rel="noopener">${ic('ext')}Open</a>` : ''}
        </div>`;
      }).join('')}</div>` : `<div class="card empty">${ic('plug')}<b>No ports match</b></div>`;
    } else {
      let cs = d.conns || [];
      if (q) cs = cs.filter(c => `${c.remote} ${c.rport} ${c.lport} ${c.proc} ${c.state}`.toLowerCase().includes(q));
      const byRemote = {};
      cs.forEach(c => { byRemote[c.remote] = (byRemote[c.remote] || 0) + 1; });
      const top = Object.entries(byRemote).sort((a, b) => b[1] - a[1]).slice(0, 8);
      $('#pt-b').innerHTML = `<div class="grid g3">
        <div class="card flush span2"><div class="tbl-wrap" style="max-height:calc(100vh - 390px);min-height:300px"><table class="tbl"><thead><tr><th>Remote</th><th>Local</th><th>Process</th><th>State</th><th>Dir</th></tr></thead><tbody>
          ${cs.map(c => `<tr><td class="mono">${esc(c.remote)}<span class="dim">:${c.rport}</span></td><td class="mono">${esc(c.local)}<span class="dim">:${c.lport}</span></td>
            <td>${c.pid ? `<a href="javascript:void 0" data-pid="${c.pid}" data-name="${esc(c.proc)}">${esc(c.proc)}</a>` : '<span class="dim">—</span>'}</td>
            <td><span class="badge ${c.state === 'ESTABLISHED' ? 'ok' : 'mute'}">${esc(c.state.toLowerCase().replace('_', ' '))}</span></td>
            <td>${c.in ? `<span class="badge info">${ic('down')}in</span>` : `<span class="badge vio">${ic('up')}out</span>`}</td></tr>`).join('') || '<tr><td colspan="5"><div class="empty">No connections</div></td></tr>'}
        </tbody></table></div></div>
        <div class="card"><div class="card-h"><h3>Top remotes</h3></div>${top.map(([ip, n]) => `<div class="li"><span class="mono grow trunc">${esc(ip)}</span><span class="badge mute">${n}</span></div>`).join('') || '<span class="mute">—</span>'}
          <div class="card-h" style="margin-top:18px"><h3>TCP states</h3></div><div class="col" style="gap:6px">${Object.entries(d.states).sort((a, b) => b[1] - a[1]).map(([k, v]) => `<div class="kv"><span>${esc(k.toLowerCase().replace('_', ' '))}</span><span>${v}</span></div>`).join('')}</div></div>
      </div>`;
    }
  },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: network
// ═══════════════════════════════════════════════════════════════════════════

P.network = {
  every: 10000,
  mount(el) {
    this.el = el;
    this.d = null;
    el.innerHTML = `
      <div class="grid g3">
        <div class="card span2"><div class="card-h"><h3>${ic('activity')} Throughput</h3><div class="r legend" id="nw-l"></div></div><div id="nw-c"></div></div>
        <div class="card"><div class="card-h"><h3>${ic('globe')} Routing</h3></div><dl class="kvs" id="nw-r"><dt>…</dt><dd></dd></dl></div>
      </div>
      <div class="grid g3 mt" id="nw-if"></div>`;
    this.ch = new LineChart($('#nw-c', el), { max: 0, min: 1024, fmt: fmtRate, fmtAxis: v => fmtB(v), series: [{ c: '--acc', label: 'Down' }, { c: '--acc-3', label: 'Up' }] });
    this.ch.set(H.t, [H.rx, H.tx]);
  },
  async poll() { const d = await api('/api/network'); if (pageId !== 'network') return; this.d = d; this.render(); },
  sample(s) {
    this.ch.set(H.t, [H.rx, H.tx]);
    $('#nw-l').innerHTML = `<span><i style="background:${COL['--acc']}"></i>↓ <b>${fmtRate(s.rx)}</b></span><span><i style="background:${COL['--acc-3']}"></i>↑ <b>${fmtRate(s.tx)}</b></span>`;
    (s.net || []).forEach(n => {
      const card = document.querySelector(`[data-if="${CSS.escape(n.n)}"]`);
      if (!card) return;
      $('[data-rx]', card).textContent = fmtRate(n.rx);
      $('[data-tx]', card).textContent = fmtRate(n.tx);
      spark($('canvas', card), S.net[n.n], '--acc');
    });
  },
  render() {
    const d = this.d;
    if (!d) return;
    $('#nw-r').innerHTML = `<dt>Hostname</dt><dd><b>${esc(d.hostname)}</b></dd><dt>Gateway</dt><dd class="mono">${esc(d.gateway || '—')} <span class="mute">${esc(d.gatewayIf || '')}</span></dd><dt>DNS</dt><dd class="mono">${(d.dns || []).map(esc).join('<br>') || '—'}</dd><dt>Interfaces</dt><dd>${d.ifaces.length}</dd><dt>Browsing from</dt><dd class="mono">${esc(location.host)}</dd>`;
    $('#nw-if').innerHTML = d.ifaces.map(i => {
      const up = i.state === 'up' || i.state === 'unknown';
      const w = i.wifi;
      const bars = w ? [1, 2, 3, 4].map(k => `<i style="display:inline-block;width:4px;margin-right:2px;border-radius:1px;height:${k * 4}px;background:${w.quality >= k * 25 - 10 ? 'var(--acc)' : 'var(--line-2)'}"></i>`).join('') : '';
      return `<div class="card" data-if="${esc(i.name)}">
        <div class="row"><span class="dot ${up ? 'ok' : ''}"></span><b style="font-size:15px">${esc(i.name)}</b>
          ${i.virtual ? '<span class="badge mute">virtual</span>' : ''}${w ? `<span class="badge info" title="${w.level} dBm">${esc(w.ssid || 'Wi-Fi')}</span>` : ''}
          <span style="margin-left:auto;display:flex;align-items:flex-end;height:16px">${bars}</span>
          ${i.speed > 0 ? `<span class="chip">${i.speed >= 1000 ? i.speed / 1000 + ' Gb/s' : i.speed + ' Mb/s'}</span>` : ''}</div>
        <canvas class="spark" style="height:40px;margin:12px 0 8px"></canvas>
        <div class="row" style="justify-content:space-between;font-size:12.5px"><span>↓ <b class="num" data-rx>—</b></span><span>↑ <b class="num" data-tx>—</b></span></div>
        <dl class="kvs" style="margin-top:12px;font-size:12.5px">
          <dt>Addresses</dt><dd class="mono">${(i.addrs || []).map(esc).join('<br>') || '—'}</dd>
          <dt>MAC</dt><dd class="mono">${esc(i.mac || '—')}</dd>
          <dt>Since boot</dt><dd>↓ ${fmtB(i.rxTotal)} · ↑ ${fmtB(i.txTotal)}</dd>
          <dt>MTU</dt><dd>${i.mtu}</dd>
          ${i.errors || i.drops ? `<dt>Errors / drops</dt><dd class="warn-t">${i.errors} / ${i.drops}</dd>` : ''}
          ${w ? `<dt>Signal</dt><dd>${w.quality}% · ${w.level} dBm</dd>` : ''}
        </dl></div>`;
    }).join('');
    if (S.s) this.sample(S.s);
  },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: storage
// ═══════════════════════════════════════════════════════════════════════════

P.storage = {
  every: 10000,
  mount(el) {
    this.el = el;
    el.innerHTML = `
      <div class="grid g3" id="st-m"><div class="card"><div class="skel"></div><div class="skel"></div></div></div>
      <div class="grid g2 mt">
        <div class="card"><div class="card-h"><h3>${ic('activity')} Disk I/O</h3><div class="r legend" id="st-l"></div></div><div id="st-c"></div></div>
        <div class="card flush"><div class="card-h"><h3>${ic('disk')} Devices</h3></div><div class="tbl-wrap"><table class="tbl"><thead><tr><th>Device</th><th>Model</th><th class="r">Size</th><th>Type</th><th class="r">Read</th><th class="r">Write</th></tr></thead><tbody id="st-d"></tbody></table></div></div>
      </div>`;
    this.ch = new LineChart($('#st-c', el), { max: 0, min: 1024, fmt: fmtRate, fmtAxis: v => fmtB(v), series: [{ c: '--info', label: 'Read' }, { c: '--acc-3', label: 'Write' }] });
    this.ch.set(H.t, [H.dr, H.dw]);
  },
  async poll() { const d = await api('/api/storage'); if (pageId !== 'storage') return; this.d = d; this.render(); },
  sample(s) {
    this.ch.set(H.t, [H.dr, H.dw]);
    $('#st-l').innerHTML = `<span><i style="background:${COL['--info']}"></i>R <b>${fmtRate(s.dr)}</b></span><span><i style="background:${COL['--acc-3']}"></i>W <b>${fmtRate(s.dw)}</b></span>`;
    (s.disk || []).forEach(dk => { const r = document.querySelector(`[data-dev="${CSS.escape(dk.n)}"]`); if (r) { r.children[4].textContent = fmtRate(dk.rx); r.children[5].textContent = fmtRate(dk.tx); } });
  },
  render() {
    const d = this.d;
    if (!d) return;
    $('#st-m').innerHTML = (d.mounts || []).map(m => {
      const p = m.total ? m.used / m.total * 100 : 0;
      const l = lvl(p, 80, 90);
      return `<div class="card ${p >= 90 ? 'hl' : ''}">
        <div class="row"><b class="mono grow trunc" style="font-size:14px">${esc(m.path)}</b>${m.ro ? '<span class="badge info">read-only</span>' : ''}<span class="badge mute">${esc(m.fs)}</span></div>
        <div class="row" style="margin:14px 0 8px;align-items:baseline"><span class="big ${l ? l + '-t' : ''}">${m.err ? '—' : p.toFixed(0) + '%'}</span><span class="mute grow" style="text-align:right">${m.err ? '<span class="crit-t">not responding</span>' : `${fmtB(m.used)} of ${fmtB(m.total)}`}</span></div>
        ${bar(p, l, 'fat')}
        <dl class="kvs" style="margin-top:12px;font-size:12.5px"><dt>Free</dt><dd>${fmtB(m.free)}</dd><dt>Device</dt><dd class="mono">${esc(m.dev)}</dd><dt>Inodes</dt><dd class="${m.inode > 85 ? 'warn-t' : ''}">${m.inode}% used</dd></dl>
      </div>`;
    }).join('') || '<div class="card empty">No filesystems</div>';
    $('#st-d').innerHTML = (d.devices || []).map(x => `<tr data-dev="${esc(x.name)}"><td class="mono"><b>${esc(x.name)}</b><div class="sub">${(x.parts || []).map(esc).join(' ')}</div></td><td>${esc(x.model || '—')}</td><td class="r num">${fmtB(x.size)}</td>
      <td>${x.name.startsWith('mmcblk') ? '<span class="badge vio">SD card</span>' : x.name.startsWith('nvme') ? '<span class="badge acc">NVMe</span>' : x.rota ? '<span class="badge mute">HDD</span>' : '<span class="badge info">SSD / flash</span>'}${x.rem ? ' <span class="badge warn">removable</span>' : ''}</td>
      <td class="r num mute">—</td><td class="r num mute">—</td></tr>`).join('');
  },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: logs (journal)
// ═══════════════════════════════════════════════════════════════════════════

P.logs = {
  mount(el, q) {
    this.el = el;
    el.innerHTML = `
      <div class="toolbar">
        <input class="in mono" id="lg-u" list="lg-units" placeholder="All units" value="${esc(q.get('unit') || '')}" style="width:240px"><datalist id="lg-units"></datalist>
        <select class="in" id="lg-p"><option value="">All priorities</option><option value="3">Errors &amp; worse</option><option value="4">Warnings &amp; worse</option><option value="6">Info &amp; worse</option></select>
        <select class="in" id="lg-s"><option value="">Latest</option><option value="-1h">Last hour</option><option value="today">Today</option><option value="boot">This boot</option></select>
        <div class="search">${ic('search')}<input class="in" id="lg-g" placeholder="Search (regex), Enter…"></div>
        <div class="r">
          <label class="check"><span class="switch"><input type="checkbox" id="lg-f" checked><i></i></span>Follow</label>
          <span class="mute num" id="lg-n"></span>
          <button class="ib" id="lg-c" title="Clear">${ic('trash')}</button>
          <button class="ib" id="lg-d" title="Download">${ic('download')}</button>
        </div>
      </div>
      <div class="logview" id="lg-v"></div>`;
    const reload = () => this.open();
    $('#lg-u', el).onchange = reload;
    $('#lg-p', el).onchange = reload;
    $('#lg-s', el).onchange = reload;
    $('#lg-g', el).onkeydown = e => { if (e.key === 'Enter') reload(); };
    $('#lg-f', el).onchange = e => { this.follow = e.target.checked; if (this.follow) this.v.scrollTop = this.v.scrollHeight; };
    $('#lg-c', el).onclick = () => { this.v.innerHTML = ''; this.lines = []; this.count(); };
    $('#lg-d', el).onclick = () => download('journal.txt', this.lines.map(l => `${new Date(l[0]).toISOString()} ${l[2]}: ${l[3]}`).join('\n'));
    this.v = $('#lg-v', el);
    this.v.addEventListener('scroll', () => {
      const atEnd = this.v.scrollHeight - this.v.scrollTop - this.v.clientHeight < 40;
      if (!atEnd && this.follow) { this.follow = false; $('#lg-f').checked = false; }
    });
    api('/api/services').then(s => { const dl = $('#lg-units'); if (dl) dl.innerHTML = s.map(x => `<option value="${esc(x.unit)}">`).join(''); }).catch(() => { });
    this.open();
  },
  unmount() { this.src && this.src.close(); },
  count() { $('#lg-n').textContent = plural(this.lines.length, 'line'); },
  open() {
    this.src && this.src.close();
    this.lines = [];
    this.follow = true;
    $('#lg-f').checked = true;
    this.v.innerHTML = '';
    const p = new URLSearchParams({ n: '500' });
    const u = $('#lg-u').value.trim();
    if (u) p.set('unit', /\.(service|timer|socket|mount|path|target)$/.test(u) ? u : u + '.service');
    if ($('#lg-p').value) p.set('prio', $('#lg-p').value);
    const s = $('#lg-s').value;
    if (s === 'boot') p.set('boot', '1'); else if (s) { p.set('since', s); p.set('n', '5000'); }
    const g = $('#lg-g').value.trim();
    if (g) p.set('grep', g);
    const hl = g ? new RegExp(`(>[^<]*?)(${reEsc(esc(g))})`, 'gi') : null;
    this.src = new EventSource('/api/logs?' + p);
    this.src.addEventListener('l', e => {
      const batch = JSON.parse(e.data);
      this.lines.push(...batch);
      if (this.lines.length > 8000) this.lines.splice(0, this.lines.length - 8000);
      let html = batch.map(journalLine).join('');
      if (hl) html = html.replace(hl, '$1<mark>$2</mark>');
      this.v.insertAdjacentHTML('beforeend', html);
      while (this.v.childElementCount > 5000) this.v.firstElementChild.remove();
      this.count();
      if (this.follow) this.v.scrollTop = this.v.scrollHeight;
    });
    this.src.addEventListener('end', () => { this.src.close(); this.v.insertAdjacentHTML('beforeend', '<div class="ll mute">— end —</div>'); });
    this.src.onerror = () => { if (this.src.readyState === 2) this.v.insertAdjacentHTML('beforeend', '<div class="ll p3">— stream closed —</div>'); };
  },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: terminal — sessions survive page switches
// ═══════════════════════════════════════════════════════════════════════════

const T = { sessions: [], active: null, root: null, loaded: null, seq: 0, pending: null };
function loadXterm() {
  if (T.loaded) return T.loaded;
  T.loaded = new Promise((res, rej) => {
    const css = document.createElement('link'); css.rel = 'stylesheet'; css.href = '/vendor/xterm.css'; document.head.appendChild(css);
    const s1 = document.createElement('script'); s1.src = '/vendor/xterm.js';
    s1.onload = () => { const s2 = document.createElement('script'); s2.src = '/vendor/addon-fit.js'; s2.onload = res; s2.onerror = rej; document.head.appendChild(s2); };
    s1.onerror = rej;
    document.head.appendChild(s1);
  });
  return T.loaded;
}
function termTheme() {
  return document.documentElement.dataset.theme === 'light'
    ? { background: '#ffffff', foreground: '#0e1525', cursor: '#4f46e5', selectionBackground: '#c7d2fe' }
    : { background: '#05070c', foreground: '#e7edf7', cursor: '#5eead4', cursorAccent: '#05070c', selectionBackground: 'rgba(129,140,248,.35)',
        black: '#1b2130', red: '#fb7185', green: '#34d399', yellow: '#fbbf24', blue: '#60a5fa', magenta: '#c084fc', cyan: '#5eead4', white: '#e7edf7',
        brightBlack: '#4f5a6e', brightRed: '#fda4af', brightGreen: '#6ee7b7', brightYellow: '#fcd34d', brightBlue: '#93c5fd', brightMagenta: '#d8b4fe', brightCyan: '#99f6e4', brightWhite: '#ffffff' };
}

async function openTerminal(opt = {}) {
  if (pageId !== 'terminal') { T.pending = opt; navigate('terminal'); return; }
  try { await loadXterm(); } catch { toast('Could not load the terminal', 'crit'); return; }
  const id = ++T.seq;
  const host = document.createElement('div');
  host.style.height = '100%';
  const term = new window.Terminal({ fontFamily: getComputedStyle(document.documentElement).getPropertyValue('--mono'), fontSize: 13.5, lineHeight: 1.2, cursorBlink: true, theme: termTheme(), scrollback: 5000 });
  const fit = new window.FitAddon.FitAddon();
  term.loadAddon(fit);
  const sess = { id, name: opt.name || `shell ${id}`, ctr: opt.ctr, term, fit, host, ws: null, alive: true };
  T.sessions.push(sess);
  T.root.appendChild(host);
  term.open(host);
  activate(sess);
  try { fit.fit(); } catch { }
  const qs = new URLSearchParams({ cols: term.cols, rows: term.rows });
  if (opt.ctr) qs.set('ctr', opt.ctr);
  const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/term?${qs}`);
  ws.binaryType = 'arraybuffer';
  sess.ws = ws;
  const enc = new TextEncoder();
  ws.onmessage = e => term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data));
  ws.onclose = () => { sess.alive = false; term.write('\r\n\x1b[90m— session ended —\x1b[0m\r\n'); renderTermTabs(); };
  ws.onopen = () => { try { fit.fit(); } catch { } ws.send(JSON.stringify({ r: [term.cols, term.rows] })); };
  term.onData(d => { if (ws.readyState === 1) ws.send(enc.encode(d)); });
  term.onResize(({ cols, rows }) => { if (ws.readyState === 1) ws.send(JSON.stringify({ r: [cols, rows] })); });
  renderTermTabs();
}
function activate(s) {
  if (!s) return;
  T.active = s;
  T.sessions.forEach(x => { x.host.style.display = x === s ? '' : 'none'; });
  renderTermTabs();
  requestAnimationFrame(() => { try { s.fit.fit(); } catch { } s.term.focus(); });
}
function closeSession(s) {
  if (!s) return;
  s.ws && s.ws.close();
  s.term.dispose();
  s.host.remove();
  T.sessions = T.sessions.filter(x => x !== s);
  if (T.active === s) T.active = T.sessions[T.sessions.length - 1] || null;
  if (T.active) activate(T.active); else renderTermTabs();
}
function renderTermTabs() {
  const el = $('#tm-tabs');
  if (!el) return;
  el.innerHTML = T.sessions.map(s => `<div class="term-tab ${s === T.active ? 'on' : ''}" data-s="${s.id}"><span class="dot ${s.alive ? 'ok' : ''}"></span>${ic(s.ctr ? 'box' : 'term')}${esc(s.name)}<button class="ib" data-close="${s.id}" title="Close">${ic('x')}</button></div>`).join('') +
    `<button class="btn sm" id="tm-new">${ic('plus')}New shell</button><span class="mute hide-sm" style="margin-left:auto;font-size:12px">Sessions stay open while you browse other pages.</span>`;
  const empty = $('#tm-empty');
  if (empty) empty.classList.toggle('hide', T.sessions.length > 0);
}
P.terminal = {
  mount(el) {
    el.innerHTML = `<div class="term-tabs" id="tm-tabs"></div><div class="term-wrap" id="tm-w"><div class="empty hide" id="tm-empty" style="position:absolute;inset:0;display:grid;place-content:center">${ic('term')}<b>No open sessions</b></div></div>`;
    if (!T.root) { T.root = document.createElement('div'); T.root.style.height = '100%'; }
    $('#tm-w', el).appendChild(T.root);
    el.onclick = e => {
      if (e.target.closest('#tm-new')) return openTerminal();
      const c = e.target.closest('[data-close]');
      if (c) { e.stopPropagation(); return closeSession(T.sessions.find(s => s.id === +c.dataset.close)); }
      const t = e.target.closest('[data-s]');
      if (t) activate(T.sessions.find(s => s.id === +t.dataset.s));
    };
    this.onResize = debounce(() => { try { T.active && T.active.fit.fit(); } catch { } }, 100);
    addEventListener('resize', this.onResize);
    renderTermTabs();
    if (T.pending) { const o = T.pending; T.pending = null; openTerminal(o); }
    else if (!T.sessions.length) openTerminal();
    else activate(T.active || T.sessions[0]);
  },
  unmount() { removeEventListener('resize', this.onResize); T.root && T.root.remove(); },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: security
// ═══════════════════════════════════════════════════════════════════════════

P.security = {
  mount(el) {
    this.el = el;
    el.innerHTML = `<div class="summary" id="sc-sum">${'<div class="card"><div class="skel"></div><div class="skel"></div></div>'.repeat(4)}</div><div id="sc-b"></div>`;
    el.onclick = e => { if (e.target.closest('#sc-upd')) this.updates(); const g = e.target.closest('[data-go]'); if (g) navigate(g.dataset.go); };
    this.load().catch(e => { $('#sc-b').innerHTML = `<div class="card empty">${ic('alert')}<b>${esc(e.message)}</b></div>`; });
  },
  async load() {
    const [s, p] = await Promise.all([api('/api/security'), api('/api/ports').catch(() => null)]);
    if (pageId !== 'security') return;
    if (p) S.cache.ports = p;
    const pub = p ? p.listen.filter(l => l.scope === 'public') : [];
    const sshd = s.sshd || {};
    const fwOn = /\bactive\b/.test(s.firewall) || /nftables \(\d/.test(s.firewall);
    const posture = [];
    if (sshd.PermitRootLogin === 'yes') posture.push(['crit', 'Root can log in over SSH', 'Set PermitRootLogin to prohibit-password or no']);
    if (sshd.PasswordAuthentication !== 'no') posture.push(['warn', 'SSH accepts passwords', 'Switch to keys and set PasswordAuthentication no']);
    if (!fwOn) posture.push(['warn', 'No active firewall detected', 'e.g. sudo ufw allow ssh && sudo ufw enable']);
    if (pub.length > 8) posture.push(['warn', `${pub.length} ports are exposed on every interface`, 'Bind internal-only services to 127.0.0.1']);
    if (s.failTotal > 50) posture.push(['warn', `${s.failTotal} failed SSH logins in 24h`, 'fail2ban or key-only auth will shut this down']);
    $('#sc-sum').innerHTML = [
      ['Failed SSH logins (24h)', s.failTotal, `from ${s.failIPs} address${s.failIPs === 1 ? '' : 'es'}`, s.failTotal > 20 ? 'warn-t' : 'ok-t'],
      ['Active sessions', (s.sessions || []).length, 'logged in now', ''],
      ['Exposed ports', pub.length, 'on every interface', pub.length > 8 ? 'warn-t' : ''],
      ['Firewall', `<span style="font-size:17px">${esc(s.firewall)}</span>`, '', fwOn ? 'ok-t' : 'warn-t'],
    ].map(([l, v, sub, c]) => `<div class="card"><div class="label">${l}</div><div class="big ${c}">${v}</div><div class="label">${sub}</div></div>`).join('');
    $('#sc-b').innerHTML = `
      <div class="grid g2">
        <div class="card"><div class="card-h"><h3>${ic('shield')} Posture</h3></div>
          ${posture.map(([l, t, h]) => `<div class="alert-li ${l}">${ic(l === 'crit' ? 'alert' : 'info')}<div class="grow"><b>${esc(t)}</b><div class="mute" style="font-size:12px">${esc(h)}</div></div></div>`).join('') || `<div class="alert-li">${ic('okc', 'ok-t')}<b>No obvious problems found</b></div>`}
          <dl class="kvs" style="margin-top:12px"><dt>SSH port</dt><dd class="mono">${esc(sshd.Port || '22 (default)')}</dd><dt>Root login</dt><dd>${esc(sshd.PermitRootLogin || 'default')}</dd><dt>Password auth</dt><dd>${esc(sshd.PasswordAuthentication || 'default (yes)')}</dd></dl>
        </div>
        <div class="card"><div class="card-h"><h3>${ic('pkg')} Updates</h3><div class="r"><button class="btn sm" id="sc-upd">${ic('restart')}Check</button></div></div><div id="sc-u" class="mute">Reads the local apt cache for upgradable packages.</div></div>
        <div class="card flush"><div class="card-h"><h3>${ic('alert')} Top offending IPs</h3></div><div class="tbl-wrap"><table class="tbl"><tbody>${(s.topIPs || []).map(x => `<tr><td class="mono">${esc(x.ip)}</td><td class="r"><span class="badge crit">${x.n} attempts</span></td></tr>`).join('') || '<tr><td class="mute" style="padding:16px">No failed logins</td></tr>'}</tbody></table></div></div>
        <div class="card flush"><div class="card-h"><h3>${ic('user')} Sessions &amp; logins</h3></div><div class="tbl-wrap"><table class="tbl"><tbody>
          ${(s.sessions || []).map(x => `<tr><td><span class="dot ok"></span> <b>${esc(x.user)}</b></td><td class="mono mute">${esc(x.tty)}</td><td class="mono">${esc(x.from || 'local')}</td><td class="mute">${esc(x.since)}</td></tr>`).join('')}
          ${(s.accepts || []).slice().reverse().map(x => `<tr><td>${ic('check', 'ok-t')} ${esc(x.user)}</td><td class="mute">${esc(x.how)}</td><td class="mono">${esc(x.ip)}</td><td class="mute">${ago(x.ts)}</td></tr>`).join('')}
          ${!(s.sessions || []).length && !(s.accepts || []).length ? '<tr><td class="mute" style="padding:16px">No SSH logins in the last 24h</td></tr>' : ''}
        </tbody></table></div></div>
      </div>
      <div class="card flush mt"><div class="card-h"><h3>${ic('globe')} Exposed ports</h3><div class="r"><button class="btn sm ghost" data-go="ports">All ports ${ic('ext')}</button></div></div><div class="tbl-wrap"><table class="tbl"><tbody>
        ${pub.map(l => `<tr><td class="mono"><b>${l.port}</b>/${l.proto}</td><td>${esc(l.svc || '')}</td><td>${esc(l.proc)}</td><td>${l.ctr ? `<span class="badge acc">${esc(l.ctr)}</span>` : ''}</td></tr>`).join('') || '<tr><td class="mute" style="padding:16px">Nothing listens on every interface</td></tr>'}
      </tbody></table></div></div>
      <div class="card flush mt"><div class="card-h"><h3>${ic('list')} Recent failed attempts</h3></div><div class="tbl-wrap" style="max-height:360px"><table class="tbl"><tbody>
        ${(s.fails || []).slice().reverse().map(x => `<tr><td class="mute" style="white-space:nowrap">${ago(x.ts)}</td><td class="mono">${esc(x.user)}</td><td class="mono">${esc(x.ip)}</td></tr>`).join('') || '<tr><td class="mute" style="padding:16px">None</td></tr>'}
      </tbody></table></div></div>`;
  },
  async updates() {
    const el = $('#sc-u');
    el.innerHTML = '<span class="spin"></span>';
    let u;
    try { u = await act(() => api('/api/updates')); } catch { el.textContent = 'Check failed.'; return; }
    if (!u.supported) { el.textContent = 'apt is not available on this system.'; return; }
    const sec = u.packages.filter(p => p.security).length;
    el.innerHTML = `<div class="row wrap" style="margin-bottom:10px"><span class="big" style="color:var(--text)">${u.packages.length}</span><span>upgradable</span>${sec ? `<span class="badge crit">${sec} security</span>` : ''}${u.rebootRequired ? '<span class="badge warn">reboot required</span>' : ''}</div>
      <div style="font-size:12px;margin-bottom:8px">Package lists refreshed ${fmtDur(u.listsAge)} ago. Run <span class="mono">sudo apt update &amp;&amp; sudo apt upgrade</span> in the terminal to apply.</div>
      <div style="max-height:220px;overflow:auto">${u.packages.map(p => `<div class="li" style="padding:5px 0"><span class="mono grow" style="color:var(--text)">${esc(p.name)}</span>${p.security ? '<span class="badge crit">security</span>' : ''}<span class="mono" style="font-size:11.5px">${esc(p.version || '')}</span></div>`).join('')}</div>`;
  },
};

// ═══════════════════════════════════════════════════════════════════════════
//  page: settings
// ═══════════════════════════════════════════════════════════════════════════

P.settings = {
  async mount(el) {
    el.innerHTML = `<div class="grid g2"><div class="card"><div class="skel"></div><div class="skel"></div></div></div>`;
    const st = await api('/api/settings').catch(e => { el.innerHTML = `<div class="card empty">${esc(e.message)}</div>`; });
    if (!st || pageId !== 'settings') return;
    const a = st.alerts;
    const units = { cpu: '%', mem: '%', temp: '°C', disk: '%' };
    const th = (k, l, min, max) => `<div class="field"><label>${l}</label><div class="row"><input type="range" min="${min}" max="${max}" value="${a[k]}" data-th="${k}" class="grow"><span class="chip" style="width:66px;justify-content:center" data-thv="${k}">${a[k]}${units[k]}</span></div></div>`;
    el.innerHTML = `
      <div class="grid g2">
        <div class="col" style="gap:16px">
          <div class="card"><div class="card-h"><h3>${ic('bell')} Alert thresholds</h3></div><div class="col" style="gap:16px">
            ${th('cpu', 'CPU sustained above', 30, 100)}${th('mem', 'Memory above', 30, 100)}${th('temp', 'Temperature above', 40, 95)}${th('disk', 'Any disk fuller than', 50, 100)}
            <div><button class="btn pri" id="st-save">${ic('check')}Save thresholds</button></div></div></div>
          <div class="card"><div class="card-h"><h3>${ic('info')} About</h3></div><dl class="kvs">
            <dt>iseall</dt><dd>v${esc(st.version)}</dd><dt>Host</dt><dd>${esc(S.info ? S.info.hostname : '')}</dd>
            <dt>Shortcuts</dt><dd><kbd>Ctrl K</kbd> command palette · <kbd>/</kbd> filter · <kbd>Esc</kbd> close<br><kbd>g</kbd> then ${PAGES.map(p => `<kbd title="${p.t}">${p.k}</kbd>`).join(' ')} to jump between pages</dd></dl></div>
        </div>
        <div class="col" style="gap:16px">
          <div class="card"><div class="card-h"><h3>${ic('sun')} Appearance</h3></div><div class="col" style="gap:14px">
            <div class="row" style="justify-content:space-between"><span>Theme</span><div class="btn-group" id="st-theme"><button data-v="dark">Dark</button><button data-v="light">Light</button><button data-v="auto">System</button></div></div>
            <label class="check" style="justify-content:space-between"><span>Lite mode<div class="mute" style="font-size:12px">No animations or glows — best on the Pi's own screen</div></span><span class="switch"><input type="checkbox" id="st-lite" ${store.get('lite', false) ? 'checked' : ''}><i></i></span></label>
          </div></div>
          <div class="card"><div class="card-h"><h3>${ic('lock')} Password</h3></div><div class="col">
            <input class="in" type="password" id="pw-o" placeholder="Current password" autocomplete="current-password">
            <input class="in" type="password" id="pw-n" placeholder="New password (8+ characters)" autocomplete="new-password">
            <div class="row"><button class="btn" id="pw-s">Change password</button><button class="btn ghost" id="st-out" style="margin-left:auto">${ic('logout')}Sign out</button></div></div></div>
          <div class="card"><div class="card-h"><h3>${ic('power')} Power</h3></div><div class="row">
            <button class="btn" id="pw-rb">${ic('restart')}Reboot</button><button class="btn danger" id="pw-sd">${ic('power')}Shut down</button></div></div>
        </div>
      </div>`;
    $$('[data-th]', el).forEach(r => { r.oninput = () => { $(`[data-thv="${r.dataset.th}"]`).textContent = r.value + units[r.dataset.th]; }; });
    $('#st-save').onclick = () => { const v = {}; $$('[data-th]').forEach(r => { v[r.dataset.th] = +r.value; }); act(() => post('/api/settings', v), 'Thresholds saved').catch(() => { }); };
    const cur = store.get('theme', 'dark');
    $$('#st-theme button').forEach(b => { b.classList.toggle('on', b.dataset.v === cur); b.onclick = () => { setTheme(b.dataset.v); $$('#st-theme button').forEach(x => x.classList.toggle('on', x === b)); }; });
    $('#st-lite').onchange = e => { store.set('lite', e.target.checked); applyLite(); };
    $('#pw-s').onclick = () => act(() => post('/api/auth/password', { old: $('#pw-o').value, new: $('#pw-n').value }), 'Password changed — other sessions were signed out').then(() => { $('#pw-o').value = ''; $('#pw-n').value = ''; }, () => { });
    $('#st-out').onclick = logout;
    $('#pw-rb').onclick = () => power('reboot');
    $('#pw-sd').onclick = () => power('shutdown');
  },
};

async function power(action) {
  const host = S.info ? S.info.hostname : 'confirm';
  const ok = await confirmBox(action === 'reboot' ? 'Reboot now?' : 'Shut down now?',
    action === 'reboot' ? 'Every service and container restarts. The dashboard reconnects when the machine is back.' : 'The machine powers off. You\'ll need physical access to turn it back on.',
    { ok: action === 'reboot' ? 'Reboot' : 'Shut down', danger: true, typed: host });
  if (ok) act(() => post('/api/power', { action }), action === 'reboot' ? 'Rebooting… see you shortly' : 'Shutting down').catch(() => { });
}
async function logout() { await post('/api/auth/logout').catch(() => { }); showLogin(); }

// ═══════════════════════════════════════════════════════════════════════════
//  alerts popover
// ═══════════════════════════════════════════════════════════════════════════

let bellOpen = false;
function renderBell() {
  const p = $('#bell-pop');
  if (!p) return;
  const al = (S.slow && S.slow.alerts) || [];
  p.innerHTML = `<div class="card-h" style="margin:0 4px 8px"><h3>Alerts</h3></div>` + (al.map(a => `<div class="alert-li ${a.level}" data-nav="${esc(a.nav || 'overview')}">${ic(a.level === 'crit' ? 'alert' : 'info')}<span class="grow">${esc(a.msg)}</span></div>`).join('') || `<div class="empty" style="padding:24px">${ic('okc')}<b>All quiet</b>No alerts right now.</div>`);
}
function closeBell() { const p = $('#bell-pop'); if (p) p.remove(); bellOpen = false; }
function toggleBell() {
  if ($('#bell-pop')) return closeBell();
  const p = document.createElement('div');
  p.className = 'card pop';
  p.id = 'bell-pop';
  $('.bell').appendChild(p);
  bellOpen = true;
  renderBell();
  p.onclick = e => { e.stopPropagation(); const a = e.target.closest('[data-nav]'); if (a) { closeBell(); navigate(a.dataset.nav); } };
}
document.addEventListener('click', e => {
  if (bellOpen && !e.target.closest('.bell')) closeBell();
  if (!e.target.closest('#side') && !e.target.closest('#burger')) { const s = $('#side'); s && s.classList.remove('open'); }
});

// ═══════════════════════════════════════════════════════════════════════════
//  command palette
// ═══════════════════════════════════════════════════════════════════════════

function fuzzy(q, s) {
  if (!q) return 1;
  s = s.toLowerCase();
  const i = s.indexOf(q);
  if (i >= 0) return 1000 - i * 2 - s.length * .1;
  let j = 0, score = 0, last = -2;
  for (let k = 0; k < s.length && j < q.length; k++) if (s[k] === q[j]) { score += last === k - 1 ? 3 : 1; last = k; j++; }
  return j === q.length ? score : 0;
}

function openPalette() {
  if ($('.palette')) return;
  const o = openOverlay('modal palette', `<div class="pin">${ic('search')}<input placeholder="Jump to a page, container, service, port — or run an action…" autocomplete="off" spellcheck="false"></div><div class="res"></div>
    <div class="foot"><span><kbd>↑</kbd> <kbd>↓</kbd> move</span><span><kbd>Enter</kbd> open</span><span><kbd>Esc</kbd> close</span></div>`);
  const inp = $('input', o.el), res = $('.res', o.el);
  const items = [];
  PAGES.forEach(p => items.push({ g: 'Pages', t: p.t, i: p.i, k: `g ${p.k}`, fn: () => navigate(p.id) }));
  items.push(
    { g: 'Actions', t: 'New startup app', i: 'rocket', fn: () => { navigate('startup'); setTimeout(() => appForm(null, () => pageId === 'startup' && page.load().catch(() => { })), 120); } },
    { g: 'Actions', t: 'New terminal session', i: 'term', fn: () => openTerminal() },
    { g: 'Actions', t: 'Show failed services', i: 'alert', fn: () => navigate('services', 'f=failed') },
    { g: 'Actions', t: 'Show exposed ports', i: 'globe', fn: () => navigate('ports') },
    { g: 'Actions', t: 'Toggle light / dark theme', i: 'sun', fn: toggleTheme },
    { g: 'Actions', t: 'Toggle lite mode', i: 'zap', fn: () => { store.set('lite', !store.get('lite', false)); applyLite(); } },
    { g: 'Actions', t: 'Reboot…', i: 'restart', fn: () => power('reboot') },
    { g: 'Actions', t: 'Shut down…', i: 'power', fn: () => power('shutdown') },
    { g: 'Actions', t: 'Sign out', i: 'logout', fn: logout },
  );
  let sel = 0, shown = [];
  const draw = () => {
    const q = inp.value.trim().toLowerCase();
    shown = items.map(it => ({ it, s: fuzzy(q, `${it.t} ${it.sub || ''} ${it.g}`) })).filter(x => x.s > 0)
      .sort((a, b) => q ? b.s - a.s : 0).slice(0, 60).map(x => x.it);
    sel = Math.min(sel, Math.max(0, shown.length - 1));
    let g = '';
    res.innerHTML = shown.map((it, k) => {
      const hdr = it.g !== g ? `<div class="grp">${esc(g = it.g)}</div>` : '';
      return hdr + `<div class="it ${k === sel ? 'on' : ''}" data-k="${k}"><span class="ic">${ic(it.i)}</span><div class="grow" style="min-width:0"><div class="trunc">${esc(it.t)}</div>${it.sub ? `<div class="mute trunc" style="font-size:12px">${esc(it.sub)}</div>` : ''}</div>${it.k ? `<span class="k">${esc(it.k)}</span>` : ''}</div>`;
    }).join('') || `<div class="empty" style="padding:28px">${ic('search')}<b>No results</b></div>`;
    const on = $('.it.on', res);
    if (on) on.scrollIntoView({ block: 'nearest' });
  };
  const run = k => { const it = shown[k]; if (!it) return; o.close(); it.fn(); };
  inp.oninput = () => { sel = 0; draw(); };
  inp.onkeydown = e => {
    if (e.key === 'ArrowDown') { sel = Math.min(sel + 1, shown.length - 1); draw(); e.preventDefault(); }
    else if (e.key === 'ArrowUp') { sel = Math.max(sel - 1, 0); draw(); e.preventDefault(); }
    else if (e.key === 'Enter') { run(sel); e.preventDefault(); }
  };
  res.onclick = e => { const it = e.target.closest('[data-k]'); if (it) run(+it.dataset.k); };
  res.onmousemove = e => { const it = e.target.closest('[data-k]'); if (it && +it.dataset.k !== sel) { sel = +it.dataset.k; $$('.it', res).forEach(x => x.classList.toggle('on', +x.dataset.k === sel)); } };
  draw();
  inp.focus();
  // enrich with live entities in the background
  const add = list => { items.push(...list); if (!o.closed) draw(); };
  if (S.info && S.info.docker) api('/api/docker').then(d => d.available && add(d.containers.flatMap(c => [
    { g: 'Containers', t: c.name, sub: `${c.state} · ${c.image}`, i: 'box', fn: () => ctrLogs(c) },
    { g: 'Container actions', t: `Restart ${c.name}`, i: 'restart', fn: () => act(() => post(`/api/docker/c/${c.id}/restart`), `${c.name} restarted`).catch(() => { }) },
    { g: 'Container actions', t: `Shell into ${c.name}`, i: 'term', fn: () => openTerminal({ ctr: c.id, name: c.name }) },
  ]))).catch(() => { });
  api('/api/services').then(sv => add(sv.filter(s => s.load !== 'not-loaded').map(s => ({ g: 'Services', t: s.unit.replace(/\.service$/, ''), sub: `${s.sub} · ${s.desc}`, i: 'layers', fn: () => svcDrawer(s.unit) })))).catch(() => { });
  (S.cache.ports ? Promise.resolve(S.cache.ports) : api('/api/ports')).then(p => {
    S.cache.ports = p;
    add(p.listen.filter(l => l.web && l.scope !== 'local').map(l => ({ g: 'Web UIs', t: l.web.title || `${l.proc} :${l.port}`, sub: `${l.web.scheme}://${location.hostname}:${l.port}`, i: 'globe', fn: () => window.open(`${l.web.scheme}://${location.hostname}:${l.port}/`, '_blank', 'noopener') })));
  }).catch(() => { });
  api('/api/procs').then(d => add(d.procs.filter(p => !p.k).sort((a, b) => b.cpu - a.cpu).slice(0, 40).map(p => ({ g: 'Processes', t: `${p.name} · ${p.pid}`, sub: p.cmd, i: 'cpu', fn: () => procDrawer(p) })))).catch(() => { });
}

// ═══════════════════════════════════════════════════════════════════════════
//  theme, keyboard, login, boot
// ═══════════════════════════════════════════════════════════════════════════

function setTheme(v) { store.set('theme', v); applyTheme(); }
function applyTheme() {
  const v = store.get('theme', 'dark');
  const eff = v === 'auto' ? (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark') : v;
  document.documentElement.dataset.theme = eff;
  readColors();
  charts.forEach(c => c.draw());
  T.sessions.forEach(s => { s.term.options.theme = termTheme(); });
  updateThemeIcon();
  if (page && page.render) page.render();
}
function toggleTheme() { setTheme(document.documentElement.dataset.theme === 'light' ? 'dark' : 'light'); }
function updateThemeIcon() { const b = $('#theme'); if (b) b.innerHTML = ic(document.documentElement.dataset.theme === 'light' ? 'moon' : 'sun'); }
function applyLite() { const on = !!store.get('lite', false); document.documentElement.classList.toggle('lite', on); document.body.classList.toggle('lite', on); }

let gPending = 0;
document.addEventListener('keydown', e => {
  const app = $('#root .app');
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { if (app) { e.preventDefault(); openPalette(); } return; }
  if (e.key === 'Escape') {
    if (menuEl) return closeMenu();
    if (overlays.length) { overlays[overlays.length - 1].close(); return; }
    if (bellOpen) closeBell();
    return;
  }
  const typing = /INPUT|TEXTAREA|SELECT/.test(e.target.tagName) || e.target.isContentEditable || e.target.closest('.xterm');
  if (typing || e.ctrlKey || e.metaKey || e.altKey || !app || overlays.length) return;
  if (e.key === '/') { const s = $('#pg .search input'); if (s) { e.preventDefault(); s.focus(); s.select(); } return; }
  if (e.key === 'g') { gPending = Date.now(); return; }
  if (gPending && Date.now() - gPending < 1200) {
    gPending = 0;
    const p = PAGES.find(x => x.k === e.key);
    if (p) { e.preventDefault(); navigate(p.id); }
  }
});

function teardown() {
  stopPoll();
  if (es) { es.close(); es = null; }
  if (page && page.unmount) page.unmount();
  page = null; pageId = null;
  while (overlays.length) overlays[overlays.length - 1].close();
  charts.forEach(c => c.destroy());
  T.sessions.slice().forEach(closeSession);
}

function showLogin() {
  teardown();
  api('/api/auth').then(a => {
    if (a.authed) return start();
    const setup = a.setup;
    $('#root').innerHTML = `<div class="login"><div class="aurora"></div>
      <form class="card login-card" id="lg">${eyeSVG()}
        <h1>${setup ? 'Welcome to iseall' : 'iseall'}</h1>
        <p>${setup ? 'Create a password to protect this dashboard.' : 'Sign in to see everything.'}</p>
        <input class="in" type="password" id="pw1" placeholder="${setup ? 'New password (8+ characters)' : 'Password'}" autocomplete="${setup ? 'new-password' : 'current-password'}">
        ${setup ? '<input class="in" type="password" id="pw2" placeholder="Repeat password" autocomplete="new-password">' : ''}
        <button class="btn pri" type="submit">${setup ? 'Create &amp; continue' : 'Sign in'}</button>
        <div class="err" id="lg-e"></div>
      </form></div>`;
    hideBoot();
    $('#pw1').focus();
    $('#lg').onsubmit = async e => {
      e.preventDefault();
      const pw = $('#pw1').value;
      const fail = msg => { $('#lg-e').textContent = msg; const f = $('#lg'); f.classList.remove('shake'); void f.offsetWidth; f.classList.add('shake'); };
      if (setup && pw !== $('#pw2').value) return fail('Passwords don\'t match');
      const btn = $('#lg button');
      btn.disabled = true;
      try {
        await post(setup ? '/api/auth/setup' : '/api/auth/login', { password: pw });
        start();
      } catch (err) { fail(err.message); btn.disabled = false; $('#pw1').select(); }
    };
  }).catch(() => {
    $('#root').innerHTML = `<div class="login"><div class="card login-card">${eyeSVG()}<h1>Can't reach iseall</h1><p>Retrying…</p></div></div>`;
    hideBoot();
    setTimeout(showLogin, 3000);
  });
}

function hideBoot() { const b = $('#boot'); if (b) { b.classList.add('gone'); setTimeout(() => b.remove(), 400); } }

let started = false;
function start() {
  renderShell();
  connect();
  route();
  hideBoot();
  if (started) return;
  started = true;
  addEventListener('hashchange', route);
  document.addEventListener('visibilitychange', () => { if (!document.hidden && page && page.poll) page.poll().catch(() => { }); });
  matchMedia('(prefers-color-scheme: light)').addEventListener('change', () => { if (store.get('theme', 'dark') === 'auto') applyTheme(); });
}

applyTheme();
applyLite();
showLogin();

})();
