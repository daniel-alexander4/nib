// P01.S02 — `AddedAfter` over verified, well-formed revisions, as a reader is shown it (ADR-059).
//
// The server now says WHICH fact set the warning (`addedAfterCause`) and lists every
// signature-shaped object it refused (`refused`). /pending 661's decoy is the reason: a dictionary
// that claims to be a signature covering the whole file, and is not one. The badge must name BOTH
// facts — the append was measured (the cause is only this one where a valid signature bounds
// coverage) AND a signature Nib refused is present — the details panel must name
// the refused object, and its note must stop saying the bytes are "not covered by any signature" —
// after S02 they may sit under a failed or refused signature, so the true sentence is "any VALID
// signature".
//
// **What only this tier can see:** which cause produces which words, and that the refused object's
// attacker-typed filter reaches the page as text. The causes themselves are `internal/sign`'s.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let nextOpen = null;

const { document: doc, settle } = await boot({
  routes: {
    '/api/open': () => nextOpen,
    '/api/scan': { hidden: [] },
    '/api/attestations': () => ({ attestations: [] }),
  },
});

const badge = () => doc.getElementById('sigBadge');
const alice = [{ name: 'Alice', valid: true, timeBacking: 'none', fingerprint: 'a'.repeat(64) }];

// One document id across every case, for the reason badgetrust.test.mjs gives: a reload of the
// same id keeps the loading target the active view, so the badge is this case's and not the last.
async function openWith(signature) {
  nextOpen = {
    id: 'refused:1', name: 'lease.pdf', path: '/tmp/nib-harness/lease.pdf',
    canSave: true, canUndo: false, canRedo: false, signature, unverifiedSigners: 0,
  };
  setNextDocument({ numPages: 1 });
  doc.getElementById('pathInput').value = nextOpen.path;
  doc.getElementById('openGo').click();
  await settle();
}

async function openDetails() {
  const body = doc.getElementById('sigDetailsBody');
  body.innerHTML = '';
  doc.getElementById('sigDetailsBtn').click();
  await settle();
  return body.textContent.replace(/\s+/g, ' ');
}

const decoy = { obj: 7, filter: 'Evil\u001b[31m', cause: 'unsupported-filter' };

test('a refused signature is named on the badge beside the measured append, and in the panel', async () => {
  await openWith({
    state: 'valid', signers: alice, addedAfter: true,
    addedAfterCause: 'refused-signature-present', refused: [decoy],
  });
  // STIMULUS: the badge rendered from a valid document that warns.
  assert.match(badge().textContent, /Untampered/, `the fixture did not open as a vouched valid document: ${badge().textContent}`);
  assert.match(badge().textContent, /a signature Nib refused is present/,
    'the badge does not say a refused signature set the warning');
  assert.match(badge().textContent, /content added after signing/,
    'the badge names the refusal and drops the measured append');
  const txt = await openDetails();
  assert.match(txt, /A signature Nib refused is present — Object 7 \(Evil\u001b\[31m\): it is not a kind of signature Nib can check/,
    `the panel does not name the refused object, its filter and its cause: ${txt.slice(0, 400)}`);
  assert.match(txt, /not covered by any valid signature/,
    `the note does not say VALID signature: ${txt.slice(0, 400)}`);
  assert.doesNotMatch(txt, /covered by any signature\./,
    'the note still says the bytes are covered by no signature, which is false of a document whose last signature failed or was refused');
  assert.doesNotMatch(txt, /unchanged since you signed/i, 'AddedAfter is never "unchanged since you signed"');
  assert.equal(doc.querySelector('.sig-refused')?.children.length, 0,
    'the refused line is built from markup rather than set as text');
});

test('an ordinary append keeps its words', async () => {
  await openWith({ state: 'valid', signers: alice, addedAfter: true, addedAfterCause: 'appended' });
  assert.match(badge().textContent, /content added after signing/,
    `an append no longer says content was added: ${badge().textContent}`);
  const txt = await openDetails();
  assert.doesNotMatch(txt, /Nib refused/, 'a refused line appeared with nothing refused');
});

test('a check that could not run says so', async () => {
  await openWith({ state: 'valid', signers: alice, addedAfter: true, addedAfterCause: 'could-not-check' });
  assert.match(badge().textContent, /could not confirm nothing was added after signing/,
    `the badge claims an append nib did not measure: ${badge().textContent}`);
  const txt = await openDetails();
  assert.match(txt, /could not confirm/, `the panel claims an append nib did not measure: ${txt.slice(0, 300)}`);
});

test('a document whose only signature was refused still offers the panel', async () => {
  await openWith({ state: 'invalid', signers: [], refused: [{ obj: 5, cause: 'unparseable-contents' }] });
  // STIMULUS: no signer at all, so only the refused list can open the panel.
  assert.match(badge().textContent, /Modified since signing/, `the fixture did not open as invalid: ${badge().textContent}`);
  assert.equal(doc.getElementById('sigDetailsBtn').hidden, false,
    'the details button is hidden, so the refused signature has no surface');
  const txt = await openDetails();
  assert.match(txt, /Object 5: its signature data cannot be read/, `the panel does not name it: ${txt.slice(0, 300)}`);
});
