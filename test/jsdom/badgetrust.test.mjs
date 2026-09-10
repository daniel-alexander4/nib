// /pending 390 — the badge may only say "Untampered" about a document it can vouch for.
//
// **The defect, and it was built rather than reasoned.** Dan signs. Mallory appends changed content
// plus her OWN self-signed signature covering to EOF. `sign.Verify` returns state=valid,
// addedAfter=false and two signers both valid — because `AddedAfter` flags only content after the
// LAST signature, content added BETWEEN signatures being expected in multi-party signing. So the
// toolbar rendered `✓ Untampered · 2 signers` about a document altered after its owner signed it,
// and Mallory needed no key of Dan's: GenerateIdentity plus SignApproval is the whole attack.
//
// `AddedAfter`'s premise is true on a ROSTER and false in a dispute. The solo path has no roster,
// so the server now answers the question it can — how many signers this machine cannot vouch for —
// and the word is withheld unless that is zero.
//
// **What only this tier can see:** which of the three states produces which sentence. The counting
// itself is `internal/server`'s and is asserted there.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let nextOpen = null;

const { document: doc, settle } = await boot({
  routes: { '/api/open': () => nextOpen, '/api/scan': { hidden: [] } },
});

const badge = () => doc.getElementById('sigBadge');

// One document id across every case, deliberately. A second `/api/open` with a NEW id lands in a
// new view, and `setDocumentFromServer` only calls `updateBadge` when the loading target is the
// ACTIVE view — so the badge would keep the previous test's text and each case after the first
// would assert against the one before it. Reloading the same id keeps target === view.
async function openWith(signature) {
  nextOpen = {
    id: 'badge:1', name: 'lease.pdf', path: '/tmp/nib-harness/lease.pdf',
    canSave: true, canUndo: false, canRedo: false, ...signature,
  };
  setNextDocument({ numPages: 2 });
  doc.getElementById('pathInput').value = nextOpen.path;
  doc.getElementById('openGo').click();
  await settle();
}

const twoSigners = [
  { name: 'Dan', valid: true, timeBacking: 'none', fingerprint: 'a'.repeat(64) },
  { name: 'Mallory', valid: true, timeBacking: 'none', fingerprint: 'b'.repeat(64) },
];

test('a document with a signer this machine cannot vouch for is not called Untampered', async () => {
  await openWith({
    signature: { state: 'valid', signers: twoSigners },
    unverifiedSigners: 1,
  });
  // The stimulus floor: the badge rendered at all, from the state under test.
  assert.notEqual(badge().textContent, '', 'the badge is empty, so nothing below is asserted');
  assert.doesNotMatch(badge().textContent, /Untampered/,
    'the badge says Untampered about a document carrying a signature this machine has never '
    + 'seen before — which is the attack verbatim: a stranger appends changed content and their '
    + 'own self-signed signature, and every signature is valid over its own byte range');
  assert.match(badge().textContent, /you have not verified/,
    'the badge withholds the reassurance and does not say why, so the reader is left with a '
    + 'count and no reason to distrust it');
  assert.equal(badge().className.includes('badge-warn'), true,
    'the badge is not styled as a caution, so the withheld word is the only signal and a skim '
    + 'reads it as fine');
});

test('a document whose every signer is known keeps the reassurance', async () => {
  await openWith({
    signature: { state: 'valid', signers: twoSigners },
    unverifiedSigners: 0,
  });
  assert.match(badge().textContent, /✓ Untampered/,
    'a document signed entirely by identities this machine knows no longer says Untampered — '
    + 'the rule has become a blanket refusal, which tells the reader nothing at all');
  assert.match(badge().textContent, /2 signers/,
    'the signer count went with it');
});

test('a LOCKED vault is not the same as nothing to worry about', async () => {
  // The field is absent — the server could not read the pinned set, so it has verified nothing.
  await openWith({ signature: { state: 'valid', signers: twoSigners } });
  assert.doesNotMatch(badge().textContent, /Untampered/,
    'an absent unverifiedSigners was read as zero. The field is absent precisely when the vault '
    + 'is locked and the machine has checked NOTHING, so treating it as "all known" is the '
    + 'original defect wearing a new field');
  assert.match(badge().textContent, /unlock to check who signed/,
    'the badge warns without saying the answer is available — the user can unlock and find out, '
    + 'and nothing tells them so');
});

test('an unsigned document is unaffected', async () => {
  await openWith({ signature: { state: 'unsigned' } });
  assert.match(badge().textContent, /Unsigned/,
    'the trust rule leaked onto a document with no signatures at all');
});
