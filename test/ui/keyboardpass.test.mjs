// The keyboard-only pass — `PLAN-accessibility.md` P02.S04, WCAG SC 2.1.2 (no keyboard trap) and
// SC 2.4.3 (focus order).
//
// ── Why this is last in the phase ────────────────────────────────────────────
// S01 gave every placement tool a keyboard path, S02 made armed state programmatic, S03 gave the
// nameless buttons names. This is the pass that walks the result: Tab from the top and see where
// the focus actually goes.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Everything here. `document.activeElement` after a real Tab is a browser behaviour — jsdom's
// `.focus()` succeeds on elements inside `hidden` containers and its Tab does not move focus at
// all, so a keyboard traversal asserted in jsdom is a traversal of nothing. `dialogfocus.test.mjs`
// already records that lesson for the close half of this clause.
//
// ── What this cannot claim, said rather than implied ─────────────────────────
// "Tab reaches every interactive control" is not assertable over a UI where most controls are
// hidden behind a mode, a card, or an open document. What IS assertable, and is what SC 2.1.2
// actually requires, is that **focus is never stuck** and never lands somewhere invisible. The
// population is what Tab reaches from a real starting point, which is the population a keyboard
// user has.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('keyboardpass.pdf', { pages: 2, label: 'keyboard pass' });

// walk presses Tab n times and records where focus lands each time.
async function walk(n) {
  const seen = [];
  for (let i = 0; i < n; i++) {
    await page.keyboard.press('Tab');
    seen.push(await page.evaluate(() => {
      const a = document.activeElement;
      if (!a || a === document.body) return { tag: 'BODY', id: '', visible: true };
      const r = a.getBoundingClientRect();
      const cs = getComputedStyle(a);
      // **A DOM path, not a class name.** The first version keyed identity on `id || className`,
      // and every accordion header is `class="sbhead groupcard"` with no id — so three DIFFERENT
      // buttons in a row read as one element and the trap detector fired on an ordinary
      // traversal. An identity function that cannot tell two elements apart turns "focus moved"
      // into "focus is stuck".
      const path = (el) => {
        const parts = [];
        for (let n = el; n && n.nodeType === 1 && n !== document.body; n = n.parentElement) {
          parts.unshift(`${n.tagName}[${[...(n.parentElement?.children || [])].indexOf(n)}]`);
        }
        return parts.join('>');
      };
      return {
        tag: a.tagName,
        id: a.id || a.className || '',
        path: path(a),
        // "visible" in the sense a keyboard user cares about: it occupies space and is not
        // hidden. An offscreen-but-scrollable control is still reachable, so size is the test,
        // not viewport position.
        visible: !a.hasAttribute('hidden') && cs.visibility !== 'hidden' && cs.display !== 'none'
                 && (r.width > 0 || r.height > 0),
      };
    }));
  }
  return seen;
}

// firstStall reports the first element focus sat on for three consecutive Tab presses, or null.
//
// **Three, not two.** SC 2.1.2's trap is focus that cannot be moved on; two identical stops in a
// row is what an ordinary two-element cycle produces, and using it would fail on a correct app.
//
// The identity is a DOM path. The first version keyed on `id || className`, and every accordion
// header is `class="sbhead groupcard"` with no id — so three DIFFERENT buttons read as one and an
// ordinary traversal reported a trap.
function firstStall(seen) {
  let run = 1;
  for (let i = 1; i < seen.length; i++) {
    const same = (seen[i].path || seen[i].tag) === (seen[i - 1].path || seen[i - 1].tag);
    run = same ? run + 1 : 1;
    if (run >= 3) return { ...seen[i], run };
  }
  return null;
}

test('setup: a document is open, so the traversal covers a working app', async () => {
  await h.openDocument(DOC, 2);
  await h.topOfDocument();
  await page.evaluate(() => document.activeElement?.blur?.());
});

test('Tab moves focus — it is never stuck on one element', async () => {
  const seen = await walk(60);
  // Stimulus first: if Tab moved focus nowhere at all, every assertion below is about nothing.
  const distinct = new Set(seen.map((s) => s.path || s.tag));
  assert.ok(distinct.size > 5,
    `60 Tab presses reached ${distinct.size} distinct element(s) — focus is not moving, so this ` +
    'file is asserting nothing. Either the app did not load or the harness is not delivering keys');

  const stall = firstStall(seen);
  assert.equal(stall, null,
    stall && `focus stuck on ${stall.tag}#${stall.id} for ${stall.run} consecutive Tab presses — ` +
    'WCAG SC 2.1.2: a keyboard user who reaches this control cannot leave it');
});

test('the trap detector can actually detect a trap', async () => {
  // **This test exists because the obvious probe was inert and said nothing.** Injecting
  // `onblur="this.focus()"` into the markup does not create a trap in this app: the CSP is
  // `script-src 'self' 'wasm-unsafe-eval'` with no `unsafe-inline`, so the browser refuses the
  // inline handler outright — *"The action has been blocked."* The probe produced three unrelated
  // failures and left the detector green, which reads exactly like a detector that works.
  //
  // So the capability is asserted rather than assumed, and it is asserted the way the app could
  // actually acquire a trap: a listener added through the DOM API, which is what any real
  // focus-management bug would be.
  await page.evaluate(() => {
    const b = document.getElementById('themeToggle');
    window.__trap = () => b.focus();
    b.addEventListener('blur', window.__trap);
  });
  await page.evaluate(() => document.getElementById('themeToggle').focus());
  const seen = await walk(10);
  const stall = firstStall(seen);
  await page.evaluate(() => {
    const b = document.getElementById('themeToggle');
    b.removeEventListener('blur', window.__trap);
    delete window.__trap;
    b.blur();
  });

  assert.notEqual(stall, null,
    'a control that refocuses itself on blur did NOT trip the stall detector. The detector is ' +
    'inert, and every green run of the test above means nothing');
});

test('focus never lands on something invisible', async () => {
  // SC 2.4.3, and the failure mode `dialogfocus.test.mjs` records: a control that is hidden but
  // still in the tab order strands the user on a stop they cannot see.
  const seen = await walk(80);
  const invisible = seen.filter((s) => !s.visible && s.tag !== 'BODY')
    .map((s) => `${s.tag}#${s.id}`);
  assert.deepEqual([...new Set(invisible)], [],
    'Tab landed on element(s) that are hidden or have no box: ' + [...new Set(invisible)].join(', ') +
    '. A keyboard user stops there with nothing on screen to tell them where they are');
});

test('the traversal returns — it is a cycle, not a one-way street', async () => {
  // A tab order that never comes back is not a trap by SC 2.1.2's letter, but it is the same harm
  // for a user who Tabs past what they wanted: there is no way round again without a mouse.
  await page.evaluate(() => document.activeElement?.blur?.());
  const first = await walk(1);
  const rest = await walk(400);
  const key = (s) => s.path || s.tag;
  assert.ok(rest.some((s) => key(s) === key(first[0])),
    `400 Tab presses never returned to the first stop (${key(first[0])}) — either the tab order ` +
    'is longer than 400 or it does not cycle, and a keyboard user cannot get back');
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  h.answerDialogs(true);
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);
  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0, `${left} page divs survive the close`);
});

test('no console errors', () => {
  assert.deepEqual(h.consoleErrors, []);
});
