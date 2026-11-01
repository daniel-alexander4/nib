// /pending 917 — in a real pdf.js, a value typed into a form field is offered for Autofill at the next Save, kept
// only on "Keep selected", and then actually in the profile Autofill reads. Tier 2 holds the rules; only a browser
// shows that pdf.js's storage holds what a person typed.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('profileoffer.pdf', { pages: 1, label: 'form page', form: true });
const profile = () => page.evaluate(async () => (await nibFetch('/api/profile')).json());
const setProfile = (p) => page.evaluate((body) => nibFetch('/api/profile', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }), p);

test('a typed value is offered after Save, declined once, then kept when asked', async () => {
  const before = await profile();
  assert.equal(before.fill1, undefined, 'setup: the profile already holds fill1');
  await h.openDocument(DOC, 1);
  const input = '.viewerContainer:not([hidden]) .textWidgetAnnotation input, .viewerContainer:not([hidden]) .textWidgetAnnotation textarea';
  await page.waitForSelector(input);
  await page.click(input);
  await page.keyboard.type('Jane Doe');
  await page.keyboard.press('Tab');

  await page.click('#saveBtn');
  await page.waitForFunction(() => !document.getElementById('profileOfferModal').hidden, null, { timeout: 15000 })
    .catch(() => assert.fail('a value was typed into a form field and saved, and Nib did not offer to keep it'));
  const rows = await page.$$eval('#profileOfferList input[type=checkbox]', (bs) => bs.map((b) => `${b.dataset.name}=${b.dataset.value}`));
  assert.deepEqual(rows, ['fill1=Jane Doe']);
  assert.equal((await profile()).fill1, undefined, 'the profile was written before the user said yes');

  // "Not now": nothing kept, and the same value does not ask at the next Save.
  await page.click('#profileOfferCancel');
  assert.equal((await profile()).fill1, undefined);
  await page.click('#saveBtn');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved');
  assert.equal(await page.$eval('#profileOfferModal', (el) => el.hidden), true, 'a declined value asked again');

  // A new value is offered afresh, and "Keep selected" puts it where Autofill reads.
  await page.click(input, { clickCount: 3 });
  await page.keyboard.type('Jane Q Doe');
  await page.keyboard.press('Tab');
  await page.click('#saveBtn');
  await page.waitForFunction(() => !document.getElementById('profileOfferModal').hidden, null, { timeout: 15000 });
  await page.click('#profileOfferYes');
  await page.waitForFunction(() => document.getElementById('profileOfferModal').hidden);
  assert.equal((await profile()).fill1, 'Jane Q Doe', 'Keep selected did not reach the profile');
  await setProfile(before); // leave the shared vault as it was
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeDocument();
});
