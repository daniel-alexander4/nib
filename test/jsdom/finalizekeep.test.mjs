// PLAN-returned-document P04.S01 — "Keep a copy for my records" in the Finalize modal, in the real app over stubbed
// answers. The tick is off at every opening (D13), the request carries it and the document's name, and a copy that
// could not be kept is said as such — the signing refused, the modal left open (D14) — never "Could not finalize".
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let finalizeAnswer = null; // a Response
const posted = [];
let keptNow = { kept: [], totalBytes: 0 };
const removed = [];
let removeAnswer = true;
const probes = [];
let opens = 0;
let openSig = { state: 'unsigned' };
let probeAnswer = () => new Response(null, { status: 422, headers: { 'X-Nib-Kept-Copy-Cause': 'no-signature' } });
const h = await boot({
  routes: {
    '/api/open': () => { opens++; return { id: 'test-epoch:1', name: 'Lease.pdf', path: '/tmp/nib-harness/Lease.pdf', canSave: true,
      canUndo: false, canRedo: false, signature: openSig }; },
    '/api/identity/external': { present: false },
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/finalize': (opts) => { posted.push(opts.body); return finalizeAnswer.clone(); },
    '/api/kept': () => keptNow,
    '/api/kept/remove': (opts) => { removed.push(JSON.parse(opts.body).name); return { removed: removeAnswer }; },
    // P04.S03: the checklist's probe — HEAD, so the answer is the status and the cause header, never a body.
    '/api/document/kept-copy': (opts) => { probes.push(opts); return probeAnswer(); },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

setNextDocument({ numPages: 1 });
$('pathInput').value = '/tmp/nib-harness/Lease.pdf';
$('openGo').click();
await settle();

async function openModal() {
  $('finalizeBtn').click();
  await settle();
  assert.equal($('finalizeModal').hidden, false, 'stimulus: the Finalize modal did not open');
}
const toasts = () => [...doc.querySelectorAll('.toast, #toast, [role="status"]')].map((t) => t.textContent).join(' | ');

test('the tick is off at every opening and is never remembered (D13)', async () => {
  await openModal();
  assert.equal($('fzKeep').checked, false, 'the tick started ON');
  $('fzKeep').checked = true;
  $('fzCancel').click();
  await openModal();
  assert.equal($('fzKeep').checked, false, 'a tick from the last opening was remembered');
  assert.match($('fzKeepHint').textContent, /unencrypted/, 'the opt-in does not say the copy sits outside the vault');
  $('fzCancel').click();
});

test('the request carries the tick and the document\'s name; a kept copy is confirmed', async () => {
  finalizeAnswer = new Response(new Uint8Array([37, 80, 68, 70]), { status: 200,
    headers: { 'Content-Type': 'application/pdf', 'X-Nib-Kept': 'kept_lease_20261002-141502-0011aabb.pdf' } });
  await openModal();
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  const params = JSON.parse(posted.at(-1).get('params'));
  assert.equal(params.keep, true, 'the tick did not reach the request');
  assert.match(params.name, /Lease/, 'the document\'s name did not reach the request');
  assert.match(toasts(), /A copy was kept when you signed: ~\/nib\/signed\/kept_lease_/);
  assert.equal($('saveAsModal').hidden, false, 'stimulus: the signed file was not offered to save');
  $('saveAsCancel').click();
  await settle();
});

test('a copy that could not be kept says the document was NOT signed, why, and how to sign without it', async () => {
  finalizeAnswer = new Response(JSON.stringify({ error: 'the document was not signed: Nib could not keep the copy you asked for',
    cause: 'copy-not-kept', reason: 'mkdir /home/x/nib/signed: not a directory' }), { status: 500, headers: { 'Content-Type': 'application/json' } });
  await openModal();
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  const said = toasts();
  assert.match(said, /Not signed/);
  assert.match(said, /not a directory/);
  assert.match(said, /untick "Keep a copy"/);
  assert.doesNotMatch(said, /disk space/, 'a cause that is not a full disk was told to free space');
  assert.doesNotMatch(said, /Could not finalize/);
  assert.equal($('finalizeModal').hidden, false, 'the modal closed — unticking and signing again is no longer one press away');
  assert.equal($('saveAsModal').hidden, true, 'a refused signing offered a file to save');
  $('fzCancel').click();
});

test('a full disk, and only a full disk, is told to free some space', async () => {
  finalizeAnswer = new Response(JSON.stringify({ error: 'x', cause: 'copy-not-kept', reason: 'no space left on device', full: true }),
    { status: 500, headers: { 'Content-Type': 'application/json' } });
  await openModal();
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  assert.match(toasts(), /Free some disk space/);
  $('fzCancel').click();
});

test('the list shows each copy kept when you signed and removes one only after a confirm that says what it cannot reach', async () => {
  keptNow = { kept: [
    { name: 'kept_deed_20261002-090000-0011aabb.pdf', document: 'deed', keptAt: '2026-10-02 09:00:00', size: 2048 },
    { name: 'kept_lease_20261001-090000-2233ccdd.pdf', document: 'lease', keptAt: '2026-10-01 09:00:00', size: 1024 },
  ], totalBytes: 3072 };
  $('keptBtn').click();
  await settle();
  assert.equal($('keptModal').hidden, false, 'the list did not open');
  const rows = [...$('keptList').querySelectorAll('.keptrow')];
  assert.equal(rows.length, 2);
  assert.match(rows[0].textContent, /deed — kept 2026-10-02 09:00:00/);
  assert.match($('keptTotal').textContent, /2 copies/);
  const win = doc.defaultView;
  let asked = '';
  win.confirm = (msg) => { asked = msg; return false; };
  rows[1].querySelector('button').click();
  await settle();
  assert.match(asked, /permanent/);
  assert.match(asked, /does not reach any backup/);
  assert.equal(removed.length, 0, 'a declined confirm still removed the copy');
  win.confirm = () => true;
  keptNow = { kept: [keptNow.kept[0]], totalBytes: 2048 };
  rows[1].querySelector('button').click();
  await settle();
  await settle();
  assert.deepEqual(removed, ['kept_lease_20261001-090000-2233ccdd.pdf'], 'the remove did not name the copy by its name');
  assert.equal($('keptList').querySelectorAll('.keptrow').length, 1, 'the list was not refreshed after the remove');
  removeAnswer = false;
  $('keptList').querySelector('button').click();
  await settle();
  await settle();
  assert.match(toasts(), /already gone/);
  keptNow = { kept: [], totalBytes: 0 };
  $('keptClose').click();
  $('keptBtn').click();
  await settle();
  assert.match($('keptList').textContent, /No copies kept yet/);
  $('keptClose').click();
});

// ── P04.S03: the Simple Sign checklist's row, from the server's one match door ──────────────────────────────────────
//
// Each state is driven by the PROBE's answer alone — the document, its id and its signature status stay the same — so a
// row that rendered any state without reading the answer would fail the other two. Re-asked by re-opening the card,
// which is the one trigger that always asks.
const ROW = 'A copy kept when you signed';
const simpleSignHead = () => [...doc.querySelectorAll('#commands .sbhead.groupcard')].find((x) => x.textContent.trim() === 'Simple Sign');
const keptRow = () => [...$('signSteps').children].find((r) => r.querySelector('.signstep-label').textContent === ROW);
const answer = (status, cause) => () => new Response(null, { status, headers: cause ? { 'X-Nib-Kept-Copy-Cause': cause } : {} });
async function reopen() {
  setNextDocument({ numPages: 1 });
  $('pathInput').value = '/tmp/nib-harness/Lease.pdf';
  $('openGo').click();
  await settle();
  await settle();
}
async function reopenCard() {
  doc.querySelector('.modetab[data-tab="collaborate"]').click(); // the mode that shows the checklist
  await settle();
  const head = simpleSignHead();
  if (head.getAttribute('aria-expanded') === 'true') head.click(); // a click on the open card closes it
  head.click();
  await settle();
  await settle();
}

test('the kept-copy row is asked only on demand, with HEAD, pinned to the open document', async () => {
  const before = probes.length;
  const opened = opens;
  await reopen(); // a load, with the card closed: nothing may be asked
  assert.ok(opens > opened, 'stimulus: the document was not loaded again');
  assert.equal(probes.length, before, 'the probe ran with the Simple Sign card closed — it is on demand only (D10)');
  probeAnswer = answer(422, 'no-signature');
  await reopenCard();
  assert.ok(probes.length > before, 'stimulus: opening the Simple Sign card did not ask the server');
  const p = probes.at(-1);
  assert.equal(p.method, 'HEAD', 'the checklist fetched the kept copy\'s BYTES to learn whether one exists');
  assert.equal(p.headers['X-Nib-Doc'], 'test-epoch:1', 'the probe was not pinned to the open document (ADR-001)');
});

test('— when the document carries no signature, with its own hover', async () => {
  probeAnswer = answer(422, 'no-signature');
  const n = probes.length;
  await reopenCard();
  assert.ok(probes.length > n, 'stimulus: opening the card did not ask the server');
  const r = keptRow();
  assert.ok(r, 'the checklist has no kept-copy row');
  assert.equal(r.dataset.state, 'untracked');
  assert.match(r.querySelector('.signstep-mark').title, /carries no signature/, 'the — row reads the generic hover, not its own');
  assert.match(r.querySelector('.signstep-mark').title, /click to mark this done yourself/, 'the row no longer says it can be ticked by hand');
  assert.match(r.querySelector('.signstep-mark').getAttribute('aria-label'), /carries no signature/,
    'a screen reader hears "Nib cannot tell" where the hover says the document carries no signature');
  assert.equal(r.querySelector('.signstep-extra'), null);
});

test('✓ when a kept copy matches, labelled as what Nib saw', async () => {
  probeAnswer = answer(200);
  await reopenCard();
  const r = keptRow();
  assert.equal(r.dataset.state, 'done', 'a matching kept copy did not tick the row');
  assert.equal(r.dataset.by, 'nib', 'the probed tick is labelled as the user\'s own claim');
  assert.match(r.querySelector('.signstep-mark').title, /matches this file/);
});

test('○ when none matches, with how many are kept and a link to the list', async () => {
  keptNow = { kept: [
    { name: 'kept_deed_20261002-090000-0011aabb.pdf', document: 'deed', keptAt: '2026-10-02 09:00:00', size: 2048 },
    { name: 'kept_lease_20261001-090000-2233ccdd.pdf', document: 'lease', keptAt: '2026-10-01 09:00:00', size: 1024 },
  ], totalBytes: 3072 };
  probeAnswer = answer(422, 'none-kept');
  await reopenCard();
  const r = keptRow();
  assert.equal(r.dataset.state, 'todo', 'no matching kept copy, and the row did not read ○');
  assert.match(r.querySelector('.signstep-mark').title, /No kept copy matches this file — 2 copies are kept/);
  const link = r.querySelector('.signstep-extra');
  assert.ok(link, 'the ○ row carries no link to the list');
  assert.match(link.textContent, /2 kept/);
  link.click();
  await settle();
  assert.equal($('keptModal').hidden, false, 'the ○ row\'s link did not open the list');
  $('keptClose').click();
  keptNow = { kept: [], totalBytes: 0 };
});

test('a failed probe reads —, never the last answer it gave', async () => {
  probeAnswer = answer(200);
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'done', 'setup: the row was not ✓ before the failure');
  probeAnswer = answer(500);
  await reopenCard();
  const r = keptRow();
  assert.equal(r.dataset.state, 'untracked', 'a failed probe left the earlier ✓ standing');
  assert.match(r.querySelector('.signstep-mark').title, /could not check/);
  assert.match(r.querySelector('.signstep-mark').getAttribute('aria-label'), /could not check/);
  probeAnswer = answer(422, 'unreadable');
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'untracked');
  assert.match(keptRow().querySelector('.signstep-mark').title, /could not be read/);
});

test('a Finalize that kept a copy, and a removal, ask again', async () => {
  probeAnswer = answer(422, 'no-signature');
  await reopenCard();
  finalizeAnswer = new Response(new Uint8Array([37, 80, 68, 70]), { status: 200,
    headers: { 'Content-Type': 'application/pdf', 'X-Nib-Kept': 'kept_lease_20261003-090000-0011aabb.pdf' } });
  await openModal();
  let n = probes.length; // after the modal opened, so only the Finalize's answer can move it
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  assert.ok(probes.length > n, 'a Finalize that kept a copy did not re-ask the row');
  // The open view is still the unsigned document, so the row stays — (ADR-073), never a tick from the response.
  assert.equal(keptRow().dataset.state, 'untracked', 'the row ticked from the Finalize response rather than the probe');
  $('saveAsCancel').click();
  await settle();
  n = probes.length;
  keptNow = { kept: [{ name: 'kept_deed_20261002-090000-0011aabb.pdf', document: 'deed', keptAt: '2026-10-02 09:00:00', size: 2048 }], totalBytes: 2048 };
  removeAnswer = true;
  doc.defaultView.confirm = () => true;
  $('keptBtn').click();
  await settle();
  $('keptList').querySelector('button').click();
  await settle();
  await settle();
  assert.ok(probes.length > n, 'a removal did not re-ask the row');
  $('keptClose').click();
});

test('a load asks again only when the document\'s signatures changed, and a late answer about the old ones is dropped', async () => {
  probeAnswer = answer(200);
  await reopenCard(); // the card stays open from here
  assert.equal(keptRow().dataset.state, 'done', 'setup');
  let n = probes.length;
  await reopen(); // same id, same signatures: the held answer stands
  assert.equal(probes.length, n, 'a load that changed nothing about the document asked again');
  assert.equal(keptRow().dataset.state, 'done');

  // The answer about the OLD state is held open; the signatures change underneath it (ADR-001).
  let release;
  probeAnswer = () => new Promise((r) => { release = () => r(new Response(null, { status: 200 })); });
  openSig = { state: 'untampered', signers: [{ coverageEnd: 100, name: 'Me' }] };
  await reopen();
  assert.equal(probes.length, n + 1, 'a document whose signatures changed was not asked again');
  assert.notEqual(keptRow().dataset.state, 'done', 'the ✓ about the document as it WAS stood while its signatures changed');
  probeAnswer = answer(422, 'none-kept');
  openSig = { state: 'untampered', signers: [{ coverageEnd: 100, name: 'Me' }, { coverageEnd: 200, name: 'Me' }] };
  await reopen();
  assert.equal(keptRow().dataset.state, 'todo', 'setup: the second answer did not land');
  release(); // the first answer arrives last
  await settle();
  await settle();
  assert.equal(keptRow().dataset.state, 'todo', 'an answer about the document as it was overwrote the answer about it as it is');
  openSig = { state: 'unsigned' };
});

test('a check that answers late is not applied over a newer one for the same document', async () => {
  probeAnswer = answer(200);
  await reopenCard();
  let release;
  probeAnswer = () => new Promise((r) => { release = () => r(new Response(null, { status: 200 })); });
  await reopenCard(); // the older ask, held open: it would say ✓
  probeAnswer = answer(422, 'no-signature');
  await reopenCard(); // the newer ask answers first: —
  assert.equal(keptRow().dataset.state, 'untracked', 'setup: the newer answer did not land');
  release();
  await settle();
  await settle();
  assert.equal(keptRow().dataset.state, 'untracked', 'an older check that answered late overwrote the newer answer');
});

test('a load in another mode asks nothing — the checklist is not on screen', async () => {
  probeAnswer = answer(200);
  await reopenCard();
  assert.equal(simpleSignHead().getAttribute('aria-expanded'), 'true', 'setup: the card is not open');
  doc.querySelector('.modetab[data-tab="secure"]').click(); // the real door; it leaves the card open
  await settle();
  assert.equal(doc.querySelector('.tbtab[data-tab="collaborate"]').classList.contains('active'), false,
    'stimulus: the mode did not change');
  openSig = { state: 'untampered', signers: [{ coverageEnd: 300, name: 'Me' }] }; // a changed key: it WOULD ask
  const n = probes.length;
  await reopen();
  assert.equal(probes.length, n, 'a load asked the server while the checklist was in a mode not on screen');
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  openSig = { state: 'unsigned' };
});

test('a forced re-ask does not show the answer it is replacing while it waits', async () => {
  probeAnswer = answer(200);
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'done', 'setup');
  let release;
  probeAnswer = () => new Promise((r) => { release = () => r(new Response(null, { status: 422, headers: { 'X-Nib-Kept-Copy-Cause': 'no-signature' } })); });
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'untracked', 'the old ✓ stood while the check that may overturn it was in flight');
  assert.match(keptRow().querySelector('.signstep-mark').title, /not checked this document yet/);
  release();
  await settle();
  await settle();
  assert.match(keptRow().querySelector('.signstep-mark').title, /carries no signature/);
});

test('a load whose signature VERDICT changed asks again, even where the coverage ends did not', async () => {
  openSig = { state: 'untampered', signers: [{ coverageEnd: 400, name: 'Me' }] };
  await reopen();
  probeAnswer = answer(200);
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'done', 'setup');
  const n = probes.length;
  openSig = { state: 'invalid', signers: [{ coverageEnd: 400, name: 'Me' }] }; // a rewrite that kept the /ByteRange number
  probeAnswer = answer(422, 'none-kept');
  await reopen();
  assert.equal(probes.length, n + 1, 'a document whose signature broke, at the same coverage end, was not asked again');
  assert.equal(keptRow().dataset.state, 'todo');
  openSig = { state: 'unsigned' };
});
