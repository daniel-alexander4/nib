// Page Functions → Edit Objects (PLAN-image-edit P01.S04, ADR-219): the outlines, the one Tab stop, selection and
// refusal said in the polite region, the cache that keeps arrows and re-renders off the server, and the Delete key
// that removes the focused picture and nothing else.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { PDFViewer } from './stub-viewer.mjs';

const OPEN = {
  id: 'objects:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf', rev: 1,
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const pic = (ordinal, box, extra = {}) => ({
  page: 1, ordinal, kind: 'image', digest: 'd'.repeat(63) + ordinal, box, onPage: true,
  editable: true, why: '', placeSpan: null, qDepth: 1, inForm: false, tagged: false, decoration: false,
  mcid: -1, stream: 0, alt: '', ...extra,
});
let listing = {
  page: 1, formsNotRead: 0,
  objects: [pic(0, [0.1, 0.1, 0.3, 0.2], { replaceable: false, replaceWhy: 'image-mask' }), pic(1, [0.5, 0.5, 0.8, 0.7], { alt: 'A cat' }), pic(2, [0.1, 0.8, 0.2, 0.9], { editable: false, why: 'form-drawn' })],
};
const gets = [];
const posts = [];
let postReply = null;
// The revision the server holds: a successful removal moves it, so `/api/docs` agrees with what the window loaded
// unless a test says otherwise (reloadIfMoved reads it on a 409).
let serverRev = 1;

const { document: doc, settle } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [{ ...OPEN, rev: serverRev }], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/page/objects': (opts) => {
      if ((opts.method || 'GET') === 'GET') {
        gets.push(opts);
        return listing;
      }
      posts.push(JSON.parse(opts.body));
      if (postReply) return postReply();
      serverRev = 2;
      return {
        ...OPEN, rev: 2, canUndo: true,
        removed: [{ page: 1, ordinal: 1, kind: 'image', elementRemoved: true, alt: 'A cat', linksOver: 0, sharedWith: [], staysInFile: false }],
      };
    },
  },
});

// The stub viewer draws no pages; this file gives each viewer one page div of its own (tagtree.test.mjs gives one test
// one), because each open document has its own pages and its own outlines — a second document is a second tab.
const pageDivs = [];
PDFViewer.prototype.getPageView = function getPageView() {
  if (!this.nibPageDiv) {
    this.nibPageDiv = doc.createElement('div');
    doc.body.appendChild(this.nibPageDiv);
    pageDivs.push(this.nibPageDiv);
  }
  return { div: this.nibPageDiv };
};
// The newest viewer is the document in front.
const pageDiv = () => pageDivs[pageDivs.length - 1];

const $ = (id) => doc.getElementById(id);
const outlines = () => (pageDiv() ? [...pageDiv().querySelectorAll('.obj-outline')] : []);
const notice = () => ($('objectNotice').hidden ? '' : $('objectNoticeText').textContent);
const key = (el, k) => el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }));
async function settleMore() { for (let i = 0; i < 4; i++) await settle(); }

