// PLAN-returned-document P03.S03: "see what changed" — in the real app, over stubbed answers.
//
// The sheet hands a copy to the shipped Compare as BYTES (D11): the version you signed when the check recovered one,
// then the fallback chain in the plan's order — the copy kept at signing (P04.S02's slot, named and empty), this
// machine's ceremony copy, a file you choose. In every case the copy is the BASELINE, so text that came back and was
// not in what you signed reads as ADDED. One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

const YOU = 'a'.repeat(64);
let n = 0;
let openAs = null;
let revision = null; // a Response, or null when the test expects no request
let ceremonyCopy = null; // a Response, or a function returning one (or a promise of one)

const h = await boot({
  routes: {
    '/api/open': () => ({ id: `test-epoch:${++n}`, name: `back${n}.pdf`, path: `/tmp/nib-harness/back${n}.pdf`, canSave: true,
      canUndo: false, canRedo: false, ...openAs }),
    '/api/document/revision': () => revision.clone(),
    '/api/document/ceremony-copy': () => (typeof ceremonyCopy === 'function' ? ceremonyCopy() : ceremonyCopy.clone()),
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
  },
});
const { document: doc, settle, calls } = h;
const $ = (id) => doc.getElementById(id);

const SIGNED_BYTES = Uint8Array.from([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
const MIRROR_BYTES = Uint8Array.from([42, 43, 44]);
const pdfResponse = (bytes, header, facts) => new Response(bytes.slice(), {
  status: 200, headers: { 'Content-Type': 'application/pdf', [header]: JSON.stringify(facts) },
});
const refused = (body) => new Response(JSON.stringify(body), { status: 422, headers: { 'Content-Type': 'application/json' } });
const MINE = {
  signature: { state: 'valid', signers: [{ name: 'You', valid: true, fingerprint: YOU, coverageEnd: 500 }] },
  signerWhose: ['you'], unverifiedSigners: 0,
};
const callsTo = (route) => calls.filter((c) => c.url.includes(route));

// openSheet opens a document whose text is `text` and the sheet over it, with the revision route answering `answer`.
async function openSheet(open, answer, text = 'alpha beta gamma', more = {}) {
  $('compareModal').hidden = true;
  openAs = open;
  revision = answer;
  pdfjs.setNextDocument({ numPages: 1, text, ...more });
  $('pathInput').value = '/tmp/nib-harness/back.pdf';
  $('openGo').click();
  await settle();
  $('returnedBtn').click();
  await settle();
  await settle();
}

test('the chain is offered in order — kept copy (named, empty), ceremony copy, a file you choose — and nothing is asked on open', async () => {
  const before = callsTo('/api/document/ceremony-copy').length;
  await openSheet({ signature: { state: 'unsigned' } }, null);
  const items = [...$('returnedCompare').querySelectorAll('.rvchain > li')].map((li) => li.textContent.trim());
  assert.equal(items.length, 3, 'the fallback chain does not have its three links');
  assert.match(items[0], /copy kept when you signed.*does not keep one yet/, 'the kept copy\'s slot is not first, or not named empty');
  assert.match(items[1], /ceremony/, 'the ceremony copy is not second');
  assert.match(items[2], /file you choose/, 'a file you choose is not last');
  assert.equal($('returnedCmpSigned').hidden, true, 'an unsigned file offers "the version you signed"');
  assert.equal(callsTo('/api/document/ceremony-copy').length, before, 'opening the sheet asked for the ceremony copy (D10)');
});

test('the version you signed is compared from the bytes the check already holds, as the baseline', async () => {
  await openSheet(MINE, pdfResponse(SIGNED_BYTES, 'X-Nib-Revision', { obj: 7, end: 10, size: 20, history: 'none' }));
  assert.equal($('returnedCmpSigned').hidden, false, 'stimulus: a recovered version does not offer its comparison');
  const asked = callsTo('/api/document/revision').length;
  pdfjs.setNextDocument({ numPages: 1, text: 'alpha beta' }); // what you signed
  $('returnedCmpSigned').click();
  await settle();
  await settle();
  assert.equal($('compareModal').hidden, false, 'Compare did not open');
  assert.deepEqual([...pdfjs.lastGetDocumentData], [...SIGNED_BYTES], 'Compare did not load the recovered bytes');
  assert.equal(callsTo('/api/document/revision').length, asked, 'the signed version was fetched a second time (D10)');
  const body = $('compareBody');
  assert.match(body.textContent, /“the version you signed” → this file/, 'the caption does not read from the signed version');
  const added = [...body.querySelectorAll('.diffadd')].map((s) => s.textContent).join('');
  const removed = [...body.querySelectorAll('.diffdel')].map((s) => s.textContent).join('');
  assert.match(added, /gamma/, 'text that came back and was not signed does not read as ADDED');
  assert.doesNotMatch(removed, /gamma/, 'text that came back reads as removed — the diff runs the wrong way');
  // A second press compares again: the held bytes were not handed over and detached (the stub detaches what it is
  // given, as pdf.js's worker transfer does).
  $('compareClose').click();
  const first = pdfjs.lastGetDocumentData;
  pdfjs.setNextDocument({ numPages: 1, text: 'alpha beta' });
  $('returnedCmpSigned').click();
  await settle();
  await settle();
  assert.notEqual(pdfjs.lastGetDocumentData, first, 'stimulus: the second press never reached pdf.js');
  assert.deepEqual([...pdfjs.lastGetDocumentData], [...SIGNED_BYTES], 'the second comparison lost the bytes');
});

test('this machine\'s ceremony copy: asked on the press, pinned, worded, and compared as the baseline', async () => {
  ceremonyCopy = pdfResponse(MIRROR_BYTES, 'X-Nib-Ceremony-Copy', { ended: true, signed: true, extends: true });
  await openSheet({ signature: { state: 'unsigned' } }, null);
  const id = `test-epoch:${n}`;
  pdfjs.setNextDocument({ numPages: 1, text: 'alpha' });
  $('returnedCmpCeremony').click();
  await settle();
  await settle();
  const asks = callsTo('/api/document/ceremony-copy');
  assert.equal(asks.at(-1).headers['X-Nib-Doc'], id, 'the ceremony copy was not asked for the sheet\'s own document (ADR-001)');
  const note = $('returnedCmpCeremonyNote').textContent;
  assert.match(note, /That ceremony has ended/);
  assert.match(note, /last copy this machine stored.*carries a signature.*not necessarily the final copy everyone signed/);
  assert.match(note, /begins with that copy byte for byte/);
  assert.equal($('compareModal').hidden, false, 'Compare did not open');
  assert.deepEqual([...pdfjs.lastGetDocumentData], [...MIRROR_BYTES], 'Compare did not load the ceremony copy');
  assert.match($('compareBody').textContent, /“this machine's ceremony copy” → this file/);
  assert.equal($('returnedCmpCeremony').disabled, false, 'the button stayed disabled');
  // The other two facts' wordings.
  ceremonyCopy = pdfResponse(MIRROR_BYTES, 'X-Nib-Ceremony-Copy', {});
  $('compareClose').click();
  $('returnedCmpCeremony').click();
  await settle();
  await settle();
  const other = $('returnedCmpCeremonyNote').textContent;
  assert.match(other, /as the ceremony was convened, before anyone signed/);
  assert.match(other, /does not begin with that copy byte for byte/);
  assert.doesNotMatch(other, /has ended/);
});

test('each refusal of the ceremony copy has its own sentence, and nothing opens', async () => {
  const want = {
    'no-record': /names no signing ceremony/,
    'record-invalid': /ceremony record does not check out/,
    'not-on-this-machine': /holds no copy of the signing ceremony/,
    'different-proceeding': /belongs to a different ceremony/,
    'damaged': /copy of that ceremony is damaged/,
    'unreadable': /could not read this machine's copy/,
  };
  await openSheet({ signature: { state: 'unsigned' } }, null);
  // After the tests above showed it for a recovered version, a sheet over an unsigned file must not offer it again.
  assert.equal($('returnedCmpSigned').hidden, true, 'a previous opening\'s "version you signed" survived into this one');
  const seen = new Set();
  for (const [cause, re] of Object.entries(want)) {
    ceremonyCopy = refused({ cause });
    $('compareModal').hidden = true;
    $('returnedCmpCeremony').click();
    await settle();
    const text = $('returnedCmpCeremonyNote').textContent;
    assert.match(text, re, `cause ${cause}`);
    seen.add(text);
    assert.equal($('compareModal').hidden, true, `${cause} opened Compare`);
  }
  assert.equal(seen.size, Object.keys(want).length, 'two causes share one sentence');
  ceremonyCopy = new Response('locked', { status: 401 });
  $('returnedCmpCeremony').click();
  await settle();
  assert.match($('returnedCmpCeremonyNote').textContent, /Unlock Nib to read this machine's ceremony copy/);
});

test('a sheet closed while the ceremony copy was asked opens nothing when it arrives', async () => {
  let release;
  ceremonyCopy = () => new Promise((r) => { release = () => r(pdfResponse(MIRROR_BYTES, 'X-Nib-Ceremony-Copy', {})); });
  await openSheet({ signature: { state: 'unsigned' } }, null);
  $('returnedCmpCeremony').click();
  await settle();
  assert.equal(typeof release, 'function', 'stimulus: the request was never made');
  $('returnedClose').click();
  release();
  await settle();
  await settle();
  assert.equal($('compareModal').hidden, true, 'a closed sheet opened Compare');
});

test('a file you choose opens Compare and asks for the file', async () => {
  await openSheet({ signature: { state: 'unsigned' } }, null);
  let picked = 0;
  const input = $('compareInput');
  const was = input.click;
  input.click = () => { picked++; };
  try {
    $('returnedCmpFile').click();
  } finally {
    input.click = was;
  }
  assert.equal($('compareModal').hidden, false, 'Compare did not open');
  assert.equal(picked, 1, 'the file picker was not asked');
});

test('a file you choose is read as the baseline too: what came back and is not in your file reads as ADDED', async () => {
  await openSheet({ signature: { state: 'unsigned' } }, null, 'alpha beta gamma');
  const input = $('compareInput');
  const was = input.click;
  input.click = () => {};
  try {
    $('returnedCmpFile').click();
  } finally {
    input.click = was;
  }
  const win = doc.defaultView;
  const file = new win.File([new Uint8Array([7, 7, 7])], 'my-copy.pdf', { type: 'application/pdf' });
  file.arrayBuffer = async () => new Uint8Array([7, 7, 7]).buffer; // jsdom's File has no arrayBuffer
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  pdfjs.setNextDocument({ numPages: 1, text: 'alpha beta' });
  input.dispatchEvent(new win.Event('change'));
  await settle();
  await settle();
  delete input.files;
  const body = $('compareBody');
  assert.match(body.textContent, /“my-copy.pdf” → this file/, 'a picked file was not read as the baseline');
  assert.match([...body.querySelectorAll('.diffadd')].map((x) => x.textContent).join(''), /gamma/);
});

test('in baseline mode an empty side is named as the side it is', async () => {
  await openSheet(MINE, pdfResponse(SIGNED_BYTES, 'X-Nib-Revision', { obj: 7, end: 10, size: 20, history: 'none' }));
  pdfjs.setNextDocument({ numPages: 1, text: '' }); // the version you signed is a scan
  $('returnedCmpSigned').click();
  await settle();
  await settle();
  assert.match($('compareBody').textContent, /“the version you signed” has no extractable text/,
    'the empty copy was reported as the open document');
});

test('in baseline mode a page added after signing is "in this file and not in" the copy, and counts as +1', async () => {
  // The open document has a second page the signed version lacks.
  await openSheet(MINE, pdfResponse(SIGNED_BYTES, 'X-Nib-Revision', { obj: 7, end: 10, size: 20, history: 'none' }),
    ['the first page of the lease', 'a second page added after signing'], { numPages: 2, renders: true });
  pdfjs.setNextDocument({ numPages: 1, text: ['the first page of the lease'], renders: true });
  $('returnedCmpSigned').click();
  await settle();
  await settle();
  doc.querySelector('.cmmode[data-mode="side"]').click();
  await settle();
  await settle();
  const stat = $('cmAlignStat').textContent;
  assert.match(stat, /^\+1$/, `a page only in this file did not count as +1 against the copy: ${stat}`);
  $('cmNext').click();
  await settle();
  await settle();
  assert.match($('compareSummary').textContent, /Page 2 of this file is not in the version you signed/,
    'the banner reads the page the wrong way round');
});

test('a Compare closed while its copy was still loading installs nothing when the load lands', async () => {
  await openSheet(MINE, pdfResponse(SIGNED_BYTES, 'X-Nib-Revision', { obj: 7, end: 10, size: 20, history: 'none' }));
  pdfjs.setNextDocument({ numPages: 1, text: 'alpha beta' });
  $('returnedCmpSigned').click();
  $('compareClose').click(); // before the load's first await returns
  await settle();
  await settle();
  assert.equal($('compareModal').hidden, true);
  assert.doesNotMatch($('compareBody').textContent, /→ this file|No text differences/,
    'a superseded load rendered its comparison into a closed Compare');
  assert.equal($('compareTools').hidden, true, 'a superseded load revealed the compare tools');
});
