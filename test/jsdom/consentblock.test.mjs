// P02.S03 — the signer sees WHERE their signature will land, on the screen they decide from.
//
// **The defect this drives.** The consent screen rendered every page of the received document and
// said nothing about the block. `handleSessionQuote`'s rect is `p2p.NominalBlockRect()`, whose own
// doc calls it *"a size template, not a placement — the caller wants a rect of the right shape and
// must not care where it says it is"*, and app.js consumed only its width and height. The real
// placement was computed server-side after the user had already consented.
//
// **What only this tier can see: the FLIP.** A PDF rect is in points with the origin at the bottom
// left; a canvas measures from the top left. Getting that wrong does not throw and does not fail a
// Go test — it draws a box, correctly sized, in the wrong half of the page, on the screen where a
// signer is being told where their signature goes. The Go side asserts the numbers are the ones
// that will be stamped; nothing there can see what they become on screen.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
// A block low on page **2**, and both halves of that are deliberate.
//
// `lly: 40` in PDF points is near the BOTTOM of a 792pt page, which is what makes the flip
// observable: unflipped it renders near the top.
//
// **Page 2 rather than page 1 because a one-page fixture cannot see the page match at all** —
// measured: with `numPages: 1`, dropping `block.page === i` from the draw condition left this file
// green, because the condition it removed was always true. A two-page document makes the wrong
// answer land somewhere a test can point at.
const BLOCK = { page: 2, rect: [40, 40, 320, 124] };

let armed = false;
let withBlock = true;
const h = await boot({
  routes: {
    '/api/peers': () => ({ self: 'f'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => (armed
      ? {
        armed: true,
        address: '127.0.0.1:8443',
        pending: {
          signer: 'Ada Landlord',
          fingerprint: 'a'.repeat(64),
          reason: 'I agree to co-sign the lease',
          signers: [],
          // Absent, not empty, when the server could not compute one — the field is `omitempty`.
          ...(withBlock ? { block: BLOCK } : {}),
        },
      }
      : { armed: false }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

async function reachConsent() {
  // `renders: true` is required here and nowhere else: the consent preview's failure path replaces
  // the whole column with "could not render the document", so with the default stub the geometry
  // under test is wiped before it can be read. See stub-pdfjs.mjs.
  setNextDocument({ numPages: 2, renders: true });
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  await new Promise((r) => setTimeout(r, 1800));
  await settle();
}

async function cleanUp() {
  doc.getElementById('srvCancel').click();
  await settle();
}

test('the block is drawn on its own page, measured from the bottom of the page', async () => {
  withBlock = true;
  await reachConsent();
  const onConsent = !doc.getElementById('srvConsent').hidden;
  const pages = doc.querySelectorAll('#srvPreview .srvpage');
  const box = doc.querySelector('#srvPreview .srvblock');
  const geom = box ? { ...box.style } && {
    left: box.style.left, top: box.style.top, width: box.style.width, height: box.style.height,
  } : null;
  const onItsOwnPage = box ? box.parentElement === pages[1] : false;
  await cleanUp();

  // SETUP: without these the assertions below are satisfied by a screen that never opened or a
  // preview that rendered nothing.
  assert.ok(onConsent, 'the consent screen was never reached, so nothing below proves anything');
  assert.equal(pages.length, 2, 'the preview did not render both pages, so "the block is on '
    + 'page 2 and not page 1" is not a distinction this run could have drawn');
  assert.ok(box, 'the consent screen shows the document and says nothing about where this '
    + 'signature will land — the signer decides blind');
  assert.ok(onItsOwnPage, 'the block was drawn on the wrong page, or outside a page wrapper '
    + 'altogether — on the first it tells the signer their signature lands somewhere it does not, '
    + 'and on the second it is positioned against the scrolling column and slides off on the '
    + 'first scroll');

  // The scale: the preview fits pages to 380px, so 380/612 for a US-Letter page.
  const scale = 380 / 612;
  const pageHeight = 792 * scale;
  const top = parseFloat(geom.top);
  const left = parseFloat(geom.left);

  // THE FLIP. lly=40 sits near the bottom of the page, so the box's CSS top must be in the LOWER
  // half. Unflipped (`top = lly * scale`) it lands at ~25px, near the top — correctly sized, and
  // in the wrong place.
  assert.ok(top > pageHeight / 2,
    `the block is drawn ${top}px from the top of a ${pageHeight}px page, which is the upper half `
    + '— a PDF rect measures from the BOTTOM left and a canvas from the top left, so this is the '
    + 'flip being missed rather than the rect being wrong');
  assert.ok(Math.abs(top - (pageHeight - 124 * scale)) < 1,
    `the block's top is ${top}px, want about ${pageHeight - 124 * scale}px`);
  assert.ok(Math.abs(left - 40 * scale) < 1, `the block's left is ${left}px, want ${40 * scale}px`);
  assert.ok(Math.abs(parseFloat(geom.width) - 280 * scale) < 1,
    `the block is ${geom.width} wide, want ${280 * scale}px — the size is what the appearance `
    + 'image is rasterised to, so a box of the wrong size misreports what will be stamped');
  assert.ok(Math.abs(parseFloat(geom.height) - 84 * scale) < 1,
    `the block is ${geom.height} tall, want ${84 * scale}px`);
});

test('a document with no computable placement still shows its pages', async () => {
  withBlock = false;
  await reachConsent();
  const onConsent = !doc.getElementById('srvConsent').hidden;
  const pages = doc.querySelectorAll('#srvPreview .srvpage');
  const box = doc.querySelector('#srvPreview .srvblock');
  await cleanUp();

  assert.ok(onConsent, 'the consent screen was never reached, so nothing below proves anything');
  assert.equal(pages.length, 2, 'the preview stopped rendering pages when there was no block — '
    + 'the block is an annotation ON the review and must not be able to take the review with it');
  assert.equal(box, null, 'a box was drawn for a document whose placement the server could not '
    + 'compute, so the signer is being shown a location Nib does not have');
});
