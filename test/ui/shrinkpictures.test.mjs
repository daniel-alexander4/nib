// /pending 908 — Reduce → Shrink pictures against the real server. The fixture has no pictures, so the honest answer
// is "nothing to shrink", and getting it proves the whole path: the dialog's mode, the real /api/shrink-pictures, the
// X-Nib-Shrink header read by a real fetch, and the sentence the window builds from it. (A document WITH pictures
// is internal/server's and internal/pdfops's: the harness has no JPEG to embed.)
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('shrinkpictures.pdf', { pages: 1, label: 'no pictures here' });

test('Shrink pictures on a document without pictures says there is nothing to shrink', async () => {
  await h.openDocument(DOC, 1);
  await h.card('Export & Print').catch(() => {});
  await page.evaluate(() => document.getElementById('reduceBtn').click());
  await page.waitForFunction(() => !document.getElementById('reduceModal').hidden);
  await page.click('input[name="reduceMode"][value="pictures"]');
  await page.waitForFunction(() => !document.getElementById('reducePictures').hidden);
  assert.equal(await page.$eval('#reduceQuality', (el) => el.hidden), true, 'Compress\'s quality showed beside Shrink pictures');

  await page.click('#reduceGo');
  await page.waitForFunction(() => !document.getElementById('reduceResult').hidden, null, { timeout: 15000 })
    .catch(() => assert.fail('Shrink pictures gave no answer'));
  const text = await page.$eval('#reduceResult', (el) => el.textContent);
  assert.match(text, /0 of 0 pictures shrunk/, `the result reads "${text}"`);
  assert.match(text, /Nothing here would get smaller by shrinking pictures/);
  await page.click('#reduceCancel');
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeDocument();
});
