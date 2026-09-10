// P03.S01 — the convener's setup sheet (D3).
//
// **The defect this ends.** `#ceremonyConveneForm` lived inside `<aside id="sidebar">`, which is
// 200px wide (`style.css`), and it holds a roster picker plus two fields. D3's answer is a
// full-width sheet owned by the Ceremony mode.
//
// **What only this tier can see:** that the sheet is a SIBLING of the viewer rather than a child of
// the sidebar, and that dismissing and re-entering keeps what was typed. What it cannot see is the
// reader's page surviving the round trip — jsdom lays nothing out, so that is tier 3's and is
// asserted there rather than approximated here.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
// /pending 412's test needs a document open, because `.markers button` is disabled without one —
// which is also why that test is LAST in the file: nothing above it should run against an open
// document it did not ask for.
import { setNextDocument } from './stub-pdfjs.mjs';

// Two peers, because P03.S04's clause is about the ROSTER surviving a round trip and an empty
// picker has no roster to lose. The three tests that predate it assert nothing about the picker.
// The stored draft is a MUTABLE fixture, empty for every test but the last one. A route that
// returned a draft to all of them would repopulate the form on each fresh entry and quietly become
// the starting state of every later assertion in the file.
let storedDraft = '';

let nextOpen = {
  id: 'test-epoch:1', name: 'lease.pdf', path: '/tmp/nib-harness/lease.pdf', canSave: true,
  signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
};

// Set to a pending promise to hold `/api/peers` open; null for every test that does not care.
let peersGate = null;

const { document: doc, settle, calls } = await boot({
  routes: {
    // **`peersGate` holds the request open**, which is the only way this tier can reach /pending
    // 414: the defect lives entirely in what the sheet does WHILE the two opening GETs are in
    // flight, and a route that answers in the same tick has no such window. boot.mjs awaits a
    // route's return value for exactly this (see its own note on /pending 370).
    '/api/peers': async () => {
      if (peersGate) await peersGate;
      return {
      self: 'f'.repeat(64),
      peers: [
        { fingerprint: 'a'.repeat(64), label: 'lively otter marble finch amber cove' },
        { fingerprint: 'b'.repeat(64), label: 'quiet heron ribbon plum copper lane' },
      ],
      };
    },
    '/api/ceremony/draft': (opts) => (opts.method === 'POST' ? { draft: '' } : { draft: storedDraft }),
    // A convene that succeeds, so the CONSUME path can be driven end to end rather than read.
    '/api/ceremony/convene': () => ({ ceremony: 'c'.repeat(64), invitations: [] }),
    '/api/ceremonies': () => ({ ceremonies: [] }),
    // Mutable, because /pending 413's pin is that the line follows the BOUND document and not the
    // active one — which needs two documents that are actually different.
    '/api/open': () => nextOpen,
    '/api/scan': { hidden: [] },
  },
});

const bar = () => doc.getElementById('cerSetupBar');

// **Declared FIRST in this file, and that placement is the whole assertion.** node:test runs a
// file's tests in declaration order, and the first `showCeremonyForm` call repairs the bar's state
// for good. Sitting further down — where it was first written — it went GREEN against a build with
// the `hidden` attribute deleted, because three tests above it had already opened the sheet. The
// probe that found that is why this comment exists.
// Asserted on a PRISTINE page, before anything opens a ceremony form — which is the only moment
// that can see it. `reflectCeremonyPark` has two callers and neither runs at boot, so the bar's
// initial state comes from the markup alone: drop the `hidden` attribute and every post-park and
// post-cancel assertion in this file stays green while a fresh load claims a ceremony setup is
// open, permanently, over the user's document. Found by a reviewer who had not read these tests.
test('a fresh load does not claim a ceremony setup is open', () => {
  assert.equal(bar().hidden, true,
    'the parked-setup bar is showing on a page where no ceremony form has ever been opened. '
    + 'Nothing calls reflectCeremonyPark at boot, so this state comes from the markup and nothing '
    + 'else would ever notice it');
  // The bar appears without a user action on the far side of a transition, so it has to announce
  // itself. `aria-live` overrides role="status"'s implicit politeness, so the role alone is not
  // the assertion — a bar with role="status" and aria-live="off" says nothing at all.
  // The predicate is the ATTRIBUTE, and the attribute is necessary rather than sufficient: a live
  // region whose own root is what toggles is not reliably announced by every AT, and nothing at any
  // tier here reads an announcement. Asserted anyway because parking raises no toast, so this
  // region is the only thing that says a setup is still open — and `aria-live` overrides
  // role="status"'s implicit politeness, so the role alone would pass while the bar said nothing.
  assert.equal(bar().getAttribute('aria-live'), 'polite',
    'the bar carries no live-region politeness, so nothing about the excursion is announced at all '
    + '— a screen-reader user gets only "Back to setup, button" from the focus move');
});

const sheet = () => doc.getElementById('ceremonySheet');
const form = () => doc.getElementById('ceremonyConveneForm');

