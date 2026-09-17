// Tier 2: dragging a file onto the window — /pending 540.
//
// **The window `drop` listener had no test at any tier.** The only `dataTransfer` use under
// `test/` was the per-view thumbnail reorder, which is a different listener entirely. That is why
// a filter discarding every convertible document, silently, survived two rounds of work on this
// exact handler: /pending 400 widened it from one MIME type to three and left the discard as it
// was, while stating in its own comment the rule it was breaking.
//
// What is asserted here is the RULE, not the list: a file the server would have answered for must
// reach the server. A whitelist of any length fails that, which is why there is no longer one.

import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

// A stub shaped like the Go handler's refusal: the point of these tests is that the file REACHES
// the server, not what it answers, so every drop gets the same 415 and the assertions read the
// upload calls rather than the outcome.
const { window, document, calls } = await boot({
  // A 415 Response, the shape the Go handler returns for a file it will not open. The point of
  // these tests is that the file REACHES the server, so every drop gets the same refusal and the
  // assertions read the upload calls rather than the outcome.
  routes: {
    '/api/upload': () => new Response(JSON.stringify({ error: "that file isn't a PDF" }), {
      status: 415, headers: { 'Content-Type': 'application/json' },
    }),
  },
});

// dropFiles fires a real `drop` event carrying `files`, the way a browser does.
function dropFiles(files) {
  const before = calls.length;
  const ev = new window.Event('drop', { bubbles: true, cancelable: true });
  // jsdom has no DataTransfer constructor; the handler reads `.files` and nothing else.
  Object.defineProperty(ev, 'dataTransfer', { value: { files } });
  window.dispatchEvent(ev);
  return calls.slice(before);
}

const file = (name, type) => ({ name, type, size: 10, slice: () => ({}) });

test('a dropped document reaches the server whatever the browser typed it', async () => {
  for (const f of [
    file('report.docx', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'),
    file('notes.md', 'text/markdown'),
    file('sheet.ods', 'application/vnd.oasis.opendocument.spreadsheet'),
    // A PDF the browser failed to type — ordinary, and previously discarded in silence.
    file('contract.pdf', ''),
    file('scan.png', 'image/png'),
  ]) {
    const made = dropFiles([f]);
    await new Promise((r) => setTimeout(r, 0));
    const uploads = made.filter((c) => c.url.includes('/api/upload'));
    assert.equal(uploads.length, 1,
      `dropping ${f.name} (type ${JSON.stringify(f.type)}) made ${uploads.length} upload call(s). ` +
      `The server is what decides whether a file opens — it sniffs magic bytes, converts an image ` +
      `and names the convert route for a document — so a file discarded here never gets an answer, ` +
      `and the user is told nothing at all.`);
  }
});

test('a drop carrying no file says nothing, because nothing was asked for', async () => {
  const made = dropFiles([]);
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(made.filter((c) => c.url.includes('/api/upload')).length, 0,
    'an empty drop — dragged text, a link — uploaded something');
});

test('a multi-file drop opens one and SAYS the others were not opened', async () => {
  const made = dropFiles([file('a.pdf', 'application/pdf'), file('b.pdf', 'application/pdf')]);
  // **Read the toast BEFORE awaiting the upload.** The notice is synchronous and the upload's own
  // refusal lands after it in the same element, so awaiting first reads the second message and
  // reports the first as missing — which is what the initial version of this assertion did.
  const el = document.querySelector('#toast, .toast');
  const said = el ? el.textContent : '';
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(made.filter((c) => c.url.includes('/api/upload')).length, 1,
    'a multi-file drop should open exactly one');
  assert.ok(/drop one file at a time/i.test(said),
    `silently taking one file of two is the same defect one level up; the toast read ${JSON.stringify(said)}`);
});
