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
// The reload-icon test gets its OWN file. `DOC` is rewritten on disk by the tests above —
// `rewriteWithPages(5)` among them — so by the end of this file it is neither two pages nor the
// bytes `writeFixture` wrote, and a later test opening it would be opening something it did not
// describe.
const RELOAD_DOC = writeFixture('reloadicon.pdf', { pages: 2, label: 'reload page' });

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

// ── The toolbar's reload icon ────────────────────────────────────────────────
//
// The banner's Reload button only exists when the file has CHANGED underneath you. The icon
// beside Undo/Redo is the same act with no precondition: throw away what is in memory and
// re-read the file. Both go through `reloadDiscarding`, so this exercises the door rather than
// a second copy of it — and the icon is the caller the banner cannot cover, because reaching
// the banner requires an external rewrite and reaching the icon requires only unsaved work.
//
// Tier 3 for the same reason as everything else in this file: the assertion is that the bytes
// the SERVER holds went back to the file's, which needs a real server, a real file and a real
// re-render.
test('the reload icon discards unsaved work, and asks first', async () => {
  // The test above leaves its document OPEN, and re-opening a path Nib already holds is not a
  // fresh open — it is reported as the same file in another tab, so `openDocument` would wait
  // for a `has-doc` transition that never comes. Found exactly that way.
  if (await page.evaluate(() => document.getElementById('viewerWrap').className === 'has-doc')) {
    h.answerDialogs(true);
    await h.closeDocument();
  }
  await h.openDocument(RELOAD_DOC, 2);
  const aspect = () => page.evaluate(() => {
    const r = document.querySelector('.viewerContainer:not([hidden]) .page').getBoundingClientRect();
    return +(r.width / r.height).toFixed(3);
  });
  const onDisk = await aspect();

  // A real unsaved change, made through the app: every page rotated.
  await h.mode('edit');
  await h.group('Rotate All Pages');
  await page.click('#rotateRightBtn');
  // Null-guarded: a rotate tears the pages down and rebuilds them, so this predicate polls
  // across a window where `.page` does not exist. Without the guard it throws inside the poll
  // and the wait fails as a TypeError rather than as a timeout.
  await page.waitForFunction((was) => {
    const el = document.querySelector('.viewerContainer:not([hidden]) .page');
    if (!el) return false;
    const r = el.getBoundingClientRect();
    return +(r.width / r.height).toFixed(3) !== was;
  }, onDisk);
  const rotated = await aspect();
  assert.notEqual(rotated, onDisk, 'setup: nothing was changed, so there is nothing for a reload to discard');

  // CANCEL first — a discard the user did not agree to must not happen.
  h.dialogs.length = 0;
  h.answerDialogs(false);
  await page.click('#reloadBtn');
  await page.waitForTimeout(800);
  assert.equal(h.dialogs.length, 1,
    'the reload icon threw away unsaved work without asking. It is one click beside Undo, and the whole point of the confirm is that the click is easy to make by accident');
  assert.match(h.dialogs[0], /unsaved changes/);
  assert.equal(await aspect(), rotated, 'cancelling the confirm still reloaded — the answer is not being read');

  // Then accept, and the document goes back to what the file says.
  h.answerDialogs(true);
  await page.click('#reloadBtn');
  await page.waitForFunction((want) => {
    const el = document.querySelector('.viewerContainer:not([hidden]) .page');
    if (!el) return false;
    const r = el.getBoundingClientRect();
    return +(r.width / r.height).toFixed(3) === want;
  }, onDisk);
  assert.equal(await aspect(), onDisk, 'the reload did not restore the file on disk');

  // And the reload leaves nothing unsaved behind: the bytes now MATCH the file, so a close
  // must not prompt. This is the half `reloadFromDisk` clears `dirty` for, and it is asserted
  // here rather than assumed — a reload that left the document dirty would prompt on close
  // about work that no longer exists.
  h.dialogs.length = 0;
  h.answerDialogs(true);
  await h.closeDocument();
  assert.deepEqual(h.dialogs, [],
    'closing after a reload prompted about unsaved work. The reload replaced the bytes with the file\'s own, so there is nothing unsaved to lose');

  // Re-opened, because the close above is an ASSERTION and the file's last test is a cleanup
  // that closes what is open — it clicks #closeBtn, which is disabled with nothing open, and a
  // disabled button is a 30-second Playwright timeout rather than a failed assertion. Leaving
  // the server as this file's convention expects is part of the test, not tidiness.
  await h.openDocument(RELOAD_DOC, 2);
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
