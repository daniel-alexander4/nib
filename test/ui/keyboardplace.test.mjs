// Placing and moving a mark without a pointer — `PLAN-accessibility.md` P02.S01, WCAG SC 2.1.1.
//
// ── What only this tier can see ──────────────────────────────────────────────
// `keyboardplacement.test.mjs` (tier 2) proves the POPULATION: every `view.*Mode` the code
// defines has a keyboard path, so a twelfth tool cannot ship pointer-only. It cannot prove the
// path WORKS. The door synthesises a real `pointerdown`/`pointerup` pair on the page div, and
// every reason that could fail needs a real browser: `startedInActiveView` walks up from
// `e.target` looking for `.viewerContainer`, `pageAt` needs page rects with non-zero geometry,
// and the drag tools read their second coordinate off the `pointerup`. jsdom has no layout, so
// there every one of those is zero and the whole mechanism is invisible.
//
// ── The stimulus, which is what makes the rest mean anything ─────────────────
// The control presses Enter with NO tool armed and requires that nothing is placed. Without it,
// a door that placed a note on every Enter would pass every assertion below.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('keyboardplace.pdf', { pages: 2, label: 'keyboard placement' });
const ACTIVE = '.viewerContainer:not([hidden])';

// ensureTools re-opens the Mark Up mode's Annotate card before each test that needs a tool.
//
// **Not hoisted into a single setup**, because a test in this file presses Enter while a card
// header may hold focus, and a focused header takes Enter as an activation and closes the card —
// which is correct app behaviour and exactly what the door must not override. Each test that
// needs a button therefore asks for it rather than assuming the last one left it open.
async function ensureTools() {
  await h.mode('markup');
  await h.card('Annotate & Draw');
  await page.waitForSelector('#noteBtn', { state: 'visible' });
}

async function arm(id) {
  const on = await page.evaluate((i) => document.getElementById(i).classList.contains('active'), id);
  if (!on) await page.click(`#${id}`);
  await page.waitForFunction((i) => document.getElementById(i).classList.contains('active'), id);
}
async function disarm(id) {
  const on = await page.evaluate((i) => document.getElementById(i).classList.contains('active'), id);
  if (on) await page.click(`#${id}`);
  await page.waitForFunction((i) => !document.getElementById(i).classList.contains('active'), id);
}
const count = (sel) => page.evaluate((s) => document.querySelectorAll(s).length, `${ACTIVE} ${sel}`);

test('setup: the document opens and the annotate tools are reachable', async () => {
  await h.openDocument(DOC, 2);
  await h.topOfDocument();
  await h.mode('markup');
  await h.group('Annotate & Draw');
  assert.ok(await page.$('#noteBtn'), 'the Note tool is not on screen — nothing below arms anything');
});

test('Enter with NO tool armed places nothing', async () => {
  // The stimulus check, and it runs FIRST so that a door placing marks unconditionally is caught
  // before any test that would be satisfied by it.
  await ensureTools();
  await disarm('noteBtn');
  await page.evaluate(() => document.activeElement?.blur?.());
  const before = await count('.ovl');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(150);
  assert.equal(await count('.ovl'), before,
    'Enter placed a mark with no tool armed — the door is not asking what is armed, and every ' +
    'assertion in this file would pass against it');
});

test('Enter with the Note tool armed places a note, keyboard alone', async () => {
  await ensureTools();
  await arm('noteBtn');
  // Focus leaves the button that armed the tool: Enter belongs to a focused control, and the
  // door deliberately does not take it from one. That rule has its own test below.
  await page.evaluate(() => document.activeElement?.blur?.());
  const before = await count('.ovl-note');
  await page.keyboard.press('Enter');
  await page.waitForSelector(`${ACTIVE} .ovl-note`, { timeout: 5000 });
  assert.equal(await count('.ovl-note'), before + 1,
    'no note appeared — SC 2.1.1: this tool cannot be used without a pointer');
});

test('the placed mark takes focus, so it can be moved without hunting for it with Tab', async () => {
  const focused = await page.evaluate(() => !!document.activeElement?.closest?.('.ovl'));
  assert.ok(focused,
    'the new mark does not have focus. "Keyboard alone" then means placing it and then Tabbing ' +
    'through the document to find it, which is the kind of pass that satisfies a checklist and ' +
    'not a user');
});

test("a note's text field keeps the arrow keys — they move the caret, not the note", async () => {
  // **This is why the nudge test below uses a border box and not a note.** A placed note focuses
  // its own `textarea` so the user can type, and inside a text field the arrow keys belong to the
  // caret. `isTypingTarget` bails before the nudge handler for exactly that reason, and it is the
  // right call — a note you cannot type into is worse than one you cannot nudge.
  //
  // Asserted rather than assumed, because it is the reason a whole test was rewritten.
  const inField = await page.evaluate(() => {
    const a = document.activeElement;
    return !!a && !!a.closest('.ovl') && (a.tagName === 'TEXTAREA' || a.tagName === 'INPUT');
  });
  assert.ok(inField,
    'a just-placed note does not put focus in its text field. If that changed, the nudge test ' +
    'below is no longer avoiding a case it was rewritten to avoid, and the reasoning in this ' +
    'file is stale');
});

