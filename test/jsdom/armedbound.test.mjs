// The armed pill says what its figure is a bound ON (/pending 366).
//
// `until` is the ARM's own bound, and for a ceremony arm that is `ceremony.MaxCeremonyLife` —
// thirty days — because an invitation carries no deadline (`/pending 247`). So a user arming for a
// two-day proceeding was told they were "armed until" a date a month away: a true sentence about
// the arm and a misleading one about the ceremony, indistinguishable on that pill. The
// proceeding's own deadline is the ceremonies panel's "Open until", a few pixels away and correct.
//
// **The figure itself is NOT the defect and must survive**, which is why this file asserts the
// date is still rendered. It was added at C05 precisely so a five-minute manual bound and a
// thirty-day ceremony bound are distinguishable — deleting it to fix the wording would trade one
// ambiguity for the one it replaced.
//
// **What this can and cannot check.** The property is a sentence, so the assertion is over prose
// and is worth exactly what a prose assertion is worth: it catches the wording being reverted or
// dropped, and it cannot tell whether a reader understands it. That is stated here rather than
// dressed up, and it is the honest ceiling on a fix whose whole content is what the tooltip says.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
// Thirty days out: the ceremony-arm bound, and the figure that made the old wording misleading.
const UNTIL = new Date(Date.now() + 30 * 24 * 3600 * 1000).toISOString();

let armed = false;
const h = await boot({
  routes: {
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => ({ armed, address: '127.0.0.1:8443', until: armed ? UNTIL : undefined }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

test('the armed pill names the arm as what its date bounds, not the ceremony', async () => {
  const pill = doc.getElementById('armedPill');
  assert.ok(pill, 'there is no #armedPill in index.html');

  doc.getElementById('sessionRecvBtn').click();
  await settle();
  const peerSel = doc.getElementById('srvPeer');
  peerSel.selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  // The title comes from `pollRecv`, not from the arm response — index.html ships a base title and
  // the poll replaces it. **1800 ms and not 1400**: the poll interval is 1500 ms, so a wait shorter
  // than one interval measures the static attribute and passes on the wrong thing. Same figure and
  // same reason as armprogress.test.mjs.
  await new Promise((r) => setTimeout(r, 1800));
  await settle();
  const title = pill.title;
  const shown = pill.hidden;

  // Observe, clean up, THEN assert — an armed session leaves a repeating poll timer, and a file
  // that leaves one does not fail, it HANGS. armed.test.mjs measured both of the other orderings.
  doc.getElementById('srvCancel').click();
  await settle();

  assert.equal(shown, false, 'setup: the pill is not showing, so its title describes nothing');
  assert.ok(title, 'setup: the pill has no title at all — `until` never reached it, so nothing ' +
    'below is being tested');
  assert.match(title, /\d/,
    'the title carries no date. The figure is C05’s and is the whole reason a five-minute ' +
    'manual bound and a thirty-day ceremony bound are distinguishable at all — the fix for the ' +
    'wording must not remove it');
  assert.match(title, /not the ceremony deadline/,
    'the title states a date without saying what it bounds. For a ceremony arm that date is ' +
    'thirty days out (MaxCeremonyLife, because an invitation carries no deadline), so a user ' +
    'arming for a two-day proceeding reads it as the proceeding’s deadline — which is on ' +
    'screen a few pixels away, correct, and different');
});
