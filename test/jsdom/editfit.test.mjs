// P01.S03 — the overlay agrees with the bake.
//
// # Why the client does not decide the fit
//
// The measurement lives once: pdfcpu's core-font metrics, reached through
// mdpdf.CoreWidth (law 4, ADR-009). A browser cannot reproduce that decision — its
// own font metrics are not the AFM tables — so a client that computed the answer
// itself would be a second implementation of the rule, disagreeing with the bake in
// exactly the cases that matter. The client reads the answer back instead, from the
// X-Nib-Fit header P01.S02 put on the response.
//
// # What this tier can see
//
// The behaviour cannot be driven here: applying a report needs a loaded document, a
// laid-out page and an overlay field, and `applyFitReport` is module scope like every
// other function in app.js — this repo deliberately keeps no test-only export surface
// (see view.test.mjs, which records the same ceiling for relayoutOverlays). So the
// assertions are over the SOURCE, and each one names the defect it forbids rather
// than the shape it likes. The end-to-end proof is the live drive recorded in the
// slice's inventory section: four outcomes over a real binary and a real POST.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { boot, REPO } from './boot.mjs';

await boot({});
const CODE = readFileSync(join(REPO, 'web', 'app.js'), 'utf8');

function fnBody(name, opening) {
  const start = CODE.indexOf(opening);
  assert.ok(start !== -1, `${name} is gone or its signature changed — the pin has nothing to bind to`);
  return CODE.slice(start, CODE.indexOf('\n}', start));
}

