// P01's third exit criterion, live: "the overflow measurement is asserted at tier 1
// AND VISIBLE AT TIER 3."
//
// # Why this tier and not the one below
//
// Tier 2 asserts the rule over app.js's source, because `applyFitReport` is module
// scope and this repo keeps no test-only export surface. What that cannot see is
// whether any of it reaches a person: jsdom has no rendering, so a class that is
// applied but styled into invisibility, or an outline painted the same colour as the
// page, passes it. This tier runs a real browser against the real binary, drags a
// real box over real text, types a replacement too long for it, saves — which is what
// drives /api/bake — and then asks the DOM what a user would see.
//
// # The stimulus, which is the half that makes the rest mean anything
//
// The control below places a SHORT replacement in the same box and requires NO marker.
// Without it, a bug that marked every edit unconditionally would pass everything here.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('editfit.pdf', { pages: 1, label: 'edit fit page' });
const EDIT = '.viewerContainer:not([hidden]) .ovl-edit';
// bakeViaSaveEditable drives a bake that KEEPS the overlay.
//
// "Save" is the obvious choice and it is the wrong one: it reloads the document from
// the server after writing, so `clearOverlays` destroys the field the marker was just
// applied to — measured, `edits: 0` immediately after — and its own "Saved" toast
// replaces the fit message. That is a real limitation of where the report is
// delivered, filed as /pending 460, not something this test should paper over. "Save
// editable copy" bakes through the same /api/bake and hands back a download, so the
// overlay survives and the marker is observable.
async function bakeViaSaveEditable() {
  await h.mode('file');
  await h.card('Save a Copy');
  await page.click('#saveEditableBtn');
  // openSaveAs raises the naming modal once the bake returns — that is the signal the
  // round trip finished, and dismissing it leaves the shared server as we found it.
  await page.waitForSelector('#saveAsModal:not([hidden])');
  await page.click('#saveAsCancel');
  await page.evaluate(() => { const m = document.getElementById('saveAsModal'); if (m) m.hidden = true; });
}

// A replacement far too long for the box it was drawn in must come back MARKED, and
// the marker must be something a person can see.
test('an edit too long for its box is visibly marked after the bake', async () => {
  await h.openDocument(DOC, 1);
  await h.placeEditField();
  await page.waitForSelector(EDIT);

  // Long enough that neither shrinking to the ratio floor nor wrapping a one-line box
  // can rescue it — so the outcome is `overran`, the one that bakes what was typed.
  const LONG = 'this replacement is very considerably longer than the box it was drawn into and cannot be made to fit';
  await page.fill(EDIT, LONG);
  assert.equal(await page.inputValue(EDIT), LONG,
    'setup: the replacement did not land in the overlay, so nothing below is about an edit');
  assert.equal(await page.locator(EDIT).evaluate((el) => el.classList.contains('ovl-misfit')), false,
    'setup: the field is marked BEFORE any bake, so the marker says nothing about the fit');

  await bakeViaSaveEditable();

  // **Class and computed style in ONE evaluate.** Read separately, the element can be
  // gone between them: measured 2026-09-09, `Save` reloads the document from the
  // server after writing it — correctly, since the file now carries the baked edits —
  // and `clearOverlays` takes the field with it. This path (Save editable copy) hands
  // back a download and does not reload, which is why it is the one driven here.
  const seen = await page.locator(EDIT).evaluate((el) => {
    const s = getComputedStyle(el);
    return { cls: [...el.classList], style: s.outlineStyle, width: s.outlineWidth, color: s.outlineColor };
  });

  assert.ok(seen.cls.includes('ovl-misfit'),
    `an edit that overruns its box carries no marker after the bake (classes ${JSON.stringify(seen.cls)}). `
      + 'The server measured the overrun and reported it on X-Nib-Fit; if nothing on screen changes, '
      + 'the only signal is a toast the user may not have been looking at when it fired.');

  // VISIBLE, not merely classed. A rule that resolves to `none`, or to zero width,
  // satisfies tier 2 and shows a user nothing — which is the whole reason this
  // criterion names tier 3 rather than the tier below it.
  assert.notEqual(seen.style, 'none', `the marker resolves to no outline: ${JSON.stringify(seen)}`);
  assert.ok(parseFloat(seen.width) > 0, `the marker's outline has no width: ${JSON.stringify(seen)}`);
});

// The control: the same box, a replacement that fits, and NO marker. This is what
// stops the assertion above being true of a build that marks everything.
test('an edit that fits its box is not marked', async () => {
  await h.closeDocument();
  await h.openDocument(DOC, 1);
  await h.placeEditField();
  await page.waitForSelector(EDIT);

  await page.fill(EDIT, 'ok');
  await bakeViaSaveEditable();

  const cls = await page.locator(EDIT).evaluate((el) => [...el.classList]);
  assert.ok(!cls.includes('ovl-misfit') && !cls.includes('ovl-refit'),
    `a two-character replacement in the same box came back marked ${JSON.stringify(cls)} — `
      + 'the marker fires on every edit rather than on the ones that did not fit, which makes '
      + 'the assertion above vacuous');
});

// **The primary path, and the one the marker cannot serve.**
//
// Save writes the file and then reloads the document from the server — correctly, since
// the file now carries the baked edits — so `clearOverlays` destroys the marked field
// (measured: `edits: 0` immediately after) and `toast()` has no queue, so "Saved"
// replaces the fit sentence. Both signals are gone on the action a user takes most.
//
// After a save the subject is no longer an overlay; it is the FILE, which now contains
// text running past its box. A marker cannot describe a file. A persistent notice can,
// and that is what this asserts.
test('after a real Save, the file that was written is reported on', async () => {
  await h.closeDocument();
  await h.openDocument(DOC, 1);
  await h.placeEditField();
  await page.waitForSelector(EDIT);

  const LONG = 'this replacement is very considerably longer than the box it was drawn into and cannot be made to fit';
  await page.fill(EDIT, LONG);
  assert.equal(await page.inputValue(EDIT), LONG,
    'setup: the replacement did not land in the overlay, so nothing below is about an edit');

  await h.mode('file');
  await page.click('#saveBtn');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved');

  // The overlay really is gone — this is the premise, asserted rather than assumed, so
  // that if Save ever stops reloading, this test says so instead of quietly passing for
  // a different reason.
  assert.equal(await page.locator('.ovl-edit').count(), 0,
    'the overlay survived the save, so the notice this test asserts may not be what a user '
      + 'would actually rely on — re-derive which signal carries this path');

  const notice = page.locator('#fitNotice');
  await notice.waitFor({ state: 'visible', timeout: 5000 }).catch(() => {});
  const seen = await page.evaluate(() => {
    const el = document.getElementById('fitNotice');
    if (!el) return { present: false };
    const s = getComputedStyle(el);
    return { present: true, hidden: el.hidden, display: s.display, text: el.textContent || '' };
  });

  assert.ok(seen.present && !seen.hidden && seen.display !== 'none',
    `nothing persistent reports the fit after a Save (${JSON.stringify(seen)}). The overlay is `
      + 'destroyed by the reload and the "Saved" toast overwrites the fit sentence, so a user '
      + 'who saved an over-long edit is told nothing at all about the file they just wrote.');
  assert.match(seen.text, /too long for the box/,
    `the notice is up but does not name the cause: ${JSON.stringify(seen.text)}`);
});

test('this file leaves the shared server as it found it', async () => {
  // Tier 3 runs every file against ONE nib process; a document left open here is a
  // document the next file counts. Observe, clean up, THEN assert — an `after` hook
  // does not run when an assertion throws, which is exactly when the cleanup matters.
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
