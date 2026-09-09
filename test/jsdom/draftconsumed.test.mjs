// P03.S03 — a consumed draft cannot come back (D4).
//
// # The half the scope sentence does not name
//
// "A successful `convene` clears the draft and a failed one does not" reads as server-only, and the
// server clearing its store is not enough. After a successful convene the client calls
// `showCeremonyForm(null)`, which HIDES the sheet without clearing it — so the consumed draft's
// values stay in the fields, where `#ceremonyConveneForm`'s change listener re-saves them on the
// next keystroke, and where reopening shows them as though restored because `restoreCeremonyDraft`
// finds nothing and returns early.
//
// That falsifies the slice's own third acceptance clause — "a second convene cannot reuse a
// consumed draft" — with the server half working perfectly.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { boot, REPO } from './boot.mjs';

const h = await boot({});
const APP_SRC = readFileSync(join(REPO, 'web', 'app.js'), 'utf8');

// **Comments stripped before any ORDERING check, and a probe is why.** The first cut compared
// `indexOf('clearCeremonyForm()')` against `indexOf('showCeremonyForm(null)')` over raw source and
// went red against correct code — because the comment explaining the fix mentions
// `showCeremonyForm(null)`, and the comparison found the explanation before the call. That is
// `/pending 342`'s finding exactly: "a comment I wrote about a guard broke it — regex over source
// text read the explanation as an arm."
const codeOnly = (src) => src.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');

test('a successful convene clears the form, not only the server store', () => {
  const fn = APP_SRC.slice(APP_SRC.indexOf('async function convene'));
  const body = codeOnly(fn.slice(0, fn.indexOf('\n}\n')));
  assert.ok(body.length > 100, 'the convene handler did not parse out of app.js');

  assert.ok(
    body.includes('clearCeremonyForm()'),
    'a successful convene hides the sheet without emptying it. The consumed draft\'s values stay '
      + 'in the fields, and the form\'s own change listener re-saves them on the next keystroke — '
      + 'resurrecting a draft the server has already consumed.',
  );
  // Ordering: cleared before the sheet is hidden, so nothing can observe a filled hidden form.
  assert.ok(
    body.indexOf('clearCeremonyForm()') < body.indexOf('showCeremonyForm(null)'),
    'the form is cleared after it is hidden. Ordering it the other way leaves a window in which '
      + 'the sheet is gone and its values are still live to a change event.',
  );
});

test('the clear writes every field the restore reads', () => {
  // A clear that missed a field would leave exactly that field to be re-saved by the next change
  // event — a partial resurrection, harder to notice than a whole one because the sheet looks blank.
  //
  // **The ASSIGNMENT is asserted, not the mention, and two probes are why.** The first version
  // checked `clear.includes('cerISign')` and stayed green against a mutation that deleted the
  // assignment, because the `getElementById('cerISign')` line above it still mentions the name.
  // Same for the roster: `cerpeerbox` survives in the querySelector after `box.checked = false` is
  // gone. This is the third time in one session that "the word is in the file" passed for "the
  // thing is done"; the fix is always to assert the effect.
  const grab = (name) => {
    const at = APP_SRC.indexOf(`function ${name}(`);
    assert.notEqual(at, -1, `${name} not found in app.js`);
    const rest = APP_SRC.slice(at);
    return codeOnly(rest.slice(0, rest.indexOf('\n}\n')));
  };
  const clear = grab('clearCeremonyForm');
  const restore = grab('restoreCeremonyDraft');

  const fields = [
    { id: 'cerIntent', wrote: /intent\.value\s*=\s*''/, what: 'the recital' },
    { id: 'cerExpires', wrote: /expires\.value\s*=\s*''/, what: 'the deadline' },
    { id: 'cerISign', wrote: /iSign\.checked\s*=\s*false/, what: 'the convener-signs box' },
  ];
  for (const f of fields) {
    // STIMULUS: the restore really writes this field, or "the clear must too" is about nothing.
    assert.ok(restore.includes(f.id), `sanity: restoreCeremonyDraft does not touch ${f.id}`);
    assert.ok(
      f.wrote.test(clear),
      `clearCeremonyForm does not RESET ${f.what} (${f.id}), though restoreCeremonyDraft writes it. `
        + `That field survives the consume and is re-saved by the next change event — a partial `
        + `resurrection, which is harder to see than a whole one because the sheet looks blank.`,
    );
  }

  // The roster too — the part of a consumed draft that names other people, and the one a naive
  // clear forgets because it lives in generated rows rather than in a fixed field.
  assert.ok(restore.includes('cerpeerbox'), 'sanity: the restore does not touch the roster rows');
  assert.ok(
    /box\.checked\s*=\s*false/.test(clear),
    'clearCeremonyForm leaves the roster checkboxes SET. The roster is the part of a consumed '
      + 'draft that names other people.',
  );
  assert.ok(
    /cap\.value\s*=\s*''/.test(clear),
    'clearCeremonyForm leaves each party\'s capacity text in place, so a consumed draft\'s '
      + 'roster detail is re-saved by the next change event.',
  );
});

test('the clear does not re-save what it just emptied', () => {
  const at = APP_SRC.indexOf('function clearCeremonyForm(');
  const body = APP_SRC.slice(at, APP_SRC.indexOf('\n}\n', at));
  assert.ok(
    !body.includes('saveCeremonyDraft'),
    'clearCeremonyForm posts the emptied form back. The server has already cleared its copy, so '
      + 'this would be a second writer of the same fact — and a failure of it would leave an empty '
      + 'draft stored where none should exist.',
  );
  void h;
});
