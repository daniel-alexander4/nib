// The freeze's SURFACE, and the ceremony record's label (P06.S09, D29).
//
// **The server-side freeze was built at P07.S02a and is guarded** —
// `TestEveryMutatingRouteReachesTheCeremonyFreeze` asserts the routing for the whole mutating
// inventory. What it cannot say is whether the user ever reads the refusal, and until this slice
// they did not: `pageOp` answered a failed mutation with `toast('page operation failed')` and threw
// the server's sentence away. D29's rule is that the refusal **names the ceremony**; at that route
// it named nothing, so a user whose every edit was refused had no account of why.
//
// **Driven through a real edit, not asserted on a flag**, which is the criterion's own wording: the
// test clicks a page operation and reads what the user is told.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/lease.pdf';
// The server's own sentence, verbatim from `ceremonyFreeze`'s refusal.
const FROZEN = 'this document is part of a signing ceremony';

const h = await boot({
  routes: {
    '/api/open': () => ({
      id: 'test-epoch:1', name: 'lease.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: true, canRedo: false, inCeremony: true,
    }),
    // A whole Response, because the STATUS is what is under test — boot.mjs's own convention.
    '/api/pages': () => new Response(JSON.stringify({ error: FROZEN + ', so it cannot be edited' }),
      { status: 409, headers: { 'Content-Type': 'application/json' } }),
    '/api/attachments': () => ({
      attachments: [
        { name: 'nib-ceremony.json', desc: '', ceremony: true },
        { name: 'schedule-a.txt', desc: 'the rent schedule' },
      ],
    }),
  },
});
const { document: doc, settle } = h;

test('a refused edit tells the user it is the ceremony, not "page operation failed"', async () => {
  setNextDocument({ numPages: 3 });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
  // SETUP: a document is open, or the page operation below never fires and the toast is empty for
  // a reason that has nothing to do with the freeze.
  assert.equal(doc.getElementById('viewerWrap').className, 'has-doc',
    'setup: no document is open');

  doc.getElementById('rotateRightBtn').click();
  await settle();

  const toast = doc.getElementById('toast');
  const said = toast ? toast.textContent : '';
  assert.ok(said, 'setup: nothing was toasted at all, so the refusal never reached this surface');
  assert.match(said, new RegExp(FROZEN),
    `a refused edit said "${said}". The server's refusal names the ceremony and says where the ` +
    'document is; a generic "page operation failed" throws that away, and a user whose every edit ' +
    'is refused is left with no account of why. D29 says the refusal NAMES the proceeding.');
});

test('the ceremony record is labelled in the attachments list', async () => {
  // Re-opened rather than relying on the first test's document: `attachBtn` refuses without one,
  // and a file that boots once (see boot.mjs) must not make one test's cleanup another's setup.
  setNextDocument({ numPages: 3 });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
  assert.equal(doc.getElementById('viewerWrap').className, 'has-doc',
    'setup: no document is open, so the attachments button refuses before it lists anything');
  doc.getElementById('attachBtn').click();
  await settle();
  await settle();
  const body = doc.getElementById('attachBody');
  const rows = body.querySelectorAll('.attachrow');
  // SETUP: both attachments rendered. With one row the label below could be the only thing drawn
  // and would not show that the OTHER file is left alone.
  assert.equal(rows.length, 2, `the panel drew ${rows.length} rows for two attachments`);
  const text = body.textContent;
  assert.match(text, /signing ceremony/,
    'the ceremony record is listed as an anonymous embedded file. It is the one attachment in the ' +
    'product the user did not add, and the reason every edit on this document is refused.');
  // And the user's own attachment is NOT labelled — otherwise the label says nothing.
  const plain = Array.from(rows).find((r) => r.textContent.includes('schedule-a.txt'));
  assert.ok(plain, 'setup: the ordinary attachment did not render');
  assert.equal(/signing ceremony/.test(plain.textContent), false,
    'the user’s own attachment is labelled as the ceremony record too, so the label distinguishes ' +
    'nothing');
});
