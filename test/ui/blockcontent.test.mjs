// The block a user actually gets: `renderAttestation`'s own ink, on the page (/pending 305).
//
// ── The gap, and why it survived two tiers ──────────────────────────────────
// `web/app.js:955` is where a Nib signature block becomes pixels. It fills white, strokes a
// black border, and draws the server's `AppearanceLines` in black — and the PNG it returns is
// the appearance every co-sign and every ceremony hop signs into the document.
//
// Nothing rendered it. Tier 2 cannot: jsdom has no canvas, so `ladderdefault.test.mjs`
// replaces `getContext` with a stub whose `fillText` is a no-op and `toBlob` with a
// one-byte PNG — "just enough for renderAttestation() to resolve and the POST to fire", in
// its own words. That is honest about what it proves (the request happens) and it means the
// drawing code has never had its output looked at by anything.
//
// `blockink.test.mjs` (/pending 302) rendered the OTHER half: given an appearance, the /AP
// stream puts it inside the widget's rect, fills it, and is the right way up. It supplies its
// own solid magenta field on purpose — a known colour is what makes "is this pixel the
// block's" a question about the pixel rather than about a threshold — so it says nothing
// about what `renderAttestation` draws. A white-on-white block would satisfy every one of its
// assertions except by accident.
//
// This file is the missing half: drive the REAL Co-sign flow through the UI, so the appearance
// is the one the product makes, and then look at it.
//
// ── Why the UI and not the API ──────────────────────────────────────────────
// `renderAttestation` is not exported and is reached only from `cosign()`, `sessionQuote` and
// the pending-document path. Exporting it for a test would put the test beside the function
// rather than behind the flow, and the interesting failures are at the seam: the server's
// `lines` and `rect` arriving, being rasterised at the rect's aspect, and coming back as an
// appearance the signing library stretches to fill. So this clicks the button.
//
// ── What it does not cover, and what does ───────────────────────────────────
// The block still lands on the readme page, because outside a ceremony `PlacementFor` is
// `NextPlacement`. A block on an ALLOCATED SIGNATURE PAGE — which is a page that is not the
// last one — is still unrendered, and stays /pending 305: no local door contributes with a
// roster (`buildCoSigned` passes `p2p.Roster{}` at the only route that takes a document from
// the client), so producing one needs nine identities and therefore nine nibs.
//
// ── Which of these has been seen to fail ────────────────────────────────────
// All four, each a replayable row against `renderAttestation` itself
// (`./build/redproof.sh <name>`):
//   A1  block-is-drawn-white-on-white       — strokeStyle and fillStyle to '#fff': 0 dark px
//   A2  block-frame-draws-but-text-does-not — the fillText loop removed: 0 interior dark px
//   A3  block-lines-share-one-baseline      — `pad + i*lineH` to `pad`: strips [6927,0,0,0,0]
//   A4  block-line-stack-overflows-its-frame — lineH over `lines.length - 2`: ink at row 239
//                                              of a 240-row interior
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK } from './harness.mjs';
import { makePDF, writeRawFixture } from './fixtures.mjs';

const PEER_FP = 'cd'.repeat(32);
const SRC = writeRawFixture('blockcontent-src.pdf', makePDF({ pages: 1, label: 'agreement' }));
const OUT_DIR = path.join(WORK, 'cosigned');
fs.mkdirSync(OUT_DIR, { recursive: true });

// Rendered at 3x. The block is 280x84 pt, so at 1:1 its five lines are about 9 px tall and
// their rows blur into each other — and the line count is this file's sharpest assertion.
// Three makes each line ~27 px, which is what the client rasterises at anyway.
const SCALE = 3;

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

// A peer has to be pinned before the modal will offer one — `openCosign` disables the button
// with none, and `cosignAttestation` refuses an unpinned fingerprint server-side. Pinned
// through the API rather than the Peers UI: this file is about the block, and driving a second
// surface to set up the first buys nothing it asserts.
await page.evaluate(async (fp) => {
  const csrf = (await (await fetch('/api/status')).json()).csrf;
  const r = await fetch('/api/peers/pin', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint: fp, label: 'Test Counterparty' }),
  });
  if (!r.ok) throw new Error('pin failed: ' + r.status + ' ' + await r.text());
}, PEER_FP);

await h.openDocument(SRC, 1);

