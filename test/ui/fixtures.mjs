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
// **Vertical structure is forced by what the hash measures.** Every comparison is a cell against
// the cell to its RIGHT, so a page of horizontal text bars on flat paper produces almost nothing
// but equal pairs. A first draft built that way could not discriminate its own pages, and its own
// setup assertion is what caught it.
function pageField(idx, w, h) {
  // **The greys come from a per-page PRNG, and that is the whole reason.** Two closed-form
  // drafts both aliased and both produced "different pages align" results that were the fixture,
  // not the hash: `idx * 7 % 7` is zero for EVERY page, and its replacement
  // `(idx * 3 + c * 5 + …) % 7` has period 7 in `idx`, so pages 0 and 7 were measured 4 bits
  // apart — a different page reading as the same one. A closed form over a small modulus keeps
  // finding new ways to collide; seeding on the index has no period to find.
  // `pagesDiffer` below is the guard, because a third slip of this shape is likelier than not.
  const rnd = mulberry32(0x9E37 + idx * 2654435761);
  const px = new Uint8Array(w * h).fill(238); // paper
  paintBar(px, w, h, 0.05, 0.12, 0.10, 0.35 + 0.35 * rnd(), 40); // heading
  for (let c = 0; c < 9; c++) {
    const g = 60 + Math.floor(rnd() * 7) * 26; // 60..216, page-specific
    paintBar(px, w, h, 0.20, 0.86, c / 9 + 0.005, (c + 1) / 9 - 0.005, g);
  }
  return px;
}

// sparseField paints what most scanned documents actually look like: paper, a heading, and a few
// lines of text-like ink. It is deliberately NOT `pageField` — that one paints nine full-height
// columns of strongly differing greys, which is generous to a perceptual hash in a way a real page
// is not. Two `sparseField` pages differ only in where their lines end, and that is the case where
// a 9x8 reduction has the least to work with. It exists to hold the hash's LIMIT in place, not to
// flatter it.
export function sparseField(idx, w, h) {
  const px = new Uint8Array(w * h).fill(238); // paper
  paintBar(px, w, h, 0.08, 0.12, 0.12, 0.55 + 0.05 * idx, 40); // heading
  // A PRNG seeded per page, NOT a closed form over a small modulus. `(idx*3 + i*5) % 7`
  // has period 7 in idx, so page 7's eight body lines were byte-identical to page 0's and
  // page 8's to page 1's — and every page was a cyclic ROTATION of one multiset of eight
  // lengths, which also makes the column ink profile identical across the whole family.
  // Measured on the shipped hash: sp0 vs sp7 was 3 bits, a "different page" this family
  // rated as closer than any same-page degradation. That is the THIRD slip of the shape
  // the comment above predicts, in the one fixture whose job is to hold the hash's limit.
  const rnd = mulberry32(1000 + idx * 97);
  for (let i = 0; i < 8; i++) {
    const end = 0.55 + 0.32 * rnd(); // ragged right margin
    paintBar(px, w, h, 0.20 + i * 0.07, 0.225 + i * 0.07, 0.12, end, 70);
  }
  return px;
}

// pagesDiffer reports how many of the 72 grid cells two page indexes disagree on, reduced the
// way the hash reduces. It exists so a test can assert its OWN fixture distinguishes the pages
// it is about to claim the hash distinguishes — the check both aliasing drafts above needed.
// **`field` is a parameter because hard-wiring it is what let the third aliasing slip
// through.** This guard was written for exactly that class and then pointed only at
// `pageField`, so `sparseField` — the fixture most in need of it, being the sparse one —
// was never checked by it at all. A guard that covers one of two populations reports a
// safety it did not establish for the other.
export function pagesDiffer(a, b, { w = 170, h = 220, field = pageField } = {}) {
  const cells = (idx) => {
    const src = field(idx, w, h), out = [];
    for (let gy = 0; gy < 8; gy++) {
      for (let gx = 0; gx < 9; gx++) {
        const x0 = Math.floor((gx * w) / 9), x1 = Math.floor(((gx + 1) * w) / 9);
        const y0 = Math.floor((gy * h) / 8), y1 = Math.floor(((gy + 1) * h) / 8);
        let s = 0, n = 0;
        for (let y = y0; y < y1; y++) for (let x = x0; x < x1; x++) { s += src[y * w + x]; n++; }
        out.push(Math.round(s / n));
      }
    }
    return out;
  };
  const p = cells(a), q = cells(b);
  return p.reduce((n, v, i) => n + (Math.abs(v - q[i]) > 4 ? 1 : 0), 0);
}

// degrade applies what a scanner does. `ramp` is the one that matters: an illumination gradient
// biases EVERY left-to-right comparison in the same direction, and the page hash is nothing but 64
// left-to-right comparisons. Under a STRICT `>` its sign decided everything — a brightening ramp
// moved nothing and a darkening one flipped every tied pair. That is what `pageDHash` was rewritten
// to survive (/pending 276), and `ramp` in both signs is the fixture that holds it to it.
function degrade(src, w, h, { ramp = 0, shiftX = 0, shiftY = 0, noise = 0, invert = false, rot = 0, contrast = 1, seed = 7 } = {}) {
  const rnd = mulberry32(seed);
  const out = new Uint8Array(w * h);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      // Rotation about the page centre — real scans skew, and it was the one degradation
      // this fixture could not express. Nearest-neighbour is enough: the hash reads a 9x8
      // reduction, which destroys any interpolation difference long before it compares.
      let fx = x - shiftX, fy = y - shiftY;
      if (rot) {
        const a = (rot * Math.PI) / 180, cx = (w - 1) / 2, cy = (h - 1) / 2;
        const dx = fx - cx, dy = fy - cy;
        fx = cx + dx * Math.cos(a) + dy * Math.sin(a);
        fy = cy - dx * Math.sin(a) + dy * Math.cos(a);
      }
      const sx = Math.min(w - 1, Math.max(0, Math.round(fx)));
      const sy = Math.min(h - 1, Math.max(0, Math.round(fy)));
      let v = src[sy * w + sx];
      // contrast < 1 is a pale scan, pulled toward mid-grey. A FIXED comparison band dies
      // here — every real difference falls inside it — which is why the band adapts.
      if (contrast !== 1) v = 128 + (v - 128) * contrast;
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
export function makeScanPDF(pages = [0, 1, 2, 3], deg = {}, { w = 170, h = 220, field = pageField } = {}) {
  const objs = [];
  const kids = [];
  pages.forEach((idx, i) => {
    const pageObj = 3 + i * 3;
    kids.push(`${pageObj} 0 R`);
    const img = zlib.deflateSync(Buffer.from(degrade(field(idx, w, h), w, h, deg)));
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
