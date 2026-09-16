// What a user whose vault file is present but unreadable is shown — /pending 502.
//
// A corrupt vault, or one written by a newer Nib, reached the client as `key-missing`, whose screen
// asks the user to find the SSH key it was set up with. No key fixes a vault that cannot be read,
// so the one recovery on offer was a dead end. The server now reports `vault-unreadable`; these
// assertions are what that state must mean to the person looking at it.
//
// ── Ceiling ─────────────────────────────────────────────────────────────────
// The server half (which errors map to this state) is `TestAnUnreadableVaultIsNotReportedAsAMissingKey`
// in internal/server. This tier sees only what the client renders from a status it is handed.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const UNREADABLE = {
  state: 'vault-unreadable',
  problem: 'the vault file cannot be read: corrupt vault: unexpected end of JSON input',
  vaultPath: '/home/u/.config/nib/vault.nib',
  version: 'test',
};
let statusCalls = 0;
let enrollCalls = 0;
const h = await boot({
  routes: {
    '/api/status': () => { statusCalls++; return UNREADABLE; },
    '/api/ssh/enroll': () => { enrollCalls++; return UNREADABLE; },
  },
});
const { document: doc, settle } = h;

function visibleText() {
  const shown = (el) => {
    for (let n = el; n; n = n.parentElement) if (n.hidden) return false;
    return true;
  };
  return [...doc.querySelectorAll('p, h1, h2, h3, label, span, div')]
    .filter((el) => shown(el) && el.children.length === 0 && el.textContent.trim())
    .map((el) => el.textContent.replace(/\s+/g, ' ').trim());
}

test('an unreadable vault names the file and the reason, not a missing key', async () => {
  await settle();
  const text = visibleText().join('\n');
  assert.match(text, /vault can't be read/i, 'the unreadable-vault title is not shown');
  assert.ok(text.includes(UNREADABLE.vaultPath), 'the screen does not say which file could not be read');
  assert.ok(text.includes('unexpected end of JSON input'), 'the screen does not say why the vault could not be read');
  assert.doesNotMatch(text, /can't read the SSH key/i,
    'an unreadable vault is still presented as a missing key — the user is sent to find a key no key can fix');
});

test('nothing on the screen offers to set up over the unreadable vault', async () => {
  await settle();
  const choice = doc.getElementById('keyChoice');
  // STIMULUS: the element exists, so its being hidden is a fact about this state and not a
  // renamed id reading as hidden forever.
  assert.ok(choice, 'no #keyChoice element — this assertion is not reading what it thinks');
  assert.equal(choice.hidden, true, 'the key-choice form is offered over a vault that could not be read');
});

test('Retry re-reads the status and never enrols', async () => {
  await settle();
  const before = statusCalls;
  doc.getElementById('authForm').dispatchEvent(new h.window.Event('submit', { bubbles: true, cancelable: true }));
  await settle();
  assert.ok(statusCalls > before, 'Retry did not re-read /api/status');
  assert.equal(enrollCalls, 0, 'Retry on an unreadable vault posted an enrolment');
});
