/* ------------------------------------------------------------------ *
 * Lingle - browser client.
 *
 * The server owns the answer, the word list, the try limit and hard-mode
 * enforcement. This file owns nothing but presentation: it collects a
 * guess, posts it, and paints whatever comes back.
 *
 * Local storage is used only for per-browser display preferences. Game
 * progress and statistics live on the server, keyed to an opaque cookie.
 * ------------------------------------------------------------------ */
'use strict';

const PREFS_KEY = 'lingle.prefs';

/* ------------------------------ preferences ----------------------- */
function loadPrefs() {
  try {
    return Object.assign(
      { theme: 'dark', colorblind: false, lang: null, seenHelp: false },
      JSON.parse(localStorage.getItem(PREFS_KEY) || '{}'));
  } catch (e) {
    return { theme: 'dark', colorblind: false, lang: null, seenHelp: false };
  }
}
function savePrefs() {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch (e) { /* private mode - the game still plays */ }
}

/* -------------------------------- state --------------------------- */
let config = null;      // { title, defaultLang, languages: [meta] }
let meta = null;        // the active language's metadata
let S = null;           // the active language's UI strings
let game = null;        // the server's view of today's game
let current = '';       // the row being typed
let busy = false;
let prefs = loadPrefs();

const $ = id => document.getElementById(id);

/* --------------------------------- API ---------------------------- */
class ApiError extends Error {
  constructor(message, offline) {
    super(message);
    this.offline = !!offline;
  }
}

async function api(path, body) {
  let res;
  try {
    res = await fetch(path, {
      method: body === undefined ? 'GET' : 'POST',
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: 'same-origin',
      cache: 'no-store',
    });
  } catch (e) {
    throw new ApiError('network', true);
  }
  if (!res.ok) {
    let reason = 'http_' + res.status;
    try {
      reason = (await res.json()).error || reason;
    } catch (e) { /* not JSON */ }
    throw new ApiError(reason, false);
  }
  return res.json();
}

const API = {
  config: () => api('/api/config'),
  game: (lang, hardMode) =>
    api('/api/game', hardMode === undefined ? { lang } : { lang, hardMode }),
  guess: (lang, guess) => api('/api/guess', { lang, guess }),
  stats: lang => api('/api/stats?lang=' + encodeURIComponent(lang)),
};

function apiToast(err) {
  if (!S) return;
  toast(err.offline ? S.offline : S.serverError, 2400);
}

/* ------------------------------ normalising ----------------------- */
// Mirrors the server: fold the letters a language treats as equivalent
// (Russian ё onto е) so the tile shows what will actually be submitted.
function fold(text) {
  let out = text.toLowerCase();
  for (const [from, to] of Object.entries(meta.normalize || {})) {
    out = out.split(from).join(to);
  }
  return out;
}

/* ------------------------------- toasts --------------------------- */
function toast(message, ms = 1600) {
  if (!message) return;
  const el = document.createElement('div');
  el.className = 'toast';
  el.textContent = message;
  $('toaster').appendChild(el);
  setTimeout(() => {
    el.classList.add('out');
    setTimeout(() => el.remove(), 300);
  }, ms);
}

/* ------------------------------ rendering ------------------------- */
function buildBoard() {
  const board = $('board');
  board.innerHTML = '';
  board.style.setProperty('--cols', meta.length);
  for (let r = 0; r < meta.tries; r++) {
    const row = document.createElement('div');
    row.className = 'row';
    row.style.setProperty('--cols', meta.length);
    row.setAttribute('role', 'row');
    for (let c = 0; c < meta.length; c++) {
      const tile = document.createElement('div');
      tile.className = 'tile';
      tile.setAttribute('role', 'gridcell');
      row.appendChild(tile);
    }
    board.appendChild(row);
  }
  sizeBoard();
}

// Keep the board square and inside the available space on every screen.
function sizeBoard() {
  if (!meta) return;
  const wrap = $('board-wrap');
  const gap = 5;
  const availH = wrap.clientHeight - 10;
  const availW = wrap.clientWidth - 10;
  const byH = (availH - gap * (meta.tries - 1)) / meta.tries;
  const byW = (availW - gap * (meta.length - 1)) / meta.length;
  const side = Math.max(28, Math.floor(Math.min(byH, byW)));
  $('board').style.width = (side * meta.length + gap * (meta.length - 1)) + 'px';
}

