// Guard: every element id `web/app.js` asks for exists in `web/index.html`.
//
// app.js builds its handle map with 428 `$('id')` lookups at module-evaluation
// time and assigns ~220 handlers to the results. A renamed or deleted id does not
// fail loudly — `getElementById` returns null, the handle lands as undefined, and
// the break surfaces later as "that button does nothing", possibly only on the
// one dialog nobody opened. That is the shape of the Windows report chased in
// v1.102.1: the visible symptom was "no button responds".
//
// This costs a regex sweep and closes the whole class. It reads the two shipped
// files directly rather than booting, so it stays honest even if the harness's
// stubs rot.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const html = fs.readFileSync(path.join(REPO, 'web', 'index.html'), 'utf8');
const app = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const present = new Set([...html.matchAll(/id="([^"]+)"/g)].map((m) => m[1]));
const requested = [...new Set([...app.matchAll(/\$\((["'])([A-Za-z0-9_]+)\1\)/g)].map((m) => m[2]))];

test('the id scan actually found ids', () => {
  // The stimulus assertion, before grading the response: a regex that matched
  // nothing would report "0 missing" and look like perfect health forever.
  assert.ok(requested.length > 300,
    `expected app.js to request hundreds of ids, found ${requested.length} — the scan regex is probably wrong`);
  assert.ok(present.size > 300,
    `expected index.html to define hundreds of ids, found ${present.size}`);
});

test('every id app.js requests exists in index.html', () => {
  const missing = requested.filter((id) => !present.has(id));
  assert.deepEqual(missing, [],
    `app.js asks for ids that index.html does not define: ${missing.join(', ')}`);
});