async function openSheet() {
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
}

test('the setup form is not inside the sidebar', () => {
  const f = form();
  assert.ok(f, 'there is no #ceremonyConveneForm');
  assert.equal(f.closest('#sidebar'), null,
    'the convene form is still inside the 200px sidebar. It holds a roster picker and two fields, '
    + 'which is what D3 says does not fit there');
  assert.ok(f.closest('#ceremonySheet'), 'the form is outside the sidebar and not in the sheet either');
});

test('the sheet is a sibling of the viewer, not an overlay over it', () => {
  const s = sheet();
  const v = doc.getElementById('viewerWrap');
  assert.ok(s && v, 'the sheet or the viewer is missing');
  assert.equal(s.parentElement, v.parentElement,
    'the sheet is not a sibling of the viewer. D7 reserves overlay for the two synchronised '
    + 'moments, and setup is neither — it stands IN PLACE of the document, which is what makes '
    + 'P03.S04\'s trip out to the page and back a matter of hiding one and showing the other');
  // `#viewerCol`, not `#main` — the deepdive said `#main` was a flex over `#sidebar` and
  // `#viewerWrap` and that was wrong: there is a `#viewerCol` between them holding the tab strip
  // and the viewer. Landing there is the better placement anyway, because the sheet takes the
  // document COLUMN's space and leaves the strip and the sidebar where they were.
  assert.equal(s.parentElement.id, 'viewerCol',
    'the sheet is not in the document column, so it is not standing where the document stands');
});

test('opening setup shows the sheet and stands the viewer down', async () => {
  await openSheet();
  assert.equal(sheet().hidden, false, 'pressing Convene did not show the sheet');
  assert.equal(doc.getElementById('viewerWrap').hidden, true,
    'the viewer is still shown beside the sheet, so the sheet is not standing in its place and '
    + 'both are competing for the same row');
});

test('dismissing returns the document and keeps what was typed', async () => {
  await openSheet();
  doc.getElementById('cerIntent').value = 'We agree to the lease of 14 Elm Row';
  doc.getElementById('cerSheetClose').click();
  await settle();

  assert.equal(sheet().hidden, true, 'Close did not put the sheet away');
  assert.equal(doc.getElementById('viewerWrap').hidden, false,
    'the viewer did not come back, so dismissing the sheet leaves the user with nothing');

  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.equal(doc.getElementById('cerIntent').value, 'We agree to the lease of 14 Elm Row',
    're-entering setup lost what was typed. D3 calls the sheet "dismissible and re-enterable", and '
    + 'a sheet that forgets on dismissal is a form that cannot be left — which is the abandonment '
    + 'case D4 exists to end');
});

test('leaving the Ceremony mode puts the sheet away', async () => {
  await openSheet();
  doc.querySelector('.modetab[data-tab="markup"]').click();
  await settle();
  assert.equal(sheet().hidden, true,
    'the sheet survived a mode change. It stands in place of the document, so a user who switches '
    + 'to Mark Up is left looking at a convene form with no route back to the page');
  assert.equal(doc.getElementById('viewerWrap').hidden, false, 'the viewer did not come back');
});

test('accepting an invitation does NOT take the sheet', async () => {
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  doc.getElementById('ceremonyAcceptBtn').click();
  await settle();
  assert.equal(sheet().hidden, true,
    'accepting took the full-width sheet. It is a paste and a button — it fits the sidebar and '
    + 'always did — so spending the document\'s space on it is the sheet being used for its size '
    + 'rather than for what needs the room');
});

// ── P03.S04 — leaving the sheet for the document, and coming back ─────────────────────────────
//
// **The slice's own scope sentence was fiction and the pin in PLAN.md records it.** It said
// "signature-block placement leaves the sheet for the page", and in a ceremony nobody places a
// signature block: `PlacementFor` sends every ceremony party to `ceremonyPlacement`, which puts
// their block on the signature page their ROSTER POSITION allocates. The acceptance clauses never
// mentioned a block — they say leaving for the page and returning preserves what was typed, and
// that the sheet is re-entered rather than rebuilt — and those are what these tests assert.
//
// **"Rebuilt" is asserted as an ABSENCE OF REQUESTS, not as a description.** The rebuild door is
// `loadPeerPicker()` + `restoreCeremonyDraft()`, which are a GET `/api/peers` and a GET
// `/api/ceremony/draft`. Counting them is the only version of this clause a test can be wrong
// about, and the last test below is its stimulus floor: a genuine fresh entry must make both.

// **GET only, matching tier 3.** The rebuild door READS — `loadPeerPicker` GETs `/api/peers` and
// `restoreCeremonyDraft` GETs `/api/ceremony/draft` — while a POST to the same path is the draft
// SAVING, which is the draft working. Counting method-blind here would make the failure message
// below ("the return leg re-read the draft") untrue the first time a save landed on this path, and
// tier 3 already learned that lesson the hard way; a tier that does not apply it is the half of the
// pair that goes wrong quietly.
const countCalls = (frag) => calls.filter((c) => c.method === 'GET' && c.url.includes(frag)).length;