function buildKeyboard() {
  const kb = $('keyboard');
  kb.innerHTML = '';
  const layout = meta.keyboard;
  const spacer = () => {
    const s = document.createElement('div');
    s.className = 'kspacer';
    return s;
  };
  layout.forEach((letters, idx) => {
    const row = document.createElement('div');
    row.className = 'krow';
    const last = idx === layout.length - 1;
    const pad = idx === 1 && layout[1].length < layout[0].length;
    if (last) row.appendChild(makeKey('enter', true));
    else if (pad) row.appendChild(spacer());
    letters.forEach(ch => row.appendChild(makeKey(ch, false)));
    if (last) row.appendChild(makeKey('backspace', true));
    else if (pad) row.appendChild(spacer());
    kb.appendChild(row);
  });
}

function makeKey(value, wide) {
  const b = document.createElement('button');
  b.className = 'key' + (wide ? ' wide' : '');
  b.dataset.key = value;
  b.type = 'button';
  if (value === 'enter') {
    b.textContent = S.enterKey || 'ENTER';
    b.setAttribute('aria-label', 'Enter');
  } else if (value === 'backspace') {
    b.innerHTML = '<svg viewBox="0 0 24 24"><path d="M22 3H7L0 12l7 9h15a2 2 0 002-2V5a2 2 0 00-2-2zm-3.3 12.3-1.4 1.4L14 13.4l-3.3 3.3-1.4-1.4L12.6 12 9.3 8.7l1.4-1.4L14 10.6l3.3-3.3 1.4 1.4L15.4 12z"/></svg>';
    b.setAttribute('aria-label', 'Backspace');
  } else {
    b.textContent = value;
  }
  b.addEventListener('click', () => { b.blur(); press(value); });
  return b;
}

function paintRow(index, animate) {
  const row = $('board').children[index];
  const { guess, marks } = game.rows[index];
  const letters = [...guess];
  for (let i = 0; i < letters.length; i++) {
    const tile = row.children[i];
    tile.textContent = letters[i];
    tile.dataset.filled = '1';
    if (!animate) {
      tile.dataset.state = marks[i];
      continue;
    }
    setTimeout(() => {
      tile.classList.add('flip');
      setTimeout(() => { tile.dataset.state = marks[i]; }, 250);
    }, i * 300);
  }
}

function paintCurrent() {
  const row = $('board').children[game.rows.length];
  if (!row) return;
  const letters = [...current];
  for (let i = 0; i < meta.length; i++) {
    const tile = row.children[i];
    const ch = letters[i] || '';
    if (tile.textContent !== ch) {
      tile.textContent = ch;
      tile.dataset.filled = ch ? '1' : '';
    }
  }
}

const RANK = { absent: 0, present: 1, correct: 2 };

function paintKeyboard() {
  const best = Object.create(null);
  game.rows.forEach(({ guess, marks }) => {
    [...guess].forEach((ch, i) => {
      if (best[ch] === undefined || RANK[marks[i]] > RANK[best[ch]]) best[ch] = marks[i];
    });
  });
  document.querySelectorAll('.key[data-key]').forEach(k => {
    const ch = k.dataset.key;
    if (ch.length > 1) return;
    if (best[ch]) k.dataset.state = best[ch];
    else delete k.dataset.state;
  });
}

function renderGame(animateLast) {
  const shown = animateLast ? game.rows.length - 1 : game.rows.length;
  for (let i = 0; i < shown; i++) paintRow(i, false);
  if (animateLast) paintRow(game.rows.length - 1, true);
  paintCurrent();
  if (!animateLast) paintKeyboard();
}

/* ------------------------------ gameplay -------------------------- */
function press(key) {
  if (busy || !game || game.status !== 'playing') return;
  if (key === 'enter') return submit();
  if (key === 'backspace') {
    current = [...current].slice(0, -1).join('');
    return paintCurrent();
  }
  if ([...current].length >= meta.length) return;
  current += key;
  paintCurrent();
}

const REJECTION = {
  not_in_list: () => S.notInList,
  bad_length: () => S.notEnough,
  hard_mode: () => S.hardModeViolation,
  finished: () => null,
};

