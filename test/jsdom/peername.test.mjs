// P01.S02: the pairing surfaces show a six-word name, and the hex fingerprint is behind
// an advanced disclosure rather than merely moved down the panel.
//
// The distinction is the phase's first exit criterion, settled by its plan review (W7):
// "normal pairing path" means the default pairing screen, and hex must be "reachable only
// behind the advanced disclosure, **never merely de-emphasised in place**". The slice's
// own scope line said "hex moves to a secondary position", which the review noted admits
// a second reading in which hex is still on screen — so this test asserts the stronger
// one, because that is the one the plan settled on.
//
// ── Absence AND presence, in that order ─────────────────────────────────────
// An absence assertion on its own is the classic vacuous green: "no 64-hex string in the
// rendered text" passes just as well against a panel that never rendered, a fetch that
// failed, or an element id that was renamed. So each check here first proves the panel
// is populated — the name is on screen — then proves hex is not in the open text, then
// opens the disclosure and proves it IS there. A guard that cannot distinguish
// "successfully hidden" from "never drawn" is not a guard.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';

const SELF_FP = 'aa'.repeat(32);
const PEER_FP = 'bb'.repeat(32);
// The names the server would derive. This tier stubs /api/peers, so the words themselves
// are the harness's, not pairing.Name's — what is under test here is the CLIENT's
// rendering, and internal/server's own tests assert the derivation.
const SELF_NAME = 'harbour candle mirror thicket pledge willow';
const PEER_NAME = 'anchor bramble kettle orchard signal timber';

const HEX_RUN = /\b[0-9a-f]{64}\b/i;
// The panel groups hex into spaced quads for comparison, so the raw 64-run does not
// appear as one token on screen — this is the form a reader actually meets.
const GROUPED_RUN = /(?:\b[0-9a-f]{4}\b[ \t]+){8,}/i;

const peersPayload = {
  fingerprint: SELF_FP,
  name: SELF_NAME,
  peers: [{ fingerprint: PEER_FP, name: PEER_NAME }],
};

// The stub table hands back the BODY, not a {status, json} envelope — an envelope is
// serialised as the body and every field reads undefined, which is how the first run of
// this file failed. `loadExtSigner` rides along on the same click, so it is stubbed too.
const { window } = await boot({
  routes: {
    '/api/peers': peersPayload,
    '/api/identity/external': { present: false },
  },
});
const doc = window.document;

// visibleText returns an element's text with the contents of every closed <details>
// removed — which is what "a user never sees" means. Reading textContent alone would
// count the disclosure's contents as on-screen and the test would fail against a correct
// implementation, which is the failure mode that teaches people to weaken assertions.
function visibleText(root) {
  const clone = root.cloneNode(true);
  for (const d of clone.querySelectorAll('details:not([open])')) d.remove();
  return clone.textContent.replace(/\s+/g, ' ');
}

// openPeersPanel drives the real control — the app's own click handler calls loadPeers(),
// which fetches the stubbed route and runs renderPeers. Populating the elements by hand
// and then asserting they are populated would assert this file's own assignment; the
// point is that the APP puts the name there.
async function openPeersPanel(reload = false) {
  const el = doc.getElementById('peerSelfName');
  if (reload) el.textContent = '';
  doc.getElementById('managePeersBtn').click();
  for (let i = 0; i < 50 && !el.textContent; i++) {
    await new Promise((r) => setTimeout(r, 5));
  }
}

test('the identity panel leads with the name and hides hex behind the disclosure', async () => {
  const panel = doc.getElementById('peersModal');
  assert.ok(panel, 'setup: the Identity & peers panel is not in the document');

  const nameEl = doc.getElementById('peerSelfName');
  const fpEl = doc.getElementById('peerSelfFp');
  assert.ok(nameEl, 'no #peerSelfName element — the panel has no place to show a name');
  assert.ok(fpEl, 'setup: #peerSelfFp is gone; hex must still be REACHABLE, not deleted');

  await openPeersPanel();

  // The stimulus, asserted rather than assumed: the app populated the panel from the
  // payload. Without this the absence check below passes against a panel that never
  // rendered — which is the same green a broken fetch produces.
  assert.equal(nameEl.textContent, SELF_NAME,
    `the app did not render the name from the payload (got ${JSON.stringify(nameEl.textContent)}) ` +
    `— so nothing below is measuring what a user sees`);
  assert.ok(fpEl.textContent.includes('aaaa'),
    'the app did not render the fingerprint into the disclosure either, so the panel is empty');

  const open = visibleText(panel);
  assert.ok(open.includes(SELF_NAME),
    `the panel does not show the six-word name. Visible text was: ${open.slice(0, 200)}`);
  assert.ok(!HEX_RUN.test(open) && !GROUPED_RUN.test(open),
    `a fingerprint is visible on the default pairing screen without opening anything. ` +
    `The phase criterion is that hex is reachable only behind the advanced disclosure, ` +
    `never merely de-emphasised in place. Visible text: ${open.slice(0, 240)}`);

  // …and it really is reachable, which is the half that stops this being satisfied by
  // deleting the fingerprint outright.
  const details = fpEl.closest('details');
  assert.ok(details, '#peerSelfFp is not inside a <details> — it is hidden by something ' +
    'that is not a disclosure, so there is no way for a user to reach it');
  details.setAttribute('open', '');
  const opened = visibleText(panel);
  assert.ok(GROUPED_RUN.test(opened) || HEX_RUN.test(opened),
    'opening the disclosure did not reveal the fingerprint');

  // Close it again. One DOM per file (boot.mjs), so an opened disclosure left behind is
  // this file's own state leaking into the next test — which is how the fallback test
  // below first failed, reporting an app defect that was this line's doing.
  details.removeAttribute('open');
});

