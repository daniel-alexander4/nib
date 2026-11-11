// Edit Objects' move and resize (PLAN-image-edit P02.S03, ADR-221): a drag moves a movable picture and its corner
// resizes it, one request at the drop and none while dragging; Shift+arrow nudges it and a burst of nudges is one
// request; a picture that cannot move says why; a refused move puts the outline back.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { PDFViewer } from './stub-viewer.mjs';

const OPEN = {
  id: 'objmove:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf', rev: 1,
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const pic = (ordinal, box, extra = {}) => ({
  page: 1, ordinal, kind: 'image', digest: 'd'.repeat(63) + ordinal, box, onPage: true,
  editable: true, why: '', movable: true, moveWhy: '', placeSpan: null, qDepth: 1, inForm: false, tagged: false,
  decoration: false, mcid: -1, stream: 0, alt: '', ...extra,
});
const listing = {
  page: 1, formsNotRead: 0,
  objects: [pic(0, [0.1, 0.1, 0.3, 0.2]), pic(1, [0.5, 0.5, 0.8, 0.7], { alt: 'A cat' }),
    pic(2, [0.1, 0.8, 0.2, 0.9], { editable: false, why: 'form-drawn', movable: false, moveWhy: 'form-drawn' })],
};
const posts = [];
let postReply = null;
let serverRev = 1;

const { document: doc, window: win, settle } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [{ ...OPEN, rev: serverRev }], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/page/objects': (opts) => {
      if ((opts.method || 'GET') === 'GET') return listing;
      const body = JSON.parse(opts.body);
      posts.push(body);
      if (postReply) return postReply();
      serverRev += 1;
      const e = body.edits[0];
      return { ...OPEN, rev: serverRev, canUndo: true, removed: [], moved: [{ page: e.page, ordinal: e.ordinal, box: e.box, linksOver: 1 }] };
    },
  },
});

const pageDivs = [];
const viewers = [];
PDFViewer.prototype.getPageView = function getPageView() {
  if (!this.nibPageDiv) {
    this.nibPageDiv = doc.createElement('div');
    // jsdom computes no layout: a 1000 × 1000 page, so a pixel is a thousandth of the page.
    for (const k of ['clientWidth', 'clientHeight']) Object.defineProperty(this.nibPageDiv, k, { configurable: true, value: 1000 });
    doc.body.appendChild(this.nibPageDiv);
    pageDivs.push(this.nibPageDiv);
    viewers.push(this);
  }
  return { div: this.nibPageDiv };
};
const pageDiv = () => pageDivs[pageDivs.length - 1];

const $ = (id) => doc.getElementById(id);
const outlines = () => (pageDiv() ? [...pageDiv().querySelectorAll('.obj-outline')] : []);
const notice = () => ($('objectNotice').hidden ? '' : $('objectNoticeText').textContent);
const key = (el, k, mods = {}) => el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...mods }));
const ptr = (el, type, x, y, mods = {}) => el.dispatchEvent(new win.MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0, ...mods }));
const near = (got, want, what) => got.forEach((v, i) => assert.ok(Math.abs(v - want[i]) < 1e-9, `${what}: ${JSON.stringify(got)}, want ${JSON.stringify(want)}`));
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
async function settleMore() { for (let i = 0; i < 4; i++) await settle(); }

