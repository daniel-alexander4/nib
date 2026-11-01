// /pending 919 F14 — a fill that could not reach a field says so.
//
// A form whose widget's /Parent leads nowhere takes no value, and the filled file would otherwise look as though it
// had. The server says which in `X-Nib-Skipped` (internal/server/form.go); the window reads it after the merge or
// the XFDF import. `tellSkippedFields` is module scope like every function in app.js (no test-only export surface,
// see editfit.test.mjs), so the function is cut out of the source and run with a stub for `toast`, and each fill is
// pinned to call it after `res.ok` and before the save dialog opens.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { boot, REPO } from './boot.mjs';

await boot({});
const CODE = readFileSync(join(REPO, 'web', 'app.js'), 'utf8');

function run(header) {
  const start = CODE.indexOf('function tellSkippedFields(');
  assert.ok(start !== -1, 'tellSkippedFields is gone or renamed — the pin has nothing to bind to');
  const src = CODE.slice(start, CODE.indexOf('\n}', start) + 2);
  const said = [];
  const fn = new Function('toast', `${src}\nreturn tellSkippedFields;`)((m) => said.push(m));
  fn({ headers: { get: (k) => (k === 'X-Nib-Skipped' ? header : null) } });
  return said;
}

test('one field skipped is named in a sentence', () => {
  assert.deepEqual(run('{"count":1,"names":["orphan"]}'),
    ['1 field was not filled: the form cannot take a value for it (orphan)']);
});

test('many are counted whole and only the first three are named', () => {
  const said = run('{"count":12,"names":["a","b","c","d","e","f","g","h","i","j"]}');
  assert.equal(said.length, 1);
  assert.match(said[0], /^12 fields were not filled/);
  assert.match(said[0], /\(a, b, c and 9 more\)$/);
});

test('no header, an empty one, or a broken one says nothing', () => {
  assert.deepEqual(run(null), []);
  assert.deepEqual(run('{}'), []);
  assert.deepEqual(run('not json'), []);
});

test('both fills tell it after the answer is good and before the save dialog opens', () => {
  for (const [route, save] of [['/api/form/fill-csv', "'Save merged PDFs (ZIP)'"], ['/api/form/fill-xfdf', "'Save filled PDF'"]]) {
    const at = CODE.indexOf(`apiFetch('${route}'`);
    assert.ok(at !== -1, `${route} is no longer posted from app.js`);
    const tail = CODE.slice(at, CODE.indexOf(save, at));
    const ok = tail.indexOf('if (!res.ok)');
    const tell = tail.indexOf('tellSkippedFields(res)');
    assert.ok(ok !== -1 && tell > ok, `${route}: tellSkippedFields(res) must follow the res.ok refusal`);
  }
});
