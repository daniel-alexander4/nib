// PDF fixtures, generated rather than committed.
//
// A checked-in binary fixture is opaque in review — you cannot see what changed,
// or what it contains, without opening it in something. This is forty lines you
// can read, and it takes a page count as a parameter, which is what makes the
// 80-page document the thumbnail-race test needs free rather than another blob.
//
// Deliberately hand-rolled rather than routed through internal/testpdf: that is a
// Go package, importable only from Go, and shelling out to a generator would put
// a build step between the test and its input.
import fs from 'node:fs';
import path from 'node:path';
import { WORK } from './harness.mjs';

// makePDF builds a minimal but genuinely valid PDF: a catalog, a page tree, one
// content stream per page, and a real xref table with byte offsets. pdf.js parses
// it the same way it parses anything else — the point of tier 3 is that nothing
// here is stubbed.
// `form: true` adds a single AcroForm text field to page one.
//
// `widePage: n` makes page n twice as wide as the rest (1224x792 against 612x792).
// It exists for one thing: pdf.js sizes every page div from PAGE ONE's viewport at
// `pagesinit` and only gives each page its true size as that page resolves, so a
// document whose pages are all the same size cannot tell "still loading" from
// "loaded" by looking at the DOM. One differently-shaped page makes both edges of
// the load window observable, which is what lets a test prove it ran inside it.
//
// It exists for one acceptance clause and it is worth saying which: P06's "switching
// preserves … form fills". A form fill is a pdf.js `annotationStorage` value, which is a
// different mechanism from a Nib overlay — the overlay lives in our DOM, the fill lives
// in pdf.js's — and the two are preserved for the same underlying reason (the view is
// hidden, never torn down). "Same reason" is exactly the argument that lets an
// unexercised clause read as met, so the clause gets its own fixture and its own drive.
export function makePDF({ pages = 3, label = 'page', form = false, widePage = 0 } = {}) {
  const objs = [];
  const add = (s) => objs.push(s);
  const kids = [];
  for (let i = 0; i < pages; i++) kids.push(`${3 + i * 2} 0 R`);

  const fontObj = 3 + pages * 2;
  const fieldObj = fontObj + 1; // appended last; only referenced when `form`
  add(form
    ? `<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [${fieldObj} 0 R] /DA (/Helv 0 Tf 0 g) /DR << /Font << /Helv ${fontObj} 0 R >> >> >> >>`
    : '<< /Type /Catalog /Pages 2 0 R >>');
  add(`<< /Type /Pages /Kids [${kids.join(' ')}] /Count ${pages} >>`);
  for (let i = 0; i < pages; i++) {
    const annots = form && i === 0 ? ` /Annots [${fieldObj} 0 R]` : '';
    const box = i + 1 === widePage ? '[0 0 1224 792]' : '[0 0 612 792]';
    add(`<< /Type /Page /Parent 2 0 R /MediaBox ${box} /Contents ${4 + i * 2} 0 R /Resources << /Font << /F1 ${fontObj} 0 R >> >>${annots} >>`);
    // Distinct text per page, so "a different document opened" is observable
    // rather than asserted.
    const content = `BT /F1 36 Tf 72 700 Td (${label} ${i + 1} of ${pages}) Tj ET`;
    add(`<< /Length ${content.length} >>\nstream\n${content}\nendstream`);
  }
  add('<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>');
  if (form) {
    // A widget annotation IS the field here — the single-widget case, where pdf.js reads
    // the merged dictionary directly. /F 4 is the print flag, which pdf.js requires
    // before it renders an interactive control at all.
    add(`<< /Type /Annot /Subtype /Widget /FT /Tx /T (fill1) /V () /Rect [72 560 400 596] /F 4 /P 3 0 R /DA (/Helv 12 Tf 0 g) >>`);
  }

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
  // latin1: the bytes above are all single-byte, and the xref offsets are byte
  // offsets — encoding as utf8 would shift them and produce a corrupt file.
  return Buffer.from(out, 'latin1');
}

// writeFixture puts a generated PDF inside the run's throwaway work dir, so it is
// cleaned up with everything else and never lands in the repo.
export function writeFixture(name, opts) {
  const dir = path.join(WORK, 'fixtures');
  fs.mkdirSync(dir, { recursive: true });
  const file = path.join(dir, name);
  fs.writeFileSync(file, makePDF(opts));
  return file;
}
