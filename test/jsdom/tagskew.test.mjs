// A signature this build cannot READ is not a signature that disagrees (P07.S09c, D32, C13).
//
// ── The fourth skew surface, and the one that produced an accusation ────────
// D32 says a version skew "produces a sentence naming the mismatch, not a parse error". Three
// surfaces had one: the ceremony record, the invitation, and the session protocol. The fourth —
// the attestation tag inside every signature's /Reason — was excused, and its skew was the worst
// of the four because it did not fail loudly at all.
//
// `Attestations` required `[NibCoSign:1]` verbatim before it would read anything, so a signature
// from a build that had moved to `[NibCoSign:2]` matched nothing: no acceptedPeer, no rosterHash.
// `markOneProceeding` treats an empty commitment on a VALID signature as disqualifying, so ONE
// such signature made the whole document report *"This document was not produced by a single
// agreed proceeding."*
//
// An accusation about the parties, on a document everybody signed correctly, caused by one of
// them having updated Nib — which is verbatim the harm the roster token's own version was added
// to prevent one level down.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const A = 'aa'.repeat(32);
const B = 'bb'.repeat(32);
const ROSTER = 'c0ffee'.repeat(10) + 'abcd';

// att builds one attestation as `/api/attestations` serialises it. `tagVersion` is the field
// under test; 1 is what this build writes.
const att = (fp, peer, over = {}) => ({
  signer: 'S-' + fp.slice(0, 2), fingerprint: fp, acceptedPeer: peer,
  reason: '[NibCoSign:1] Accepts [SPKI:' + peer + ']', when: '2026-08-21T10:00:00Z',
  valid: true, matched: true, pinned: false, rosterHash: ROSTER, rosterVersion: 4,
  oneProceeding: true, tagVersion: 1,
  ...over,
});

// A signature from a NEWER build: Go read the version and deliberately parsed nothing else, so
// every token field is empty and only `tagVersion` says why.
const fromNewerNib = (fp) => ({
  signer: 'S-' + fp.slice(0, 2), fingerprint: fp, acceptedPeer: '',
  reason: '[NibCoSign:2] something this build does not know how to read',
  when: '2026-08-21T10:02:00Z', valid: true, matched: false, pinned: false,
  rosterHash: '', rosterVersion: 0, oneProceeding: false, tagVersion: 2,
});

let attestations = [];
const { document: doc, settle } = await boot({
  routes: {
    '/api/attestations': () => ({ attestations }),
    '/api/open': {
      id: 'test-epoch:1', name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf',
      canSave: false, canUndo: false, canRedo: false,
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

setNextDocument({ numPages: 1 });
doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
doc.getElementById('openGo').click();
await settle();

async function panelText(next) {
  attestations = next;
  const body = doc.getElementById('sigDetailsBody');
  body.innerHTML = '';
  doc.getElementById('sigDetailsBtn').click();
  // Wait for the ATTESTATION box, not for any text: the signer rows render synchronously and
  // `augmentSigDetails` is a fetch behind them, so waiting on `textContent` returns a panel that
  // has drawn the rows and none of the verdicts — which reads as "the fix does nothing".
  for (let i = 0; i < 80 && !body.querySelector('.sigatt'); i++) {
    await new Promise((r) => setTimeout(r, 5));
  }
  return body.textContent.replace(/\s+/g, ' ');
}

test('a signature from a newer Nib is reported as unreadable, not as a disagreement', async () => {
  const txt = await panelText([att(A, B), fromNewerNib(B)]);

  // STIMULUS: the panel drew. Without this the absence assertion below passes against a panel
  // that rendered nothing, which is what a broken fetch produces.
  assert.ok(txt.length > 0, 'the panel rendered nothing, so nothing below measures anything');

  assert.ok(!/was not produced by a single agreed proceeding/.test(txt),
    `a document whose only anomaly is that one party has a NEWER Nib was reported as not being ` +
    `one proceeding. That is an accusation about the parties caused by an upgrade — verbatim the ` +
    `harm D32 exists to prevent, and the harm the roster token's own version already prevents ` +
    `one level down: ${txt}`);
  assert.match(txt, /newer version of Nib and this one cannot read them/,
    `the panel says nothing about the skew, so a reader is left with a document that has one ` +
    `signature carrying no attestation and no explanation of why: ${txt}`);
  assert.match(txt, /version difference, not a disagreement/,
    `the sentence does not distinguish the two, which is the whole point of saying it: ${txt}`);
});

test('a document all of whose signatures this build can read still gets its verdict', async () => {
  // The control, and it is what the fix would break if the new branch swallowed the old one.
  // Suppressing the proceeding verdict whenever ANY tagVersion is present — rather than a newer
  // one — would silence the check on every ordinary ceremony in the product.
  const txt = await panelText([att(A, B), att(B, A)]);
  assert.match(txt, /✓ One proceeding/,
    `a ceremony this build can read in full lost its proceeding verdict: ${txt}`);
  assert.ok(!/cannot read them/.test(txt),
    `signatures at this build's own tag version were reported as unreadable: ${txt}`);
});

test('a genuine disagreement is still reported when every signature is readable', async () => {
  // The other control. The unreadable branch stands in FRONT of the disagreement line, so a
  // version that suppressed it unconditionally would hide real disagreements — the failure
  // `oneproceeding.test.mjs` was written against, reintroduced by its own fix.
  const other = 'dec0de'.repeat(10) + 'abcd';
  const txt = await panelText([att(A, B), att(B, A, { rosterHash: other, oneProceeding: false })]);
  assert.match(txt, /⚠ Not one proceeding/,
    `two signatures naming DIFFERENT ceremonies were no longer reported: ${txt}`);
});

// TestTheClientAndGoAgreeOnTheTagVersion — the cross-language constant.
//
// The number lives in two languages: `attestationTagVersion` in Go decides what gets parsed, and
// `ATTESTATION_TAG_VERSION` in app.js decides what gets reported as unreadable. If Go moves to 2
// and the client does not, every signature this build writes reports itself as unreadable to its
// own user — the skew sentence firing on a document with no skew in it.
test('the client and Go agree on the attestation tag version', () => {
  const goSrc = fs.readFileSync(path.join(REPO, 'internal/p2p/attestation.go'), 'utf8');
  const jsSrc = fs.readFileSync(path.join(REPO, 'web/app.js'), 'utf8');

  const go = goSrc.match(/attestationTagVersion\s*=\s*(\d+)/);
  const js = jsSrc.match(/ATTESTATION_TAG_VERSION\s*=\s*(\d+)/);
  assert.ok(go, 'attestationTagVersion is not declared in Go; this guard read the wrong file');
  assert.ok(js, 'ATTESTATION_TAG_VERSION is not declared in app.js');
  assert.equal(js[1], go[1],
    `app.js reads attestation tag version ${js[1]} and Go writes ${go[1]}. A build whose two ` +
    `halves disagree reports its OWN signatures as unreadable to its own user — the skew ` +
    `sentence firing on a document with no skew in it.`);

  // And the tag constant itself carries that version, or the parser and the writer disagree about
  // what a current signature looks like.
  assert.ok(goSrc.includes('attestationTag = "[NibCoSign:' + go[1] + ']"'),
    `attestationTag does not carry version ${go[1]}, so what this build WRITES and what it ` +
    `considers current are two different numbers`);
});
