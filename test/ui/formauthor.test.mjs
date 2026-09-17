// The fillable-form authoring flow, driven by a person rather than described — `/pending 476`.
//
// ── The absence this closes ──────────────────────────────────────────────────
// `grep -rln "form/author\|pendingAuthor\|fillable" test/` answered with three files before
// this one, and all three are SOURCE SCANS: `test/jsdom/fieldlabel.test.mjs` and
// `test/jsdom/clientdoors.test.mjs` read `els.fieldNameGo.onclick`'s text out of `app.js`
// and match patterns in it, and `test/jsdom/editfit.test.mjs` only mentions the flow in a
// comment. Not one of them clicks anything. The whole path — *detect a blank* → *Save as
// fillable form…* → *name the fields* → `POST /api/form/author` → the PDF that comes back —
// had never been run end to end at any tier.
//
// ── Why the Go suite cannot see the defect this guards ───────────────────────
// `pdfops.AuthorForm` is thoroughly covered, and every one of those tests SUPPLIES ITS OWN
// `FormField{Name, Label}`. `Label` becomes `/TU`, the accessible name a screen reader
// announces, and it exists only because the client sends it: `els.fieldNameGo.onclick`
// keeps the typed text as `label` and derives a separate, de-duplicated `name` for `/T`.
// Delete `spec.label = typed` and every `/TU` in every form nib authors disappears — with
// `go test ./...` green from end to end, because the Go tests pass the label themselves.
// That is the regression this file exists to catch, and nothing else can.
//
// ── Why it has to be tier 3 ──────────────────────────────────────────────────
// `view` and `pendingAuthor` are module-scope in a plain script, and `collectAuthorFields`
// reads `view.overlayFields` — so the naming dialog cannot be reached without a REAL field
// on a REAL page. Placing one means `Detect fields`, which renders the page to a canvas and
// reads its pixels (`detect.js`). jsdom has no canvas and no layout, so tier 2 cannot place
// a field at all, and with no field the button returns at `if (!fields.length)`.
//
// ── What it asserts, and why this one case is the right one ──────────────────
// Two fields, both named the same thing — which is what a person does, because two blanks
// on a form are often both "Full name". `app.js` splits that one typed string in two:
//
//   * `/T` (the identifier) must de-duplicate — `Full name` and `Full name_2` — or pdfcpu
//     refuses the document outright (`pdfcpu: duplicate form field`);
//   * `/TU` (the announced name) must NOT — both must stay `Full name`, because "Full name"
//     and "Full name_2" are the same field to a human and must not be read out as two
//     different ones. That is what the comment in `fieldNameGo` forbids in as many words.
//
// One drive covers the label/name split, the de-dupe and the round trip, and each half
// fails the other's mutation: derive the label from the name and `/TU` goes wrong while
// `/T` stays right; drop the de-dupe and `/T` fails while the label is untouched.
//
// ── How the answer is read ───────────────────────────────────────────────────
// The authored PDF is saved to disk through the app's own Save As, then OPENED IN NIB and
// read off the rendered annotation layer — pdf.js puts `/T` on the input's `name` and `/TU`
// on the section's `title` (`s.title = e.alternativeText`). So the thing asserted is what a
// reader actually exposes to assistive technology, not a byte pattern in the file: this
// repo's own lesson is that a compressed PDF cannot be grepped, and `AuthorTaggedForm`
// writes object streams.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK, shutdown } from './harness.mjs';
import { writeRuledFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

// Two hairline underlines and nothing else — see makeRuledPDF for why the page carries no
// text and how the geometry is picked against detect.js's own thresholds.
const DOC = writeRuledFixture('author-form.pdf', { rules: 2 });
const OUT_DIR = path.join(WORK, 'authored');
fs.mkdirSync(OUT_DIR, { recursive: true });

const NAME = 'Full name';           // what the person types, into BOTH rows
const DEDUPED = 'Full name_2';      // what app.js must make of the second one, for /T only

test('a named field announces the name the user typed, and a repeat gets a new id and the SAME name', async () => {
  await h.openDocument(DOC, 1);

  // ── Stimulus 1: two real text fields on the page ──────────────────────────
  // Asserted, not waited for. A drive that silently places nothing reaches
  // `if (!fields.length) { toast(...); return; }`, the dialog never opens, and every
  // assertion below would then fail as a bare Playwright TimeoutError — which prints no
  // sentence of its own, so a red proof recorded against it can only match the test's
  // NAME, and a name appears in the output whether it passed or failed.
  await h.mode('markup');
  await h.card('Detect & Fill Fields');
  await h.topOfDocument();
  await page.click('#detectBtn');
  await page.waitForFunction(
    () => document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-text').length > 0,
    null, { timeout: 15000 },
  ).catch(() => { /* the assertion below is the one worth reading */ });
  const placed = await page.evaluate(
    () => document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-text').length);
  assert.equal(placed, 2,
    `setup: Detect put ${placed} text fields on a page drawn with exactly two hairline underlines, `
    + 'so the naming dialog below is not the one this test is about. Two is what detect.js\'s '
    + 'underline band should yield for this fixture; another number means the fixture and the '
    + 'detector have drifted apart, not that the authoring flow is broken');

  // ── Stimulus 2: the naming dialog, with one row per field ─────────────────
  await h.mode('file');
  await h.card('Save a Copy');
  await page.click('#saveFillableBtn');
  await page.waitForSelector('#fieldNameModal:not([hidden])', { timeout: 15000 });
  const rows = await page.$$('#fieldNameList input');
  assert.equal(rows.length, 2,
    `setup: the naming dialog offers ${rows.length} rows for 2 detected fields — `
    + 'collectAuthorFields and the dialog disagree, so typing into them proves nothing');

  // The person types the SAME thing into both. This is the whole case.
  await rows[0].fill(NAME);
  await rows[1].fill(NAME);
  assert.deepEqual(await page.$$eval('#fieldNameList input', (els) => els.map((e) => e.value)), [NAME, NAME],
    `setup: the two rows do not both read "${NAME}" after being typed into, so nothing below `
    + 'is about two fields sharing a name');

  // ── The call ──────────────────────────────────────────────────────────────
  // Waited on explicitly so a REFUSED author call names itself. Dropping the client-side
  // de-dupe sends two fields called "Full name" and pdfcpu answers
  // `duplicate form field: Full name` — a 400. Without this, that shows up as the Save As
  // dialog never opening, thirty seconds later, saying nothing about why.
  const authored = page.waitForResponse((r) => r.url().includes('/api/form/author'), { timeout: 30000 });
  await page.click('#fieldNameGo');
  const res = await authored;
  assert.equal(res.status(), 200,
    `POST /api/form/author answered ${res.status()} — ${(await res.text()).slice(0, 200)}. `
    + 'The server refuses a field set it cannot author; two fields carrying one /T is the '
    + 'shape that gets refused, and de-duplicating them client-side is what stops it');

  // ── Through the app's own Save As, to a file this test can name ───────────
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  await page.fill('#saveAsName', 'authored.pdf');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');
  await page.waitForFunction(() => document.getElementById('saveAsModal').hidden);

  const out = path.join(OUT_DIR, 'authored.pdf');
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(out) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.existsSync(out),
    `nothing was written to ${out} — the author call returned 200 and the dialog closed, so the `
    + 'app believes it saved a fillable form that is not there');

  // ── Read it back with a real reader ───────────────────────────────────────
  // The original is CLOSED first: `openDocument` waits for the page count to reach N, both
  // documents have one page, and with the first still open that wait is ALREADY TRUE — so
  // the helper would return before the new view existed and everything below would read the
  // detected OVERLAYS instead of the authored WIDGETS. stamplace.test.mjs learned this the
  // same way.
  h.answerDialogs(true);
  await h.closeDocument();
  await page.waitForFunction(() => document.getElementById('viewerWrap').className !== 'has-doc');
  await h.openDocument(out, 1);
  await h.topOfDocument();
  await page.waitForFunction(
    () => document.querySelectorAll('.viewerContainer:not([hidden]) .annotationLayer .textWidgetAnnotation').length === 2,
    null, { timeout: 30000 },
  ).catch(() => { /* the assertion below reports what was actually there */ });

  // `name` is pdf.js's rendering of /T; `title` is its rendering of /TU, and it is set ONLY
  // when /TU is non-empty (`e.alternativeText && (s.title = e.alternativeText)`), so a
  // missing accessible name arrives here as null rather than as an empty string.
  const widgets = await page.$$eval(
    '.viewerContainer:not([hidden]) .annotationLayer .textWidgetAnnotation',
    (secs) => secs.map((s) => ({
      name: s.querySelector('input')?.name ?? null,
      announced: s.getAttribute('title'),
    })));
  assert.equal(widgets.length, 2,
    `the authored document renders ${widgets.length} text widgets, not 2 — the fields the user `
    + 'named did not reach the file, so the names below are not the question yet');

  // /T: identifiers, and they must differ or the document does not exist.
  assert.deepEqual(widgets.map((w) => w.name).sort(), [NAME, DEDUPED].sort(),
    `the authored fields are named ${JSON.stringify(widgets.map((w) => w.name))}, want `
    + `${JSON.stringify([NAME, DEDUPED])}. The user typed "${NAME}" into both rows; app.js is `
    + 'what makes the second one unique, and /T is an identifier');

  // /TU: the announced name, and it must NOT have been de-duplicated with it. This is the
  // assertion the Go suite structurally cannot make, because it hands AuthorForm the Label.
  assert.deepEqual(widgets.map((w) => w.announced), [NAME, NAME],
    `a screen reader meeting this form announces ${JSON.stringify(widgets.map((w) => w.announced))}. `
    + `Both fields must announce "${NAME}" — the name the person actually typed. `
    + `${widgets.some((w) => w.announced === null)
      ? 'A null means no /TU was written at all: the client stopped sending `label`, and every '
      + 'form nib authors is back to widgets a screen reader cannot name — with the whole Go '
      + 'suite green, because those tests supply the Label themselves.'
      : `A "${DEDUPED}" means the announced name was derived from the de-duplicated identifier `
      + 'instead of from the typed text: two blanks a human calls by one name, read out as two '
      + 'different ones. fieldNameGo\'s own comment forbids exactly this.'}`);
});

test('this file leaves the shared server as it found it', async () => {
  const openPages = (await h.counts()).pages;
  h.answerDialogs(true);
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
