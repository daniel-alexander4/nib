// The signature panel says whether every signature commits to the SAME ceremony.
//
// ── The defect this is written against ──────────────────────────────────────
// `SignerAttestation.OneProceeding` is computed in Go, serialized to the client as
// `oneProceeding`, and was rendered NOWHERE. Its own doc comment states the harm exactly:
//
//   "A verifier that said only 'co-signed' about such a document would be describing a
//    proceeding that did not happen."
//
// Which is what the panel did. `augmentSigDetails` printed "✓ Mutually co-signed" whenever
// every attestation was `matched`, and `matched` is per-pair — it says each signature
// attests to the other's KEY. It says nothing about whether both signers agreed to the same
// proceeding. Two parties can each hold a valid mutual co-signature while their signatures
// carry different Ceremony Record commitments.
//
// Found by `observables_test.go`, which scans Go-side published fields for one with no
// reader — the Go twin of `published.test.mjs`, and this was its first find.
//
// ── Three states, and the third is the reason a bare `!oneProceeding` is wrong ─────
// `oneProceeding` is false for an ORDINARY two-party co-sign as well, because such a
// document carries no Ceremony Record and `markOneProceeding` returns early on an empty
// commitment. So the three cases below are not decoration: a test that only drove the
// disagreeing case would pass just as well against `if (!a.oneProceeding) warn()`, which
// would slander every ordinary co-signed document in the product.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const A = 'aa'.repeat(32);
const B = 'bb'.repeat(32);
const ROSTER = 'c0ffee'.repeat(10) + 'abcd';
const OTHER = 'dec0de'.repeat(10) + 'abcd';

// Two signers who each accept the other: `matched` true on both, which is what makes the
// "Mutually co-signed" line fire and is therefore the case that must be distinguished.
const att = (fp, peer, rosterHash, oneProceeding) => ({
  signer: 'S-' + fp.slice(0, 2), fingerprint: fp, acceptedPeer: peer,
  reason: 'Accepted [SPKI:' + peer + ']', when: '2026-08-21T10:00:00Z',
  valid: true, matched: true, pinned: false,
  ...(rosterHash ? { rosterHash } : {}),
  ...(oneProceeding ? { oneProceeding } : {}),
});

// One boot, one opened document, and only the ATTESTATIONS vary. The signer rows are
// identical in all three cases — what is under test is the verdict `augmentSigDetails`
// draws from the attestations, so holding everything else fixed is what makes the three
// outcomes attributable to the one input that differs.
let attestations = [];
const { window, document: doc, settle } = await boot({
  routes: {
    '/api/attestations': () => ({ attestations }),
    '/api/open': {
      id: 'test-epoch:1', name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf',
      canSave: false, canUndo: false, canRedo: false,
      // 'valid' is what internal/sign emits (verify.go: unsigned | valid | invalid).
      // A fixture that says 'signed' drives updateBadge's fallback arm and the details
      // button stays hidden — arrival.test.mjs records the same trap, and this file hit it.
      signature: {
        state: 'valid',
        signers: [
          { name: 'S-aa', valid: true, when: '2026-08-21T10:00:00Z' },
          { name: 'S-bb', valid: true, when: '2026-08-21T10:01:00Z' },
        ],
      },
    },
  },
});

// The document is opened through the app's own control, once, before any case runs.
setNextDocument({ numPages: 1 });
doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
doc.getElementById('openGo').click();
await settle();

// panelText drives the REAL control and returns what a reader sees. `openSigDetails`
// returns early unless the app already holds signers — that is the app's own precondition,
// reached by opening a signed document rather than by reaching into module state.
async function panelText(next) {
  attestations = next;
  const body = doc.getElementById('sigDetailsBody');
  body.innerHTML = '';
  doc.getElementById('sigDetailsBtn').click();
  for (let i = 0; i < 80 && !body.querySelector('.sigatt'); i++) {
    await new Promise((r) => setTimeout(r, 5));
  }
  return body.textContent.replace(/\s+/g, ' ');
}

test('an ordinary co-sign is not accused of anything', async () => {
  const txt = await panelText([att(A, B, null, false), att(B, A, null, false)]);

  // STIMULUS: the panel really rendered the attestations. Without this the two absence
  // assertions below pass against a panel that never drew — the exact green a broken
  // fetch produces, and the reason this file asserts presence before absence.
  assert.match(txt, /Accepts a co-signer/,
    `the panel did not render any attestation, so nothing below is measuring what a user sees: ${txt.slice(0, 200)}`);
  assert.match(txt, /Mutually co-signed/,
    'the mutual line did not fire, so this case is not the one that needs distinguishing');

  // No document carries a ceremony here, so there is no proceeding to agree about and the
  // panel must say nothing either way. A warning here would slander every ordinary
  // co-signed document — which is exactly what reading `!oneProceeding` alone would do.
  assert.ok(!/Not one proceeding/.test(txt),
    `an ordinary two-party co-sign was accused of not being one proceeding: ${txt}`);
  assert.ok(!/One proceeding/.test(txt),
    `an ordinary co-sign claimed a proceeding it never had: ${txt}`);
});

test('signatures that agree on the ceremony are reported as one proceeding', async () => {
  const txt = await panelText([att(A, B, ROSTER, true), att(B, A, ROSTER, true)]);
  assert.match(txt, /Accepts a co-signer/, 'setup: the panel rendered nothing');
  assert.match(txt, /✓ One proceeding/,
    `signatures carrying one roster commitment were not reported as one proceeding: ${txt}`);
  assert.ok(!/Not one proceeding/.test(txt), `contradictory verdicts in one panel: ${txt}`);
});

test('signatures naming DIFFERENT ceremonies are reported, not summarised away', async () => {
  // The defect, exactly: both matched, both valid, mutually co-signed — and committing to
  // two different proceedings. This used to render "✓ Mutually co-signed" and nothing else.
  const txt = await panelText([att(A, B, ROSTER, false), att(B, A, OTHER, false)]);

  assert.match(txt, /Accepts a co-signer/, 'setup: the panel rendered nothing');
  // The mutual line is still TRUE and must still appear — the fix adds a statement, it does
  // not suppress one. Asserting its absence would be asserting a different, wrong design.
  assert.match(txt, /Mutually co-signed/,
    'the mutual line vanished; each signature does still attest to the other key');

  assert.match(txt, /⚠ Not one proceeding/,
    `two signatures naming DIFFERENT ceremonies were summarised as a co-signed document ` +
    `with no qualification — the verifier describing a proceeding that did not happen: ${txt}`);
  assert.match(txt, /do not all commit to the same one/,
    `the warning does not say what is wrong, so a reader is alarmed and given nothing: ${txt}`);
});