// typeSetup fills the sheet the way a convener does, and deliberately leaves the CAPACITY
// uncommitted: no `change` event, so no draft was ever saved for it. A round trip that restored
// from the draft rather than keeping the DOM would lose exactly this value and nothing else.
//
// **A real user cannot produce that state, and saying so is the honest version.** Every click that
// leaves the sheet blurs the focused input first, firing `change` → `saveCeremonyDraft`; only a
// script can set `.value` without an event. So this field is a sensitive DETECTOR of a rebuild, not
// a datum a user would lose — the clause itself is carried by the request counts and by the
// row-identity check, which hold whatever the draft happens to contain.
async function typeSetup() {
  doc.getElementById('cerIntent').value = 'We agree to the lease of 14 Elm Row';
  doc.getElementById('cerExpires').value = '2026-10-01T12:00';
  const row = doc.querySelector('#cerPeerPick .cerpeerrow');
  row.querySelector('.cerpeerbox').checked = true;
  row.querySelector('.cerpeercap').value = 'as tenant';
  await settle();
}

test('stepping out to the document keeps every field, and says setup is still open', async () => {
  await openSheet();
  await typeSetup();
  assert.equal(doc.querySelectorAll('#cerPeerPick .cerpeerrow').length, 2,
    'setup: the picker has no rows, so "the roster survived" would be true of a build that drops it');

  doc.getElementById('cerSeeDoc').click();
  await settle();

  assert.equal(sheet().hidden, true, 'See the document did not put the sheet away');
  assert.equal(doc.getElementById('viewerWrap').hidden, false,
    'the document did not come back, so the excursion goes nowhere — which is the whole trip');
  assert.equal(bar().hidden, false,
    'nothing on screen says a ceremony setup is open. The user stepped out to read the lease; the '
    + 'app now looks exactly like an ordinary document and her half-filled ceremony is invisible');
  assert.equal(bar().closest('#viewerWrap')?.id, 'viewerWrap',
    'the way back is not inside #viewerWrap. #sidebar.collapsed is display:none and a crossing '
    + 'listener collapses it below 899px, so a control in a sidebar panel disappears when the user '
    + 'narrows the window mid-excursion');
  assert.equal(doc.getElementById('cerIntent').value, 'We agree to the lease of 14 Elm Row',
    'stepping out cleared the recital');
  // The control the user was on (#cerSeeDoc) has just been hidden, so focus MUST be moved or it is
  // stranded and the next Tab restarts from the top of the page. This tier can only see that the
  // move was attempted against a real element — an `els.cerBackToSetup?.focus()` typo is a silent
  // no-op here, since `els` never caches that id. Whether the move LANDS is tier 3's, because jsdom
  // will happily focus an element inside a display:none subtree.
  assert.equal(doc.activeElement?.id, 'cerBackToSetup',
    'focus was not moved to the way back. It was on the button that has just been hidden, so a '
    + 'keyboard user is left with focus nowhere and no idea the excursion happened');
});

test('coming back re-enters the sheet — it re-reads nothing', async () => {
  const peersBefore = countCalls('/api/peers');
  const draftBefore = countCalls('/api/ceremony/draft');
  const rowsBefore = Array.from(doc.querySelectorAll('#cerPeerPick .cerpeerrow'));

  doc.getElementById('cerBackToSetup').click();
  await settle();

  assert.equal(sheet().hidden, false, 'Back to setup did not bring the sheet back');
  assert.equal(doc.getElementById('viewerWrap').hidden, true, 'the sheet is not standing in the document\'s place');
  assert.equal(bar().hidden, true, 'the parked bar survived the return, so it now sits behind the sheet claiming setup is paused');
  assert.equal(countCalls('/api/peers'), peersBefore,
    'the return leg re-fetched the peers, which means it rebuilt the picker. loadPeerPicker empties '
    + '#cerPeerPick BEFORE its fetch, so every checkbox state and every capacity typed and not yet '
    + 'blurred is destroyed — and where /api/peers answers nothing, the next change event posts an '
    + 'empty roster over the saved one');
  assert.equal(countCalls('/api/ceremony/draft'), draftBefore,
    'the return leg re-read the draft. A draft read is a REBUILD from the last committed value, so '
    + 'anything typed and not yet blurred is silently replaced by what the server had');

  const rowsAfter = Array.from(doc.querySelectorAll('#cerPeerPick .cerpeerrow'));
  assert.equal(rowsAfter.length, rowsBefore.length, 'the picker changed shape across the trip');
  assert.ok(rowsAfter.every((r, i) => r === rowsBefore[i]),
    'the picker rows are different elements, so the sheet was rebuilt rather than re-entered even '
    + 'if the values happen to match');
  assert.equal(rowsAfter[0].querySelector('.cerpeerbox').checked, true, 'the picked party came back unpicked');
  assert.equal(rowsAfter[0].querySelector('.cerpeercap').value, 'as tenant',
    'the capacity was lost. It was typed and never blurred, so no draft holds it — it exists only '
    + 'in the DOM, which is exactly what a re-entry preserves and a rebuild cannot');
  assert.equal(doc.getElementById('cerIntent').value, 'We agree to the lease of 14 Elm Row', 'the recital was lost');
  assert.equal(doc.getElementById('cerExpires').value, '2026-10-01T12:00', 'the deadline was lost');
  assert.equal(doc.activeElement?.id, 'cerIntent',
    'focus was not moved back into the sheet. It was on #cerBackToSetup, which the return has just '
    + 'hidden, so the returning user has no caret in the form she came back to');
});

