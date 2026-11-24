'use strict';
/* ---------- tiny helpers ---------- */
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
class Raw { constructor(s) { this.s = s; } }
const raw = s => new Raw(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const val = v => v instanceof Raw ? v.s : Array.isArray(v) ? v.map(val).join('') : (v === false || v == null) ? '' : esc(v);
const h = (strs, ...vals) => { let o = strs[0]; vals.forEach((v, i) => { o += val(v) + strs[i + 1]; }); return new Raw(o); };
const setHTML = (el, r) => { el.innerHTML = r.s; return el; };

const ICON = {
  back: '<path d="M19 12H5M11 6l-6 6 6 6"/>',
  home: '<path d="M3 11.5 12 4l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z"/>',
  library: '<path d="M4 4h5v16H4zM11 4h5v16h-5zM18 6l3 .8-3.6 13.4-3-.8z"/>',
  chart: '<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1"/>',
  plus: '<path d="M12 5v14M5 12h14"/>', search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  play: '<path d="M7 4.5v15l13-7.5z" fill="currentColor"/>', stop: '<rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor"/>',
  grid: '<rect x="4" y="4" width="7" height="7"/><rect x="13" y="4" width="7" height="7"/><rect x="4" y="13" width="7" height="7"/><rect x="13" y="13" width="7" height="7"/>',
  list: '<path d="M4 6h16M4 12h16M4 18h16"/>', star: '<path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9z" fill="currentColor"/>',
  folder: '<path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  file: '<path d="M6 3h8l5 5v13H6z"/><path d="M14 3v5h5"/>', edit: '<path d="M4 20h4L19 9l-4-4L4 16zM14 6l4 4"/>',
  check: '<path d="M5 12l5 5 9-10"/>', down: '<path d="M12 5v14M5 12l7 7 7-7"/>', trash: '<path d="M5 7h14M10 7V4h4v3M7 7l1 13h8l1-13"/>', x: '<path d="M6 6l12 12M18 6 6 18"/>', up: '<path d="M12 19V5M5 12l7-7 7 7"/>',
  scan: '<path d="M4 8V5a1 1 0 0 1 1-1h3M16 4h3a1 1 0 0 1 1 1v3M20 16v3a1 1 0 0 1-1 1h-3M8 20H5a1 1 0 0 1-1-1v-3M4 12h16"/>',
};
const icon = n => raw(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICON[n]}</svg>`);

const STATUS = { planned: 'Planned', playing: 'Playing', completed: 'Completed', onhold: 'On hold', dropped: 'Dropped' };

function fmtDur(sec) {
  sec = Math.round(sec || 0);
  if (sec < 60) return sec ? sec + 's' : '—';
  const m = Math.round(sec / 60), hh = Math.floor(m / 60), mm = m % 60;
  return hh ? `${hh}h ${String(mm).padStart(2, '0')}m` : `${mm}m`;
}
/* live play timer: only runs while the game is the window on top (the server tracks that and sends
   the focused seconds); elements with class "rt" and data-id tick once a second */
const clock = sec => {
  sec = Math.max(0, Math.floor(sec));
  const hr = Math.floor(sec / 3600), m = Math.floor(sec % 3600 / 60), s = sec % 60;
  return (hr ? hr + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(s).padStart(2, '0');
};
const stampRun = r => { const out = {}, at = Date.now(); for (const id in r || {}) out[id] = { ...r[id], at }; return out; };
const runSecs = r => r.active + (r.paused ? 0 : Math.min(1.5, (Date.now() - r.at) / 1000));
const runTime = id => h`<span class="rt" data-id="${id}">${S.running[id] ? clock(runSecs(S.running[id])) : ''}</span>`;
function tickRun() {
  for (const el of $$('.rt')) { const r = S.running[el.dataset.id]; if (r) el.textContent = clock(runSecs(r)); }
  for (const el of $$('[data-live]')) {
    const r = S.running[el.dataset.id]; if (!r) continue;
    el.classList.toggle('paused', !!r.paused);
    const l = $('.rt-lbl', el); if (l) l.textContent = r.paused ? el.dataset.pause : el.dataset.live;
  }
  const chip = $('#runchip'); if (!chip) return;
  const ids = Object.keys(S.running), key = ids.join();
  if (chip.dataset.k === key) return;
  chip.dataset.k = key;
  const g = ids.length && S.lib.games.find(x => x.id === ids[0]);
  if (!g) { setHTML(chip, h``); return; }
  setHTML(chip, h`<button class="runchip" data-live="Playing" data-id="${g.id}" data-act="open" title="Only counts while ${g.title} is the window on top"><span class="dot pulse" style="background:var(--good)"></span><span class="rc-t">${g.title}</span>${ids.length > 1 ? h`<span class="rc-n">+${ids.length - 1}</span>` : ''}<span class="rc-p">Paused</span>${runTime(g.id)}</button>`);
  tickRun();
}
setInterval(tickRun, 1000);
const isZero = d => !d || new Date(d).getFullYear() < 2000;
const st0 = () => S.lib.settings;
const fmtDate = d => isZero(d) ? 'Never' : new Date(d).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
function rel(d) {
  if (isZero(d)) return 'Never';
  const days = Math.floor((Date.now() - new Date(d)) / 864e5);
  if (days <= 0) return 'Today'; if (days === 1) return 'Yesterday';
  if (days < 30) return days + ' days ago';
  return fmtDate(d);
}
const hue = t => { let x = 0; for (const c of t) x = (x * 31 + c.codePointAt(0)) % 360; return x; };
const total = g => g.sessions.reduce((a, s) => a + s.seconds, 0);
const lastOf = g => { let m = isZero(g.lastPlayed) ? 0 : +new Date(g.lastPlayed); g.sessions.forEach(x => { const t = +new Date(x.start); if (t > m) m = t; }); return m ? new Date(m).toISOString() : null; };

async function api(method, url, body, rawBody) {
  const opt = { method, headers: { 'X-ShioriBox': '1' } };
  if (rawBody) opt.body = rawBody;
  else if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  const r = await fetch(url, opt);
  const ct = r.headers.get('content-type') || '';
  const data = ct.includes('json') ? await r.json() : null;
  if (!r.ok) throw new Error((data && data.error) || `Request failed (${r.status})`);
  return data;
}
function toast(msg, kind = '') {
  const t = document.createElement('div'); t.className = 'toast ' + kind; t.textContent = msg;
  $('#toasts').append(t); setTimeout(() => t.remove(), kind === 'err' ? 6500 : 3200);
}
const guard = fn => async (...a) => { try { return await fn(...a); } catch (e) { toast(e.message, 'err'); } };

/* ---------- state ---------- */
const S = {
  lib: { games: [], categories: [], settings: {} }, running: {}, info: {},
  view: 'home', q: '', status: 'all', tag: '', cat: '', fav: false, sel: null,
  sort: localStorage.getItem('ns.sort') || 'lastPlayed', layout: localStorage.getItem('ns.layout') || 'grid',
};
const game = id => S.lib.games.find(g => g.id === id);

async function load() {
  const d = await api('GET', '/api/library');
  S.lib = d; S.running = stampRun(d.running); S.info = d.info || {};
  document.documentElement.dataset.theme = d.settings.theme || 'dark';
  applyBg(d.settings);
  render();
}
const reload = guard(load);

function applyBg(s) {
  const b = document.body, st = b.style;
  if (!s.bgImage) { b.classList.remove('hasbg'); return; }
  b.classList.add('hasbg');
  st.setProperty('--bgurl', `url("/background/${encodeURIComponent(s.bgImage)}")`);
  st.setProperty('--bgblur', (s.bgBlur || 0) + 'px');
  st.setProperty('--bgov', (s.bgOverlay || 0) / 100);
  st.setProperty('--panelpct', (s.bgPanel || 80) + '%');
  st.setProperty('--bgpos', s.bgPos === 'top' ? 'center top' : s.bgPos === 'bottom' ? 'center bottom' : 'center');
}

/* ---------- covers ---------- */
function cover(g) {
  const nb = g.nsfw ? ' nsfw' : '';
  return g.cover ? h`<img class="${nb}" src="/covers/${g.cover}" alt="" loading="lazy">`
    : h`<div class="ph${nb}" style="--h:${hue(g.title)}">${[...g.title.trim()][0] || '?'}</div>`;
}

/* ---------- navigation rail and top bar ---------- */
function renderSide() {
  const navb = (v, ic, label, on, act = 'nav') => h`<button class="rbtn ${on ? 'on' : ''}" data-act="${act}" data-v="${v}" title="${label}" aria-label="${label}">${icon(ic)}<span>${label}</span></button>`;
  const lib = S.view === 'library';
  setHTML($('#side'), h`
    <div class="brand" title="ShioriBox"><img src="/logo-96.png" alt="" width="40" height="40"></div>
    ${navb('home', 'home', 'Home', S.view === 'home')}
    ${navb('library', 'library', 'Library', lib && !S.fav)}
    ${h`<button class="rbtn ${lib && S.fav ? 'on' : ''}" data-act="nav-fav" title="Favorites" aria-label="Favorites">${icon('star')}<span>Favorites</span></button>`}
    ${navb('discover', 'search', 'Discover', S.view === 'discover')}
    ${navb('anime', 'play', 'Anime', S.view === 'anime')}
    ${navb('stats', 'chart', 'Stats', S.view === 'stats')}
    <div class="rail-gap"></div>
    ${navb('settings', 'gear', 'Settings', S.view === 'settings')}
    <div class="side-foot">v${S.info.version || ''}${S.upd && S.upd.info && S.upd.info.available && !S.upd.info.skipped ? h`<button class="upd-chip" data-act="upd-open" title="Version ${S.upd.info.latest} is available">Update</button>` : ''}</div>`);
  renderTopbar();
}
function renderTopbar() {
  const tb = $('#topbar'); if (!tb) return;
  const title = { home: 'Home', discover: 'Discover', anime: 'Anime', stats: 'Statistics', settings: 'Settings' }[S.view]
    || (S.fav ? 'Favorites' : S.cat ? ((S.lib.categories.find(c => c.id === S.cat) || {}).name || 'Library') : 'Library');
  const showSearch = S.view === 'home' || S.view === 'library';
  const showActions = showSearch && S.lib.games.length > 0;
  const q = $('#q');
  if (q && showSearch && tb.dataset.v === S.view && tb.dataset.n === String(S.lib.games.length) && tb.dataset.sel === String(!!S.sel) && tb.dataset.t === title) return;
  tb.dataset.v = S.view; tb.dataset.n = String(S.lib.games.length); tb.dataset.sel = String(!!S.sel); tb.dataset.t = title;
  setHTML(tb, h`<h1 class="tb-title">${title}</h1>
    ${showSearch && S.lib.games.length ? h`<label class="search" aria-label="Search your library">${icon('search')}<input type="search" id="q" placeholder="Search your library…   ( / )" value="${S.q}"></label>` : ''}
    <span class="grow"></span><span id="runchip"></span>
    ${showActions && S.view === 'library' ? h`<button class="btn ${S.sel ? 'on' : ''}" data-act="selmode">${icon('check')}${S.sel ? 'Done selecting' : 'Select'}</button>` : ''}
    ${showSearch ? h`<button class="btn" data-act="scan">${icon('scan')}Scan folder</button><button class="btn primary" data-act="add">${icon('plus')}Add game</button>` : ''}`);
  tickRun();
}

/* ---------- library ---------- */
function filtered() {
  const q = S.q.trim().toLowerCase();
  let list = S.lib.games.filter(g => {
    if (S.status !== 'all' && g.status !== S.status) return false;
    if (S.tag && !g.tags.includes(S.tag)) return false;
    if (S.cat && !g.categories.includes(S.cat)) return false;
    if (S.fav && !g.favorite) return false;
    if (q && !(g.title + ' ' + g.originalTitle + ' ' + g.developer + ' ' + g.tags.join(' ')).toLowerCase().includes(q)) return false;
    return true;
  });
  const t = x => isZero(x) ? 0 : +new Date(x);
  const by = {
    title: (a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }),
    added: (a, b) => t(b.addedAt) - t(a.addedAt),
    lastPlayed: (a, b) => t(lastOf(b)) - t(lastOf(a)),
    playtime: (a, b) => total(b) - total(a),
    rating: (a, b) => b.rating - a.rating,
    release: (a, b) => (b.releaseDate || '').localeCompare(a.releaseDate || ''),
  };
  return list.sort(by[S.sort] || by.title);
}

function playBtn(g, cls = 'play') {
  const run = S.running[g.id];
  return h`<button class="${cls} ${run ? 'run' : ''}" data-act="${run ? 'stop' : 'launch'}" data-id="${g.id}" title="${run ? 'Stop' : 'Play'} ${g.title}" aria-label="${run ? 'Stop' : 'Play'} ${g.title}">${icon(run ? 'stop' : 'play')}${/\bbtn\b/.test(cls) ? (run ? 'Stop' : 'Play') : ''}</button>`;
}

function renderLibrary() {
  const all = S.lib.games;
  if (!all.length) {
    return h`<div class="empty"><img class="hero-art" src="/logo.png" alt="">
      <div class="big">Welcome to ShioriBox</div>
      <p class="muted">Point it at the folder where you keep your visual novels and it will find them, or add one by hand.</p>
      <div class="row"><button class="btn primary" data-act="scan">${icon('scan')}Scan a folder</button><button class="btn" data-act="add">${icon('plus')}Add manually</button></div></div>`;
  }
  const list = filtered();
  const tags = [...new Set(all.flatMap(g => g.tags))].sort((a, b) => a.localeCompare(b));
  const counts = { all: all.length }; Object.keys(STATUS).forEach(k => counts[k] = all.filter(g => g.status === k).length);
  const cnt = c => all.filter(g => g.categories.includes(c.id)).length;
  return h`
    <div class="shelves">
      <button class="chip ${!S.cat && !S.fav ? 'on' : ''}" data-act="nav" data-v="library">All<span class="n">${all.length}</span></button>
      <button class="chip ${S.fav ? 'on' : ''}" data-act="nav-fav">${icon('star')}Favorites<span class="n">${all.filter(g => g.favorite).length}</span></button>
      ${S.lib.categories.map(c => h`<span class="shelf-chip ${S.cat === c.id ? 'on' : ''}"><button class="chip ${S.cat === c.id ? 'on' : ''}" data-act="nav-cat" data-id="${c.id}">${c.name}<span class="n">${cnt(c)}</span></button><span class="tools"><button class="icon-btn" data-act="cat-rename" data-id="${c.id}" title="Rename" aria-label="Rename ${c.name}">${icon('edit')}</button><button class="icon-btn" data-act="cat-del" data-id="${c.id}" title="Delete" aria-label="Delete ${c.name}">${icon('trash')}</button></span></span>`)}
      <button class="chip ghost" data-act="cat-new" title="New collection">${icon('plus')}Collection</button>
    </div>
    <div class="toolbar">
      <select id="tagsel" style="width:auto" aria-label="Filter by tag"><option value="">All tags</option>${tags.map(t => h`<option ${S.tag === t ? 'selected' : ''}>${t}</option>`)}</select>
      <select id="sortsel" style="width:auto" aria-label="Sort">${[['lastPlayed', 'Recently played'], ['title', 'Title'], ['added', 'Recently added'], ['playtime', 'Playtime'], ['rating', 'Rating'], ['release', 'Release date']].map(([k, l]) => h`<option value="${k}" ${S.sort === k ? 'selected' : ''}>${l}</option>`)}</select>
      <div class="seg"><button class="${S.layout === 'grid' ? 'on' : ''}" data-act="layout" data-v="grid" aria-label="Grid">${icon('grid')}</button><button class="${S.layout === 'list' ? 'on' : ''}" data-act="layout" data-v="list" aria-label="List">${icon('list')}</button></div>
    </div>
    <div class="chips">${['all', ...Object.keys(STATUS)].map(k => h`<button class="chip ${S.status === k ? 'on' : ''}" data-act="status" data-v="${k}">${k === 'all' ? 'All' : STATUS[k]}<span class="n">${counts[k]}</span></button>`)}</div>
    <div id="bulk">${bulkHTML()}</div>
    <div id="results" class="${S.sel ? 'selmode' : ''}">${resultsHTML(list)}</div>`;
}

function bulkHTML() {
  if (!S.sel) return '';
  const n = S.sel.size;
  return h`<div class="bulkbar"><b>${n} selected</b>
    <button class="btn sm" data-act="sel-all">Select all shown</button><button class="btn sm" data-act="sel-none" ${n ? '' : 'disabled'}>Clear</button>
    <span class="grow"></span>
    <select id="bulk-status" class="sm" ${n ? '' : 'disabled'} aria-label="Set status"><option value="">Set status…</option>${Object.keys(STATUS).map(k => h`<option value="${k}">${STATUS[k]}</option>`)}</select>
    ${S.lib.categories.length ? h`<select id="bulk-cat" class="sm" ${n ? '' : 'disabled'} aria-label="Collections"><option value="">Collections…</option><optgroup label="Add to">${S.lib.categories.map(c => h`<option value="add:${c.id}">${c.name}</option>`)}</optgroup><optgroup label="Remove from">${S.lib.categories.map(c => h`<option value="rm:${c.id}">${c.name}</option>`)}</optgroup></select>` : ''}
    <button class="btn sm" data-act="bulk-nsfw-on" ${n ? '' : 'disabled'}>Blur covers</button><button class="btn sm" data-act="bulk-nsfw-off" ${n ? '' : 'disabled'}>Unblur</button>
    <button class="btn sm" data-act="bulk-ja-on" ${n ? '' : 'disabled'}>Japanese locale on</button><button class="btn sm" data-act="bulk-ja-off" ${n ? '' : 'disabled'}>off</button>
    <button class="btn sm danger" data-act="bulk-del" ${n ? '' : 'disabled'}>${icon('trash')}Remove</button></div>`;
}

function resultsHTML(list) {
  const sel = g => S.sel ? `data-act="toggle" data-id="${g.id}"` : `data-act="open" data-id="${g.id}"`;
  const mark = g => S.sel ? h`<span class="pick ${S.sel.has(g.id) ? 'on' : ''}">${S.sel.has(g.id) ? '✓' : ''}</span>` : '';
  if (!list.length) return h`<p class="muted" style="margin-top:40px;text-align:center">Nothing matches those filters.</p>`;
  if (S.layout === 'list') return h`<div class="list">${list.map(g => h`
    <div class="li ${S.sel && S.sel.has(g.id) ? 'picked' : ''}" ${raw(sel(g))}>${mark(g)}<div class="cv">${cover(g)}</div>
      <div style="min-width:0"><div class="ti">${g.title}</div><div class="tiny muted">${g.developer || '—'}</div>${g.progress ? h`<div class="tiny prog" title="Progress">◔ ${g.progress}</div>` : ''}</div>
      <div class="x"><span class="dot ${g.status}"></span> ${STATUS[g.status]}</div><div class="x muted">${fmtDur(total(g))}</div><div class="x muted">${g.rating ? '★ ' + g.rating : ''}</div>${playBtn(g)}</div>`)}</div>`;
  return h`<div class="grid">${list.map(g => cardHTML(g, sel, mark))}</div>`;
}

function cardHTML(g, sel = g2 => `data-act="open" data-id="${g2.id}"`, mark = () => '') {
  return h`
    <div class="card ${S.sel && S.sel.has(g.id) ? 'picked' : ''}" tabindex="0" ${raw(sel(g))}>
      <div class="cv">${cover(g)}${mark(g)}
        ${g.favorite ? h`<span class="fav">${icon('star')}</span>` : ''}${g.locale === 'ja' ? h`<span class="jp" title="Launches with Japanese locale">JP</span>` : ''}
        ${S.running[g.id] ? h`<span class="badge"><span class="dot pulse" style="background:var(--good)"></span>Running ${runTime(g.id)}</span>` : h`<span class="badge"><span class="dot ${g.status}"></span>${STATUS[g.status]}</span>`}
        ${playBtn(g)}</div>
      <div class="ti">${g.title}</div><div class="su">${total(g) ? fmtDur(total(g)) : 'Not played'}${g.rating ? ' · ★ ' + g.rating : ''}</div>${g.progress ? h`<div class="su prog" title="Progress">◔ ${g.progress}</div>` : ''}</div>`;
}

/* ---------- home ---------- */
function weekSeconds() {
  const since = Date.now() - 7 * 864e5;
  return S.lib.games.reduce((a, g) => a + g.sessions.filter(s => +new Date(s.start) >= since).reduce((x, s) => x + s.seconds, 0), 0);
}
function renderHome() {
  const all = S.lib.games;
  if (!all.length) return renderLibrary();
  const t = x => isZero(x) ? 0 : +new Date(x);
  const running = all.find(g => S.running[g.id]);
  const played = all.filter(g => lastOf(g)).sort((a, b) => t(lastOf(b)) - t(lastOf(a)));
  const hero = running || played[0] || [...all].sort((a, b) => t(b.addedAt) - t(a.addedAt))[0];
  const row = (title, list, see) => list.length ? h`<section class="shelf"><div class="shelf-h"><h2>${title}</h2>${see ? h`<button class="btn sm ghost" data-act="home-all" data-k="${see}">See all</button>` : ''}</div><div class="hrow">${list.map(g => cardHTML(g))}</div></section>` : '';
  const others = g => g.id !== hero.id;
  const bg = hero.cover ? `url("/covers/${hero.cover}")` : 'none';
  const done = all.filter(g => g.status === 'completed').length;
  return h`
    <section class="homehero"><div class="hh-bg ${hero.nsfw ? 'nsfw' : ''}" style="background-image:${raw(esc(bg))}"></div><div class="hh-in">
      <div class="cv">${cover(hero)}</div>
      <div class="hh-t"><div class="kicker">${running ? 'Playing now' : played.length ? 'Continue playing' : 'Latest addition'}</div>
        <h1>${hero.title}</h1>
        <div class="meta-line">${hero.developer ? h`<span>${hero.developer}</span>` : ''}<span><span class="dot ${hero.status}"></span> ${STATUS[hero.status]}</span>${S.running[hero.id] ? h`<span class="hh-live" data-live="Playing for" data-pause="Paused at" data-id="${hero.id}"><span class="dot pulse" style="background:var(--good)"></span><span class="rt-lbl">Playing for</span> ${runTime(hero.id)}</span>` : lastOf(hero) ? h`<span>Last played ${rel(lastOf(hero))}</span>` : ''}${total(hero) ? h`<span>${fmtDur(total(hero))} played</span>` : ''}</div>
        ${hero.progress ? h`<div class="prog hh-prog">◔ ${hero.progress}</div>` : ''}
        ${hero.description ? h`<p class="hh-desc">${hero.description}</p>` : ''}
        <div class="row" style="flex-wrap:wrap">${playBtn(hero, 'btn primary big')}<button class="btn big" data-act="open" data-id="${hero.id}">Details</button><button class="btn big ghost" data-act="open-guides" data-id="${hero.id}">${icon('file')}Guides${hero.guides && hero.guides.length ? ` (${hero.guides.length})` : ''}</button></div>
      </div></div></section>
    <div class="tiles home-tiles"><div class="tile"><b>${all.length}</b><span>Games</span></div><div class="tile"><b>${fmtDur(all.reduce((a, g) => a + total(g), 0))}</b><span>Total playtime</span></div><div class="tile"><b>${fmtDur(weekSeconds())}</b><span>Last 7 days</span></div><div class="tile"><b>${done}</b><span>Completed</span></div></div>
    ${row('Continue playing', played.filter(others).slice(0, 12), 'played')}
    ${row('Recently added', [...all].sort((a, b) => t(b.addedAt) - t(a.addedAt)).filter(others).slice(0, 12), 'added')}
    ${row('Favorites', all.filter(g => g.favorite && others(g)).slice(0, 12), 'fav')}
    ${row('Up next', all.filter(g => g.status === 'planned' && others(g)).slice(0, 12), 'planned')}`;
}


/* ---------- discover (VNDB search) ---------- */
const DC_PLAT = [['', 'Any platform'], ['win', 'Windows'], ['lin', 'Linux'], ['mac', 'macOS'], ['web', 'Web'], ['and', 'Android'], ['ios', 'iOS'], ['ps4', 'PlayStation 4'], ['ps3', 'PlayStation 3'], ['ps2', 'PlayStation 2'], ['psv', 'PS Vita'], ['psp', 'PSP'], ['swi', 'Nintendo Switch'], ['nds', 'Nintendo DS'], ['xb3', 'Xbox 360'], ['dos', 'DOS'], ['p98', 'PC-98'], ['drc', 'Dreamcast'], ['sat', 'Saturn']];
const DC_LANG = [['', 'Any language'], ['en', 'English'], ['ja', 'Japanese'], ['zh-Hans', 'Chinese (simplified)'], ['zh-Hant', 'Chinese (traditional)'], ['ko', 'Korean'], ['ru', 'Russian'], ['es', 'Spanish'], ['de', 'German'], ['fr', 'French'], ['it', 'Italian'], ['pt-br', 'Portuguese (Brazil)'], ['id', 'Indonesian'], ['vi', 'Vietnamese'], ['tr', 'Turkish'], ['uk', 'Ukrainian'], ['pl', 'Polish'], ['th', 'Thai'], ['ar', 'Arabic']];
const DC_SORT = [['searchrank', 'Best match'], ['rating', 'Rating'], ['votecount', 'Most voted'], ['released', 'Release date'], ['title', 'Title']];
S.dc = { q: '', tags: [], platform: '', lang: '', olang: '', sort: 'searchrank', reverse: false, results: null, more: false, page: 1, busy: false, err: '', sug: [], mode: 'search', again: false, producer: null, hist: [], last: null, blur: (() => { try { return localStorage.getItem('ns.dcblur') || 'auto'; } catch { return 'auto'; } })(), rec: null, recBusy: false, recErr: '' };
const platName = c => (DC_PLAT.find(p => p[0] === c) || [0, c])[1];
const langName = c => (DC_LANG.find(p => p[0] === c) || [0, c])[1];
const sel = (id, opts, val, label) => h`<label class="f"><span>${label}</span><select id="${id}">${opts.map(([v, t]) => h`<option value="${v}" ${v === val ? 'selected' : ''}>${t}</option>`)}</select></label>`;

function renderDiscover() {
  const d = S.dc;
  const tabs = h`<div class="seg dc-tabs"><button class="${d.mode === 'search' ? 'on' : ''}" data-act="dc-mode" data-m="search">Search</button><button class="${d.mode === 'rec' ? 'on' : ''}" data-act="dc-mode" data-m="rec">For you</button></div>`;
  const head = h`<div class="dc-head"><div class="row" style="gap:12px">${d.hist.length ? h`<button class="btn" data-act="dc-back" title="Back to the previous results">${icon('back')}Back</button>` : ''}<div class="muted tiny">Search the visual novel database at vndb.org</div></div><div class="row" style="gap:10px"><label class="row tiny muted" style="gap:6px">Covers<select id="dc-blur" class="sm"><option value="auto" ${d.blur === 'auto' ? 'selected' : ''}>Auto (blur 18+ only)</option><option value="all" ${d.blur === 'all' ? 'selected' : ''}>Blur all</option><option value="none" ${d.blur === 'none' ? 'selected' : ''}>Unblur all</option></select></label>${tabs}</div></div>`;
  if (d.mode === 'rec') return h`${head}<div id="dc-res" class="dcb-${d.blur}"></div>`;
  return h`${head}<div class="dc-layout"><aside class="panel dc-form">
      <label class="f"><span>Keyword or name (or a VNDB id like v17)</span><input type="search" id="dc-q" value="${d.q}" placeholder="e.g. Steins;Gate, 白色相簿, nakige…"></label>
      <div class="f"><span>Tags</span><div class="dc-tagbox"><span id="dc-chips"></span><input type="text" id="dc-tag" placeholder="type a tag, e.g. “school”" autocomplete="off"></div><div id="dc-sug" class="dc-sug"></div></div>
      <div class="f"><span>Developer / publisher</span>${d.producer ? h`<div class="dc-tagbox"><span class="tagchip">${d.producer.name}<button class="icon-btn" data-act="dc-prod-clear" aria-label="Remove developer filter">${icon('x')}</button></span></div>` : h`<div class="dc-tagbox"><input type="text" id="dc-prod" placeholder="type a company, e.g. “Key”" autocomplete="off"></div><div id="dc-psug" class="dc-sug"></div>`}</div>
      <div class="dc-four">${sel('dc-plat', DC_PLAT, d.platform, 'Platform')}${sel('dc-lang', DC_LANG, d.lang, 'Available in language')}${sel('dc-olang', DC_LANG, d.olang, 'Originally written in')}${sel('dc-sort', DC_SORT, d.sort, 'Sort by')}</div>
      <label class="chk"><input type="checkbox" id="dc-rev" ${d.reverse ? 'checked' : ''}>Reverse order</label>
      <button class="btn primary big" data-act="dc-go" style="width:100%;justify-content:center;margin-top:14px">${icon('search')}Search</button>
    </aside><div id="dc-res" class="dcb-${d.blur}"></div></div>`;
}
function dcChips() {
  const c = $('#dc-chips'); if (!c) return;
  setHTML(c, h`${S.dc.tags.map(t => h`<span class="tagchip">${t.name}<button class="icon-btn" data-act="dc-untag" data-id="${t.id}" aria-label="Remove tag ${t.name}">${icon('x')}</button></span>`)}`);
}
function dcSug() {
  const s = $('#dc-sug'); if (!s) return;
  setHTML(s, S.dc.sug.length ? h`${S.dc.sug.map(t => h`<button class="sug" data-act="dc-addtag" data-id="${t.id}" data-name="${t.name}"><span>${t.name}</span><span class="tiny muted">${t.category === 'ero' ? 'sexual · ' : t.category === 'tech' ? 'technical · ' : ''}${t.count} VNs</span></button>`)}` : h``);
}
function dcCard(r) {
  const inLib = S.lib.games.some(g => g.vndbId === r.id);
  const nsfw = r.sexual >= 1;
  const rating = r.rating ? (r.rating / 10).toFixed(1) : '';
  return h`<div class="dc-card" data-act="dc-detail" data-id="${r.id}" tabindex="0" role="button" aria-label="Details of ${r.title}"><div class="cv">${r.image ? h`<img class="${nsfw ? 'nsfw' : ''}" src="${r.image}" alt="" loading="lazy" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(r.title)}">${[...r.title.trim()][0] || '?'}</div>`}${nsfw ? h`<span class="jp" title="VNDB flags this cover as sexual">18+</span>` : ''}</div>
    <div class="dc-info"><div class="ti">${r.title}</div>
      <div class="tiny muted">${[r.released ? r.released.slice(0, 4) : '', r.developers.slice(0, 2).join(', '), r.lang ? langName(r.lang) : ''].filter(Boolean).join(' · ') || '—'}</div>
      <div class="tiny muted">${rating ? '★ ' + rating + (r.votes ? ` (${r.votes})` : '') : ''}${r.platforms.length ? (rating ? ' · ' : '') + r.platforms.slice(0, 4).map(platName).join(', ') : ''}</div>
      <div class="dc-tags">${r.tags.slice(0, 5).map(t => h`<span class="tag">${t}</span>`)}</div>
      <div class="row" style="margin-top:8px;gap:6px">${inLib ? h`<span class="tag" style="padding:5px 9px">In library</span>` : h`<button class="btn sm primary" data-act="dc-add" data-id="${r.id}">${icon('plus')}Add to library</button>`}<button class="btn sm ghost" data-act="dc-open" data-id="${r.id}">View on VNDB</button></div></div></div>`;
}
function dcResults() {
  const box = $('#dc-res'); if (!box) return;
  const d = S.dc;
  if (d.busy && !d.results) return setHTML(box, h`<p class="muted" style="text-align:center;margin-top:30px">Searching VNDB…</p>`);
  if (d.err) return setHTML(box, h`<p style="margin-top:20px"><b>Search failed.</b> <span class="muted">${d.err}</span></p>`);
  if (!d.results) return setHTML(box, h`<p class="muted" style="text-align:center;margin-top:30px">Choose some filters and press Search. With no keyword it lists popular visual novels that match them.</p>`);
  if (!d.results.length) return setHTML(box, h`${dcProducerBar()}<p class="muted" style="text-align:center;margin-top:30px">Nothing found. Try fewer tags or a different spelling.</p>`);
  setHTML(box, h`${dcProducerBar()}<div class="dc-grid">${d.results.map(dcCard)}</div>${d.more ? h`<div style="text-align:center;margin:18px 0"><button class="btn" data-act="dc-more" ${d.busy ? 'disabled' : ''}>${d.busy ? 'Loading…' : 'Load more'}</button></div>` : ''}`);
}
function dcProducerBar() {
  const p = S.dc.producer; if (!p) return h``;
  return h`<div class="panel dc-prod"><span>Games developed or published by <b>${p.name}</b></span><button class="btn sm" data-act="dc-prod-clear">Show everything</button></div>`;
}
function dcFill() { if (S.dc.mode === 'rec') { dcRecView(); if (!S.dc.rec && !S.dc.recBusy && !S.dc.recErr) dcRecLoad(); } else { dcChips(); dcSug(); dcResults(); } }
async function dcRecLoad() {
  const d = S.dc; if (d.recBusy) return;
  d.recBusy = true; d.recErr = ''; dcRecView();
  try { d.rec = await api('GET', '/api/vndb/recommend'); } catch (e) { d.rec = null; d.recErr = e.message; }
  d.recBusy = false; dcRecView();
}
function dcRecView() {
  const box = $('#dc-res'); if (!box || S.dc.mode !== 'rec') return;
  const d = S.dc;
  if (d.recBusy) return setHTML(box, h`<p class="muted" style="text-align:center;margin-top:30px">Finding games you might like…</p>`);
  if (d.recErr) return setHTML(box, h`<p style="margin-top:20px">${d.recErr}</p><button class="btn" data-act="dc-rec-refresh">Try again</button>`);
  if (!d.rec) return setHTML(box, h``);
  setHTML(box, h`<div class="panel" style="margin-bottom:6px"><div class="tiny muted">Based on games you liked: ${d.rec.games.join(', ')}</div><div class="tiny muted" style="margin-top:4px">Shared tags: ${d.rec.tags.join(', ')}</div><div style="margin-top:8px"><button class="btn sm" data-act="dc-rec-refresh">Refresh</button></div></div>
    ${d.rec.results.length ? h`<div class="dc-grid">${d.rec.results.map(dcCard)}</div>` : h`<p class="muted" style="text-align:center;margin-top:30px">No new matches found — everything popular with those tags is already in your library.</p>`}`);
}
const dcParams = () => { const d = S.dc; return JSON.stringify([d.q, d.tags.map(t => t.id), d.platform, d.lang, d.olang, d.sort, d.reverse, d.producer && d.producer.id]); };
const dcSnap = () => { const d = S.dc; return { p: dcParams(), q: d.q, tags: d.tags.slice(), platform: d.platform, lang: d.lang, olang: d.olang, sort: d.sort, reverse: d.reverse, producer: d.producer, results: d.results, more: d.more, page: d.page }; };
function dcBack() {
  const d = S.dc, p = d.hist.pop(); if (!p) return;
  Object.assign(d, { q: p.q, tags: p.tags, platform: p.platform, lang: p.lang, olang: p.olang, sort: p.sort, reverse: p.reverse, producer: p.producer, results: p.results, more: p.more, page: p.page, err: '', mode: 'search', sug: [] });
  d.last = p; render();
}
function dcBackBtn() { if (S.dc.hist.length === 1 && !$('[data-act=dc-back]')) render(); }
function dcRead() {
  const d = S.dc;
  d.q = $('#dc-q').value.trim(); d.platform = $('#dc-plat').value; d.lang = $('#dc-lang').value; d.olang = $('#dc-olang').value;
  d.sort = $('#dc-sort').value; d.reverse = $('#dc-rev').checked;
}
async function dcSearch(more) {
  const d = S.dc; if (d.busy) { if (!more) d.again = true; return; }
  if (!more) {
    dcRead(); d.page = 1;
    const cur = dcParams();
    if (d.last && d.last.p !== cur) { d.hist.push(d.last); if (d.hist.length > 30) d.hist.shift(); if (S.view === 'discover') dcBackBtn(); }
    d.cur = cur;
  } else d.page++;
  d.busy = true; d.err = ''; dcResults();
  try {
    const r = await api('POST', '/api/vndb/discover', { q: d.q, tags: d.tags.map(t => t.id), platform: d.platform, lang: d.lang, olang: d.olang, sort: d.sort, reverse: d.reverse, page: d.page, producer: d.producer ? d.producer.id : '' });
    d.results = more ? [...(d.results || []), ...r.results] : r.results; d.more = r.more;
  } catch (e) { d.err = e.message; if (more) d.page--; }
  d.busy = false;
  if (!more && d.results && !d.err) d.last = dcSnap();
  dcResults();
  if (d.again) { d.again = false; dcSearch(false); }
}
let dcTimer;
let dcPTimer, dcPSugList = [];
function dcPSug() {
  const s = $('#dc-psug'); if (!s) return;
  setHTML(s, dcPSugList.length ? h`${dcPSugList.map(t => h`<button class="sug" data-act="dc-pick-prod" data-id="${t.id}" data-name="${t.name}"><span>${t.name}${t.original && t.original !== t.name ? h` <span class="muted">${t.original}</span>` : ''}</span><span class="tiny muted">${{ co: 'company', in: 'individual', ng: 'amateur group' }[t.type] || ''}</span></button>`)}` : h``);
}
function dcProdInput(v) {
  clearTimeout(dcPTimer);
  if (v.trim().length < 2) { dcPSugList = []; return dcPSug(); }
  dcPTimer = setTimeout(async () => {
    try { dcPSugList = await api('GET', '/api/vndb/producers?q=' + encodeURIComponent(v.trim())); } catch { dcPSugList = []; }
    dcPSug();
  }, 300);
}
function dcTagInput(v) {
  clearTimeout(dcTimer);
  if (v.trim().length < 2) { S.dc.sug = []; return dcSug(); }
  dcTimer = setTimeout(async () => {
    try { S.dc.sug = (await api('GET', '/api/vndb/tags?q=' + encodeURIComponent(v.trim()))).filter(t => !S.dc.tags.some(x => x.id === t.id)); } catch { S.dc.sug = []; }
    dcSug();
  }, 300);
}
async function dcAdd(id) {
  const r = [...(S.dc.results || []), ...((S.dc.rec || {}).results || [])].find(x => x.id === id); if (!r) return;
  const g = await api('POST', '/api/games', { title: r.title, originalTitle: r.altTitle || '', developer: r.developers.join(', '), releaseDate: r.released || '', description: r.description || '', vndbId: r.id, tags: r.tags, status: 'planned', nsfw: r.sexual >= 1, locale: S.lib.settings.defaultLocale || '', exePath: '', launchArgs: '', savePath: '', notes: '', rating: 0, categories: [], favorite: false, useWrapper: false });
  if (r.image) await api('POST', `/api/games/${g.id}/cover-from-url`, { url: r.image }).catch(() => { });
  await load(); toast(`Added “${r.title}” — open it to choose the game’s .exe`, 'ok');
}


/* ---------- discover: full details window ---------- */
const bbHTML = s => raw(esc(s || '')
  .replace(/\[spoiler\]([\s\S]*?)\[\/spoiler\]/gi, '<details class="sp"><summary>Show spoiler</summary>$1</details>')
  .replace(/\[url=[^\]]*\]([\s\S]*?)\[\/url\]/gi, '$1')
  .replace(/\[b\]([\s\S]*?)\[\/b\]/gi, '<b>$1</b>').replace(/\[i\]([\s\S]*?)\[\/i\]/gi, '<i>$1</i>')
  .replace(/\[\/?(quote|raw|code|u|s|url)[^\]]*\]/gi, '').replace(/\n/g, '<br>'));
const bbPlain = s => (s || '').replace(/\[spoiler\][\s\S]*?\[\/spoiler\]/gi, '').replace(/\[url=[^\]]*\]([\s\S]*?)\[\/url\]/gi, '$1').replace(/\[[^\]]*\]/g, '').trim();
const DEV_STATUS = ['Finished', 'In development', 'Cancelled'];
const LENGTHS = ['', 'Very short (< 2 hours)', 'Short (2–10 hours)', 'Medium (10–30 hours)', 'Long (30–50 hours)', 'Very long (> 50 hours)'];
const ROLES = { scenario: 'Scenario', chardesign: 'Character design', art: 'Art', music: 'Music', songs: 'Songs', director: 'Director', translator: 'Translation', editor: 'Editing', qa: 'QA', staff: 'Other staff' };
const REL = { seq: 'Sequel', preq: 'Prequel', set: 'Same setting', alt: 'Alternative version', char: 'Shares characters', side: 'Side story', par: 'Parent story', ser: 'Same series', fan: 'Fandisc', orig: 'Original game' };
const fmtLen = m => m ? (m >= 60 ? `${Math.floor(m / 60)}h ${m % 60 ? (m % 60) + 'm' : ''}` : `${m}m`) : '';

async function dcDetail(id, trail = []) {
  const dlg = openDialog({ title: 'Loading…', body: h`<p class="muted" style="text-align:center;padding:30px">Fetching details from VNDB…</p>` });
  dlg.el.classList.add('wide');
  let d;
  try { d = await api('GET', '/api/vndb/vn/' + encodeURIComponent(id)); }
  catch (e) { dlg.title('Could not load'); return setHTML(dlg.body, h`<p>${e.message}</p>`); }
  dlg.title(d.title);
  dlg.vn = d; dlg.spoil = 0; dlg.shot = null; dlg.trail = trail;
  const draw = () => dcDetailDraw(dlg);
  dlg.draw = draw; draw();
  dlg.el.addEventListener('click', async e => {
    const b = e.target.closest('[data-vd]'); if (!b) return;
    const k = b.dataset.vd;
    if (k === 'add') { b.disabled = true; try { await dcAddDetail(dlg.vn); draw(); } catch (err) { toast(err.message, 'err'); b.disabled = false; } }
    else if (k === 'open') api('POST', '/api/open-url', { url: 'https://vndb.org/' + dlg.vn.id }).catch(err => toast(err.message, 'err'));
    else if (k === 'shot') { dlg.shot = +b.dataset.i; draw(); }
    else if (k === 'shot-x') { dlg.shot = null; draw(); }
    else if (k === 'tag') { S.dc.tags.push({ id: b.dataset.id, name: b.dataset.name }); S.dc.mode = 'search'; S.view = 'discover'; dlg.close(); render(); dcSearch(false); }
    else if (k === 'rel') { dlg.close(); dcDetail(b.dataset.id, [...dlg.trail, { id: dlg.vn.id, title: dlg.vn.title }]); }
    else if (k === 'back') { const t = dlg.trail.slice(), p = t.pop(); dlg.close(); dcDetail(p.id, t); }
    else if (k === 'char') charDetail(b.dataset.id, dlg.spoil, [...dlg.trail, { id: dlg.vn.id, title: dlg.vn.title }]);
    else if (k === 'prod') { S.dc.producer = { id: b.dataset.id, name: b.dataset.name }; S.dc.q = ''; S.dc.tags = []; S.dc.platform = S.dc.lang = S.dc.olang = ''; S.dc.mode = 'search'; S.view = 'discover'; dlg.close(); render(); dcSearch(false); }
  });
  dlg.el.addEventListener('change', e => { if (e.target.id === 'vd-spoil') { dlg.spoil = +e.target.value; draw(); } });
}
const SEX = { m: 'Male', f: 'Female', b: 'Both', n: 'Sexless' };
async function charDetail(id, spoil, trail) {
  const dlg = openDialog({ title: 'Loading…', body: h`<p class="muted" style="text-align:center;padding:30px">Fetching character from VNDB…</p>` });
  dlg.el.classList.add('wide');
  let d;
  try { d = await api('GET', '/api/vndb/char/' + encodeURIComponent(id)); }
  catch (e) { dlg.title('Could not load'); return setHTML(dlg.body, h`<p>${e.message}</p>`); }
  dlg.title(d.name); dlg.ch = d; dlg.spoil = spoil || 0;
  const fromVn = trail[trail.length - 1].id;
  dlg.draw = () => charDraw(dlg, fromVn); dlg.draw();
  dlg.el.addEventListener('click', e => {
    const b = e.target.closest('[data-vd]'); if (!b) return;
    if (b.dataset.vd === 'back') dlg.close();
    else if (b.dataset.vd === 'vn') { if (b.dataset.id === fromVn) return dlg.close(); document.querySelectorAll('dialog.wide').forEach(x => x.close()); dcDetail(b.dataset.id, trail); }
  });
  dlg.el.addEventListener('change', e => { if (e.target.id === 'cd-spoil') { dlg.spoil = +e.target.value; dlg.draw(); } });
}
function charDraw(dlg, fromVn) {
  const d = dlg.ch, nsfw = d.sexual >= 1;
  const body = [['Original name', d.original], ['Aliases', d.aliases.join(', ')], ['Sex', SEX[d.sex] || ''], ['Age', d.age || ''],
    ['Birthday', d.birthday], ['Blood type', d.bloodType ? d.bloodType.toUpperCase() : ''], ['Height', d.height ? d.height + ' cm' : ''], ['Weight', d.weight ? d.weight + ' kg' : ''],
    ['Bust / waist / hips', d.bust || d.waist || d.hips ? `${d.bust || '?'} / ${d.waist || '?'} / ${d.hips || '?'} cm` : ''], ['Cup', d.cup ? d.cup.toUpperCase() : '']].filter(r => r[1] !== '' && r[1] != null);
  const traits = d.traits.filter(t => t.spoiler <= dlg.spoil);
  const groups = [...new Set(traits.map(t => t.group))];
  const vns = d.vns.filter(v => v.spoiler <= dlg.spoil || v.id === fromVn);
  const role = r => ({ main: 'Protagonist', primary: 'Main character', side: 'Side character', appears: 'Minor appearance' }[r] || r);
  setHTML(dlg.body, h`<button class="btn sm" data-vd="back" style="margin-bottom:10px">${icon('back')}Back to the game</button><div class="vd-top">
      <div class="vd-cover">${d.image ? h`<img class="${nsfw ? 'nsfw' : ''}" src="${d.image}" alt="" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(d.name)}">${[...d.name.trim()][0] || '?'}</div>`}</div>
      <div class="vd-head"><h2 style="margin:0">${d.name}</h2>${body.length ? h`<table class="vd-info">${body.map(([k, v]) => h`<tr><th>${k}</th><td>${v}</td></tr>`)}</table>` : h`<p class="muted">VNDB has no personal details for this character.</p>`}</div></div>
    <h3 class="vd-h">Find CGs elsewhere</h3>${cgLinkRow(d.title, d.altTitle)}
    ${d.description ? h`<h3 class="vd-h">Description</h3><div class="vd-desc">${bbHTML(d.description)}</div>` : ''}
    ${d.traits.length ? h`<h3 class="vd-h">Traits <select id="cd-spoil" class="sm" style="margin-left:10px"><option value="0" ${dlg.spoil === 0 ? 'selected' : ''}>No spoilers</option><option value="1" ${dlg.spoil === 1 ? 'selected' : ''}>Minor spoilers</option><option value="2" ${dlg.spoil === 2 ? 'selected' : ''}>All spoilers</option></select></h3>
      ${groups.map(g => h`<div class="tiny muted" style="margin:8px 0 4px">${g}</div><div class="vd-tags">${traits.filter(t => t.group === g).map(t => h`<span class="tag">${t.name}</span>`)}</div>`)}` : ''}
    ${vns.length ? h`<h3 class="vd-h">Appears in</h3><div class="vd-rel">${vns.map(v => h`<button class="vd-relrow" data-vd="vn" data-id="${v.id}"><span class="tiny muted">${role(v.role)}</span><span>${v.title}</span></button>`)}</div>` : ''}`);
}
function dcDetailDraw(dlg) {
  const d = dlg.vn, inLib = S.lib.games.some(g => g.vndbId === d.id), nsfw = d.sexual >= 1;
  const tags = d.tags.filter(t => t.spoiler <= dlg.spoil).sort((x, y) => y.rating - x.rating);
  const cat = c => tags.filter(t => t.category === c);
  const chip = t => h`<button class="tag tagbtn" data-vd="tag" data-id="${t.id}" data-name="${t.name}" title="Search for this tag${t.spoiler ? ' (spoiler)' : ''}">${t.name}</button>`;
  const staff = {}; d.staff.forEach(s => { (staff[s.role] = staff[s.role] || []).push(s.name); });
  const info = [
    ['Released', d.released || ''], ['Status', DEV_STATUS[d.devStatus] || ''], ['Developer', d.developers.join(', ')],
    ['Original language', d.olang ? langName(d.olang) : ''], ['Available in', d.languages.map(langName).join(', ')],
    ['Platforms', d.platforms.map(platName).join(', ')],
    ['Length', d.length ? LENGTHS[d.length] + (d.lengthMinutes ? ` · avg ${fmtLen(d.lengthMinutes)}` : '') : (d.lengthMinutes ? 'avg ' + fmtLen(d.lengthMinutes) : '')],
    ['Rating', d.rating ? `★ ${(d.rating / 10).toFixed(2)} from ${d.votes} votes` : ''],
    ['Other titles', d.titles.filter(t => t.title !== d.title).map(t => `${t.title} (${langName(t.lang)})`).slice(0, 6).join(' · ')],
    ['Aliases', d.aliases.slice(0, 8).join(', ')],
  ].filter(r => r[1]);
  const prev = dlg.trail.length ? dlg.trail[dlg.trail.length - 1] : null;
  setHTML(dlg.body, h`${prev ? h`<button class="btn sm" data-vd="back" style="margin-bottom:10px" title="Back to ${prev.title}">${icon('back')}Back to ${prev.title}</button>` : ''}<div class="vd-top">
      <div class="vd-cover">${d.image ? h`<img class="${nsfw ? 'nsfw' : ''}" src="${d.image}" alt="" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(d.title)}">${[...d.title.trim()][0] || '?'}</div>`}</div>
      <div class="vd-head"><h2 style="margin:0">${d.title}</h2>${d.altTitle && d.altTitle !== d.title ? h`<div class="muted">${d.altTitle}</div>` : ''}
        <table class="vd-info">${info.map(([k, v]) => h`<tr><th>${k}</th><td>${v}</td></tr>`)}</table>
        <div class="row" style="gap:8px;margin-top:10px">${inLib ? h`<span class="tag" style="padding:6px 10px">In your library</span>` : h`<button class="btn primary" data-vd="add">${icon('plus')}Add to library</button>`}<button class="btn" data-vd="open">View on VNDB</button></div>
        ${d.producers.length ? h`<div class="vd-prod"><span class="tiny muted">More games from</span>${d.producers.slice(0, 6).map(p => h`<button class="btn sm" data-vd="prod" data-id="${p.id}" data-name="${p.name}" title="Show other games by ${p.name}">${p.name}<span class="tiny muted"> · ${p.developer && p.publisher ? 'Developer & publisher' : p.developer ? 'Developer' : 'Publisher'}</span></button>`)}</div>` : ''}</div></div>
    ${d.description ? h`<h3 class="vd-h">Description</h3><div class="vd-desc">${bbHTML(d.description)}</div>` : ''}
    ${d.tags.length ? h`<h3 class="vd-h">Tags <select id="vd-spoil" class="sm" style="margin-left:10px"><option value="0" ${dlg.spoil === 0 ? 'selected' : ''}>No spoilers</option><option value="1" ${dlg.spoil === 1 ? 'selected' : ''}>Minor spoilers</option><option value="2" ${dlg.spoil === 2 ? 'selected' : ''}>All spoilers</option></select></h3>
      ${cat('cont').length ? h`<div class="vd-tags">${cat('cont').map(chip)}</div>` : ''}
      ${cat('ero').length ? h`<div class="tiny muted" style="margin:8px 0 4px">Sexual content</div><div class="vd-tags">${cat('ero').map(chip)}</div>` : ''}
      ${cat('tech').length ? h`<div class="tiny muted" style="margin:8px 0 4px">Technical</div><div class="vd-tags">${cat('tech').map(chip)}</div>` : ''}` : ''}
    ${d.screenshots.length ? h`<h3 class="vd-h">Screenshots</h3>${dlg.shot !== null ? h`<div class="vd-big"><img class="${d.screenshots[dlg.shot].sexual >= 1 ? 'nsfw' : ''}" src="${d.screenshots[dlg.shot].url}" alt="" referrerpolicy="no-referrer"><button class="btn sm" data-vd="shot-x">Close preview</button></div>` : ''}
      <div class="vd-shots">${d.screenshots.map((s, i) => h`<button class="vd-shot" data-vd="shot" data-i="${i}"><img class="${s.sexual >= 1 ? 'nsfw' : ''}" src="${s.thumb || s.url}" alt="Screenshot ${i + 1}" loading="lazy" referrerpolicy="no-referrer"></button>`)}</div>` : ''}
    ${d.characters.length ? h`<h3 class="vd-h">Characters</h3><div class="vd-chars">${d.characters.map(c => h`<button class="vd-char" data-vd="char" data-id="${c.id}" title="Show details of ${c.name}"><div class="cimg">${c.image ? h`<img class="${c.sexual >= 1 ? 'nsfw' : ''}" src="${c.image}" alt="" loading="lazy" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(c.name)}">${[...c.name][0] || '?'}</div>`}</div><div><b>${c.name}</b>${c.original ? h`<div class="tiny muted">${c.original}</div>` : ''}<div class="tiny muted">${c.role === 'main' ? 'Protagonist' : c.role === 'primary' ? 'Main character' : c.role === 'side' ? 'Side character' : c.role === 'appears' ? 'Minor appearance' : ''}</div></div></button>`)}</div>` : ''}
    ${Object.keys(staff).length ? h`<h3 class="vd-h">Staff</h3><table class="vd-info">${Object.keys(staff).map(r => h`<tr><th>${ROLES[r] || r}</th><td>${[...new Set(staff[r])].slice(0, 12).join(', ')}${new Set(staff[r]).size > 12 ? ' …' : ''}</td></tr>`)}</table>` : ''}
    ${d.relations.length ? h`<h3 class="vd-h">Related</h3><div class="vd-rel">${d.relations.map(r => h`<button class="vd-relrow" data-vd="rel" data-id="${r.id}"><span class="tiny muted">${REL[r.relation] || r.relation}${r.official ? '' : ' (unofficial)'}</span><span>${r.title}</span></button>`)}</div>` : ''}`);
}
async function dcAddDetail(d) {
  const g = await api('POST', '/api/games', { title: d.title, originalTitle: d.altTitle || '', developer: d.developers.join(', '), releaseDate: d.released || '', description: bbPlain(d.description), vndbId: d.id, tags: d.tags.filter(t => t.spoiler === 0 && t.category !== 'tech').sort((x, y) => y.rating - x.rating).slice(0, 12).map(t => t.name), status: 'planned', nsfw: d.sexual >= 1, locale: S.lib.settings.defaultLocale || '', exePath: '', launchArgs: '', savePath: '', notes: '', rating: 0, categories: [], favorite: false, useWrapper: false });
  if (d.image) await api('POST', `/api/games/${g.id}/cover-from-url`, { url: d.image }).catch(() => { });
  await load(); toast(`Added “${d.title}” — open it to choose the game’s .exe`, 'ok');
}

/* ---------- statistics ---------- */
function renderStats() {
  const games = S.lib.games;
  const sess = games.flatMap(g => g.sessions.map(s => ({ g, t: new Date(s.start), sec: s.seconds })));
  const tot = sess.reduce((a, s) => a + s.sec, 0);
  const day = d => { const x = new Date(d); x.setHours(0, 0, 0, 0); return x; };
  const key = d => { const x = day(d); return `${x.getFullYear()}-${x.getMonth()}-${x.getDate()}`; };
  const perDay = {}; sess.forEach(s => { perDay[key(s.t)] = (perDay[key(s.t)] || 0) + s.sec; });
  const today = day(new Date());
  const weekAgo = new Date(today); weekAgo.setDate(weekAgo.getDate() - 6);
  const week = sess.filter(s => s.t >= weekAgo).reduce((a, s) => a + s.sec, 0);
  let streak = 0; { const d = new Date(today); if (!perDay[key(d)]) d.setDate(d.getDate() - 1); while (perDay[key(d)]) { streak++; d.setDate(d.getDate() - 1); } }
  const done = games.filter(g => g.status === 'completed').length;

  // heatmap: 26 weeks, Sunday-first
  const W = 26, start = new Date(today); start.setDate(start.getDate() - start.getDay() - (W - 1) * 7);
  const vals = []; for (let i = 0; i < W * 7; i++) { const d = new Date(start); d.setDate(d.getDate() + i); vals.push({ d, v: perDay[key(d)] || 0, f: d > today }); }
  const mx = Math.max(1, ...vals.map(x => x.v));
  const lvl = v => v ? Math.min(4, Math.ceil(v / mx * 4)) : 0;
  const cs = 30, gap = 5, left = 38, top = 20;
  let months = '', lastM = -1;
  for (let w = 0; w < W; w++) { const d = vals[w * 7].d; if (d.getMonth() !== lastM) { lastM = d.getMonth(); months += `<text x="${left + w * (cs + gap)}" y="12" font-size="12" fill="var(--muted)">${d.toLocaleDateString(undefined, { month: 'short' })}</text>`; } }
  const cells = vals.map((x, i) => { const w = Math.floor(i / 7), r = i % 7;
    return `<rect x="${left + w * (cs + gap)}" y="${top + r * (cs + gap)}" width="${cs}" height="${cs}" rx="3" class="${x.f ? 'lf' : 'l' + lvl(x.v)}"><title>${x.d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })} — ${x.v ? fmtDur(x.v) : 'no play'}</title></rect>`; }).join('');
  const dl = ['', 'Mon', '', 'Wed', '', 'Fri', ''].map((t, r) => t ? `<text x="0" y="${top + r * (cs + gap) + 20}" font-size="12" fill="var(--muted)">${t}</text>` : '').join('');
  const hw = left + W * (cs + gap), hh = top + 7 * (cs + gap);

  /* ---- area ("mountain") chart: playtime per day / week / month ---- */
  const range = S.statRange || '30d';
  const dshort = d => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  const pts = [];
  if (range === '30d') {
    for (let i = 29; i >= 0; i--) { const d = new Date(today); d.setDate(d.getDate() - i); pts.push({ v: perDay[key(d)] || 0, lab: dshort(d), tip: d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' }) }); }
  } else if (range === '12w') {
    for (let i = 11; i >= 0; i--) {
      const e = new Date(today); e.setDate(e.getDate() - i * 7); const s = new Date(e); s.setDate(s.getDate() - 6);
      let v = 0; for (let k = 0; k < 7; k++) { const d = new Date(s); d.setDate(d.getDate() + k); v += perDay[key(d)] || 0; }
      pts.push({ v, lab: dshort(s), tip: `${dshort(s)} – ${dshort(e)}` });
    }
  } else {
    const perMonth = {}; sess.forEach(s => { const k = s.t.getFullYear() * 12 + s.t.getMonth(); perMonth[k] = (perMonth[k] || 0) + s.sec; });
    for (let i = 11; i >= 0; i--) { const d = new Date(today.getFullYear(), today.getMonth() - i, 1); pts.push({ v: perMonth[d.getFullYear() * 12 + d.getMonth()] || 0, lab: d.toLocaleDateString(undefined, { month: 'short' }), tip: d.toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) }); }
  }
  const rangeSum = pts.reduce((a, p) => a + p.v, 0), peak = pts.reduce((a, p) => p.v > a.v ? p : a, pts[0]);
  const unit = { '30d': 'day', '12w': 'week', '12m': 'month' }[range];
  const AW = 1000, AH = 260, aL = 48, aR = 14, aT = 14, aB = 28, pw = AW - aL - aR, ph = AH - aT - aB;
  const amax = Math.max(...pts.map(p => p.v));
  const steps = [600, 900, 1800, 3600, 7200, 10800, 21600, 43200, 86400, 172800, 432000];
  const astep = steps.find(s => s >= Math.max(amax, 1800) / 4) || 432000, atop = Math.ceil(Math.max(amax, 1800) / astep) * astep;
  const ax = i => aL + i * pw / (pts.length - 1), ay = v => aT + ph * (1 - v / atop);
  const tick = v => v >= 3600 ? (v / 3600) + 'h' : Math.round(v / 60) + 'm';
  const mono = (xs, ys) => {
    const n = xs.length, dx = [], m = [], t = [];
    for (let i = 0; i < n - 1; i++) { dx[i] = xs[i + 1] - xs[i]; m[i] = (ys[i + 1] - ys[i]) / dx[i]; }
    t[0] = m[0]; t[n - 1] = m[n - 2];
    for (let i = 1; i < n - 1; i++) t[i] = m[i - 1] * m[i] <= 0 ? 0 : (m[i - 1] + m[i]) / 2;
    for (let i = 0; i < n - 1; i++) {
      if (m[i] === 0) { t[i] = t[i + 1] = 0; continue; }
      const a = t[i] / m[i], b = t[i + 1] / m[i], s = a * a + b * b;
      if (s > 9) { const k = 3 / Math.sqrt(s); t[i] = k * a * m[i]; t[i + 1] = k * b * m[i]; }
    }
    let d = `M${xs[0].toFixed(1)},${ys[0].toFixed(1)}`;
    for (let i = 0; i < n - 1; i++) d += ` C${(xs[i] + dx[i] / 3).toFixed(1)},${(ys[i] + t[i] * dx[i] / 3).toFixed(1)} ${(xs[i + 1] - dx[i] / 3).toFixed(1)},${(ys[i + 1] - t[i + 1] * dx[i] / 3).toFixed(1)} ${xs[i + 1].toFixed(1)},${ys[i + 1].toFixed(1)}`;
    return d;
  };
  const xs = pts.map((_, i) => ax(i)), ys = pts.map(p => ay(p.v)), base = ay(0);
  const line = mono(xs, ys), area = `${line} L${xs[xs.length - 1].toFixed(1)},${base} L${xs[0].toFixed(1)},${base} Z`;
  const grid = []; for (let v = 0; v <= atop; v += astep) grid.push(`<line x1="${aL}" x2="${AW - aR}" y1="${ay(v)}" y2="${ay(v)}" stroke="var(--line)" ${v ? 'stroke-dasharray="3 5"' : ''}/><text x="${aL - 8}" y="${ay(v) + 4}" font-size="12" text-anchor="end" fill="var(--muted)">${v ? tick(v) : '0'}</text>`);
  const every = Math.ceil(pts.length / 6);
  const xlab = pts.map((p, i) => i % every === 0 || (i === pts.length - 1 && (pts.length - 1) % every >= every / 2) ? `<text x="${xs[i]}" y="${AH - 6}" font-size="12" text-anchor="${i === 0 ? 'start' : i === pts.length - 1 ? 'end' : 'middle'}" fill="var(--muted)">${esc(p.lab)}</text>` : '').join('');
  const half = pw / (pts.length - 1) / 2;
  const hover = pts.map((p, i) => `<g class="ap"><rect x="${xs[i] - half}" y="${aT}" width="${half * 2}" height="${ph}" fill="transparent"/><line x1="${xs[i]}" x2="${xs[i]}" y1="${aT}" y2="${base}"/><circle cx="${xs[i]}" cy="${ys[i]}" r="5"/><title>${esc(p.tip)} — ${p.v ? fmtDur(p.v) : 'no play'}</title></g>`).join('');

  /* ---- donut ("pie") chart: playtime per game, all time ---- */
  const palette = ['#ff5c7a', '#8f7cff', '#5fc59a', '#e9b95a', '#7fa6f5', '#e07fd0', '#4cc3d9'];
  const ranked = games.filter(g => total(g)).sort((a, b) => total(b) - total(a));
  const slices = ranked.slice(0, 7).map((g, i) => ({ id: g.id, name: g.title, v: total(g), c: palette[i] }));
  if (ranked.length > 7) slices.push({ name: `Other (${ranked.length - 7} game${ranked.length - 7 > 1 ? "s" : ""})`, v: ranked.slice(7).reduce((a, g) => a + total(g), 0), c: 'var(--muted)' });
  const R = 74, C = 2 * Math.PI * R, gapLen = slices.length > 1 ? 2.5 : 0;
  let cum = 0;
  const rings = slices.map(s => { const len = s.v / tot * C, o = cum; cum += len;
    return `<circle class="seg" cx="110" cy="110" r="${R}" fill="none" stroke="${s.c}" stroke-width="34" stroke-dasharray="${Math.max(len - gapLen, 0.5)} ${C}" stroke-dashoffset="${-o}" transform="rotate(-90 110 110)"><title>${esc(s.name)} — ${fmtDur(s.v)} (${Math.round(s.v / tot * 100)}%)</title></circle>`; }).join('');
  const sc = k => games.filter(g => g.status === k).length;
  const col = { planned: 'var(--muted)', playing: 'var(--info)', completed: 'var(--good)', onhold: 'var(--warn)', dropped: 'var(--bad)' };

  return h`<h1>Statistics</h1><p class="sub">Everything ShioriBox has timed while you played.</p>
    <div class="tiles">
      <div class="tile"><b>${fmtDur(tot)}</b><span>Total playtime</span></div>
      <div class="tile"><b>${fmtDur(week)}</b><span>Last 7 days</span></div>
      <div class="tile"><b>${streak} day${streak === 1 ? '' : 's'}</b><span>Current streak</span></div>
      <div class="tile"><b>${done} / ${games.length}</b><span>Completed</span></div>
    </div>
    <div class="panel"><div class="ph"><div><h2>Playtime over time</h2>${rangeSum ? h`<p class="muted tiny">${fmtDur(rangeSum)} in this range · busiest ${unit}: ${peak.tip} (${fmtDur(peak.v)})</p>` : h`<p class="muted tiny">No play recorded in this range yet.</p>`}</div>
      <div class="seg-ctl">${[['30d', '30 days'], ['12w', '12 weeks'], ['12m', '12 months']].map(([k, l]) => h`<button class="chip ${range === k ? 'on' : ''}" data-act="stat-range" data-r="${k}">${l}</button>`)}</div></div>
      <svg class="area" viewBox="0 0 ${AW} ${AH}" width="100%" role="img" aria-label="Playtime per ${unit}"><defs><linearGradient id="ag" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="var(--accent)" stop-opacity=".55"/><stop offset="1" stop-color="var(--accent2)" stop-opacity=".04"/></linearGradient></defs>
        ${raw(grid.join('') + `<path d="${area}" fill="url(#ag)"/><path d="${line}" fill="none" stroke="var(--accent)" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round"/>` + xlab + hover)}</svg></div>
    <div class="split">
      <div class="panel"><h2>Playtime by game</h2>${slices.length ? h`<div class="donut-wrap">
        <svg class="donut" viewBox="0 0 220 220" role="img" aria-label="Share of total playtime per game">${raw(`<circle cx="110" cy="110" r="${R}" fill="none" stroke="var(--surface2)" stroke-width="34"/>` + rings)}
          <text x="110" y="107" text-anchor="middle" font-size="24" font-weight="600" fill="var(--text)" font-family="var(--serif)">${fmtDur(tot)}</text><text x="110" y="127" text-anchor="middle" font-size="12" fill="var(--muted)">all time</text></svg>
        <div class="dl">${slices.map(s => h`<div class="dr"><span class="dot" style="background:${s.c}"></span>${s.id ? h`<span class="nm" data-act="open" data-id="${s.id}">${s.name}</span>` : h`<span class="nm" style="cursor:default">${s.name}</span>`}<span class="v">${fmtDur(s.v)}</span><span class="pc">${Math.round(s.v / tot * 100)}%</span></div>`)}</div></div>` : h`<p class="muted">Play a game from ShioriBox and it will show up here.</p>`}</div>
      <div class="panel"><h2>Library by status</h2>
        <div class="stack">${Object.keys(STATUS).map(k => h`<i style="width:${games.length ? sc(k) / games.length * 100 : 0}%;background:${col[k]}"></i>`)}</div>
        <div class="dl" style="margin-top:14px">${Object.keys(STATUS).map(k => h`<div class="dr"><span class="dot" style="background:${col[k]}"></span><span class="nm" style="cursor:default">${STATUS[k]}</span><span class="v">${sc(k)}</span><span class="pc">${games.length ? Math.round(sc(k) / games.length * 100) : 0}%</span></div>`)}</div></div>
    </div>
    <div class="panel"><h2>Activity — last 26 weeks</h2>
      <svg class="heat" viewBox="0 0 ${hw} ${hh}" role="img" aria-label="Play activity heatmap">${raw(months + dl + cells)}</svg></div>`;
}

/* ---------- settings ---------- */
function renderSettings() {
  const st = S.lib.settings;
  return h`<div class="set"><h1>Settings</h1><p class="sub">Everything is stored locally in <code>${S.info.dataDir}</code></p>
    <div class="panel"><h2>Appearance</h2>
      <label class="f"><span>Theme</span><select id="s-theme"><option value="dark" ${st.theme === 'dark' ? 'selected' : ''}>Dark</option><option value="light" ${st.theme === 'light' ? 'selected' : ''}>Light</option></select></label></div>
    <div class="panel"><h2>Japanese locale</h2>
      <p class="muted" style="margin-top:0">Many older Japanese games only display correctly when Windows is set to Japanese. With this on, ShioriBox starts the game with a Japanese locale, so just that game sees a Japanese PC — your Windows settings are not changed. Turn it on per game (Edit tab) or for all new games below.</p>
      <label class="f"><span>Engine</span><select id="s-eng"><option value="jle" ${st.localeEngine === 'jle' ? 'selected' : ''}>Shiori Locale</option><option value="custom" ${st.localeEngine === 'custom' ? 'selected' : ''}>Add Locale Emu (use your own)</option></select></label>
      <p class="hint" style="margin-top:-4px">After changing the engine, press <b>Save settings</b>.</p>
      ${st.localeEngine === 'custom' ? h`<label class="f"><span>Locale emulator program (for example Locale Emulator’s LEProc.exe or your own)</span><div class="pathrow"><input type="text" id="s-lp" value="${st.localePath}" placeholder="e.g. C:\\Tools\\MyEmulator.exe"><button class="btn" data-act="pick-lp">Browse</button></div></label>
      <label class="f"><span>Arguments — <code>{exe}</code> is the game; leave empty for “program game.exe”</span><input type="text" id="s-la" value="${st.localeArgs}" placeholder="{exe}"></label>` : h`<p style="margin:0 0 12px"><span class="dot" style="background:${(S.info.engine || {}).ready ? 'var(--good)' : 'var(--bad)'}"></span> ${(S.info.engine || {}).ready ? h`<b>Ready.</b> ShioriLocale is unpacked in <code>${S.info.engine.dir}</code>. Its options are in <code>jle.ini</code> in that folder.` : h`<b>The engine could not be unpacked.</b> ${(S.info.engine || {}).problem || ''}`}</p>`}
      <input type="hidden" id="s-keep-le" value="">
      <label class="f"><span>New games and scan imports start with</span><select id="s-dloc"><option value="" ${!st.defaultLocale ? 'selected' : ''}>System locale (normal)</option><option value="ja" ${st.defaultLocale === 'ja' ? 'selected' : ''}>Japanese locale</option></select></label>
      <p class="hint" style="margin-bottom:0">Prefer to change Windows itself instead? <button class="btn sm ghost" data-act="open-region">Open Windows region settings</button> → Administrative → “Change system locale…” → Japanese (needs a restart and affects the whole PC).</p></div>
    <div class="panel"><h2>Custom launcher &amp; timing</h2>
      <label class="f"><span>Other launcher program (optional)</span><div class="pathrow"><input type="text" id="s-wp" value="${st.wrapperPath}" placeholder="e.g. C:\\Tools\\Something\\launcher.exe"><button class="btn" data-act="pick-wrapper">Browse</button></div></label>
      <label class="f"><span>Launcher arguments — <code>{exe}</code> is replaced by the game’s executable</span><input type="text" id="s-wa" value="${st.wrapperArgs}"></label>
      <p class="hint">Choose “Custom launcher program” for a game to use this. Launchers that exit right after starting the game can’t be timed automatically — use “Add time” on the Playtime tab.</p>
      <label class="chk" style="margin:4px 0 12px"><input type="checkbox" id="s-reveal" ${st.revealOnHover ? 'checked' : ''}>Un-blur NSFW covers while the pointer is over them</label>
      <label class="f"><span>Ignore sessions shorter than (seconds)</span><input type="number" id="s-min" min="1" value="${st.minSessionSec}"></label></div>
    <div class="panel"><h2>Save backups</h2>
      <label class="chk" style="margin-bottom:12px"><input type="checkbox" id="s-auto" ${st.autoBackup ? 'checked' : ''}>Back up a game’s saves automatically when it closes</label>
      <label class="f"><span>Keep this many automatic backups per game (0 = keep all)</span><input type="number" id="s-keep" min="0" value="${st.backupKeep}"></label></div>
    <div class="row" style="margin-bottom:18px"><button class="btn primary" data-act="save-settings">Save settings</button></div>
    <div class="panel"><h2>Auto-add games</h2><p class="muted" style="margin-top:0">Pick the folders where you keep your visual novels. ShioriBox looks through them (including nested folders) when it starts, every 15 minutes while it's open, and whenever you press Scan now, and adds any game it hasn't seen before. A game you remove from the library is not added again.</p>
      <div id="auto-box"><p class="muted">Loading…</p></div></div>
    <div class="panel"><h2>Privacy hotkey</h2><p class="muted" style="margin-top:0">One key combination that instantly blurs ShioriBox and hides it in the system tray — from anywhere, even while another program is in front. Nothing closes: your place, running games and play timers carry on. Press it again (or click the tray icon) to bring the app back; it stays blurred until you click it.</p>
      <div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="btn" id="pk-btn" data-act="pk-set" style="min-width:190px">${st.panicKey || 'Not set — click to choose'}</button>${st.panicKey ? h`<button class="btn sm" data-act="pk-clear">Remove</button><button class="btn sm" data-act="pk-test">Try it now</button>` : ''}</div>
      <p class="hint" id="pk-st" style="margin:8px 0 14px">Click the button, then press the keys you want, for example Ctrl + Alt + H. Esc cancels.</p>
      <div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><strong style="font-size:13px">Or: tap one key quickly</strong>
        <button class="btn" id="pk-tapbtn" data-act="pk-tapkey" style="min-width:120px">${st.panicTapKey || 'Choose key'}</button><span class="muted">pressed</span>
        <select id="pk-taps">${[2, 3, 4, 5, 6].map(n => h`<option value="${n}" ${(st.panicTaps || 3) === n ? 'selected' : ''}>${n} times</option>`)}</select><span class="muted">within half a second each</span>
        ${st.panicTapKey ? h`<button class="btn sm" data-act="pk-tapclear">Remove</button>` : ''}</div>
      <p class="hint" id="pk-tapst" style="margin-bottom:0">For example Esc three times. The key still reaches the other program too, and pressing any other key in between starts the count over. A letter key can trigger while you type, so a key like Esc or F8 is safer. Doesn't work over programs running as administrator.</p></div>
    <div class="panel"><h2>Library backup</h2><p class="muted" style="margin-top:0">Export your whole library (details, playtime, covers) to a zip, or restore from one. Save-file backups stay in the data folder.</p>
      <div class="row"><a class="btn" href="/api/export" download>Export library</a><button class="btn" data-act="import-lib">Import library…</button><input type="file" id="libfile" accept=".zip" hidden></div></div>
    <div class="panel"><h2>Background picture</h2><p class="muted" style="margin-top:0">Show a picture behind the whole app. Changes apply as you move the sliders.</p>
      <div class="row" style="align-items:center;gap:14px;flex-wrap:wrap">${st.bgImage ? h`<div class="bgprev" style="background-image:url('/background/${encodeURIComponent(st.bgImage)}')"></div>` : ''}
        <div class="row" style="gap:8px"><button class="btn primary" data-act="bg-pick">${st.bgImage ? 'Change picture…' : 'Choose picture…'}</button>${st.bgImage ? h`<button class="btn" data-act="bg-remove">Remove</button>` : ''}<input type="file" id="bgfile" accept="image/png,image/jpeg,image/gif,image/webp" hidden></div></div>
      ${st.bgImage ? h`<div class="sliders">
        <label><span>Blur <output id="o-blur">${st.bgBlur}px</output></span><input type="range" id="bg-blur" min="0" max="40" value="${st.bgBlur}"></label>
        <label><span>Overlay (dims the picture) <output id="o-ov">${st.bgOverlay}%</output></span><input type="range" id="bg-ov" min="0" max="95" value="${st.bgOverlay}"></label>
        <label><span>Card &amp; sidebar opacity <output id="o-pn">${st.bgPanel}%</output></span><input type="range" id="bg-pn" min="30" max="100" value="${st.bgPanel}"></label>
        <label><span>Picture position</span><select id="bg-pos"><option value="center" ${st.bgPos === 'center' ? 'selected' : ''}>Center</option><option value="top" ${st.bgPos === 'top' ? 'selected' : ''}>Top</option><option value="bottom" ${st.bgPos === 'bottom' ? 'selected' : ''}>Bottom</option></select></label>
      </div><button class="btn sm" data-act="bg-reset">Reset sliders</button>` : ''}
    </div>
    <div class="panel"><h2>Library snapshots</h2><p class="muted" style="margin-top:0">ShioriBox keeps small copies of your library list (details, playtime, collections) in the <code>library-backups</code> folder inside the data folder: when it starts, every few hours while open, before removing games in bulk and before a restore. Only the newest 3 are kept; older ones are deleted automatically. Cover images and save files are not in them — export the library zip above or copy the whole data folder for a complete backup.</p>
      <div class="row" style="margin-bottom:12px"><button class="btn primary" data-act="snap-now">Back up now</button><button class="btn" data-act="snap-open">Open backup folder</button></div><div id="snaplist"><p class="muted">Loading…</p></div></div>
    <div class="panel"><h2>Updates</h2><p class="muted" style="margin-top:0">ShioriBox ${S.info.version} is installed.</p>
      <div id="upd-box"><p class="muted">Loading…</p></div></div>
    <div class="panel about"><img src="/logo.png" alt=""><div><h2>About</h2><p class="muted" style="margin-top:0">ShioriBox ${S.info.version}. Game metadata and covers come from <a href="https://vndb.org" target="_blank" rel="noopener">VNDB</a> (data under the ODbL; cover images belong to their owners). The app only listens on this computer (127.0.0.1).</p>
      <button class="btn danger" data-act="quit">Quit ShioriBox</button></div></div></div>`;
}

/* ---------- main render ---------- */
function render() {
  document.body.classList.toggle('reveal', !!S.lib.settings.revealOnHover);
  renderSide();
  const m = $('#main');
  const keepSearch = document.activeElement && document.activeElement.id === 'q';
  const pos = m.scrollTop;
  if (S.view === 'anime') { animePage(m); return; }
  animeLeave();
  setHTML(m, S.view === 'home' ? renderHome() : S.view === 'discover' ? renderDiscover() : S.view === 'stats' ? renderStats() : S.view === 'settings' ? renderSettings() : renderLibrary());
  m.scrollTop = pos;
  if (S.view === 'settings') { loadSnaps(); loadPanic(); loadAuto(); loadUpd(); }
  if (S.view === 'discover') dcFill();
  if (keepSearch) { const q = $('#q'); q.focus(); q.setSelectionRange(q.value.length, q.value.length); }
}
/* ---------- updates ---------- */
function drawUpd() {
  const box = $('#upd-box'), u = S.upd; if (!box || !u) return;
  const i = u.info || {};
  let st = '';
  if (i.available) st = h`<div class="upd-new"><b>ShioriBox ${i.latest} is available</b> <span class="muted">(you have ${i.current})</span>
      ${i.notes ? h`<pre class="upd-notes">${i.notes}</pre>` : ''}
      <div class="row" style="gap:8px;flex-wrap:wrap;margin-top:10px">${i.download && u.canInstall ? h`<button class="btn primary" data-act="upd-install">Update now</button>` : ''}${i.page ? h`<a class="btn" href="${i.page}" target="_blank" rel="noopener noreferrer">Release page</a>` : ''}${!i.skipped ? h`<button class="btn" data-act="upd-skip" data-v="${i.latest}">Skip this version</button>` : ''}</div>
      <p class="hint" style="margin:8px 0 0">${i.download && u.canInstall ? 'Update now downloads the update, closes ShioriBox and installs it. Your library and settings are kept.' : 'This release has no automatic update attached. Open the release page to download it.'}</p></div>`;
  else if (i.error && i.checked) st = h`<p class="hint" style="color:var(--bad);margin:10px 0 0">${i.error}</p>`;
  else if (i.checked) st = h`<p class="hint" style="margin:10px 0 0">You're up to date. Checked ${new Date(i.checked).toLocaleString()}.</p>`;
  setHTML(box, h`<div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="btn" data-act="upd-check">Check for updates</button><span class="muted tiny">ShioriBox also checks by itself every time it starts.</span></div>${st}`);
}
async function loadUpd() {
  try { S.upd = await api('GET', '/api/update'); } catch { return; }
  drawUpd();
}
// the popup that tells you an update exists; clicking it opens Settings → Updates
function updPopup(i) {
  if (!i || !i.available || i.skipped || S.updShown === i.latest) return;
  S.updShown = i.latest;
  const old = $('#upd-pop'); if (old) old.remove();
  const p = document.createElement('div'); p.id = 'upd-pop'; p.setAttribute('role', 'alert');
  setHTML(p, h`<button class="up-x" aria-label="Dismiss" data-x="1">×</button><b>Update available</b><span>ShioriBox ${i.latest} is ready. Click here to update.</span>`);
  p.addEventListener('click', e => { p.remove(); if (e.target.closest('[data-x]')) return; S.view = 'settings'; render(); });
  document.body.appendChild(p);
}
async function updPoll() {
  try {
    S.upd = await api('GET', '/api/update');
    renderSide();
    if (S.view === 'settings') drawUpd();
    updPopup(S.upd.info);
  } catch { }
}
document.addEventListener('click', async e => {
  const b = e.target.closest('[data-act^="upd-"]'); if (!b) return;
  const a = b.dataset.act;
  if (a === 'upd-check') {
    b.disabled = true; b.textContent = 'Checking…';
    try { S.upd = await api('POST', '/api/update/check', {}); updPopup(S.upd.info); } catch (er) { toast(er.message, 'err'); }
    drawUpd(); renderSide();
  } else if (a === 'upd-skip') { try { S.upd = await api('PUT', '/api/update/skip', { version: b.dataset.v }); } catch (er) { toast(er.message, 'err'); } drawUpd(); renderSide(); }
  else if (a === 'upd-install') {
    b.disabled = true; b.textContent = 'Downloading…';
    try { await api('POST', '/api/update/install', {}); toast('Starting the installer… ShioriBox will close.', 'ok'); }
    catch (er) { toast(er.message, 'err'); b.disabled = false; b.textContent = 'Update now'; }
  } else if (a === 'upd-open') { S.view = 'settings'; render(); }
});
// right after launch (the app checks as it starts), then every few minutes while it is open
setTimeout(updPoll, 7000); setTimeout(updPoll, 20000); setInterval(updPoll, 300000);

