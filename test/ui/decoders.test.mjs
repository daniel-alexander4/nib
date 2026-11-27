// A JBIG2, CCITT fax or JPEG 2000 picture is drawn, and so is the text of a copier scan whose words are
// CCITT stencil masks (/pending 984).
//
// pdf.js decodes those three encodings through modules its worker loads at run time from `wasmUrl`
// (`jbig2.wasm`, `openjpeg.wasm`, or their `*_nowasm_fallback.js` twins); CCITT goes through the JBIG2 one.
// With `wasmUrl` unset both loads fail on a URL beginning "null", and the worker drops the picture with a
// console warning: the page draws, the picture is not on it, and nothing tells the user. A Canon copier
// scan opened that way showed its background and none of its words.
//
// The observable is the viewer's OWN canvas — the pages `PDFJS_OPTS` drew — not a getDocument this file
// makes, which would test options of its own choosing. Each page of the fixture (build/decoderfixture.py,
// committed as test/ui/decoders.pdf) draws one picture in the same rectangle; page 1 is a baseline JPEG,
// which pdf.js decodes with no module, so it is drawn whatever `wasmUrl` says and is the control. A
// dropped picture leaves the rectangle white (page 5: the pale background, which has no dark pixel).
//
// jsdom cannot see this: tier 2's pdf.js is a stub, and the decode is the worker's.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { launch, shutdown } from './harness.mjs';

const DOC = path.join(path.dirname(fileURLToPath(import.meta.url)), 'decoders.pdf');
const PAGES = ['JPEG (control)', 'JBIG2', 'CCITT fax', 'JPEG 2000', 'CCITT stencil over a background'];

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const decoderLoads = [];
page.on('response', (r) => {
  const u = new URL(r.url());
  if (u.pathname.includes('/vendor/pdfjs/') && /wasm/.test(u.pathname)) decoderLoads.push(`${r.status()} ${u.pathname}`);
});
page.on('requestfailed', (r) => { if (/wasm/.test(r.url())) decoderLoads.push(`failed ${new URL(r.url()).pathname}`); });

// The dark and light pixels inside the picture's rectangle (56, 56 from the top-left, 500 x 300 pt on a
// 612 x 792 page), on page n's canvas as the viewer drew it, two points in from each edge so that
// anti-aliasing at the border counts for neither side.
const inkOn = (n) => page.evaluate(async (n) => {
  const div = document.querySelector(`.viewerContainer:not([hidden]) .page[data-page-number="${n}"]`);
  div.scrollIntoView();
  const t0 = Date.now();
  for (;;) {
    const cv = div.querySelector('canvas');
    if (cv && cv.width && div.hasAttribute('data-loaded') && !div.classList.contains('loadingIcon')) break;
    if (Date.now() - t0 > 20000) return { error: `page ${n} was not drawn in 20 s` };
    await new Promise((r) => setTimeout(r, 50));
  }
  const cv = div.querySelector('canvas');
  const sx = cv.width / 612, sy = cv.height / 792;
  const x0 = Math.ceil((56 + 2) * sx), x1 = Math.floor((556 - 2) * sx);
  const y0 = Math.ceil((56 + 2) * sy), y1 = Math.floor((356 - 2) * sy);
  const d = cv.getContext('2d').getImageData(x0, y0, x1 - x0, y1 - y0).data;
  let dark = 0, light = 0;
  for (let i = 0; i < d.length; i += 4) {
    const lum = 0.299 * d[i] + 0.587 * d[i + 1] + 0.114 * d[i + 2];
    if (lum < 128) dark++; else light++;
  }
  return { dark, light, ms: Date.now() - t0 };
}, n);

test('every picture encoding pdf.js decodes through a module is drawn, as the JPEG control is', async () => {
  await page.evaluate(() => {
    window.__cspViolations = [];
    document.addEventListener('securitypolicyviolation', (e) => window.__cspViolations.push(`${e.violatedDirective} ${e.blockedURI}`));
  });
  await h.openDocument(DOC, PAGES.length);
  const got = [];
  for (let n = 1; n <= PAGES.length; n++) got.push(await inkOn(n));
  const report = got.map((g, i) => `${PAGES[i]}: ${g.error || `${g.dark} dark, ${g.light} light, ${g.ms} ms`}`).join('; ');
  console.log(`decoders: ${report}; loads: ${decoderLoads.join(', ') || 'none'}`);

  const control = got[0];
  assert.ok(!control.error && control.dark > 1000,
    `setup: the JPEG control drew ${control.dark} dark pixels in its rectangle (${report}), so nothing below can tell drawn from dropped`);
  for (let i = 1; i < PAGES.length; i++) {
    const g = got[i];
    assert.ok(!g.error, g.error);
    assert.ok(g.dark > control.dark * 0.85 && g.dark < control.dark * 1.15,
      `the ${PAGES[i]} picture drew ${g.dark} dark pixels where the JPEG control drew ${control.dark} (${report}) — ` +
      `pdf.js dropped it, which it does when its worker cannot load the decoder from wasmUrl (loads: ${decoderLoads.join(', ') || 'none'})`);
  }
  assert.deepEqual(await page.evaluate(() => window.__cspViolations), [], 'the decoders broke the page\'s content security policy');
  assert.deepEqual(decoderLoads.filter((l) => !l.startsWith('200 ')), [], 'a decoder the worker asked for was not served');
});

// The other half of `useWasm`'s default: a browser that refuses WebAssembly gets the JS twin, which the
// WORKER imports itself — so a relative `wasmUrl` resolves against the worker's URL and finds nothing,
// while the `.wasm`, which the page fetches for the worker, still loads. The first test cannot see that
// (measured: green with a relative `wasmUrl`); refusing the `.wasm` here is what reaches the import.
// A document is opened again so its worker is a new one, with no decoder already loaded.
test('with the .wasm decoders refused, their JS twins draw every picture', async () => {
  await h.closeAll();
  decoderLoads.length = 0;
  await page.route('**/vendor/pdfjs/wasm/*.wasm', (r) => r.abort());
  try {
    await h.openDocument(DOC, PAGES.length);
    const got = [];
    for (let n = 1; n <= PAGES.length; n++) got.push(await inkOn(n));
    const report = got.map((g, i) => `${PAGES[i]}: ${g.error || `${g.dark} dark, ${g.ms} ms`}`).join('; ');
    console.log(`decoders, JS path: ${report}; loads: ${decoderLoads.join(', ') || 'none'}`);
    assert.ok(decoderLoads.some((l) => /failed .*\.wasm$/.test(l)), `setup: no .wasm request was refused (loads: ${decoderLoads.join(', ') || 'none'}), so this is not the JS path`);
    for (let i = 1; i < PAGES.length; i++) {
      assert.ok(!got[i].error && got[i].dark > got[0].dark * 0.85 && got[i].dark < got[0].dark * 1.15,
        `with WebAssembly refused the ${PAGES[i]} picture drew ${got[i].dark} dark pixels where the control drew ${got[0].dark} (${report}) — ` +
        `the worker could not import the JS decoder (a wasmUrl that is not absolute resolves against the worker's URL; loads: ${decoderLoads.join(', ') || 'none'})`);
    }
    assert.deepEqual(decoderLoads.filter((l) => /_nowasm_fallback\.js$/.test(l) && !l.startsWith('200 ')), [], 'a JS decoder was not served');
  } finally {
    await page.unroute('**/vendor/pdfjs/wasm/*.wasm');
  }
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeAll();
});
