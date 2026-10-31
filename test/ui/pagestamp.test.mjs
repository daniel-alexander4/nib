// /pending 907 — "Stamp every page…": a line of text, a diagonal watermark or a picture on every page or the
// pages named. Tier 3 because the dialog's rows are shown and hidden by the chosen kind, the picture is a real file
// input, and what the user is told afterwards (the double-stamp note) is read from the server's own answer.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('pagestamp.pdf', { pages: 3, label: 'stamp page' });
// A 1×1 PNG: the smallest picture the server will draw.
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');
const hidden = (id) => page.$eval(`#${id}`, (el) => el.hidden);
const toastText = () => page.evaluate(() => document.getElementById('toast')?.textContent);

test('the dialog shows the controls its kind reads, and stamps text, then a picture, onto the named pages', async () => {
  await h.openDocument(DOC, 3);
  await h.mode('edit');
  await h.group('Size & Number Pages');
  await page.click('#pageStampBtn');
  await page.waitForFunction(() => !document.getElementById('pageStampModal').hidden);

  // The stimulus: nothing has been stamped, so the note must be absent — or "it warns after" is true of a
  // note that is always on — and the text kind shows its words and hides the picture.
  await page.waitForFunction(() => document.getElementById('pstStamped').hidden === true);
  assert.equal(await hidden('pstTextRow'), false);
  assert.equal(await hidden('pstImageRow'), true);

  // Nothing typed: refused in the dialog, which stays open.
  await page.click('#pstGo');
  assert.equal(await hidden('pstWhy'), false, 'an empty stamp was sent');
  assert.equal(await hidden('pageStampModal'), false);

  await page.fill('#pstText', 'Confidential');
  await page.fill('#pstPages', '1');
  await page.click('#pstGo');
  await page.waitForFunction(() => document.getElementById('pageStampModal').hidden, null, { timeout: 15000 });
  assert.equal(await toastText(), 'Stamped');

  // It is a stamp now, and the dialog says so on the next opening.
  await page.click('#pageStampBtn');
  await page.waitForFunction(() => document.getElementById('pstStamped').hidden === false, null, { timeout: 10000 })
    .catch(() => assert.fail('the dialog does not warn on a document it has already stamped'));

  // The picture kind: its own rows, and no file is a refusal that keeps the dialog open.
  await page.selectOption('#pstKind', 'image');
  assert.equal(await hidden('pstTextRow'), true);
  assert.equal(await hidden('pstImageRow'), false);
  assert.equal(await hidden('pstWidthRow'), false);
  await page.fill('#pstPages', '');
  await page.click('#pstGo');
  assert.equal(await hidden('pstWhy'), false, 'a picture stamp with no picture was sent');

  await page.setInputFiles('#pstImage', { name: 'logo.png', mimeType: 'image/png', buffer: PNG });
  await page.selectOption('#pstPosition', 'tr');
  await page.click('#pstGo');
  await page.waitForFunction(() => document.getElementById('pageStampModal').hidden, null, { timeout: 15000 })
    .catch(() => assert.fail('the picture stamp was not accepted'));
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
});
