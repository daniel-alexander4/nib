// Settings → Colours: the sidebar's cards as ONE hue at stepped tints, and the choice surviving.
//
// Tier 2 measures the ladder's contrast from the stylesheet and compares the three lists that
// describe the hue set. What it cannot do is resolve `color-mix()` — the rendered colour of a card
// is the browser's arithmetic over a per-theme token, and no source scan reaches it. So this
// asserts the two things only a real browser can: that choosing a hue actually REPAINTS the cards
// into a monotone ladder, and that the choice comes back after a reload, which is the vault
// round-trip through /api/settings and /api/status.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const cards = () => page.evaluate(() => {
  const vis = (e) => { const b = e.getBoundingClientRect(); return b.width > 0 && b.height > 0; };
  return [...document.querySelectorAll('#sbFunctions .sbhead')].filter(vis)
    .map((e) => ({ step: e.dataset.step, bg: getComputedStyle(e).backgroundColor }));
});

const pick = async (value) => {
  await h.mode('settings');
  await h.card('Colours');
  await page.click(`input[name="cardhue"][value="${value}"]`);
  await page.waitForFunction((v) => (document.documentElement.dataset.cardhue || 'all') === v, value);
};

test('choosing a hue repaints every card into one colour', async () => {
  await h.mode('edit');   // the mode with the most cards, so a ladder is visible
  const rainbow = await cards();
  assert.ok(rainbow.length >= 6,
    `only ${rainbow.length} cards are showing — a ladder of six needs six to be observable`);
  // The setup has to establish the ROTATION, not merely "several colours" — a single-hue ladder
  // also has six distinct shades, so counting them cannot tell the two apart. Found the hard way:
  // a server whose vault already held a hue produced a six-shade "before", and the assertion that
  // picking a hue changes the screen then failed for a reason that had nothing to do with the code.
  const dominant = (bg) => { const [r, g, b] = bg.match(/[\d.]+/g).slice(0, 3).map(Number);
    return r >= g && r >= b ? 'r' : g >= b ? 'g' : 'b'; };
  assert.ok(new Set(rainbow.map((c) => dominant(c.bg))).size > 1,
    `every card in the default sidebar leans the same way (${rainbow.map((c) => dominant(c.bg)).join('')}), so this server is already on a single hue and "picking one changes the screen" cannot be observed from here`);

  await pick('blue');
  await h.mode('edit');
  const single = await cards();

  // Every card is now the SAME hue at a different strength. Rendered colours are what is compared,
  // because that is the thing `color-mix` decides and the stylesheet only describes.
  const bySt = new Map();
  for (const c of single) if (!bySt.has(c.step)) bySt.set(c.step, c.bg);
  assert.ok(bySt.size >= 5,
    `the single-hue sidebar shows ${bySt.size} distinct steps — the ladder collapsed, so adjacent cards are indistinguishable`);
  assert.notDeepEqual(single.map((c) => c.bg), rainbow.map((c) => c.bg),
    'picking a hue changed nothing on screen');

  // …and it is ONE hue: for blue, every card's blue channel is its largest.
  const chan = (s) => s.match(/[\d.]+/g).slice(0, 3).map(Number);
  for (const c of single) {
    const [r, g, b] = chan(c.bg);
    assert.ok(b > r && b > g,
      `a card painted ${c.bg} is not predominantly blue after choosing the blue scheme — the hue is not reaching the card`);
  }
});

test('the chosen hue survives a reload', async () => {
  await page.reload();
  await page.waitForFunction(() => document.documentElement.dataset.cardhue === 'blue', null, { timeout: 15000 });
  const checked = await page.evaluate(() => document.querySelector('input[name="cardhue"]:checked')?.value);
  assert.equal(checked, 'blue',
    'the Colours picker came back on a different option than the one saved — the radio is reading something other than the stored setting');
});

test('this file leaves the shared server as it found it', async () => {
  // Back to the rotation: the next file in this tier inherits this server, and a sidebar stuck on
  // one hue would silently change what every other colour assertion is looking at.
  await pick('all');
  assert.equal(await page.evaluate(() => document.documentElement.dataset.cardhue), undefined,
    'the rotation did not come back — `all` is the ABSENCE of the attribute, and a leftover value follows this server into the next file');
});
