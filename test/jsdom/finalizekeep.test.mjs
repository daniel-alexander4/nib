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
const h = await boot({
  routes: {
    '/api/open': () => ({ id: 'test-epoch:1', name: 'Lease.pdf', path: '/tmp/nib-harness/Lease.pdf', canSave: true,
      canUndo: false, canRedo: false, signature: { state: 'unsigned' } }),
    '/api/identity/external': { present: false },
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/finalize': (opts) => { posted.push(opts.body); return finalizeAnswer.clone(); },
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