test('the Convene button resumes a parked sheet instead of starting over', async () => {
  doc.getElementById('cerSeeDoc').click();
  await settle();
  assert.equal(bar().hidden, false, 'setup: nothing is parked, so "resumes" is vacuous');
  const peersBefore = countCalls('/api/peers');

  doc.getElementById('ceremonyConveneBtn').click();
  await settle();

  assert.equal(sheet().hidden, false, 'the Convene button did not bring the parked sheet back');
  assert.equal(countCalls('/api/peers'), peersBefore,
    'the Convene button rebuilt the parked sheet. It is the control the user reads as the way back '
    + 'to her ceremony, so it and the bar have to be two call sites of one rule');
  assert.equal(bar().hidden, true, 'the bar survived the resume');
});

test('abandoning setup clears the park, and a fresh entry really does rebuild', async () => {
  doc.getElementById('cerSeeDoc').click();
  await settle();
  assert.equal(bar().hidden, false, 'setup: nothing was parked, so clearing it proves nothing');

  // Any dismissal at all — this is the one that also hides the convene form, which is the pairing
  // a second writer of the sheet would break.
  doc.getElementById('ceremonyAcceptBtn').click();
  await settle();
  assert.equal(bar().hidden, true,
    'choosing another surface left the parked bar on screen, so the boolean and the control it '
    + 'drives disagree — and pressing it would raise the sheet over a hidden convene form');

  const peersBefore = countCalls('/api/peers');
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  // THE STIMULUS FLOOR for the two tests above: if a fresh entry made no request either, their
  // "no request" assertions would be true of a build that never fetched anything.
  assert.ok(countCalls('/api/peers') > peersBefore,
    'a fresh entry made no /api/peers request, so "the return leg makes none" is vacuous — it '
    + 'would be true of a build in which nothing ever rebuilds');
  assert.equal(doc.getElementById('ceremonyConveneForm').hidden, false,
    'the fresh entry did not show the convene form');
});

// **The park deliberately survives a mode change, and the return has to undo it.** Stepping out to
// mark the document up means going to Mark Up, so a park that died on a mode change would send the
// user back through the rebuild and lose the uncommitted values this slice exists to keep. But
// `syncSidebarForMode` puts the sheet away on leaving Collaborate, so a resume that did not go back
// there would raise a sheet D3 calls "owned by the Ceremony mode" over another mode's sidebar — the
// two rules contradicting each other. Found by a reviewer who had not read these tests.
test('the excursion survives a mode change, and coming back returns to the mode that owns the sheet', async () => {
  await openSheet();
  await typeSetup();
  doc.getElementById('cerSeeDoc').click();
  await settle();
  assert.equal(bar().hidden, false, 'setup: nothing was parked, so the mode change tests nothing');

  doc.querySelector('.modetab[data-tab="markup"]').click();
  await settle();
  assert.equal(bar().hidden, false,
    'the way back died on a mode change. Marking the document up IS the excursion, so the thread '
    + 'back has to survive going to Mark Up — otherwise the user re-enters through the rebuild and '
    + 'loses whatever she typed and never blurred');

  doc.getElementById('cerBackToSetup').click();
  await settle();
  assert.equal(doc.body.dataset.tab, 'collaborate',
    'the sheet came back in the wrong mode. It is owned by the Ceremony mode — syncSidebarForMode '
    + 'puts it away on leaving — so raising it over Mark Up\'s sidebar is two rules disagreeing');
  assert.equal(sheet().hidden, false, 'the sheet did not come back');
  assert.equal(doc.querySelector('#cerPeerPick .cerpeerrow .cerpeercap').value, 'as tenant',
    'the uncommitted capacity was lost, so the mode round trip went through the rebuild after all');
});

test('parking is impossible when there is no setup to park', async () => {
  doc.getElementById('cerSheetClose').click();
  await settle();
  assert.equal(sheet().hidden, true, 'setup: the sheet is still open, so this tests nothing');

  doc.getElementById('cerSeeDoc').click(); // the control is out of reach for a user; not for a test
  await settle();

  assert.equal(bar().hidden, true,
    'the bar came up with no setup open. `parked` is supposed to MEAN "the setup sheet is waiting", '
    + 'and a bar offering to return to a sheet nobody opened would raise an empty full-width surface');
  assert.equal(doc.getElementById('viewerWrap').hidden, false, 'the document was taken away by a park that should not have happened');
});

