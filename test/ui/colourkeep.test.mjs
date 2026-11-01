// /pending 930 — against the real server: a pin and a tool's style reach the vault and come back through status. Tier 2
// holds the rules; only the real /api/settings and /api/status show that the vault keeps what the window sent.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('colourkeep.pdf', { pages: 1, label: 'keep page' });
const status = () => page.evaluate(async () => (await nibFetch('/api/status')).json());
const post = (body) => page.evaluate((b) => nibFetch('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(b) }), body);
const pick = (hex) => page.$eval('#hlCustom', (el, v) => { el.value = v; el.dispatchEvent(new Event('change', { bubbles: true })); }, hex);

test('a pinned colour and a Draw style are kept by the vault', async () => {
  const before = await status();
  assert.deepEqual(before.pinnedColors ?? [], [], 'setup: the vault already has pins');
  assert.equal(before.markupStyles?.INK, undefined, 'setup: the vault already has a Draw style');
  await h.openDocument(DOC, 1);
  await h.mode('markup');
  await h.group('Annotate & Draw');

  await page.click('#highlightToolBtn');
  await page.waitForFunction(() => !document.getElementById('hlColors').hidden);
  await pick('#123456');
  await page.click('#hlPin');
  await page.waitForFunction(() => document.querySelector('#hlSwatches .hlswatch.pinned') !== null);
  let st = await status();
  assert.deepEqual(st.pinnedColors, ['#123456'], `the vault holds ${JSON.stringify(st.pinnedColors)} after pinning`);
  assert.equal(await page.$eval('#hlSwatches .hlswatch', (b) => b.classList.contains('pinned')), true, 'the pinned colour does not lead the row');
  await page.click('#highlightToolBtn');

  await page.click('#drawToolBtn');
  await page.waitForFunction(() => !document.getElementById('inkOpts').hidden);
  await pick('#0000ff');
  await page.$eval('#inkThickness', (el) => { el.value = '6'; el.dispatchEvent(new Event('change', { bubbles: true })); });
  await page.waitForFunction(async () => (await (await nibFetch('/api/status')).json()).markupStyles?.INK?.thickness === 6, null, { timeout: 10000 })
    .catch(() => assert.fail('a Draw style was set and the vault did not keep it'));
  st = await status();
  assert.deepEqual(st.markupStyles.INK, { color: '#0000ff', thickness: 6 }, `the vault holds ${JSON.stringify(st.markupStyles.INK)} for Draw`);
  await page.click('#drawToolBtn');

  // Leave the shared vault as it was found.
  await post({ pinnedColors: [], markupStyles: {} });
  st = await status();
  assert.deepEqual(st.pinnedColors ?? [], []);
  assert.deepEqual(st.markupStyles ?? {}, {});
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeDocument();
});
