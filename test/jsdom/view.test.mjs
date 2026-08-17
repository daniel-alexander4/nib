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
// Full-line `//` comments only. Trailing comments and `/* */` blocks survive, and the
// bare-name scans below match inside them — so a trailing comment that happens to name
// one of the scanned bindings produces a FALSE RED accusing the code of a defect it does
// not have. That direction is deliberate: it is self-announcing and the fix is to reword
// the comment. A cleverer stripper would have to reason about `//` inside string literals,
// and getting that wrong produces false GREENS, which is the direction that hurts.
const CODE = APP.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');

const PER_VIEW = ['redactMarks', 'signLocked', 'lastSig', 'docGen', 'outlineItems', 'originalName'];

// P05.S03 — the four pdf.js objects. They are scanned by the same machinery because
// they fail the same way, but their reason is stronger than the state bindings': the
// vendored library FORCES them per view. PDFViewer's constructor registers on the bus
// it is handed and mutates the find controller it is handed; PDFFindController
// registers three more handlers on that bus; PDFLinkService holds a single viewer slot.
// Sharing any one of them makes N documents interfere through pdf.js itself.
const PER_VIEW_ENGINE = ['viewer', 'eventBus', 'linkService', 'findController'];

// P05.S04 — the bindings that must SWAP when the active view changes, which is a different
// question from "which bindings have many references" and finds a different set. The phase
// sized itself on the second and was short by roughly 3x.
//
// `selectedPages` is the one to read twice: 1-based page numbers in one document's
// pagination, driving the bulk rotate / delete / reorder bar. Shared, it applies document
// A's page numbers to document B's pages — a destructive wrong-document operation, and it
// appears in no enumeration this plan made before S04's deepdive.
//
// They move onto the record rather than being stashed and restored at the switch, which is
// the phase-open decision that refused swap-on-switch. The first version of this comment
// justified that with a list of after-await reads that was mostly untrue on inspection —
// only `splitRects` was one, and it is now pinned at its operation entry. The real
// justification needs no such list: each is one document's state reaching a shared toolbar
// button, the shared cursor, or a destructive bulk operation.
// The receivers a per-view binding may legitimately be read through. Named explicitly
// rather than accepting any property path: P05.S05 moved several of these onto CAPTURED
// owners, which is stronger than reading the active view — but "reached through a dot" is
// weaker than the check was, and would stay green if a refactor parked them on some
// non-view object. This list is the middle: captured owners are allowed, arbitrary hosts
// are not.
const VIEW_RECEIVERS = ['view', 'owner', 'target', 'v', 'out', 'arrival'];

const PER_VIEW_TOOLS = [
  'docHadFlags', 'signTotal', 'signStarted',
  'redactMode', 'editMode', 'markerMode', 'activeMarker', 'fillTarget', 'activeTool',
  'splitBoxMode', 'splitRects', 'sbPage', 'cropMode', 'cropRect', 'cropPage',
  'borderMode', 'dropdownMode', 'radioMode', 'shapeMode', 'noteMode',
  'selectedPages', 'selAnchor',
];

// The transient counterpart, and the distinction is the point: these hold live DOM nodes in
// the OUTGOING view's page divs mid-gesture. They must be ABORTED on a switch, never
// restored — restoring them is the plausible wrong shape, and it would leave a half-drawn
// preview attached to a document the user is no longer looking at, whose pointerup then
// writes the drag into whichever document is active by then.
const TRANSIENT = ['sbStart', 'sbDiv', 'sbHit', 'cropStart', 'cropDiv', 'cropHit',
  'redStart', 'redDiv', 'redHit', 'edStart', 'edDiv', 'edHit',
  'bdStart', 'bdDiv', 'bdHit', 'ddStart', 'ddDiv', 'ddHit',
  'rdStart', 'rdDiv', 'rdHit', 'shStart', 'shCanvas', 'shHit'];

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