test('the bake reads the fit report off the response', () => {
  const baked = fnBody('bakedBytes', 'async function bakedBytes(');
  assert.match(baked, /res\.headers\.get\('X-Nib-Fit'\)/,
    'the bake ignores X-Nib-Fit. The server measured the fit and the overlay keeps showing '
      + 'the size the client asked for, which is the disagreement this slice exists to end.');
  assert.match(baked, /applyFitReport\(owner, fieldSources,/,
    'the report is read but not applied to the owning view\'s fields');
});

test('a shrunk field takes the size the PAGE carries', () => {
  const fn = fnBody('applyFitReport', 'function applyFitReport(owner, sources, header)');
  assert.match(fn, /f\.size = fit\.stampedPt/,
    'a shrunk field keeps the size it asked for. Since P01.S02 that is not the size on the '
      + 'page — a 12pt edit can be stamped at 10 — so the preview lies about the document '
      + 'the user just produced.');
  assert.match(fn, /layoutFieldNow\(owner, f\)/,
    'the size is written but the overlay is not re-laid-out, so nothing on screen moves');
});

// ADR-001. The report names fields by INDEX, and the overlay that index named may be
// gone by the time the answer arrives — the user can delete a field, or switch
// documents, across the bake's awaits.
test('a stale index cannot write to a field that is no longer open', () => {
  const fn = fnBody('applyFitReport', 'function applyFitReport(owner, sources, header)');
  assert.match(fn, /owner\.overlayFields\.includes\(f\)/,
    'applyFitReport writes to sources[fit.field] without checking the field is STILL open. '
      + 'A field deleted during the bake leaves its index pointing at whatever is there now '
      + '(ADR-001).');
  assert.doesNotMatch(fn, /(?<![.\w$])view\b/,
    'applyFitReport reaches the ACTIVE view — it must use only the owner it was handed, or a '
      + 'background bake resizes the foreground document\'s fields');
});

test('a report that cannot be read changes nothing', () => {
  const fn = fnBody('applyFitReport', 'function applyFitReport(owner, sources, header)');
  assert.match(fn, /try \{ fits = JSON\.parse\(header\); \} catch \{ return \[\]; \}/,
    'a malformed header throws out of the bake. The bake succeeded and the bytes are good; '
      + 'failing the whole save over an advisory header would be worse than ignoring it.');
  assert.match(fn, /if \(!Array\.isArray\(fits\)\) return \[\]/,
    'valid JSON that is not an array (a bare string, a number) is iterated anyway');
  assert.match(fn, /if \(!header\) return \[\]/,
    'an ABSENT header is the common case — every field fitted — and must be a no-op');
});

// Law 3: every fallback names its cause. The three causes have three different things
// a user can do, so a lumped "some edits did not fit" is worth nothing.
test('each cause is named separately, with its own count', () => {
  const words = CODE.slice(CODE.indexOf('const FIT_WORDS'), CODE.indexOf('};', CODE.indexOf('const FIT_WORDS')));
  for (const cause of ['shrunk', 'wrapped', 'overran']) {
    assert.ok(words.includes(`${cause}:`),
      `FIT_WORDS has no wording for "${cause}" — that outcome would reach the user as a raw `
        + 'protocol word, or not at all (law 3)');
  }
  const tell = fnBody('tellFitReport', 'function tellFitReport(applied)');
  assert.match(tell, /byCause\.set\(fit\.outcome,/,
    'tellFitReport does not key its grouping on the OUTCOME. One count across every cause is the '
      + 'lumped message the seam inventory\'s P5 row forbids by name — and an earlier cut of this '
      + 'assertion only checked that a variable called byCause existed, which a mutation that '
      + 'lumped everything under one key passed.');
  assert.match(tell, /n === 1 \? 'edit was' : 'edits were'/,
    'the message does not agree in number, so a single misfit reads as "1 edits were"');
  // The measured overrun reaches the user. "too long for the box" leaves them guessing
  // whether to cut a word or a sentence; the number is what tells them, and it is why
  // the server publishes overrunPt rather than a bare verdict.
  assert.match(tell, /f\.overrunPt/,
    'the overrun is measured, published on the wire and never shown. The user is told their '
      + 'edit is too long and not by how much, which is the half they can act on.');
  assert.match(tell, /by \$\{Math\.round\(worst\)\}pt/,
    'the overrun is read but not put in the sentence');
});

// Law 4 / ADR-009: which overlays bake is ONE rule. The fit report names fields by
// their index in the posted array, so a second walk that filtered differently would
// map a report onto the wrong overlay.
// S04: the document's own font name is POSTED, not just computed and dropped.
//
// `addEdit` has always derived it — `commonObjs.get(fontName).name`, measured in a
// real browser as "Courier" / "Helvetica-Bold" — and then discarded it the moment
// classifyFont had collapsed it to one of twelve. Without it the server cannot tell
// an exact width measurement from Helvetica standing in for a display face.
test('the edit carries the document\'s own font name to the server', () => {
  const walk = fnBody('collectFieldsWithSources', 'function collectFieldsWithSources(owner = view)');
  assert.match(walk, /baseFont: f\.baseFont \|\| ''/,
    'the posted field drops baseFont, so every fit verdict reaches the server as though it '
      + 'were measured in the document\'s own face when it may be a stand-in');
  const add = fnBody('addEdit', 'async function addEdit(hit, frac)');
  assert.match(add, /baseFont = name\.trim\(\)/,
    'addEdit computes the real font name and throws it away again');
  assert.match(CODE, /baseFont: opts\.baseFont \|\| ''/,
    'makeEditField does not keep baseFont on the field, so collectFields has nothing to send');
});

test('there is one field-collection walk, not two', () => {
  assert.match(CODE, /function collectFields\(owner = view\) \{ return collectFieldsWithSources\(owner\)\.fields; \}/,
    'collectFields no longer delegates — a second copy of the filter decides which overlays '
      + 'bake, and the fit report\'s indices would address the wrong ones (ADR-009)');
  const walks = CODE.match(/for \(const f of owner\.overlayFields\) \{\s*if \(f\.kind === 'text'/g) || [];
  assert.equal(walks.length, 1,
    `${walks.length} walks decide which overlay fields bake; there must be exactly one`);
});

// Two markers, and the split is the point: `shrunk`/`wrapped` mean Nib ALTERED what
// gets baked, `overran` means it baked exactly what was typed. Collapsing them loses
// the distinction between "we changed your text" and "your text is too long", and the
// first is the one a user must be able to spot without having caught the toast.
test('every outcome that CHANGES the bake is marked, distinctly from one that does not', () => {
  const fn = fnBody('applyFitReport', 'function applyFitReport(owner, sources, header)');
  assert.match(fn, /classList\.toggle\('ovl-refit', fit\.outcome === 'shrunk' \|\| fit\.outcome === 'wrapped'\)/,
    'a shrunk or wrapped edit carries no marker. Those are the outcomes that REWRITE what '
      + 'is baked, so their only signal would be a toast the user can miss — and the document '
      + 'then no longer says what the preview said.');
  assert.match(fn, /classList\.toggle\('ovl-misfit', fit\.outcome === 'overran'\)/,
    'an overrunning edit carries no marker');
  const css = readFileSync(join(REPO, 'web', 'style.css'), 'utf8');
  for (const cls of ['ovl-misfit', 'ovl-refit']) {
    assert.match(css, new RegExp(`input\\.ovl-edit\\.${cls}`),
      `${cls} is set by app.js and styled by nothing — the class is applied and invisible`);
  }
  // And they must not be the same rule, or the two facts render identically.
  const misfit = css.slice(css.indexOf('input.ovl-edit.ovl-misfit'));
  assert.ok(!/ovl-misfit[^{]*\{[^}]*\}\s*input\.ovl-edit\.ovl-refit\s*\{\s*\}/.test(misfit),
    'ovl-refit has an empty rule, so an altered edit looks like an unaltered one');
});

// The overrun sentence must tell the user what to DO. The ratio bound means Nib
// deliberately refuses to shrink very long text, so "too long by 119pt" without a
// next step reads as a failure rather than a decision.
test('the overrun message says what to do about it', () => {
  const tell = fnBody('tellFitReport', 'function tellFitReport(applied)');
  assert.match(tell, /FIT_OVERRAN_ADVICE/,
    'an overrun reports a measurement and no remedy; the user is told their edit does not '
      + 'fit and not that shortening it or redrawing the box is the fix');
  assert.match(CODE, /const FIT_OVERRAN_ADVICE = 'shorten it or redraw the box'/,
    'the advice string is gone or reworded past recognition');
});

// /pending 460: the report must survive the operation that produced it.
//
// Five of the app's bake call sites replace the document and reload it, so the marked
// overlay is destroyed and the operation's own toast overwrites the fit sentence. A
// toast alone is therefore not delivery on the path a user takes most.
test('the fit report is also said somewhere persistent', () => {
  const tell = fnBody('tellFitReport', 'function tellFitReport(applied)');
  assert.match(tell, /showFitNotice\(sentence\)/,
    'the fit report is announced only by toast(). toast() sets textContent with no queue and '
      + 'clears after 2.5s, and Save\'s own "Saved" replaces it — so on the primary path the '
      + 'user is told nothing (/pending 460).');
  assert.match(tell, /if \(!applied\.length\) \{ clearFitNotice\(\); return; \}/,
    'a bake where everything fitted returns without taking down an earlier notice, so the '
      + 'banner outlives the problem it describes and reports a document already fixed');

  const show = fnBody('showFitNotice', 'function showFitNotice(sentence)');
  assert.match(show, /els\.fitNotice\.hidden = false/, 'showFitNotice does not reveal the notice');
  assert.match(show, /els\.fitNoticeText\.textContent = sentence/, 'the notice is shown without its text');
});

// It is polite and dismissible, and it is not the alert element.
test('the notice does not spend the one assertive announcement', () => {
  const html = readFileSync(join(REPO, 'web', 'index.html'), 'utf8');
  const el = html.slice(html.indexOf('<div id="fitNotice"'), html.indexOf('</div>', html.indexOf('<div id="fitNotice"')));
  assert.match(el, /role="status"/,
    'the fit notice claims role="alert" — assertive is reserved for #sessionNotice\'s "you are '
      + 'about to lose a signature", and spending it on a recoverable fit report devalues it');
  assert.match(el, /aria-live="polite"/, 'the notice appears without a user action and announces nothing');
  assert.match(el, /id="fitNoticeDismiss"/, 'a persistent notice with no way to dismiss it is a permanent one');
  const css = readFileSync(join(REPO, 'web', 'style.css'), 'utf8');
  assert.match(css, /#fitNotice \{/, 'the notice is shown by app.js and styled by nothing');
});
