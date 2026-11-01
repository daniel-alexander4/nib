// /pending 910 — "Fill from headings" in the outline editor. The server's reading (the tagger's headings) is
// tested in Go; what only a browser shows is that the button fills the editor's LIST for review, writes nothing
// until Save, says so when there is nothing to propose, and asks before it replaces a list the user has.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('outlineheadings.pdf', { pages: 3, label: 'heading page' });
const rows = () => page.$$eval('#outlineEditList .outlinerow', (rs) => rs.map((r) => ({
  title: r.querySelector('.outline-title').value,
  page: r.querySelector('.outline-page').value,
  indent: parseInt(r.style.paddingLeft, 10) || 0,
})));

test('the button fills the list for review, nests it, and writes nothing until Save', async () => {
  await h.openDocument(DOC, 3);
  // The Jump to Section card is the File tab's; the sidebar's cards are settled by a mode CHANGE, and the
  // document opens on File already, so leave it and come back.
  await h.mode('edit');
  await h.mode('file');
  await h.panel('outline');
  await page.click('.outline-edit');
  await page.waitForFunction(() => !document.getElementById('outlineModal').hidden);
  assert.deepEqual(await rows(), [], 'setup: the fixture already has bookmarks, so a filled list proves nothing');

  await page.route('**/api/outline/propose*', (route) => route.fulfill({
    status: 200, contentType: 'application/json',
    body: JSON.stringify({ items: [
      { title: 'Chapter one', page: 1, level: 0 },
      { title: 'A section', page: 2, level: 1 },
      { title: 'Chapter two', page: 3, level: 0 },
    ] }),
  }));
  await page.click('#outlineFromHeadingsBtn');
  await page.waitForFunction(() => document.querySelectorAll('#outlineEditList .outlinerow').length === 3);
  const got = await rows();
  assert.deepEqual(got.map((r) => r.title), ['Chapter one', 'A section', 'Chapter two']);
  assert.ok(got[1].indent > got[0].indent && got[2].indent === got[0].indent, `the section is not nested under its chapter: ${JSON.stringify(got)}`);
  assert.equal(await page.$eval('#outlineWhy', (el) => el.hidden), false, 'the proposal was not announced');

  // Cancel leaves the document's outline as it was: nothing was written by filling the list.
  await page.click('#outlineCancel');
  await page.click('.outline-edit');
  await page.waitForFunction(() => !document.getElementById('outlineModal').hidden);
  assert.deepEqual(await rows(), [], 'filling the list wrote the outline before Save');
});

test('a document with no headings is told so, and the list is left alone', async () => {
  await page.route('**/api/outline/propose*', (route) => route.fulfill({
    status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }),
  }));
  await page.click('#outlineAddBtn');
  await page.click('#outlineFromHeadingsBtn');
  await page.waitForFunction(() => /No headings/.test(document.getElementById('outlineWhy').textContent));
  assert.equal((await rows()).length, 1, 'an empty proposal replaced the user\'s list');
});

test('this file leaves the shared server as it found it', async () => {
  await page.click('#outlineCancel');
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
});
