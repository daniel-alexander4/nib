// Edit Objects with drawings (PLAN-image-edit P03.S03): a page's drawings are outlined beside its pictures, numbered
// on their own (picture 0 and drawing 0 are two objects), named as drawings, moved but never resized, and every edit
// names which list its ordinal counts in.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { PDFViewer } from './stub-viewer.mjs';

const OPEN = {
  id: 'drawings:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf', rev: 1,
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const entry = (object, ordinal, kind, box, extra = {}) => ({
  page: 1, object, ordinal, kind, digest: (object === 'drawing' ? 'e' : 'd').repeat(63) + ordinal, box, onPage: true,
  editable: true, why: '', movable: true, moveWhy: '', replaceable: object !== 'drawing',
  replaceWhy: object === 'drawing' ? 'not-a-picture' : '', placeSpan: null, qDepth: 1, inForm: false, tagged: false,
  decoration: false, mcid: -1, stream: 0, alt: '', ...extra,
});
const listing = {
  page: 1, formsNotRead: 0, drawingsUngrouped: false,
  // A picture over most of the page, a chart drawn on it, and a tagged rule.
  objects: [entry('picture', 0, 'image', [0.05, 0.05, 0.95, 0.95])],
  drawings: [
    entry('drawing', 0, 'drawing', [0.2, 0.2, 0.4, 0.4]),
    // Listed replaceable on purpose: the window must not offer Replace… for a drawing whatever the flag says.
    entry('drawing', 1, 'rule', [0.1, 0.9, 0.6, 0.901], { editable: false, why: 'tagged', tagged: true, replaceable: true }),
  ],
};
const posts = [];
let serverRev = 1;

const { document: doc, settle } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [{ ...OPEN, rev: serverRev }], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/page/objects': (opts) => {
      if ((opts.method || 'GET') === 'GET') return listing;
      const body = JSON.parse(opts.body);
      posts.push(body);
      serverRev++;
      const e = body.edits[0];
      return e.kind === 'move'
        ? { ...OPEN, rev: serverRev, canUndo: true, moved: [{ page: 1, object: e.object, ordinal: e.ordinal, box: e.box, linksOver: 0 }] }
        : { ...OPEN, rev: serverRev, canUndo: true,
          removed: [{ page: 1, object: e.object, ordinal: e.ordinal, kind: 'drawing', elementRemoved: false, alt: '', linksOver: 0, sharedWith: [], staysInFile: false }] };
    },
  },
});

let pageDiv = null;
PDFViewer.prototype.getPageView = function getPageView() {
  if (!this.nibPageDiv) {
    this.nibPageDiv = doc.createElement('div');
    doc.body.appendChild(this.nibPageDiv);
    pageDiv = this.nibPageDiv;
  }
  return { div: this.nibPageDiv };
};

const $ = (id) => doc.getElementById(id);
const outlines = () => (pageDiv ? [...pageDiv.querySelectorAll('.obj-outline')] : []);
const notice = () => ($('objectNotice').hidden ? '' : $('objectNoticeText').textContent);
const key = (el, k, mods = {}) => el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...mods }));
async function settleMore() { for (let i = 0; i < 4; i++) await settle(); }

