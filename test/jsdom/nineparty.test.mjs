// The signature panel on a document with more than two parties (P07.S07c, C09, C14).
//
// ── Three defects, all of them invisible at N=2 ─────────────────────────────
// `augmentSigDetails` was written when a co-signed document meant two people, and every one of
// its three faults is correct arithmetic on that document and wrong on a nine-party ceremony:
//
//  1. It returned early on `!a.acceptedPeer`, which is exactly the FIRST SIGNER of a ceremony.
//     `PredecessorOf` returns "" for them because there is nobody before them — C14 as amended
//     names that state — so a nine-signature document rendered EIGHT rows and silently dropped
//     the party who went first.
//  2. Its "of N" denominator was `attested.length`, the count of rows it had just drawn. So the
//     number a reader compares against Go's signature count was short by the row it skipped,
//     and short again by any foreign approval signature on the document.
//  3. `attested.every(matched)` printed *"each party's signature attests to the OTHER's key"*.
//     On a completed baton every signature after the first matches its predecessor, so the
//     condition held — and a reader checking a nine-party deed was told it is a mutual exchange
//     between two people. "Mutually" is false of a baton twice over: party 1 accepts nobody and
//     nobody accepts party 9.
//
// ── Why the discriminator is the shape and not the roster token ─────────────
// The slice's own acceptance said the two-party sentence must be unreachable "on a document
// carrying a roster token". Driving it refuted that, though not in the way the first attempt
// assumed. The sentence is true of a MUTUAL PAIR — two signatures each accepting the other,
// which is what an ordinary co-sign produces — and `oneproceeding.test.mjs` drives exactly such
// a document carrying roster tokens, asserting the positive survives so a disagreement is
// reported rather than summarised away by deleting the good news. Suppressing on the record
// would have removed a true sentence to fix a false one.
//
// **And a two-party CEREMONY is not a mutual pair**, which the control below found: it is a
// baton of length two, party 0 accepts nobody, so `every(matched)` is false for it. A chain
// branch written as `length > 2` left the smallest ceremony the product can convene falling
// through both branches with no positive verdict at all. The two branches are therefore
// mutual-pair and baton, not two-party and many-party.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const ROSTER = 'c0ffee'.repeat(10) + 'abcd';
const OTHER = 'dec0de'.repeat(10) + 'abcd';
const N = 9;

const fpOf = (i) => String(i).repeat(2).slice(0, 2).repeat(32);

// A baton: party 0 accepts nobody, party i accepts party i-1. That is what a completed ceremony
// produces, and its shape is the reason every defect above survived — every signature but the
// first is `matched`, so a two-party condition reads as satisfied.
const chain = (n, rosterHash = ROSTER, over = {}) =>
  Array.from({ length: n }, (_, i) => ({
    signer: 'Party Label ' + i,
    fingerprint: fpOf(i),
    acceptedPeer: i === 0 ? '' : fpOf(i - 1),
    reason: '[NibCoSign:1] Accepts Party ' + (i - 1) + ' [SPKI:' + (i === 0 ? '' : fpOf(i - 1)) + ']',
    when: '2026-08-21T10:0' + (i % 10) + ':00Z',
    valid: true,
    matched: i !== 0,
    pinned: false,
    rosterHash,
    rosterVersion: 4,
    oneProceeding: true,
    ...(over[i] || {}),
  }));

let attestations = [];
let obliged = 0;
let signed = 0;

const { document: doc, settle } = await boot({
  routes: {
    '/api/attestations': () => ({ attestations, obliged, signed }),
    '/api/open': {
      id: 'test-epoch:1', name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf',
      canSave: false, canUndo: false, canRedo: false,
      signature: {
        state: 'valid',
        // Nine signer rows, because `augmentSigDetails` appends each attestation box to the
        // ROW at the same index — `rows[i]` — and with fewer rows than attestations it drops
        // the surplus silently. A fixture short of rows would make "the panel renders every
        // signature" fail for the fixture's reason rather than the code's.
        signers: Array.from({ length: N }, (_, i) => ({
          name: 'Party Label ' + i, valid: true, when: '2026-08-21T10:00:00Z',
        })),
      },
    },
  },
});

setNextDocument({ numPages: 1 });
doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
doc.getElementById('openGo').click();
await settle();

async function panelText(next, counts = {}) {
  attestations = next;
  obliged = counts.obliged || 0;
  signed = counts.signed || 0;
  const body = doc.getElementById('sigDetailsBody');
  body.innerHTML = '';
  doc.getElementById('sigDetailsBtn').click();
  for (let i = 0; i < 80 && !body.querySelector('.sigatt'); i++) {
    await new Promise((r) => setTimeout(r, 5));
  }
  return { text: body.textContent.replace(/\s+/g, ' '), boxes: body.querySelectorAll('.sigatt').length };
}

