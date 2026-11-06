// Scan… is offered only where the server found scanimage (/pending 903, ADR-213). A machine without it must not
// show a button that cannot work, so the default status (no `scanner` key) leaves it hidden. One boot per file.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({});
await h.settle();

test('without scanimage the Scan… button stays hidden, and so does its dialog', () => {
  const button = h.document.getElementById('scannerBtn');
  assert.ok(button, 'there is no Scan… button in the page at all');
  assert.equal(button.hidden, true, 'the button shows on a machine with no scanner tool');
  assert.equal(h.document.getElementById('scannerModal').hidden, true);
});