// The scan below excludes newView()'s own body, because that function is the one place
// these names legitimately appear bare — it is where the record is built. Cutting that
// region out is done by locating two string anchors, and THAT is a hazard in its own
// right, so it is checked rather than trusted.
//
// Measured, on the code as it stood at P05.S03: if the closing anchor goes stale,
// `String.slice(start, -1)` swallows 200,944 of the file's 248,519 characters into the
// excluded region, `outside` collapses to the 47,575-character prefix, and all six
// names report zero hits. The scan then reads 19% of the file while claiming to cover
// it — a green over a population it no longer sees.
//
// It is not silent TODAY: the anchors are the same literals asserted above, so a stale
// one turns this file red there first. The failure mode this guards is the realistic
// one — someone fixes the assertion above and misses this independent copy of the same
// string — and it costs three lines to remove.
function outsideTheRecord() {
  const start = CODE.indexOf('function newView()');
  const end = CODE.indexOf('let view = newView();');
  assert.ok(start !== -1, 'the newView() anchor is stale — this scan would silently read almost nothing');
  assert.ok(end !== -1, 'the `let view = newView();` anchor is stale — this scan would silently read almost nothing');
  assert.ok(start < end, 'the anchors are out of order — the excluded region would be empty and the scan would flag the record itself');

  const outside = CODE.replace(CODE.slice(start, end), '');

  // Stimulus, in three parts, because the first two alone do not close it.
  //
  // `indexOf` takes the FIRST match, so the excluded region can only grow UPWARD
  // undetected — and `CODE` strips only lines whose trimmed text begins with `//`, which
  // leaves block comments as a live vector. Measured: a `/* built by function newView()
  // */` planted near the top of app.js grows the excluded region from ~2.4k to ~49k
  // characters, drops roughly 1,300 lines out of the scan, and BOTH downstream sentinels
  // still pass. So a sentinel from before the record is required, and a bound on how much
  // the exclusion may eat is what actually makes the growth visible.
  assert.ok(outside.includes('const $ = (id) => document.getElementById(id)'),
    'the scanned region no longer reaches the top of the file — the exclusion grew upward');
  assert.ok(outside.includes('function setDocumentFromServer'),
    'the scanned region no longer reaches past newView() — the exclusion swallowed the file');
  assert.ok(outside.includes('function relayoutOverlays'),
    'the scanned region no longer reaches the end of the file');

  const eaten = CODE.length - outside.length;
  assert.ok(eaten < 8000,
    `the excluded region is ${eaten} characters — newView() is nowhere near that big, so an anchor has drifted and the scan is reading a population it does not cover`);
  return outside;
}