test('every signature on a nine-party document gets a row, including the one that accepts nobody', async () => {
  const { text, boxes } = await panelText(chain(N));

  // STIMULUS before response: the panel drew at all. Without this the count assertion below is
  // satisfied by a panel that rendered nothing and a fetch that failed.
  assert.match(text, /Accepts a co-signer/,
    `the panel rendered no attestation at all: ${text.slice(0, 200)}`);

  // The count is the point. Eight of nine is what shipped, and every assertion about the
  // CONTENT of a row passes against it — the missing party leaves no trace in the text.
  // One trailing box is the provenance note, which carries the same class.
  assert.equal(boxes, N + 1,
    `the panel drew ${boxes - 1} attestation row(s) for ${N} signatures. The party who signed ` +
    `FIRST accepts nobody (C14), and dropping them removes the one signature a reader is most ` +
    `likely to be checking: ${text}`);

  // And the first signer's row says the true thing rather than the accusatory one. Reporting
  // "not a confirmed co-signer" about the signature that cannot be anything else is the failure
  // C14's amendment names.
  assert.match(text, /First signer — there was no earlier party/,
    `the first signer's row does not state C14's no-predecessor case: ${text}`);
  assert.ok(!/Party Label 0.*not a confirmed co-signer/.test(text),
    `the first signer is reported as failing a check that cannot apply to them: ${text}`);
});

test('the two-party sentence is unreachable above two parties, and the chain gets its own', async () => {
  const { text } = await panelText(chain(N));

  assert.ok(!/attests to the other/.test(text),
    `a ${N}-party document is described as a mutual exchange between two people. "Mutually" is ` +
    `false of a baton twice over — party 1 accepts nobody and nobody accepts party ${N - 1}: ${text}`);
  assert.match(text, new RegExp('Every signature after the first attests to the party before it'),
    `the chain says nothing positive at all, so the fix deleted a sentence instead of ` +
    `correcting it: ${text}`);
  assert.match(text, new RegExp(N + ' parties'),
    `the chain sentence does not say how many parties there are: ${text}`);
});

test('the "of N" denominator is every signature on the document, not the rows the panel drew', async () => {
  // One party naming a different ceremony: the disagreement branch, which is the only line
  // carrying a denominator.
  //
  // **Plus a FOREIGN approval signature — and without it this test could not fail.** Its first
  // version drove nine ceremony signatures, where the rows drawn and the signatures reported are
  // both nine, so `attested.length` and `atts.length` are the same number and swapping one for
  // the other changes nothing. The red proof caught that: the patch went in and the test stayed
  // green. A tenth signature that carries no attestation at all — an ordinary Finalize, which
  // any party can add — is drawn by no row and counted by Go, and it is the only shape that
  // separates the two counts.
  const withForeign = chain(N, ROSTER, { 4: { rosterHash: OTHER, oneProceeding: false } });
  withForeign.push({
    signer: 'Someone Else', fingerprint: fpOf(9), acceptedPeer: '', reason: 'Finalized in Nib',
    when: '2026-08-21T11:00:00Z', valid: true, matched: false, pinned: false,
  });
  const { text } = await panelText(withForeign);

  assert.match(text, /⚠ Not one proceeding/, `setup: the disagreement was not reported: ${text}`);
  // SETUP: the foreign signature really is skipped by the row loop, or the two counts are still
  // the same number and this test is measuring nothing.
  assert.ok(!/Finalized in Nib/.test(text),
    `the foreign approval signature was drawn as an attestation, so the rows drawn and the ` +
    `signatures reported are still the same count and this test cannot fail: ${text}`);
  assert.match(text, new RegExp('of ' + (N + 1) + ' signature'),
    `the denominator is not the ${N + 1} signatures Go reports. It was the count of rows the panel ` +
    `had drawn, which is short by the first signer it skipped and short again by any foreign ` +
    `approval signature on the document — so a reader comparing it against the signature count ` +
    `sees two numbers that disagree: ${text}`);
});

test('an ordinary two-party co-sign keeps the sentence that is true of it', async () => {
  // The control, and the reason the discriminator is not simply "has a record". Deleting the
  // two-party sentence wholesale would satisfy every assertion above and remove a true statement
  // from the document type the product has shipped longest.
  //
  // A MUTUAL pair, not a chain of two: both signatures accept each other, which is what an
  // ordinary co-sign produces and the only shape "the other's key" is true of.
  const mutual = chain(2, null);
  mutual[0].acceptedPeer = fpOf(1);
  mutual[0].matched = true;
  const { text } = await panelText(mutual);
  assert.match(text, /Mutually co-signed/,
    `an ordinary two-party co-sign lost the sentence that is true of it — it IS mutual: ${text}`);
  assert.ok(!/Every signature after the first/.test(text),
    `a mutual pair was given the chain sentence: ${text}`);
});

test('a two-party CEREMONY is a baton and gets the chain sentence, not silence', async () => {
  // Found by driving the control above. A two-party ceremony is a baton as well — party 0
  // accepts nobody — so `every(matched)` is false for it, and a chain branch written as
  // `length > 2` left it falling through both: no positive statement at all on the smallest
  // ceremony the product can convene.
  const { text } = await panelText(chain(2));
  assert.ok(!/attests to the other/.test(text),
    `a two-party ceremony is described as mutual, but its first party accepts nobody: ${text}`);
  assert.match(text, /Every signature after the first attests to the party before it/,
    `the smallest ceremony gets no positive verdict at all: ${text}`);
});
