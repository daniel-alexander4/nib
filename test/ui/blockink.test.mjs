// D25's RENDERED half: a signature block puts ink on the composited page, inside its own
// widget rect and nowhere else (/pending 302, owed by P07.S06).
//
// ── What was already true, and what was not ─────────────────────────────────
// P07.S06 shipped the STRUCTURAL positive control: `pdfops.SignatureWidgets` reads the widget
// back out of the signed bytes and `internal/p2p/placement_test.go` asserts it is on the page
// the placement named, at the rect the placement named, carrying an /AP stream. That
// distinguishes "placed correctly" from "not placed at all", which placement arithmetic alone
// cannot — but it is structure, and it says at its own line what it cannot see: a block drawn
// white on white, or an /AP stream whose content its BBox clips away. Both leave a widget with
// a correct /Rect and a blank page.
//
// The clause the slice could not close asks for the other half: "rendered and measured, and the
// measurement is DIFFERENTIAL".
//
// ── Why this needs no ceremony fixture ──────────────────────────────────────
// /pending 302 recorded the blocker as "a tier-3 ceremony fixture", and that is not where the
// unrendered risk lives. `p2p.Contribute` hands `sign.Appearance{Image, Page, Rect}` to
// `sign.SignApproval`, which hands it to the VENDORED digitorus/pdfsign — and that library
// writes the /AP stream from those three inputs with no knowledge of a roster. `buildCoSigned`
// (the manual co-sign) and `coSignExchange` (the ceremony) call the same function with the same
// struct; a roster changes only the VALUES of Page and Rect, and that those values reach the
// file verbatim is exactly what the Go guard already proves.
//
// So the question this file answers — does that /AP paint ink at that rect on the composited
// page — is reachable through `POST /api/cosign/sign`, which is entirely local: a multipart PDF
// and an appearance PNG, no peer session, no network. Its one precondition is a pinned peer
// fingerprint, and `POST /api/peers/pin` takes any 64 hex characters.
//
// What stays owed is narrower than the item said and is recorded rather than implied: a NINE
// PARTY ceremony document, rendered, whose blocks carry per-party CONTENT. That is P07.S07's
// fixture, and this file deliberately does not pretend to cover it. "The same code runs" is the
// argument test/ui/fixtures.mjs records this repo being burned by.
//
// ── Why the block sits on the readme page, and why that is the good case ────
// Outside a ceremony `PlacementFor` is `NextPlacement`: blocks stack from the bottom of the
// trailing readme page, 84 pt tall with a 12 pt gap. So this drives D25's ORIGINAL geometry —
// a block over the trust-explainer's prose — which the ceremony's dedicated signature pages no
// longer produce. The differential below is a page against ITSELF one contribution later, so
// the readme's own ink subtracts out and what remains is the new block and only the new block.
//
// ── The renderer, and which reader it stands for ────────────────────────────
// The offscreen renders pass `annotationMode: ENABLE`, which composites annotation appearance
// streams onto the page canvas. That is the receiving reader's view — the forensic question is
// what a counterparty's PDF viewer draws, not what Nib's own chrome does — and Nib's viewer is
// checked separately in the second test, which runs at ENABLE_FORMS like the real app.
//
// ── Which of these has been seen to fail, and how ───────────────────────────
// Three are replayable rows against production code (`./build/redproof.sh <name>`):
//   A1  block-falls-off-the-page                  — and `TestABlockIsActuallyDRAWNWhereItWas
//                                                    Placed` stays GREEN under that patch
//   A3  blocks-overlap-and-the-ink-does-not-fill  — 51.9% of its own rect
//   A5  a-contribution-repaints-an-earlier-block  — 189 px of the fifth block repainted
// Two were probed by breaking the appearance this file supplies, which is a test-side change
// and is recorded as one: A4 with a translucent field (0.0% block colour), A6 with the
// orientation marker drawn at the image's bottom instead of its top. A2 has no red proof and
// says why at its own line.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK } from './harness.mjs';
import { makePDF, writeRawFixture } from './fixtures.mjs';