// ── P03's first exit criterion, driven at last: "setup survives closing and reopening Nib" ────
//
// **Found MISSING by the phase-close acceptance ledger, not by a slice.** P03.S02 built the draft
// and proved the BLOB survives a process restart — `TestTheDraftSurvivesTheProcess` stands up a
// second `Server` over the same HOME and asserts byte-identity. Nothing anywhere asserted the other
// half: that a stored draft is put back into the FORM. `restoreCeremonyDraft` appeared in this
// tier only inside source scans and inside P03.S04's "no rebuild happened" assertions — which
// observe it NOT running. A criterion whose two halves are each owned by a different tier is
// exactly the shape a per-slice ledger reads as covered.
//
// Reopening Nib is a fresh page against a server that still holds the draft, and this tier is the
// one that can express that: the process restart is tier 1's, already proved, and what remains is
// the client's read of it.
test("a stored draft is put back into the form — P03's first exit criterion, client half", async () => {
  // Nothing has restored anything so far in this file, so this is also the floor for the
  // assertions below: they cannot be satisfied by values a previous test left in the fields.
  doc.getElementById('cerSheetClose').click();
  await settle();
  doc.getElementById('cerIntent').value = '';
  doc.getElementById('cerExpires').value = '';
  for (const r of doc.querySelectorAll('#cerPeerPick .cerpeerrow')) {
    r.querySelector('.cerpeerbox').checked = false;
    r.querySelector('.cerpeercap').value = '';
  }

  storedDraft = JSON.stringify({
    intent: 'We agree to the lease of 14 Elm Row',
    expires: '2026-10-01T12:00',
    iSign: false,
    roster: [{ fingerprint: 'b'.repeat(64), capacity: 'as landlord', picked: true }],
  });
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  storedDraft = '';

  assert.equal(doc.getElementById('cerIntent').value, 'We agree to the lease of 14 Elm Row',
    'the recital did not come back, so a convener who closed Nib mid-setup starts again from blank '
    + '— which is the abandonment case D4 exists to end');
  assert.equal(doc.getElementById('cerExpires').value, '2026-10-01T12:00', 'the deadline did not come back');
  assert.equal(doc.getElementById('cerISign').checked, false,
    '"I sign this too" did not come back. It defaults CHECKED in the markup, so a restore that '
    + 'skipped booleans would silently add the convener to the roster of a ceremony they had '
    + 'decided not to sign');

  // The roster is the half that fails silently: the picker is rebuilt from /api/peers first, and a
  // restore matching on the wrong attribute finds no row and drops every party while the recital
  // and the deadline come back looking correct.
  const rows = [...doc.querySelectorAll('#cerPeerPick .cerpeerrow')];
  const picked = rows.filter((r) => r.querySelector('.cerpeerbox').checked);
  assert.equal(picked.length, 1,
    `${picked.length} parties came back picked, not 1. The picker is rebuilt from /api/peers before `
    + 'the restore runs, so a restore that matched on the wrong attribute would find no row and '
    + 'drop the whole roster — leaving the recital and the deadline looking restored');
  assert.equal(picked[0].querySelector('.cerpeerbox').dataset.fingerprint, 'b'.repeat(64),
    'the wrong party came back picked');
  assert.equal(picked[0].querySelector('.cerpeercap').value, 'as landlord',
    'the capacity did not come back. D20 makes capacity part of the agreement rather than a label, '
    + 'so a roster restored without it has lost what each party is signing AS');
});

// ── Two defects the phase-close review found in code four slices had already reviewed ─────────

test('emptying the form returns it to how it OPENS, not to all-false', async () => {
  await openSheet();
  const iSign = doc.getElementById('cerISign');
  // **`defaultChecked`, not `checked`** — it reflects the HTML attribute whatever the live state
  // is, so this floor is order-independent. Written as `checked` first and it failed here, because
  // the test above had left the box unchecked from a restored draft: a floor that reads live state
  // measures the test that ran before it.
  assert.equal(iSign.defaultChecked, true,
    'setup: #cerISign does not ship checked, so "returns to the markup default" is about nothing '
    + 'and the assertion below would pass for the wrong reason');

  doc.getElementById('cerIntent').value = 'We agree to the lease of 14 Elm Row';
  doc.getElementById('cerExpires').value = '2027-10-01T12:00';
  doc.querySelector('#cerPeerPick .cerpeerbox').checked = true;
  doc.getElementById('ceremonyConveneForm').dispatchEvent(new doc.defaultView.Event('submit', { bubbles: true, cancelable: true }));
  await settle();

  assert.equal(doc.getElementById('cerIntent').value, '', 'the recital survived a successful convene');
  assert.equal(iSign.checked, iSign.defaultChecked,
    'after convening, "I sign this too" is UNCHECKED — so the next ceremony in this session '
    + 'defaults to the convener not signing. The server seats them at roster position 0 with '
    + 'Signs:false and the invitations screen lists only invitees, so nothing on screen tells them '
    + 'they left themselves out. Emptying a form means returning it to how it opens, which for a '
    + 'checkbox is not the same as clearing it');
});

