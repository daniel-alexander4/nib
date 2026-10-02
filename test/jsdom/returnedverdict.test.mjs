// PLAN-returned-document P03.S02: the verdict, told for a dispute — in the real app, over stubbed answers.
//
// The sheet asks the signed version only for a fingerprint the SERVER called yours (`signerWhose`), and words each
// of D8's terminal states, the five causes and the facts the route sends. The acceptance clauses, one test each:
// a stranger co-signing after you never reads as an unqualified "Untampered"; a re-saved file says the signed version
// is not inside it; an unsigned file says so without implying you never signed.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const YOU = 'a'.repeat(64);
const THEM = 'b'.repeat(64);
let n = 0;
let openAs = null; // the next /api/open answer, set per test
let revision = null; // the next /api/document/revision answer: a Response
const asked = [];

const h = await boot({
  routes: {
    '/api/open': () => ({ id: `test-epoch:${++n}`, name: `back${n}.pdf`, path: `/tmp/nib-harness/back${n}.pdf`, canSave: true,
      canUndo: false, canRedo: false, ...openAs }),
    '/api/document/revision': (opts) => { asked.push(opts); return revision.clone(); },
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
  },
});
const { document: doc, settle, calls } = h;
const $ = (id) => doc.getElementById(id);

const found = (facts) => new Response(new Uint8Array(10), {
  status: 200, headers: { 'Content-Type': 'application/pdf', 'X-Nib-Revision': JSON.stringify(facts) },
});
const refused = (body) => new Response(JSON.stringify(body), { status: 422, headers: { 'Content-Type': 'application/json' } });

async function check(open, answer) {
  openAs = open;
  revision = answer;
  asked.length = 0;
  setNextDocument({ numPages: 1 });
  $('pathInput').value = '/tmp/nib-harness/back.pdf';
  $('openGo').click();
  await settle();
  $('returnedBtn').click();
  await settle();
  await settle();
  return $('returnedVerdict').textContent + '\n' + $('returnedSigners').textContent;
}
const revisionCalls = () => calls.filter((c) => c.url.includes('/api/document/revision'));

test('a stranger co-signed after you: your version is inside, the bytes after it are not covered, and they are NOT yours', async () => {
  const before = revisionCalls().length;
  const text = await check({
    signature: { state: 'valid', signers: [
      { name: 'Mallory', valid: true, fingerprint: THEM, coverageEnd: 900 },
      { name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 },
    ] },
    signerWhose: ['', 'you'],
    unverifiedSigners: 1,
  }, found({ obj: 7, end: 500, size: 900, history: 'none' }));
  const asks = revisionCalls().slice(before);
  assert.equal(asks.length, 1, 'stimulus: the sheet did not ask for the signed version');
  assert.match(asks[0].url, new RegExp(`signer=${YOU}`), 'it asked with a fingerprint that is not yours');
  assert.doesNotMatch(text, /Untampered/, 'the surface calls a stranger-co-signed document Untampered');
  assert.match(text, /The version you signed is inside this file, and 400 bytes were added after it/);
  assert.match(text, /does not cover what was added/);
  assert.match(text, /object 7, covering the first 500 bytes/);
  const rows = [...$('returnedSigners').querySelectorAll('.sigrow')].map((r) => r.textContent);
  assert.equal(rows.length, 2);
  assert.match(rows[0], /You.*yours.*the version you signed/, 'yours is not first by coverage');
  assert.match(rows[1], /Mallory.*not known to this machine.*added after yours/, 'the stranger is not named as unknown and later');
});

test('a re-saved file: the version you signed is not inside it, and nothing says "re-saved" (D8, W10, C1)', async () => {
  const text = await check({
    signature: { state: 'invalid', signers: [{ name: 'You', valid: false, fingerprint: YOU, coverageEnd: 500 }] },
    signerWhose: ['you'], unverifiedSigners: 0,
  }, refused({ cause: 'resaved', attributed: false }));
  assert.match(text, /The version you signed is not inside this file\./);
  assert.match(text, /naming your certificate.*cannot say it is yours/, 'an unattributed name was worded as yours (C1)');
  assert.doesNotMatch(text, /re-?saved/i, 'W10: the surface alleges a re-save nib never observed');
  const attributed = await check({
    signature: { state: 'invalid', signers: [{ name: 'You', valid: false, fingerprint: YOU, coverageEnd: 500 }] },
    signerWhose: ['you'], unverifiedSigners: 0,
  }, refused({ cause: 'resaved', attributed: true }));
  assert.match(attributed, /made with your key over different bytes/);
});

test('an unsigned file says so as it stands, without implying you never signed, and asks nothing (I8, D10)', async () => {
  const before = revisionCalls().length;
  const text = await check({ signature: { state: 'unsigned' } }, null);
  assert.equal(revisionCalls().length, before, 'an unsigned file was asked about');
  assert.match(text, /as it stands, carries no signature/);
  assert.match(text, /does not mean nobody signed it/);
  assert.doesNotMatch(text, /you (never|did not) sign/i);
});

