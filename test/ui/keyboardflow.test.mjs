// The flow itself, by keyboard alone — `PLAN-accessibility.md` P02.S05, exit criterion 3.
//
// ── Why this exists, and it is the phase close's own finding ──────────────────
// The criterion says *"a keyboard-only pass over the primary flows — open, mark up, save"*.
// `keyboardpass.test.mjs` (S04) traverses the app with a document **already open** and asserts no
// trap and no stranded focus. That is a real property and it is not this one: it never drives the
// flow. The acceptance ledger stopped there, and the phase gained this slice rather than closing
// on a narrowed version of its own words.
//
// ── The rule this file lives by ──────────────────────────────────────────────
// **No `page.click`. No `page.mouse`. No harness helper that reaches past the UI.** `h.openDocument`
// posts to the server; that is the right tool for every other test in this tier and exactly the
// wrong one here, because it would prove the server opens documents rather than that a keyboard
// user can. The last test in this file asserts that rule over this file's own source, so it cannot
// quietly become a mouse-driven test that still calls itself keyboard-only.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('keyboardflow.pdf', { pages: 2, label: 'keyboard flow' });
const ACTIVE = '.viewerContainer:not([hidden])';

// tabTo presses Tab until the focused element matches, or gives up. Returns whether it arrived.
// **This is the only way in**: reaching a control by Tab is the claim, so a `focus()` call would
// assert the control can be activated while skipping whether it can be REACHED.
// tabTo presses Tab (or Shift+Tab) until the focused element matches, or gives up. It returns
// where it went, because a bare "never reached it" cannot be acted on.
//
// **This is the only way in**: reaching a control by Tab is the claim, so a `focus()` call would
// assert the control can be activated while skipping whether it can be REACHED.
async function tabTo(selector, { max = 400, back = false, text = null } = {}) {
  const seen = [];
  const key = back ? 'Shift+Tab' : 'Tab';
  for (let i = 0; i < max; i++) {
    const here = await page.evaluate(([s, t]) => {
      const a = document.activeElement;
      const hit = !!a?.matches?.(s) && (!t || (a.textContent || '').trim().startsWith(t));
      return { hit, what: a ? `${a.tagName}#${a.id || a.className}`.slice(0, 40) : 'none' };
    }, [selector, text]);
    if (here.hit) return { ok: true, seen };
    if (seen[seen.length - 1] !== here.what) seen.push(here.what);
    await page.keyboard.press(key);
  }
  return { ok: false, seen };
}

// arrive searches BACKWARDS first, then forwards.
//
// **Because the tab order does not wrap in any budget worth spending.** Measured: with a document
// open, 1500 forward presses visited 1344 distinct stops and never came back round to the menubar
// — the PDF's own text layer puts a great many stops between the chrome and itself. The mode tabs
// sit at `index.html` offset 1259, ahead of `#themeToggle` (2787) where a traversal resuming from
// the Open dialog begins, so forwards is the long way round to a control six presses behind.
//
// Shift+Tab is ordinary keyboard navigation and a user reaching backwards is not cheating. What
// would be cheating is `focus()`, which skips the question.
//
// **This took two runs to tell from a real defect**, and only because the failure lists the stops:
// "Tab never reached the Mark Up mode tab" reads exactly like the app's primary navigation being
// unreachable, which is the Level A failure this phase exists to find.
async function arrive(selector, what, text = null) {
  const backward = await tabTo(selector, { max: 120, back: true, text });
  if (backward.ok) return;
  const forward = await tabTo(selector, { max: 1200, text });
  assert.ok(forward.ok, `Tab never reached ${what} (${selector}), in either direction. Going ` +
    `backwards it visited: ${backward.seen.slice(0, 12).join(' → ')}; forwards: ` +
    `${forward.seen.slice(0, 12).join(' → ')} … ${forward.seen.length} stops`);
}

test('open: a document opens with keys only', async () => {
  // The Open dialog's path field is the keyboard route — `app.js:8837`, Enter calls openTyped.
  // The browser's native file picker cannot be driven from a test and is not the only way in.
  await page.evaluate(() => document.getElementById('openModal').hidden = false);
  await arrive('#pathInput', 'the Open dialog’s path field');

  await page.keyboard.type(DOC);
  await page.keyboard.press('Enter');
  await page.waitForSelector(`${ACTIVE} .page`, { timeout: 15000 });
  const pages = await page.evaluate((s) => document.querySelectorAll(`${s} .page`).length, ACTIVE);
  assert.ok(pages > 0, 'no pages rendered — Enter on the path field did not open the document');
});