/* ---------- auto-add folders ---------- */
function drawAuto() {
  const box = $('#auto-box'), a = S.auto; if (!box || !a) return;
  const when = a.last ? new Date(a.last).toLocaleString() : '';
  setHTML(box, h`${a.folders.length ? h`<div class="auto-list">${a.folders.map((f, i) => h`<div class="auto-row"><code>${f}</code><button class="btn sm" data-act="auto-del" data-i="${i}">Remove</button></div>`)}</div>` : h`<p class="muted" style="margin:0 0 10px">No folders yet.</p>`}
    <div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="btn primary" data-act="auto-add">Add folder…</button>${a.folders.length ? h`<button class="btn" data-act="auto-scan" ${a.busy ? 'disabled' : ''}>${a.busy ? 'Scanning…' : 'Scan now'}</button>` : ''}</div>
    <label class="chk" style="margin-top:12px"><input type="checkbox" id="auto-lookup" ${a.lookupOff ? '' : 'checked'}>Look up details &amp; covers on VNDB for games added this way</label>
    <p class="hint" style="margin-bottom:0" id="auto-st" ${a.error ? 'style="color:var(--bad)"' : ''}>${a.error ? a.error : a.last ? 'Last scan: ' + when + '.' : a.folders.length ? 'Not scanned yet.' : ''}</p>`);
}
async function loadAuto() {
  try { S.auto = await api('GET', '/api/auto-add'); } catch { return; }
  drawAuto();
}
async function saveAuto(folders, lookupOff) {
  try { S.auto = await api('PUT', '/api/auto-add', { folders, lookupOff }); } catch (e) { return toast(e.message, 'err'); }
  drawAuto();
}
async function runAutoScan() {
  if (S.auto) { S.auto.busy = true; drawAuto(); }
  try {
    const d = await api('POST', '/api/auto-add/scan', {});
    S.auto = d; S.autoSeq = d.seq;
    await load();
    toast(d.added ? `Added ${d.added} new game${d.added === 1 ? '' : 's'}` : 'No new games found', d.added ? 'ok' : '');
    if (d.added && !d.lookupOff) enrichAll(d.recent);
  } catch (e) { toast(e.message, 'err'); if (S.auto) S.auto.busy = false; }
  drawAuto();
}
document.addEventListener('click', async e => {
  const b = e.target.closest('[data-act^="auto-"]'); if (!b || !S.auto) return;
  const a = b.dataset.act;
  if (a === 'auto-add') {
    const p = await pickPath({ mode: 'dir', start: S.auto.folders[0] || '', title: 'Choose a folder to watch for games' });
    if (p) { await saveAuto([...S.auto.folders, p], S.auto.lookupOff); runAutoScan(); }
  } else if (a === 'auto-del') saveAuto(S.auto.folders.filter((_, i) => i !== +b.dataset.i), S.auto.lookupOff);
  else if (a === 'auto-scan') runAutoScan();
});
document.addEventListener('change', e => { if (e.target.id === 'auto-lookup' && S.auto) saveAuto(S.auto.folders, !e.target.checked); });
// games found by the background scan show up on their own
async function autoPoll() {
  if (document.hidden) return;
  try {
    const d = await api('GET', '/api/auto-add');
    if (S.autoSeq !== undefined && d.seq !== S.autoSeq) {
      S.autoSeq = d.seq; S.auto = d; await load();
      toast(`Added ${d.recent.length} new game${d.recent.length === 1 ? '' : 's'} from your watched folders`, 'ok');
      if (!d.lookupOff) enrichAll(d.recent);
    }
    S.autoSeq = d.seq;
  } catch { }
}
setInterval(autoPoll, 30000);
autoPoll();

