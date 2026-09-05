// M1's and M5's acceptance, live: what the user types has to reach the FILE, and what
// autofill writes has to reach the SCREEN.
//
// This is the flow the M1–M9 ledger was written for and the one nothing tested end to
// end. The halves were each covered and the seam between them was not: tier 1 posts
// bytes to `/api/save` and asserts the server writes them, and tier 3 already asserts a
// typed value survives in `annotationStorage` — but nothing joined "the user typed it"
// to "the bytes on disk carry it". Everything in between is client-side and untested by
// either: `saveDocument()` baking annotation storage, `bakedBytes` choosing that path
// over `getData()`, the multipart bake, and the POST.
//
// The file on disk is read with `fs`, not through the app. Asking the app whether it
// saved is asking the thing under test, and it would answer yes off its own toast.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const FORM = writeFixture('save-form.pdf', { pages: 1, label: 'save page', form: true });
const FIELD = '.viewerContainer:not([hidden]) .annotationLayer input[type="text"]';

// The toast is the app's own report that the save returned. It is a WAIT, never the
// assertion — `save()` toasts 'Saved' before anything is read back off the disk, and a
// failure toasts 'save failed: …', so waiting for the literal string also fails loudly
// on the error path instead of timing out with no reason.
const saved = () => page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved');

test('a form fill reaches the file on disk when you Save', async () => {
  await h.openDocument(FORM, 1);
  await page.waitForSelector(FIELD);
  const before = fs.readFileSync(FORM);

  const TYPED = 'typed into the form and saved';
  await page.fill(FIELD, TYPED);
  // The stimulus. Without it the assertions below are true of a document nobody typed
  // into: an unchanged file would still be an unchanged file, and the value the disk
  // does not carry was never on screen either.
  assert.deepEqual(await h.formValues(), [TYPED],
    'setup: the fill did not land in the rendered form, so nothing below is about a save');

  await h.mode('file'); // Save lives on the File tab — see harness.mjs
  await page.click('#saveBtn');
  await saved();

  // ONE assertion, because the two ways this fails are two diagnoses of one property and
  // splitting them puts the weaker one first: with this fixture "the fill was lost" and
  // "nothing was written" produce the same file, so a byte-identity check trips first and
  // reds on a proxy. The property is that the value reached the disk; the message says
  // which of the two shapes it failed in.
  const after = fs.readFileSync(FORM);
  assert.ok(after.includes(TYPED),
    `the saved file does not carry "${TYPED}" — ${after.equals(before)
      ? `it is byte-identical to the ${before.length} bytes that were there before the save, so nothing was written at all`
      : `${after.length} bytes were written without the fill among them`}. The value was in pdf.js's annotationStorage and on screen; what reaches the disk is whatever bakedBytes() produced`);
});

test('the saved file reopens with the fill still in it', async () => {
  // The other direction, and it is a different claim: the bytes carrying the string is
  // a fact about the file, and pdf.js reading its own output back as a FIELD VALUE is a
  // fact about the document being well-formed. A save that appended the text as loose
  // content, or wrote an increment pdf.js cannot chain, passes the test above and fails
  // this one.
  const TYPED = 'typed into the form and saved';
  await h.closeDocument();
  await h.openDocument(FORM, 1);
  await page.waitForSelector(FIELD);

  assert.deepEqual(await h.formValues(), [TYPED],
    'the reopened document does not show the saved fill — the file was written but not as a form value pdf.js can read back');
});

test('autofill from the saved profile visibly updates the rendered form', async () => {
  // **M5, and the word in the ledger entry is "best-effort".** `autofillBtn` writes into
  // pdf.js's annotationStorage and then calls `view.viewer.refresh?.()` — optional-chained,
  // so if that method ever stops existing the values are stored and the screen silently
  // does not change. The user sees an untouched form and a toast claiming N fields were
  // filled. That is why this reads the RENDERED input's value rather than the storage:
  // storage is what the app already believes, the DOM is what the user gets.
  //
  // The document open here is the one the tests above saved, so the field holds the typed
  // value — which is the stimulus. Autofilling a field that already showed the profile
  // value would be a test of nothing.
  const AUTO = 'from the autofill profile';
  const before = await h.formValues();
  assert.deepEqual(before, ['typed into the form and saved'],
    `setup: the field holds ${JSON.stringify(before)}, so autofill has nothing to visibly change`);

  await h.mode('markup'); // Detect/Autofill moved to Mark Up with the re-cut
  await page.click('#editProfileBtn');
  await page.fill('#profileText', `fill1 = ${AUTO}`);
  await page.click('#profileSave');
  await page.waitForFunction(() => document.getElementById('profileModal').hidden);

  await page.click('#autofillBtn');
  // The toast reports the app's own count. Waited on, never asserted as the outcome:
  // "Filled 1 field(s)" is true of a storage write that never reached a pixel, and
  // "No matching field names" would otherwise leave this timing out with no reason.
  await page.waitForFunction(() => /^Filled \d+ field/.test(document.getElementById('toast')?.textContent ?? ''));

  // Bounded wait, not an immediate read: `refresh()` re-renders, and re-rendering is
  // asynchronous, so reading straight after the toast would red on a race rather than on
  // the property. If it never converges, that IS the property failing.
  await page.waitForFunction((v) => {
    const i = document.querySelector('.viewerContainer:not([hidden]) .annotationLayer input[type="text"]');
    return i && i.value === v;
  }, AUTO, { timeout: 10000 }).catch(async () => {
    assert.fail(`the rendered input still shows ${JSON.stringify(await h.formValues())} rather than ${JSON.stringify([AUTO])} ten seconds after the toast said the fields were filled: autofill wrote to annotationStorage and nothing repainted, so the user is looking at a form the app believes it filled`);
  });
});

test('this file leaves the shared server as it found it', async () => {
  // Tier 3 runs every file against ONE nib process — build/uirepro.sh says so, and says
  // what it cost when files stopped clearing up after each other. This file's document
  // was still open when it ended, and the next file's "the second Open did not add a
  // view — 3 !== 2" was the bill.
  //
  // A test rather than an `after` hook, deliberately: a module-scope `after` does not run
  // when an assertion throws, which is exactly when the cleanup matters. And the order is
  // observe, clean up, THEN assert — asserting first would skip the cleanup on the very
  // run that failed.
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0,
    'setup: no document was open, so this cleanup did not cover what the file left behind');
  assert.equal(left, 0,
    `${left} page divs survive the close — a document is still open, and the next file in this tier will count it as one of its own`);
});