test('every read of a per-view binding goes through the record', () => {
  // A reference not prefixed by `view.` is a read of something that no longer exists —
  // or worse, a re-introduced module-level shadow.
  //
  // The guard is NAME-shaped, and the test name promises more than that: it proves "no
  // bare name", which is not quite "every read goes through the record". A module-level
  // singleton reached by property path — `const shared = {}; shared.viewer = view.viewer;`
  // — passes both this and the declaration check. Stated so the next reader does not
  // over-trust it; closing it properly needs a parser, not a regex.
  const outside = outsideTheRecord();
  for (const name of [...PER_VIEW, ...PER_VIEW_ENGINE, ...PER_VIEW_TOOLS]) {
    // `(?<![.\\w$])` alone is BLIND to the spread form. In `[...selectedPages]` the character
    // before the identifier is the third dot of `...`, so a property-access lookbehind
    // rejects it — and that is not hypothetical: the two references this scan missed in
    // P05.S04 were the only two spread reads among 193 sites, and both were live
    // ReferenceErrors that killed every bulk page operation. The guard whose whole job is to
    // make a missed rename loud was silent on the one syntactic form the rename missed.
    // So: reject a preceding dot only when it is NOT part of `...`.
    const bare = new RegExp(`(?<!\\.\\.)(?<![\\w$])(?<!\\.)${name}\\b|(?<=\\.\\.\\.)${name}\\b`, 'g');
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

// P05.S02 — the bulk bindings, and the ONE hot path this feature has.
//
// `relayoutOverlays` runs on scroll and zoom. The dimension review pinned it because a
// per-view refactor has an obvious wrong shape — iterate every open document's overlays,
// so that a hidden view's fields are laid out too — and that turns the path a user feels
// most into an N× regression. It must walk the ACTIVE view's fields and only those.
test('relayoutOverlays walks one view, not every open document', () => {
  // Re-derived for P05.S03, NOT loosened. The function took no argument and read the
  // module-level `view`; it now takes the OWNING view, because it fires on that view's
  // own event bus and pairing one view's page geometry with another's field list would
  // move the active document's overlay elements into a background page div. The pin it
  // discharges is unchanged: ONE view's fields, never a collection of views.
  const start = CODE.indexOf('function relayoutOverlays(owner)');
  assert.ok(start !== -1, 'relayoutOverlays is gone or no longer takes its owning view — the hot-path pin has nothing to bind to');
  const fn = CODE.slice(start, CODE.indexOf('\n}', start));

  assert.match(fn, /for \(const f of owner\.overlayFields\)/,
    'relayoutOverlays does not iterate the owning view\'s fields');

  // The wrong shape, named so it fails loudly rather than merely not matching above:
  // any loop over a collection of views inside this function is the N× regression.
  assert.doesNotMatch(fn, /\bviews\b|forEach\s*\(\s*\(?\s*v\b/,
    'relayoutOverlays iterates more than one view — this is the N× regression on scroll and zoom that the hot-path pin exists to prevent');

  // And it must not reach back to the active view: that is the same defect wearing a
  // different shape, and it would pass both assertions above.
  assert.doesNotMatch(fn, /(?<![.\w$])view\b/,
    'relayoutOverlays reads the active view — it must use only the view it was handed');
});

test('the bulk bindings are per-view too', () => {
  for (const name of ['pdfDocument', 'docMeta', 'overlayFields']) {
    assert.doesNotMatch(CODE, new RegExp(`^(?:let|var|const)\\s+${name}\\b`, 'm'),
      `${name} is still module-level — 220 references would all reach whichever document is active`);
    // Reached through SOME record, not specifically the active one. P05.S05 moved several
    // of these onto captured owners (`owner.selAnchor`, `owner.selectedPages`) because the
    // handler that reads them is baked into one view's thumbnail and fires later — which is
    // STRONGER than reading the active view, not weaker.
    //
    // This is a re-derivation, not a loosening: the property form is only a stimulus check
    // that the name is used at all. What actually forbids a module-level binding is the
    // doesNotMatch above, and what forbids a bare read is the scan in the sibling test.
    assert.match(CODE, new RegExp(`(?:${VIEW_RECEIVERS.join('|')})\\.${name}\\b`),
      `${name} is not reached through a view record — expected one of ${VIEW_RECEIVERS.join('/')} as the receiver`);
  }
});

// ── P05.S03 — the viewer, the DOM, and the sweeps ───────────────────────────
//
// What this tier CAN reach: the source shape, the real DOM the app builds at boot, and
// — via a decoy container — whether a cleanup sweep is view-scoped or document-scoped.
// What it cannot: a real switch. One view exists until P06 gives opens a way to add
// one, so "inactive views hidden, never destroyed" and "the listeners survive a view
// being hidden" are asserted structurally here and recorded `not exercised` behaviourally.

test('the four pdf.js objects are built per view, not once for the app', () => {
  // Stimulus first: the names must appear, or "none is module-scope" is a green over an
  // empty population, satisfied by deleting the viewer entirely.
  for (const name of PER_VIEW_ENGINE) {
    assert.ok(CODE.includes(name), `${name} does not appear at all — the scan is not reading what it thinks`);
  }
  for (const name of PER_VIEW_ENGINE) {
    assert.doesNotMatch(CODE, new RegExp(`^(?:let|var|const)\\s+${name}\\b`, 'm'),
      `${name} is still a module-level singleton — pdf.js makes N documents interfere through it`);
  }
  // Each is constructed inside the record's own builder.
  assert.match(CODE, /v\.eventBus = new EventBus\(\)/, 'the event bus is not per view');
  assert.match(CODE, /v\.linkService = new PDFLinkService\(/, 'the link service is not per view');
  assert.match(CODE, /v\.findController = new PDFFindController\(/, 'the find controller is not per view');
  assert.match(CODE, /v\.viewer = new PDFViewer\(/, 'the viewer is not per view');
});

test('the reason the engine is per view survives in the record', () => {
  // Prose, deliberately — the same call V4 made for the safety bindings. Nothing in the
  // types says PDFViewer's constructor mutates the find controller it is handed, so an
  // edit that keeps the mechanism and drops the reason leaves the next reader unable to
  // tell this apart from ordinary tidiness.
  assert.match(APP, /MUTATES the find controller/,
    'the record no longer records that PDFViewer mutates the find controller — the strongest reason these are per view');
});

test('the drawing tools listen on the stable wrap, not on a per-view container', () => {
  // A listener bound to a container would serve only the view that existed when the
  // module evaluated; a document opened later would get no drawing tools at all.
  const onWrap = (CODE.match(/els\.viewerWrap\.addEventListener\('pointer/g) || []).length;
  assert.ok(onWrap >= 26, `only ${onWrap} pointer listeners on the stable wrap — some have moved off it`);
  assert.doesNotMatch(CODE, /els\.viewerContainer/,
    'the module-load container handle is back — it can only ever name the first view');

  // Counting the wrap alone is not enough, and this is the hole the first version had: a
  // NEW tool bound to `view.container` leaves the count at 26 and every other assertion
  // here green, while being exactly the defect described above. So the negative
  // population is named too. The count is a floor rather than an equality for the
  // opposite reason — an equality of 26 goes red when a legitimate eleventh tool is
  // added, which reads as a regression and trains the next person to bump the literal.
  assert.doesNotMatch(CODE, /view\.container\.addEventListener\('pointer/,
    'a pointer listener is bound to a per-view container — it would serve only that view, and a document opened later gets no drawing tools');
  assert.doesNotMatch(CODE, /\.pagesEl\.addEventListener\('pointer/,
    'a pointer listener is bound to a per-view page stack — same defect one level in');

  // Every pointerdown asks whether the event started in the ACTIVE view, because the
  // wrap also receives events over #signBanner, which floats above the page.
  //
  // Paired per handler, not counted. Totals alone are satisfied by moving a guard off a
  // pointerdown and onto its sibling pointermove: the two counts still match, and one
  // pointerdown is left unguarded on the wrap.
  const blocks = CODE.split(/els\.viewerWrap\.addEventListener\('pointerdown'/).slice(1);
  assert.equal(blocks.length, 10, `expected 10 pointerdown handlers on the wrap, found ${blocks.length}`);
  blocks.forEach((b, i) => {
    const head = b.slice(0, 400); // the guard sits two lines in, after the mode bail
    assert.match(head, /if \(!startedInActiveView\(e\)\) return;/,
      `pointerdown handler ${i + 1} of ${blocks.length} has no origin guard — a click on #signBanner reaches it`);
  });
  assert.match(CODE, /e\.target\.closest\('\.viewerContainer'\) === view\.container/,
    'the origin guard no longer compares against the active view container');
});

test('the armed-tool and selection state is per view, and the transient state is not', () => {
  // Stimulus first: every name must appear, or the module-scope check below is a green
  // over an empty population.
  for (const name of PER_VIEW_TOOLS) {
    assert.ok(CODE.includes(name), `${name} does not appear at all — the scan is not reading what it thinks`);
  }
  for (const name of PER_VIEW_TOOLS) {
    assert.doesNotMatch(CODE, new RegExp(`^(?:let|var|const)\\s+${name}\\b`, 'm'),
      `${name} is still module-level — with two views it applies one document's state to another`);
    // Reached through SOME record, not specifically the active one — P05.S05 moved several
    // onto captured owners (`owner.selAnchor`), which is stronger than reading the active
    // view, not weaker. A re-derivation: the module-scope ban above and the bare-read scan
    // in the sibling test are what forbid the defect; this line is only the stimulus.
    assert.match(CODE, new RegExp(`(?:${VIEW_RECEIVERS.join('|')})\\.${name}\\b`),
      `${name} is not reached through a view record — expected one of ${VIEW_RECEIVERS.join('/')} as the receiver`);
  }

  // And the mirror: the transient drag state must STAY module-level. Moving it onto the
  // record would look like consistency and would be wrong — it is aborted on a switch, so
  // a per-view copy is state nobody should ever read back.
  for (const name of TRANSIENT) {
    assert.doesNotMatch(CODE, new RegExp(`(?<![.\\w$])view\\.${name}\\b`),
      `${name} was moved onto the view record — transient drag state is aborted on a switch, never restored`);
  }
});

// P05.S04 — a background load must not tear down the view the user is looking at.
//
// This is the slice's critical, and it is asserted at the source because the behaviour
// cannot be driven: the arrival originates in a p2p session (arrival.test.mjs records that
// ceiling), so no tier can make a background load happen. What IS checkable is the property
// that makes it survivable — the teardown takes an owner, and the load path hands it one.
//
// The defect: setDocumentFromServer guarded every shared-chrome write on `target === view`
// and then called resetSharedDocState() unguarded, which is entirely module-`view` bound —
// it empties overlayFields, nulls the marker bindings, clears redactMarks and drops the
// overlay undo stack. An arrival therefore destroyed the active document's typed values and
// redaction marks, which is verbatim what the arrival path was rewritten to stop doing.
test('the document teardown is owner-scoped, so a background load cannot reach the active view', () => {
  assert.match(CODE, /function clearOverlays\(owner = view\)/,
    'clearOverlays no longer takes an owner — a background load would empty the ACTIVE view\'s overlayFields and redactMarks');
  assert.match(CODE, /function resetSharedDocState\(owner = view\)/,
    'resetSharedDocState no longer takes an owner');
  assert.match(CODE, /resetSharedDocState\(target\);/,
    'setDocumentFromServer calls the teardown without passing its target — the destroyer would resolve the active view');

  // The owner must actually be used, not merely accepted. A parameter that is ignored is a
  // specification with no caller, and it would read as a fix while changing nothing.
  const start = CODE.indexOf('function clearOverlays(owner = view)');
  const body = CODE.slice(start, CODE.indexOf('\n}', start));
  assert.match(body, /owner\.overlayFields = \[\]/, 'clearOverlays does not empty the OWNER\'s fields');
  assert.match(body, /owner\.redactMarks = \[\]/, 'clearOverlays does not clear the OWNER\'s redaction marks');
  assert.doesNotMatch(body, /(?<![.\w$])view\.(overlayFields|redactMarks|activeMarker|fillTarget)/,
    'clearOverlays still reaches the active view for a per-document field — that is the defect');
});

// P05.S05 — the drag listeners, and why this test exists separately from the one above.
//
// The pointer guard cannot see these. Its regexes key on `addEventListener('pointer`, and
// the thumbnail reorder uses `dragstart`/`dragover`/`drop`/`dragend`. Bound to a per-view
// grid they would serve only the view that existed at module evaluation, and every document
// opened later would have thumbnails that are `draggable` and completely inert — the S03
// failure restated in an event family the S03 guard does not match.
test('the drag listeners are on the stable sidebar, not on a per-view grid', () => {
  const DRAG = ['dragstart', 'dragover', 'drop', 'dragend'];
  for (const ev of DRAG) {
    assert.match(CODE, new RegExp(`els\\.thumbs\\.addEventListener\\('${ev}'`),
      `the ${ev} listener is not on the stable #thumbs — a per-view grid serves only the first view`);
  }
  assert.doesNotMatch(CODE, /(?<![.\w$])view\.thumbGrid\.addEventListener\(/,
    'a drag listener is bound to a per-view grid — documents opened later get inert thumbnails');
  assert.doesNotMatch(CODE, /\.thumbgrid'\)\.addEventListener\(/,
    'a drag listener is bound to a queried grid rather than the stable parent');
});

// The capture, which is the part that prevents a destructive wrong-document operation
// rather than merely an inert one.
test('a drag captures the grid and the view it started in', () => {
  assert.match(CODE, /let dragGrid = null;/, 'the drag does not capture its grid');
  assert.match(CODE, /let dragView = null;/, 'the drag does not capture its view');
  assert.match(CODE, /dragGrid = wrap\.closest\('\.thumbgrid'\)/,
    'dragstart does not capture the grid the gesture began in');

  // The cancel path is the dangerous one: re-appending into a resolve-at-call-time grid
  // physically moves one document's thumbnails into another's, after which the next drop
  // reads their dataset.page and reorders the WRONG document. No docGen check catches it,
  // because a drag never touches docGen.
  const start = CODE.indexOf('function onThumbDragEnd()');
  assert.ok(start !== -1, 'onThumbDragEnd is gone — the cancel-path guard has nothing to bind to');
  const fn = CODE.slice(start, CODE.indexOf('\n}', start));
  assert.match(fn, /dragOrig\.forEach\(\(w\) => dragGrid\.appendChild\(w\)\)/,
    'the cancel path restores into a resolved grid rather than the captured one — it can relocate another view\'s thumbnails');
  assert.doesNotMatch(fn, /(?<![.\w$])view\./,
    'onThumbDragEnd reads the active view — it must use only what the gesture captured');

  // And the drop refuses rather than misapplying, because pageOp resolves the module view.
  const dstart = CODE.indexOf('function onThumbDrop(e)');
  const drop = CODE.slice(dstart, CODE.indexOf('\n}', dstart));
  assert.match(drop, /dragView !== view/,
    'the drop does not refuse a reorder whose view has changed — pageOp would apply it to the wrong document');
});