test('arming lays one outline per picture, one Tab stop, and reads the page once', async () => {
  setNextDocument({ numPages: 1 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settleMore();
  doc.querySelector('.modetab[data-tab="edit"]').click();
  await settle();
  assert.equal($('objectsRemoveBtn').disabled, true, 'Remove from page is enabled with nothing selected');
  $('objectsBtn').click();
  await settleMore();
  const o = outlines();
  assert.equal(o.length, 3, `armed, the page shows ${o.length} outlines, want 3`);
  assert.equal($('objectsBtn').getAttribute('aria-pressed'), 'true', 'the armed button does not say it is pressed');
  assert.deepEqual(o.map((b) => b.tabIndex), [0, -1, -1], 'the outlines are not one Tab stop');
  assert.equal(o[1].getAttribute('aria-label'), 'Picture 2 of 3, page 1: A cat', 'an outline is not named by place and description');
  assert.equal(o[1].style.left, '50%', 'the outline is not placed from the displayed-page fractions');
  assert.equal(gets.length, 1, 'arming read the page more than once');
  assert.match(gets[0].headers['X-Nib-Doc'] || '', /objects:1/, 'the read is not pinned to the document');
});

test('arrows move inside the group without asking the server, and Enter selects and says so', async () => {
  const before = gets.length;
  const o = outlines();
  o[0].focus();
  key(o[0], 'ArrowRight');
  assert.equal(doc.activeElement, o[1], 'ArrowRight did not move to the next picture');
  assert.deepEqual(o.map((b) => b.tabIndex), [-1, 0, -1], 'the Tab stop did not move with focus');
  key(o[1], 'Enter');
  await settle();
  assert.equal(o[1].getAttribute('aria-pressed'), 'true', 'the selected outline does not say it is selected');
  assert.match(notice(), /^Picture 2 of 3, page 1: A cat, selected\. Remove from page takes it off the page\. Its description goes with it\.$/,
    `the region says "${notice()}"`);
  assert.equal($('objectsRemoveBtn').disabled, false, 'Remove from page is off with a removable picture selected');
  assert.equal(gets.length, before, 'moving and selecting asked the server again');
});

test('a picture that cannot be replaced says why when selected', async () => {
  outlines()[0].click();
  await settle();
  assert.match(notice(), /It cannot be replaced: This is a shape painted in one colour/, `the region says "${notice()}"`);
  assert.equal($('objectsReplaceBtn').disabled, true, 'Replace… is on for a picture that cannot be replaced');
});

test('a refused picture says why and cannot be removed', async () => {
  const o = outlines();
  o[2].click();
  await settle();
  assert.match(notice(), /repeated page element, such as a letterhead/, `the refusal says "${notice()}"`);
  assert.equal($('objectsRemoveBtn').disabled, true, 'Remove from page is on for a picture the door refuses');
  key(o[2], 'Delete');
  await settleMore();
  assert.equal(posts.length, 0, 'Delete on a refused picture sent a removal');
});

test('Delete away from an outline removes nothing', async () => {
  const other = $('objectsBtn');
  other.focus();
  key(other, 'Delete');
  key(doc.body, 'Delete');
  await settleMore();
  assert.equal(posts.length, 0, 'a Delete that was not on an outline sent a removal');
});

test('Delete on the focused picture removes exactly it, by address, and says what went', async () => {
  setNextDocument({ numPages: 1 });
  const o = outlines();
  o[1].focus();
  key(o[1], 'Delete');
  await settleMore();
  assert.equal(posts.length, 1, `Delete sent ${posts.length} removals, want 1`);
  assert.deepEqual(posts[0], { edits: [{ kind: 'delete', object: 'picture', page: 1, ordinal: 1, digest: 'd'.repeat(63) + '1' }] },
    'the removal does not name the focused picture by its address');
  assert.match(notice(), /^Removed the picture from the page\. Undo brings it back until you close this document\. Its description, "A cat", went with it\.$/,
    `after the removal the region says "${notice()}"`);
  assert.ok(doc.activeElement && doc.activeElement.classList.contains('obj-outline'),
    'after Delete the keyboard user is dropped out of the group: focus is not on an outline');
  assert.ok(gets.length >= 2, 'the new revision the removal installed was not read: the outlines are the old page\'s');
});

test('when the history was let go, the words do not promise Undo', async () => {
  postReply = () => (serverRev = 3, { ...OPEN, rev: 3, canUndo: false, historyEvicted: true,
    removed: [{ page: 1, ordinal: 0, kind: 'image', elementRemoved: false, alt: '', linksOver: 2, sharedWith: [2], staysInFile: true }] });
  setNextDocument({ numPages: 1 });
  const o = outlines();
  o[0].focus();
  key(o[0], 'Delete');
  await settleMore();
  assert.match(notice(), /^Removed the picture from the page\. Undo cannot bring it back/, `the region says "${notice()}"`);
  assert.match(notice(), /2 links over it are still on the page\./, 'the links kept over the picture are not said');
  assert.match(notice(), /still in the file, because page 2 still uses it;/, 'a picture that stays in the file is not said to, or not where');
  postReply = null;
});

test('the same revision is not read twice; a stale address reads the page again and says so', async () => {
  const before = gets.length;
  $('objectsBtn').click(); // off
  $('objectsBtn').click(); // on, at the revision the last removal installed — read when it was installed
  await settleMore();
  assert.equal(gets.length, before, 'arming again at the same revision asked the server again');
  listing = { page: 1, formsNotRead: 0, objects: [pic(0, [0.1, 0.1, 0.3, 0.2])] };
  postReply = () => new Response(JSON.stringify({ cause: 'object-changed', error: 'x' }), { status: 409, headers: { 'Content-Type': 'application/json' } });
  const o = outlines();
  o[0].focus();
  const asked = gets.length;
  key(o[0], 'Delete');
  await settleMore();
  assert.equal(notice(), 'The page changed since you selected this. Select it again.', `a stale address says "${notice()}"`);
  // The stimulus: the server answered 409 and its revision did not move, so only a forgotten listing reads again.
  assert.equal(gets.length, asked + 1, 'a stale address did not read the page again');
  assert.equal(outlines().length, 1, 'the outlines were not laid from the page read again');
});

test('a second Delete while a removal is in flight is not sent', async () => {
  let release;
  const held = new Promise((r) => { release = r; });
  postReply = () => held.then(() => new Response(JSON.stringify({ cause: 'object-changed', error: 'x' }), { status: 409, headers: { 'Content-Type': 'application/json' } }));
  const o = outlines();
  const sent = posts.length;
  o[0].focus();
  key(o[0], 'Delete');
  key(o[0], 'Delete');
  await settle();
  release();
  await settleMore();
  assert.equal(posts.length, sent + 1, `two presses sent ${posts.length - sent} removals`);
  postReply = null;
});

test('a selection does not outlive its document: opening another one drops it and Remove from page', async () => {
  postReply = null;
  listing = { page: 1, formsNotRead: 0, objects: [pic(0, [0.1, 0.1, 0.3, 0.2])] };
  if ($('objectsBtn').getAttribute('aria-pressed') !== 'true') $('objectsBtn').click();
  await settleMore();
  outlines()[0].click();
  await settle();
  assert.equal($('objectsRemoveBtn').disabled, false, 'setup: nothing is selected');
  setNextDocument({ numPages: 1 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settleMore();
  assert.equal($('objectsRemoveBtn').disabled, true, 'Remove from page stayed on for a picture of the document that went');
  const sent = posts.length;
  $('objectsRemoveBtn').click();
  await settleMore();
  assert.equal(posts.length, sent, 'a selection from the previous document was sent for the new one');
  if ($('objectsBtn').getAttribute('aria-pressed') !== 'true') $('objectsBtn').click();
  await settleMore();
});

test('Escape on an outline disarms the tool and clears the outlines', async () => {
  const o = outlines();
  assert.ok(o.length > 0, 'setup: no outlines to press Escape on');
  key(o[0], 'Escape');
  await settle();
  assert.equal(outlines().length, 0, 'Escape left the outlines on the page');
  assert.equal($('objectsBtn').getAttribute('aria-pressed'), 'false', 'Escape left the tool armed');
});