test('leaving the Ceremony mode PARKS the setup rather than dropping the thread', async () => {
  await openSheet();
  doc.getElementById('cerIntent').value = 'We agree to the lease of 14 Elm Row';
  doc.querySelector('#cerPeerPick .cerpeercap').value = 'as tenant';
  await settle();

  doc.querySelector('.modetab[data-tab="markup"]').click();
  await settle();
  assert.equal(sheet().hidden, true, 'setup: the sheet survived the mode change, so this tests nothing');
  assert.equal(bar().hidden, false,
    'leaving the mode dropped the thread back to a half-filled ceremony. This is the path the '
    + 'excursion exists for — finishing markup before convening — and a user who reaches Mark Up '
    + 'by the mode tab rather than by "See the document" got no way back but the rebuild');

  const peersBefore = countCalls('/api/peers');
  doc.getElementById('cerBackToSetup').click();
  await settle();
  assert.equal(sheet().hidden, false, 'the way back did not bring the sheet back');
  assert.equal(doc.body.dataset.tab, 'collaborate', 'the sheet came back over another mode\'s sidebar');
  assert.equal(countCalls('/api/peers'), peersBefore,
    'coming back from a mode change rebuilt the picker, so the mode exit and the button exit are '
    + 'still two different mechanisms');
  assert.equal(doc.querySelector('#cerPeerPick .cerpeercap').value, 'as tenant',
    'the uncommitted capacity was lost across the mode round trip');
});

// ── /pending 414 — an open the user walked away from must not write to the sheet ──────────────
//
// Opening the setup is two GETs deep, and the sheet is live the whole time: "See the document",
// "Back to setup", and typing are all reachable before either answers. The entry found the picker's
// clear running BEFORE its fetch, so a load that was slow, that failed, or that belonged to an open
// the user had already left emptied the picker anyway. Underneath that sat the wider case — the
// draft restore landing on a form somebody had started filling in, and putting the saved copy back
// over it.
//
// **Two rules, and they cover different halves.** `ceremonySetupGen` says whether the sheet is
// still this open's; it cannot say whether the user has typed into it, which is what the restore's
// own pristine test is for. Each is asserted separately below, because an assertion that passes
// through both cannot say which one is carrying it.
test('a slow open does not empty the picker it is still loading', async () => {
  storedDraft = '';
  let release;
  peersGate = new Promise((r) => { release = r; });
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  // Fill the picker once, from a fast load, so there is something to destroy.
  peersGate = null;
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.equal(doc.querySelectorAll('#cerPeerPick .cerpeerrow').length, 2,
    'the picker never filled, so this test cannot see a clear that destroys it — the stimulus '
    + 'floor, not the assertion');
  doc.querySelector('#cerPeerPick .cerpeercap').value = 'as attorney-in-fact';
  // Cancel, which is a dismissal, then start a fresh open that never answers.
  doc.getElementById('cerConveneCancel').click();
  await settle();
  peersGate = new Promise((r) => { release = r; });
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  // The load is in flight. Nothing may have been emptied yet.
  assert.equal(doc.querySelectorAll('#cerPeerPick .cerpeerrow').length, 2,
    'the picker was emptied before its fetch answered, so an unfinished open destroys the roster '
    + 'state of the one before it — including a capacity typed and not yet blurred');
  release();
  await settle();
  peersGate = null;
});

test('a setup the user walked away from does not write over what they came back and typed', async () => {
  storedDraft = JSON.stringify({
    intent: 'the recital saved in the draft', expires: '', iSign: false, roster: [],
  });
  doc.getElementById('cerConveneCancel').click();
  await settle();
  doc.getElementById('cerIntent').value = '';
  let release;
  peersGate = new Promise((r) => { release = r; });
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.equal(doc.getElementById('cerIntent').value, '',
    'the draft was already restored while /api/peers was still held open, so this test is not '
    + 'measuring the window it claims to — the stimulus floor');
  // Step out to the document and come back, which is the whole excursion P03.S04 built.
  doc.getElementById('cerSeeDoc').click();
  await settle();
  doc.getElementById('cerBackToSetup').click();
  await settle();
  assert.equal(sheet().hidden, false, 'the return leg did not put the sheet back, so nothing below '
    + 'is about a sheet the user is looking at');
  // The user types. Now let both opens answer.
  doc.getElementById('cerIntent').value = 'what the convener is typing right now';
  release();
  await settle();
  assert.equal(doc.getElementById('cerIntent').value, 'what the convener is typing right now',
    'the saved draft was written over live input — the form reverted under the user, which is '
    + 'exactly the state D4 makes the draft a recovery FROM rather than an authority OVER');
  storedDraft = '';
  peersGate = null;
});