test('drawings are outlined beside the pictures, named and numbered on their own, a smaller one above a larger', async () => {
  setNextDocument({ numPages: 1 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settleMore();
  doc.querySelector('.modetab[data-tab="edit"]').click();
  await settle();
  $('objectsBtn').click();
  await settleMore();
  const o = outlines();
  assert.equal(o.length, 3, `armed, the page shows ${o.length} outlines, want the picture and two drawings`);
  assert.deepEqual(o.map((b) => b.getAttribute('aria-label')),
    ['Picture 1 of 1, page 1', 'Drawing 1 of 2, page 1', 'Drawing 2 of 2 (a line), page 1'], 'the outlines are not named by their own lists');
  assert.deepEqual(o.map((b) => b.dataset.object), ['picture', 'drawing', 'drawing'], 'an outline does not say which list it is in');
  assert.equal(o[1].querySelector('.obj-resize'), null, 'a drawing has a resize corner: a drawing is moved, never resized');
  assert.ok(o[0].querySelector('.obj-resize'), 'the movable picture lost its resize corner');
  assert.deepEqual(o.map((b) => b.classList.contains('obj-holds')), [true, false, false],
    'the picture the chart is drawn on does not lie under it, so the chart cannot be pressed');
  assert.ok(o.every((b) => !b.style.zIndex), 'an outline climbs over the page\'s marks or the notices with a z-index of its own');
});

test('selecting drawing 0 does not select picture 0, and says what a drawing can do', async () => {
  const o = outlines();
  o[1].click();
  await settle();
  assert.deepEqual(o.map((b) => b.getAttribute('aria-pressed')), ['false', 'true', 'false'], 'picture 0 and drawing 0 are not two objects');
  assert.equal(notice(), 'Drawing 1 of 2, page 1, selected. Remove from page takes it off the page. Drag it, or press Shift and an arrow key, to move it.',
    `the region says "${notice()}"`);
  assert.equal($('objectsReplaceBtn').disabled, true, 'Replace… is on for a drawing');
});

test('Control, Shift and an arrow does not resize a drawing, and sends nothing', async () => {
  const o = outlines();
  o[1].focus();
  key(o[1], 'ArrowRight', { shiftKey: true, ctrlKey: true });
  await settleMore();
  assert.equal(notice(), 'A drawing is moved, not resized.', `the region says "${notice()}"`);
  await new Promise((r) => setTimeout(r, 700)); // past a burst's wait: nothing was held to be sent later
  assert.equal(posts.length, 0, 'a resize of a drawing was sent');
});

test('a nudge moves the drawing, naming it as a drawing', async () => {
  setNextDocument({ numPages: 1 });
  const o = outlines();
  o[1].focus();
  key(o[1], 'ArrowRight', { shiftKey: true });
  key(o[1], 'Enter');
  await settleMore();
  assert.equal(posts.length, 1, `the burst sent ${posts.length} edits, want one`);
  const e = posts[0].edits[0];
  assert.deepEqual({ ...e, box: undefined }, { kind: 'move', object: 'drawing', page: 1, ordinal: 0, digest: 'e'.repeat(63) + '0', box: undefined },
    'the move does not name drawing 0');
  assert.ok(e.box[0] > 0.2 && Math.abs((e.box[2] - e.box[0]) - 0.2) < 1e-9, `the box ${e.box} is not the drawing moved right at its own size`);
  assert.match(notice(), /^Moved the drawing\. Undo puts it back\./, `the region says "${notice()}"`);
  const after = outlines();
  assert.deepEqual(after.map((b) => b.getAttribute('aria-pressed')), ['false', 'true', 'false'],
    'after the move the drawing is not selected again (picture 0 shares its ordinal)');
});

test('Delete on a drawing removes exactly it, by its own address, and says a drawing went', async () => {
  setNextDocument({ numPages: 1 });
  const o = outlines();
  o[1].focus();
  key(o[1], 'Delete');
  await settleMore();
  assert.equal(posts.length, 2, `Delete sent ${posts.length - 1} removals, want 1`);
  assert.deepEqual(posts[1], { edits: [{ kind: 'delete', object: 'drawing', page: 1, ordinal: 0, digest: 'e'.repeat(63) + '0' }] },
    'the removal does not name drawing 0');
  assert.match(notice(), /^Removed the drawing from the page\./, `the region says "${notice()}"`);
});

test('a tagged drawing says why it cannot be removed, and is never offered Replace…', async () => {
  const o = outlines();
  o[2].click();
  await settle();
  assert.equal($('objectsReplaceBtn').disabled, true, 'Replace… is on for a drawing the listing calls replaceable');
  assert.match(notice(), /It cannot be removed: This is tagged in a way Nib cannot take apart safely/, `the region says "${notice()}"`);
  key(o[2], 'Delete');
  await settleMore();
  assert.equal(posts.length, 2, 'Delete on a refused drawing sent a removal');
});
