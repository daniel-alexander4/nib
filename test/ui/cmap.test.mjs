// A CJK document encoded with a predefined CMap must actually read as text.
//
// pdf.js resolves `/Encoding /UniJIS-UCS2-H` — and the other ~150 named Adobe CMaps —
// from a DATA FILE it fetches at `cMapUrl`. The worker carries only the list of names it
// recognises; the tables themselves are not compiled in. Nib's five `getDocument()` calls
// set no `cMapUrl`, no `standardFontDataUrl`, and the vendored tree has neither directory
// — so on a Japanese, Chinese or Korean document produced by an office suite that names a
// standard Adobe face, the encoding cannot be resolved from anything the app has.
//
// The fixture has NO `/ToUnicode` map, deliberately (see makeCJKPDF). That is what makes
// this test about the CMap and not about anything else: with no ToUnicode there is no
// second route from those bytes to characters, so text comes out of the document only if
// the CMap data was found and used.
//
// ── Why the text layer is the observable ────────────────────────────────────
// `.textLayer` is what the user selects, copies and searches, and it is pdf.js's own
// extraction rendered into the DOM. Asserting on it means the check fails for the reason
// the user would notice, rather than on an internal the app could satisfy while the
// document still could not be read.
//
// jsdom cannot host this at all: the text layer is positioned from real glyph metrics,
// and tier 2's pdf.js is a stub that parses nothing.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { makeCJKPDF, writeRawFixture } from './fixtures.mjs';

const TEXT = '日本語';
const DOC = writeRawFixture('cjk-cmap.pdf', makeCJKPDF(TEXT));

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

test('a document using a predefined CJK CMap yields its text', async () => {
  await h.openDocument(DOC, 1);
  await h.topOfDocument();

  // The text layer is built after the canvas, so wait for it to exist before reading it —
  // an empty read taken too early would be indistinguishable from the defect.
  await page.waitForFunction(() => {
    const tl = document.querySelector('.viewerContainer:not([hidden]) .page .textLayer');
    return tl && tl.dataset.mainRotation !== undefined;
  }, null, { timeout: 20000 }).catch(() => {});

  const got = await page.$eval('.viewerContainer:not([hidden]) .page .textLayer',
    (el) => el.textContent.replace(/\s+/g, '')).catch(() => null);

  assert.notEqual(got, null,
    'page one rendered no text layer at all, so there is nothing to select or copy on a CJK document');
  assert.equal(got, TEXT,
    `the page reads as ${JSON.stringify(got)}, not ${JSON.stringify(TEXT)}. The document names /Encoding /UniJIS-UCS2-H and carries no /ToUnicode, so its bytes become characters only through the predefined CMap table — which pdf.js fetches from cMapUrl and which this build does not ship. Every CJK document from an office suite is unreadable, unsearchable and uncopyable in the same way.`);
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
