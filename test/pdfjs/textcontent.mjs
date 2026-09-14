// pdf.js's own text extraction, run in Node — the second reader in seam S7
// (`PLAN-accessibility.md` P08.S02, `PLAN-text-reflow.md` P03).
//
// Usage: node test/pdfjs/textcontent.mjs FILE.pdf
// Prints one JSON object: {version, pages: [{page, items: [{str, x, y, w, h, font, eol}]}]}.
//
// ── Why the polyfills ────────────────────────────────────────────────────────
// The vendored build (`web/vendor/pdfjs/pdf.min.mjs`) targets current browsers, and this machine's
// Node is 20. Measured, in order: `DOMMatrix` is not defined at import; then `Promise.withResolvers`,
// `Promise.try`, and `Uint8Array.prototype.toHex` are missing at `getDocument`. Each is filled with
// the smallest thing text extraction needs — the DOMMatrix stand-in does no arithmetic, because
// `getTextContent` returns raw text-space transforms and never renders. Anything that DID render
// through these stubs would be wrong, which is why nothing here renders.
//
// The same vendored file the app ships is loaded, not an npm copy: agreement with a different
// pdf.js would be agreement with a reader the user never runs.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

globalThis.DOMMatrix ??= class DOMMatrix {
  constructor(m) { [this.a, this.b, this.c, this.d, this.e, this.f] = Array.isArray(m) ? m : [1, 0, 0, 1, 0, 0]; }
  multiplySelf() { return this; }
  preMultiplySelf() { return this; }
  translate() { return this; }
  scale() { return this; }
  invertSelf() { return this; }
};
globalThis.Path2D ??= class Path2D {};
globalThis.ImageData ??= class ImageData {};
Promise.withResolvers ??= function withResolvers() {
  let resolve; let reject;
  const promise = new Promise((a, b) => { resolve = a; reject = b; });
  return { promise, resolve, reject };
};
Promise.try ??= function tryFn(fn, ...args) { return new Promise((resolve) => resolve(fn(...args))); };
Uint8Array.prototype.toHex ??= function toHex() { return Array.from(this, (b) => b.toString(16).padStart(2, '0')).join(''); };
Uint8Array.fromHex ??= function fromHex(s) {
  const u = new Uint8Array(s.length / 2);
  for (let i = 0; i < u.length; i++) u[i] = parseInt(s.substr(i * 2, 2), 16);
  return u;
};
Uint8Array.prototype.toBase64 ??= function toBase64() { return Buffer.from(this).toString('base64'); };
Uint8Array.fromBase64 ??= function fromBase64(s) { return new Uint8Array(Buffer.from(s, 'base64')); };
Math.sumPrecise ??= function sumPrecise(xs) { let s = 0; for (const x of xs) s += x; return s; };

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const vendor = path.join(repo, 'web', 'vendor', 'pdfjs');
const file = process.argv[2];
if (!file) {
  process.stderr.write('usage: node test/pdfjs/textcontent.mjs FILE.pdf\n');
  process.exit(2);
}

// pdf.js logs through console.log; stdout must carry only the JSON.
const log = console.log;
console.log = (...a) => process.stderr.write(a.join(' ') + '\n');

const pdfjs = await import(pathToFileURL(path.join(vendor, 'pdf.min.mjs')).href);
pdfjs.GlobalWorkerOptions.workerSrc = pathToFileURL(path.join(vendor, 'pdf.worker.min.mjs')).href;
const task = pdfjs.getDocument({
  data: new Uint8Array(fs.readFileSync(file)),
  cMapUrl: path.join(vendor, 'cmaps') + path.sep,
  cMapPacked: true,
  verbosity: 0,
});
const doc = await task.promise;

const pages = [];
for (let p = 1; p <= doc.numPages; p++) {
  const tc = await (await doc.getPage(p)).getTextContent();
  pages.push({
    page: p,
    items: tc.items.filter((it) => 'str' in it).map((it) => ({
      str: it.str,
      x: it.transform[4],
      y: it.transform[5],
      w: it.width,
      h: it.height,
      font: it.fontName,
      eol: it.hasEOL,
    })),
  });
}
log(JSON.stringify({ version: pdfjs.version, pages }));
// The LOADING TASK owns teardown in pdf.js 6; the document proxy has no destroy (measured).
await task.destroy();