async function submit() {
  if ([...current].length < meta.length) return reject(S.notEnough);

  busy = true;
  let res;
  try {
    res = await API.guess(meta.code, current);
  } catch (err) {
    busy = false;
    apiToast(err);
    return;
  }

  if (!res.accepted) {
    busy = false;
    game = res.game;
    const message = (REJECTION[res.reason] || (() => S.serverError))();
    return reject(message, res.reason === 'hard_mode' ? 2200 : undefined);
  }

  current = '';
  game = res.game;
  renderGame(true);

  const revealMs = meta.length * 300 + 260;
  setTimeout(() => {
    paintKeyboard();
    busy = false;
    if (game.status === 'won') {
      $('board').children[game.rows.length - 1].classList.add('win');
      toast(S.won[Math.min(game.rows.length - 1, S.won.length - 1)]);
      setTimeout(openStats, 1900);
    } else if (game.status === 'lost') {
      toast(`${S.lost} ${(game.answer || '').toUpperCase()}`, 3000);
      setTimeout(openStats, 2200);
    }
  }, revealMs);
}

function reject(message, ms) {
  toast(message, ms);
  const row = $('board').children[game.rows.length];
  if (!row) return;
  row.classList.add('shake');
  setTimeout(() => row.classList.remove('shake'), 620);
}

/* -------------------------------- stats --------------------------- */
async function openStats() {
  let stats;
  try {
    stats = await API.stats(meta.code);
  } catch (err) {
    return apiToast(err);
  }

  const pct = stats.played ? Math.round(stats.wins / stats.played * 100) : 0;
  $('stat-numbers').innerHTML = [
    [stats.played, S.played],
    [pct, S.winPct],
    [stats.streak, S.streak],
    [stats.maxStreak, S.maxStreak],
  ].map(([v, k]) => `<div class="stat"><div class="v">${v}</div><div class="k">${k}</div></div>`).join('');

  // Built with DOM calls rather than an HTML string: the bar widths are
  // dynamic, and the page's Content-Security-Policy forbids style attributes.
  const dist = stats.dist || [];
  const peak = Math.max(1, ...dist);
  const distNode = $('dist');
  distNode.innerHTML = '';
  dist.forEach((n, i) => {
    const line = document.createElement('div');
    line.className = 'bar-line';

    const label = document.createElement('div');
    label.className = 'n';
    label.textContent = String(i + 1);

    const wrap = document.createElement('div');
    wrap.className = 'bar-wrap';

    const bar = document.createElement('div');
    bar.className = 'bar';
    if (game.status === 'won' && game.rows.length === i + 1) bar.classList.add('cur');
    bar.style.width = Math.max(7, Math.round(n / peak * 100)) + '%';
    bar.textContent = String(n);

    wrap.appendChild(bar);
    line.append(label, wrap);
    distNode.appendChild(line);
  });

  renderWordCard();

  $('btn-share').style.display = game.status === 'playing' ? 'none' : '';
  openModal('modal-stats');
  tickCountdown();
}

// Once the puzzle is over the server sends the word and, where the pack has
// one, its dictionary entry - the bit that makes this useful for a language
// you are still learning.
function renderWordCard() {
  const card = $('word-card');
  if (game.status === 'playing' || !game.answer) {
    card.hidden = true;
    return;
  }
  const def = game.definition || null;

  $('wc-word').textContent = (def && def.accented) || game.answer;

  const bits = [];
  if (def && def.pos) bits.push((S.pos && S.pos[def.pos]) || def.pos);
  if (def && def.note) bits.push((S.note && S.note[def.note]) || def.note);
  $('wc-meta').textContent = bits.join(' · ');

  $('wc-heading').textContent = def && def.gloss ? S.meaning : '';
  $('wc-gloss').textContent = (def && def.gloss) || '';

  card.hidden = false;
}

let countdownTimer = null;
function tickCountdown() {
  clearInterval(countdownTimer);
  const paint = () => {
    const ms = game.nextRollover - Date.now();
    if (ms <= 0) return location.reload();
    const h = Math.floor(ms / 3600000);
    const m = Math.floor(ms % 3600000 / 60000);
    const s = Math.floor(ms % 60000 / 1000);
    $('next-time').textContent = [h, m, s].map(v => String(v).padStart(2, '0')).join(':');
  };
  paint();
  countdownTimer = setInterval(paint, 1000);
}