// The generation's own case, which neither test above reaches: TWO opens in flight, the stale one
// answering last. Clearing after the fetch makes `loadPeerPicker` atomic, and atomic is not the
// same as addressed — an abandoned open that answers late still replaces a live picker's contents
// with its own, and every checkbox and unblurred capacity in it goes.
test('an abandoned open that answers after a newer one does not replace its picker', async () => {
  // A draft naming the SAME peer the user is about to type a capacity for, so the abandoned open's
  // restore has something to write and the picker's own guard is not the only one under test. The
  // recital stays empty on purpose: the pristine test refuses a restore over typed text, and a
  // draft that tripped it would carry this assertion on the wrong mechanism.
  storedDraft = JSON.stringify({
    intent: '', expires: '', iSign: false,
    roster: [{ fingerprint: 'a'.repeat(64), capacity: 'from the stale draft', picked: true }],
  });
  doc.getElementById('cerConveneCancel').click();
  await settle();
  // Cleared because the test above left the convener mid-sentence, and a non-empty recital would
  // send every restore below down the pristine test's early return — which would carry this
  // assertion on a mechanism it is not about, and leave the generation check red nowhere.
  doc.getElementById('cerIntent').value = '';
  // Open A, held.
  let releaseA;
  peersGate = new Promise((r) => { releaseA = r; });
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  // Abandon A and open B, which answers at once.
  doc.getElementById('cerConveneCancel').click();
  await settle();
  peersGate = null;
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.equal(doc.querySelectorAll('#cerPeerPick .cerpeerrow').length, 2,
    'open B never filled the picker, so there is nothing for open A to destroy — the stimulus floor');
  doc.querySelector('#cerPeerPick .cerpeercap').value = 'as sole trustee';
  // Now A answers.
  releaseA();
  await settle();
  assert.equal(doc.querySelector('#cerPeerPick .cerpeercap').value, 'as sole trustee',
    'the abandoned open replaced the live picker when it finally answered, taking the capacity '
    + 'typed into it — atomic is not the same as belonging to this sheet');
  storedDraft = '';
});

// **The park does not cancel the open, and that is a decision this pins rather than a side effect.**
// The obvious reading of the entry is that stepping out abandons whatever the open still owes — and
// building it that way costs a second flag and a re-finish on the return leg, because a cancelled
// open leaves an empty picker and the return leg is forbidden to rebuild. Letting it complete
// instead is both simpler and better: the roster is there when the user comes back. The one thing
// the excursion must not license is the saved draft landing on top of what they have since typed,
// and the restore's own pristine test is what refuses that.
test('a slow open completes across a step out to the document and back', async () => {
  storedDraft = '';
  doc.getElementById('cerConveneCancel').click();
  await settle();
  // Emptied by the harness, not by the app — since the fix, nothing clears the picker until a load
  // has an answer to put there, so the previous test's rows are still in it. The point of this test
  // is what fills it, so it has to start empty.
  doc.getElementById('cerPeerPick').textContent = '';
  let release;
  peersGate = new Promise((r) => { release = r; });
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.equal(doc.querySelectorAll('#cerPeerPick .cerpeerrow').length, 0,
    'the picker filled while /api/peers was still held, so this test is not about an unfinished '
    + 'open at all — the stimulus floor');
  doc.getElementById('cerSeeDoc').click();   // parks, and invalidates the open in flight
  await settle();
  doc.getElementById('cerBackToSetup').click();
  await settle();
  release();
  await settle();
  assert.equal(doc.querySelectorAll('#cerPeerPick .cerpeerrow').length, 2,
    'the picker is still empty after the return leg, so the convener has a setup sheet with no '
    + 'roster to choose from and no control that would fill it — Convene resumes rather than '
    + 'rebuilds, which is that clause working, so nothing else would ever fill it');
  peersGate = null;
});

// The nothing-bound half of /pending 413, and it has to be declared HERE — above the test that
// opens a document — because after that there is no way back to a session with nothing open.
test('with nothing open the sheet says so, rather than waiting until submit', async () => {
  storedDraft = '';
  peersGate = null;
  doc.getElementById('cerConveneCancel').click();
  await settle();
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  const line = doc.getElementById('cerSheetDoc');
  assert.ok(line, 'the sheet has no #cerSheetDoc at all, so nothing below is asserted');
  assert.equal(line.classList.contains('cerdocnone'), true,
    'nothing is open and the sheet is not saying so — the convener fills in the whole form and '
    + 'learns at submit, from a 404 about a document');
  assert.match(line.textContent, /See the document/,
    'the empty state does not name the way out of it. This button is deliberately NOT in '
    + 'DOC_REQUIRED so that opening the lease from inside setup is a supported path, which only '
    + 'works if the sheet says that is what to do');
  assert.doesNotMatch(line.textContent, /built from/,
    'the sheet is claiming a document while none is bound');
  doc.getElementById('cerConveneCancel').click();
  await settle();
});

