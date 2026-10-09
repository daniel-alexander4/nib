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
let openCanUndo = false;
let undoSig = { state: 'unsigned' };
let openFlags; // a document prepared for signing carries its flags in the open answer (/pending 814)
let probeAnswer = () => new Response(null, { status: 422, headers: { 'X-Nib-Kept-Copy-Cause': 'no-signature' } });
const h = await boot({
  routes: {
    '/api/open': () => { opens++; return { id: 'test-epoch:1', name: 'Lease.pdf', path: '/tmp/nib-harness/Lease.pdf', canSave: true,
      canUndo: openCanUndo, canRedo: false, signature: openSig, flags: openFlags }; },
    // A server operation that changes the signatures (an undo of a signing), answered with the document's new meta.
    '/api/undo': () => ({ id: 'test-epoch:1', name: 'Lease.pdf', path: '/tmp/nib-harness/Lease.pdf', canSave: true,
      canUndo: false, canRedo: true, signature: undoSig }),
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
// row that rendered any state without reading the answer would fail the other two. Re-asked by bringing the Simple Sign
// PAGE forward again (ADR-105: the checklist is a page, and its coming forward is the one trigger that always asks).
const ROW = 'A copy kept when you signed';
const simpleSignHead = () => [...doc.querySelectorAll('#commands .sbhead.groupcard')].find((x) => x.textContent.trim() === 'Simple Sign');
const stepsPage = () => $('signingStepsPage');
const stepsTab = () => doc.querySelector('#tabstrip [data-apppage="signingStepsPage"]');
const keptRow = () => [...$('signSteps').querySelectorAll('.signstep')].find((r) => r.querySelector('.signstep-label').textContent === ROW);
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
  // An entry clicked while its page is already in front does nothing, so the page is closed first: the click
  // below is then always the page coming forward.
  if (!stepsPage().hidden) stepsTab().querySelector('.tabclose').click();
  simpleSignHead().click();
  await settle();
  await settle();
}

// The page coming forward from its TAB — the forced ask, with the page left open rather than closed first.
async function pageForward() {
  if (stepsPage().hidden) stepsTab().click();
  await settle();
  await settle();
}
// Back to the document. A page in front is, for Finalize and for Undo, no document (ADR-104 §5), so anything that
// acts on the document is done from here — which is also why none of it can ask the row while it is on screen.
async function toDocument() {
  doc.querySelector('.modetab[data-tab="secure"]').click();
  await settle();
  assert.equal(stepsPage().hidden, true, 'stimulus: the page is still in front of the document');
}

test('the kept-copy row is asked only on demand, with HEAD, pinned to the open document', async () => {
  const before = probes.length;
  const opened = opens;
  await reopen(); // a load, with the card closed: nothing may be asked
  assert.ok(opens > opened, 'stimulus: the document was not loaded again');
  assert.equal(probes.length, before, 'the probe ran with the Simple Sign page not showing — it is on demand only (D10)');
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

test('after a Finalize that kept a copy the page asks afresh when it comes back; a removal with the page in front asks at once', async () => {
  probeAnswer = answer(422, 'no-signature');
  await reopenCard();
  finalizeAnswer = new Response(new Uint8Array([37, 80, 68, 70]), { status: 200,
    headers: { 'Content-Type': 'application/pdf', 'X-Nib-Kept': 'kept_lease_20261003-090000-0011aabb.pdf' } });
  await toDocument();
  await openModal();
  let n = probes.length; // after the modal opened, so only what follows can move it
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  $('saveAsCancel').click();
  await settle();
  // The row is not on screen, so the Finalize asks nothing and ticks nothing; the page asks as it comes forward.
  assert.notEqual(keptRow().dataset.state, 'done', 'the row ticked from the Finalize response');
  await pageForward();
  assert.ok(probes.length > n, 'the page came back after a kept Finalize without asking afresh');
  // The open view is still the unsigned document, so the row stays — (ADR-073), never a tick from the response.
  assert.equal(keptRow().dataset.state, 'untracked', 'the row ticked from the Finalize response rather than the probe');
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
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'done', 'setup');
  let n = probes.length;
  await reopen(); // same id, same signatures, loaded with the page still in front: the held answer stands
  assert.equal(probes.length, n, 'a load that changed nothing about the document asked again');
  assert.equal(keptRow().dataset.state, 'done');
  // The load showed the document (a document the user opens is what she asked to see), so the page comes forward
  // again before each load below — and that is itself an ask, counted from after it.
  await pageForward();
  n = probes.length;

  // The answer about the OLD state is held open; the signatures change underneath it (ADR-001).
  let release;
  probeAnswer = () => new Promise((r) => { release = () => r(new Response(null, { status: 200 })); });
  openSig = { state: 'untampered', signers: [{ coverageEnd: 100, name: 'Me' }] };
  await reopen();
  assert.equal(probes.length, n + 1, 'a document whose signatures changed was not asked again');
  assert.notEqual(keptRow().dataset.state, 'done', 'the ✓ about the document as it WAS stood while its signatures changed');
  probeAnswer = answer(422, 'none-kept');
  openSig = { state: 'untampered', signers: [{ coverageEnd: 100, name: 'Me' }, { coverageEnd: 200, name: 'Me' }] };
  await pageForward();
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
  assert.equal(stepsPage().hidden, false, 'setup: the page is not in front');
  doc.querySelector('.modetab[data-tab="secure"]').click(); // the real door; it leaves the page, open in its tab
  await settle();
  assert.equal(stepsPage().hidden, true, 'stimulus: leaving the Signing menu did not leave the page');
  assert.ok(stepsTab(), 'stimulus: the page was closed, not left');
  assert.equal(doc.querySelector('.tbtab[data-tab="collaborate"]').classList.contains('active'), false,
    'stimulus: the mode did not change');
  openSig = { state: 'untampered', signers: [{ coverageEnd: 300, name: 'Me' }] }; // a changed key: it WOULD ask
  const n = probes.length;
  await reopen();
  assert.equal(probes.length, n, 'a load asked the server while the checklist was in a mode not on screen');
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  await settle();
  // Back in the Signing mode the page is still only a tab (entering a mode opens no page), so no row is on screen
  // until the page comes forward — and that is the forced ask.
  assert.equal(stepsPage().hidden, true, 'the checklist is on screen without having been asked');
  assert.equal(probes.length, n, 'returning to the Signing mode asked with the checklist not on screen');
  stepsTab().click();
  await settle();
  await settle();
  assert.ok(probes.length > n, 'the page coming forward from its tab did not ask');
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

// ── The P04 phase-close review's fixes ─────────────────────────────────────────────────────────────────────────────

test('nothing but the probe ticks the row after a kept Finalize — not even while the probe is still out', async () => {
  probeAnswer = answer(422, 'no-signature');
  await reopenCard();
  let release;
  probeAnswer = () => new Promise((r) => { release = () => r(new Response(null, { status: 422, headers: { 'X-Nib-Kept-Copy-Cause': 'no-signature' } })); });
  finalizeAnswer = new Response(new Uint8Array([37, 80, 68, 70]), { status: 200,
    headers: { 'Content-Type': 'application/pdf', 'X-Nib-Kept': 'kept_lease_20261003-100000-0011aabb.pdf' } });
  await toDocument();
  await openModal();
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  $('saveAsCancel').click();
  await settle();
  assert.notEqual(keptRow().dataset.state, 'done', 'the row ticked from the Finalize response');
  await pageForward(); // the ask, held open
  assert.ok(release, 'stimulus: the page coming back after the kept Finalize did not ask the server');
  assert.notEqual(keptRow().dataset.state, 'done', 'the row ticked from the Finalize response while its own check was still out');
  release();
  await settle();
  await settle();
  assert.equal(keptRow().dataset.state, 'untracked');
});

test('a second press of Finalize while the first is signing signs nothing more', async () => {
  let answerIt;
  const held = new Promise((r) => { answerIt = r; });
  const before = posted.length;
  finalizeAnswer = { clone: () => held }; // the route hands back what clone() returns, so the request stays open
  await toDocument();
  await openModal();
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  $('fzGo').click();
  await settle();
  assert.equal(posted.length, before + 1, 'a second press signed again while the first signing was in flight');
  assert.equal($('fzGo').disabled, true, 'the button stayed live while a signing was in flight');
  answerIt(new Response(new Uint8Array([37, 80, 68, 70]), { status: 200, headers: { 'Content-Type': 'application/pdf' } }));
  await settle();
  await settle();
  assert.equal($('fzGo').disabled, false, 'the button was not given back after the signing answered');
  $('saveAsCancel').click();
  await settle();
});

test('after a server operation that changes the signatures the page asks afresh, off the NEW signatures', async () => {
  openSig = { state: 'valid', signers: [{ coverageEnd: 500, name: 'Me' }] };
  openCanUndo = true;
  await reopen();
  probeAnswer = answer(200);
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'done', 'setup');
  const n = probes.length;
  undoSig = { state: 'unsigned' };
  probeAnswer = answer(422, 'no-signature');
  setNextDocument({ numPages: 1 });
  await toDocument(); // Undo is the document's: with the page in front the shortcut does nothing (ADR-104 §5)
  doc.defaultView.dispatchEvent(new doc.defaultView.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }));
  await settle();
  await settle();
  await settle();
  await pageForward();
  assert.ok(probes.length > n, 'stimulus: the page came back after the undo that removed the signature without asking');
  assert.equal(keptRow().dataset.state, 'untracked', 'the row still answers for the signatures the undo removed');
  assert.match(keptRow().querySelector('.signstep-mark').title, /carries no signature/);
  openSig = { state: 'unsigned' };
  openCanUndo = false;
  await reopen();
});

test('a failed check is asked again on the next load — a failure is not a held answer', async () => {
  probeAnswer = answer(500);
  await reopenCard();
  assert.equal(keptRow().dataset.state, 'untracked', 'setup');
  const n = probes.length;
  await reopen(); // the same key, at once: a load runs on every overlay edit, so a failure is not retried immediately
  assert.equal(probes.length, n, 'a load re-asked a check that had just failed — every edit would re-ask a broken folder');
  await pageForward(); // the load showed the document; the page again, failing again, so the back-off runs from here
  assert.equal(keptRow().dataset.state, 'untracked', 'setup: the forced ask did not fail');
  const m = probes.length;
  const realNow = Date.now;
  Date.now = () => realNow() + 11000; // past the back-off
  try {
    probeAnswer = answer(200);
    await reopen();
  } finally { Date.now = realNow; }
  assert.ok(probes.length > m, 'a load after the back-off, with the page in front, did not ask the failed check again');
  assert.ok(probes.length > n, 'a load after the back-off did not ask the failed check again');
  assert.equal(keptRow().dataset.state, 'done');
});

// ADR-105. The checklist was a card, and with the sidebar shut its group sat in the toolbar and counted as on screen.
// It is a page: with the sidebar shut the ENTRY is what sits in the toolbar's pane, the checklist is on its page, and
// the page is asked when it comes forward by either door — the entry's own button, or its tab.
test('with the sidebar shut the entry in the toolbar opens the page, and the page is asked as it comes forward — by the entry and by its tab', async () => {
  probeAnswer = answer(422, 'no-signature');
  await reopenCard();
  stepsTab().querySelector('.tabclose').click();
  await settle();
  let n = probes.length;
  probeAnswer = answer(200);
  $('toggleSidebarBtn').click(); // shut: the panes move into the toolbar
  await settle();
  await settle();
  const entry = doc.querySelector('#toolbar .tbgroup[data-label="Simple Sign"] button[data-apppage="signingStepsPage"]');
  assert.ok(entry, 'stimulus: the Simple Sign entry did not move into the toolbar');
  assert.equal(doc.querySelector('#toolbar #signSteps'), null, 'the checklist itself is in the toolbar — it belongs to its page');
  assert.equal(probes.length, n, 'shutting the sidebar asked with the page not showing');
  entry.click();
  await settle();
  await settle();
  assert.equal(stepsPage().hidden, false, 'the entry in the toolbar did not open the page');
  assert.ok(probes.length > n, 'the page came forward from the toolbar entry without asking');
  assert.equal(keptRow().dataset.state, 'done');
  doc.querySelector('.modetab[data-tab="secure"]').click();
  await settle();
  n = probes.length;
  probeAnswer = answer(422, 'none-kept');
  stepsTab().click(); // back to the checklist by its tab
  await settle();
  await settle();
  assert.ok(probes.length > n, 'returning to the page by its tab did not ask again');
  assert.equal(keptRow().dataset.state, 'todo', 'the row kept the answer from before it left the screen');
  $('toggleSidebarBtn').click();
  await settle();
});

// ── /pending 814: Complete & sign's tick, in the signing banner (ADR-078) ───────────────────────────────────────────
//
// The recipient's one-press flow keeps a copy only when its own tick says so: off at every opening of the banner, sent
// as `keep` exactly as the modal sends it, and refused in the modal's own words.
async function openForSigning() {
  openFlags = [{ page: 1, frac: [0.1, 0.1, 0.3, 0.15], type: 'date' }];
  await reopen();
  openFlags = undefined;
  assert.equal($('signBanner').hidden, false, 'stimulus: a document with a flag did not show the signing banner');
}
async function completeAndSign() {
  doc.defaultView.confirm = () => true; // "1 field still empty — complete and sign anyway?"
  $('signDone').click(); // Finish & sign: the flag is unfilled, so this is the one-press entry
  await settle();
  await settle();
  await settle();
}

test('Complete & sign\'s tick is off at every opening of the banner and says what the modal says', async () => {
  await openForSigning();
  assert.equal($('signKeep').checked, false, 'the banner\'s tick started ON');
  assert.equal($('signKeepHint').textContent, $('fzKeepHint').textContent,
    'the banner\'s disclosure is not the Finalize modal\'s — two sources for the words that make the opt-in informed');
  assert.match($('signKeepHint').textContent, /unencrypted/);
  $('signKeep').checked = true;
  await openForSigning();
  assert.equal($('signKeep').checked, false, 'a tick from the last opening of the banner was remembered');
});

test('Complete & sign sends the tick as keep, with the document\'s name, and says where the copy went', async () => {
  finalizeAnswer = new Response(new Uint8Array([37, 80, 68, 70]), { status: 200,
    headers: { 'Content-Type': 'application/pdf', 'X-Nib-Kept': 'kept_lease_20261003-120000-0011aabb.pdf' } });
  await openForSigning();
  const before = posted.length;
  $('signKeep').checked = true;
  await completeAndSign();
  assert.equal(posted.length, before + 1, 'stimulus: Complete & sign did not post');
  const params = JSON.parse(posted.at(-1).get('params'));
  assert.equal(params.keep, true, 'the banner\'s tick did not reach the request');
  assert.match(params.name, /Lease/, 'the document\'s name did not reach the request');
  assert.match($('toast').textContent, /A copy was kept when you signed: ~\/nib\/signed\/kept_lease_/);
  assert.equal($('saveAsModal').hidden, false, 'stimulus: the signed file was not offered to save');
  $('saveAsCancel').click();
  await settle();

  finalizeAnswer = new Response(new Uint8Array([37, 80, 68, 70]), { status: 200, headers: { 'Content-Type': 'application/pdf' } });
  await openForSigning();
  await completeAndSign();
  assert.equal(JSON.parse(posted.at(-1).get('params')).keep, false, 'an unticked banner asked for a copy');
  $('saveAsCancel').click();
  await settle();
});

test('a copy Complete & sign could not keep is refused in the Finalize modal\'s words, and the banner stays', async () => {
  const refusal = () => new Response(JSON.stringify({ error: 'the document was not signed: Nib could not keep the copy you asked for',
    cause: 'copy-not-kept', reason: 'mkdir /home/x/nib/signed: not a directory' }), { status: 500, headers: { 'Content-Type': 'application/json' } });
  finalizeAnswer = refusal();
  await openModal();
  $('fzKeep').checked = true;
  $('fzGo').click();
  await settle();
  await settle();
  const modalSaid = $('toast').textContent;
  assert.match(modalSaid, /Not signed/, 'setup: the modal did not word the refusal');
  $('fzCancel').click();

  finalizeAnswer = refusal();
  await openForSigning();
  $('toast').textContent = '';
  $('signKeep').checked = true;
  await completeAndSign();
  assert.equal($('toast').textContent, modalSaid, 'Complete & sign worded the refusal differently from Finalize');
  assert.doesNotMatch($('toast').textContent, /Could not complete and sign/);
  assert.equal($('signBanner').hidden, false, 'the banner went away — unticking and signing again is no longer one press away');
  assert.equal($('signKeep').checked, true, 'the refusal cleared the tick it tells the user to untick');
  assert.equal($('saveAsModal').hidden, true, 'a refused signing offered a file to save');
});