test('setup: Edit Objects is armed on a page with two movable pictures and one that is not', async () => {
  setNextDocument({ numPages: 1 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settleMore();
  $('objectsBtn').click();
  await settleMore();
  assert.equal(outlines().length, 3);
  assert.equal(outlines()[0].querySelectorAll('.obj-resize').length, 1, 'a movable picture has no resize corner');
  assert.equal(outlines()[2].querySelectorAll('.obj-resize').length, 0, 'a picture that cannot move has a resize corner');
});

test('a press that does not travel is a click: it selects, says how to move, and sends nothing', async () => {
  const o = outlines()[0];
  ptr(o, 'pointerdown', 100, 100);
  ptr(o, 'pointermove', 101, 101);
  ptr(o, 'pointerup', 101, 101);
  o.click();
  await settleMore();
  assert.equal(posts.length, 0, 'a click sent an edit');
  assert.ok(o.classList.contains('selected'), 'a click did not select');
  assert.match(notice(), /Drag it, or press Shift and an arrow key, to move it/, `selecting says "${notice()}"`);
});

test('a drag moves the picture: the outline follows, nothing is sent until the drop, then one move by address', async () => {
  const o = outlines()[0];
  ptr(o, 'pointerdown', 100, 100);
  ptr(o, 'pointermove', 200, 150);
  ptr(o, 'pointermove', 300, 200);
  assert.ok(Math.abs(parseFloat(o.style.left) - 30) < 1e-9, `the outline did not follow the drag (left ${o.style.left})`);
  assert.equal(posts.length, 0, 'a request was sent while dragging');
  ptr(o, 'pointerup', 300, 200);
  await settleMore();
  assert.equal(posts.length, 1, `one drop sent ${posts.length} requests`);
  const e = posts[0].edits[0];
  assert.deepEqual({ ...e, box: undefined }, { kind: 'move', page: 1, ordinal: 0, digest: 'd'.repeat(63) + '0', box: undefined });
  near(e.box, [0.3, 0.2, 0.5, 0.3], 'the move box');
  assert.match(notice(), /^Moved the picture\. Undo puts it back\. 1 link over where it was is still there\.$/, `after the move: "${notice()}"`);
  const again = outlines()[0];
  assert.ok(again.classList.contains('selected'), 'the moved picture is not selected again after the install');
});

test('the corner resizes, and Shift keeps the proportion', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  o.click();
  const h = o.querySelector('.obj-resize');
  ptr(h, 'pointerdown', 300, 200);
  ptr(h, 'pointermove', 400, 200, { shiftKey: true });
  ptr(h, 'pointerup', 400, 200, { shiftKey: true });
  await settleMore();
  assert.equal(posts.length, sent + 1);
  near(posts[sent].edits[0].box, [0.1, 0.1, 0.4, 0.25], 'a Shift resize');
});

test('a burst of Shift+arrow nudges is one move, sent after the pause; Control+Shift resizes', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  o.focus();
  key(o, 'ArrowRight', { shiftKey: true });
  key(o, 'ArrowRight', { shiftKey: true });
  key(o, 'ArrowDown', { shiftKey: true, ctrlKey: true });
  await settle();
  assert.equal(posts.length, sent, 'a nudge was sent before the pause');
  assert.equal(doc.activeElement, o, 'a Shift+arrow moved focus to another outline');
  await wait(700);
  await settleMore();
  assert.equal(posts.length, sent + 1, `a burst sent ${posts.length - sent} requests`);
  near(posts[sent].edits[0].box, [0.12, 0.1, 0.32, 0.21], 'the burst');
});

test('Enter sends a pending burst at once', async () => {
  const sent = posts.length;
  const o = outlines()[1];
  o.focus();
  key(o, 'ArrowLeft', { shiftKey: true });
  key(o, 'Enter');
  await settleMore();
  assert.equal(posts.length, sent + 1, 'Enter did not send the pending nudge');
  near(posts[sent].edits[0].box, [0.49, 0.5, 0.79, 0.7], 'Enter');
  assert.match(notice(), /Its description, "A cat", stays with it\./, `a described picture's move says "${notice()}"`);
});

test('a picture that cannot move says why, and a nudge or a drag on it sends nothing', async () => {
  const sent = posts.length;
  const o = outlines()[2];
  o.focus();
  key(o, 'ArrowRight', { shiftKey: true });
  await settle();
  assert.match(notice(), /^It cannot be moved: This picture is part of a repeated page element/, `says "${notice()}"`);
  ptr(o, 'pointerdown', 150, 850);
  ptr(o, 'pointermove', 400, 850);
  ptr(o, 'pointerup', 400, 850);
  await wait(700);
  await settleMore();
  assert.equal(posts.length, sent, 'an unmovable picture was sent');
  assert.ok(Math.abs(parseFloat(o.style.left) - 10) < 1e-9, 'an unmovable outline moved');
});

test('Escape during a burst sends nothing and the tool disarms', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  o.focus();
  key(o, 'ArrowRight', { shiftKey: true });
  key(o, 'Escape');
  // Armed again at once, inside the burst's pause: what Escape dropped stays dropped.
  $('objectsBtn').click();
  await wait(700);
  await settleMore();
  assert.equal(posts.length, sent, 'a burst dropped by Escape was sent');
});

test('a refused move says why and puts the outline back', async () => {
  postReply = () => new Response(JSON.stringify({ cause: 'clipped', error: 'x' }), { status: 422, headers: { 'Content-Type': 'application/json' } });
  const o = outlines()[0];
  const left = o.style.left;
  ptr(o, 'pointerdown', 100, 100);
  ptr(o, 'pointermove', 300, 300);
  ptr(o, 'pointerup', 300, 300);
  await settleMore();
  assert.match(notice(), /^This picture is drawn under a clip/, `a refusal says "${notice()}"`);
  assert.equal(outlines()[0].style.left, left, 'a refused move left the outline where it was dropped');
  postReply = null;
});

