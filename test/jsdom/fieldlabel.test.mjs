// The accessible name a field announces is NOT the identifier it is filed under —
// `PLAN-accessibility.md` P06.S05.
//
// ── The defect this guards ───────────────────────────────────────────────────
// `/TU` is what a screen reader speaks for a form field; `/T` is the field's internal name.
// The client has exactly ONE string from the user — what they typed in the naming modal —
// and it derives `/T` from it by trimming, defaulting a blank to `field_N`, and de-duping
// collisions with a numeric suffix. Every one of those steps is right for an identifier and
// wrong for a spoken name: two fields a person calls "Full name" are announced "Full name"
// and "Full name_2", and a field they did not name is announced "field_3".
//
// So the two must be sent SEPARATELY. `pdfops.AuthorForm` writes `/TU` only from `label`, and
// omits the key entirely when it is absent — which means a client that stops sending `label`
// silently reverts the whole slice with every Go test still green, because the Go tests supply
// their own.
//
// ── What this tier can see, and what it cannot ───────────────────────────────
// `view` and `pendingAuthor` are module-scope in a plain script, so this tier cannot drive the
// naming modal end to end. The search is `grep -rln "form/author\|pendingAuthor\|fillable"
// test/`, which returned ZERO before this file and returns only this file after it — so re-run it
// excluding this one, or the claim reads as refuted by its own guard. This is a source scan in the
// shape `keyboardplacement.test.mjs` uses, and it is honest about being one:
// it proves the derivation is still WRITTEN the way the server requires, not that a click
// produces it.
//
// **The driver now exists, one tier up: `test/ui/formauthor.test.mjs`** (`/pending 476`). It
// detects two blanks, types one name into both rows, and reads `/T` and `/TU` back off the
// authored PDF rendered in a real browser. This file is kept rather than retired because it is
// the cheap half: it names the defect in the source, at tier 2 speed, where tier 3 needs a
// built binary and a browser and will not run in a fresh clone without both.
//
// One boot per file — see boot.mjs. This file needs none.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const APP = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');

// The block that builds one field's POST spec, located by the request it feeds rather than by
// a line number.
function authorSpecBlock() {
  const post = APP.indexOf("'/api/form/author'");
  assert.ok(post > 0, "the /api/form/author call is gone — this file's whole subject has moved " +
    'and every assertion below would be scanning unrelated source');
  const start = APP.lastIndexOf('pendingAuthor.forEach', post);
  assert.ok(start > 0 && start < post,
    'the field loop that builds the POST spec is no longer above the request it feeds');
  return APP.slice(start, post);
}

test('the spec sends the typed label alongside the derived name', () => {
  const block = authorSpecBlock();

  // Stimulus floor: a block that does not build a spec at all makes everything below vacuous.
  assert.match(block, /const spec = \{[^}]*\bname\b/,
    'the spec no longer carries a `name`, so this scan is not looking at the field builder');

  assert.match(block, /\bspec\.label\b/,
    'the POST spec carries no `label`. `pdfops.AuthorForm` writes /TU ONLY from `label` and omits ' +
    'the key when it is absent, so every authored field would go back to announcing its internal ' +
    'identifier — with the Go tests still green, because they pass their own Label.');
});

test('the label is the typed value, taken before the default and the de-dupe', () => {
  const block = authorSpecBlock();

  const typed = /const typed = \(f\.input\.value \|\| ''\)\.trim\(\);/;
  assert.match(block, typed,
    'the typed value is no longer captured on its own. It has to be, because every later step — ' +
    'the field_N default and the numeric de-dupe suffix — is correct for an identifier and wrong ' +
    'for a name a person hears.');

  // Order is the assertion, not presence: `label` must read the value BEFORE `base` defaults it.
  const iTyped = block.search(typed);
  const iBase = block.search(/const base = /);
  const iDedupe = block.search(/for \(let n = 2; seen\.has\(name\)/);
  const iLabel = block.search(/spec\.label = typed/);
  assert.ok(iTyped >= 0 && iBase > iTyped,
    '`base` is no longer derived from the separately captured typed value');
  assert.ok(iDedupe > iBase, 'the de-dupe no longer follows the default');
  assert.ok(iLabel > iDedupe,
    '`spec.label` is assigned before the name is finished, which says nothing either way — but ' +
    'the point of this assertion is that it reads `typed` and never `name`');
  assert.ok(!/spec\.label = (name|base)\b/.test(block),
    'the label is being set from the derived name. That is the defect in full: "Full name" and ' +
    '"Full name_2" are one field to a person, and an unnamed field would announce "field_3".');
});

test('a field nobody named sends no label at all', () => {
  const block = authorSpecBlock();
  assert.match(block, /if \(typed\) spec\.label = typed;/,
    'the label is sent unconditionally. An empty one is not harmless: `AuthorForm` writes /TU ' +
    'whenever a label is non-empty, and the point of omitting it is that "field_3" in a /TU is ' +
    'identical to the /T a reader already falls back to — a label that says nothing while ' +
    'claiming to be one.');
});
