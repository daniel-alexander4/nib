// M3's "placement matches baked output exactly" — the half the ledger left human.
//
// A placed stamp is a Nib overlay widget: an absolutely-positioned element inside the
// page div, carried as a FRACTION of that div, and turned into PDF points at bake time
// by `rectPoints` (app.js) — `[fx0*W, (1-fy1)*H, fx1*W, (1-fy0)*H]`. That expression is
// the whole seam. A screen y grows downward from the top; a PDF y grows upward from the
// bottom; and the one place the two meet is that `1 -`.
//
// pdf.js's own STAMP editor was dropped for this exact reason — its saveDocument() baked
// placed stamps a few points high, so the mark no longer sat on the line it was put on
// (app.js, "image library + stamping"). The replacement has never been driven end to end.
//
// ── What this asserts, and why it needs tier 3 ──────────────────────────────
// Place the red APPROVED quick-stamp, drag it into the LOWER half of page one, save, and
// read the saved file back — then open it and look at the pixels pdf.js renders. The
// stamp is `#c1121f`; the fixture's text is black; so "is there red ink, and where" is a
// question about the baked document that the app cannot answer on its own behalf.
//
// Every step of that is layout and canvas. jsdom has neither: `clientWidth` is 0 there,
// so the fraction a drag produces is NaN before it is ever wrong, and there are no
// rendered pixels to read afterwards. Tier 1 owns the other end — pdfops.StampImages puts
// bytes where it is told — and the gap between them is precisely what nobody had crossed:
// whether what the user SEES on the page is what the server is TOLD to stamp.
//
// The genuinely human residue is unchanged and stays on the ledger: how the drag and the
// resize FEEL. Where the mark lands is not a matter of feel.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('stamp-place.pdf', { pages: 1, label: 'stamp page' });
const OUT_DIR = path.join(WORK, 'stamped');
fs.mkdirSync(OUT_DIR, { recursive: true });

const h = await launch();
const { page } = h;

