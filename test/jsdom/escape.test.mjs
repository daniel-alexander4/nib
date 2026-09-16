// Escape closes what is open — the dialogs and the first-run intro.
//
// Before v1.108.16 the ONLY thing in web/app.js bound to Escape was closeMenu(): none of
// the 37 *Modal dialogs closed on it, and the first-run intro overlay had no close control
// of any kind — a keyboard-only or screen-reader user had no announced way past it to the
// setup form beneath. (That overlay is gone as of v1.109.3, folded into the setup card;
// what remains here is the dialog half, which is the larger one.)
//
// **The dialog test asserts the CLEANUP, not just the hiding.** Escape clicks the
// dialog's own cancel rather than hiding the element, because hiding would skip what each
// cancel does — dropping a pending export's bytes, disarming a live listener, tearing
// down a second pdf.js document. A test that only checked `hidden` would pass against the
// cheap wrong implementation, which is the one someone would naturally write.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';

const h = await boot({ routes: {} });
const { document: doc, settle } = h;

const esc = () => {
  doc.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
};

// The first-run overlay this file used to test is GONE, and that is the stronger fix.
//
// v1.108.16 gave it a button and an Escape handler, because it had no close control of any
// kind and trapped a keyboard-only or screen-reader user on first run. v1.109.3 folded its
// content into the setup card instead: there is no second overlay to dismiss, so there is
// nothing to be trapped by and no Escape special-case to keep working.
//
// Kept as an assertion rather than deleted with its subject. "Escape dismisses the intro"
// stopped being a claim about anything the moment the intro stopped existing, and a test
// quietly removed alongside the thing it tested is how a property gets lost — the fold was
// supposed to REMOVE the trap, and this is what says it did rather than that someone
// deleted the check.
test('there is no separate first-run overlay left to trap anyone', () => {
  assert.equal(doc.getElementById('introOverlay'), null,
    'the first-run intro overlay is back as a separate stacked overlay — it sat at z-index 200 over the setup form and swallowed every click aimed at it');
  assert.ok(doc.getElementById('introBlock'),
    'the first-run explanation is gone entirely — it was folded INTO the setup card, not deleted, and a user asked to choose an SSH key needs to be told what it is for');
});

test('Escape closes an open dialog by clicking its cancel, so the cancel\'s cleanup runs', async () => {
  const modal = doc.getElementById('saveAsModal');
  const cancel = doc.getElementById('saveAsCancel');
  assert.ok(modal && cancel, 'setup: the Save As dialog is not in index.html under the expected ids');

  // A spy on the cancel, because "the modal is hidden" is true for BOTH the right
  // implementation and the wrong one. saveAsCancel is what drops the pending export's
  // bytes; an Escape that merely hid the dialog would leave them held.
  let cancelled = 0;
  cancel.addEventListener('click', () => { cancelled++; });

  modal.hidden = false;
  esc();
  await settle();

  assert.equal(modal.hidden, true, 'Escape left the dialog open');
  assert.equal(cancelled, 1,
    'Escape hid the dialog without going through its Cancel, so the cleanup that button performs did not run — for this dialog that means the pending export\'s bytes and chosen name stay held');
});

test('every dialog has a control Escape can reach', () => {
  // The mechanism is a convention — Escape looks for a button whose id ends in Cancel or
  // Close INSIDE the open dialog — and a convention with an exception is a dialog that
  // traps the user. Asserted across all of them rather than trusting the two driven
  // above, because the next dialog added is the one that will not have it.
  const modals = [...doc.querySelectorAll('div[id$="Modal"]')];
  assert.ok(modals.length >= 30, `only ${modals.length} dialogs found — this scan is not reading index.html`);
  const trapped = modals
    .filter((m) => !m.querySelector('button[id$="Cancel"], button[id$="Close"]'))
    .map((m) => m.id);
  assert.deepEqual(trapped, [],
    `these dialogs have no Cancel/Close button, so the Escape handler has nothing to click and they cannot be dismissed from the keyboard: ${trapped.join(', ')}`);
});