// A pinned counterparty is the only precondition /api/cosign/sign has. The fingerprint is
// arbitrary: `parseFingerprint` wants 64 hex characters and `pinnedLabel` wants it in the
// vault, and nothing on this path dials it.
const PEER_FP = 'ab'.repeat(32);

// The appearance is a SOLID MAGENTA field with a GREEN marker square in its top-left corner,
// and both choices are load-bearing.
//
// Magenta because the readme is black prose on white paper: "is this pixel the block's" is
// then a question about the pixel and not about a threshold. The green marker is the
// ORIENTATION control — an image placed into a PDF rect can arrive flipped, which is the
// defect class stamplace.test.mjs exists for one level down ("baked a few points high", and
// pdf.js's own stamp editor was dropped over it). A solid field cannot tell a flip from a
// correct placement; a field with a corner can.
const MAGENTA = { r: 255, g: 0, b: 255 };
const GREEN = { r: 0, g: 255, b: 0 };

const SRC = writeRawFixture('blockink-src.pdf', makePDF({ pages: 1, label: 'agreement' }));
const srcBytes = [...fs.readFileSync(SRC)];

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

// measure does the whole drive inside the page: pin, quote, build the appearance, co-sign
// twice, render the readme page of both results, and reduce to numbers.
//
// It runs in the page rather than from node for two reasons. The CSRF token and the loopback
// Origin the write routes require are the browser's to hold; and `window.pdfjsLib` is the
// vendored pdf.js the app itself loads, so the renderer measuring the document is the renderer
// shipping in the product (compare-align.test.mjs is the precedent for both).
async function measure() {
  return page.evaluate(async ([src, peerFP, magenta, green]) => {
    const csrf = (await (await fetch('/api/status')).json()).csrf;
    const post = (url, body, extra = {}) => fetch(url, {
      method: 'POST', headers: { 'X-CSRF-Token': csrf, ...(extra.headers || {}) }, body,
    });
    const json = (url, obj) => post(url, JSON.stringify(obj), {
      headers: { 'Content-Type': 'application/json' },
    });

    const pinned = await json('/api/peers/pin', { fingerprint: peerFP, label: 'Test Counterparty' });
    if (!pinned.ok) throw new Error('pin failed: ' + pinned.status + ' ' + await pinned.text());

    // /quote gives the block's size template, which is what app.js rasterises against. Read
    // rather than copied: a literal rect here would be a second implementation of
    // `NominalBlockRect`, which is the ADR-009 shape that function was written to remove.
    const qr = await json('/api/cosign/quote', { fingerprint: peerFP, intent: 'approve' });
    if (!qr.ok) throw new Error('quote failed: ' + qr.status + ' ' + await qr.text());
    const q = await qr.json();
    const wPt = q.rect[2] - q.rect[0], hPt = q.rect[3] - q.rect[1];

    const appearance = await new Promise((resolve) => {
      const s = 4; // rasterise above page resolution, as the client does
      const cv = document.createElement('canvas');
      cv.width = Math.round(wPt * s); cv.height = Math.round(hPt * s);
      const ctx = cv.getContext('2d');
      ctx.fillStyle = `rgb(${magenta.r},${magenta.g},${magenta.b})`;
      ctx.fillRect(0, 0, cv.width, cv.height);
      // Top-left quarter-height square, in IMAGE coordinates where y grows downward.
      const m = Math.round(cv.height / 4);
      ctx.fillStyle = `rgb(${green.r},${green.g},${green.b})`;
      ctx.fillRect(0, 0, m, m);
      cv.toBlob(resolve, 'image/png');
    });

    const cosign = async (pdfBytes) => {
      const form = new FormData();
      form.append('pdf', new Blob([new Uint8Array(pdfBytes)], { type: 'application/pdf' }), 'doc.pdf');
      form.append('params', JSON.stringify({ fingerprint: peerFP, intent: 'approve', when: q.when }));
      form.append('appearance', appearance, 'attestation.png');
      const r = await post('/api/cosign/sign', form);
      if (!r.ok) throw new Error('sign failed: ' + r.status + ' ' + await r.text());
      return [...new Uint8Array(await r.arrayBuffer())];
    };

    // SIX contributions on one document, and the count is the measurement's whole point.
    //
    // `stackPlacement` puts block i at y = 40 + i*96, 84 pt tall, so the first two blocks sit
    // in the readme page's blank bottom margin — MEASURED: zero non-paper pixels under block 1.
    // A differential there would be magenta painted on white, and D25's clause is about a block
    // painted over the readme's PROSE. Block 5 lands at y 520..604, in the body text, which is
    // the overlap D25 named and the reason placement_test.go drives n=4.
    //
    // The differential is the last step: the same page with five blocks and with six, so the
    // readme's ink and the five earlier blocks all subtract out.
    const docs = [];
    let cur = src;
    for (let i = 0; i < 6; i++) { cur = await cosign(cur); docs.push(cur); }
    const docA = docs[4], docB = docs[5];

    // renderLast draws the document's LAST page — the readme, where NextPlacement stacks —
    // at scale 1, so a canvas pixel IS a PDF point and the only conversion is the y flip.
    const renderLast = async (bytes) => {
      const doc = await window.pdfjsLib.getDocument({ data: new Uint8Array(bytes) }).promise;
      const pg = await doc.getPage(doc.numPages);
      const vp = pg.getViewport({ scale: 1 });
      const cv = document.createElement('canvas');
      cv.width = Math.ceil(vp.width); cv.height = Math.ceil(vp.height);
      await pg.render({
        canvasContext: cv.getContext('2d'),
        viewport: vp,
        annotationMode: window.pdfjsLib.AnnotationMode.ENABLE,
      }).promise;
      const annots = await pg.getAnnotations({ intent: 'display' });
      return {
        w: cv.width,
        h: cv.height,
        pages: doc.numPages,
        data: cv.getContext('2d').getImageData(0, 0, cv.width, cv.height).data,
        // A signature widget is the one whose field type is Sig. pdf.js exposes fieldType on
        // widget annotations; nothing else on these pages is a widget at all.
        sigRects: annots.filter((a) => a.fieldType === 'Sig').map((a) => a.rect),
      };
    };

    const [A, B] = [await renderLast(docA), await renderLast(docB)];

    // The source document, rendered, is the control: if IT already carried the block's
    // colours then the ink measured below is not the block's own (stamplace's lesson).
    const srcDoc = await window.pdfjsLib.getDocument({ data: new Uint8Array(src) }).promise;
    const sp = await srcDoc.getPage(1);
    const svp = sp.getViewport({ scale: 1 });
    const scv = document.createElement('canvas');
    scv.width = Math.ceil(svp.width); scv.height = Math.ceil(svp.height);
    await sp.render({ canvasContext: scv.getContext('2d'), viewport: svp }).promise;
    const sd = scv.getContext('2d').getImageData(0, 0, scv.width, scv.height).data;

    const near = (d, i, c, tol = 60) =>
      Math.abs(d[i] - c.r) < tol && Math.abs(d[i + 1] - c.g) < tol && Math.abs(d[i + 2] - c.b) < tol;
    const countColour = (d, c) => {
      let n = 0;
      for (let i = 0; i < d.length; i += 4) if (near(d, i, c)) n++;
      return n;
    };

    if (A.w !== B.w || A.h !== B.h) {
      return { mismatch: { a: [A.w, A.h], b: [B.w, B.h] } };
    }

    // rectToBox converts a PDF rect to canvas pixels. y flips: PDF y grows up from the bottom,
    // canvas y grows down from the top.
    const rectToBox = ([x0, y0, x1, y1]) => ({
      x0: Math.round(Math.min(x0, x1)), x1: Math.round(Math.max(x0, x1)),
      y0: Math.round(B.h - Math.max(y0, y1)), y1: Math.round(B.h - Math.min(y0, y1)),
    });

    // The new block is the widget B has and A does not, identified by rect rather than by
    // order: annotation order is the file's, not the signing order's.
    const key = (r) => r.map((v) => Math.round(v)).join(',');
    const oldKeys = new Set(A.sigRects.map(key));
    const added = B.sigRects.filter((r) => !oldKeys.has(key(r)));

    // The differential: every pixel where the two renders of the SAME page disagree.
    let n = 0, x0 = 1e9, y0 = 1e9, x1 = -1, y1 = -1;
    for (let i = 0; i < A.data.length; i += 4) {
      const d = Math.abs(A.data[i] - B.data[i]) + Math.abs(A.data[i + 1] - B.data[i + 1]) +
        Math.abs(A.data[i + 2] - B.data[i + 2]);
      if (d < 24) continue; // JPEG-free renders; this only rejects antialiasing jitter
      const p = i / 4, x = p % B.w, y = Math.floor(p / B.w);
      n++;
      if (x < x0) x0 = x; if (x > x1) x1 = x;
      if (y < y0) y0 = y; if (y > y1) y1 = y;
    }
    const changed = n ? { n, x0, y0, x1, y1 } : { n: 0 };

    // Inside the added widget's box: how much is the block's own ink, and where is the
    // orientation marker's centroid.
    const box = added.length === 1 ? rectToBox(added[0]) : null;
    let inBlock = 0, inTotal = 0, greenN = 0, gx = 0, gy = 0, changedInBox = 0, proseUnder = 0;
    if (box) {
      for (let y = box.y0; y < box.y1; y++) {
        for (let x = box.x0; x < box.x1; x++) {
          const i = (y * B.w + x) * 4;
          inTotal++;
          if (near(B.data, i, magenta) || near(B.data, i, green)) inBlock++;
          const wasWhite = A.data[i] > 240 && A.data[i + 1] > 240 && A.data[i + 2] > 240;
          if (!wasWhite) proseUnder++;
          if (near(B.data, i, green)) { greenN++; gx += x; gy += y; }
          const d = Math.abs(A.data[i] - B.data[i]) + Math.abs(A.data[i + 1] - B.data[i + 1]) +
            Math.abs(A.data[i + 2] - B.data[i + 2]);
          if (d >= 24) changedInBox++;
        }
      }
    }

    // Did any changed pixel land inside the FIRST block's box? That is the incremental-update
    // property, visually: contributing a second signature must not disturb the first's mark.
    let changedInOld = 0;
    for (const r of A.sigRects) {
      const ob = rectToBox(r);
      for (let y = ob.y0; y < ob.y1; y++) {
        for (let x = ob.x0; x < ob.x1; x++) {
          const i = (y * B.w + x) * 4;
          const d = Math.abs(A.data[i] - B.data[i]) + Math.abs(A.data[i + 1] - B.data[i + 1]) +
            Math.abs(A.data[i + 2] - B.data[i + 2]);
          if (d >= 24) changedInOld++;
        }
      }
    }

    // Non-paper, non-block pixels on the readme page: the prose the differential is claiming
    // to leave alone. Zero here would make "unchanged elsewhere" a statement about a blank page.
    let prose = 0;
    for (let i = 0; i < A.data.length; i += 4) {
      const white = A.data[i] > 240 && A.data[i + 1] > 240 && A.data[i + 2] > 240;
      if (!white && !near(A.data, i, magenta) && !near(A.data, i, green)) prose++;
    }

    return {
      pageW: B.w, pageH: B.h, pagesA: A.pages, pagesB: B.pages,
      sigsA: A.sigRects.length, sigsB: B.sigRects.length,
      addedRects: added, box, changed, changedInBox, changedInOld,
      inBlock, inTotal, prose, proseUnder,
      greenCentroid: greenN ? { n: greenN, x: gx / greenN, y: gy / greenN } : { n: 0 },
      srcMagenta: countColour(sd, magenta), srcGreen: countColour(sd, green),
      docB: btoa(String.fromCharCode(...new Uint8Array(docB))),
    };
  }, [srcBytes, PEER_FP, MAGENTA, GREEN]);
}

