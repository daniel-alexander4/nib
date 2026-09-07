// The version pill says which Nib this is — in every state, including the one where a newer
// release exists.
//
// It used to relabel itself `Update to v<latest> ↓` the moment an update was found, which answered
// the wrong question: the pill's standing job is to report the INSTALLED version, and it stopped
// doing that at exactly the moment a second version number entered the conversation. Colour
// carries "there is an update" (the base is red; `.latest` green, `.unknown` yellow) and the
// tooltip carries which one, so the label never has to stop being the running version.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The label, the title and the class, driven through the real `/api/update/check` round trip and
// the real click handler. All of it is DOM state.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// That red is red. The palette is asserted from source in `theme.test.mjs`, and whether the pill
// is legible on it is a rendered-contrast question no jsdom assertion can answer.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const CURRENT = '1.125.0';
let check = { updateAvailable: false, current: CURRENT, latest: CURRENT };

const h = await boot({
  routes: {
    '/api/status': () => ({
      state: 'ready', csrf: 'test-csrf', version: CURRENT,
      autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false,
    }),
    '/api/update/check': () => check,
  },
});
const doc = h.document;
const pill = () => doc.getElementById('updatePill');
const get = () => doc.getElementById('updateGet');

test('with no check run, the pill shows the installed version', () => {
  assert.equal(get().textContent, `v${CURRENT}`,
    'the pill does not name the installed version before any check has run — that is its standing job');
  assert.equal(pill().classList.contains('unknown'), true, 'the un-checked state is not marked unknown');
});

test('an available update turns the pill red and KEEPS the installed version on it', async () => {
  check = { updateAvailable: true, current: CURRENT, latest: '1.126.0', url: 'https://example.invalid/r' };
  h.setConfirmAnswer(false); // decline the download; this test is about the pill, not the fetch
  get().click();
  await h.settle();

  assert.equal(get().textContent, `v${CURRENT}`,
    `with an update available the pill reads "${get().textContent}". It must still name the version you are RUNNING — the colour says an update exists and the tooltip says which`);
  assert.match(get().title, /1\.126\.0/,
    `the tooltip does not name the newer version: "${get().title}". Without it the red pill says only that something is out there`);
  assert.match(get().title, new RegExp(CURRENT.replace(/\./g, '\\.')),
    'the tooltip does not name the version you have, so it cannot say what the update is FROM');
  // Red is the base: it is the absence of the three status classes, so assert all three are off
  // rather than that some "red" class is on — a class that does not exist would pass that.
  for (const cls of ['current', 'latest', 'unknown']) {
    assert.equal(pill().classList.contains(cls), false,
      `the pill still carries .${cls} with an update available, so it is not showing the red base`);
  }
});

test('an up-to-date check goes green and still shows the installed version', async () => {
  check = { updateAvailable: false, current: CURRENT, latest: CURRENT };
  get().click();
  await h.settle();
  assert.equal(get().textContent, `v${CURRENT}`, 'the up-to-date pill stopped naming the installed version');
  assert.equal(pill().classList.contains('latest'), true, 'a confirmed up-to-date check is not marked latest');
});