// ── The real flow: Co-sign… → pick the peer → Co-sign & save… → Save ────────
// The Co-sign button lives on the Collaborate tab under the ORIGINATE role — the role
// toggle swaps the tool set, so selecting the tab is not enough to reach it.
await h.mode('collaborate');
await page.click('.roleopt[data-role="originate"]');
await page.click('#cosignBtn');
await page.waitForFunction(() => !document.getElementById('cosignModal').hidden);
await page.selectOption('#cosignPeer', PEER_FP);
await page.fill('#cosignIntent', 'I agree to sign this document.');
await page.click('#cosignGo');
await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
await page.fill('#saveAsName', 'cosigned.pdf');
await page.fill('#saveAsDir', OUT_DIR);
await page.click('#saveAsGo');
await page.waitForFunction(() => document.getElementById('saveAsModal').hidden);

const OUT = path.join(OUT_DIR, 'cosigned.pdf');

// The lines are the server's, and the count below is read from it rather than written here:
// `AppearanceLines` is Go's, and a literal 5 in this file would be a second statement of it
// that goes stale the first time a line is added (ADR-009 in the small).
const quote = await page.evaluate(async (fp) => {
  const csrf = (await (await fetch('/api/status')).json()).csrf;
  const r = await fetch('/api/cosign/quote', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint: fp, intent: 'I agree to sign this document.' }),
  });
  return r.json();
}, PEER_FP);

// measure renders the saved document's last page and reduces the block to numbers: where its
// dark ink is, and how it is arranged in rows.
const m = await page.evaluate(async ([bytes, scale, lineCount]) => {
  const doc = await window.pdfjsLib.getDocument({ data: new Uint8Array(bytes) }).promise;
  const pg = await doc.getPage(doc.numPages);
  const vp = pg.getViewport({ scale });
  const cv = document.createElement('canvas');
  cv.width = Math.ceil(vp.width); cv.height = Math.ceil(vp.height);
  await pg.render({
    canvasContext: cv.getContext('2d'),
    viewport: vp,
    annotationMode: window.pdfjsLib.AnnotationMode.ENABLE,
  }).promise;
  const d = cv.getContext('2d').getImageData(0, 0, cv.width, cv.height).data;
  const annots = await pg.getAnnotations({ intent: 'display' });
  const sigs = annots.filter((a) => a.fieldType === 'Sig');
  if (sigs.length !== 1) return { pages: doc.numPages, sigs: sigs.length };

  // PDF points to canvas pixels: x scales, y scales and flips.
  const [rx0, ry0, rx1, ry1] = sigs[0].rect;
  const box = {
    x0: Math.round(Math.min(rx0, rx1) * scale), x1: Math.round(Math.max(rx0, rx1) * scale),
    y0: Math.round(cv.height - Math.max(ry0, ry1) * scale),
    y1: Math.round(cv.height - Math.min(ry0, ry1) * scale),
  };
  const dark = (x, y) => {
    const i = (y * cv.width + x) * 4;
    return d[i] < 140 && d[i + 1] < 140 && d[i + 2] < 140;
  };

  // The border band is the outer 2 pt on each side; the interior is everything inside it, and
  // it is where the TEXT lives. Separating them is what lets "the frame arrived" and "the
  // writing arrived" fail independently — an appearance drawn in white loses both, and a
  // layout fault loses only the second.
  const band = Math.max(2, Math.round(2 * scale));
  let borderDark = 0, interiorDark = 0, outsideDark = 0;
  const rows = [];
  for (let y = box.y0; y < box.y1; y++) {
    let rowDark = 0;
    for (let x = box.x0; x < box.x1; x++) {
      const inner = x >= box.x0 + band && x < box.x1 - band &&
        y >= box.y0 + band && y < box.y1 - band;
      if (!inner) { if (dark(x, y)) borderDark++; continue; }
      if (dark(x, y)) { interiorDark++; rowDark++; }
    }
    if (y >= box.y0 + band && y < box.y1 - band) rows.push(rowDark);
  }
  for (let y = 0; y < cv.height; y++) {
    for (let x = 0; x < cv.width; x++) {
      if (x >= box.x0 && x < box.x1 && y >= box.y0 && y < box.y1) continue;
      if (dark(x, y)) outsideDark++;
    }
  }

  // ── How the lines are counted, and why not by clustering ──────────────────
  // The first draft grouped contiguous inked rows into bands and expected one band per line.
  // It reported SIX for five lines, and the profile said why: "Signer: Nib User" ends its
  // body at row 72 and its 'g' descender resumes at 75, with two rows between carrying fewer
  // than three dark pixels. That is a measurement of antialiasing, and tuning the threshold
  // until it answered five would have been tuning until green.
  //
  // The interior is divided into `lines` equal strips instead. That is not a copy of
  // `renderAttestation`'s layout arithmetic — it never mentions `pad` or `lineH` — it is the
  // observable consequence of it: evenly spaced lines filling the block put one line's worth
  // of ink in each strip. An empty strip is a line that did not draw, or every line drawn on
  // one baseline, or a stack that ran off the bottom into the clip.
  const strips = [];
  const n = lineCount;
  for (let s = 0; s < n; s++) {
    const from = Math.floor((s * rows.length) / n), to = Math.floor(((s + 1) * rows.length) / n);
    strips.push(rows.slice(from, to).reduce((a, b) => a + b, 0));
  }
  let lastInk = -1;
  for (let i = 0; i < rows.length; i++) if (rows[i] >= 3) lastInk = i;

  return {
    pages: doc.numPages, sigs: sigs.length, canvas: [cv.width, cv.height], box,
    borderDark, interiorDark, outsideDark,
    interiorRows: rows.length, strips, lastInk,
  };
}, [[...fs.readFileSync(OUT)], SCALE, quote.lines.length]);

