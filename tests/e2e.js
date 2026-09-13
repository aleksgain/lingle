/* End-to-end suite. Boots the real server in Chromium and checks both the
 * gameplay and the thing the backend exists for: that the client is never
 * told the answer or the word list until the game is over. */
const { chromium } = require('playwright');
const path = require('path');

const BASE = process.env.BASE || 'http://127.0.0.1:8080';
const SHOTS = path.join(__dirname, '..', '.shots');
let failures = 0;

function check(name, ok, detail) {
  console.log(`${ok ? '  PASS' : '  FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`);
  if (!ok) failures++;
}

// Cyrillic has no KeyX code, so dispatch the key events directly.
async function typeText(page, word) {
  for (const ch of word) {
    await page.evaluate(c => document.dispatchEvent(
      new KeyboardEvent('keydown', { key: c, bubbles: true })), ch);
    await page.waitForTimeout(15);
  }
}
// Wiping localStorage resets the "seen the rules" flag, so the help modal
// comes back on the next load and would swallow clicks.
async function dismissHelp(page) {
  if (await page.isVisible('#modal-help')) {
    await page.click('#modal-help [data-close]');
    await page.waitForTimeout(150);
  }
}

const key = (page, k) => page.evaluate(c => document.dispatchEvent(
  new KeyboardEvent('keydown', { key: c, bubbles: true })), k);

// The scoring rule, reimplemented here so the test does not simply trust
// whatever the server returns.
function expectedMarks(guess, answer) {
  const g = [...guess], a = [...answer];
  const out = g.map(() => 'absent');
  const pool = {};
  g.forEach((c, i) => {
    if (c === a[i]) out[i] = 'correct';
    else pool[a[i]] = (pool[a[i]] || 0) + 1;
  });
  g.forEach((c, i) => {
    if (out[i] !== 'correct' && pool[c] > 0) { out[i] = 'present'; pool[c]--; }
  });
  return out;
}

async function newGame(browser, lang) {
  const ctx = await browser.newContext({ viewport: { width: 420, height: 860 } });
  const page = await ctx.newPage();
  const errors = [];
  const bodies = [];
  page.on('pageerror', e => errors.push(e.message));
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  page.on('response', async res => {
    if (!res.url().includes('/api/')) return;
    try { bodies.push({ url: res.url(), body: await res.text() }); } catch (e) { /* ignore */ }
  });
  await page.goto(`${BASE}/?__test=1${lang ? '&lang=' + lang : ''}`, { waitUntil: 'networkidle' });
  await page.waitForSelector('#app:not([hidden])', { timeout: 15000 });
  return { ctx, page, errors, bodies };
}

