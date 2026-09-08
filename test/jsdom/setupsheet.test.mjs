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

const { document: doc, settle } = await boot({
  routes: { '/api/peers': () => ({ self: 'f'.repeat(64), peers: [] }) },
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
    + 'moments, and setup is neither — it stands IN PLACE of the document, which is also why '
    + '"placing blocks leaves the sheet for the page" works at all');
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
