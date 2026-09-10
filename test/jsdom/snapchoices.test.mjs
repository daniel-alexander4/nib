// /pending 418 — `snapChoices` takes the page's pixels so it does not read the canvas
// once per choice group, and the caller was computing them and not passing them.
//
// # What was wrong
//
// `app.js` hoisted the read out of the loop — "the canvas is read ONCE for the whole loop.
// snapChoices used to read it per group" — computed `groupPixels`, and then called
// `snapChoices(canvas, grp.choices, grp.marker)` with three arguments. The fourth is the
// whole point. So the per-group read still happened AND a wasted whole-canvas read was
// added: N+1 reads where the design intended 1.
//
// # Why this can be tested here at all, with no canvas in jsdom
//
// `canvas` is used in exactly one place inside `snapChoices` — `pixels || pixelsOf(canvas)`
// — so passing pixels and a NULL canvas is a complete call. That is the assertion: if the
// function ever reaches for the canvas again, this throws instead of quietly costing a read
// per group. Measured in Chrome, 2026-09-09: one whole-canvas read is 4.32 ms at A4 @2x
// (7.76 MB) and 8.95 ms at Letter @2x (14.96 MB), so the saving is N reads per page.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';
import { snapChoices } from '../../web/detect.js';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

// A synthetic page: two ink blobs on a light ground, shaped like two choice words.
function pixels(W, H, blobs) {
  const data = new Uint8ClampedArray(W * H * 4).fill(255);
  for (const [x0, y0, x1, y1] of blobs) {
    for (let y = y0; y < y1; y++) {
      for (let x = x0; x < x1; x++) {
        const i = (y * W + x) * 4;
        data[i] = data[i + 1] = data[i + 2] = 0;
        data[i + 3] = 255;
      }
    }
  }
  return { W, H, data };
}

test('snapChoices given pixels never reaches for the canvas', () => {
  const px = pixels(200, 60, [[20, 10, 50, 30], [90, 10, 130, 30]]);
  const choices = [
    { x0: 18, y0: 8, x1: 52, y1: 32, word: true },
    { x0: 88, y0: 8, x1: 132, y1: 32, word: true },
  ];
  // NULL canvas: the only use of it inside is the pixels fallback, so a complete call
  // with pixels must not touch it. If it does, this throws rather than silently costing
  // a whole-canvas getImageData per group.
  const out = snapChoices(null, choices, null, px);
  assert.equal(out.length, 2, 'snapChoices did not return both choices');
  assert.ok(out.every((c) => Number.isFinite(c.x0) && Number.isFinite(c.x1)),
    `snapChoices returned non-numeric bounds: ${JSON.stringify(out)}`);
  // It snapped to the ink rather than returning the estimate unchanged — otherwise this
  // test would pass against a function that ignored the pixels entirely.
  assert.notDeepEqual(out.map((c) => c.x0), choices.map((c) => c.x0),
    'the bounds are identical to the estimate, so the pixels were not read and this test ' +
    'would pass against a snapChoices that ignored its fourth argument');
});

test('the caller passes the pixels it computed', () => {
  assert.match(APP, /const groupPixels = choiceGroups\.length \? pixelsOf\(canvas\) : null;/,
    'groupPixels is gone — the hoist this pins no longer exists');
  assert.match(APP, /snapChoices\(canvas, grp\.choices, grp\.marker, groupPixels\)/,
    'the loop calls snapChoices without groupPixels. The read was hoisted out of the loop and '
      + 'then not passed in, so the per-group getImageData still happened AND one extra '
      + 'whole-canvas read was added — N+1 reads where the design intended 1 (/pending 418).');
});