/* ---------- privacy hotkey ---------- */
const PK_NAMES = { Space: 'Space', Tab: 'Tab', Backspace: 'Backspace', Enter: 'Enter', Escape: 'Esc', PageUp: 'PageUp', PageDown: 'PageDown', End: 'End', Home: 'Home', ArrowLeft: 'Left', ArrowUp: 'Up', ArrowRight: 'Right', ArrowDown: 'Down', Insert: 'Insert', Delete: 'Delete', Backquote: '`', Minus: '-', Equal: '=', BracketLeft: '[', BracketRight: ']', Backslash: '\\', Semicolon: ';', Quote: "'", Comma: ',', Period: '.', Slash: '/' };
const pkKey = e => /^Key[A-Z]$/.test(e.code) ? e.code.slice(3) : /^Digit\d$/.test(e.code) ? e.code.slice(5) : /^F\d{1,2}$/.test(e.code) ? e.code : (PK_NAMES[e.code] || '');
const pkCombo = e => { const k = pkKey(e); return k ? [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && 'Win', k].filter(Boolean).join('+') : ''; };
function panicNow() {
  document.body.classList.add('panic');
  document.querySelectorAll('video').forEach(v => { try { v.pause(); } catch { } });
  document.querySelectorAll('.an-player iframe').forEach(f => { f.src = 'about:blank'; }); // embedded players keep playing sound otherwise
}
window.__panic = panicNow;
(() => {
  const mk = () => { if (document.getElementById('panic')) return; const o = document.createElement('div'); o.id = 'panic'; o.innerHTML = '<b>ShioriBox</b><span>Click to continue</span>'; o.onclick = () => document.body.classList.remove('panic'); document.body.appendChild(o); };
  if (document.body) mk(); else document.addEventListener('DOMContentLoaded', mk);
})();
async function loadPanic() {
  const st = $('#pk-st'); if (!st) return;
  try {
    const d = await api('GET', '/api/panic');
    if (d.error) { st.textContent = d.error; st.style.color = 'var(--danger, #e5534b)'; }
    else if (d.registered) { st.textContent = 'Active. Press ' + d.key + ' anywhere to hide ShioriBox.'; st.style.color = ''; }
    const ts = $('#pk-tapst');
    if (ts && d.tapsError) { ts.textContent = d.tapsError; ts.style.color = 'var(--danger, #e5534b)'; }
    else if (ts && d.tapsRegistered) { ts.textContent = 'Active. Tap ' + d.tapKey + ' ' + d.taps + ' times quickly, anywhere, to hide ShioriBox. Pressing any other key in between starts the count over. A letter key can trigger while you type, so a key like Esc or F8 is safer.'; ts.style.color = ''; }
  } catch { }
}
async function setPanicKey(key) {
  try { S.lib.settings.panicKey = (await api('PUT', '/api/panic/key', { key })).key; } catch (e) { toast(e.message, 'err'); }
  if (S.view === 'settings') { const m = $('#main'), pos = m.scrollTop; setHTML(m, renderSettings()); m.scrollTop = pos; loadSnaps(); loadPanic(); }
}
async function setTapKey(key, count) {
  try { const d = await api('PUT', '/api/panic/taps', { key, count }); S.lib.settings.panicTapKey = d.tapKey; S.lib.settings.panicTaps = d.taps; } catch (e) { toast(e.message, 'err'); }
  if (S.view === 'settings') { const m = $('#main'), pos = m.scrollTop; setHTML(m, renderSettings()); m.scrollTop = pos; loadSnaps(); loadPanic(); }
}
let pkCapturing = false; // false | 'combo' | 'tap'
// quick taps while the page itself has focus (the Windows app watches the whole keyboard instead)
const pkTap = { n: 0, last: 0, held: false };
function pkTapCheck(e, down) {
  const s = S.lib && S.lib.settings, key = s && s.panicTapKey; if (!key) return;
  if (pkKey(e) !== key) { if (down) pkTap.n = 0; return; }
  if (!down) { pkTap.held = false; return; }
  if (pkTap.held) return; pkTap.held = true;
  const now = performance.now(); if (pkTap.n && now - pkTap.last > 500) pkTap.n = 0;
  pkTap.n++; pkTap.last = now;
  if (pkTap.n >= (s.panicTaps || 3)) { pkTap.n = 0; panicNow(); }
}
document.addEventListener('keyup', e => pkTapCheck(e, false), true);
document.addEventListener('change', e => { if (e.target.id === 'pk-taps' && S.lib.settings.panicTapKey) setTapKey(S.lib.settings.panicTapKey, +e.target.value); });
document.addEventListener('click', e => {
  const b = e.target.closest('[data-act^="pk-"]'); if (!b) return;
  const a = b.dataset.act;
  if (a === 'pk-tapkey') { pkCapturing = pkCapturing === 'tap' ? false : 'tap'; b.textContent = pkCapturing ? 'Press the key…' : (S.lib.settings.panicTapKey || 'Choose key'); }
  else if (a === 'pk-tapclear') setTapKey('', +($('#pk-taps') || {}).value || 3);
  else if (a === 'pk-set') { pkCapturing = 'combo'; b.textContent = 'Press the keys…'; const st = $('#pk-st'); if (st) { st.textContent = 'Hold your modifiers (Ctrl, Alt, Shift, Win) and press a key. Esc cancels.'; st.style.color = ''; } }
  else if (a === 'pk-clear') setPanicKey('');
  else if (a === 'pk-test') api('POST', '/api/panic/now', {}).catch(err => toast(err.message, 'err'));
});
document.addEventListener('keydown', e => {
  if (pkCapturing) {
    e.preventDefault(); e.stopPropagation();
    if (['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) return;
    if (pkCapturing === 'tap') {
      pkCapturing = false;
      const k = pkKey(e); if (!k) { toast('That key can\'t be used', 'err'); return loadPanic(); }
      return setTapKey(k, +($('#pk-taps') || {}).value || 3);
    }
    pkCapturing = false;
    if (e.key === 'Escape' && !e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey) { const b = $('#pk-btn'); if (b) b.textContent = S.lib.settings.panicKey || 'Not set — click to choose'; return; }
    const c = pkCombo(e);
    if (!c) { toast('That key can\'t be used', 'err'); return loadPanic(); }
    setPanicKey(c);
    return;
  }
  // In the browser fallback there is no global hotkey; the same combination still blurs the page while it has focus.
  const want = S.lib && S.lib.settings && S.lib.settings.panicKey;
  if (want && pkCombo(e) === want) { e.preventDefault(); panicNow(); }
  pkTapCheck(e, true);
}, true);
async function loadSnaps() {
  const box = $('#snaplist'); if (!box) return;
  try {
    const list = await api('GET', '/api/snapshots');
    const lab = n => n === 'auto' ? 'automatic' : n === 'manual' ? 'manual' : n === 'bulk' ? 'before bulk remove' : n === 'prerestore' ? 'before a restore' : '';
    setHTML(box, list.length ? h`<table class="t"><tr><th>Taken</th><th>Games</th><th></th></tr>${list.slice(0, 12).map(s => h`<tr><td>${new Date(s.time).toLocaleString()} ${s.note ? h`<span class="tag">${lab(s.note)}</span>` : ''}</td><td>${s.games}</td><td style="text-align:right;white-space:nowrap"><button class="btn sm" data-act="snap-restore" data-n="${s.name}">Restore</button> <button class="icon-btn" data-act="snap-del" data-n="${s.name}" aria-label="Delete snapshot">${icon('trash')}</button></td></tr>`)}</table>${list.length > 12 ? h`<p class="hint">${list.length - 12} older snapshots are in the folder.</p>` : ''}` : h`<p class="muted">No snapshots yet.</p>`);
  } catch (e) { setHTML(box, h`<p class="muted">Could not load snapshots.</p>`); }
}
function rerenderResults() { const r = $('#results'); if (r) { r.className = S.sel ? 'selmode' : ''; setHTML(r, resultsHTML(filtered())); } const b = $('#bulk'); if (b) setHTML(b, bulkHTML()); renderSide(); }

/* ---------- dialogs ---------- */
function openDialog({ title, body, footer, narrow, onClose }) {
  const d = document.createElement('dialog'); if (narrow) d.className = 'narrow';
  setHTML(d, h`<div class="dlg"><div class="dlg-h"><h3>${title}</h3><button class="icon-btn" data-act="dlg-close" aria-label="Close">${icon('x')}</button></div><div class="dlg-b"></div>${footer ? h`<div class="dlg-f"></div>` : ''}</div>`);
  document.body.append(d);
  const api = { el: d, body: $('.dlg-b', d), foot: $('.dlg-f', d), close: () => d.close(), title: t => { $('h3', d).textContent = t; } };
  if (body) setHTML(api.body, body); if (footer && api.foot) setHTML(api.foot, footer);
  d.addEventListener('close', () => { d.remove(); onClose && onClose(); });
  d.showModal(); return api;
}
const confirmBox = (title, msg, okLabel = 'Delete', danger = true) => new Promise(res => {
  let done = false; const fin = v => { if (!done) { done = true; res(v); } };
  const d = openDialog({ title, narrow: true, onClose: () => fin(false),
    body: h`<p style="margin:0">${msg}</p>`, footer: h`<button class="btn" data-x="no">Cancel</button><button class="btn ${danger ? 'danger' : 'primary'}" data-x="yes">${okLabel}</button>` });
  d.el.addEventListener('click', e => { const b = e.target.closest('[data-x]'); if (b) { fin(b.dataset.x === 'yes'); d.close(); } });
});
const promptBox = (title, label, value = '') => new Promise(res => {
  let done = false; const fin = v => { if (!done) { done = true; res(v); } };
  const d = openDialog({ title, narrow: true, onClose: () => fin(null),
    body: h`<label class="f"><span>${label}</span><input type="text" id="pv" value="${value}"></label>`, footer: h`<button class="btn" data-x="no">Cancel</button><button class="btn primary" data-x="yes">OK</button>` });
  const inp = $('#pv', d.el); inp.focus(); inp.select();
  const go = () => { fin(inp.value.trim() || null); d.close(); };
  inp.addEventListener('keydown', e => { if (e.key === 'Enter') go(); });
  d.el.addEventListener('click', e => { const b = e.target.closest('[data-x]'); if (b) { if (b.dataset.x === 'yes') go(); else d.close(); } });
});

/* file / folder picker (server-side, so we get real paths) */
function pickPath({ mode, exts, start, title }) {
  return new Promise(res => {
    let chosen = null, cur = '';
    const d = openDialog({ title: title || (mode === 'dir' ? 'Choose a folder' : 'Choose a file'), onClose: () => res(chosen),
      body: h`<div class="row" style="margin-bottom:10px"><input type="text" id="fp" placeholder="Type or paste a path and press Enter"><button class="btn" data-x="go">Go</button></div><div class="fsl"><div class="places" id="pl"></div><div class="fsb" id="fl"></div></div>`,
      footer: mode === 'dir' ? h`<button class="btn" data-x="cancel">Cancel</button><button class="btn primary" data-x="use">Use this folder</button>` : h`<button class="btn" data-x="cancel">Cancel</button>` });
    const nav = guard(async p => {
      const q = new URLSearchParams({ path: p || '' }); if (mode === 'dir') q.set('dirsOnly', '1'); else if (exts) q.set('exts', exts);
      const r = await api('GET', '/api/fs?' + q);
      cur = r.path; $('#fp', d.el).value = r.path;
      setHTML($('#pl', d.el), h`${r.places.map(pl => h`<button data-p="${pl.path}">${pl.name}</button>`)}`);
      setHTML($('#fl', d.el), h`${r.path ? h`<button data-p="${r.parent}">${icon('up')}<span>..</span></button>` : ''}
        ${r.entries.map(e => h`<button data-p="${e.path}" data-dir="${e.isDir ? 1 : 0}">${icon(e.isDir ? 'folder' : 'file')}<span>${e.name}</span>${e.isDir ? '' : h`<span class="sz">${(e.size / 1048576).toFixed(1)} MB</span>`}</button>`)}
        ${r.entries.length ? '' : h`<p class="muted" style="padding:14px;margin:0">${mode === 'dir' ? 'No sub-folders here.' : 'No matching files here.'}</p>`}`);
    });
    d.el.addEventListener('click', e => {
      const b = e.target.closest('button'); if (!b) return;
      if (b.dataset.x === 'cancel') return d.close();
      if (b.dataset.x === 'use') { chosen = cur; return d.close(); }
      if (b.dataset.x === 'go') return nav($('#fp', d.el).value.trim());
      if (b.dataset.p !== undefined) {
        if (b.dataset.dir === '0') { chosen = b.dataset.p; d.close(); } else nav(b.dataset.p);
      }
    });
    $('#fp', d.el).addEventListener('keydown', e => { if (e.key === 'Enter') { const v = e.target.value.trim(); nav(v); } });
    nav(start && /^([a-zA-Z]:|\/)/.test(start) ? start.replace(/[\\/][^\\/]*$/, '') || start : '');
  });
}

/* ---------- game dialog ---------- */
function payload(g, over = {}) {
  const { id, title, originalTitle, developer, releaseDate, description, exePath, launchArgs, useWrapper, locale, nsfw, savePath, status, rating, tags, categories, notes, progress, vndbId, favorite } = g;
  return { id, title, originalTitle, developer, releaseDate, description, exePath, launchArgs, useWrapper, locale, nsfw, savePath, status, rating, tags, categories, notes, progress, vndbId, favorite, ...over };
}
async function bulkNsfw(v) {
  const ids = [...S.sel]; for (const id of ids) await api('PUT', `/api/games/${id}`, payload(game(id), { nsfw: v }));
  await load(); toast(`Cover blur ${v ? 'on' : 'off'} for ${ids.length} game${ids.length === 1 ? '' : 's'}`, 'ok');
}
async function bulkLocale(v) {
  const ids = [...S.sel]; for (const id of ids) await api('PUT', `/api/games/${id}`, payload(game(id), { locale: v }));
  await load(); toast(`Japanese locale ${v ? 'on' : 'off'} for ${ids.length} game${ids.length === 1 ? '' : 's'}`, 'ok');
}
const patchGame = guard(async (id, over) => { await api('PUT', `/api/games/${id}`, payload(game(id), over)); await load(); });

function openGame(id, tab = 'overview') {
  const g0 = id ? game(id) : null;
  let tabNow = g0 ? tab : 'edit';
  const dlg = openDialog({ title: g0 ? g0.title : 'Add game', body: raw('') });
  dlg.el.dataset.gid = id || '';
  if (id) dlg.el.classList.add('gamepage');
  dlg.rerender = () => {
    const g = id ? game(id) : null;
    if (id && !g) return dlg.close();
    dlg.el.style.setProperty('--gcover', g && g.cover ? `url("/covers/${g.cover}")` : 'none');
    dlg.el.classList.toggle('nsfwpage', !!(g && g.nsfw));
    const tabs = g ? h`<div class="tabs">${[['overview', 'Overview'], ['sessions', 'Playtime'], ['guides', 'Guides' + (g.guides && g.guides.length ? ` (${g.guides.length})` : '')], ['cg', 'CG'], ['anime', 'Anime'], ['saves', 'Saves'], ['edit', 'Edit']].map(([k, l]) => h`<button class="tab ${tabNow === k ? 'on' : ''}" data-x="tab" data-t="${k}">${l}</button>`)}</div>` : '';
    dlg.title(g ? g.title : 'Add game');
    let old = $('.tabs', dlg.el); if (old) old.remove();
    if (tabs.s) dlg.el.querySelector('.dlg-h').insertAdjacentHTML('afterend', tabs.s);
    if (tabNow === 'overview') viewOverview(dlg, g);
    else if (tabNow === 'sessions') viewSessions(dlg, g);
    else if (tabNow === 'guides') viewGuides(dlg, g);
    else if (tabNow === 'cg') viewCG(dlg, g);
    else if (tabNow === 'anime') viewAnime(dlg, g);
    else if (tabNow === 'saves') viewSaves(dlg, g);
    else viewEdit(dlg, g);
  };
  dlg.el.addEventListener('click', e => { const b = e.target.closest('[data-x="tab"]'); if (b) { tabNow = b.dataset.t; dlg.rerender(); } });
  dlg.go = t => { tabNow = t; dlg.rerender(); };
  dlg.cur = () => tabNow;
  dlg.rerender();
  return dlg;
}

function viewOverview(dlg, g) {
  const cats = g.categories.map(id => (S.lib.categories.find(c => c.id === id) || {}).name).filter(Boolean);
  setHTML(dlg.body, h`<div class="hero"><div class="cv">${cover(g)}</div><div>
    <h2>${g.title}</h2>${g.originalTitle && g.originalTitle !== g.title ? h`<div class="orig">${g.originalTitle}</div>` : ''}
    <div class="meta-line">${g.developer ? h`<span>${g.developer}</span>` : ''}${g.releaseDate ? h`<span>Released ${g.releaseDate}</span>` : ''}${g.vndbId ? h`<a href="https://vndb.org/${g.vndbId}" target="_blank" rel="noopener">VNDB ${g.vndbId}</a>` : ''}</div>
    <div class="row" style="flex-wrap:wrap">${playBtn(g, 'btn primary')}
      <select data-x="status" style="width:auto" aria-label="Status">${Object.entries(STATUS).map(([k, l]) => h`<option value="${k}" ${g.status === k ? 'selected' : ''}>${l}</option>`)}</select>
      <button class="btn ${g.favorite ? '' : 'ghost'}" data-x="fav" style="${g.favorite ? 'color:var(--gold)' : ''}">${icon('star')}${g.favorite ? 'Favorite' : 'Favorite?'}</button></div>
    <div class="stats">${S.running[g.id] ? h`<div class="stat live" data-live="Playing now" data-pause="Paused — game not on top" data-id="${g.id}"><span class="dot pulse" style="background:var(--good)"></span><span class="rt-lbl">Playing now</span><b>${runTime(g.id)}</b></div>` : ''}<div class="stat"><b>${fmtDur(total(g))}</b><span>Time played</span></div><div class="stat"><b>${rel(lastOf(g))}</b><span>Last played</span></div><div class="stat"><b>${g.sessions.length}</b><span>Sessions</span></div></div>
    <label class="f"><span>Progress</span><input type="text" data-x="progress" maxlength="80" value="${g.progress || ''}" placeholder="e.g. Kotori route · chapter 3 — shown on the card"></label>
    <label class="f"><span>My rating: <b>${g.rating ? g.rating.toFixed(1) : 'unrated'}</b> / 10</span><input type="range" min="0" max="10" step="0.5" value="${g.rating}" data-x="rating"></label>
    ${g.description ? h`<div class="desc">${g.description}</div>` : h`<p class="muted">No description yet — open the Edit tab and use “Find on VNDB”.</p>`}
    <div class="tagrow">${g.locale === 'ja' ? h`<span class="tag c" title="Starts with Japanese system locale">🇯🇵 Japanese locale</span>` : ''}${g.tags.map(t => h`<span class="tag">${t}</span>`)}${cats.map(t => h`<span class="tag c">▣ ${t}</span>`)}</div>
    <label class="f" style="margin-top:16px"><span>Notes</span><textarea data-x="notes" placeholder="Routes, thoughts, where you left off…">${g.notes}</textarea></label>
  </div></div>`);
  const b = dlg.body;
  b.onchange = guard(async e => {
    const t = e.target.dataset.x;
    if (t === 'status') await patchGame(g.id, { status: e.target.value });
    if (t === 'rating') await patchGame(g.id, { rating: +e.target.value });
    if (t === 'notes') await patchGame(g.id, { notes: e.target.value });
    if (t === 'progress') await patchGame(g.id, { progress: e.target.value.trim() });
  });
  b.onclick = e => { if (e.target.closest('[data-x="fav"]')) patchGame(g.id, { favorite: !g.favorite }); };
}


/* ---------- Anime: episodes of this visual novel's anime adaptation, found on a few streaming sites ---------- */
const ANIME_SITES = [['AniHentai', 'https://anihentai.com']];
async function viewAnime(dlg, g) {
  const body = dlg.body;
  const a = dlg.anime && dlg.anime.for === g.id ? dlg.anime : (dlg.anime = { for: g.id, busy: false, sites: null, err: '', eps: {}, show: !!g.page && (() => { try { return localStorage.getItem('an-show') === '1'; } catch { return false; } })(), bound: dlg.anime && dlg.anime.bound });
  if (a.play) stopPlay();
  const epKey = (site, url) => site + '|' + url;
  const epItem = (it, list, site) => h`<span class="an-epw"><button class="btn sm an-ep ${a.seen && a.seen.has(it.url) ? 'seen' : ''}" data-an="play" data-site="${site}" data-list="${list}" data-url="${it.url}" title="${a.seen && a.seen.has(it.url) ? 'Watched · play again' : 'Play here'}">${a.seen && a.seen.has(it.url) ? '✓' : '▶'} ${it.n ? 'Episode ' + it.n : it.title}${it.n && it.title && !/episode|\bep\b/i.test(it.title) && it.title.length < 70 ? h`<span class="tiny muted"> · ${it.title}</span>` : ''}</button><button class="btn sm" data-cg="link" data-url="${it.url}" title="Open on the site in your browser" aria-label="Open on the site">↗</button></span>`;
  const seriesCard = (s, it) => {
    const e = a.eps[epKey(s.id, it.url)];
    return h`<div class="an-card"><div class="an-th">${it.thumb ? h`<img class="${a.show ? '' : 'nsfw'}" src="${it.thumb}" alt="" loading="lazy" referrerpolicy="no-referrer">` : ''}</div>
      <div class="an-body"><div class="an-ti">${it.title}</div>
        <div class="row" style="gap:6px;flex-wrap:wrap;margin:6px 0"><button class="btn sm" data-an="eps" data-site="${s.id}" data-url="${it.url}">${e && e.list ? 'Refresh episodes' : 'Show episodes'}</button><button class="btn sm" data-cg="link" data-url="${it.url}">Open page</button></div>
        ${e && e.busy ? h`<p class="tiny muted">Loading episodes…</p>` : e && e.err ? h`<p class="tiny muted">${e.err}</p>` : e && e.list ? (e.list.length ? h`<div class="an-eps">${e.list.map(x => epItem(x, epKey(s.id, it.url), s.id))}</div>` : h`<p class="tiny muted">No episode links found on that page — use “Open page”.</p>`) : ''}</div></div>`;
  };
  const draw = () => {
    if (dlg.cur() !== 'anime' || a.play) return;
    if (g.page) return drawPage();
    const q = encodeURIComponent;
    const total = a.sites ? a.sites.reduce((n, s) => n + s.items.length, 0) : 0;
    const errs = a.sites ? a.sites.filter(s => s.error) : [];
    let main;
    if (a.busy || !a.sites) main = h`<p class="muted cg-st">${a.err || 'Searching AniHentai…'}</p>`;
    else if (!total && errs.length === a.sites.length) main = h`<p class="cg-st">Couldn't reach the anime sites (${errs.map(s => s.name).join(', ')}). Check your connection, or search them yourself below. <button class="btn sm" data-an="search">Try again</button></p>`;
    else if (!total && g.page) main = h`<p class="cg-st"><strong>Nothing found${g.title ? ' for “' + g.title + '”' : ''}.</strong> <button class="btn sm" data-an="search">Search again</button></p>`;
    else if (!total) main = h`<p class="cg-st"><strong>No anime available for this visual novel.</strong>${errs.length ? h`<span class="tiny muted"> (${errs.map(s => s.name).join(', ')} could not be checked.)</span>` : ''} <button class="btn sm" data-an="search">Search again</button></p>`;
    else main = h`${a.sites.filter(s => s.items.length).map(s => {
      const series = s.items.filter(i => !i.episode), eps = s.items.filter(i => i.episode).sort((x, y) => x.n - y.n);
      return h`<section class="an-site"><h4>${s.name}</h4>
        ${series.map(it => seriesCard(s, it))}
        ${eps.length ? h`<div class="an-eps">${eps.map(x => epItem(x, 'site:' + s.id, s.id))}</div>` : ''}</section>`;
    })}${errs.length ? h`<p class="tiny muted">${errs.map(s => s.name).join(', ')} could not be checked.</p>` : ''}`;
    setHTML(body, h`<p class="muted" style="margin-top:0">${g.page ? h`${g.title ? 'Results for “' + g.title + '” on AniHentai. ' : 'The latest on AniHentai. '}Episodes play right here (↗ opens the site instead). Adult thumbnails stay blurred until you press Unblur.` : h`Anime adaptations of “${g.title}”, looked up by title on AniHentai. Episodes play right here (↗ opens the site instead). Adult thumbnails stay blurred until you press Unblur.`}</p>
      <div class="chips" style="margin-bottom:10px"><button class="chip ghost" data-an="reveal">${a.show ? 'Blur images' : 'Unblur images'}</button><button class="chip ghost" data-an="search">Search again</button></div>
      ${main}
      ${a.sites ? h`<details class="cg-more"><summary class="tiny muted">What was checked</summary>${a.sites.map(s => h`<p class="tiny muted" style="margin:6px 0"><strong>${s.name}</strong>: ${s.error ? s.error : s.items.length + ' match(es)'}${(s.debug || []).map(d => h`<br>${d}`)}</p>`)}</details>` : ''}
      ${g.page ? '' : h`<details class="cg-more"><summary class="tiny muted">Search the sites yourself</summary><div class="row" style="gap:8px;flex-wrap:wrap">${ANIME_SITES.map(([l, u]) => h`<button class="btn sm" data-cg="link" data-url="${u}/?s=${q(g.title)}">${l}</button>`)}</div></details>`}`);
  };
  const loadEps = async (site, url) => {
    const k = epKey(site, url); a.eps[k] = { busy: true };
    draw();
    try { const d = await api('GET', `/api/anime/episodes?site=${site}&url=${encodeURIComponent(url)}`); a.eps[k] = { list: d.episodes }; }
    catch (e) { a.eps[k] = { err: e.message }; }
    draw();
  };
  const search = async () => {
    if (g.page) return pageLoad(true);
    if (a.busy) return;
    a.busy = true; a.err = ''; a.sites = null; a.eps = {}; draw();
    try {
      const d = g.page && !g.title ? await api('GET', '/api/anime/browse') : await api('GET', `/api/anime/search?q=${encodeURIComponent(g.title)}&alt=${encodeURIComponent(g.originalTitle && g.originalTitle !== g.title ? g.originalTitle : '')}`);
      a.sites = d.sites;
    } catch (e) { a.err = e.message; }
    a.busy = false; draw();
    // A handful of series: list their episodes right away.
    const series = a.sites ? a.sites.flatMap(s => s.items.filter(i => !i.episode).map(i => [s.id, i.url])) : [];
    if (series.length && series.length <= 4) series.forEach(([s, u]) => loadEps(s, u));
  };

  // ----- player -----
  function stopPlay() {
    if (dlg.anime && dlg.anime.hls) { try { dlg.anime.hls.destroy(); } catch { } dlg.anime.hls = null; }
    if (dlg.anime) dlg.anime.play = null;
  }
  const listFor = key => key === 'grid' ? (a.items || []).filter(i => i.episode) : key.startsWith('site:') ? (a.sites.find(s => s.id === key.slice(5)) || { items: [] }).items.filter(i => i.episode).sort((x, y) => x.n - y.n) : ((a.eps[key] || {}).list || []);
  const loadHls = () => window.Hls ? Promise.resolve() : new Promise((ok, no) => { const s = document.createElement('script'); s.src = 'hls.min.js'; s.onload = ok; s.onerror = () => no(new Error('player component missing')); document.head.appendChild(s); });
  const msg = t => { const m = $('#anmsg', body); if (m) { m.textContent = t; m.hidden = !t; } };
  const attach = async () => {
    const P = a.play; if (!P || P.mode !== 'video') return;
    const v = $('#anv', body), s = P.sources[P.si]; if (!v || !s) return;
    if (a.hls) { try { a.hls.destroy(); } catch { } a.hls = null; }
    const fail = why => {
      if (a.play !== P) return;
      if (P.si + 1 < P.sources.length) { P.si++; drawPlayer(); return; }
      if (P.embeds.length) { P.mode = 'embed'; P.ei = 0; drawPlayer(); return; }
      msg('This episode could not be played here (' + why + '). Use “Open on the site”.');
    };
    msg('');
    v.onerror = () => fail('the video host refused or the format is not supported');
    v.onended = () => { if (P.i + 1 < P.list.length) playAt(P.list, P.i + 1, P.site); };
    try {
      if (s.type === 'hls') {
        await loadHls();
        if (a.play !== P) return;
        if (window.Hls && Hls.isSupported()) {
          const hls = a.hls = new Hls({ maxBufferLength: 40 });
          hls.on(Hls.Events.ERROR, (_, d) => { if (d.fatal) fail('stream error'); });
          hls.loadSource(s.url); hls.attachMedia(v);
        } else if (v.canPlayType('application/vnd.apple.mpegurl')) v.src = s.url;
        else return fail('HLS is not supported');
      } else v.src = s.url;
      v.play().catch(() => { });
    } catch (e) { fail(e.message); }
  };
  const drawPlayer = () => {
    const P = a.play; if (!P) return;
    const it = P.list[P.i] || {};
    const title = it.n ? `Episode ${it.n}` : (it.title || 'Episode');
    let stage;
    if (P.status === 'loading') stage = h`<p class="muted an-wait">Finding the video…</p>`;
    else if (P.status === 'err') stage = h`<p class="muted an-wait">${P.err}</p>`;
    else if (P.mode === 'embed') stage = h`<iframe src="${P.embeds[P.ei]}" allowfullscreen allow="fullscreen; autoplay; encrypted-media" referrerpolicy="origin"></iframe>`;
    else if (P.mode === 'external') stage = h`<div class="an-wait" style="text-align:center"><p style="margin:0 0 6px"><strong>${SITES[P.site] || 'This site'} plays its videos on its own page.</strong></p><p class="muted" style="margin:0 0 12px">It doesn't allow its player to be shown inside the page, so the episode opened in a ShioriBox browser window.</p><button class="btn primary" data-an="browse" data-url="${P.external}">Open it again</button> <button class="btn" data-cg="link" data-url="${P.external}">Open in my browser</button></div>`;
    else if (P.mode === 'video') stage = h`<video id="anv" controls autoplay playsinline></video>`;
    else stage = h`<p class="muted an-wait">No playable video was found on this page. Use “Open on the site”.</p>`;
    setHTML(body, h`<div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="btn sm" data-an="stop">‹ Episodes</button><strong class="an-pt">${g.page ? title : (P.series || g.title) + ' — ' + title}</strong></div>
      <div class="an-player">${stage}</div><p class="tiny muted" id="anmsg" hidden></p>
      <div class="row" style="gap:6px;flex-wrap:wrap;margin-top:8px"><button class="btn sm" data-an="prev" ${P.i > 0 ? '' : 'disabled'}>‹ Previous</button><button class="btn sm" data-an="next" ${P.i + 1 < P.list.length ? '' : 'disabled'}>Next ›</button>
        ${P.mode === 'video' && P.sources.length > 1 ? P.sources.map((s, k) => h`<button class="btn sm ${k === P.si ? 'primary' : ''}" data-an="src" data-k="${k}" title="${s.label}">Source ${k + 1}${s.label ? ' · ' + s.label.split(' · ')[0] : ''}</button>`) : ''}
        ${P.mode === 'video' && P.embeds.length ? h`<button class="btn sm" data-an="embed">Site's player</button>` : ''}
        ${P.mode === 'embed' && P.sources.length ? h`<button class="btn sm" data-an="video">Built-in player</button>` : ''}
        <button class="btn sm" data-cg="link" data-url="${it.url}">Open on the site</button></div>`);
    if (P.mode === 'video') attach();
  };
  const histSet = items => { a.hist = items || []; a.seen = new Set(a.hist.flatMap(e => e.seen || [])); };
  const histLoad = () => api('GET', '/api/anime/history').then(d => { histSet(d.items); if (!a.play) draw(); }).catch(() => { });
  const histRecord = (it, site) => {
    const o = a.open, inf = o && (a.info || {})[o.url], d = inf && inf.data;
    const ent = o ? { site: o.site, url: o.url, title: (d && d.title) || o.title, thumb: (d && d.poster) || o.thumb || '' } : { site: it.site || site, url: it.url, title: it.title || 'Episode', thumb: it.thumb || '' };
    if (!ent.title) return;
    api('POST', '/api/anime/history', { ...ent, epUrl: it.url, epTitle: it.title || '', epN: it.n || 0 }).then(r => histSet(r.items)).catch(() => { });
  };
  const playAt = async (list, i, site) => {
    stopPlay();
    const it = list[i]; if (!it) return;
    histRecord(it, site);
    const P = a.play = { list, i, site, series: g.page ? '' : g.title, status: 'loading', mode: '', sources: [], embeds: [], si: 0, ei: 0, err: '' };
    drawPlayer();
    try {
      const d = await api('GET', `/api/anime/source?site=${it.site || site}&url=${encodeURIComponent(it.url)}`);
      if (a.play !== P) return;
      P.sources = d.sources; P.embeds = d.embeds; P.status = 'ready';
      P.mode = P.sources.length ? 'video' : P.embeds.length ? 'embed' : d.external ? 'external' : '';
      if (d.external) { P.external = d.external; api('POST', '/api/browse', { url: d.external }).catch(err => toast(err.message, 'err')); }
    } catch (e) { if (a.play !== P) return; P.status = 'err'; P.err = e.message; }
    drawPlayer();
  };
  // ----- Anime page: grid of cards from several sources, series detail, load more, random -----
  const SITES = { anihentai: 'AniHentai', akih: 'Aki-H', hstream: 'HStream' };
  const SITE_BASE = { anihentai: 'https://anihentai.com', akih: 'https://aki-h.com' };
  const srcList = () => g.src === 'all' || !g.src ? Object.keys(SITES) : [g.src];
  const pageUrl = (site, reset) => {
    if (!reset) return `/api/anime/list?site=${site}&kind=url&v=${encodeURIComponent(a.next[site])}`;
    if (site === 'hstream') {
      const q = g.title ? `&kind=search&q=${encodeURIComponent(g.title)}` : '&kind=latest';
      const tag = (g.hTag || '').toLowerCase().trim().replace(/[\s_]+/g, '-');
      return `/api/anime/list?site=hstream${q}${tag ? '&tags=' + encodeURIComponent(tag) : ''}`;
    }
    const S = `site=${site}`;
    if (g.startUrl && g.src === site) return `/api/anime/list?${S}&kind=url&v=${encodeURIComponent(g.startUrl)}`;
    if (g.title) return `/api/anime/list?${S}&kind=search&q=${encodeURIComponent(g.title)}`;
    if (site === 'anihentai') {
      const fil = Object.entries(g.fil || {}).filter(([, v]) => v);
      if (fil.length) return '/api/anime/list?kind=browse' + fil.map(([k, v]) => '&f=' + encodeURIComponent(k + ':' + v)).join('');
    } else if (g.hTag) return `/api/anime/list?${S}&kind=tag&tags=${encodeURIComponent(g.hTag.toLowerCase().trim().replace(/[\s_]+/g, '-'))}`;
    return `/api/anime/list?${S}&kind=latest`;
  };
  const hasNext = () => Object.values(a.next || {}).some(Boolean);
  async function pageLoad(reset) {
    if (a.busy || (!reset && !hasNext())) return;
    if (reset) { a.showHist = false; a.items = []; a.next = {}; a.open = null; a.err = ''; a.dbg = ''; a.eps = {}; }
    const sites = reset ? srcList() : Object.keys(a.next).filter(k => a.next[k]);
    a.busy = true; draw();
    const res = await Promise.all(sites.map(async site => {
      try { return { site, d: await api('GET', pageUrl(site, reset)) }; } catch (e) { return { site, err: e.message }; }
    }));
    const have = new Set((a.items || []).map(i => i.url)), lists = [];
    const dbg = [];
    for (const r of res) {
      if (r.err) { dbg.push(`${SITES[r.site]}: ${r.err}`); a.next[r.site] = ''; continue; }
      a.next[r.site] = r.d.next || '';
      dbg.push(`${SITES[r.site]}: ${r.d.url} → ${r.d.links} links, ${r.d.items.length} shown`);
      const fresh = r.d.items.filter(i => { i.site = i.site || r.site; if (have.has(i.url)) return false; have.add(i.url); return true; });
      if (!reset && !fresh.length) a.next[r.site] = '';
      lists.push(fresh);
    }
    for (let i = 0; lists.some(l => i < l.length); i++) lists.forEach(l => { if (i < l.length) a.items.push(l[i]); });
    a.dbg = dbg.join('\n');
    a.err = !lists.length && res.length ? (res.find(r => r.err) || {}).err || '' : '';
    a.busy = false; draw();
  }
  function pageOpen(i) {
    const it = (a.items || [])[i]; if (!it) return;
    if (it.episode) { const list = listFor('grid'); return playAt(list, Math.max(0, list.findIndex(x => x.url === it.url)), it.site || 'anihentai'); }
    a.open = { site: it.site || 'anihentai', url: it.url, title: it.title, thumb: it.thumb };
    loadInfo(it.url, a.open.site);
  }
  async function loadInfo(url, site) {
    a.info = a.info || {};
    if (a.info[url] && a.info[url].data) return draw();
    a.info[url] = { busy: true }; draw();
    try {
      const d = await api('GET', `/api/anime/info?site=${site || 'anihentai'}&url=${encodeURIComponent(url)}`);
      a.info[url] = { data: d }; a.eps[epKey(site || 'anihentai', url)] = { list: d.episodes };
      const c = (a.items || []).find(i => i.url === url); if (c && !c.thumb && d.poster) c.thumb = d.poster;
    } catch (e) { a.info[url] = { err: e.message }; }
    draw();
  }
  // Cards the listing gave no picture for: fetch the cover from the series page, a few at a time.
  let thumbBusy = 0;
  function fillThumbs() {
    for (const it of (a.items || [])) {
      if (thumbBusy >= 3) return;
      if (it.thumb || it.episode || it.tried) continue;
      it.tried = true; thumbBusy++;
      const site = it.site || 'anihentai';
      api('GET', `/api/anime/info?site=${site}&url=${encodeURIComponent(it.url)}`).then(d => {
        a.info = a.info || {}; a.info[it.url] = { data: d }; a.eps[epKey(site, it.url)] = { list: d.episodes };
        if (d.poster && !it.thumb) { it.thumb = d.poster; const i = a.items.indexOf(it), th = body.querySelector(`.an-gc[data-i="${i}"] .th`); if (th && !th.querySelector('img')) { const im = document.createElement('img'); im.className = a.show ? '' : 'nsfw'; im.src = d.poster; im.alt = ''; im.loading = 'lazy'; th.prepend(im); const ph = th.querySelector('.ph'); if (ph) ph.remove(); } }
      }).catch(() => { }).finally(() => { thumbBusy--; fillThumbs(); });
    }
  }
  async function pageRandom() {
    if (a.busy) return;
    a.showHist = false; a.busy = true; a.err = ''; draw();
    const order = srcList().sort(() => Math.random() - 0.5);
    let it = null, lastErr = '';
    for (const site of order) {
      try { it = (await api('GET', `/api/anime/random?site=${site}`)).item; it.site = it.site || site; break; } catch (e) { lastErr = e.message; }
    }
    a.busy = false;
    if (!it) { a.err = lastErr || 'No random pick available.'; return draw(); }
    if (it.episode) return playAt([it], 0, it.site);
    a.open = { site: it.site, url: it.url, title: it.title, thumb: it.thumb };
    loadInfo(it.url, it.site);
  }
  function drawPage() {
    const intro = h`<p class="muted" style="margin:0 0 8px">Episodes play right here (↗ opens the site instead). Thumbnails stay blurred until you press Unblur.</p>
      <div class="chips" style="margin-bottom:10px"><button class="chip ghost" data-an="reveal">${a.show ? 'Blur images' : 'Unblur images'}</button></div>`;
    if (a.open) {
      const o = a.open, inf = (a.info || {})[o.url] || {}, d = inf.data, e = a.eps[epKey(o.site, o.url)];
      const eps = d ? (e && e.list && e.list.length ? h`<div class="an-eps">${e.list.map(x => epItem(x, epKey(o.site, o.url), o.site))}</div>` : h`<p class="tiny muted">No episode links found on that page — use “Open page”.</p>`) : '';
      const poster = d && d.poster || o.thumb;
      const info = inf.busy || (!d && !inf.err) ? h`<p class="muted">Loading details…</p>` : inf.err ? h`<p class="tiny muted">Couldn't load details: ${inf.err} <button class="btn sm" data-an="retry">Try again</button></p>` : h`
        <div class="an-det"><div class="an-poster">${poster ? h`<img class="${a.show ? '' : 'nsfw'}" src="${poster}" alt="" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(o.title)}">${[...o.title.trim()][0] || '?'}</div>`}<span class="an-site-tag">${SITES[o.site] || o.site}</span></div>
          <div class="an-meta"><h2>${d.title || o.title}</h2>
            ${d.facts.length || d.year ? h`<div class="an-facts">${d.year && !d.facts.some(f => /year|release|aired/i.test(f.k)) ? h`<div><span>Year</span>${d.year}</div>` : ''}${d.facts.map(f => h`<div><span>${f.k}</span>${f.v}</div>`)}</div>` : ''}
            ${d.studios.length ? h`<div class="chips" style="margin:8px 0"><span class="tiny muted" style="align-self:center">Studio</span>${d.studios.map(x => h`<button class="chip" data-an="tag" data-kind="studio" data-site="${o.site}" data-path="${x.v}" data-label="${x.l}">${x.l}</button>`)}</div>` : ''}
            ${d.genres.length ? h`<div class="chips" style="margin:8px 0"><span class="tiny muted" style="align-self:center">Tags</span>${d.genres.map(x => h`<button class="chip" data-an="tag" data-kind="tag" data-site="${o.site}" data-path="${x.v}" data-label="${x.l}">${x.l}</button>`)}</div>` : ''}
            ${d.synopsis ? h`<p class="an-syn">${d.synopsis}</p>` : h`<p class="tiny muted">The site gives no description for this one.</p>`}</div></div>`;
      return setHTML(body, h`<div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="btn sm" data-an="back">‹ Back</button><button class="btn sm" data-cg="link" data-url="${o.url}">Open page</button></div>
        <div style="margin-top:12px">${info}</div>
        ${d ? h`<h3 style="margin:16px 0 6px">Episodes${e && e.list ? ' (' + e.list.length + ')' : ''}</h3>${eps}` : ''}`);
    }
    if (a.showHist) {
      const L = a.hist || [], when = t => { const m = Math.max(0, Math.round((Date.now() / 1000 - t) / 60)); return m < 1 ? 'just now' : m < 60 ? m + ' min ago' : m < 1440 ? Math.round(m / 60) + ' h ago' : m < 43200 ? Math.round(m / 1440) + ' d ago' : new Date(t * 1000).toLocaleDateString(); };
      return setHTML(body, h`<div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="btn sm" data-an="hback">‹ Back</button><h3 style="margin:0">Watch history</h3>${L.length ? h`<button class="btn sm" data-an="hclear" style="margin-left:auto">Clear all</button>` : ''}</div>
        ${L.length ? h`<div class="an-hist">${L.map((e, i) => h`<div class="an-hi"><button class="an-hth" data-an="hopen" data-i="${i}" title="Open details">${e.thumb ? h`<img class="${a.show ? '' : 'nsfw'}" src="${e.thumb}" alt="" loading="lazy" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(e.title)}">${[...e.title.trim()][0] || '?'}</div>`}<span class="an-site-tag">${SITES[e.site] || e.site}</span></button>
          <div class="an-hb"><div class="an-ti">${e.title}</div><div class="tiny muted">Last watched: ${e.epN ? 'Episode ' + e.epN : (e.epTitle || 'an episode')} · ${when(e.at)}</div><div class="tiny muted">${(e.seen || []).length} episode${(e.seen || []).length === 1 ? '' : 's'} watched</div>
            <div class="row" style="gap:6px;flex-wrap:wrap;margin-top:8px"><button class="btn sm primary" data-an="hcont" data-i="${i}">▶ Continue</button><button class="btn sm" data-an="hopen" data-i="${i}">Episodes</button><button class="btn sm" data-an="hdel" data-i="${i}" title="Remove from history">Remove</button></div></div></div>`)}</div>` : h`<p class="muted cg-st">Nothing here yet. Anime you play will show up here, with the episodes you've watched.</p>`}`);
    }
    const items = a.items || [];
    const cards = items.map((it, i) => h`<button class="an-gc" data-an="open" data-i="${i}" title="${it.title}"><div class="th">${it.thumb ? h`<img class="${a.show ? '' : 'nsfw'}" src="${it.thumb}" alt="" loading="lazy" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(it.title)}">${[...it.title.trim()][0] || '?'}</div>`}${it.episode ? h`<span class="an-badge">${it.n ? 'EP ' + it.n : 'Episode'}</span>` : ''}<span class="an-site-tag">${SITES[it.site] || ''}</span></div><div class="t">${it.title}</div></button>`);
    const status = a.busy && !items.length ? h`<p class="muted cg-st">Loading…</p>` : a.err ? h`<p class="cg-st">${a.err} <button class="btn sm" data-an="search">Try again</button></p>` : !items.length ? h`<p class="cg-st"><strong>Nothing found.</strong> <button class="btn sm" data-an="search">Try again</button></p>` : '';
    setHTML(body, h`${intro}${g.label ? h`<p class="tiny muted">Showing: <strong>${g.label}</strong></p>` : ''}${status}<div class="an-grid">${cards}</div>
      ${items.length && hasNext() ? h`<div style="text-align:center;margin:14px 0"><button class="btn" data-an="more" ${a.busy ? 'disabled' : ''}>${a.busy ? 'Loading…' : 'Load more'}</button></div>` : ''}
      ${a.dbg ? h`<details class="cg-more"><summary class="tiny muted">What was loaded</summary>${a.dbg.split('\n').map(l => h`<p class="tiny muted" style="margin:4px 0">${l}</p>`)}</details>` : ''}`);
    fillThumbs();
  }
  if (!a.bound) {
    a.bound = true;
    dlg.el.addEventListener('click', e => {
      const b = e.target.closest('[data-an]'); if (!b || dlg.cur() !== 'anime') return;
      const k = b.dataset.an, A = dlg.anime;
      if (k === 'reveal') { A.show = !A.show; try { localStorage.setItem('an-show', A.show ? '1' : '0'); } catch { } return dlg.animeDraw(); }
      if (k === 'browse') return api('POST', '/api/browse', { url: b.dataset.url }).catch(err => toast(err.message, 'err'));
      if (k === 'open') return dlg.animeOpen(+b.dataset.i);
      if (k[0] === 'h' && ['hback', 'hclear', 'hopen', 'hcont', 'hdel'].includes(k)) return dlg.animeHist(k.slice(1), +b.dataset.i);
      if (k === 'back') { A.open = null; return dlg.animeDraw(); }
      if (k === 'retry') { const u = A.open && A.open.url; if (u) { delete A.info[u]; return dlg.animeInfo(u, A.open.site); } return; }
      if (k === 'tag') return dlg.animeTag(b.dataset.site, b.dataset.kind, b.dataset.path, b.dataset.label);
      if (k === 'more') return dlg.animeMore();
      if (k === 'search') return dlg.animeSearch();
      if (k === 'eps') return dlg.animeEps(b.dataset.site, b.dataset.url);
      if (k === 'play') { const list = dlg.animeList(b.dataset.list), i = list.findIndex(x => x.url === b.dataset.url); return dlg.animePlay(list, Math.max(0, i), b.dataset.site); }
      const P = A.play; if (!P) return;
      if (k === 'stop') { dlg.animeStop(); return dlg.animeDraw(); }
      if (k === 'prev') return dlg.animePlay(P.list, P.i - 1, P.site);
      if (k === 'next') return dlg.animePlay(P.list, P.i + 1, P.site);
      if (k === 'src') { P.si = +b.dataset.k; return dlg.animeRedraw(); }
      if (k === 'embed') { P.mode = 'embed'; P.ei = 0; return dlg.animeRedraw(); }
      if (k === 'video') { P.mode = 'video'; P.si = 0; return dlg.animeRedraw(); }
    });
  }
  dlg.animeHist = async (k, i) => {
    const e = (a.hist || [])[i];
    if (k === 'show') { a.showHist = true; a.open = null; histLoad(); return draw(); }
    if (k === 'back') { a.showHist = false; return draw(); }
    if (k === 'clear') { if (!confirm('Clear your whole anime watch history?')) return; try { histSet((await api('DELETE', '/api/anime/history')).items); } catch (err) { toast(err.message, 'err'); } return draw(); }
    if (!e) return;
    if (k === 'del') { try { histSet((await api('DELETE', `/api/anime/history?site=${e.site}&url=${encodeURIComponent(e.url)}`)).items); } catch (err) { toast(err.message, 'err'); } return draw(); }
    const isEp = e.url === e.epUrl;
    if (k === 'cont' && isEp) { a.showHist = false; return playAt([{ url: e.epUrl, title: e.epTitle || e.title, n: e.epN, site: e.site }], 0, e.site); }
    a.showHist = false; a.open = { site: e.site, url: e.url, title: e.title, thumb: e.thumb };
    await loadInfo(e.url, e.site);
    if (k === 'cont') {
      const L = ((a.eps[epKey(e.site, e.url)] || {}).list) || [], j = L.findIndex(x => x.url === e.epUrl);
      if (j >= 0) playAt(L, j, e.site);
    }
  };
  dlg.animeDraw = draw; dlg.animeSearch = search; dlg.animeEps = loadEps; dlg.animeList = listFor; dlg.animePlay = playAt; dlg.animeStop = stopPlay; dlg.animeRedraw = drawPlayer; dlg.animeOpen = pageOpen; dlg.animeMore = () => pageLoad(false); dlg.animeRandom = pageRandom; dlg.animeInfo = loadInfo; dlg.animeTag = (site, kind, path, label) => { g.title = ''; g.fil = {}; g.startUrl = ''; g.hTag = g.hBrand = ''; g.label = label; g.src = site; if (site === 'hstream') { if (kind === 'studio') g.title = label; else g.hTag = path; } else g.startUrl = SITE_BASE[site] + path; if (dlg.resetBar) dlg.resetBar(); pageLoad(true); };
  if (g.page) { histLoad(); if (!a.items && !a.busy) return pageLoad(true); return draw(); }
  if (!a.sites && !a.busy) return search();
  draw();
}

/* ---------- Anime page (sidebar): search or browse AniHentai and play with the same player ---------- */
const AN = { el: null, dlg: null, g: { id: '__page', title: '', page: true, src: (() => { try { const v = localStorage.getItem('an-src'); return v === 'anihentai' || v === 'akih' || v === 'hstream' ? v : 'all'; } catch { return 'all'; } })() } };
function animeLeave() {
  const A = AN.dlg && AN.dlg.anime;
  if (A && A.play) { AN.dlg.animeStop(); AN.dlg.animeDraw(); }
}
function animePage(m) {
  if (AN.el && m.firstChild === AN.el) return; // already showing: keep a playing video untouched
  if (!AN.el) {
    const el = AN.el = document.createElement('div');
    el.innerHTML = '<form class="an-bar" id="anform"><input id="anq" type="search" placeholder="Search anime by title…" autocomplete="off"><button class="btn primary" type="submit">Search</button><button class="btn" type="button" id="anlatest">Latest</button><select id="ansrc" aria-label="Source" style="width:auto"><option value="all">All sites</option><option value="anihentai">AniHentai</option><option value="akih">Aki-H</option><option value="hstream">HStream</option></select><button class="btn" type="button" id="anhist" title="Anime you have watched">🕘 History</button><button class="btn" type="button" id="anrand" title="Open a random anime">🎲 Random</button></form><div class="an-bar" id="anfilters"></div><div id="anbody"></div>';
    AN.dlg = { el, body: el.querySelector('#anbody'), cur: () => S.view === 'anime' ? 'anime' : '' };
    const go = () => { AN.g.startUrl = ''; AN.g.label = ''; AN.g.hTag = AN.g.hBrand = ''; AN.dlg.animeStop(); AN.dlg.animeSearch(); };
    const srcSel = el.querySelector('#ansrc'); srcSel.value = AN.g.src;
    srcSel.addEventListener('change', () => { AN.g.src = srcSel.value; try { localStorage.setItem('an-src', AN.g.src); } catch { } go(); });
    AN.dlg.resetBar = () => { el.querySelector('#anq').value = ''; sels().forEach(s => s.value = ''); srcSel.value = AN.g.src; };
    const hf = () => { AN.g.hTag = AN.g.hBrand = ''; sels().forEach(s => { const t = s.value ? s.selectedOptions[0].text : ''; if (/genre|tag/i.test(s.name)) AN.g.hTag = t; if (/studio|brand/i.test(s.name)) AN.g.hBrand = t; }); };
    const sels = () => [...el.querySelectorAll('#anfilters select')];
    el.querySelector('#anform').addEventListener('submit', e => { e.preventDefault(); AN.g.title = el.querySelector('#anq').value.trim(); if (AN.g.title) { AN.g.fil = {}; sels().forEach(s => s.value = ''); } go(); });
    el.querySelector('#anfilters').addEventListener('change', () => { AN.g.fil = {}; sels().forEach(s => { if (s.value) AN.g.fil[s.name] = s.value; }); AN.g.title = ''; el.querySelector('#anq').value = ''; hf(); AN.g.startUrl = ''; AN.g.label = ''; AN.dlg.animeStop(); AN.dlg.animeSearch(); });
    el.querySelector('#anlatest').addEventListener('click', () => { AN.g.title = ''; AN.g.fil = {}; el.querySelector('#anq').value = ''; sels().forEach(s => s.value = ''); go(); });
    el.querySelector('#anhist').addEventListener('click', () => { AN.dlg.animeStop(); AN.dlg.animeHist('show'); });
    el.querySelector('#anrand').addEventListener('click', () => { AN.dlg.animeStop(); AN.dlg.animeRandom(); });
    api('GET', '/api/anime/options').then(o => {
      const cap = t => t.charAt(0).toUpperCase() + t.slice(1);
      setHTML(el.querySelector('#anfilters'), h`${(o.fields || []).map(f => h`<select name="${f.name}" aria-label="${cap(f.label)}"><option value="">${f.name === 'sort' || /sort|order/i.test(f.name) ? 'Sort: default' : 'Any ' + f.label}</option>${f.options.map(x => h`<option value="${x.v}">${x.l}</option>`)}</select>`)}`);
    }).catch(() => { });
  }
  m.replaceChildren(AN.el);
  m.scrollTop = 0;
  viewAnime(AN.dlg, AN.g);
}

/* ---------- CG: VNDB screenshots + E-Hentai game-CG galleries ---------- */
const cgLinks = (title, orig) => {
  const q = encodeURIComponent, out = [];
  out.push(['E-Hentai', 'https://e-hentai.org/?f_cats=1007&f_search=' + q(title)]);
  out.push(['Hitomi.la', 'https://hitomi.la/search.html?' + q(title)]);
  if (orig && orig !== title) out.push(['Hitomi.la (original title)', 'https://hitomi.la/search.html?' + q(orig)]);
  out.push(['HentaiEra', 'https://hentaiera.com/search/?key=' + q(title)]);
  out.push(['HentaiEnvy', 'https://hentaienvy.com/search/?key=' + q(title)]);
  out.push(['Google Images', 'https://www.google.com/search?tbm=isch&q=' + q((orig || title) + ' CG')]);
  out.push(['Pixiv', 'https://www.pixiv.net/en/tags/' + q(title) + '/artworks']);
  return out;
};
const cgLinkRow = (title, orig) => h`<div class="row" style="gap:8px;flex-wrap:wrap">${cgLinks(title, orig).map(([l, u]) => h`<button class="btn sm" data-cg="link" data-url="${u}">${l}</button>`)}</div>`;
S.cgCache = {};
// A picture that fails to load (busy server, dropped connection) is asked for again a few times before it is given up.
document.addEventListener('error', e => {
  const im = e.target;
  if (!(im instanceof HTMLImageElement) || !im.closest('#cgmain')) return;
  const src = im.getAttribute('src') || '';
  if (!src.startsWith('/api/cg/img')) return;
  const n = +(im.dataset.rt || 0);
  if (n >= 3) {
    const box = im.closest('.cg-img');
    if (box) { const p = document.createElement('p'); p.className = 'muted'; p.style.padding = '60px 12px'; p.textContent = 'This picture could not be loaded from the site right now. Try Next / Previous, or open it in your browser.'; im.replaceWith(p); }
    return;
  }
  im.dataset.rt = n + 1;
  setTimeout(() => { if (im.isConnected) im.src = src.replace(/&_r=\d+$/, '') + '&_r=' + (n + 1); }, 700 * (n + 1));
}, true);
const CG_SRC = { vndb: 'VNDB', ehentai: 'E-Hentai', hitomi: 'Hitomi.la', hentaiera: 'HentaiEra', hentaienvy: 'HentaiEnvy' };
const cgPickKey = gid => 'ns.cgpick.' + gid;
const cgPickGet = gid => { try { return JSON.parse(localStorage.getItem(cgPickKey(gid)) || 'null'); } catch { return null; } };
const cgPickSet = (gid, v) => { try { if (v) localStorage.setItem(cgPickKey(gid), JSON.stringify(v)); else localStorage.removeItem(cgPickKey(gid)); } catch { } };

async function viewCG(dlg, g) {
  const body = dlg.body;
  // per-dialog state; kept between redraws and tab switches
  const c = dlg.cg || (dlg.cg = { src: 'vndb', idx: null, show: false, vndb: null, vBusy: false, vErr: '', q: {}, res: {}, gal: null, tok: 0 });
  const isGal = () => c.src !== 'vndb' && c.gal && c.gal.site === c.src;
  const cur = () => c.src === 'vndb' ? (c.vndb || []) : isGal() ? c.gal.imgs : [];
  const nsfw = s => c.src !== 'vndb' || g.nsfw || (s && s.sexual >= 1);
  const blurCls = s => nsfw(s) && !c.show ? 'nsfw' : '';
  const r = () => c.res[c.src] || (c.res[c.src] = { items: [], next: -1, busy: false, err: '', done: false, scanned: 0, total: 0, q: null });

  const viewer = () => {
    const shots = cur(), i = c.idx;
    if (i === null || i >= shots.length) return '';
    const s = shots[i];
    return h`<div class="cg-big"><button class="btn" data-cg="prev" aria-label="Previous">‹</button><div class="cg-img">${s ? h`<img class="${blurCls(s)}" src="${s.url}" alt="" referrerpolicy="no-referrer">` : h`<p class="muted cg-vwait" style="padding:60px 0">Loading image ${i + 1}…</p>`}</div><button class="btn" data-cg="next" aria-label="Next">›</button>
      <div class="cg-bar"><span class="tiny muted">${i + 1} / ${shots.length}</span>${s && nsfw(s) ? h`<button class="btn sm" data-cg="reveal">${c.show ? 'Blur' : 'Unblur'}</button>` : ''}<button class="btn sm" data-cg="close">Close viewer</button></div></div>`;
  };
  const grid = () => {
    const shots = cur();
    return h`<div class="vd-shots">${shots.map((s, k) => h`<button class="vd-shot ${k === c.idx ? 'on' : ''}" data-cg="shot" data-i="${k}"><img class="${blurCls(s)}" src="${s.thumb || s.url}" alt="Image ${k + 1}" loading="lazy" referrerpolicy="no-referrer"></button>`)}</div>`;
  };
  const results = () => {
    const x = r(), lbl = CG_SRC[c.src];
    const cards = x.items.length ? h`<div class="cg-results">${x.items.map((it, n) => h`<button class="cg-card" data-cg="open" data-ref="${it.ref}" data-title="${it.title}"><div class="cg-th"><span class="cg-num">${n + 1}</span>${it.thumb ? h`<img class="${blurCls()}" src="${it.thumb}" alt="" loading="lazy" referrerpolicy="no-referrer">` : ''}</div><div class="cg-ti">${it.title}</div>${it.pages ? h`<div class="tiny muted">${it.pages} images</div>` : ''}</button>`)}</div>` : '';
    let status = '';
    if (x.busy) status = h`<p class="muted cg-st">${c.src === 'hitomi' && x.total && x.scanned ? `Searching ${lbl}… checked ${x.scanned.toLocaleString()} of ${x.total.toLocaleString()} galleries` : `Searching ${lbl}…`} ${c.src === 'hitomi' ? h`<button class="btn sm" data-cg="stop">Stop</button>` : ''}</p>`;
    else if (x.err) status = h`<p class="cg-st">${x.err} <button class="btn sm" data-cg="search">Try again</button></p>`;
    else if (x.q !== null && !x.items.length && x.indexed) status = h`<p class="muted cg-st">Nothing on ${lbl} matches “${x.q}” in its search index. <button class="btn sm" data-cg="deep" title="Reads every gallery title; can take several minutes">Scan all galleries (slow)</button></p>`;
    else if (x.q !== null && !x.items.length) status = h`<p class="muted cg-st">${x.next >= 0 ? `Nothing found yet (checked ${x.scanned.toLocaleString()} of ${x.total.toLocaleString()}).` : `No Game CG galleries on ${lbl} match “${x.q}”.`} ${x.next >= 0 ? h`<button class="btn sm" data-cg="more">Keep searching</button>` : 'Try a shorter or different title.'}</p>`;
    else if (x.next >= 0) status = h`<p class="cg-st"><button class="btn sm" data-cg="more">${c.src === 'hitomi' && x.q ? `Keep searching (checked ${x.scanned.toLocaleString()} of ${x.total.toLocaleString()})` : 'More results'}</button></p>`;
    return h`${cards}${status}`;
  };
  const galStatus = () => {
    const G = c.gal;
    if (c.galBusy) return h`<p class="muted cg-st">Loading images…</p>`;
    if (c.galErr) return h`<p class="cg-st">${c.galErr} <button class="btn sm" data-cg="moreimg">Try again</button></p>`;
    return h`<p class="muted cg-st">${G.have} of ${G.imgs.length} images loaded. Scroll, or type a number above to jump to it.</p>`;
  };
  const galSt = () => { const m = $('#cgst', body); if (m && c.gal) setHTML(m, galStatus()); };
  const gallery = () => {
    const G = c.gal, total = G.imgs.length;
    return h`<div class="row" style="gap:8px;flex-wrap:wrap;margin:14px 0 4px"><button class="btn sm" data-cg="back">‹ Results</button><strong class="cg-gt">${G.title}</strong>${G.count ? h`<span class="tiny muted">${G.count} images</span>` : ''}
      ${total ? h`<span class="cg-jump"><input type="number" id="cgjump" min="1" max="${total}" placeholder="Go to #" aria-label="Go to image number"><button class="btn sm" data-cg="jump">Go</button></span>` : ''}
      <button class="btn sm" data-cg="forget" title="Forget this gallery for this game">Forget</button></div>
      ${viewer()}<div class="cg-vg" id="cgvg"></div><div id="cgst">${galStatus()}</div>`;
  };
  const main = () => {
    if (c.src === 'vndb') {
      if (c.vBusy) return h`<p class="muted" style="margin-top:20px">Loading from VNDB…</p>`;
      if (c.vErr) return h`<p style="margin-top:20px">${c.vErr}</p>`;
      if (!g.vndbId) return h`<p class="muted" style="margin-top:20px">This game isn't linked to VNDB yet. Open the Edit tab and use “Find on VNDB” to get its screenshots here.</p>`;
      if (!cur().length) return h`<p class="muted" style="margin-top:20px">VNDB has no screenshots for this game.</p>`;
      return h`${viewer()}<h3 class="vd-h">${cur().length} screenshots</h3>${grid()}`;
    }
    return isGal() ? gallery() : results();
  };
  /* Virtualised grid for E-Hentai galleries: every image has a fixed slot (its number), only the rows near the
     viewport are in the DOM, and only the gallery pages that cover visible slots are fetched. */
  const VGAP = 8, VMIN = 150;
  const vgGeo = () => {
    const el = $('#cgvg', body); if (!el || !c.gal) return null;
    const W = el.clientWidth; if (!W) return null;
    const total = c.gal.imgs.length;
    const cols = Math.max(1, Math.floor((W + VGAP) / (VMIN + VGAP)));
    const tw = (W - VGAP * (cols - 1)) / cols, th = tw * 9 / 16, rh = th + VGAP;
    return { el, total, cols, tw, th, rh, rows: Math.ceil(total / cols) };
  };
  const vgNeed = (a, b) => {
    const G = c.gal;
    if (c.galBusy || c.galErr || !G.perPage) return;
    let best = -1;
    if (c.idx !== null && c.idx < G.imgs.length && !G.imgs[c.idx]) best = c.idx;
    else {
      const mid = (a + b) >> 1; let bd = 1e9;
      for (let k = a; k <= b && k < G.imgs.length; k++) if (!G.imgs[k]) { const d = Math.abs(k - mid); if (d < bd) { bd = d; best = k; } }
    }
    if (best >= 0) { loadGallery(G.site, G.ref, G.title, Math.floor(best / G.perPage)); return; }
    // Gallery size unknown: fall back to loading page after page.
    if (!G.count && G.more && b >= G.imgs.length - 1) loadGallery(G.site, G.ref, G.title, (G.maxPage || 0) + 1);
  };
  const vgUpdate = () => {
    if (dlg.cur() !== 'cg' || !isGal()) return;
    const G = c.gal, v = vgGeo(); if (!v) return;
    const { el, total, cols, tw, th, rh, rows } = v;
    el.style.height = Math.max(0, rows * rh - VGAP) + 'px';
    const top = el.getBoundingClientRect().top - body.getBoundingClientRect().top;
    const r0 = Math.max(0, Math.floor(-top / rh) - 2), r1 = Math.min(rows - 1, Math.ceil((-top + body.clientHeight) / rh) + 2);
    if (r1 < r0) return;
    const key = [cols, Math.round(tw), r0, r1, total, G.have, c.idx, c.show, G.hl].join('|');
    if (key !== c.vgKey) {
      c.vgKey = key;
      const tiles = [];
      for (let k = r0 * cols; k <= Math.min(total - 1, (r1 + 1) * cols - 1); k++) {
        const st = `left:${(k % cols) * (tw + VGAP)}px;top:${Math.floor(k / cols) * rh}px;width:${tw}px;height:${th}px`, s = G.imgs[k];
        tiles.push(s
          ? h`<button class="vd-shot cg-vt ${k === c.idx ? 'on' : ''} ${k === G.hl ? 'hl' : ''}" style="${st}" data-cg="shot" data-i="${k}"><span class="cg-num">${k + 1}</span><img class="${blurCls(s)}" src="${s.thumb || s.url}" alt="Image ${k + 1}" loading="lazy" referrerpolicy="no-referrer"></button>`
          : h`<div class="vd-shot cg-vt cg-ph ${k === G.hl ? 'hl' : ''}" style="${st}"><span class="cg-num">${k + 1}</span></div>`);
      }
      setHTML(el, h`${tiles}`);
    }
    vgNeed(r0 * cols, Math.min(total - 1, (r1 + 1) * cols - 1));
    // The open viewer may be showing a picture whose page was never fetched (Next / Previous past the loaded part).
    if (c.idx !== null && !G.imgs[c.idx] && G.perPage && !c.galBusy) loadGallery(G.site, G.ref, G.title, Math.floor(c.idx / G.perPage));
  };
  const jump = () => {
    const inp = $('#cgjump', body), v = vgGeo(); if (!inp || !v) return;
    const n = Math.floor(+inp.value); if (!(n >= 1)) return;
    const slot = Math.min(n, v.total) - 1, G = c.gal;
    G.hl = slot; c.galErr = ''; clearTimeout(c.hlT); c.hlT = setTimeout(() => { G.hl = -1; c.vgKey = ''; vgUpdate(); }, 2500);
    const top = v.el.getBoundingClientRect().top - body.getBoundingClientRect().top;
    body.scrollTop += top + Math.floor(slot / v.cols) * v.rh - Math.max(0, (body.clientHeight - v.th) / 2);
    c.vgKey = ''; galSt(); vgUpdate();
  };
  const drawMain = () => {
    const m = $('#cgmain', body);
    if (m && dlg.cur() === 'cg') { setHTML(m, main()); c.vgKey = ''; vgUpdate(); }
  };
  const draw = () => {
    if (dlg.cur() !== 'cg') return;
    const web = c.src !== 'vndb';
    const qv = c.q[c.src] !== undefined ? c.q[c.src] : g.title;
    setHTML(body, h`<p class="muted" style="margin-top:0">${web ? `Game CG galleries from ${CG_SRC[c.src]}, fetched live and shown here. Adult images stay blurred until you press Unblur.` : 'Screenshots and CGs from VNDB.'} Use the tabs to pull CGs from E-Hentai or Hitomi.la.</p>
      <div class="chips" style="margin-bottom:10px">${Object.entries(CG_SRC).map(([k, l]) => h`<button class="chip ${c.src === k ? 'on' : ''}" data-cg="src" data-s="${k}">${l}</button>`)}${web ? h`<button class="chip ghost" data-cg="reveal">${c.show ? 'Blur images' : 'Unblur images'}</button>` : ''}</div>
      ${web && !isGal() ? h`<div class="row cg-search"><input type="text" id="cgq" value="${qv}" placeholder="Search Game CG by title" aria-label="Search title"><button class="btn primary" data-cg="search">Search</button>${g.originalTitle && g.originalTitle !== g.title ? h`<button class="btn" data-cg="orig" title="Search with the original title">Original title</button>` : ''}</div>` : ''}
      <div id="cgmain">${main()}</div>
      <details class="cg-more"><summary class="tiny muted">Open in your browser instead</summary>${cgLinkRow(g.title, g.originalTitle)}</details>`);
    c.vgKey = ''; vgUpdate();
  };
  c.draw = draw; c.drawMain = drawMain;

  /* --- loaders --- */
  const loadVNDB = async () => {
    if (!g.vndbId || c.vFor === g.vndbId) return;
    c.vFor = g.vndbId; c.vndb = null; c.vErr = '';
    if (S.cgCache[g.vndbId]) { c.vndb = S.cgCache[g.vndbId]; return; }
    c.vBusy = true; if (c.src === 'vndb') drawMain();
    try { const d = await api('GET', '/api/vndb/vn/' + encodeURIComponent(g.vndbId)); c.vndb = S.cgCache[g.vndbId] = d.screenshots; }
    catch (e) { c.vErr = e.message; c.vFor = ''; }
    c.vBusy = false;
  };
  const search = async (src, q, more, deep) => {
    const x = (c.res[src] || (c.res[src] = {})); const tok = ++c.tok;
    if (!more) Object.assign(x, { items: [], next: -1, err: '', done: false, scanned: 0, total: 0, q, indexed: false, deep: !!deep });
    x.busy = true; x.err = ''; x.stop = false; if (c.src === src) drawMain();
    let page = more ? x.next : 0, calls = 0;
    const alt = x.q === g.originalTitle ? g.title : (g.originalTitle || '');
    try {
      while (page >= 0 && tok === c.tok && !x.stop) {
        const d = await api('GET', `/api/cg/search?site=${src}&q=${encodeURIComponent(x.q)}&alt=${encodeURIComponent(alt)}&page=${page}${x.deep ? '&deep=1' : ''}`);
        if (tok !== c.tok) return;
        const have = new Set(x.items.map(i => i.ref));
        let added = 0;
        for (const it of d.items) if (!have.has(it.ref)) { x.items.push(it); added++; }
        x.indexed = !!d.indexed; x.next = d.next; x.scanned = d.scanned || 0; x.total = d.total || 0; page = d.next; calls++;
        if (c.src === src) drawMain();
        // E-Hentai: one result page per request. Hitomi: each call scans for a few seconds; keep going until
        // something turns up (or about a minute has passed), then offer "Keep searching".
        if (src !== 'hitomi' || x.indexed || !x.q || x.items.length || calls >= 8) break;
      }
    } catch (e) { if (tok === c.tok) x.err = e.message; }
    if (tok === c.tok) { x.busy = false; if (c.src === src) drawMain(); }
  };
  const loadGallery = async (site, ref, title, p) => {
    if (c.galBusy) return;
    if (!c.gal || c.gal.ref !== ref || c.gal.site !== site) c.gal = { site, ref, title, imgs: [], loaded: new Set(), have: 0, more: false, count: 0, perPage: 0, maxPage: 0, hl: -1 };
    const G = c.gal;
    c.src = site;
    if (G.loaded.has(p)) { draw(); return; }
    const tok = ++c.tok, first = !G.loaded.size;
    c.galBusy = true; c.galErr = ''; c.galWant = p;
    if (first) draw(); else galSt();
    try {
      const d = await api('GET', `/api/cg/gallery?site=${site}&ref=${encodeURIComponent(ref)}&p=${p}`);
      if (tok !== c.tok) return;
      if (d.perPage) G.perPage = d.perPage;
      let added = 0;
      d.images.forEach((im, k) => {
        const slot = im.n > 0 ? im.n - 1 : p * (G.perPage || d.images.length) + k;
        if (!G.imgs[slot]) { G.imgs[slot] = im; G.have++; added++; }
      });
      if (!added) throw new Error('The site returned no new images for this page. Please retry; if it continues, the site may be limiting pagination.');
      G.loaded.add(p); G.maxPage = Math.max(G.maxPage, p);
      G.title = d.title || G.title; G.count = d.count || G.count; G.more = d.more;
      if (G.count > G.imgs.length) G.imgs.length = G.count;
    } catch (e) { if (tok === c.tok) c.galErr = e.message; }
    if (tok === c.tok) {
      c.galBusy = false;
      if (dlg.cur() === 'cg') {
        // Rebuild the whole view only when it has to change shape: first page, or the open viewer was waiting for this image.
        if (first || !$('#cgvg', body) || ($('.cg-vwait', body) && c.idx !== null && G.imgs[c.idx])) drawMain();
        else { galSt(); c.vgKey = ''; vgUpdate(); }
      }
    }
  };
  c.loadGallery = loadGallery;

  /* --- events (bound once per dialog) --- */
  if (!dlg.cgBound) {
    dlg.cgBound = true;
    const go = async (src) => {
      c.src = src; c.idx = null; draw();
      if (src === 'vndb') { await loadVNDB(); if (c.src === 'vndb') drawMain(); return; }
      const pick = cgPickGet(g.id);
      if (!isGal() && pick && pick.site === src && !c.pickTried) { c.pickTried = true; return loadGallery(pick.site, pick.ref, pick.title, 0); }
      const x = r();
      if (x.q === null || x.q === undefined) search(src, c.q[src] !== undefined ? c.q[src] : g.title, false);
    };
    dlg.el.addEventListener('click', e => {
      const b = e.target.closest('[data-cg]'); if (!b || dlg.cur() !== 'cg') return;
      const k = b.dataset.cg, n = cur().length;
      if (k === 'link') return;
      if (k === 'src') return go(b.dataset.s);
      if (k === 'shot') { c.idx = +b.dataset.i; drawMain(); const v = $('.cg-big', body); if (v) v.scrollIntoView({ block: 'nearest' }); return; }
      if (k === 'prev') { c.idx = (c.idx + n - 1) % n; return drawMain(); }
      if (k === 'next') { c.idx = (c.idx + 1) % n; return drawMain(); }
      if (k === 'close') { c.idx = null; return drawMain(); }
      if (k === 'reveal') { c.show = !c.show; return draw(); }
      if (k === 'search') { const v = ($('#cgq', body) || {}).value; c.q[c.src] = (v !== undefined ? v : g.title).trim(); return search(c.src, c.q[c.src], false); }
      if (k === 'orig') { c.q[c.src] = g.originalTitle; const i = $('#cgq', body); if (i) i.value = g.originalTitle; return search(c.src, g.originalTitle, false); }
      if (k === 'more') return search(c.src, r().q, true);
      if (k === 'deep') return search(c.src, r().q, false, true);
      if (k === 'stop') { r().stop = true; return; }
      if (k === 'open') { cgPickSet(g.id, { site: c.src, ref: b.dataset.ref, title: b.dataset.title }); c.idx = null; return loadGallery(c.src, b.dataset.ref, b.dataset.title, 0); }
      if (k === 'back') { c.gal = null; c.idx = null; c.pickTried = true; return draw(); }
      if (k === 'forget') { cgPickSet(g.id, null); c.gal = null; c.idx = null; c.pickTried = true; return draw(); }
      if (k === 'jump') return jump();
      if (k === 'moreimg') return loadGallery(c.gal.site, c.gal.ref, c.gal.title, c.galWant || 0);
    });
    dlg.el.addEventListener('keydown', e => {
      if (dlg.cur() !== 'cg') return;
      if (e.target.id === 'cgjump') { if (e.key === 'Enter') jump(); return; }
      if (e.target.id === 'cgq') { if (e.key === 'Enter') { c.q[c.src] = e.target.value.trim(); search(c.src, c.q[c.src], false); } return; }
      const n = cur().length;
      if (c.idx === null || !n) return;
      if (e.key === 'ArrowLeft') { c.idx = (c.idx + n - 1) % n; drawMain(); } else if (e.key === 'ArrowRight') { c.idx = (c.idx + 1) % n; drawMain(); }
    });
    let vgRaf = 0;
    const vgSoon = () => { if (!vgRaf) vgRaf = requestAnimationFrame(() => { vgRaf = 0; vgUpdate(); }); };
    body.addEventListener('scroll', vgSoon, { passive: true });
    window.addEventListener('resize', vgSoon);
    dlg.el.addEventListener('input', e => { if (e.target.id === 'cgq') c.q[c.src] = e.target.value; });
  }
  dlg.cgDraw = () => draw();

  // First time on this tab for this game: show VNDB shots (the old behaviour), and jump straight back into
  // the gallery the user picked last time, if any.
  if (c.for !== g.id) {
    c.for = g.id;
    const pick = cgPickGet(g.id);
    draw();
    if (pick && (pick.site in CG_SRC && pick.site !== 'vndb')) { c.pickTried = true; c.src = pick.site; return loadGallery(pick.site, pick.ref, pick.title, 0); }
    c.src = 'vndb';
    await loadVNDB(); if (dlg.cur() === 'cg') { if (c.src === 'vndb') drawMain(); }
    return;
  }
  draw();
  if (c.src === 'vndb') { await loadVNDB(); if (c.src === 'vndb') drawMain(); }
}

/* ---------- guides: walkthrough images and text files ---------- */
const fmtSize = n => n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB';
async function guideUpload(g, files) {
  let ok = 0;
  for (const f of files) {
    try { await api('POST', `/api/games/${g.id}/guides?name=${encodeURIComponent(f.name || 'pasted.png')}`, undefined, f); ok++; }
    catch (e) { toast(`${f.name || 'file'}: ${e.message}`, 'err'); }
  }
  if (ok) { await load(); toast(`Attached ${ok} file${ok === 1 ? '' : 's'}`, 'ok'); }
}
function viewGuides(dlg, g) {
  const list = g.guides || [], imgs = list.filter(x => x.kind === 'image');
  const cur = dlg.gv ? list.find(x => x.id === dlg.gv) : null;
  if (dlg.gv && !cur) dlg.gv = null;
  const onTab = () => ((dlg.el.querySelector('.tab.on') || { dataset: {} }).dataset.t === 'guides');
  if (!dlg._gbound) {
    dlg._gbound = true;
    dlg.el.addEventListener('paste', e => {
      if (!onTab() || dlg.el.dataset.gid !== g.id) return;
      const fs = [...(e.clipboardData ? e.clipboardData.files : [])].filter(f => f.type.startsWith('image/'));
      if (fs.length) { e.preventDefault(); guideUpload(game(g.id), fs.map(f => new File([f], 'pasted-' + new Date().toISOString().slice(0, 16).replace(/[:T]/g, '') + '.png', { type: f.type }))); }
    });
    dlg.el.addEventListener('keydown', e => {
      if (!dlg.gv || !onTab()) return;
      const gm = game(dlg.el.dataset.gid), im = (gm.guides || []).filter(x => x.kind === 'image'), i = im.findIndex(x => x.id === dlg.gv);
      if (i < 0) return;
      if (e.key === 'ArrowRight' && im[i + 1]) { dlg.gv = im[i + 1].id; dlg.rerender(); }
      if (e.key === 'ArrowLeft' && im[i - 1]) { dlg.gv = im[i - 1].id; dlg.rerender(); }
    });
  }
  if (cur && cur.kind === 'image') {
    const i = imgs.findIndex(x => x.id === cur.id);
    setHTML(dlg.body, h`<div class="row" style="justify-content:space-between;margin-bottom:10px;flex-wrap:wrap"><button class="btn sm" data-x="gback">← All guides</button><b style="min-width:0;overflow:hidden;text-overflow:ellipsis">${cur.name}</b><span class="row" style="gap:6px"><button class="btn sm" data-x="gprev" ${i > 0 ? '' : 'disabled'}>‹ Prev</button><span class="tiny muted">${i + 1} / ${imgs.length}</span><button class="btn sm" data-x="gnext" ${i < imgs.length - 1 ? '' : 'disabled'}>Next ›</button></span></div>
      <div class="row zbar"><button class="btn sm" data-x="zout" aria-label="Zoom out">−</button><input type="range" id="zrange" min="10" max="800" value="100" aria-label="Zoom"><button class="btn sm" data-x="zin" aria-label="Zoom in">+</button><span class="zpct" id="zpct">100%</span><button class="btn sm" data-x="zfit">Fit</button><button class="btn sm" data-x="z100">100%</button></div>
      <div class="row mbar" id="mbar"><button class="btn sm" data-m="toggle" id="mtog">${icon('edit')}Mark</button><button class="btn sm" data-m="save" title="Save a copy of this image with your marks as a PNG">${icon('down')}Save marked image</button>
        <span class="row mtools" id="mtools" hidden style="gap:6px;flex-wrap:wrap"><button class="btn sm" data-m="tool" data-t="pen">Pen</button><button class="btn sm" data-m="tool" data-t="hl">Highlighter</button><button class="btn sm" data-m="tool" data-t="er">Eraser</button>
          ${['#ff3b30', '#ff9500', '#ffd60a', '#34c759', '#0a84ff', '#bf5af2', '#ffffff', '#111111'].map(c => h`<button class="sw" data-m="col" data-c="${c}" style="background:${c}" aria-label="Color ${c}"></button>`)}
          <input type="range" id="msize" min="1" max="4" value="2" aria-label="Brush size" style="width:80px"><button class="btn sm" data-m="undo">Undo</button><button class="btn sm" data-m="clear">Clear marks</button></span></div>
      <div class="gview" id="gview"><div class="gwrap" id="gwrap"><img id="gimg" src="/guides/${encodeURIComponent(g.id)}/${encodeURIComponent(cur.file)}" alt="${cur.name}" draggable="false"><canvas class="gmark" id="gmark"></canvas></div></div><div class="tiny muted" style="margin-top:6px">Mouse wheel or + / − to zoom · drag to move · double-click toggles fit / 100% · ← → change page · <b>Mark</b> draws on the page (saved with the guide; hold Space or use the middle button to move while drawing)</div>`);
    guideZoomInit(dlg);
    guideMarksInit(dlg, g, cur);
  } else if (cur) {
    setHTML(dlg.body, h`<div class="row" style="justify-content:space-between;margin-bottom:10px;flex-wrap:wrap"><button class="btn sm" data-x="gback">← All guides</button><b style="min-width:0;overflow:hidden;text-overflow:ellipsis">${cur.name}</b><span class="row" style="gap:6px"><button class="btn sm" data-x="gfont" data-d="-1" aria-label="Zoom out">A−</button><span class="zpct" id="zpct">${Math.round((dlg.gfs || 14) / 14 * 100)}%</span><button class="btn sm" data-x="gfont" data-d="1" aria-label="Zoom in">A+</button><button class="btn sm" data-x="gfont" data-d="0">Reset</button></span></div><pre class="gtext" id="gtext" tabindex="0" style="font-size:${dlg.gfs || 14}px">Loading…</pre><div class="tiny muted" style="margin-top:6px">Ctrl + mouse wheel or + / − to zoom the text</div>`);
    guideTextZoomInit(dlg);
    api('GET', `/api/games/${g.id}/guides/${cur.id}/text`).then(r => { const p = $('#gtext', dlg.el); if (p) p.textContent = r.text; }).catch(e => { const p = $('#gtext', dlg.el); if (p) p.textContent = e.message; });
  } else {
    setHTML(dlg.body, h`<div class="gdrop" data-x="gdrop"><div><b>Attach walkthroughs, route charts or notes</b></div><div class="muted tiny" style="margin:4px 0 10px">Images (PNG, JPEG, GIF, WebP) and text files (.txt, .md, Shift-JIS is fine). Drop files here, paste a screenshot with Ctrl+V, or browse.</div><button class="btn primary" data-x="gadd">${icon('plus')}Add files…</button><input type="file" id="gfile" multiple accept="image/png,image/jpeg,image/gif,image/webp,.txt,.md,.text,.log" hidden></div>
      ${list.length ? h`<div class="gridg">${list.map(x => h`<div class="gitem" data-x="gopen" data-id="${x.id}" tabindex="0" role="button">${x.kind === 'image' ? h`<img src="/guides/${encodeURIComponent(g.id)}/${encodeURIComponent(x.file)}" alt="" loading="lazy">` : h`<div class="gtxt">📄</div>`}<div class="gcap"><span title="${x.name}">${x.name}</span><span class="tiny muted">${fmtSize(x.size)}</span></div><button class="icon-btn gdel" data-x="gdel" data-id="${x.id}" aria-label="Remove ${x.name}">${icon('trash')}</button></div>`)}</div>` : h`<p class="muted" style="text-align:center;margin-top:20px">Nothing attached yet.</p>`}`);
  }
  const b = dlg.body;
  b.onclick = guard(async e => {
    const t = e.target.closest('[data-x]'); if (!t) return;
    const k = t.dataset.x;
    if (k === 'gadd') $('#gfile', b).click();
    else if (k === 'gdel') {
      e.stopPropagation();
      if (await confirmBox('Remove this attachment?', 'The file is deleted from ShioriBox’s data folder. Your original file elsewhere is not touched.')) { await api('DELETE', `/api/games/${g.id}/guides/${t.dataset.id}`); await load(); }
    }
    else if (k === 'gopen') { dlg.gv = t.dataset.id; dlg.gscale = 'fit'; dlg.rerender(); }
    else if (k === 'gback') { dlg.gv = null; dlg.rerender(); }
    else if (k === 'gprev' || k === 'gnext') { const i = imgs.findIndex(x => x.id === dlg.gv) + (k === 'gnext' ? 1 : -1); if (imgs[i]) { dlg.gv = imgs[i].id; dlg.rerender(); } }
    else if (k === 'zin') dlg.zoomBy && dlg.zoomBy(1.25);
    else if (k === 'zout') dlg.zoomBy && dlg.zoomBy(0.8);
    else if (k === 'zfit') dlg.zoomTo && dlg.zoomTo('fit');
    else if (k === 'z100') dlg.zoomTo && dlg.zoomTo(1);
    else if (k === 'gfont') dlg.textZoom && dlg.textZoom(+t.dataset.d);
  });
  b.onchange = guard(async e => { if (e.target.id === 'gfile') { const fs = [...e.target.files]; e.target.value = ''; await guideUpload(game(g.id), fs); } });
  b.ondragover = e => { e.preventDefault(); };
  b.ondrop = guard(async e => { e.preventDefault(); const fs = [...e.dataTransfer.files]; if (fs.length) await guideUpload(game(g.id), fs); });
}

/* zoom for the guide viewers */
function guideZoomInit(dlg) {
  const view = $('#gview', dlg.el), img = $('#gimg', dlg.el), wrap = $('#gwrap', dlg.el), pct = $('#zpct', dlg.el), range = $('#zrange', dlg.el);
  if (!view || !img) return;
  const MIN = 0.1, MAX = 8;
  const nat = () => img.naturalWidth || 1;
  const cur = () => dlg.gscale === 'fit' || dlg.gscale == null ? img.clientWidth / nat() : dlg.gscale;
  const paint = () => { const s = cur(); pct.textContent = Math.round(s * 100) + '%'; range.value = Math.round(s * 100); view.classList.toggle('panable', img.clientWidth > view.clientWidth || img.clientHeight > view.clientHeight); };
  const apply = () => {
    if (dlg.gscale === 'fit' || dlg.gscale == null) { wrap.style.maxWidth = '100%'; img.style.width = ''; img.style.maxWidth = '100%'; img.style.maxHeight = '62vh'; }
    else { wrap.style.maxWidth = 'none'; img.style.maxWidth = 'none'; img.style.maxHeight = 'none'; img.style.width = Math.round(nat() * dlg.gscale) + 'px'; }
    paint();
  };
  const zoomTo = (s, mx, my) => {
    if (s === 'fit') { dlg.gscale = 'fit'; apply(); view.scrollLeft = view.scrollTop = 0; return; }
    s = Math.min(MAX, Math.max(MIN, s));
    const oldW = img.clientWidth, oldH = img.clientHeight;
    mx = mx ?? view.clientWidth / 2; my = my ?? view.clientHeight / 2;
    const rx = (view.scrollLeft + mx - wrap.offsetLeft) / oldW, ry = (view.scrollTop + my - wrap.offsetTop) / oldH;
    dlg.gscale = s; apply();
    view.scrollLeft = rx * img.clientWidth + wrap.offsetLeft - mx; view.scrollTop = ry * img.clientHeight + wrap.offsetTop - my;
  };
  dlg.zoomTo = zoomTo;
  dlg.zoomBy = f => zoomTo(cur() * f);
  img.addEventListener('load', apply); if (img.complete) apply();
  range.addEventListener('input', () => zoomTo(range.value / 100));
  view.addEventListener('wheel', e => { e.preventDefault(); const r = view.getBoundingClientRect(); zoomTo(cur() * (e.deltaY < 0 ? 1.15 : 1 / 1.15), e.clientX - r.left, e.clientY - r.top); }, { passive: false });
  view.addEventListener('dblclick', e => { if (dlg.mk && dlg.mk.on) return; const r = view.getBoundingClientRect(); if (dlg.gscale === 'fit' || dlg.gscale == null) zoomTo(1, e.clientX - r.left, e.clientY - r.top); else zoomTo('fit'); });
  let drag = null;
  view.addEventListener('pointerdown', e => { if (e.button !== 0 && e.button !== 1) return; if (e.button === 0 && dlg.mk && dlg.mk.on && !dlg._space) return; e.preventDefault(); drag = { x: e.clientX, y: e.clientY, l: view.scrollLeft, t: view.scrollTop }; view.setPointerCapture(e.pointerId); view.classList.add('dragging'); });
  view.addEventListener('pointermove', e => { if (!drag) return; view.scrollLeft = drag.l - (e.clientX - drag.x); view.scrollTop = drag.t - (e.clientY - drag.y); });
  const end = () => { drag = null; view.classList.remove('dragging'); };
  view.addEventListener('pointerup', end); view.addEventListener('pointercancel', end);
  if (!dlg._zkeys) {
    dlg._zkeys = true;
    dlg.el.addEventListener('keydown', e => {
      if (!dlg.zoomBy || !$('#gview', dlg.el) || e.target.matches('input[type=text],textarea')) return;
      if (e.key === '+' || e.key === '=') { e.preventDefault(); dlg.zoomBy(1.25); }
      else if (e.key === '-' || e.key === '_') { e.preventDefault(); dlg.zoomBy(0.8); }
      else if (e.key === '0') { e.preventDefault(); dlg.zoomTo('fit'); }
    });
  }
}
/* brush marks over a guide image: strokes are stored relative to the image size and saved next to it */
function guideMarksInit(dlg, g, cur) {
  const view = $('#gview', dlg.el), img = $('#gimg', dlg.el), cv = $('#gmark', dlg.el), bar = $('#mbar', dlg.el);
  if (!view || !img || !cv || !bar) return;
  const mk = dlg.mk || (dlg.mk = { on: false, tool: 'pen', color: '#ff3b30', size: 2 });
  const ctx = cv.getContext('2d');
  const gameId = g.id, gid = cur.id;
  let strokes = [], live = null, hist = [], timer = 0;
  const widthFor = () => ({ 1: 0.0025, 2: 0.005, 3: 0.009, 4: 0.015 }[mk.size] * (mk.tool === 'hl' ? 3.2 : mk.tool === 'er' ? 3 : 1));
  const drawStroke = (s, c = ctx, W = cv.width, H = cv.height) => {
    if (!s.p.length) return;
    c.save(); c.globalAlpha = s.t === 'hl' ? 0.38 : 1; c.globalCompositeOperation = s.t === 'er' ? 'destination-out' : 'source-over'; c.strokeStyle = s.c || '#000'; c.lineWidth = Math.max(1, s.w * W); c.lineCap = 'round'; c.lineJoin = 'round';
    c.beginPath(); const P = s.p.map(q => [q[0] * W, q[1] * H]);
    c.moveTo(P[0][0], P[0][1]);
    if (P.length === 1) c.lineTo(P[0][0] + 0.01, P[0][1]);
    for (let i = 1; i < P.length - 1; i++) c.quadraticCurveTo(P[i][0], P[i][1], (P[i][0] + P[i + 1][0]) / 2, (P[i][1] + P[i + 1][1]) / 2);
    if (P.length > 1) c.lineTo(P[P.length - 1][0], P[P.length - 1][1]);
    c.stroke(); c.restore();
  };
  const paint = () => { const s = cur(); pct.textContent = Math.round(s * 100) + '%'; range.value = Math.round(s * 100); view.classList.toggle('panable', img.clientWidth > view.clientWidth || img.clientHeight > view.clientHeight); };
  const apply = () => {
    if (dlg.gscale === 'fit' || dlg.gscale == null) { wrap.style.maxWidth = '100%'; img.style.width = ''; img.style.maxWidth = '100%'; img.style.maxHeight = '62vh'; }
    else { wrap.style.maxWidth = 'none'; img.style.maxWidth = 'none'; img.style.maxHeight = 'none'; img.style.width = Math.round(nat() * dlg.gscale) + 'px'; }
    paint();
  };
  const zoomTo = (s, mx, my) => {
    if (s === 'fit') { dlg.gscale = 'fit'; apply(); view.scrollLeft = view.scrollTop = 0; return; }
    s = Math.min(MAX, Math.max(MIN, s));
    const oldW = img.clientWidth, oldH = img.clientHeight;
    mx = mx ?? view.clientWidth / 2; my = my ?? view.clientHeight / 2;
    const rx = (view.scrollLeft + mx - wrap.offsetLeft) / oldW, ry = (view.scrollTop + my - wrap.offsetTop) / oldH;
    dlg.gscale = s; apply();
    view.scrollLeft = rx * img.clientWidth + wrap.offsetLeft - mx; view.scrollTop = ry * img.clientHeight + wrap.offsetTop - my;
  };
  dlg.zoomTo = zoomTo;
  dlg.zoomBy = f => zoomTo(cur() * f);
  img.addEventListener('load', apply); if (img.complete) apply();
  range.addEventListener('input', () => zoomTo(range.value / 100));
  view.addEventListener('wheel', e => { e.preventDefault(); const r = view.getBoundingClientRect(); zoomTo(cur() * (e.deltaY < 0 ? 1.15 : 1 / 1.15), e.clientX - r.left, e.clientY - r.top); }, { passive: false });
  view.addEventListener('dblclick', e => { if (dlg.mk && dlg.mk.on) return; const r = view.getBoundingClientRect(); if (dlg.gscale === 'fit' || dlg.gscale == null) zoomTo(1, e.clientX - r.left, e.clientY - r.top); else zoomTo('fit'); });
  let drag = null;
  view.addEventListener('pointerdown', e => { if (e.button !== 0 && e.button !== 1) return; if (e.button === 0 && dlg.mk && dlg.mk.on && !dlg._space) return; e.preventDefault(); drag = { x: e.clientX, y: e.clientY, l: view.scrollLeft, t: view.scrollTop }; view.setPointerCapture(e.pointerId); view.classList.add('dragging'); });
  view.addEventListener('pointermove', e => { if (!drag) return; view.scrollLeft = drag.l - (e.clientX - drag.x); view.scrollTop = drag.t - (e.clientY - drag.y); });
  const end = () => { drag = null; view.classList.remove('dragging'); };
  view.addEventListener('pointerup', end); view.addEventListener('pointercancel', end);
  if (!dlg._zkeys) {
    dlg._zkeys = true;
    dlg.el.addEventListener('keydown', e => {
      if (!dlg.zoomBy || !$('#gview', dlg.el) || e.target.matches('input[type=text],textarea')) return;
      if (e.key === '+' || e.key === '=') { e.preventDefault(); dlg.zoomBy(1.25); }
      else if (e.key === '-' || e.key === '_') { e.preventDefault(); dlg.zoomBy(0.8); }
      else if (e.key === '0') { e.preventDefault(); dlg.zoomTo('fit'); }
    });
  }
}
/* brush marks over a guide image: strokes are stored relative to the image size and saved next to it */
function guideMarksInit(dlg, g, cur) {
  const view = $('#gview', dlg.el), img = $('#gimg', dlg.el), cv = $('#gmark', dlg.el), bar = $('#mbar', dlg.el);
  if (!view || !img || !cv || !bar) return;
  const mk = dlg.mk || (dlg.mk = { on: false, tool: 'pen', color: '#ff3b30', size: 2 });
  const ctx = cv.getContext('2d');
  const gameId = g.id, gid = cur.id;
  let strokes = [], live = null, hist = [], timer = 0;
  const widthFor = () => ({ 1: 0.0025, 2: 0.005, 3: 0.009, 4: 0.015 }[mk.size] * (mk.tool === 'hl' ? 3.2 : mk.tool === 'er' ? 3 : 1));
  const drawStroke = s => {
    const W = cv.width; if (!s.p.length) return;
    ctx.save(); ctx.globalAlpha = s.t === 'hl' ? 0.38 : 1; ctx.globalCompositeOperation = s.t === 'er' ? 'destination-out' : 'source-over'; ctx.strokeStyle = s.c || '#000'; ctx.lineWidth = Math.max(1, s.w * W); ctx.lineCap = 'round'; ctx.lineJoin = 'round';
    ctx.beginPath(); const P = s.p.map(q => [q[0] * cv.width, q[1] * cv.height]);
    ctx.moveTo(P[0][0], P[0][1]);
    if (P.length === 1) ctx.lineTo(P[0][0] + 0.01, P[0][1]);
    for (let i = 1; i < P.length - 1; i++) ctx.quadraticCurveTo(P[i][0], P[i][1], (P[i][0] + P[i + 1][0]) / 2, (P[i][1] + P[i + 1][1]) / 2);
    if (P.length > 1) ctx.lineTo(P[P.length - 1][0], P[P.length - 1][1]);
    ctx.stroke(); ctx.restore();
  };
  const paint = () => { ctx.clearRect(0, 0, cv.width, cv.height); strokes.forEach(drawStroke); if (live) drawStroke(live); };
  const fit = () => { const d = window.devicePixelRatio || 1, w = Math.round(img.clientWidth * d), hh = Math.round(img.clientHeight * d); if (w && (cv.width !== w || cv.height !== hh)) { cv.width = w; cv.height = hh; } paint(); };
  new ResizeObserver(fit).observe(img);
  const save = () => {
    clearTimeout(timer);
    const body = JSON.stringify({ v: 1, strokes });
    timer = setTimeout(() => { api('PUT', `/api/games/${gameId}/guides/${gid}/marks`, null, body).catch(e => toast(e.message, 'err')); }, 400);
  };
  const tools = () => {
    bar.classList.toggle('on', mk.on); $('#mtools', dlg.el).hidden = !mk.on; $('#mtog', dlg.el).classList.toggle('primary', mk.on);
    view.classList.toggle('drawing', mk.on);
    $$('[data-m="tool"]', bar).forEach(b => b.classList.toggle('primary', b.dataset.t === mk.tool));
    $$('[data-m="col"]', bar).forEach(b => b.classList.toggle('on', b.dataset.c === mk.color));
    $('#msize', dlg.el).value = mk.size;
  };
  tools();
  api('GET', `/api/games/${gameId}/guides/${gid}/marks`).then(r => { strokes = (r && r.strokes) || []; paint(); }).catch(() => {});
  bar.addEventListener('click', e => {
    const t = e.target.closest('[data-m]'); if (!t) return; const k = t.dataset.m;
    if (k === 'toggle') mk.on = !mk.on;
    else if (k === 'tool') { mk.tool = t.dataset.t; mk.on = true; }
    else if (k === 'col') { mk.color = t.dataset.c; if (mk.tool === 'er') mk.tool = 'pen'; mk.on = true; }
    else if (k === 'undo') { if (hist.length) { strokes = hist.pop(); paint(); save(); } }
    else if (k === 'save') { saveMarked(); return; }
    else if (k === 'clear') { if (strokes.length) { hist.push(strokes); strokes = []; paint(); save(); } }
    tools();
  });
  /* export: the original image with the marks burned in, at the image's full resolution, as a new PNG */
  const saveMarked = () => {
    if (!strokes.length) { toast('Nothing marked on this image yet', 'err'); return; }
    const W = img.naturalWidth, H = img.naturalHeight; if (!W) return;
    const layer = document.createElement('canvas'), out = document.createElement('canvas');
    layer.width = out.width = W; layer.height = out.height = H;
    const lc = layer.getContext('2d'), oc = out.getContext('2d');
    strokes.forEach(s => drawStroke(s, lc, W, H));
    oc.drawImage(img, 0, 0, W, H); oc.drawImage(layer, 0, 0);
    out.toBlob(blob => {
      if (!blob) { toast('Could not create the image', 'err'); return; }
      const url = URL.createObjectURL(blob), el = document.createElement('a');
      el.href = url; el.download = cur.name.replace(/\.[^.]+$/, '') + '-marked.png';
      document.body.appendChild(el); el.click(); el.remove(); setTimeout(() => URL.revokeObjectURL(url), 10000);
      toast('Saved ' + el.download, 'ok');
    }, 'image/png');
  };
  $('#msize', dlg.el).addEventListener('input', e => { mk.size = +e.target.value; });
  const pos = e => { const r = cv.getBoundingClientRect(); return [Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)), Math.min(1, Math.max(0, (e.clientY - r.top) / r.height))]; };
  let down = false;
  cv.addEventListener('pointerdown', e => {
    if (!mk.on || e.button !== 0 || dlg._space) return;
    e.preventDefault(); cv.setPointerCapture(e.pointerId); down = true;
    const [x, y] = pos(e);
    live = { t: mk.tool, c: mk.color, w: widthFor(), p: [[+x.toFixed(4), +y.toFixed(4)]] }; paint();
  });
  cv.addEventListener('pointermove', e => {
    if (!down) return; const [x, y] = pos(e);
    if (!live) return; const l = live.p[live.p.length - 1];
    if (Math.hypot((x - l[0]) * cv.width, (y - l[1]) * cv.height) < 1.5) return;
    live.p.push([+x.toFixed(4), +y.toFixed(4)]); paint();
  });
  const up = () => {
    if (!down) return; down = false;
    if (live) { hist.push(strokes); strokes = strokes.concat([live]); live = null; paint(); save(); if (hist.length > 50) hist.shift(); }
  };
  cv.addEventListener('pointerup', up); cv.addEventListener('pointercancel', up);
  dlg.markUndo = () => { if (hist.length) { strokes = hist.pop(); paint(); save(); } };
  if (!dlg._mkeys) {
    dlg._mkeys = true;
    dlg.el.addEventListener('keydown', e => {
      if (e.code === 'Space' && dlg.mk && dlg.mk.on && $('#gview', dlg.el) && !e.target.matches('input,textarea,button,select')) { dlg._space = true; e.preventDefault(); }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z' && dlg.markUndo && $('#gview', dlg.el) && !e.target.matches('input[type=text],textarea')) { e.preventDefault(); dlg.markUndo(); }
    });
    dlg.el.addEventListener('keyup', e => { if (e.code === 'Space') dlg._space = false; });
  }
}
function guideTextZoomInit(dlg) {
  const pre = $('#gtext', dlg.el), pct = $('#zpct', dlg.el);
  if (!pre) return;
  dlg.zoomBy = dlg.zoomTo = null;
  const set = fs => { dlg.gfs = Math.min(48, Math.max(9, Math.round(fs))); pre.style.fontSize = dlg.gfs + 'px'; pct.textContent = Math.round(dlg.gfs / 14 * 100) + '%'; };
  dlg.textZoom = d => set(d === 0 ? 14 : (dlg.gfs || 14) * (d > 0 ? 1.15 : 1 / 1.15));
  pre.addEventListener('wheel', e => { if (!e.ctrlKey) return; e.preventDefault(); dlg.textZoom(e.deltaY < 0 ? 1 : -1); }, { passive: false });
  if (!dlg._tkeys) {
    dlg._tkeys = true;
    dlg.el.addEventListener('keydown', e => {
      if (!$('#gtext', dlg.el)) return;
      if (e.key === '+' || e.key === '=') { e.preventDefault(); dlg.textZoom(1); }
      else if (e.key === '-' || e.key === '_') { e.preventDefault(); dlg.textZoom(-1); }
      else if (e.key === '0') { e.preventDefault(); dlg.textZoom(0); }
    });
  }
}