// /api/cosign/quote answers 400 without an open document, by design — quoting a block for no
// document is a client bug. So the drive starts where a user's does: the document open in the
// viewer, then Co-sign.
await h.openDocument(SRC, 1);
const m = await measure();
console.error('MEASURED ' + JSON.stringify({...m, docB: m.docB.length}));

test('a signature block paints ink inside its own widget rect, and nowhere else on the page', () => {
  assert.equal(m.mismatch, undefined,
    `setup: the two renders of the same page differ in size (${JSON.stringify(m.mismatch)}) — ` +
    'a pixel-for-pixel difference is meaningless across different canvases');

  // ── Setup assertions: the stimulus happened, and the control is clean ──────
  assert.equal(m.srcMagenta, 0,
    `setup: the source document already renders ${m.srcMagenta} magenta pixel(s) — the ink ` +
    'measured below would not be the block\'s own');
  assert.equal(m.srcGreen, 0,
    `setup: the source document already renders ${m.srcGreen} green pixel(s)`);
  assert.equal(m.pagesA, 2, `setup: co-signing produced ${m.pagesA} pages, want source + readme`);
  assert.equal(m.pagesB, 2,
    `setup: the sixth contribution changed the page count to ${m.pagesB} — it must be an ` +
    'incremental update on the same pages, not a rewrite');
  assert.equal(m.sigsA, 5, `setup: five contributions left ${m.sigsA} signature widget(s), want 5`);
  assert.equal(m.sigsB, 6,
    `setup: the sixth contribution left ${m.sigsB} signature widget(s), want 6 — with no ` +
    'sixth block there is nothing for the differential to find');
  assert.ok(m.prose > 2000,
    `setup: the readme page carries only ${m.prose} non-paper pixel(s), so "the rest of the ` +
    'page is untouched" would be a claim about a blank page');
  assert.ok(m.box, `setup: expected exactly one added widget, got ${JSON.stringify(m.addedRects)}`);
  assert.ok(m.proseUnder > 500,
    `setup: only ${m.proseUnder} of the ${m.inTotal} pixels under this block were anything but ` +
    'blank paper before it was drawn, so "the block covered the readme\'s prose" would be a ' +
    'claim about an empty margin — which is exactly what blocks 1 and 2 sit on (measured: 0)');

  // ── A1. There is ink at all ───────────────────────────────────────────────
  // The half `HasAP` cannot reach: a widget whose /AP stream exists and draws nothing —
  // white on white, or content its own BBox clips away — passes every structural check and
  // leaves the page exactly as it was.
  assert.ok(m.changed.n > 0,
    'the second contribution changed no pixel on the page it placed its block on: the widget ' +
    'is in the file, carries an appearance stream, and draws nothing a reader can see');

  // ── A2. The ink is inside the widget's own rect ───────────────────────────
  // The rect comes from the DOCUMENT (pdf.js `getAnnotations`), not from a constant copied
  // out of Go: structure says where the block is, pixels say where the ink is, and the two
  // are read by different parts of pdf.js — the dictionary parser and the renderer.
  //
  // **This is the one assertion here with NO red proof, and the reason is worth keeping.**
  // Nib passes ONE rect to `sign.Appearance`, and digitorus/pdfsign derives both the widget
  // /Rect and the appearance's placement from it — so every Nib-side mutation moves the two
  // together and this stays green (the off-the-page row moves both, and A1 is what fires).
  // It is therefore a guard on the VENDORED writer: a pdfsign upgrade that gets the form
  // XObject's BBox or Matrix wrong is what it is here to catch, and that defect cannot be
  // induced from this repo. Recorded rather than deleted, and recorded rather than implied
  // to be proven.
  const b = m.box, c = m.changed, tol = 2;
  assert.ok(
    c.x0 >= b.x0 - tol && c.x1 <= b.x1 + tol && c.y0 >= b.y0 - tol && c.y1 <= b.y1 + tol,
    `the block's ink spans x ${c.x0}..${c.x1}, y ${c.y0}..${c.y1} while its widget rect is ` +
    `x ${b.x0}..${b.x1}, y ${b.y0}..${b.y1} — the appearance stream draws outside the ` +
    'rectangle the document says it occupies');

  // ── A3. The ink FILLS the rect ────────────────────────────────────────────
  // A BBox or Matrix that clips the appearance to a sliver satisfies A1 and A2 and still
  // leaves a block a reader cannot read.
  const fill = m.changedInBox / m.inTotal;
  assert.ok(fill > 0.85,
    `the block's ink covers ${(fill * 100).toFixed(1)}% of its own widget rect — an ` +
    'appearance clipped to part of its rectangle is placed, drawn, and unreadable');

  // ── A4. Nothing shows through it ──────────────────────────────────────────
  // D25's clause, literally: this block is over the readme's body text (the setup assertion
  // above measured how much), and the block's field is opaque, so that prose is COVERED rather
  // than overlaid. A semi-transparent or undersized appearance leaves it legible underneath.
  const opaque = m.inBlock / m.inTotal;
  assert.ok(opaque > 0.95,
    `only ${(opaque * 100).toFixed(1)}% of the block's rect renders in the block's own ` +
    `colours — ${m.proseUnder} pixel(s) of readme prose sit beneath it and it is not covering them`);

  // ── A5. The first block is undisturbed ────────────────────────────────────
  // `Contribute`'s premise is that a later contribution is an incremental update that never
  // disturbs an earlier signature. That is asserted structurally today — `sign.Verify`
  // reports both signers — and never visually.
  assert.equal(m.changedInOld, 0,
    `contributing a sixth signature changed ${m.changedInOld} pixel(s) inside the five EARLIER ` +
    'blocks\' rects: an incremental update repainted a mark that was already signed for');

  // ── A6. It is the right way up ────────────────────────────────────────────
  // The marker is drawn in the appearance image's top-left, where y grows DOWNWARD. In the
  // page it must land at the rect's top-left, where PDF y grows upward — the one place those
  // two conventions meet, and the seam a wrong-but-self-consistent transform passes.
  const g = m.greenCentroid;
  assert.ok(g.n > 0, 'the orientation marker did not render at all inside the block');
  assert.ok(g.x < (b.x0 + b.x1) / 2 && g.y < (b.y0 + b.y1) / 2,
    `the orientation marker's centroid is at (${g.x.toFixed(0)}, ${g.y.toFixed(0)}) in a rect ` +
    `of x ${b.x0}..${b.x1}, y ${b.y0}..${b.y1} — the appearance is mirrored or rotated on the page`);
});