test('selecting another picture mid-burst sends the burst for the picture it was made on', async () => {
  const sent = posts.length;
  const [a, b] = outlines();
  a.focus();
  key(a, 'ArrowDown', { shiftKey: true });
  b.click();
  await settleMore();
  assert.equal(posts.length, sent + 1, 'the burst was dropped when the selection moved on');
  assert.equal(posts[sent].edits[0].ordinal, 0, 'the burst was sent for the newly selected picture');
  near(posts[sent].edits[0].box, [0.1, 0.11, 0.3, 0.21], 'the burst');
  assert.ok(outlines()[1].classList.contains('selected') && !outlines()[0].classList.contains('selected'),
    'the install selected the picture the burst was for, not the one the user picked after it');
});

test('a cancelled pointer sends nothing and puts the outline back', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  ptr(o, 'pointerdown', 100, 100);
  ptr(o, 'pointermove', 300, 300);
  o.dispatchEvent(new win.MouseEvent('pointercancel', { bubbles: true }));
  ptr(o, 'pointerup', 300, 300);
  await settleMore();
  assert.equal(posts.length, sent, 'a cancelled drag was sent');
  assert.ok(Math.abs(parseFloat(o.style.left) - 10) < 1e-9, `a cancelled drag left the outline at ${o.style.left}`);
});

test('a Shift resize past the least keeps the least and the proportion', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  o.click();
  const h = o.querySelector('.obj-resize');
  ptr(h, 'pointerdown', 300, 200);
  ptr(h, 'pointermove', 0, 200, { shiftKey: true });
  ptr(h, 'pointerup', 0, 200, { shiftKey: true });
  await settleMore();
  assert.equal(posts.length, sent + 1);
  near(posts[sent].edits[0].box, [0.1, 0.1, 0.12, 0.11], 'a Shift resize to nothing');
});

test('a re-render that takes the outline mid-drag ends the drag, and the next render lays the outlines again', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  ptr(o, 'pointerdown', 100, 100);
  ptr(o, 'pointermove', 300, 300);
  outlines().forEach((x) => x.remove()); // pdf.js' PageView.reset takes every node of the page div that is not its own layer
  o.dispatchEvent(new win.MouseEvent('lostpointercapture', { bubbles: true }));
  // The render that follows, as pdf.js says it (not a disarm and arm, which would clear the gesture on its own).
  viewers[viewers.length - 1].eventBus.dispatch('pagerendered', { pageNumber: 1 });
  await settleMore();
  assert.equal(outlines().length, 3, `after a lost drag the page lays ${outlines().length} outlines`);
  assert.equal(posts.length, sent, 'a lost drag was sent');
});

test('a drag brought back to where it started, or a burst that undoes itself, sends nothing', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  ptr(o, 'pointerdown', 150, 150);
  ptr(o, 'pointermove', 250, 150);
  ptr(o, 'pointermove', 150, 150);
  ptr(o, 'pointerup', 150, 150);
  o.focus();
  key(o, 'ArrowRight', { shiftKey: true });
  key(o, 'ArrowLeft', { shiftKey: true });
  await wait(700);
  await settleMore();
  assert.equal(posts.length, sent, 'a move to where the picture already is was sent');
});

test('a drag after a burst starts where the nudges left the picture, and is one request', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  o.focus();
  key(o, 'ArrowRight', { shiftKey: true });
  ptr(o, 'pointerdown', 100, 100);
  ptr(o, 'pointermove', 200, 100);
  ptr(o, 'pointerup', 200, 100);
  await wait(700);
  await settleMore();
  assert.equal(posts.length, sent + 1, `a burst then a drag sent ${posts.length - sent} requests`);
  near(posts[sent].edits[0].box, [0.21, 0.1, 0.41, 0.2], 'the drag after a burst');
});

test('an edit while another is in flight says so, and is not sent', async () => {
  let release;
  const held = new Promise((r) => { release = r; });
  postReply = () => held.then(() => new Response(JSON.stringify({ cause: 'clipped', error: 'x' }), { status: 422, headers: { 'Content-Type': 'application/json' } }));
  const sent = posts.length;
  const [a, b] = outlines();
  ptr(a, 'pointerdown', 150, 150);
  ptr(a, 'pointermove', 250, 150);
  ptr(a, 'pointerup', 250, 150);
  await settle();
  b.click();
  key(b, 'Delete');
  await settle();
  assert.match(notice(), /^Nib is still changing the last picture/, `a second edit in flight says "${notice()}"`);
  release();
  await settleMore();
  assert.equal(posts.length, sent + 1, `two edits sent ${posts.length - sent} requests`);
  postReply = null;
});

test('a burst pending when another document comes to the front is dropped, not sent', async () => {
  const sent = posts.length;
  const o = outlines()[0];
  o.focus();
  key(o, 'ArrowRight', { shiftKey: true });
  setNextDocument({ numPages: 1 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await wait(700);
  await settleMore();
  assert.equal(posts.length, sent, 'a burst was sent after its document went behind another');
});