function viewSessions(dlg, g) {
  const rows = g.sessions.map((s, i) => ({ s, i })).sort((a, b) => new Date(b.s.start) - new Date(a.s.start));
  const today = new Date().toISOString().slice(0, 10);
  setHTML(dlg.body, h`<div class="row" style="margin-bottom:14px;flex-wrap:wrap"><div class="stat" style="min-width:150px"><b>${fmtDur(total(g))}</b><span>Total · ${g.sessions.length} sessions</span></div></div>
    <h2>Add time manually</h2>
    <div class="row" style="flex-wrap:wrap;margin-bottom:22px"><input type="date" id="ms-d" value="${today}" style="width:auto"><input type="number" id="ms-h" min="0" value="1" style="width:80px" aria-label="Hours"> h <input type="number" id="ms-m" min="0" max="59" value="0" style="width:80px" aria-label="Minutes"> m <button class="btn" data-x="add-s">Add</button></div>
    <h2>History</h2>${rows.length ? h`<table class="t"><tr><th>Started</th><th>Length</th><th></th></tr>${rows.map(({ s, i }) => h`<tr><td>${new Date(s.start).toLocaleString()}</td><td>${fmtDur(s.seconds)}</td><td style="text-align:right"><button class="icon-btn" data-x="del-s" data-i="${i}" aria-label="Delete session">${icon('trash')}</button></td></tr>`)}</table>` : h`<p class="muted">No playtime recorded yet. Launch the game from ShioriBox and it is timed automatically.</p>`}`);
  dlg.body.onclick = guard(async e => {
    const a = e.target.closest('[data-x]'); if (!a) return;
    if (a.dataset.x === 'add-s') {
      const sec = (+$('#ms-h', dlg.body).value || 0) * 3600 + (+$('#ms-m', dlg.body).value || 0) * 60;
      if (sec <= 0) return toast('Enter a length of time', 'err');
      const d = new Date($('#ms-d', dlg.body).value + 'T12:00:00');
      await api('POST', `/api/games/${g.id}/sessions`, { start: d.toISOString(), seconds: sec }); await load();
    }
    if (a.dataset.x === 'del-s') { await api('DELETE', `/api/games/${g.id}/sessions/${a.dataset.i}`); await load(); }
  });
}