after(async () => {
  try {
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

// canvasPainted waits for pdf.js to have actually painted the page.
//
// `openDocument` waits for the page DIV and the page count; the CANVAS inside it arrives
// later, on pdf.js's own render loop. Reading pixels before it exists gave
// `redInk()` null and fired this test's setup assertion — "no page canvas was rendered"
// — in roughly one run in six. That assertion did its job: it refused to grade the
// response because the stimulus had not happened, which is the whole reason it is
// written as a setup check rather than left to fail later as a confusing zero.
//
// It was already waited for after the REOPEN and not before the first read, which is why
// the failure looked intermittent rather than constant: the control read is the one that
// races.
function canvasPainted() {
  return page.waitForFunction(() => {
    const cv = document.querySelector('.viewerContainer:not([hidden]) .page canvas');
    return cv && cv.width > 0 && cv.height > 0;
  }, null, { timeout: 20000 });
}

// redInk scans the visible view's first page canvas and returns the bounding box and
// centroid of every red pixel, as fractions of the canvas — with (0,0) at the TOP left,
// the same origin the overlay's own fraction uses, so the two are directly comparable.
//
// It reads the canvas rather than the PDF's bytes deliberately. The question is where the
// mark ENDS UP for a reader of the document, and a content-stream matrix answers a
// different, easier question — one that a wrong-but-self-consistent transform would pass.
function redInk() {
  return page.evaluate(() => {
    const cv = document.querySelector('.viewerContainer:not([hidden]) .page canvas');
    if (!cv) return null;
    const { width: w, height: hgt } = cv;
    const d = cv.getContext('2d').getImageData(0, 0, w, hgt).data;
    let n = 0, sx = 0, sy = 0, x0 = 1, y0 = 1, x1 = 0, y1 = 0;
    for (let i = 0; i < d.length; i += 4) {
      // #c1121f against black text and white paper: red high, green and blue low.
      if (!(d[i] > 120 && d[i + 1] < 90 && d[i + 2] < 90)) continue;
      const p = i / 4, x = (p % w) / w, y = Math.floor(p / w) / hgt;
      n++; sx += x; sy += y;
      if (x < x0) x0 = x; if (x > x1) x1 = x;
      if (y < y0) y0 = y; if (y > y1) y1 = y;
    }
    return n ? { n, cx: sx / n, cy: sy / n, x0, y0, x1, y1 } : { n: 0 };
  });
}

test('a stamp bakes where it was placed, not mirrored up the page', async () => {
  await h.openDocument(DOC, 1);
  await h.topOfDocument();
  await canvasPainted();

  // The control, and it is what makes every red pixel below mean something: this
  // document has no red ink in it before the stamp is placed. Without this line the
  // assertions after the bake could be measuring the fixture.
  const before = await redInk();
  assert.ok(before, 'setup: no page canvas was rendered, so there are no pixels to read');
  assert.equal(before.n, 0,
    `setup: the fixture already carries ${before.n} red pixels before anything was stamped — the ink this test measures is not its own`);

  await h.mode('markup');
  await page.click('.sbhead[data-panel="library"]');
  await page.click('.quickstamps button[data-stamp="approved"]');
  await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-stamp');

  // placeStamp centres the widget. Drag it into the LOWER-RIGHT quadrant: a Y-flip
  // defect sends a stamp placed low on the screen HIGH into the page, and centre is the
  // one position where that is invisible.
  const box = await page.locator('.viewerContainer:not([hidden]) .page').first().boundingBox();
  const stamp = await page.locator('.viewerContainer:not([hidden]) .ovl-stamp').first().boundingBox();
  const targetX = box.x + box.width * 0.62;
  const targetY = box.y + box.height * 0.74;
  await page.mouse.move(stamp.x + stamp.width / 2, stamp.y + stamp.height / 2);
  await page.mouse.down();
  await page.mouse.move(targetX, targetY, { steps: 10 });
  await page.mouse.up();

  // Where the widget actually sits, as a fraction of the page div — the number the app
  // is about to convert. Read from the DOM rather than assumed from the drag, because a
  // drag that did not take would otherwise be compared against where it meant to go.
  const placed = await page.evaluate(() => {
    const p = document.querySelector('.viewerContainer:not([hidden]) .page');
    const s = document.querySelector('.viewerContainer:not([hidden]) .ovl-stamp');
    const pr = p.getBoundingClientRect(), sr = s.getBoundingClientRect();
    return {
      cx: (sr.x + sr.width / 2 - pr.x) / pr.width,
      cy: (sr.y + sr.height / 2 - pr.y) / pr.height,
    };
  });
  assert.ok(placed.cy > 0.55,
    `setup: after the drag the stamp's centre is at ${placed.cy.toFixed(3)} down the page, still in the top half — the drag did not take, and a mirrored bake would land in the same half it started in`);

  // "Save editable copy" rather than Save, so the fixture survives for a re-read and the
  // assertion below is about a file this test can name. It is also the path that puts
  // the overlay through `collectStamps` → `/api/bake` → `pdfops.StampImages`, which is
  // the seam under test; the flattened path rasterises the page instead and would prove
  // something else. The item lives in a dropdown that only exists while its menu is
  // open, so the menu is clicked first — Playwright refuses a display:none target, which
  // is the UI's own rule showing through.
  await h.mode('file');
  await page.click('.menu:has(#saveEditableBtn) .menutop');
  await page.click('#saveEditableBtn');
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  await page.fill('#saveAsName', 'stamped.pdf');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');
  await page.waitForFunction(() => document.getElementById('saveAsModal').hidden);

  const out = path.join(OUT_DIR, 'stamped.pdf');
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(out) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.existsSync(out),
    `nothing was written to ${out} — the bake round-trip finished and the dialog closed, so the app believes it saved a stamped document that is not there`);

  // Read the BAKED document back, which is the only copy whose pixels are the server's
  // work rather than the overlay still sitting on top of the old one.
  //
  // The original is CLOSED first, and that is not tidiness. `openDocument` waits for the
  // page count to reach N — and both documents have one page, so with the first still
  // open that wait is ALREADY TRUE and the helper would return before the new view
  // existed, leaving every assertion below reading the old document's canvas while
  // looking like it read the new one. A state check where a transition check is needed:
  // the same shape that cost this repo four sessions on the zoom flake.
  await h.closeDocument();
  await page.waitForFunction(() => document.getElementById('viewerWrap').className !== 'has-doc');
  await h.openDocument(out, 1);
  await h.topOfDocument();
  await canvasPainted();

  const baked = await redInk();
  assert.ok(baked && baked.n > 0,
    'the saved document carries no red ink at all — the stamp was never baked, so nothing below is measuring placement');

  // The property, in the axis that can be silently wrong. A stamp placed at 0.74 down
  // the page and baked with the Y flip dropped renders at 0.26 — the right size, the
  // right colour, the right page, and the wrong end of the document.
  assert.ok(Math.abs(baked.cy - placed.cy) < 0.06,
    `the baked stamp's centre is ${baked.cy.toFixed(3)} down the page but it was placed at ${placed.cy.toFixed(3)} — off by ${(Math.abs(baked.cy - placed.cy) * 100).toFixed(1)}% of the page height. ${baked.cy < 0.5 && placed.cy > 0.5 ? 'It is in the OPPOSITE half: the screen-to-PDF y flip in rectPoints is wrong, and every stamp, border, checkbox and circled choice bakes at the wrong end of the page.' : 'The mark is not where the user put it.'}`);
  assert.ok(Math.abs(baked.cx - placed.cx) < 0.06,
    `the baked stamp's centre is ${baked.cx.toFixed(3)} across the page but it was placed at ${placed.cx.toFixed(3)}`);

  // Size, not just position: a bake that scaled the rect could still centre correctly.
  const w = baked.x1 - baked.x0;
  assert.ok(w > 0.04 && w < 0.6,
    `the baked stamp spans ${(w * 100).toFixed(1)}% of the page width, which is not the size of the widget that was placed`);
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
