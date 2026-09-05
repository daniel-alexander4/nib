// /pending 333, end to end: the file underneath an open document is rewritten, and the
// user is told rather than shown the copy Nib opened forever.
//
// This is the only tier that can reproduce the reported symptom at all. The report was
// "Nib didn't show the updated PDF and a hard reload didn't update it", and both halves
// need the real pieces: a real file on disk that a real other process rewrites, a real
// Go server holding the bytes it read at open, and a real browser whose reload genuinely
// re-fetches. Tier 1 has the server and no client; tier 2 has the client and a stubbed
// server, so a field the server never actually sets would leave every tier-2 assertion
// green. The seam is the whole defect, and this is where it is joined.
//
// The file is rewritten with `fs`, not through the app — asking the app to change the
// file would be asking the thing under test to create its own stimulus.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('diskchanged.pdf', { pages: 2, label: 'disk page' });

const bannerText = () => page.evaluate(() => {
  const b = document.getElementById('staleBanner');
  return b && !b.hidden ? document.getElementById('staleMsg').textContent : null;
});

// Rewrites the file the way another program would: a whole new file in place, longer
// than before. Returns the bytes so the save assertion can prove they survived.
function rewriteOnDisk() {
  const orig = fs.readFileSync(DOC);
  const updated = Buffer.concat([orig, Buffer.from('\n% rewritten by another program\n')]);
  fs.writeFileSync(DOC, updated);
  return updated;
}

// rewriteWithPages replaces the file with a DIFFERENT PAGE COUNT. That is the stimulus a
// content-identical append cannot be: an appended comment changes the bytes and nothing on
// screen, so a test using it can only ever assert that the server noticed. The page count
// is a fact the RENDER carries, so this is the one assertion that says the pixels moved —
// which is the whole reason this tier exists.
function rewriteWithPages(n) {
  writeFixture('diskchanged.pdf', { pages: n, label: 'reloaded page' });
  return fs.readFileSync(DOC);
}

const pageCount = () => page.evaluate(() => {
  const el = document.querySelector('.viewerContainer:not([hidden])')
    ? document.querySelector('.pageCount') : null;
  return el ? el.textContent.trim() : null;
});

test('a clean document whose file changed is reloaded on return-to-foreground', async () => {
  await h.openDocument(DOC, 2);
  assert.equal(await bannerText(), null,
    'setup: a banner is already up before anything changed, so nothing below is about a disk change');
  assert.match(await pageCount(), /\/\s*2$/,
    'setup: the open document is not the 2-page fixture, so a change to 5 below proves nothing');

  // Move off page 1 before the change. Until /pending 372 every in-place reload — the twenty
  // page operations, undo, redo, OCR, and this one — returned the reader to the top: measured
  // scrollTop 1363 before a reload and 25 after.
  await page.click('#nextBtn');
  await page.waitForFunction(
    () => (document.querySelector('.viewerContainer:not([hidden])')?.scrollTop ?? 0) > 100,
    null, { timeout: 10000 });
  const scrolledTo = await page.evaluate(() =>
    Math.round(document.querySelector('.viewerContainer:not([hidden])').scrollTop));

  const external = rewriteWithPages(5);
  const tabsBefore = await page.evaluate(() => document.querySelectorAll('.viewerContainer').length);

  // The real sequence: the rewrite happens while Nib is in the background, because the
  // user is in a terminal or another application when they do it.
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await page.waitForFunction(
    () => /\/\s*5$/.test(document.querySelector('.pageCount')?.textContent ?? ''),
    null, { timeout: 20000 },
  ).catch(() => {});

  assert.match(await pageCount(), /\/\s*5$/,
    'the user came back to a document with no unsaved work whose file had changed, and the pages on screen are still the ones read at open — the reload either never fired or never reached the render');
  assert.equal(await bannerText(), null,
    'the document was reloaded and the banner is still up, describing a state that no longer exists');

  // The old Reload went through /api/open, so it built a SECOND view on the same path and
  // closed the first — which reported sameFileOpen every time and moved the user's document
  // to the end of the tab strip. Doing that silently, on a focus event, is the thing this
  // assertion exists to stop coming back.
  assert.equal(await page.evaluate(() => document.querySelectorAll('.viewerContainer').length), tabsBefore,
    'the automatic reload changed the number of open views — it opened a second copy rather than re-reading in place');

  // She is still where she was reading. The page NUMBER, not the offset: the document grew
  // from two pages to five, so the same scroll position is a different place in it.
  assert.equal(await page.evaluate(() => document.querySelector('.pageNum')?.value), '2',
    'the automatic reload put the reader back on page 1 of a document she was reading at page 2');
  // Waited for, not sampled once: the restore is deliberately asynchronous — it runs on
  // `pagesloaded`, after the width fit — so an immediate read races the render. It still fails
  // if the restore never happens, which is the case this assertion is for.
  await page.waitForFunction(
    () => (document.querySelector('.viewerContainer:not([hidden])')?.scrollTop ?? 0) > 100,
    null, { timeout: 10000 }).catch(() => {});
  assert.ok(await page.evaluate(() => (document.querySelector('.viewerContainer:not([hidden])')?.scrollTop ?? 0) > 100),
    `the counter says page 2 but the view is at the top (it was scrolled to ${scrolledTo} before the reload) — the number was updated without the scroll, which is what a restore that never reached pdf.js looks like`);

});