// The CSP comment in server.go claims the app has "no inline event handlers, no eval, no
// new Function, no insertAdjacentHTML, and every innerHTML assignment a static literal".
// The last clause was already false: app.js has one template literal interpolating a page
// counter. That is harmless — the values are integers — but the sentence is what the next
// person adding an innerHTML assignment will rely on, so the property is asserted rather
// than described.
test('no innerHTML assignment takes anything but a literal or a number', () => {
  const files = ['app.js', 'detect.js'];
  let sites = 0;
  for (const f of files) {
    const src = fs.readFileSync(path.join(REPO, 'web', f), 'utf8');
    const lines = src.split('\n').filter((l) => !l.trim().startsWith('//'));
    for (const line of lines) {
      const m = /\.innerHTML\s*=\s*(.+)$/.exec(line);
      if (!m) continue;
      sites++;
      const rhs = m[1].trim();
      // The RHS may be followed by a terminator and closing braces on a one-line arrow —
      // `x.innerHTML = \`…\`; };` — so match the literal itself rather than requiring it
      // to end the line. The first draft required that and went red against a site it
      // should have accepted.
      //
      // **And the tail after it must be nothing but punctuation.** Both patterns were
      // anchored at the START only, so `x.innerHTML = '<b>' + userInput;` matched the
      // leading `'<b>'`, set plain, and passed — the guard proved the first token was a
      // literal and said nothing about the concatenation that followed it. Same hole on
      // the template arm: the allowlist policed `${…}` INSIDE the backticks while
      // `` `…` + userInput `` sailed past outside them.
      const rest = (after) => /^[\s;,)\]}]*$/.test(after);
      const pm = /^(['"])(?:(?!\1).)*\1/.exec(rhs);
      const plain = pm !== null && rest(rhs.slice(pm[0].length));
      const tm = /^`([^`]*)`/.exec(rhs);
      const tmpl = tm !== null && rest(rhs.slice(tm[0].length));
      const interps = tmpl ? [...tm[1].matchAll(/\$\{([^}]*)\}/g)].map((x) => x[1].trim()) : [];
      // An ALLOWLIST of interpolated expressions, not a regex trying to prove a bare
      // identifier is numeric — it cannot, and a predicate that admits any identifier
      // admits the user-controlled ones this exists to keep out. Adding an interpolation
      // means adding it here, which is the review the CSP comment's claim deserves.
      const ALLOWED = ['++done', 'total'];
      const numeric = interps.every((e) => ALLOWED.includes(e));
      assert.ok(plain || (tmpl && numeric),
        `${f}: an innerHTML assignment takes a value this guard cannot prove is a literal `
        + `or a number — that is the CSP comment's claim, and the sandbox is what is left `
        + `if it is wrong:\n  ${line.trim()}`);
    }
  }
  // The floor: the app has innerHTML sites and always has. Zero means the scan stopped
  // matching and every assertion above ran over nothing.
  assert.ok(sites >= 5, `found ${sites} innerHTML site(s) — the scanner has gone blind`);
});

// --- dialog semantics and focus ------------------------------------------------
//
// These assert the live DOM, not index.html, and that is not a stylistic choice: the
// attributes are stamped at boot over `body > div[id$="Modal"]` and are never written
// into the file. A static scan of index.html would go red forever, including against a
// correct fix. Same boot, same population, same floor and same offender-naming message
// as the Escape test above, because the failure mode is identical — dialog 39 is the one
// that will not have it.

const modals = () => [...doc.querySelectorAll('body > div[id$="Modal"]')];

test('every dialog says it is a dialog', () => {
  const ms = modals();
  assert.ok(ms.length >= 30, `only ${ms.length} dialogs found — this scan is not reading the DOM`);
  for (const [attr, want] of [['role', 'dialog'], ['aria-modal', 'true'], ['tabindex', '-1']]) {
    const bare = ms.filter((m) => m.getAttribute(attr) !== want).map((m) => m.id);
    assert.deepEqual(bare, [],
      `these dialogs are missing ${attr}="${want}", so a screen reader does not treat them as dialogs: ${bare.join(', ')}`);
  }
});

