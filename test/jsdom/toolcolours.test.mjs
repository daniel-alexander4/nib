// /pending 913 — every markup tool has a colour of its own, and Draw and Text have the controls pdf.js can take.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// Which pdf.js parameter, with which value, app.js sends to the viewer's bus when a swatch or a control is used —
// the stub bus records what was asked, and the numbers are pdf.js 6.1.200's own. Which colour the row shows for the
// armed tool, and that picking for one tool leaves the others.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// That pdf.js DRAWS with it. test/ui/toolcolours.test.mjs puts a stroke on a real page.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { viewersMade, EventBus } from './stub-viewer.mjs';

const DOC = '/tmp/nib-harness/colours.pdf';
let opened = { id: 'test-epoch:1', name: 'colours.pdf', path: DOC };
const h = await boot({
  routes: {
    '/api/open': () => ({ ...opened, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false }),
    '/api/close': () => ({}),
    '/api/lastopen': () => [],
    '/api/settings': () => ({}),
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

const PARAM = { FREETEXT_SIZE: 11, FREETEXT_COLOR: 12, INK_COLOR: 21, INK_THICKNESS: 22, INK_OPACITY: 23, HIGHLIGHT_COLOR: 31 };
const sent = [];
// Every bus, from the first: a document opened later builds a fresh one and the app tells it the user's choices
// while it loads, before any test could wrap that bus by hand.
const byBus = new Map();
const realDispatch = EventBus.prototype.dispatch;
EventBus.prototype.dispatch = function (name, data) {
  if (name === 'switchannotationeditorparams') {
    (byBus.get(this) || byBus.set(this, []).get(this)).push([data.type, data.value]);
    sent.push([data.type, data.value]);
  }
  return realDispatch.call(this, name, data);
};
test('setup: a document is open', async () => {
  setNextDocument({ numPages: 1, outline: null });
  $('pathInput').value = DOC;
  $('openGo').click();
  await settle(); await settle();
  assert.ok(viewersMade.length >= 1);
});

const swatch = (i) => doc.querySelectorAll('#hlSwatches .hlswatch')[i];
const armedSwatch = () => [...doc.querySelectorAll('#hlSwatches .hlswatch')].find((b) => b.getAttribute('aria-pressed') === 'true' || b.classList.contains('armed'));
const hex = (b) => b && doc.defaultView.getComputedStyle(b).backgroundColor;

test('Draw and Text show the colour row, each sends its own pdf.js parameter, and the tools do not share a colour', () => {
  assert.equal($('hlColors').hidden, true, 'setup: the row shows with no tool armed');

  $('drawToolBtn').click();
  assert.equal($('hlColors').hidden, false, 'Draw has no colour row');
  assert.equal($('inkOpts').hidden, false, 'Draw has no thickness / opacity');
  assert.equal($('textOpts').hidden, true);
  sent.length = 0;
  swatch(2).click();
  assert.deepEqual(sent.map((s) => s[0]), [PARAM.INK_COLOR], `Draw's swatch sent ${JSON.stringify(sent)}, want INK_COLOR only`);
  const inkPick = sent[0][1];

  $('textToolBtn').click();
  assert.equal($('textOpts').hidden, false, 'Text has no size control');
  assert.equal($('inkOpts').hidden, true);
  sent.length = 0;
  swatch(3).click();
  assert.deepEqual(sent.map((s) => s[0]), [PARAM.FREETEXT_COLOR], `Text's swatch sent ${JSON.stringify(sent)}, want FREETEXT_COLOR only`);
  assert.notEqual(sent[0][1], inkPick, 'setup: the two picks are the same colour, so independence below proves nothing');

  // Back to Draw: the row shows Draw's colour, not the one chosen since.
  $('drawToolBtn').click(); // Text is armed, so this arms Draw in its place
  assert.equal($('hlCustom').value, inkPick, `Draw came back showing ${$('hlCustom').value}, not its own ${inkPick}`);
});

test('a border and a highlight keep colours of their own', () => {
  $('highlightToolBtn').click();
  swatch(0).click();
  const highlight = $('hlCustom').value;
  $('highlightToolBtn').click(); // off
  $('borderBtn').click();
  assert.equal($('hlColors').hidden, false);
  swatch(4).click();
  const border = $('hlCustom').value;
  assert.notEqual(border, highlight, 'setup: same colour');
  $('borderBtn').click(); // off
  $('highlightToolBtn').click();
  assert.equal($('hlCustom').value, highlight, 'picking a border colour changed the highlight colour');
});

test('a document opened later is told what the user chose, and only that', async () => {
  // Chosen so far: Draw's and Text's colour (and Highlight's). Never chosen: thickness, opacity, size.
  const before = new Set(byBus.keys());
  opened = { id: 'test-epoch:2', name: 'second.pdf', path: '/tmp/nib-harness/second.pdf' };
  setNextDocument({ numPages: 1, outline: null });
  $('pathInput').value = opened.path;
  $('openGo').click();
  await settle(); await settle();
  const fresh = [...byBus.keys()].filter((b) => !before.has(b));
  assert.ok(fresh.length >= 1, 'setup: the second document built no bus of its own');
  const told = fresh.flatMap((b) => byBus.get(b)).map((x) => x[0]);
  assert.ok(told.includes(PARAM.INK_COLOR) && told.includes(PARAM.FREETEXT_COLOR),
    `the new document was told ${JSON.stringify(told)}; Draw's and Text's colours were chosen and are missing`);
  for (const never of [PARAM.INK_THICKNESS, PARAM.INK_OPACITY, PARAM.FREETEXT_SIZE]) {
    assert.ok(!told.includes(never), `the new document was told parameter ${never}, which nobody chose`);
  }
});

test('Draw thickness and opacity and Text size are sent as the numbers pdf.js reads, held to their range', () => {
  $('drawToolBtn').click(); // Text is armed (the test above), so this arms Draw
  sent.length = 0;
  $('inkThickness').value = '7'; $('inkThickness').dispatchEvent(new doc.defaultView.Event('change'));
  $('inkOpacity').value = '40'; $('inkOpacity').dispatchEvent(new doc.defaultView.Event('change'));
  assert.deepEqual(sent, [[PARAM.INK_THICKNESS, 7], [PARAM.INK_OPACITY, 0.4]]);
  sent.length = 0;
  $('inkThickness').value = '500'; $('inkThickness').dispatchEvent(new doc.defaultView.Event('change'));
  $('inkOpacity').value = '0'; $('inkOpacity').dispatchEvent(new doc.defaultView.Event('change'));
  assert.deepEqual(sent, [[PARAM.INK_THICKNESS, 20], [PARAM.INK_OPACITY, 0.05]], 'an out-of-range value reached pdf.js as typed');
  $('textToolBtn').click();
  sent.length = 0;
  $('textSize').value = '18'; $('textSize').dispatchEvent(new doc.defaultView.Event('change'));
  assert.deepEqual(sent, [[PARAM.FREETEXT_SIZE, 18]]);
});

test('nothing the user did not choose is sent: arming a tool dispatches no Draw or Text parameter', () => {
  $('textToolBtn').click(); // off
  sent.length = 0;
  $('highlightToolBtn').click();
  assert.ok(sent.every((s) => s[0] === PARAM.HIGHLIGHT_COLOR), `arming Highlight sent ${JSON.stringify(sent)}`);
});
