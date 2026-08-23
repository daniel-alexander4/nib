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
import zlib from 'node:zlib';
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

// makeCJKPDF builds a Type0 document encoded with a PREDEFINED CMap — the one
// class of PDF that pdf.js cannot decode from the file alone.
//
// `/Encoding /UniJIS-UCS2-H` is a named CMap that ships with pdf.js as a separate
// data file, not compiled into the worker: the worker carries only the LIST of
// names it recognises and fetches the table from `cMapUrl`. So the mapping from
// these bytes to CIDs — and, with no `/ToUnicode` here deliberately, to
// characters — exists nowhere in this file or in the viewer. That is the whole
// point of the fixture: text comes out of it only if the CMap data was found.
//
// The font is deliberately NOT embedded. A Japanese document produced by a
// Japanese office suite names one of the standard Adobe CJK faces and expects the
// reader to substitute; that is the document Nib meets, and an embedded font would
// side-step the CMap question entirely (an embedded CIDFont is normally
// Identity-H, which pdf.js resolves internally and which would prove nothing).
export function makeCJKPDF(text = '日本語') {
  // UniJIS-UCS2-H consumes two bytes per code unit, big-endian — so the string is
  // written as UTF-16BE and every glyph is addressed by its Unicode code point.
  const hex = [...text].map((c) => c.codePointAt(0).toString(16).padStart(4, '0')).join('');
  const content = `BT /F1 36 Tf 72 700 Td <${hex}> Tj ET`;
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>',
    `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
    '<< /Type /Font /Subtype /Type0 /BaseFont /KozMinPr6N-Regular /Encoding /UniJIS-UCS2-H /DescendantFonts [6 0 R] >>',
    '<< /Type /Font /Subtype /CIDFontType0 /BaseFont /KozMinPr6N-Regular /CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 6 >> /FontDescriptor 7 0 R /DW 1000 >>',
    '<< /Type /FontDescriptor /FontName /KozMinPr6N-Regular /Flags 4 /FontBBox [0 -120 1000 880] /ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 700 /StemV 80 >>',
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

// writeRawFixture writes already-built bytes into the run's work dir.
export function writeRawFixture(name, bytes) {
  const dir = path.join(WORK, 'fixtures');
  fs.mkdirSync(dir, { recursive: true });
  const file = path.join(dir, name);
  fs.writeFileSync(file, bytes);
  return file;
}

// ── Scanned-document fixtures ────────────────────────────────────────────────
//
// makeScanPDF builds a TEXT-LESS document whose pages are greyscale images, and lets a test apply
// the degradations a scanner introduces. /pending 8's premise — that verifying the perceptual
// compare needs real scans Dan supplies — is false, and the code says why: `pageDHash` reduces a
// page to a 9x8 grid of cell means before it compares anything, so sensor noise, paper texture and
// halftoning are destroyed before the algorithm reads a byte. What survives to reach `hamming` is
// gross tonal structure and the illumination envelope, and both are exactly reproducible here.
//
// The PRNG is deterministic: a fixture that differs run to run turns a threshold measurement into
// a flake, and this repo has spent four sessions on one of those already.
function mulberry32(a) {
  return function () {
    a |= 0; a = (a + 0x6D2B79F5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function paintBar(px, w, h, y0, y1, x0, x1, ink) {
  for (let y = Math.floor(y0 * h); y < Math.floor(y1 * h); y++) {
    for (let x = Math.floor(x0 * w); x < Math.floor(x1 * w); x++) px[y * w + x] = ink;
  }
}

// pageField paints nine columns of genuinely differing greys, keyed to the page index.
//
// **Vertical structure is forced by what the hash measures.** A dHash emits
// `lum(cell) > lum(cell to its right)` with a STRICT comparison, so two adjacent cells of equal
// brightness give 0 — and a page of horizontal text bars on flat paper is mostly such pairs. A
// first draft built that way could not discriminate its own pages, and its own setup assertion is
// what caught it.
function pageField(idx, w, h) {
  const px = new Uint8Array(w * h).fill(238); // paper
  paintBar(px, w, h, 0.05, 0.12, 0.10, 0.45 + 0.12 * (idx % 4), 40); // heading
  for (let c = 0; c < 9; c++) {
    // **The multiplier must be coprime with the modulus.** A first draft used `idx * 7 % 7`,
    // which is zero for every page — so every page got identical columns, and three separate
    // "different pages align" results were this one slip rather than anything about the hash.
    const g = 60 + ((idx * 3 + c * 5 + (c % 2) * 3) % 7) * 26; // 60..216, page-specific
    paintBar(px, w, h, 0.20, 0.86, c / 9 + 0.005, (c + 1) / 9 - 0.005, g);
  }
  return px;
}

// degrade applies what a scanner does. `ramp` is the one that matters: an illumination gradient
// biases EVERY left-to-right comparison in the same direction, and a dHash is nothing but 64
// left-to-right comparisons. Its SIGN decides everything, because the comparison is strict.
function degrade(src, w, h, { ramp = 0, shiftX = 0, shiftY = 0, noise = 0, invert = false, seed = 7 } = {}) {
  const rnd = mulberry32(seed);
  const out = new Uint8Array(w * h);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const sx = Math.min(w - 1, Math.max(0, x - shiftX));
      const sy = Math.min(h - 1, Math.max(0, y - shiftY));
      let v = src[sy * w + sx];
      if (invert) v = 255 - v;
      if (ramp) v *= 1 - ramp / 2 + ramp * (x / (w - 1));
      if (noise) v += (rnd() * 2 - 1) * noise;
      out[y * w + x] = Math.max(0, Math.min(255, Math.round(v)));
    }
  }
  return out;
}

// makeScanPDF renders `pages` (an array of page INDEXES, so a test can delete, insert or reorder)
// as full-page greyscale images with no text of any kind.
export function makeScanPDF(pages = [0, 1, 2, 3], deg = {}, { w = 170, h = 220 } = {}) {
  const objs = [];
  const kids = [];
  pages.forEach((idx, i) => {
    const pageObj = 3 + i * 3;
    kids.push(`${pageObj} 0 R`);
    const img = zlib.deflateSync(Buffer.from(degrade(pageField(idx, w, h), w, h, deg)));
    const content = 'q 612 0 0 792 0 0 cm /Im0 Do Q';
    objs[pageObj - 1] =
      `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents ${pageObj + 1} 0 R ` +
      `/Resources << /XObject << /Im0 ${pageObj + 2} 0 R >> >> >>`;
    objs[pageObj] = `<< /Length ${content.length} >>\nstream\n${content}\nendstream`;
    objs[pageObj + 1] =
      `<< /Type /XObject /Subtype /Image /Width ${w} /Height ${h} /ColorSpace /DeviceGray ` +
      `/BitsPerComponent 8 /Filter /FlateDecode /Length ${img.length} >>\nstream\n` +
      img.toString('latin1') + '\nendstream';
  });
  objs[0] = '<< /Type /Catalog /Pages 2 0 R >>';
  objs[1] = `<< /Type /Pages /Kids [${kids.join(' ')}] /Count ${pages.length} >>`;

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
