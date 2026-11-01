// /pending 874 (ADR-180) — Remove on a row of the finished-ceremonies list.
//
// What this tier reaches: the button is offered only where the cards offer their actions, the confirmation says the
// copy may be the only one with the user's signature, declining sends nothing, accepting posts that one id and
// redraws the list from the server's answer. What it cannot: that the folder is gone — internal/server's test.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const A = 'a'.repeat(32), B = 'b'.repeat(32);
let primary = true;
let ended = [
  { ceremony: A, state: 'completed', observed_at: '2026-09-01T09:00:00Z', name: 'Elm Row lease' },
  { ceremony: B, state: 'declined', observed_at: '2026-09-02T09:00:00Z' },
];
const posted = [];

const { document: doc, settle, confirms, setConfirmAnswer } = await boot({
  routes: {
    '/api/ceremonies': () => ({ primary, ceremonies: [], ended }),
    '/api/ceremonies/ended/remove': (opts) => {
      const body = JSON.parse((opts && opts.body) || '{}');
      posted.push(body.ceremony);
      ended = ended.filter((e) => e.ceremony !== body.ceremony);
      return { removed: true, folders: 1 };
    },
  },
});

async function showPanel() {
  doc.querySelector('.modetab[data-tab="collaborate"]')?.click();
  await settle();
  return doc.getElementById('ceremonyList');
}
const buttons = (host) => [...host.querySelectorAll('.cerendedremove')];

test('each finished ceremony has a Remove, and a second copy of Nib on this machine offers none', async () => {
  primary = false;
  let host = await showPanel();
  assert.equal(host.querySelectorAll('.cerended-row').length, 2, 'setup: the finished list did not render');
  assert.equal(buttons(host).length, 0, 'a non-primary Nib offers Remove, which the server would refuse');
  primary = true;
  doc.querySelector('.modetab[data-tab="edit"]')?.click();
  host = await showPanel();
  assert.equal(buttons(host).length, 2, 'the primary Nib does not offer Remove on both rows');
});

test('declining sends nothing; accepting posts that one id and redraws without it', async () => {
  const host = doc.getElementById('ceremonyList');
  setConfirmAnswer(false);
  buttons(host)[0].click();
  await settle();
  assert.equal(posted.length, 0, 'a declined confirmation still posted a removal');
  assert.equal(confirms.length, 1);
  assert.match(confirms[0], /Elm Row lease/, 'the confirmation does not name the ceremony');
  assert.match(confirms[0], /signature/, 'the confirmation does not say a copy with the user\'s signature is deleted');
  assert.match(confirms[0], /gone for good/, 'the confirmation does not say it may be the only copy');

  setConfirmAnswer(true);
  buttons(host)[0].click();
  await settle(); await settle();
  assert.deepEqual(posted, [A], `the request named ${JSON.stringify(posted)}, not the one row pressed`);
  assert.equal(host.querySelectorAll('.cerended-row').length, 1, 'the row is still listed after a removal');
  assert.equal(buttons(host).length, 1);
});