// Presence is the cheap wrong check: aria-labelledby pointing at an id that does not
// exist announces NOTHING, and is worse than no label because the browser stops falling
// back. So the target must resolve AND carry text.
test('every dialog names itself, and the name resolves', () => {
  const ms = modals();
  assert.ok(ms.length >= 30, `only ${ms.length} dialogs found — this scan is not reading the DOM`);
  const broken = ms.filter((m) => {
    const id = m.getAttribute('aria-labelledby');
    if (!id) return true;
    const el = doc.getElementById(id);
    return !el || !el.textContent.trim();
  }).map((m) => m.id);
  assert.deepEqual(broken, [],
    `these dialogs have no aria-labelledby, or it points at something with no text — an unresolvable label announces nothing at all: ${broken.join(', ')}`);
});

test('opening a dialog moves focus into it', async () => {
  const m = doc.getElementById('aboutModal');
  doc.getElementById('menubar')?.querySelector('button')?.focus();
  m.hidden = false;
  await settle();
  assert.ok(m.contains(doc.activeElement) || doc.activeElement === m,
    `focus stayed on ${doc.activeElement && doc.activeElement.id} when the About dialog opened, so a keyboard user is still behind the scrim`);
  m.hidden = true;
  await settle();
});

// **A REGRESSION guard, not a fix guard — it passes today and that is the point.** Five
// dialogs focus their own field synchronously right after unhiding, and the observer's
// record arrives a microtask later. Delete the `m.contains(document.activeElement)`
// condition in app.js and this goes red.
test('a dialog that focuses its own field keeps it', async () => {
  const m = doc.getElementById('decryptModal');
  const pw = doc.getElementById('decryptPw');
  // Asserted, never `return`ed past (/pending 505): an early return made a rename of either id
  // report this test PASSED while it asserted nothing — the population guards above count
  // dialogs, and would not notice this one field going.
  assert.ok(m && pw, 'setup: #decryptModal or #decryptPw is gone from index.html, so this regression guard is guarding nothing — point it at another dialog that focuses its own field');
  m.hidden = false;
  pw.focus();
  await settle();
  assert.equal(doc.activeElement && doc.activeElement.id, 'decryptPw',
    'the focus observer stomped a dialog that had already placed focus in its own field');
  m.hidden = true;
  await settle();
});

test('focus cannot leave an open dialog', async () => {
  const m = doc.getElementById('aboutModal');
  m.hidden = false;
  await settle();
  try {
    // Both halves of the stimulus asserted (/pending 505). The old `if (outside) {…}` passed with
    // no assertion at all when the menubar lost its buttons; and a focus() that never landed —
    // leaving focus where the open put it, inside the dialog — would satisfy the trap check below
    // without the trap having run. The focusin is recorded, the same shape stackCase uses.
    const outside = doc.getElementById('menubar')?.querySelector('button');
    assert.ok(outside, 'setup: #menubar has no button, so there is nothing outside the dialog to try to focus and the trap is unexercised');
    let reached = false;
    const mark = () => { reached = true; };
    outside.addEventListener('focusin', mark);
    outside.focus();
    await settle();
    outside.removeEventListener('focusin', mark);
    assert.ok(reached, 'setup: focusing the menubar button never happened, so the trap was not exercised');
    assert.ok(m.contains(doc.activeElement) || doc.activeElement === m,
      'focus escaped to the toolbar behind an open dialog — with aria-modal="true" the reader has been told that element does not exist');
  } finally {
    m.hidden = true;
    await settle();
  }
});