function viewSaves(dlg, g) {
  if (!g.savePath) {
    setHTML(dlg.body, h`<p>Tell ShioriBox where this game keeps its save files and you can back them up (and restore them) in one click. Many games store saves in their own <code>savedata</code> folder, or under <code>%APPDATA%</code> / <code>Documents</code>.</p><button class="btn primary" data-x="goedit">Set save location</button>`);
    dlg.body.onclick = e => { if (e.target.closest('[data-x="goedit"]')) dlg.go('edit'); }; return;
  }
  setHTML(dlg.body, h`<p class="muted" style="margin-top:0">Save location: <code>${g.savePath}</code></p><div class="row" style="margin-bottom:16px"><button class="btn primary" data-x="bk">Back up now</button><span class="muted tiny">${S.lib.settings.autoBackup ? 'Also backed up automatically when the game closes.' : 'Automatic backups are off (Settings).'}</span></div><div id="bklist" class="muted">Loading…</div>`);
  const loadList = guard(async () => {
    const list = await api('GET', `/api/games/${g.id}/backups`);
    setHTML($('#bklist', dlg.body), list.length ? h`<table class="t"><tr><th>Backup</th><th>Size</th><th></th></tr>${list.map(b => h`<tr><td>${new Date(b.time).toLocaleString()}${b.name.startsWith('pre-restore') ? h` <span class="tag">before a restore</span>` : ''}</td><td>${(b.size / 1024).toFixed(0)} KB</td><td style="text-align:right;white-space:nowrap"><button class="btn sm" data-x="rs" data-n="${b.name}">Restore</button> <button class="icon-btn" data-x="rm" data-n="${b.name}" aria-label="Delete backup">${icon('trash')}</button></td></tr>`)}</table>` : h`<p class="muted">No backups yet.</p>`);
  });
  loadList();
  dlg.body.onclick = guard(async e => {
    const a = e.target.closest('[data-x]'); if (!a) return;
    if (a.dataset.x === 'bk') { await api('POST', `/api/games/${g.id}/backups`); toast('Backup created', 'ok'); loadList(); }
    if (a.dataset.x === 'rs') {
      if (await confirmBox('Restore this backup?', 'Your current save files will be replaced. A snapshot of them is taken first, so you can undo this.', 'Restore', false)) {
        await api('POST', `/api/games/${g.id}/backups/${a.dataset.n}/restore`); toast('Save restored', 'ok'); loadList();
      }
    }
    if (a.dataset.x === 'rm') { await api('DELETE', `/api/games/${g.id}/backups/${a.dataset.n}`); loadList(); }
  });
}

