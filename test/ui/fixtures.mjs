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
export function makePDF({ pages = 3, label = 'page' } = {}) {
  const objs = [];
  const add = (s) => objs.push(s);
  const kids = [];
  for (let i = 0; i < pages; i++) kids.push(`${3 + i * 2} 0 R`);

  add('<< /Type /Catalog /Pages 2 0 R >>');
  add(`<< /Type /Pages /Kids [${kids.join(' ')}] /Count ${pages} >>`);
  const fontObj = 3 + pages * 2;
  for (let i = 0; i < pages; i++) {
    add(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents ${4 + i * 2} 0 R /Resources << /Font << /F1 ${fontObj} 0 R >> >> >>`);
    // Distinct text per page, so "a different document opened" is observable
    // rather than asserted.
    const content = `BT /F1 36 Tf 72 700 Td (${label} ${i + 1} of ${pages}) Tj ET`;
    add(`<< /Length ${content.length} >>\nstream\n${content}\nendstream`);
  }
  add('<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>');

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