// /pending 498. Two dialogs open at once, in the order the co-sign flow opens them: the co-sign
// dialog first (sessionInit keeps it up while its request is in flight), then the spoken check on
// top. #verifyModal comes BEFORE #sessionInitModal in index.html, so every reader of "the last in
// document order" — the trap, Escape, and the shared z-index's paint order — picked the co-sign
// dialog underneath. Driven in BOTH opening orders, because a fix that simply reversed document
// order would pass the first and fail the second.
async function stack(firstId, secondId) {
  const first = doc.getElementById(firstId), second = doc.getElementById(secondId);
  first.hidden = false;
  await settle();
  second.hidden = false;
  await settle();
  // The stimulus, asserted before anything is graded: both are open.
  assert.equal(first.hidden || second.hidden, false, 'setup: the two dialogs are not both open');
  return { first, second };
}
async function unstack(...ms) { for (const m of ms) m.hidden = true; await settle(); }

for (const [firstId, secondId, cancelId] of [
  ['sessionInitModal', 'verifyModal', 'verifyCancel'],
  ['verifyModal', 'sessionInitModal', 'sinCancel'],
]) {
  test(`with ${secondId} opened over ${firstId}, the one opened last is in front for Escape, focus and paint`, async () => {
    const { first, second } = await stack(firstId, secondId);
    // Closed in a finally: a red here must not leave both dialogs open for the next case, which
    // would then fail on inherited stacking rather than on its own condition (seen while probing).
    try { await stackCase(first, second, cancelId); } finally { await unstack(first, second); }
  });
}

async function stackCase(first, second, cancelId) {
  const firstId = first.id, secondId = second.id;
  {
    const cancel = doc.getElementById(cancelId);
    const other = [...first.querySelectorAll('button')].find((b) => !b.disabled);
    assert.ok(cancel && other, 'setup: a dialog control is missing from index.html');

    // Focus: a control in the dialog UNDERNEATH must not keep focus. The focusin is recorded, so a
    // focus() that never landed (and left focus where the open put it) cannot pass for a trap.
    let reached = false;
    const mark = () => { reached = true; };
    other.addEventListener('focusin', mark);
    other.focus();
    await settle();
    other.removeEventListener('focusin', mark);
    assert.ok(reached, `setup: focusing a control in ${firstId} never happened, so the trap was not exercised`);
    assert.ok(second.contains(doc.activeElement) || doc.activeElement === second,
      `focus was allowed into ${firstId} while ${secondId} was open over it — the trap is guarding the dialog underneath`);

    // Paint: the one in front must stack above the one behind. The stylesheet's 110 is the base,
    // so an empty inline value reads as 110.
    const z = (m) => Number(m.style.zIndex || 110);
    assert.ok(z(second) > z(first),
      `${secondId} (z ${z(second)}) does not paint above ${firstId} (z ${z(first)}) — the dialog in front is drawn underneath`);

    // Escape: it must reach the front dialog's own cancel, and only that one.
    let hit = 0;
    const spy = () => { hit++; };
    cancel.addEventListener('click', spy);
    esc();
    await settle();
    cancel.removeEventListener('click', spy);
    assert.equal(hit, 1, `Escape did not click ${cancelId} — it dismissed ${firstId}, the dialog underneath`);
  }
}

// ## What this cannot see
//
// - **Whether a focus target is actually rendered.** jsdom ignores `hidden` and has no
//   layout: `.focus()` SUCCEEDS on an element inside a hidden container here, and hiding
//   a container does not blur its focused descendant. So focus RESTORE has no honest test
//   at this tier — a tier-2 "focus was restored to the trigger" assertion passes on the
//   exact bug it would exist to catch (a trigger inside a collapsed dropdown). Restore is
//   tested in test/ui/ instead, where activeElement is the browser's.
// - **Whether a screen reader announces the dialog, or whether aria-modal actually prunes
//   the background.** No tier in this repo runs AT. This has NO OWNER and is not deferred
//   to one — it is unfixable here, and saying so is the point.
// - Whether the container shows an unwanted UA focus ring when focused programmatically,
//   and whether the scrim genuinely blocks pointer interaction. Both are tier 3's.
