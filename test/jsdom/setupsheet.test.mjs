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

// Two peers, because P03.S04's clause is about the ROSTER surviving a round trip and an empty
// picker has no roster to lose. The three tests that predate it assert nothing about the picker.
const { document: doc, settle, calls } = await boot({
  routes: {
    '/api/peers': () => ({
      self: 'f'.repeat(64),
      peers: [
        { fingerprint: 'a'.repeat(64), label: 'lively otter marble finch amber cove' },
        { fingerprint: 'b'.repeat(64), label: 'quiet heron ribbon plum copper lane' },
      ],
    }),
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
