// Edit Objects' Replace… (PLAN-image-edit P03.S01): a picked file is one multipart request — the picture and the edit
// as the JSON the other picture route reads; a described picture first asks for the new picture's description or for
// the old one to be kept; the answer says how the new picture sits; a refusal says its cause.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { PDFViewer } from './stub-viewer.mjs';

const OPEN = {
  id: 'objreplace:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf', rev: 1,
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const pic = (ordinal, box, extra = {}) => ({
  page: 1, ordinal, kind: 'image', digest: 'd'.repeat(63) + ordinal, box, onPage: true,
  editable: true, why: '', movable: true, moveWhy: '', replaceable: true, replaceWhy: '', placeSpan: null, qDepth: 1,
  inForm: false, tagged: false, decoration: false, mcid: -1, stream: 0, alt: '', ...extra,
});
const listing = {
  page: 1, formsNotRead: 0,
  objects: [pic(0, [0.1, 0.1, 0.3, 0.2]), pic(1, [0.5, 0.5, 0.8, 0.7], { alt: 'A cat', tagged: true, editable: false, why: 'tagged-shared' }),
    pic(2, [0.1, 0.8, 0.2, 0.9], { editable: false, why: 'form-drawn', movable: false, moveWhy: 'form-drawn', replaceable: false, replaceWhy: 'form-drawn' }),
    pic(3, [0.6, 0.1, 0.8, 0.3], { alt: 'A bird', tagged: true })],
};
const posts = [];
let postReply = null;
let serverRev = 1;

const { document: doc, window: win, settle } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [{ ...OPEN, rev: serverRev }], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/status': { formats: [{ key: 'png', label: 'PNG image', mime: ['image/png'], image: true, available: true },
      { key: 'heic', label: 'HEIC image', mime: ['image/heic'], image: true, tool: 'heif-convert', available: false }] },
    '/api/page/objects': () => listing,
    '/api/page/objects/replace': (opts) => {
      const body = opts.body;
      posts.push({ file: body.get('file'), edits: JSON.parse(body.get('edits')) });
      if (postReply) return postReply();
      serverRev += 1;
      const e = JSON.parse(body.get('edits')).edits[0];
      return { ...OPEN, rev: serverRev, canUndo: true, removed: [], moved: [],
        replaced: [{ page: e.page, ordinal: e.ordinal, box: [0.1, 0.12, 0.3, 0.18], fitted: true, stretched: false, turned: 0, alt: e.alt || (e.keepAlt ? 'A cat' : '') }] };
    },
  },
});

let pageDiv = null;
PDFViewer.prototype.getPageView = function getPageView() {
  if (!this.nibPageDiv) {
    this.nibPageDiv = doc.createElement('div');
    for (const k of ['clientWidth', 'clientHeight']) Object.defineProperty(this.nibPageDiv, k, { configurable: true, value: 1000 });
    doc.body.appendChild(this.nibPageDiv);
    pageDiv = this.nibPageDiv;
  }
  return { div: this.nibPageDiv };
};

const $ = (id) => doc.getElementById(id);
const outlines = () => (pageDiv ? [...pageDiv.querySelectorAll('.obj-outline')] : []);
const notice = () => ($('objectNotice').hidden ? '' : $('objectNoticeText').textContent);
async function settleMore() { for (let i = 0; i < 4; i++) await settle(); }
let pickerOpened = 0;
$('objectsReplaceFile').click = () => { pickerOpened += 1; };
function pick(name) {
  const file = new win.File([new Uint8Array([137, 80, 78, 71])], name, { type: 'image/png' });
  Object.defineProperty($('objectsReplaceFile'), 'files', { configurable: true, value: [file] });
  $('objectsReplaceFile').dispatchEvent(new win.Event('change'));
}

