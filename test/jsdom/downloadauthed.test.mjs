// A download is fetched with the token in its header and saved from a blob — never navigated to
// (ADR-054; the review of /pending 704, #2). Navigating to `/api/vault/export?auth=<token>` put the
// token in the address bar and the history whenever the answer was not a file, and replaced the app.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const saved = [];
const h = await boot({
  routes: {
    '/api/vault/export': () => new Response(new Uint8Array([1, 2, 3]), {
      status: 200, headers: { 'Content-Disposition': 'attachment; filename="vault.nib"' },
    }),
  },
});
globalThis.URL.createObjectURL = () => 'blob:nib-test';
globalThis.URL.revokeObjectURL = () => {};
h.window.HTMLAnchorElement.prototype.click = function click() { saved.push({ href: this.href, name: this.download }); };

test('the vault backup is fetched with the token and saved under the server\'s filename', async () => {
  const before = h.window.location.href;
  h.document.getElementById('backupBtn').click();
  await h.settle();
  const call = h.calls.find((c) => c.url.endsWith('/api/vault/export'));
  assert.ok(call, 'the backup was never fetched');
  assert.equal(call.headers['X-CSRF-Token'], 'test-csrf');
  assert.doesNotMatch(call.url, /auth=/, 'the token rode in the URL');
  assert.deepEqual(saved, [{ href: 'blob:nib-test', name: 'vault.nib' }]);
  assert.equal(h.window.location.href, before, 'the page navigated away');
});
