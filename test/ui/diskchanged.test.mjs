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

test('a file rewritten under an open document is reported, and a browser reload does not clear it', async () => {
  await h.openDocument(DOC, 2);
  assert.equal(await bannerText(), null,
    'setup: a banner is already up before anything changed, so nothing below is about a disk change');

  const external = rewriteOnDisk();

  // The stimulus is complete; now the user comes back to the window. This is the real
  // sequence — the rewrite happens while Nib is in the background, because the user is
  // in a terminal or another application when they do it.
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await page.waitForFunction(
    () => { const b = document.getElementById('staleBanner'); return b && !b.hidden; },
    null, { timeout: 15000 },
  ).catch(() => {});

  const text = await bannerText();
  assert.ok(text && /changed on disk/i.test(text),
    `the file was rewritten under the open document and the app says nothing (banner: ${JSON.stringify(text)}). This is /pending 333 exactly: the bytes on screen are the ones read at open, and nothing tells the user they are looking at a copy`);

  // **The second half of the report, and the reason a fix that only reported was not
  // enough.** A browser reload re-fetches from the same in-memory copy, so it cannot
  // clear this and must not appear to: after a full reload the banner is still up,
  // because the file is still different.
  await page.reload({ waitUntil: 'load' });
  await page.waitForFunction(
    () => { const b = document.getElementById('staleBanner'); return b && !b.hidden; },
    null, { timeout: 20000 },
  ).catch(() => {});
  assert.ok(/changed on disk/i.test(await bannerText() ?? ''),
    'after a full browser reload the warning is gone while the file is still different — the reload re-fetched the same in-memory bytes and cleared the one thing that said so, which is worse than the original bug');

  // And the file is untouched by all of this: reporting must not write.
  assert.ok(fs.readFileSync(DOC).equals(external),
    'the file on disk changed while the app was merely reporting on it');
});

test('Save will not silently overwrite the changed file', async () => {
  // Runs against the state the test above left: DOC rewritten, document still open and
  // holding the bytes from before the rewrite.
  const external = fs.readFileSync(DOC);

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
