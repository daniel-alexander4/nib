// /pending 913 — in a real pdf.js, a colour picked for Draw or Text is the colour drawn, Draw's thickness and
// opacity and Text's size reach the stroke and the text, and a border keeps a colour of its own beside the
// highlight's. Tier 2 (test/jsdom/toolcolours.test.mjs) holds which parameter is sent; only a browser shows
// what pdf.js DOES with it.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('toolcolours.pdf', { pages: 1, label: 'colour page' });
const pickColour = (hex) => page.$eval('#hlCustom', (el, v) => {
  el.value = v;
  el.dispatchEvent(new Event('change', { bubbles: true }));
}, hex);
const setNumber = (id, v) => page.$eval(`#${id}`, (el, x) => { el.value = x; el.dispatchEvent(new Event('change', { bubbles: true })); }, String(v));
const pageBox = async () => (await page.$('.viewerContainer:not([hidden]) .page')).boundingBox();

test('Text, Draw and a border are drawn in the colour, size, thickness and opacity chosen for them, and the highlight keeps its own', async () => {
  await h.openDocument(DOC, 1);
  await h.mode('markup');
  await h.group('Annotate & Draw');
  const box = await pageBox();

  // Highlight first, so its colour is on record before the others are chosen.
  await page.click('#highlightToolBtn');
  await page.waitForFunction(() => !document.getElementById('hlColors').hidden);
  const highlight = await page.$eval('#hlCustom', (el) => el.value);
  await page.click('#highlightToolBtn');

  await page.click('#textToolBtn');
  await page.waitForFunction(() => !document.getElementById('textOpts').hidden);
  await pickColour('#ff0000');
  await setNumber('textSize', 30);
  await page.mouse.click(box.x + 80, box.y + 120);
  await page.waitForSelector('.freeTextEditor');
  await page.keyboard.type('hello');
  const text = await page.$eval('.freeTextEditor .internal', (el) => ({ color: el.style.color, size: el.style.fontSize }));
  assert.equal(text.color, 'rgb(255, 0, 0)', `Text was drawn in ${text.color}, not the red chosen for it`);
  assert.match(text.size, /^calc\(30px /, `Text was set at ${text.size}, not the 30 pt chosen for it`);
  await page.click('#textToolBtn'); // disarm

  await page.click('#drawToolBtn');
  await page.waitForFunction(() => !document.getElementById('inkOpts').hidden);
  await pickColour('#0000ff');
  await setNumber('inkThickness', 9);
  await setNumber('inkOpacity', 50);
  await page.mouse.move(box.x + 60, box.y + 300);
  await page.mouse.down();
  await page.mouse.move(box.x + 160, box.y + 340, { steps: 8 });
  await page.mouse.move(box.x + 260, box.y + 300, { steps: 8 });
  await page.mouse.up();
  await page.waitForSelector('.viewerContainer:not([hidden]) svg.draw');
  const ink = await page.$eval('.viewerContainer:not([hidden]) svg.draw', (el) => ({
    stroke: el.getAttribute('stroke'), opacity: el.getAttribute('stroke-opacity'), width: parseFloat(el.getAttribute('stroke-width')),
  }));
  assert.equal(ink.stroke, '#0000ff', `the stroke is ${ink.stroke}, not the blue chosen for Draw`);
  assert.equal(ink.opacity, '0.5', `the stroke's opacity is ${ink.opacity}, not the 50% chosen`);
  assert.ok(ink.width >= 9, `the stroke is ${ink.width} wide, thinner than the 9 chosen (the page's scale only makes it wider)`);
  await page.click('#drawToolBtn'); // disarm

  // A border takes the colour chosen for borders, and the highlight is where it was left.
  await page.click('#borderBtn');
  await page.waitForFunction(() => !document.getElementById('hlColors').hidden);
  await pickColour('#00aa00');
  await page.mouse.move(box.x + 60, box.y + 420);
  await page.mouse.down();
  await page.mouse.move(box.x + 200, box.y + 480, { steps: 6 });
  await page.mouse.up();
  await page.waitForSelector('.ovl-box');
  const border = await page.$eval('.ovl-box', (el) => el.style.borderColor);
  assert.equal(border, 'rgb(0, 170, 0)', `the border is ${border}, not the green chosen for it`);
  await page.click('#borderBtn'); // disarm

  await page.click('#highlightToolBtn');
  await page.waitForFunction(() => !document.getElementById('hlColors').hidden);
  assert.equal(await page.$eval('#hlCustom', (el) => el.value), highlight,
    'choosing colours for Text, Draw and a border changed the highlight colour');
  await page.click('#highlightToolBtn');
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeDocument();
});