// ── /pending 412 — a placement tool armed on a page the sheet is covering ──────────────────────
//
// The Flags panel and the Ceremony panel are two panels of ONE mode, and the tab that switches
// between them touches the sheet not at all. So Sign could be lit, and `#viewerWrap` given a
// crosshair, while the sheet had that element under `display: none` — a tool armed with no surface
// to draw on and no feedback, which reads as the button not working.
test('arming a placement tool steps out of the setup sheet', async () => {
  storedDraft = '';
  peersGate = null;
  doc.getElementById('cerConveneCancel').click();
  await settle();
  // `.markers button` is disabled with nothing open (`setDocControls`), so a test that skipped this
  // would click a dead control and assert nothing.
  setNextDocument({ numPages: 3 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/lease.pdf';
  doc.getElementById('openGo').click();
  await settle();
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.equal(sheet().hidden, false,
    'the sheet is not up, so nothing below is about a tool armed underneath one — the stimulus floor');
  const sign = doc.querySelector('.markers button[data-marker="sign"]');
  assert.ok(sign, 'the Flags panel has no Sign control, so this test arms nothing');
  assert.equal(sign.disabled, false,
    'the Sign control is disabled, so the click below is a no-op and the assertion is vacuous — '
    + 'the second half of the stimulus floor');
  sign.click();
  await settle();
  assert.equal(sign.classList.contains('active'), true,
    'the tool did not arm at all, so this test cannot tell a sheet that stood aside from a press '
    + 'that did nothing');
  assert.equal(sheet().hidden, true,
    'the tool is armed and the sheet is still covering the page it would draw on — the crosshair '
    + 'is on an element under display:none');
  assert.equal(bar().hidden, false,
    'the sheet went away without the parked-setup bar, so the convener has lost the setup with no '
    + 'way back to it — a park is not a dismissal');
  // Put it back the way the rest of the file expects.
  sign.click();
  await settle();
  doc.getElementById('cerBackToSetup').click();
  await settle();
  doc.getElementById('cerConveneCancel').click();
  await settle();
});

// ── /pending 413 — the sheet names the document it will build the ceremony from ────────────────
//
// The sheet stands IN PLACE of the page, so the convener writes the recital, sets the deadline and
// picks the roster with the file off screen. Worse, `#ceremonyConveneBtn` is deliberately NOT in
// `DOC_REQUIRED` — `bindCeremonySetupDoc` supports "I opened the sheet, then went and opened the
// lease" — so with nothing open the whole form could be filled in and the answer arrived at SUBMIT,
// as a 404 about a document. The line says which, including when the answer is "none yet".
test('the sheet says which document the ceremony will be built from', async () => {
  const line = () => doc.getElementById('cerSheetDoc');
  assert.ok(line(), 'the sheet has no #cerSheetDoc at all, so nothing below is asserted');
  // The bound case — the test above left `lease.pdf` open and the sheet cancelled.
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  assert.match(line().textContent, /lease\.pdf/,
    'the sheet does not name the document, so the convener decides the recital and the roster '
    + 'with the file they are deciding about off screen');
  assert.match(line().textContent, /3 pages/,
    'the sheet names the file and not its size, so "is this the right one" is answered by a '
    + 'filename alone — the page count is what tells a two-page draft from a signed 40-page lease');
  assert.match(line().textContent, /finish any markup before you convene/,
    'nothing on the sheet says that convening fixes the document, and it does: convene appends '
    + 'the ceremony and signature pages and takes the hash after them, so markup is a '
    + 'PRECONDITION and this surface presents Convene as the obvious next press');
  assert.equal(line().classList.contains('cerdocnone'), false,
    'a bound sheet is wearing the nothing-open styling');
  // The filename arrives from disk, so it is a text node and never markup.
  assert.equal(line().querySelector('b').textContent, 'lease.pdf',
    'the name is not in its own element, so it cannot be told apart from the sentence around it');
  // The pin follows the BOUND document, not the active one. Open a second document behind the
  // sheet and come back: the line must still name the file the ceremony is for. Reading the active
  // view instead would put a confident filename on the sheet that is not the file it will use,
  // which is worse than the silence this replaced.
  nextOpen = {
    id: 'test-epoch:2', name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf', canSave: true,
    signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
  };
  setNextDocument({ numPages: 9 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
  doc.getElementById('openGo').click();
  await settle();
  doc.getElementById('cerSeeDoc').click();
  await settle();
  doc.getElementById('cerBackToSetup').click();
  await settle();
  assert.match(line().textContent, /lease\.pdf/,
    'the sheet followed the active document to deed.pdf, so it names a file the ceremony will not '
    + 'use — the pin is on lease.pdf and the refusal would arrive at submit');
  assert.doesNotMatch(line().textContent, /deed\.pdf/,
    'the sheet is naming the document the user switched to rather than the one it is bound to');
  doc.getElementById('cerConveneCancel').click();
  await settle();
});
