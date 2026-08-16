// P05.S01 — the view record, and the three bindings where sharing is a SAFETY defect.
//
// The dimension review singled these three out because they do not fail like the other
// silent-loss bindings. A shared `splitMarks` produces a wrong split the user can see
// and redo. These three produce:
//
//   * `redactMarks`  — marks drawn on A, baked onto B. Redaction commits through
//     `commitBarrier`, which clears the undo history BY DESIGN, so the wrong-document
//     outcome is irreversible destruction of content with no path back. The plan calls
//     it the worst single outcome anywhere in it.
//   * `signLocked`   — a received signing document opens locked and non-editable, which
//     is a guarantee made to a counterparty. Ambiguity must resolve toward LOCKED.
//   * `lastSig`      — the signature-details modal is where a trust decision is made.
//     One document's verification result shown under another's name misreports a
//     cryptographic fact.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// One view exists until P05.S03, so a *switch* cannot be driven here. What these tests
// assert is the property that makes a switch survivable: the bindings live ON the record
// and nothing reads them from module scope. That is a real, falsifiable claim — the
// refactor is exactly what could have been done wrong — and it is asserted at the source
// because module-scope bindings are not observable from the DOM.
//
// The switch itself is `not exercised` until S03 gives a second view to switch to.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');
const CODE = APP.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');

const PER_VIEW = ['redactMarks', 'signLocked', 'lastSig', 'docGen', 'outlineItems', 'originalName'];

test('the view record exists and is the only home for these bindings', () => {
  assert.match(CODE, /function newView\(\)/, 'there is no view record');
  assert.match(CODE, /let view = newView\(\);/, 'no active view is established');

  // The stimulus: these names must actually appear in the source, or "none of them are
  // module-scope" is a green over an empty population — satisfied by deleting them.
  for (const name of PER_VIEW) {
    assert.ok(CODE.includes(name), `${name} does not appear at all — the scan is not reading what it thinks`);
  }
});

test('none of the per-view bindings is declared at module scope', () => {
  for (const name of PER_VIEW) {
    const decl = new RegExp(`^(?:let|var|const)\\s+${name}\\b`, 'm');
    assert.doesNotMatch(CODE, decl,
      `${name} is still a module-level binding — every open document would share one, which is what this slice removes`);
  }
});

test('every read of a per-view binding goes through the record', () => {
  // A reference not prefixed by `view.` is a read of something that no longer exists —
  // or worse, a re-introduced module-level shadow. Property definitions inside
  // newView() are excluded by requiring a non-property context.
  const body = CODE.slice(CODE.indexOf('function newView()'), CODE.indexOf('let view = newView();'));
  const outside = CODE.replace(body, '');
  for (const name of PER_VIEW) {
    const bare = new RegExp(`(?<![.\\w$])${name}\\b`, 'g');
    const hits = outside.match(bare) || [];
    assert.deepEqual(hits, [],
      `${name} is read without going through the view record (${hits.length} site(s)) — with several views open that read reaches whichever document happens to be active`);
  }
});

// The three safety bindings get their own test, named, rather than being trusted to the
// loop above. The loop proves the mechanism; these prove the mechanism was applied to
// the three that matter, so a future edit that special-cases one of them fails by name.
test('redactMarks belongs to a view — the irreversible one', () => {
  assert.match(CODE, /redactMarks: \[\],/,
    'redactMarks is not a property of the view record');
  // And the comment stating WHY must survive, because the reason is not inferable from
  // the code: redaction commits through commitBarrier, which clears undo by design.
  assert.match(APP, /commitBarrier, which clears the undo history BY DESIGN/,
    'the record no longer records why redactMarks is safety-critical — the next reader cannot infer it from the type');
});

test('signLocked belongs to a view, and its resolution rule is written down', () => {
  assert.match(CODE, /signLocked: false,/,
    'signLocked is not a property of the view record');
  assert.match(APP, /Ambiguity must resolve toward LOCKED/,
    'the resolve-toward-locked rule is not recorded — it is a guarantee made to a counterparty, and the safe direction is not guessable');
});

test('lastSig belongs to a view — the trust decision', () => {
  assert.match(CODE, /lastSig: null,/,
    'lastSig is not a property of the view record');
  assert.match(APP, /misreports a cryptographic fact/,
    'the record no longer says why lastSig is safety-critical rather than cosmetic');
});