test('setup: Replace… follows the selection, and the picker offers only pictures Nib reads itself', async () => {
  setNextDocument({ numPages: 1 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settleMore();
  $('objectsBtn').click();
  await settleMore();
  assert.equal(outlines().length, 4);
  assert.equal($('objectsReplaceBtn').disabled, true, 'Replace… is on with nothing selected');
  outlines()[2].click();
  await settleMore();
  assert.equal($('objectsReplaceBtn').disabled, true, 'Replace… is on for a picture that cannot be replaced');
  outlines()[1].click();
  await settleMore();
  assert.equal($('objectsRemoveBtn').disabled, true, 'fixture: a picture whose tag holds more content can be removed');
  assert.equal($('objectsReplaceBtn').disabled, false, 'Replace… is off for a picture a replace keeps the tag of');
  outlines()[0].click();
  await settleMore();
  assert.equal($('objectsReplaceBtn').disabled, false, 'Replace… is off for a replaceable picture');
  assert.equal($('objectsReplaceFile').accept, 'image/png', `the picker accepts "${$('objectsReplaceFile').accept}"`);
});

test('a picked file is one multipart request naming the picture, and the answer says how it sits', async () => {
  outlines()[0].click();
  await settleMore();
  const opened = pickerOpened;
  $('objectsReplaceBtn').click();
  assert.equal(pickerOpened, opened + 1, 'Replace… did not open the picker');
  pick('new.png');
  await settleMore();
  assert.equal(posts.length, 1, `one pick sent ${posts.length} requests`);
  assert.equal(posts[0].file.name, 'new.png');
  assert.deepEqual(posts[0].edits.edits, [{ kind: 'replace', object: 'picture', page: 1, ordinal: 0, digest: 'd'.repeat(63) + '0', alt: '', keepAlt: false }]);
  assert.match(notice(), /^Replaced the picture\. Undo puts the old one back\. It is fitted inside the old picture's place, keeping its proportions\.$/, `after: "${notice()}"`);
  assert.ok(outlines()[0].classList.contains('selected'), 'the replaced picture is not selected again');
});

test('a described picture asks about its description before the picker, and sends the answer', async () => {
  const sent = posts.length;
  outlines()[1].click();
  await settleMore();
  const opened = pickerOpened;
  $('objectsReplaceBtn').click();
  assert.equal(pickerOpened, opened, 'the picker opened before the description was answered');
  assert.equal($('objectsAltRow').hidden, false, 'the description row is not shown');
  assert.match(notice(), /described as "A cat"/, `says "${notice()}"`);
  $('objectsAlt').value = 'A dog';
  $('objectsAltGo').click(); // the row's own button, beside the field
  assert.equal(pickerOpened, opened + 1);
  pick('dog.png');
  await settleMore();
  assert.equal(posts.length, sent + 1);
  assert.equal(posts[sent].edits.edits[0].alt, 'A dog');
  assert.equal(posts[sent].edits.edits[0].keepAlt, false);
  assert.match(notice(), /Its description is "A dog"\./, `after: "${notice()}"`);
  assert.equal($('objectsAltRow').hidden, true, 'the description row stayed after the install');
  outlines()[1].click();
  await settleMore();
  $('objectsReplaceBtn').click();
  $('objectsKeepAlt').checked = true;
  $('objectsKeepAlt').dispatchEvent(new win.Event('change'));
  assert.equal($('objectsAlt').disabled, true, 'keeping the description left the field to type in');
  $('objectsReplaceBtn').click();
  pick('cat2.png');
  await settleMore();
  assert.equal(posts[sent + 1].edits.edits[0].keepAlt, true, 'keeping the description was not sent');
  assert.equal(posts[sent + 1].edits.edits[0].alt, '');
});

test('a stretched or turned answer says so, and a refusal says its cause', async () => {
  postReply = () => ({ ...OPEN, rev: (serverRev += 1), canUndo: true, removed: [], moved: [],
    replaced: [{ page: 1, ordinal: 0, box: [0.1, 0.1, 0.3, 0.2], fitted: false, stretched: true, turned: 0, alt: '' }] });
  outlines()[0].click();
  await settleMore();
  $('objectsReplaceBtn').click();
  pick('wide.png');
  await settleMore();
  assert.match(notice(), /It is stretched to fill the old picture's place/, `stretched: "${notice()}"`);
  postReply = () => new Response(JSON.stringify({ cause: 'image-mask', error: 'x' }), { status: 422, headers: { 'Content-Type': 'application/json' } });
  outlines()[0].click();
  await settleMore();
  $('objectsReplaceBtn').click();
  pick('mask.png');
  await settleMore();
  assert.match(notice(), /a shape painted in one colour/, `refused: "${notice()}"`);
  postReply = null;
});

test('a description typed for one picture is not carried to another', async () => {
  const sent = posts.length;
  outlines()[1].click();
  await settleMore();
  $('objectsReplaceBtn').click();
  $('objectsAlt').value = 'A dog';
  outlines()[3].click(); // another described picture
  await settleMore();
  assert.equal($('objectsAltRow').hidden, true, 'the row typed for the cat is still open on the bird');
  const opened = pickerOpened;
  $('objectsReplaceBtn').click();
  assert.equal(pickerOpened, opened, 'the bird was replaced with the cat\'s new description, unasked');
  assert.match(notice(), /described as "A bird"/, `says "${notice()}"`);
  assert.equal($('objectsAlt').value, '', 'the cat\'s description is in the bird\'s row');
  // An answer read at the press: a change to the row after the picker opened is not what is sent.
  $('objectsAlt').value = 'A crow';
  $('objectsAltGo').click();
  assert.equal(pickerOpened, opened + 1);
  $('objectsAlt').value = 'Changed after the press';
  pick('crow.png');
  await settleMore();
  assert.equal(posts.length, sent + 1);
  assert.equal(posts[sent].edits.edits[0].alt, 'A crow', `sent "${posts[sent].edits.edits[0].alt}"`);
  assert.equal(posts[sent].edits.edits[0].ordinal, 3);
});

test('a file picked after the tool was disarmed is not sent', async () => {
  const sent = posts.length;
  outlines()[0].click();
  await settleMore();
  const opened = pickerOpened;
  $('objectsReplaceBtn').click();
  assert.equal(pickerOpened, opened + 1, 'fixture: the picker did not open, so the disarm proves nothing');
  $('objectsBtn').click(); // off, before the picker answers
  pick('late.png');
  await settleMore();
  assert.equal(posts.length, sent, 'a replace was sent for a disarmed tool');
});
