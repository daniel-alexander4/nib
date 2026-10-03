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
const h = await boot({
  routes: {
    '/api/open': () => ({ id: 'test-epoch:1', name: 'Lease.pdf', path: '/tmp/nib-harness/Lease.pdf', canSave: true,
      canUndo: false, canRedo: false, signature: { state: 'unsigned' } }),
    '/api/identity/external': { present: false },
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/finalize': (opts) => { posted.push(opts.body); return finalizeAnswer.clone(); },
    '/api/kept': () => keptNow,
    '/api/kept/remove': (opts) => { removed.push(JSON.parse(opts.body).name); return { removed: removeAnswer }; },
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
