// /pending 929 — a contents page, end to end in a real browser: a bookmark is added in the outline editor and saved,
// then Contents page… inserts a page in front of the document and the document grows by one. The page's CONTENT is
// internal/pdfops's (text read back through the page map); this holds the path a user takes to it.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('contents.pdf', { pages: 3, label: 'contents page' });
const pageCount = () => page.$eval('.pageCount', (el) => el.textContent);

test('with a bookmark, Contents page… adds a page in front; without one it says there is nothing to list', async () => {
  await h.openDocument(DOC, 3);
  // Nothing to list yet: the server refuses, and its sentence reaches the user.
  await h.mode('edit');
  await h.group('Size & Number Pages');
  await page.click('#contentsBtn');
  await page.waitForFunction(() => !document.getElementById('contentsModal').hidden);
  await page.click('#contentsGo');
  await page.waitForFunction(() => /no bookmarks and no headings/.test(document.getElementById('toast')?.textContent ?? ''), null, { timeout: 15000 })
    .catch(() => assert.fail('a document with nothing to list was given a contents page, or the refusal did not reach the user'));
  assert.equal(await pageCount(), '/ 3', 'the document changed despite the refusal');
  assert.equal(await page.$eval('#contentsModal', (el) => el.hidden), false, 'the dialog closed on a refusal');

  // A bookmark through the outline editor (the Jump to Section card is the File tab's).
  await page.click('#contentsCancel');
  await h.mode('file');
  await h.mode('edit');
  await h.mode('file');
  await h.panel('outline');
  await page.click('.outline-edit');
  await page.waitForFunction(() => !document.getElementById('outlineModal').hidden);
  await page.click('#outlineAddBtn');
  await page.click('#outlineSave');
  await page.waitForFunction(() => document.getElementById('outlineModal').hidden, null, { timeout: 15000 });

  await h.mode('edit');
  await h.group('Size & Number Pages');
  await page.click('#contentsBtn');
  await page.waitForFunction(() => !document.getElementById('contentsModal').hidden);
  await page.click('#contentsGo');
  await page.waitForFunction(() => document.getElementById('contentsModal').hidden, null, { timeout: 20000 })
    .catch(() => assert.fail('the contents page was not inserted'));
  await page.waitForFunction(() => document.querySelector('.pageCount')?.textContent === '/ 4', null, { timeout: 15000 })
    .catch(async () => assert.fail(`the document has ${await pageCount()} after a contents page, want / 4`));
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeDocument();
});