test("the co-sign block carries its own ink: a frame, and one row of text per line", () => {
  // ── Setup: the flow produced what the rest of this reads ──────────────────
  assert.equal(m.pages, 2, `setup: the saved document has ${m.pages} page(s), want source + readme`);
  assert.equal(m.sigs, 1,
    `setup: the saved document carries ${m.sigs} signature widget(s), want 1 — the co-sign ` +
    'flow did not leave a visible block to look at');
  assert.ok(m.outsideDark > 1000,
    `setup: the readme page renders only ${m.outsideDark} dark pixel(s) outside the block, so ` +
    'the "dark" test below is finding nothing anywhere and would report an empty block as a ' +
    'property of the block');

  // ── A1. The frame arrived: the appearance is not invisible ────────────────
  // `renderAttestation` strokes a black rectangle around the whole canvas. If the block were
  // drawn white on white — the failure /pending 302 named and could not reach, because it
  // supplied its own opaque field — this is the assertion that goes first.
  assert.ok(m.borderDark > 200,
    `the block's border contributes ${m.borderDark} dark pixel(s): the appearance reached the ` +
    'page and drew nothing visible on it, which is a signature block a reader cannot see');

  // ── A2. The writing arrived ───────────────────────────────────────────────
  assert.ok(m.interiorDark > 200,
    `the block's interior carries ${m.interiorDark} dark pixel(s) inside its frame: the frame ` +
    'is on the page and the attestation text is not, so the block says nothing');

  // ── A3. Every line drew, down the whole block ─────────────────────────────
  // The strongest thing this file asserts, and the count is the SERVER's: `quote.lines` is
  // Go's `AppearanceLines`, so a sixth line added there makes this test expect a sixth strip
  // without anybody editing it. Five lines on one baseline empties four strips; a line that
  // never drew empties one; a layout that pushes the stack past the canvas loses the last.
  const empty = m.strips.map((n, i) => [i, n]).filter(([, n]) => n < 40);
  assert.equal(empty.length, 0,
    `strip(s) ${JSON.stringify(empty)} of the block's ${m.strips.length} carry almost no ink ` +
    `while the server sent ${m.strips.length} line(s) (${JSON.stringify(quote.lines)}); the ` +
    `per-strip ink is ${JSON.stringify(m.strips)}. The attestation is not laid out one line ` +
    'per row of the block — collapsed onto one baseline, or running off it into the clip');

  // ── A4. The last line clears the frame ────────────────────────────────────
  // `lineH` is `(height - 2*pad) / lines.length`, so the arithmetic that fits the lines is the
  // arithmetic that can overflow them. A last line running into the bottom border is text a
  // reader gets half of.
  assert.ok(m.lastInk < m.interiorRows - 4,
    `the block's lowest ink is at row ${m.lastInk} of its ${m.interiorRows}-row interior: the ` +
    'attestation is running into its own frame, so its last line is cut off');

  // ── There is no opacity assertion here, and that is deliberate ────────────
  // A draft asserted the un-inked interior is white, i.e. that `renderAttestation`'s opaque
  // fill really covers what is under it. Deleting the fill left it GREEN: the block sits at
  // the first stack position, y 40..124, and the readme page's bottom margin there is blank —
  // measured at 0 non-paper pixels — so a transparent field and a white one render the same.
  // An assertion that cannot fail is a green implying coverage it does not have, which
  // `docs/red-proofs.md` says to delete rather than keep. Opacity is proven where it can be:
  // `blockink.test.mjs` stacks six blocks so the sixth lands over 4660 pixels of body text.
});