// The measurement above is the receiving reader's view (annotationMode ENABLE). Nib's own
// viewer runs at ENABLE_FORMS, which renders form widgets through the annotation layer rather
// than onto the page canvas — a different code path in pdf.js, and one a Nib user looks at
// every time they check their own co-signed document before sending it.
test('Nib\'s own viewer shows the block on the page it was placed on', async () => {
  const file = path.join(WORK, 'fixtures', 'blockink-signed.pdf');
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, Buffer.from(m.docB, 'base64'));

  await h.openDocument(file, 2);
  await page.waitForFunction(() => {
    const cvs = document.querySelectorAll('.viewerContainer:not([hidden]) .page canvas');
    return cvs.length >= 2 && cvs[1].width > 0;
  }, null, { timeout: 20000 });

  const seen = await page.evaluate(() => {
    const cv = document.querySelectorAll('.viewerContainer:not([hidden]) .page canvas')[1];
    const d = cv.getContext('2d').getImageData(0, 0, cv.width, cv.height).data;
    let n = 0;
    for (let i = 0; i < d.length; i += 4) {
      if (d[i] > 195 && d[i + 1] < 60 && d[i + 2] > 195) n++;
    }
    return { n, w: cv.width, h: cv.height };
  });

  assert.ok(seen.n > 0,
    `the readme page renders ${seen.n} of the block's magenta pixels in Nib's own viewer on a ` +
    `${seen.w}x${seen.h} canvas: the document carries six visible signature blocks and the ` +
    'user checking their own work before sending it sees none of them');
});