test('a locked vault says so and asks nothing; a document with none of your signatures says so and asks nothing', async () => {
  const before = revisionCalls().length;
  const locked = await check({ signature: { state: 'valid', signers: [{ name: 'X', valid: true, fingerprint: THEM, coverageEnd: 9 }] } }, null);
  assert.match(locked, /Unlock Nib to check which of these signatures is yours/);
  const theirs = await check({ signature: { state: 'valid', signers: [{ name: 'X', valid: true, fingerprint: THEM, coverageEnd: 9 }] },
    signerWhose: ['known'], unverifiedSigners: 0 }, null);
  assert.match(theirs, /None of this document's signatures, as it stands, is yours/);
  assert.match(theirs, /known to this machine/);
  assert.equal(revisionCalls().length, before, 'the route was asked without a fingerprint the server called yours');
});

test('every fact the route sends is worded — or, for "none", deliberately silent (W9)', async () => {
  const text = await check({
    signature: { state: 'valid', signers: [{ name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 }] },
    signerWhose: ['you'], unverifiedSigners: 0,
  }, found({ obj: 12, end: 500, earlierRevision: true, redefinedObj: 12, later: [40, 41], laterUnchecked: true,
    earlier: [200], truncated: true, history: 'undo', size: 500 }));
  assert.match(text, /exactly the version you signed/);
  assert.match(text, /replaced your signature's dictionary \(object 12\)/);
  assert.match(text, /You signed an earlier version too/);
  assert.match(text, /2 later signatures naming your certificate/);
  assert.match(text, /could not confirm this is the last version you signed/);
  assert.match(text, /cut short/);
  assert.match(text, /edited this copy since opening it/);
  const quiet = await check({
    signature: { state: 'valid', signers: [{ name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 }] },
    signerWhose: ['you'], unverifiedSigners: 0,
  }, found({ obj: 3, end: 500, size: 500, history: 'none' }));
  assert.doesNotMatch(quiet, /as it arrived|edited|undo/i, 'history "none" was worded as a claim about the arrival');
});

test('each of the other causes has its own sentence', async () => {
  const want = {
    'not-your-signature': /None of this file's signatures names your certificate/,
    'no-signature': /as it stands, holds no signature Nib could read/,
    'prefix-failed-reverify': /does not verify over it on its own/,
    'could-not-check': /could not check this file well enough/,
  };
  for (const [cause, re] of Object.entries(want)) {
    const text = await check({
      signature: { state: 'valid', signers: [{ name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 }] },
      signerWhose: ['you'], unverifiedSigners: 0,
    }, refused({ cause, refused: [{ obj: 9, cause: 'contents-elsewhere' }] }));
    assert.match(text, re, cause);
    assert.match(text, /A signature Nib refused is present/, `${cause}: the refused signature was not named`);
  }
});

test('a re-saved file never calls the failing signature "yours" in the list either (C1 on one sheet)', async () => {
  const text = await check({
    signature: { state: 'invalid', signers: [{ name: 'You', valid: false, fingerprint: YOU, coverageEnd: 500 }] },
    signerWhose: ['you'], unverifiedSigners: 0,
  }, refused({ cause: 'resaved', attributed: false }));
  const row = $('returnedSigners').textContent;
  assert.match(text, /cannot say it is yours/, 'stimulus: the verdict withholds "yours"');
  assert.match(row, /names your certificate, and does not verify/);
  assert.doesNotMatch(row, /· yours\b/, 'the row says "yours" under a verdict that says Nib cannot say so');
});

test('when Nib could not tell who signed, it does not say none is yours, and asks nothing', async () => {
  const before = revisionCalls().length;
  const text = await check({
    signature: { state: 'valid', addedAfter: true, addedAfterCause: 'could-not-check',
      signers: [{ name: 'A', valid: true }, { name: 'B', valid: true }] },
    signerWhose: ['', ''], unverifiedSigners: 2,
  }, null);
  assert.equal(revisionCalls().length, before);
  assert.match(text, /could not tell who made these signatures, so it cannot say whether one of them is yours/);
  assert.doesNotMatch(text, /None of this document's signatures/, 'a join error was told as "you never signed"');
  assert.doesNotMatch($('returnedSigners').textContent, /not known to this machine/,
    'an unnamed signer reads as a stranger in its row, under a verdict that says Nib could not tell');
});

test('a file whose only signature is refused is not called unsigned, and names what it holds', async () => {
  const text = await check({ signature: { state: 'invalid', refused: [{ obj: 9, cause: 'contents-elsewhere' }],
    timestamps: [{ obj: 12 }] } }, null);
  assert.match(text, /carries nothing Nib accepts as a signature/);
  assert.doesNotMatch(text, /no signature survives/);
  assert.match(text, /A signature Nib refused is present/);
  assert.match(text, /A document timestamp is present — object 12/);
});

test('two certificates of yours, one recovered: the other\'s refusal is said too', async () => {
  const OTHER = 'c'.repeat(64);
  let k = 0;
  const answers = [found({ obj: 7, end: 500, size: 500, history: 'none' }), refused({ cause: 'resaved' })];
  const text = await check({
    signature: { state: 'valid', signers: [
      { name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 },
      { name: 'You (imported)', valid: true, fingerprint: OTHER, coverageEnd: 300 },
    ] },
    signerWhose: ['you', 'you'], unverifiedSigners: 0,
  }, { clone: () => answers[k++].clone() });
  assert.equal(k, 2, 'stimulus: each distinct fingerprint of yours was asked once');
  assert.match(text, /exactly the version you signed/);
  assert.match(text, /names another of your certificates, and Nib could not recover a version for it/);
  assert.doesNotMatch(text, /signed with your other certificate/, 'C1: the other refusal was worded as your signing');
});

test('the vault locking mid-check says so, rather than "looking" for ever', async () => {
  const text = await check({
    signature: { state: 'valid', signers: [{ name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 }] },
    signerWhose: ['you'], unverifiedSigners: 0,
  }, new Response('locked', { status: 401 }));
  assert.doesNotMatch(text, /Looking for the version you signed/);
  assert.match(text, /Nib locked while it was checking/);
});
