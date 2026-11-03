// The stamp's move and resize gestures are wired to the two modes enableStampGestures understands. A phase-close
// review found `begin(f.drag)` in place of `begin('drag')`: the mode was undefined and every stamp, image and border
// box stopped moving, with every tier green. Nothing drove a pointer on a stamp, so this reads the wiring.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');
const at = src.indexOf('function enableStampGestures(');
const body = src.slice(at, src.indexOf('\n}\n', at));

test('enableStampGestures begins each gesture with its named mode', () => {
  assert.match(body, /el\.addEventListener\('pointerdown', begin\('drag'\)\)/);
  assert.match(body, /handle\.addEventListener\('pointerdown', begin\('resize'\)\)/);
});
test('the modes it begins are the ones the move handler reads', () => {
  assert.match(body, /mode === 'drag'/);
  assert.match(body, /m === 'drag'/);
});