function viewEdit(dlg, g) {
  const isNew = !g;
  const f = g ? payload(g) : { title: '', originalTitle: '', developer: '', releaseDate: '', description: '', exePath: '', launchArgs: '', useWrapper: false, locale: S.lib.settings.defaultLocale || '', nsfw: false, savePath: '', status: 'planned', rating: 0, tags: [], categories: [], notes: '', vndbId: '', favorite: false };
  const pend = { file: null, url: null, remove: false };
  setHTML(dlg.body, h`<form id="gf" autocomplete="off">
    <div class="cover-edit"><div class="cv" id="cvp">${g ? cover(g) : h`<div class="ph" style="--h:260">?</div>`}</div>
      <div><div class="row" style="flex-wrap:wrap"><button type="button" class="btn sm" data-x="up">Upload cover…</button>${g && g.cover ? h`<button type="button" class="btn sm ghost" data-x="rmcv">Remove</button>` : ''}<button type="button" class="btn sm primary" data-x="vndb">Find on VNDB</button></div>
      <p class="tiny muted" style="margin:8px 0 0">“Find on VNDB” fills in the details and cover for you. <input type="file" id="cvf" accept="image/*" hidden></p></div></div>
    <div class="two"><label class="f"><span>Title *</span><input type="text" name="title" required value="${f.title}"></label><label class="f"><span>Original title</span><input type="text" name="originalTitle" value="${f.originalTitle}"></label>
    <label class="f"><span>Developer</span><input type="text" name="developer" value="${f.developer}"></label><label class="f"><span>Release date</span><input type="text" name="releaseDate" placeholder="YYYY-MM-DD" value="${f.releaseDate}"></label></div>
    <label class="f"><span>Game executable</span><div class="pathrow"><input type="text" name="exePath" value="${f.exePath}" placeholder="C:\\Games\\MyNovel\\game.exe"><button type="button" class="btn" data-x="pick-exe">Browse</button></div></label>
    <div class="two"><label class="f"><span>Launch arguments</span><input type="text" name="launchArgs" value="${f.launchArgs}"></label>
    <label class="f"><span>Launch with</span><select name="launch"><option value="" ${!f.locale && !f.useWrapper ? 'selected' : ''}>System locale (normal)</option><option value="ja" ${f.locale === 'ja' ? 'selected' : ''}>Japanese locale</option><option value="custom" ${f.locale !== 'ja' && f.useWrapper ? 'selected' : ''}>Custom launcher program (Settings)</option></select></label>
    <label class="chk" style="margin-top:10px"><input type="checkbox" name="nsfw" ${f.nsfw ? 'checked' : ''}>Blur this game’s cover (NSFW)</label></div>
    <label class="f"><span>Save-file location (folder or file) — for backups</span><div class="pathrow"><input type="text" name="savePath" value="${f.savePath}"><button type="button" class="btn" data-x="pick-save">Folder…</button></div></label>
    <div class="two"><label class="f"><span>Status</span><select name="status">${Object.entries(STATUS).map(([k, l]) => h`<option value="${k}" ${f.status === k ? 'selected' : ''}>${l}</option>`)}</select></label>
    <label class="f"><span>Tags (comma separated)</span><input type="text" name="tags" value="${f.tags.join(', ')}"></label></div>
    ${S.lib.categories.length ? h`<div class="f"><span class="tiny muted" style="display:block;margin-bottom:6px">Collections</span><div class="row" style="flex-wrap:wrap;gap:6px 18px">${S.lib.categories.map(c => h`<label class="chk"><input type="checkbox" name="cat" value="${c.id}" ${f.categories.includes(c.id) ? 'checked' : ''}>${c.name}</label>`)}</div></div>` : ''}
    <label class="f"><span>Description</span><textarea name="description">${f.description}</textarea></label>
    <input type="hidden" name="vndbId" value="${f.vndbId}">
    <div class="row" style="justify-content:space-between;margin-top:6px"><div>${g ? h`<button type="button" class="btn danger" data-x="del">${icon('trash')}Delete game</button>` : ''}</div><div class="row"><button type="button" class="btn" data-x="cancel">Cancel</button><button class="btn primary" type="submit">${isNew ? 'Add to library' : 'Save changes'}</button></div></div>
  </form>`);
  const form = $('#gf', dlg.body), F = n => form.elements[n];
  const cvp = $('#cvp', dlg.body), syncBlur = () => cvp.classList.toggle('nsfwc', form.elements.nsfw.checked);
  form.elements.nsfw.addEventListener('change', syncBlur);
  const preview = src => setHTML($('#cvp', dlg.body), src ? h`<img src="${src}" alt="">` : h`<div class="ph" style="--h:${hue(F('title').value || '?')}">${[...(F('title').value || '?')][0]}</div>`);
  $('#cvf', dlg.body).onchange = e => { const file = e.target.files[0]; if (file) { pend.file = file; pend.url = null; pend.remove = false; preview(URL.createObjectURL(file)); } };
  dlg.body.onclick = guard(async e => {
    const a = e.target.closest('[data-x]'); if (!a) return;
    const x = a.dataset.x;
    if (x === 'up') $('#cvf', dlg.body).click();
    if (x === 'rmcv') { pend.remove = true; pend.file = pend.url = null; preview(null); }
    if (x === 'cancel') g ? dlg.go('overview') : dlg.close();
    if (x === 'pick-exe') { const p = await pickPath({ mode: 'file', exts: '.exe,.bat,.cmd', start: F('exePath').value, title: 'Choose the game’s executable' }); if (p) { F('exePath').value = p; if (!F('title').value.trim()) F('title').value = p.split(/[\\/]/).slice(-2, -1)[0] || ''; } }
    if (x === 'pick-save') { const p = await pickPath({ mode: 'dir', start: F('savePath').value, title: 'Choose the save folder' }); if (p) F('savePath').value = p; }
    if (x === 'vndb') vndbDialog(F('title').value, r => {
      if (!F('title').value.trim()) F('title').value = r.title;
      F('originalTitle').value = r.altTitle || ''; F('developer').value = r.developers.join(', ');
      F('releaseDate').value = r.released || ''; F('description').value = r.description || ''; F('vndbId').value = r.id;
      const have = F('tags').value.split(',').map(s => s.trim()).filter(Boolean);
      F('tags').value = [...new Set([...have, ...r.tags])].join(', ');
      if (r.image) { pend.url = r.image; pend.file = null; pend.remove = false; preview(r.image); }
    });
    if (x === 'del') {
      if (await confirmBox('Delete this game?', `“${g.title}” will be removed from ShioriBox, along with its playtime history and save backups. The game’s own files are not touched.`)) {
        await api('DELETE', `/api/games/${g.id}`); dlg.close(); await load(); toast('Game deleted');
      }
    }
  });
  form.onsubmit = guard(async e => {
    e.preventDefault();
    const body = {
      title: F('title').value.trim(), originalTitle: F('originalTitle').value.trim(), developer: F('developer').value.trim(), releaseDate: F('releaseDate').value.trim(),
      description: F('description').value, exePath: F('exePath').value.trim(), launchArgs: F('launchArgs').value.trim(), useWrapper: F('launch').value === 'custom', nsfw: form.elements.nsfw.checked, locale: F('launch').value === 'ja' ? 'ja' : '', savePath: F('savePath').value.trim(),
      status: F('status').value, tags: F('tags').value.split(',').map(s => s.trim()).filter(Boolean), vndbId: F('vndbId').value,
      categories: $$('input[name=cat]:checked', form).map(i => i.value),
      rating: g ? g.rating : 0, notes: g ? g.notes : '', progress: g ? g.progress || '' : '', favorite: g ? g.favorite : false,
    };
    const saved = g ? await api('PUT', `/api/games/${g.id}`, body) : await api('POST', '/api/games', body);
    try {
      if (pend.file) await api('POST', `/api/games/${saved.id}/cover`, undefined, pend.file);
      else if (pend.url) await api('POST', `/api/games/${saved.id}/cover-from-url`, { url: pend.url });
      else if (pend.remove) await api('DELETE', `/api/games/${saved.id}/cover`);
    } catch (err) { toast('Saved, but the cover failed: ' + err.message, 'err'); }
    await load(); toast(isNew ? 'Added to your library' : 'Saved', 'ok');
    if (isNew) { dlg.close(); openGame(saved.id); } else dlg.go('overview');
  });
}