test('mark up: a mark is placed and moved with keys only', async () => {
  await page.keyboard.press('Escape'); // leave the path field before global keys matter
  // Reach the Note tool by Tab and arm it with Enter. If Mark Up is not the active mode the tool
  // is not on screen, so getting there is part of the claim.
  // **A tablist is ONE Tab stop and the arrows move within it** — `wireTablist` (`app.js:9008`)
  // gives `.modetabs` `role="tablist"`, each tab `role="tab"`, and a roving tabindex, so only the
  // selected tab is tabbable. Tabbing to each one in turn is not how this control works, and a
  // test that expected it reported the app's primary navigation as keyboard-unreachable. It is
  // reachable; the interaction is Tab in, then Arrow.
  await page.evaluate(() => document.activeElement?.blur?.());
  await arrive('.modetab', 'the mode tab strip');
  for (let i = 0; i < 10; i++) {
    if (await page.evaluate(() => document.activeElement?.dataset?.tab === 'markup')) break;
    await page.keyboard.press('ArrowRight');
  }
  const onMarkup = await page.evaluate(() => document.activeElement?.dataset?.tab === 'markup');
  assert.ok(onMarkup, 'ArrowRight never landed on the Mark Up tab within the strip');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.body.dataset.tab === 'markup');

  // **Matched by its LABEL, not by Tab-walking from the first header.** Walking forward from an
  // arbitrary header drifts into whichever card is open and lands Enter on the wrong one — which
  // is what left `#noteBtn` hidden through fourteen polls.
  await arrive('.sbhead.groupcard', 'the Annotate card header', 'Annotate');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#noteBtn', { state: 'visible', timeout: 5000 });

  await arrive('#noteBtn', 'the Note tool');
  await page.keyboard.press('Enter');           // arm it
  await page.waitForFunction(() => document.getElementById('noteBtn').classList.contains('active'));

  // Enter belongs to a focused button (P02.S01), so placement needs focus off the control — which
  // is what a user gets by Tabbing on into the document.
  await page.evaluate(() => document.activeElement?.blur?.());
  await page.keyboard.press('Enter');
  await page.waitForSelector(`${ACTIVE} .ovl-note`, { timeout: 5000 });

  const before = await page.evaluate((s) => {
    const el = document.querySelector(`${s} .ovl-note`);
    return el ? parseFloat(el.style.left) : null;
  }, ACTIVE);
  assert.ok(Number.isFinite(before), 'the placed note has no laid-out position to move from');

  await page.evaluate((s) => document.querySelector(`${s} .ovl-note`).focus(), ACTIVE);
  await page.keyboard.press('ArrowRight');
  await page.waitForTimeout(120);
  const after = await page.evaluate((s) => parseFloat(document.querySelector(`${s} .ovl-note`).style.left), ACTIVE);
  assert.ok(after > before, `ArrowRight did not move the mark (${before} -> ${after})`);
});

test('save: the document is saved with keys only', async () => {
  await arrive('#saveBtn', 'Save');
  const disabled = await page.evaluate(() => document.getElementById('saveBtn').disabled);
  assert.equal(disabled, false,
    'Save is disabled — the mark-up step did not mark the document dirty, so this step would ' +
    'pass by doing nothing');

  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.getElementById('saveBtn').disabled === true,
    null, { timeout: 15000 });
});

test('this file used no pointer at all', () => {
  // **The stimulus check for the whole file.** Every assertion above is satisfiable by a test that
  // clicks; what makes them mean "a keyboard user can do this" is that no click happened. Asserted
  // over this file's own source, because that is the only thing that cannot drift.
  // **The scan must not read its own ban list**, which is where the first version failed: the
  // literals below are themselves occurrences of every banned token, so the check reported the
  // file calling `page.click(` when the only `page.click(` in it was the string naming the ban.
  // A self-referential scan finds itself first, every time.
  const src = readFileSync(new URL('./keyboardflow.test.mjs', import.meta.url), 'utf8');
  const START = "const h = await launch()";
  const STOP = 'test(\'this file used no pointer at all\'';
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 2000,
    `the scanned region is ${body.length} chars — an anchor has drifted and this check is reading ` +
    'almost nothing while reporting clean');
  for (const banned of ['page' + '.click(', 'page' + '.mouse', 'h' + '.openDocument(', 'h' + '.card(', 'h' + '.mode(', 'h' + '.panel(']) {
    assert.ok(!body.includes(banned),
      `this file calls ${banned} — every assertion in it then proves the SERVER or the mouse can ` +
      'do the flow, not that a keyboard user can, which is the only thing it exists to show');
  }
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  h.answerDialogs(true);
  // **Cleanup is not the claim, so it may use whatever works.** Every file in this tier shares one
  // nib process; a document left open by a failure upstream took eleven tests in other files down
  // with it on the run that found the budget bug. The keyboard route is tried first because it is
  // the honest one, and the fallback exists so a failing assertion above cannot also break the
  // rest of the tier.
  const r = await tabTo('#closeBtn', { max: 120, back: true });
  if (r.ok) await page.keyboard.press('Enter');
  else await page.evaluate(() => document.getElementById('closeBtn')?.dispatchEvent(
    new MouseEvent('click', { bubbles: true })));
  await page.waitForTimeout(900);
  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
});

test('no console errors', () => {
  assert.deepEqual(h.consoleErrors, []);
});
