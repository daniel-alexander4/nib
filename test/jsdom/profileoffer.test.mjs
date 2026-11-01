// /pending 917 — after a Save, Nib offers to keep what the user typed into form fields for Autofill.
//
// What this tier reaches: which fields are offered (text fields the user changed, not a password box, not a value the
// profile already holds), that nothing reaches the profile unless "Keep selected" is pressed, that a "no" is
// remembered for the session so the same values do not ask again, and that a kept value is MERGED into the profile
// rather than replacing it. What it cannot: that real pdf.js puts a typed value in its storage — test/ui/profileoffer.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument, lastDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/form.pdf';
let profile = { city: 'Leeds', phone: '0113 000' };
const posted = [];
const h = await boot({
  routes: {
    '/api/open': () => ({ id: 'test-epoch:1', name: 'form.pdf', path: DOC, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false }),
    '/api/close': () => ({}),
    '/api/lastopen': () => [],
    '/api/save': () => ({ rev: 2, signature: { state: 'unsigned' } }),
    '/api/profile': (opts) => {
      if (opts && opts.method === 'POST') { profile = JSON.parse(opts.body); posted.push(profile); return { status: 'ok' }; }
      return profile;
    },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

const FIELDS = {
  fullName: [{ id: 'f1', type: 'text' }],
  email: [{ id: 'f2', type: 'text' }],
  secret: [{ id: 'f3', type: 'text' }],
  agree: [{ id: 'f4', type: 'checkbox' }],
  city: [{ id: 'f5', type: 'text' }],
};
const typed = (values) => {
  const st = lastDocument.annotationStorage;
  st.clear();
  for (const [id, value] of Object.entries(values)) st.set(id, { value });
};
const rows = () => [...$('profileOfferList').querySelectorAll('input[type=checkbox]')].map((b) => `${b.dataset.name}=${b.dataset.value}`);
async function save() { $('saveBtn').click(); await settle(); await settle(); }

test('setup: a form is open', async () => {
  setNextDocument({ numPages: 1, outline: null, fieldObjects: FIELDS });
  $('pathInput').value = DOC;
  $('openGo').click();
  await settle(); await settle();
  // The password box is a rendered element; pdf.js marks it type=password.
  const pw = doc.createElement('input');
  pw.type = 'password';
  pw.dataset.elementId = 'f3';
  doc.querySelector('.viewerContainer').appendChild(pw);
  assert.ok(lastDocument, 'no document was made');
});

test('nothing typed, nothing offered', async () => {
  typed({});
  await save();
  assert.equal($('profileOfferModal').hidden, true, 'a Save with no typed values asked to keep something');
});

test('a Save offers the text fields that were typed — not a password, a checkbox or a value the profile has', async () => {
  typed({ f1: 'Jane Doe', f2: 'jane@example.com', f3: 'hunter2', f4: 'true', f5: 'Leeds' });
  await save();
  assert.equal($('profileOfferModal').hidden, false, 'typed values were not offered');
  assert.deepEqual(rows().sort(), ['email=jane@example.com', 'fullName=Jane Doe']);
  assert.deepEqual(posted, [], 'the profile was written before the user said yes');
});

test('"Not now" keeps nothing, and the same values do not ask again; a changed value does', async () => {
  $('profileOfferCancel').click();
  assert.equal($('profileOfferModal').hidden, true);
  assert.deepEqual(posted, []);
  await save();
  assert.equal($('profileOfferModal').hidden, true, 'a declined value asked again');
  typed({ f1: 'Jane Q Doe', f2: 'jane@example.com' });
  await save();
  assert.deepEqual(rows(), ['fullName=Jane Q Doe'], 'a changed value was not offered afresh, or a declined one was');
});

test('"Keep selected" merges the ticked values into the profile, and an unticked one is declined', async () => {
  typed({ f1: 'Jane R Doe', f2: 'jane2@example.com' });
  $('profileOfferCancel').click();
  await save();
  assert.equal($('profileOfferModal').hidden, false);
  const boxes = [...$('profileOfferList').querySelectorAll('input[type=checkbox]')];
  assert.equal(boxes.length, 2, `${boxes.length} offered, want fullName and email`);
  boxes.find((b) => b.dataset.name === 'email').checked = false;
  $('profileOfferYes').click();
  await settle(); await settle();
  assert.deepEqual(posted.at(-1), { city: 'Leeds', phone: '0113 000', fullName: 'Jane R Doe' },
    `the profile became ${JSON.stringify(posted.at(-1))}: the ticked value is added and nothing else is lost or added`);
  assert.equal($('profileOfferModal').hidden, true);
  await save();
  assert.equal($('profileOfferModal').hidden, true, 'the unticked value asked again');
});
