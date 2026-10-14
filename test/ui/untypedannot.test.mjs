// An annotation with no /Subtype is drawn from its appearance (/pending 889 part 4).
//
// A real-estate forms engine writes initials, signatures and dates as page annotations that carry an
// appearance and no /Subtype: 49 in four real documents, every one with /F 4 (Print) and a single /AP /N
// stream drawing one image. Nib keeps them in the file (ADR-168). Whether the window SHOWS them was
// recorded as "pdf.js does not draw an annotation with no subtype" — measured wrong on 2026-10-13: pdf.js
// 6.1.200 builds a plain Annotation for one (a warning, "missing the required /Subtype"), and its page
// operator list draws the appearance on screen and in print; Chrome's own viewer (PDFium) draws them too, and
// poppler does not. In those four documents each repeats a mark a form field already draws in the same place,
// so nothing visible turned on it — and that is why this file draws the mark ALONE: a pdf.js bump that stopped
// drawing such an annotation would otherwise leave every real document looking the same.
//
// jsdom cannot see any of this: tier 2 stubs the viewer.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeRawFixture } from './fixtures.mjs';

// One blank 200×200 page; its only mark is a red square drawn by an annotation with no /Subtype, over the
// middle of the page (Rect 50..150). The page's own content draws nothing.
function untypedAnnotPDF() {
  const ap = '1 0 0 rg 0 0 100 100 re f';
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Annots [5 0 R] >>',
    '<< /Length 0 >>\nstream\n\nendstream',
    '<< /P 3 0 R /Rect [50 50 150 150] /F 4 /NM (untyped-1) /AP << /N 6 0 R >> >>',
    `<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Length ${ap.length} >>\nstream\n${ap}\nendstream`,
  ];
  let out = '%PDF-1.7\n';
  const offsets = [];
  objs.forEach((o, i) => {
    offsets.push(out.length);
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  for (const o of offsets) out += `${String(o).padStart(10, '0')} 00000 n \n`;
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

const DOC = writeRawFixture('untypedannot.pdf', untypedAnnotPDF());

const h = await launch();
const page = h.page;

after(async () => { await shutdown(h); });

// The colour of the page's canvas at a fraction of its width and height, once the page is drawn.
const colourAt = (fx, fy) => page.evaluate(([x, y]) => {
  const c = document.querySelector('.viewerContainer:not([hidden]) .page[data-page-number="1"] canvas');
  if (!c || !c.width) return null;
  const d = c.getContext('2d').getImageData(Math.floor(c.width * x), Math.floor(c.height * y), 1, 1).data;
  return [d[0], d[1], d[2]];
}, [fx, fy]);

test('an annotation with no /Subtype is drawn from its appearance', async () => {
  await h.openDocument(DOC, 1);
  await page.waitForFunction(() => {
    const p = document.querySelector('.viewerContainer:not([hidden]) .page[data-page-number="1"]');
    return p && p.dataset.loaded === 'true' && p.querySelector('canvas');
  });
  // Stimulus: the page is drawn, and white where nothing draws — or "red in the middle" says nothing.
  const corner = await colourAt(0.1, 0.1);
  assert.ok(corner && corner.every((v) => v > 240), `setup: the page's corner is ${JSON.stringify(corner)}, want white`);
  const middle = await colourAt(0.5, 0.5);
  assert.ok(middle && middle[0] > 200 && middle[1] < 60 && middle[2] < 60,
    `the middle of the page is ${JSON.stringify(middle)}: the annotation with no /Subtype was not drawn — the window `
    + 'no longer shows the initials and signatures a real forms engine writes this way');
});