test('arrow keys move the focused mark, and Shift+arrow resizes it', async () => {
  // Driven on a BORDER BOX, which carries no text field. A note's textarea takes the arrows for
  // its caret (above), so nudging one needs focus on the wrapper — reachable, but a different
  // question from whether the nudge mechanism works at all, which is what this asserts.
  await ensureTools();
  await disarm('noteBtn');
  await arm('borderBtn');
  await page.evaluate(() => document.activeElement?.blur?.());
  const before = await count('.ovl-box');
  await page.keyboard.press('Enter');
  await page.waitForFunction((n) => document.querySelectorAll(
    '.viewerContainer:not([hidden]) .ovl-box').length === n + 1, before, { timeout: 5000 });
  await disarm('borderBtn');

  // Focus the box itself. `layoutField` gave it tabIndex 0, which is the whole point of that door.
  await page.evaluate(() => {
    const els = document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-box');
    els[els.length - 1].focus();
  });
  const geom = () => page.evaluate(() => {
    const el = document.activeElement?.closest?.('.ovl');
    if (!el) return null;
    const s = el.style;
    return { left: parseFloat(s.left), top: parseFloat(s.top), w: parseFloat(s.width), h: parseFloat(s.height) };
  });
  const start = await geom();
  assert.ok(start && Number.isFinite(start.left) && start.w > 0,
    `setup: the focused mark has no laid-out geometry (${JSON.stringify(start)}), so "it moved" ` +
    'cannot be distinguished from "it was never placed"');

  await page.keyboard.press('ArrowRight');
  await page.waitForTimeout(80);
  const moved = await geom();
  assert.ok(moved.left > start.left,
    `ArrowRight did not move the mark (left ${start.left} -> ${moved.left})`);
  assert.ok(Math.abs(moved.w - start.w) < 0.5,
    `ArrowRight changed the mark's WIDTH (${start.w} -> ${moved.w}) — a nudge moves, it does not resize`);

  await page.keyboard.press('ArrowDown');
  await page.waitForTimeout(80);
  const down = await geom();
  assert.ok(down.top > moved.top, `ArrowDown did not move the mark (top ${moved.top} -> ${down.top})`);

  await page.keyboard.press('Shift+ArrowRight');
  await page.waitForTimeout(80);
  const wider = await geom();
  assert.ok(wider.w > down.w,
    `Shift+ArrowRight did not widen the mark (${down.w} -> ${wider.w})`);
  assert.ok(Math.abs(wider.left - down.left) < 0.5,
    `Shift+ArrowRight MOVED the mark (left ${down.left} -> ${wider.left}) — a resize holds the origin`);
});

test('a drag-to-draw tool places from the keyboard too, not just the click-to-place ones', async () => {
  // Eight of the eleven tools build their mark on `pointerup` from geometry derived there. They
  // are the reason the door synthesises a pointer PAIR rather than calling constructors, and a
  // test that only drove the click-to-place tools would never touch that half.
  await ensureTools();
  await disarm('noteBtn');
  await arm('borderBtn');
  await page.evaluate(() => document.activeElement?.blur?.());
  const before = await count('.ovl-box');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(400);
  assert.equal(await count('.ovl-box'), before + 1,
    'the Border tool placed nothing from the keyboard. It builds on pointerup, so either the ' +
    'synthetic pair is not reaching it or the second coordinate is degenerate and its own ' +
    'minimum-size check rejected the box');
  await disarm('borderBtn');
});

test('an armed tool does not steal Enter from a focused button', async () => {
  // **Found by this tier, and it is the defect a source scan cannot reach.** The first version of
  // the door took Enter whenever a tool was armed and called `preventDefault()`. With focus on a
  // toolbar button — which is where focus IS, immediately after arming the tool with the keyboard
  // — that placed a mark and swallowed the button's own activation. Arming the Note tool broke
  // every button a Tab-using user could reach, which is a worse accessibility defect than the one
  // this slice exists to fix.
  await ensureTools();
  await arm('noteBtn');
  const before = await count('.ovl-note');
  // Focus the arming button itself and press Enter: the button must toggle, and no mark may appear.
  await page.focus('#noteBtn');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(250);

  assert.equal(await count('.ovl-note'), before,
    'Enter on a focused BUTTON placed a mark — the door is taking a key that belongs to whatever ' +
    'has focus');
  const stillArmed = await page.evaluate(() => document.getElementById('noteBtn').classList.contains('active'));
  assert.equal(stillArmed, false,
    'the button did not toggle — the door called preventDefault() and swallowed the activation, ' +
    'so a keyboard user cannot disarm the tool they just armed');
});

test('this file leaves the shared server as it found it', async () => {
  // Every file in this tier shares one nib process and one registry (`uirepro.sh`:
  // `--test-concurrency=1`), so a document left open is the next file's problem. Measured the
  // hard way: without this, six later tests went red — an empty-state check, two close-prompt
  // tests, a thumbnail teardown and two sibling cleanup assertions.
  const openPages = (await h.counts()).pages;
  h.answerDialogs(true); // the close prompts, because this file placed marks
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});

test('no console errors', () => {
  assert.deepEqual(h.consoleErrors, []);
});