test('Save will not silently overwrite the changed file', async () => {
  // Its own stimulus, and deliberately WITHOUT a focus event: the automatic reload fires on
  // return-to-foreground, so a test that raised one would refresh the document and find the
  // refusal below unreachable. This is the user who changed the file and went straight back
  // to Nib's Save button without the window ever losing focus.
  const external = rewriteOnDisk();

  // Setup, and it separates the two ways this can fail: if the server no longer thinks
  // the file changed, the refusal was never reachable and the assertion below would be
  // reporting the wrong defect.
  // Unpinned deliberately: exactly one document is open here, so the compatibility
  // fallback resolves to it, and the app exposes no id to read from the page.
  const stillChanged = await page.evaluate(async () => {
    const r = await fetch('/api/doc');
    return (await r.json()).diskChanged === true;
  });
  assert.ok(stillChanged,
    'setup: the server no longer reports the file as changed, so the save below was never going to be refused and this test would be measuring nothing');

  // The harness owns dialogs and defaults to ACCEPTING them (see answerDialogs), so a
  // local page.once handler races it and loses. Said here because a test that quietly
  // accepted the overwrite prompt would assert the opposite of its own name.
  const before = h.dialogs.length;
  h.answerDialogs(false);
  await h.mode('file');
  await page.click('#saveBtn');
  // The refusal is server-side and the prompt is client-side; give both a beat to settle
  // rather than waiting on a toast the dismissed path deliberately does not raise.
  await page.waitForTimeout(1500);

  assert.ok(h.dialogs.length > before,
    'no overwrite prompt was raised at all — the user was never asked, so "she declined" is not what the assertion below would be measuring');
  assert.ok(fs.readFileSync(DOC).equals(external),
    'Save overwrote the changed file even though the user declined the overwrite prompt — this is the data loss the stale render costs, and it is the half the banner alone does not prevent');

  // The other direction, and it is not a nicety: a refusal with no way past it would
  // strand the user's unsaved edits behind the warning meant to protect them. Saying yes
  // must actually write.
  h.answerDialogs(true);
  await page.click('#saveBtn');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved',
    null, { timeout: 15000 });
  assert.ok(!fs.readFileSync(DOC).equals(external),
    'the user accepted the overwrite and the file on disk is unchanged — the override is inert, so the refusal is a wall rather than a default');
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  h.answerDialogs(true); // the close may prompt about unsaved work
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