test('a missing name still does not put hex on the default screen', async () => {
  // The branch the diff review found: the first implementation fell back to the grouped
  // fingerprint when the payload carried no name, which puts hex on the default pairing
  // screen in exactly the case the criterion exists for. A criterion that holds only on
  // the happy path is not a criterion.
  //
  // Rendered directly rather than through a second boot: one boot per file (boot.mjs),
  // and what is under test is renderPeers' fallback, which is reached by the same call.
  const nameEl = doc.getElementById('peerSelfName');
  const before = nameEl.textContent;
  assert.ok(before && before !== '(name unavailable)',
    'setup: the panel is not showing a real name, so the fallback below proves nothing');

  peersPayload.name = '';
  peersPayload.peers = [{ fingerprint: PEER_FP }];
  await openPeersPanel(true);

  const panel = doc.getElementById('peersModal');
  const open = visibleText(panel);
  assert.ok(!HEX_RUN.test(open) && !GROUPED_RUN.test(open),
    `with no name in the payload the panel fell back to showing a fingerprint: ` +
    `${open.slice(0, 240)}`);
  assert.ok(open.includes('(name unavailable)'),
    'the name slot is blank rather than saying the name is missing');

  peersPayload.name = SELF_NAME;
  peersPayload.peers = [{ fingerprint: PEER_FP, name: PEER_NAME }];
});

test('the serve dialog is a pairing surface too and gets the same treatment', async () => {
  // This is the site the slice was originally scoped past: "the UI" reads as one screen,
  // and the listen/serve dialog shows the user's own fingerprint under its own id.
  const nameEl = doc.getElementById('srvSelfName');
  const fpEl = doc.getElementById('srvSelfFp');
  assert.ok(nameEl, 'the serve dialog has no #srvSelfName — it still identifies you by hex only');
  assert.ok(fpEl, 'setup: #srvSelfFp is gone');

  // The serve dialog's loader is behind a flow this tier cannot reach (it needs an open
  // document and the collaborate mode), so this file asserts its STRUCTURE — the name
  // element exists and the fingerprint is behind a disclosure — and says so rather than
  // dressing a structural check as a behavioural one. The population path is the same
  // `data.name` read, covered by the panel test above.
  const details = fpEl.closest('details');
  assert.ok(details, '#srvSelfFp is not behind a disclosure, so the serve dialog still ' +
    'shows a fingerprint on the default screen while the other surface does not');

  // Nothing outside the disclosure may carry a fingerprint. Checked on the markup as
  // shipped, with the disclosure closed — its contents are removed by visibleText.
  const holder = details.parentElement;
  const open = visibleText(holder);
  assert.ok(!GROUPED_RUN.test(open) && !HEX_RUN.test(open),
    `the serve dialog shows a fingerprint before anything is opened: ${open.slice(0, 200)}`);
});

// The addressing rule, asserted at the SOURCE and labelled as such.
//
// **The first version of this test was vacuous and a probe caught it.** It built its own
// <option>, set value and text itself, and asserted them back — so swapping the app's
// `o.value = p.fingerprint` to `p.name` left it green. It was asserting its own
// assignment, which is the exact defect this file's header claims to avoid; the header
// was written before the probe ran.
//
// Driving the real population needs an open document (`openCosign` returns early without
// one) and that is above this tier's ceiling. So this is a source scan, named as a source
// scan — a structural check dressed as a behavioural one reads as coverage it has not got.
// The population floor is what makes it more than a grep: four peer <select>s exist, and
// a fifth added without the rule, or one of the four deleted, both fail here.
test('every peer <option> is addressed by fingerprint, never by name', () => {
  const src = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

  // L1 first, because it is the more serious diagnosis and the population floor below
  // would otherwise mask it: swapping one site's value to a name lowers the count, and a
  // count failure reads as "a dropdown lost its addressing" rather than "a name is being
  // used as an address". Measured — the first ordering reported exactly that wrong cause.
  //
  // An option value IS the address posted to /api/cosign, the serve routes and send. A
  // name there would make the six-word display identity into an identifier, which is the
  // design D31 removed.
  const byName = src.match(/o\.value = [^;]*p\.name/g) || [];
  assert.deepEqual(byName, [],
    `a peer <option> is addressed by NAME: ${byName.join(', ')}. L1 forbids a name ` +
    `resolving a pin, and the six-word name commits to only the leading 66 bits of a ` +
    `fingerprint — two keys sharing those bits would address the same peer.`);

  const byFingerprint = src.match(/o\.value = p\.fingerprint;/g) || [];
  assert.equal(byFingerprint.length, 4,
    `expected 4 peer <option> value assignments (co-sign, sign-in-person, serve, send), ` +
    `found ${byFingerprint.length}. A new one must address by fingerprint like the others; ` +
    `a missing one means a dropdown lost its addressing.`);

  // And the labels no longer carry truncated hex, which is what the criterion is about.
  const truncated = src.match(/groupFingerprint\(p\.fingerprint\.slice/g) || [];
  assert.deepEqual(truncated, [],
    'a dropdown label still shows a truncated fingerprint — eight hex characters is ' +
    'neither readable nor speakable, and the name is what a person can check');
});