/* ---------- VNDB search dialog ---------- */
function vndbDialog(q, onPick) {
  const d = openDialog({ title: 'Find on VNDB', body: h`<div class="row"><input type="text" id="vq" value="${q}" placeholder="Title, or a VNDB id like v17"><button class="btn primary" data-x="s">Search</button></div><div id="vr" style="margin-top:8px"></div>` });
  const results = [];
  const run = guard(async () => {
    const term = $('#vq', d.el).value.trim(); if (!term) return;
    setHTML($('#vr', d.el), h`<p class="muted">Searching…</p>`);
    try {
      const r = await api('GET', '/api/vndb/search?q=' + encodeURIComponent(term)); results.splice(0, results.length, ...r);
      setHTML($('#vr', d.el), r.length ? h`${r.map((v, i) => h`<div class="vn">${v.image ? h`<img src="${v.image}" alt="" loading="lazy" referrerpolicy="no-referrer">` : h`<div class="ph" style="--h:${hue(v.title)}">${[...v.title][0]}</div>`}
        <div style="min-width:0"><div style="font-family:var(--serif);font-size:15px">${v.title}</div><div class="tiny muted">${[v.altTitle, (v.released || '').slice(0, 4), v.developers.join(', ')].filter(Boolean).join(' · ')}</div><div class="tiny muted" style="margin-top:3px">${v.rating ? '★ ' + (v.rating / 10).toFixed(1) + ' on VNDB · ' : ''}${v.id}</div></div>
        <button class="btn sm primary" data-x="use" data-i="${i}">Use this</button></div>`)}` : h`<p class="muted">No results. Try a different spelling, or the Japanese title.</p>`);
    } catch (e) { setHTML($('#vr', d.el), h`<p style="color:var(--bad)">${e.message}</p><p class="muted tiny">VNDB needs an internet connection. You can still fill the details in by hand.</p>`); }
  });
  d.el.addEventListener('click', e => { const b = e.target.closest('[data-x]'); if (!b) return; if (b.dataset.x === 's') run(); if (b.dataset.x === 'use') { onPick(results[+b.dataset.i]); d.close(); } });
  $('#vq', d.el).addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); run(); } });
  if (q) run();
}

