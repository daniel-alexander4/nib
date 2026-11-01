// /pending 929 — the Contents page dialog. What this tier reaches: that the dialog opens, holds its numbers to the
// bounds the server enforces, and sends exactly the fields the server reads, with the document's page count as the
// ceiling of its position. What it cannot: the page itself — internal/pdfops, and test/ui/contents.test.mjs end to end.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/contents.pdf';
let bodies = [];
const h = await boot({
  routes: {
    '/api/open': () => ({ id: 'test-epoch:1', name: 'contents.pdf', path: DOC, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false }),
    '/api/close': () => ({}),
    '/api/lastopen': () => [],
    '/api/pages': (opts) => {
      const form = opts.body;
      bodies.push(Object.fromEntries([...form.entries()].filter(([k]) => k !== 'pdf')));
      return { id: 'test-epoch:1', name: 'contents.pdf', path: DOC, canSave: true, signature: { state: 'unsigned' }, canUndo: true, canRedo: false };
    },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

test('the dialog sends the position, depth and heading, held to their bounds', async () => {
  setNextDocument({ numPages: 5, outline: null });
  $('pathInput').value = DOC;
  $('openGo').click();
  await settle(); await settle();
  $('contentsBtn').click();
  assert.equal($('contentsModal').hidden, false, 'the Contents page button opened nothing');
  assert.equal($('contentsAfter').max, '5', 'the position is not bounded by the page count');

  $('contentsAfter').value = '99'; // beyond the end
  $('contentsDepth').value = '2';
  $('contentsTitle').value = '  In this report ';
  bodies = [];
  $('contentsGo').click();
  await settle(); await settle();
  assert.equal(bodies.length, 1, `${bodies.length} requests were made`);
  assert.deepEqual(
    { op: bodies[0].op, after: bodies[0].after, depth: bodies[0].depth, title: bodies[0].title },
    { op: 'contents', after: '5', depth: '2', title: 'In this report' },
    'the request carried what the server would have to clamp, or dropped a field',
  );
});

test('a front insertion is position 0 and an empty heading is sent empty, for the server to default', async () => {
  $('contentsBtn').click();
  $('contentsAfter').value = '0';
  $('contentsTitle').value = '';
  bodies = [];
  $('contentsGo').click();
  await settle(); await settle();
  assert.equal(bodies[0].after, '0');
  assert.equal(bodies[0].title, '');
});