/* -------------------------------- share --------------------------- */
function shareText() {
  const glyph = {
    correct: prefs.colorblind ? '🟧' : '🟩',
    present: prefs.colorblind ? '🟦' : '🟨',
    absent: document.documentElement.dataset.theme === 'light' ? '⬜' : '⬛',
  };
  const score = game.status === 'won' ? game.rows.length : 'X';
  const head = `${(config && config.title) || S.title} ${meta.code.toUpperCase()} ` +
               `#${game.puzzle} ${score}/${game.tries}` + (game.hardMode ? '*' : '');
  const grid = game.rows.map(r => r.marks.map(m => glyph[m]).join('')).join('\n');
  return `${head}\n\n${grid}`;
}

async function doShare() {
  const text = shareText();
  try {
    if (navigator.share && /Android|iPhone|iPad/i.test(navigator.userAgent)) {
      await navigator.share({ text });
      return;
    }
    await navigator.clipboard.writeText(text);
    toast(S.copied);
  } catch (e) {
    // The clipboard API needs a secure context; plain http on a LAN is not one.
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand('copy');
      toast(S.copied);
    } catch (e2) {
      toast(S.copyFailed);
    }
    ta.remove();
  }
}

/* -------------------------------- modals -------------------------- */
function openModal(id) { $(id).hidden = false; }
function closeModal(id) {
  $(id).hidden = true;
  if (id === 'modal-stats') clearInterval(countdownTimer);
}

function fillHelp() {
  $('help-title').textContent = S.howToPlay;
  $('help-intro').textContent = S.rulesIntro
    .replace('5', meta.length).replace('6', meta.tries);
  $('help-l1').textContent = S.rulesLine1;
  $('help-l2').textContent = S.rulesLine2;

  const demo = (word, index, state, node) => {
    node.innerHTML = '';
    [...word].forEach((ch, i) => {
      const t = document.createElement('div');
      t.className = 'tile';
      t.textContent = ch;
      t.dataset.filled = '1';
      if (i === index) t.dataset.state = state;
      node.appendChild(t);
    });
  };
  const a = S.exampleCorrect, b = S.exampleAbsent;
  demo(a, 0, 'correct', $('ex-correct'));
  demo(a, 2, 'present', $('ex-present'));
  demo(b, 3, 'absent', $('ex-absent'));
  $('help-correct').innerHTML = S.rulesCorrect.replace('{a}', [...a][0].toUpperCase());
  $('help-present').innerHTML = S.rulesPresent.replace('{a}', [...a][2].toUpperCase());
  $('help-absent').innerHTML = S.rulesAbsent.replace('{a}', [...b][3].toUpperCase());
  $('help-yo').textContent = S.rulesYo || '';
  $('help-yo').hidden = !S.rulesYo;
}

function fillSettings() {
  $('settings-title').textContent = S.settings;
  $('lbl-language').textContent = S.language;
  $('lbl-hard').textContent = S.hardMode;
  $('hint-hard').textContent = S.hardModeHint;
  $('lbl-theme').textContent = S.theme;
  $('lbl-cb').textContent = S.colorblind;
  $('version').textContent = (config.title || S.title);

  $('lang-select').innerHTML = config.languages
    .map(l => `<option value="${l.code}"${l.code === meta.code ? ' selected' : ''}>${l.flag} ${l.name}</option>`)
    .join('');
}

function applyStrings() {
  const title = config.title || S.title;
  $('title').textContent = title;
  document.title = title;
  $('stats-title').textContent = S.stats;
  $('dist-title').textContent = S.distribution;
  $('next-label').textContent = S.nextWord;
  $('share-label').textContent = S.share;
  $('btn-help').setAttribute('aria-label', S.howToPlay);
  $('btn-stats').setAttribute('aria-label', S.stats);
  $('btn-settings').setAttribute('aria-label', S.settings);
  document.documentElement.lang = meta.code;
  document.documentElement.dir = meta.dir || 'ltr';
  fillHelp();
  fillSettings();
}

/* ------------------------------ appearance ------------------------ */
function applyTheme() {
  document.documentElement.dataset.theme = prefs.theme;
  document.documentElement.dataset.colorblind = prefs.colorblind ? 'on' : 'off';
  document.querySelector('meta[name="theme-color"]')
    .setAttribute('content', prefs.theme === 'light' ? '#ffffff' : '#121213');
  $('sw-theme').setAttribute('aria-checked', String(prefs.theme === 'dark'));
  $('sw-cb').setAttribute('aria-checked', String(prefs.colorblind));
  $('sw-hard').setAttribute('aria-checked', String(game ? game.hardMode : false));
}