/* ---------- scan + import ---------- */
function scanDialog() {
  let items = [];
  const d = openDialog({ title: 'Scan a folder for games', body: h`<p class="muted" style="margin-top:0">Choose the folder that contains your visual novels (or a single game’s folder). ShioriBox searches inside it, including nested folders like Developer\\Game or Game\\bin, and picks the most likely executable for each game.</p>
    <div class="pathrow"><input type="text" id="sp" placeholder="D:\\Games\\VisualNovels"><button class="btn" data-x="browse">Browse…</button><button class="btn primary" data-x="scan">Scan</button></div><div id="sr" style="margin-top:16px"></div>` });
  const run = guard(async () => {
    const p = $('#sp', d.el).value.trim(); if (!p) return;
    setHTML($('#sr', d.el), h`<p class="muted">Scanning…</p>`);
    let res; try { res = await api('POST', '/api/scan', { path: p }); } catch (e) { setHTML($('#sr', d.el), h`<p style="color:var(--bad)">${e.message}</p>`); return; }
    items = res.items;
    if (!items.length) return setHTML($('#sr', d.el), h`<p style="margin:0 0 6px"><b>No games found.</b></p><p class="muted" style="margin:0">ShioriBox looked in ${res.dirs} folder${res.dirs === 1 ? '' : 's'} under <code>${res.root}</code> and saw ${res.exes} .exe file${res.exes === 1 ? '' : 's'}${res.skipped.length ? h` (ignored as installers/tools: ${res.skipped.join(', ')})` : ''}. ${res.exes === 0 ? 'If your games are archives (zip/rar/7z) or only contain other file types, extract them first, or add them one by one with “Add game”.' : ''}</p>`);
    setHTML($('#sr', d.el), h`<p class="muted tiny" style="margin:0 0 8px">Found ${items.length} game${items.length === 1 ? '' : 's'} in ${res.dirs} folders. Untick any you don’t want.</p>`);
    $('#sr', d.el).insertAdjacentHTML('beforeend', '<div id="sres"></div>');
    setHTML($('#sres', d.el), h`<table class="t" id="stbl"><tr><th><input type="checkbox" id="selall" checked aria-label="Select all"></th><th>Title</th><th>Executable</th></tr>${items.map((it, i) => h`<tr data-i="${i}"><td><input type="checkbox" class="sel" ${it.exists ? 'disabled' : 'checked'} aria-label="Import ${it.title}"></td>
      <td><input type="text" class="ttl" value="${it.title}" ${it.exists ? 'disabled' : ''}></td>
      <td>${it.exists ? h`<span class="tag">already in library</span>` : h`<select class="exe">${it.exes.map(x => h`<option value="${x.path}" ${x.path === it.chosen ? 'selected' : ''}>${x.path.slice(it.dir.length + 1)}</option>`)}</select>`}</td></tr>`)}</table>
      <div class="row" style="margin-top:16px;justify-content:space-between;flex-wrap:wrap"><div><label class="chk"><input type="checkbox" id="auto" checked>Look up details &amp; covers on VNDB afterwards</label><label class="chk" style="margin-top:8px"><input type="checkbox" id="jp" ${S.lib.settings.defaultLocale === 'ja' ? 'checked' : ''}>Launch these games with Japanese locale</label></div><button class="btn primary" data-x="imp">Import selected</button></div>`);
    $('#selall', d.el).addEventListener('change', e => $$('#stbl .sel:not(:disabled)', d.el).forEach(c => { c.checked = e.target.checked; }));
  });
  d.el.addEventListener('click', guard(async e => {
    const b = e.target.closest('[data-x]'); if (!b) return;
    if (b.dataset.x === 'browse') { const p = await pickPath({ mode: 'dir', start: $('#sp', d.el).value, title: 'Choose your games folder' }); if (p) { $('#sp', d.el).value = p; run(); } }
    if (b.dataset.x === 'scan') run();
    if (b.dataset.x === 'imp') {
      const chosen = $$('#stbl tr[data-i]', d.el).filter(tr => $('.sel', tr).checked && !$('.sel', tr).disabled).map(tr => ({ title: $('.ttl', tr).value.trim(), exePath: $('.exe', tr).value, locale: $('#jp', d.el).checked ? 'ja' : '' }));
      if (!chosen.length) return toast('Nothing selected', 'err');
      const auto = $('#auto', d.el).checked;
      const created = await api('POST', '/api/import', { items: chosen });
      await load(); d.close(); toast(`Imported ${created.length} game${created.length === 1 ? '' : 's'}`, 'ok');
      if (auto) enrichAll(created);
    }
  }));
  $('#sp', d.el).addEventListener('keydown', e => { if (e.key === 'Enter') run(); });
}

async function enrichAll(created) {
  const note = document.createElement('div'); note.className = 'toast'; $('#toasts').append(note);
  let ok = 0;
  for (let i = 0; i < created.length; i++) {
    const g = created[i]; note.textContent = `Looking up details… ${i + 1}/${created.length}`;
    try {
      const r = (await api('GET', '/api/vndb/search?q=' + encodeURIComponent(g.title)))[0]; if (!r) continue;
      await api('PUT', `/api/games/${g.id}`, payload(game(g.id) || g, { originalTitle: r.altTitle || '', developer: r.developers.join(', '), releaseDate: r.released || '', description: r.description || '', vndbId: r.id, tags: r.tags }));
      if (r.image) await api('POST', `/api/games/${g.id}/cover-from-url`, { url: r.image }).catch(() => { });
      ok++;
    } catch (e) { note.textContent = 'Lookup stopped: ' + e.message; note.classList.add('err'); setTimeout(() => note.remove(), 6000); await load(); return; }
    if (i % 3 === 2) await load();
  }
  note.remove(); await load(); toast(`Found details for ${ok} of ${created.length} games`, ok ? 'ok' : '');
}

/* ---------- actions ---------- */
const acts = {
  'dc-go': () => dcSearch(false),
  'dc-back': () => dcBack(),
  'dc-more': () => dcSearch(true),
  'dc-addtag': el => { S.dc.tags.push({ id: el.dataset.id, name: el.dataset.name }); S.dc.sug = []; $('#dc-tag').value = ''; dcChips(); dcSug(); dcSearch(false); },
  'dc-untag': el => { S.dc.tags = S.dc.tags.filter(t => t.id !== el.dataset.id); dcChips(); dcSearch(false); },
  'dc-add': guard(async el => { el.disabled = true; await dcAdd(el.dataset.id); dcFill(); }),
  'dc-detail': el => dcDetail(el.dataset.id),
  'dc-prod-clear': () => { S.dc.producer = null; dcPSugList = []; if ($('#dc-q')) { dcRead(); render(); } dcSearch(false); },
  'dc-pick-prod': el => { S.dc.producer = { id: el.dataset.id, name: el.dataset.name }; dcPSugList = []; dcRead(); render(); dcSearch(false); },
  'dc-mode': el => { S.dc.mode = el.dataset.m; render(); },
  'dc-rec-refresh': () => { S.dc.rec = null; S.dc.recErr = ''; dcRecLoad(); },
  'dc-open': guard(async el => { await api('POST', '/api/open-url', { url: 'https://vndb.org/' + el.dataset.id }); }),
  'home-all': el => {
    const k = el.dataset.k; S.view = 'library'; S.cat = ''; S.fav = false; S.status = 'all'; S.q = '';
    if (k === 'played') S.sort = 'lastPlayed'; else if (k === 'added') S.sort = 'added'; else if (k === 'fav') S.fav = true; else if (k === 'planned') S.status = 'planned';
    try { localStorage.setItem('ns.sort', S.sort); } catch { }
    render();
  },
  'open-guides': el => openGame(el.dataset.id, 'guides'),
  'stat-range': el => { S.statRange = el.dataset.r; render(); },
  nav: el => { S.view = el.dataset.v; S.cat = ''; S.fav = false; render(); },
  'nav-fav': () => { S.view = 'library'; S.fav = true; S.cat = ''; render(); },
  'nav-cat': el => { S.view = 'library'; S.cat = el.dataset.id; S.fav = false; render(); },
  layout: el => { S.layout = el.dataset.v; localStorage.setItem('ns.layout', S.layout); render(); },
  status: el => { S.status = el.dataset.v; render(); },
  open: el => openGame(el.dataset.id),
  selmode: () => { S.sel = S.sel ? null : new Set(); render(); },
  toggle: el => { const id = el.dataset.id; S.sel.has(id) ? S.sel.delete(id) : S.sel.add(id); rerenderResults(); },
  'sel-all': () => { filtered().forEach(g => S.sel.add(g.id)); rerenderResults(); },
  'sel-none': () => { S.sel.clear(); rerenderResults(); },
  'bulk-nsfw-on': guard(async () => bulkNsfw(true)),
  'bulk-nsfw-off': guard(async () => bulkNsfw(false)),
  'bulk-ja-on': guard(async () => bulkLocale('ja')),
  'bulk-ja-off': guard(async () => bulkLocale('')),
  'bulk-del': guard(async () => {
    const ids = [...S.sel]; if (!ids.length) return;
    const names = ids.slice(0, 3).map(id => '“' + (game(id) || {}).title + '”').join(', ') + (ids.length > 3 ? ` and ${ids.length - 3} more` : '');
    if (!await confirmBox(`Remove ${ids.length} game${ids.length === 1 ? '' : 's'}?`, `${names} will be removed from ShioriBox, together with their playtime history. The game files on your PC are not touched.`)) return;
    await api('POST', '/api/snapshots?note=bulk').catch(() => { });
    let bad = 0; for (const id of ids) { try { await api('DELETE', `/api/games/${id}`); } catch { bad++; } }
    S.sel = null; await load(); toast(bad ? `Removed ${ids.length - bad}; ${bad} could not be removed (still running?)` : `Removed ${ids.length}`, bad ? 'err' : 'ok');
  }),
  add: () => openGame(null),
  scan: () => scanDialog(),
  launch: guard(async el => { await api('POST', `/api/games/${el.dataset.id}/launch`); S.running[el.dataset.id] = { start: new Date().toISOString(), active: 0, paused: true, at: Date.now() }; await load(); toast('Launched', 'ok'); }),
  stop: guard(async el => { await api('POST', `/api/games/${el.dataset.id}/stop`); toast('Stopping…'); setTimeout(reload, 800); }),
  'dlg-close': el => el.closest('dialog').close(),
  'cat-new': guard(async () => { const n = await promptBox('New collection', 'Name'); if (n) { await api('POST', '/api/categories', { name: n }); await load(); } }),
  'cat-rename': guard(async el => { const c = S.lib.categories.find(c => c.id === el.dataset.id); const n = await promptBox('Rename collection', 'Name', c.name); if (n) { await api('PUT', `/api/categories/${c.id}`, { name: n }); await load(); } }),
  'cat-del': guard(async el => { const c = S.lib.categories.find(c => c.id === el.dataset.id); if (await confirmBox('Delete collection?', `“${c.name}” will be removed. The games in it stay in your library.`)) { await api('DELETE', `/api/categories/${c.id}`); if (S.cat === c.id) S.cat = ''; await load(); } }),
  'get-le': guard(async () => { await api('POST', '/api/open-url', { url: 'https://github.com/xupefei/Locale-Emulator/releases' }); }),
  'open-region': guard(async () => { await api('POST', '/api/open-region'); }),
  'pick-lp': guard(async () => { const p = await pickPath({ mode: 'file', start: $('#s-lp').value, title: 'Choose your locale emulator program' }); if (p) $('#s-lp').value = p; }),
  'pick-le': guard(async () => { const p = await pickPath({ mode: 'dir', start: $('#s-le').value, title: 'Choose the Locale Emulator folder (contains LEProc.exe)' }); if (p) $('#s-le').value = p; }),
  'pick-wrapper': guard(async () => { const p = await pickPath({ mode: 'file', exts: '.exe,.bat,.cmd', start: $('#s-wp').value, title: 'Choose the launcher program' }); if (p) $('#s-wp').value = p; }),
  'save-settings': guard(async () => {
    await api('PUT', '/api/settings', { revealOnHover: $('#s-reveal').checked, localeEngine: 'jle', localePath: $('#s-lp') ? $('#s-lp').value.trim() : st0().localePath, localeArgs: $('#s-la') ? $('#s-la').value.trim() : st0().localeArgs, defaultLocale: $('#s-dloc').value, theme: $('#s-theme').value, wrapperPath: $('#s-wp').value.trim(), wrapperArgs: $('#s-wa').value.trim() || '-run {exe}', autoBackup: $('#s-auto').checked, backupKeep: +$('#s-keep').value || 0, minSessionSec: +$('#s-min').value || 5 });
    await load(); toast('Settings saved', 'ok');
  }),
  'snap-now': guard(async () => { await api('POST', '/api/snapshots'); toast('Library backed up', 'ok'); loadSnaps(); }),
  'snap-open': guard(async () => { await api('POST', '/api/open-snapshots'); }),
  'snap-del': guard(async el => { await api('DELETE', `/api/snapshots/${el.dataset.n}`); loadSnaps(); }),
  'snap-restore': guard(async el => { if (await confirmBox('Restore this snapshot?', 'Your library list goes back to how it was then. Your current state is saved as a snapshot first, so you can undo this.', 'Restore', false)) { await api('POST', `/api/snapshots/${el.dataset.n}/restore`); await load(); toast('Snapshot restored', 'ok'); } }),
  'bg-pick': () => $('#bgfile').click(),
  'bg-remove': guard(async () => { await api('DELETE', '/api/background'); await load(); toast('Background removed', 'ok'); }),
  'bg-reset': guard(async () => { Object.assign(S.lib.settings, { bgBlur: 6, bgOverlay: 55, bgPanel: 80, bgPos: 'center' }); await bgSave(); render(); }),
  'import-lib': () => $('#libfile').click(),
  quit: guard(async () => {
    const n = Object.keys(S.running).length;
    if (await confirmBox('Quit ShioriBox?', n ? `${n} game${n > 1 ? 's are' : ' is'} still running — their playtime will not be recorded if you quit now.` : 'ShioriBox will close. Open it again any time from ShioriBox.exe.', 'Quit', n > 0)) {
      await api('POST', '/api/quit').catch(() => { });
      document.body.innerHTML = '<div style="display:grid;place-items:center;height:100vh;font:16px system-ui;color:#999">ShioriBox has closed. You can close this window.</div>';
    }
  }),
};

document.addEventListener('click', e => {
  const el = e.target.closest('[data-act]'); if (!el) return;
  if (el.closest('button[data-act="launch"],button[data-act="stop"]') && el.dataset.act !== 'open') e.stopPropagation();
  const f = acts[el.dataset.act]; if (f) { if (['launch', 'stop'].includes(el.dataset.act)) e.stopPropagation(); f(el, e); }
}, true);
document.addEventListener('click', e => {
  const a = e.target.closest('a[target=_blank]'); if (!a) return;
  e.preventDefault(); api('POST', '/api/open-url', { url: a.href }).catch(err => toast(err.message, 'err'));
});
let bgTimer;
function bgSave() {
  const s = S.lib.settings;
  return api('PUT', '/api/background/settings', { blur: s.bgBlur, overlay: s.bgOverlay, panel: s.bgPanel, pos: s.bgPos }).catch(e => toast(e.message, 'err'));
}
document.addEventListener('input', e => {
  const m = { 'bg-blur': ['bgBlur', 'o-blur', 'px'], 'bg-ov': ['bgOverlay', 'o-ov', '%'], 'bg-pn': ['bgPanel', 'o-pn', '%'] }[e.target.id];
  if (!m) return;
  S.lib.settings[m[0]] = +e.target.value; $('#' + m[1]).textContent = e.target.value + m[2];
  applyBg(S.lib.settings); clearTimeout(bgTimer); bgTimer = setTimeout(bgSave, 400);
});
document.addEventListener('click', e => {
  const b = e.target.closest('[data-cg="link"]'); if (!b) return;
  api('POST', '/api/open-url', { url: b.dataset.url }).catch(err => toast(err.message, 'err'));
});
document.addEventListener('change', async e => {
  if (e.target.id === 'dc-blur') {
    S.dc.blur = e.target.value; try { localStorage.setItem('ns.dcblur', S.dc.blur); } catch { }
    const box = $('#dc-res'); if (box) box.className = 'dcb-' + S.dc.blur;
  }
  if (e.target.id === 'bg-pos') { S.lib.settings.bgPos = e.target.value; applyBg(S.lib.settings); bgSave(); }
  if (e.target.id === 'bgfile' && e.target.files[0]) {
    const f = e.target.files[0]; e.target.value = '';
    try { await api('POST', '/api/background', undefined, f); await load(); toast('Background set', 'ok'); } catch (err) { toast(err.message, 'err'); }
  }
});
document.addEventListener('change', e => { if (['dc-plat', 'dc-lang', 'dc-olang', 'dc-sort', 'dc-rev'].includes(e.target.id)) dcSearch(false); });
document.addEventListener('input', e => { if (e.target.id === 'dc-tag') dcTagInput(e.target.value); if (e.target.id === 'dc-prod') dcProdInput(e.target.value); });
document.addEventListener('keydown', e => {
  if (e.key === 'Enter' && e.target.id === 'dc-q') { e.preventDefault(); dcSearch(false); }
  if (e.key === '/' && !/INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName) && !$('dialog[open]')) { const q = $('#q'); if (q) { e.preventDefault(); q.focus(); } }
  if ((e.key === 'Enter') && document.activeElement.classList.contains('card')) document.activeElement.click();
});
document.addEventListener('input', e => { if (e.target.id === 'q') { S.q = e.target.value; if (S.view === 'library') rerenderResults(); else { S.view = 'library'; S.cat = ''; S.fav = false; render(); } } });
document.addEventListener('change', guard(async e => {
  if (e.target.id === 'bulk-status' && e.target.value) {
    const v = e.target.value, ids = [...S.sel];
    for (const id of ids) await api('PUT', `/api/games/${id}`, payload(game(id), { status: v }));
    await load(); toast(`Marked ${ids.length} as ${STATUS[v]}`, 'ok');
  }
  if (e.target.id === 'bulk-cat' && e.target.value) {
    const [op, cid] = e.target.value.split(':'), ids = [...S.sel], cname = (S.lib.categories.find(c => c.id === cid) || {}).name;
    for (const id of ids) {
      const cur = game(id).categories || [];
      const next = op === 'add' ? [...new Set([...cur, cid])] : cur.filter(x => x !== cid);
      await api('PUT', `/api/games/${id}`, payload(game(id), { categories: next }));
    }
    await load(); toast(`${op === 'add' ? 'Added' : 'Removed'} ${ids.length} game${ids.length === 1 ? '' : 's'} ${op === 'add' ? 'to' : 'from'} “${cname}”`, 'ok');
  }
  if (e.target.id === 'tagsel') { S.tag = e.target.value; render(); }
  if (e.target.id === 'sortsel') { S.sort = e.target.value; localStorage.setItem('ns.sort', S.sort); render(); }
  if (e.target.id === 'libfile') {
    const f = e.target.files[0]; if (!f) return;
    if (await confirmBox('Replace your library?', 'This replaces the current library with the one in the zip. Your current library.json is kept as a safety copy in the data folder.', 'Import', false)) {
      await api('POST', '/api/import-library', undefined, f); await load(); toast('Library imported', 'ok');
    }
    e.target.value = '';
  }
}));

/* keep open game dialogs in sync after data reloads */
const _render = render;
render = function () { _render(); $$('dialog[data-gid]').forEach(d => { if (d.dataset.gid && d._dlg && d._dlg.cur() !== 'edit') d._dlg.rerender(); }); };
const _openGame = openGame;
openGame = function (id, tab) { const dlg = _openGame(id, tab); dlg.el._dlg = dlg; return dlg; };

/* poll for running games so playtime shows up when a session ends */
setInterval(async () => {
  try {
    const r = await api('GET', '/api/running');
    if (Object.keys(r).sort().join() !== Object.keys(S.running).sort().join()) { await load(); }
    else S.running = stampRun(r);
  } catch (e) { /* server gone */ }
}, 1000);

reload();