(async () => {
  const browser = await chromium.launch();
  const api = await (await fetch(`${BASE}/api/_test/answer?lang=ru`)).json();
  const answer = api.answer;

  const { ctx, page, errors, bodies } = await newGame(browser);
  console.log(`  (ru day=${api.day} answer hidden from client, ${answer.length} letters)`);

  check('help modal shown on first visit', await page.isVisible('#modal-help'));
  await page.click('#modal-help [data-close]');

  /* ---------------- the client must not hold the dictionary ---------- */
  const cfg = await (await fetch(`${BASE}/api/config`)).json();
  const cfgText = JSON.stringify(cfg);
  check('/api/config carries no word lists',
    !('guesses' in cfg.languages[0]) && !('answers' in cfg.languages[0]));
  check('/api/config does not contain the answer', !cfgText.includes(answer));
  check('/api/config is small (no 40k-word payload)', cfgText.length < 40000,
    `${(cfgText.length / 1024).toFixed(1)} KB`);
  const packProbe = await fetch(`${BASE}/packs/ru.json`);
  check('word packs are not served over HTTP', packProbe.status === 404,
    'status ' + packProbe.status);

  check('russian pack is the default', await page.evaluate(() => window.__test.lang()) === 'ru');
  check('board has 6 rows of 5 tiles',
    (await page.$$('#board .row')).length === 6 &&
    (await page.$$('#board .row:first-child .tile')).length === 5);
  check('russian keyboard rendered', (await page.$$('.key[data-key]')).length === 34);

  /* ---------------------------- gameplay ----------------------------- */
  await typeText(page, 'ыыыыы');
  await key(page, 'Enter');
  await page.waitForTimeout(500);
  check('word not in the dictionary is rejected by the server',
    (await page.textContent('#toaster')).trim().length > 0);
  for (let i = 0; i < 5; i++) await key(page, 'Backspace');

  await typeText(page, 'слово');
  await key(page, 'Enter');
  await page.waitForTimeout(2400);
  const states = await page.$$eval('#board .row:first-child .tile', ts => ts.map(t => t.dataset.state));
  check('first guess coloured', states.every(s => ['correct', 'present', 'absent'].includes(s)),
    states.join(','));
  check('server scoring matches the rule',
    JSON.stringify(states) === JSON.stringify(expectedMarks('слово', answer)));
  check('keyboard keys coloured', (await page.$$('.key[data-state]')).length > 0);
  await page.screenshot({ path: `${SHOTS}/01-ru-progress.png` });

  const leaked = bodies.filter(b => b.body.includes(answer));
  check('the answer never reached the browser mid-game', leaked.length === 0,
    leaked.map(b => b.url).join(', '));
  check('no definition is sent mid-game',
    await page.evaluate(() => window.__test.game().definition) === undefined);

  /* ------------------- progress lives on the server ------------------ */
  await page.evaluate(() => localStorage.clear());
  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForSelector('#app:not([hidden])');
  const restored = await page.$$eval('#board .row:first-child .tile',
    ts => ts.map(t => t.textContent).join(''));
  check('guess survives a reload with localStorage wiped', restored === 'слово', restored);
  await dismissHelp(page);

  /* --------------------------- hard mode ----------------------------- */
  await page.click('#btn-settings');
  await page.click('#sw-hard');
  await page.waitForTimeout(400);
  check('hard mode cannot be switched on mid-game',
    await page.evaluate(() => window.__test.game().hardMode) === false);
  await page.click('#modal-settings [data-close]');

  /* ------------------------------ winning ---------------------------- */
  await typeText(page, answer);
  await key(page, 'Enter');
  await page.waitForTimeout(2600);
  check('winning guess ends the game',
    await page.evaluate(() => window.__test.status()) === 'won');
  await page.waitForTimeout(1900);
  check('stats modal opens after a win', await page.isVisible('#modal-stats'));
  const share = await page.evaluate(() => window.__test.shareText());
  check('share text has a grid',
    /\n\n[\u2b1b\u2b1c\ud83d\udfe9\ud83d\udfe8\ud83d\udfe7\ud83d\udfe6]{5}/u.test(share),
    JSON.stringify(share.split('\n')[0]));
  console.log('  share text:\n' + share.split('\n').map(l => '    ' + l).join('\n'));
  check('countdown running', /^\d\d:\d\d:\d\d$/.test(await page.textContent('#next-time')));

  /* --------------------- the word and its meaning -------------------- */
  check('word card shown once the puzzle is over', await page.isVisible('#word-card'));
  const card = await page.evaluate(() => ({
    word: document.getElementById('wc-word').textContent,
    meta: document.getElementById('wc-meta').textContent,
    gloss: document.getElementById('wc-gloss').textContent,
    def: window.__test.game().definition || null,
  }));
  console.log(`  word card: ${card.word} [${card.meta}] ${card.gloss}`);
  const stripAccents = t => t.normalize('NFD').replace(/[\u0300-\u036f]/g, '');
  check('word card shows the answer', stripAccents(card.word) === answer, card.word);
  if (card.def) {
    check('definition arrived with the finished game', card.gloss.length > 0, card.gloss);
    check('definition is labelled with a part of speech', card.meta.length > 0, card.meta);
    check('russian word card carries stress marks', card.word !== answer || !card.def.accented,
      card.word);
  } else {
    console.log('  (today\'s word has no dictionary entry; card falls back to the word alone)');
    check('word card still renders without a definition', card.word.length > 0);
  }
  await page.screenshot({ path: `${SHOTS}/02-ru-win.png` });
  await page.click('#modal-stats [data-close]');

  /* ------------------- further guesses are refused -------------------- */
  const extra = await page.evaluate(async () => {
    const res = await fetch('/api/guess', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ lang: 'ru', guess: 'слово' }),
      credentials: 'same-origin',
    });
    return res.json();
  });
  check('guesses after the game ends are refused',
    extra.accepted === false && extra.reason === 'finished', JSON.stringify(extra.reason));
  check('rows did not grow', extra.game.rows.length === 2, String(extra.game.rows.length));

  /* ---------------------- stats came from the server ------------------ */
  const stats = await (await page.evaluate(() =>
    fetch('/api/stats?lang=ru', { credentials: 'same-origin' }).then(r => r.json())));
  check('server recorded the win', stats.played === 1 && stats.wins === 1 && stats.streak === 1,
    JSON.stringify(stats));
  check('guess distribution recorded at 2 tries', stats.dist[1] === 1, JSON.stringify(stats.dist));

  /* --------------------------- another player ------------------------- */
  const second = await newGame(browser);
  const freshRows = await second.page.evaluate(() => window.__test.rows().length);
  check('a different browser gets its own empty game', freshRows === 0, String(freshRows));
  const freshStats = await second.page.evaluate(() =>
    fetch('/api/stats?lang=ru', { credentials: 'same-origin' }).then(r => r.json()));
  check('a different browser gets its own stats', freshStats.played === 0,
    JSON.stringify(freshStats));
  await second.ctx.close();

  /* -------------------------- language switch ------------------------- */
  await page.click('#btn-settings');
  await page.selectOption('#lang-select', 'en');
  await page.waitForTimeout(1200);
  check('switched to english', await page.evaluate(() => window.__test.lang()) === 'en');
  check('qwerty keyboard rendered', (await page.$$('.key[data-key]')).length === 28);
  check('english game starts empty',
    await page.evaluate(() => window.__test.rows().length) === 0);
  await typeText(page, 'crane');
  await key(page, 'Enter');
  await page.waitForTimeout(2400);
  check('english guess accepted',
    (await page.$$eval('#board .row:first-child .tile',
      ts => ts.map(t => t.dataset.state))).every(Boolean));
  await page.screenshot({ path: `${SHOTS}/03-en.png` });

  /* ----------------------------- appearance --------------------------- */
  await page.click('#btn-settings');
  await page.click('#sw-theme');
  await page.waitForTimeout(200);
  check('light theme applied',
    await page.evaluate(() => document.documentElement.dataset.theme) === 'light');
  await page.screenshot({ path: `${SHOTS}/04-light.png` });
  await page.click('#sw-theme');
  await page.click('#modal-settings [data-close]');

  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForSelector('#app:not([hidden])');
  await dismissHelp(page);
  check('language remembered', await page.evaluate(() => window.__test.lang()) === 'en');

  /* ------------------------------- layout ----------------------------- */
  await page.setViewportSize({ width: 320, height: 568 });
  await page.waitForTimeout(400);
  const overflow = await page.evaluate(() => ({
    x: document.documentElement.scrollWidth > window.innerWidth + 1,
    y: document.body.scrollHeight > window.innerHeight + 1,
  }));
  check('no horizontal overflow at 320px', !overflow.x);
  check('no vertical overflow at 320x568', !overflow.y);
  await page.screenshot({ path: `${SHOTS}/05-small.png` });

  check('no console or page errors', errors.length === 0, errors.slice(0, 3).join(' | '));

  await ctx.close();
  await browser.close();
  console.log(failures ? `\n${failures} check(s) failed` : '\nall checks passed');
  process.exit(failures ? 1 : 0);
})();
