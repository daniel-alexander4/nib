// /pending 930 — pinned colours and each tool's last style survive a restart.
//
// What this tier reaches: that the pin button toggles the colour the row shows and saves the list, that the pinned
// colours lead the row and are not repeated among the recent ones, that the pins stop at eight, that only a tool the
// user has set is saved, and that a status carrying pins and styles restores the swatches, the controls and what a newly
// opened document is told. What it cannot: that the vault stores them — internal/server/markupstyle_test.go.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/keep.pdf';
const saved = []; // every /api/settings body, parsed
const restored = {
  pinnedColors: ['#ff8800', '#112233'],
  markupStyles: { INK: { color: '#0000ff', thickness: 7, opacity: 0.4 }, FREETEXT: { color: '#ff0000', size: 18 }, border: { color: '#00aa00' } },
};
const h = await boot({
  routes: {
    '/api/status': () => ({ ...BOOT_ROUTES['/api/status'], ...restored }),
    '/api/open': () => ({ id: 'test-epoch:1', name: 'keep.pdf', path: DOC, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false }),
    '/api/close': () => ({}),
    '/api/lastopen': () => [],
    '/api/settings': (opts) => { saved.push(JSON.parse(opts.body)); return {}; },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);
const swatches = () => [...doc.querySelectorAll('#hlSwatches .hlswatch')];
const last = (key) => [...saved].reverse().find((b) => key in b);

test('setup: a status with pins and styles is restored before any document is open', () => {
  const pinned = swatches().filter((b) => b.classList.contains('pinned')).map((b) => b.title);
  assert.deepEqual(pinned, ['Pinned #ff8800', 'Pinned #112233'], `the pinned swatches are ${JSON.stringify(pinned)}`);
  assert.equal(swatches()[0].classList.contains('pinned'), true, 'the pinned colours do not lead the row');
  assert.equal($('inkThickness').value, '7');
  assert.equal($('inkOpacity').value, '40');
  assert.equal($('textSize').value, '18');
});

test('a document opened after the restore is told the restored style', async () => {
  const types = [];
  const { EventBus } = await import('./stub-viewer.mjs');
  const real = EventBus.prototype.dispatch;
  EventBus.prototype.dispatch = function (name, data) {
    if (name === 'switchannotationeditorparams') types.push([data.type, data.value]);
    return real.call(this, name, data);
  };
  setNextDocument({ numPages: 1, outline: null });
  $('pathInput').value = DOC;
  $('openGo').click();
  await settle(); await settle();
  EventBus.prototype.dispatch = real;
  const byType = Object.fromEntries(types.map(([t, v]) => [t, v]));
  assert.equal(byType[21], '#0000ff', 'Draw\'s restored colour was not sent');
  assert.equal(byType[22], 7);
  assert.equal(byType[23], 0.4);
  assert.equal(byType[12], '#ff0000');
  assert.equal(byType[11], 18);
});

test('pinning toggles the colour in the row, saves the list, and stops at eight', async () => {
  $('highlightToolBtn').click();
  const colour = $('hlCustom').value;
  assert.equal($('hlPin').getAttribute('aria-pressed'), 'false');
  $('hlPin').click();
  assert.deepEqual(last('pinnedColors').pinnedColors, ['#ff8800', '#112233', colour]);
  assert.equal($('hlPin').getAttribute('aria-pressed'), 'true');
  $('hlPin').click();
  assert.deepEqual(last('pinnedColors').pinnedColors, ['#ff8800', '#112233'], 'unpinning did not remove it');

  // Fill to eight, then one more is refused.
  for (const c of ['#010101', '#020202', '#030303', '#040404', '#050505', '#060606']) {
    $('hlCustom').value = c;
    $('hlCustom').dispatchEvent(new h.window.Event('change'));
    $('hlPin').click();
  }
  assert.equal(last('pinnedColors').pinnedColors.length, 8);
  const before = saved.length;
  $('hlCustom').value = '#070707';
  $('hlCustom').dispatchEvent(new h.window.Event('change'));
  $('hlPin').click();
  assert.equal(last('pinnedColors').pinnedColors.length, 8, 'a ninth pin was saved');
  assert.ok(saved.slice(before).every((b) => !('pinnedColors' in b)), 'a refused pin still posted the list');
});

test('only a tool the user has set is saved, with what they set', async () => {
  $('highlightToolBtn').click(); // off
  $('borderBtn').click();
  $('hlCustom').value = '#abcdef';
  $('hlCustom').dispatchEvent(new h.window.Event('change'));
  const styles = last('markupStyles').markupStyles;
  assert.equal(styles.border.color, '#abcdef');
  assert.ok(!('shape' in styles), 'a tool nobody set was saved');
  assert.equal(styles.INK.thickness, 7, 'the restored Draw style was lost from the saved map');
});