/* -------------------------------- boot ---------------------------- */
async function selectLanguage(code, hardMode) {
  meta = config.languages.find(l => l.code === code) || config.languages[0];
  S = meta.strings;
  game = await API.game(meta.code, hardMode);
  current = '';

  buildBoard();
  buildKeyboard();
  applyStrings();
  applyTheme();
  renderGame(false);

  prefs.lang = meta.code;
  savePrefs();

  if (game.status !== 'playing') setTimeout(openStats, 250);
}

async function boot() {
  config = await API.config();
  if (!config.languages || !config.languages.length) {
    throw new ApiError('no language packs are installed', false);
  }

  const wanted = new URLSearchParams(location.search).get('lang')
    || prefs.lang
    || config.defaultLang;
  const chosen = config.languages.find(l => l.code === wanted) || config.languages[0];

  await selectLanguage(chosen.code);

  $('loader').hidden = true;
  $('app').hidden = false;
  sizeBoard();

  if (!prefs.seenHelp) {
    prefs.seenHelp = true;
    savePrefs();
    openModal('modal-help');
  }
}

/* ------------------------------- wiring --------------------------- */
document.addEventListener('keydown', e => {
  if (!meta || e.ctrlKey || e.metaKey || e.altKey) return;
  if (document.querySelector('.modal:not([hidden])')) {
    if (e.key === 'Escape') {
      document.querySelectorAll('.modal:not([hidden])').forEach(m => closeModal(m.id));
    }
    return;
  }
  if (e.key === 'Enter') return press('enter');
  if (e.key === 'Backspace') return press('backspace');
  const ch = fold(e.key);
  if ([...ch].length === 1 && meta.keyboard.some(r => r.includes(ch))) press(ch);
});

document.querySelectorAll('[data-close]').forEach(btn => {
  btn.addEventListener('click', () => closeModal(btn.closest('.modal').id));
});
document.querySelectorAll('.modal').forEach(m => {
  m.addEventListener('click', e => { if (e.target === m) closeModal(m.id); });
});

$('btn-help').addEventListener('click', () => openModal('modal-help'));
$('btn-stats').addEventListener('click', openStats);
$('btn-settings').addEventListener('click', () => openModal('modal-settings'));
$('btn-share').addEventListener('click', doShare);

$('sw-theme').addEventListener('click', () => {
  prefs.theme = prefs.theme === 'dark' ? 'light' : 'dark';
  savePrefs();
  applyTheme();
});
$('sw-cb').addEventListener('click', () => {
  prefs.colorblind = !prefs.colorblind;
  savePrefs();
  applyTheme();
});

// Hard mode is the server's business: it refuses the change once the first
// guess is in, so the switch reflects whatever comes back.
$('sw-hard').addEventListener('click', async () => {
  const wanted = !game.hardMode;
  try {
    game = await API.game(meta.code, wanted);
  } catch (err) {
    return apiToast(err);
  }
  applyTheme();
  if (game.hardMode !== wanted) toast(S.hardModeLocked, 2200);
});

$('lang-select').addEventListener('change', async e => {
  closeModal('modal-settings');
  $('loader').hidden = false;
  $('app').hidden = true;
  try {
    await selectLanguage(e.target.value);
  } catch (err) {
    apiToast(err);
  }
  $('loader').hidden = true;
  $('app').hidden = false;
  sizeBoard();
});

window.addEventListener('resize', sizeBoard);
window.addEventListener('orientationchange', () => setTimeout(sizeBoard, 200));

boot().catch(err => {
  const p = document.createElement('p');
  p.className = 'fatal';
  p.textContent = err.offline
    ? 'Cannot reach the server.'
    : 'Failed to start: ' + err.message;
  $('loader').innerHTML = '';
  $('loader').appendChild(p);
  console.error(err);
});

/* ------------------------------------------------------------------ *
 * Test hooks, attached only with ?__test=1. They expose the client's
 * own view of the game; the answer still has to come from the server's
 * test endpoint, which is gated behind LINGLE_TEST_MODE.
 * ------------------------------------------------------------------ */
if (new URLSearchParams(location.search).has('__test')) {
  window.__test = {
    game: () => game,
    meta: () => meta,
    lang: () => meta.code,
    status: () => game.status,
    rows: () => game.rows,
    shareText,
    reload: async () => { game = await API.game(meta.code); renderGame(false); },
  };
}
