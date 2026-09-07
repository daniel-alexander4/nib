// The Simple Sign checklist tells the truth about what is done.
//
// Tier 2 holds the DECLARATION — every step names itself, says whether it is required, goes
// somewhere, and is either observable or honestly marked untracked. What it cannot hold is whether
// a tick is TRUE, because every probe reads live state: a document being open, a stamp on a page,
// a signature on the file. That is this file.
//
// The property is one-directional and worth stating plainly: a step must not read `done` until the
// thing is actually done. A list that ticks early is worse than no list, because its whole purpose
// is answering "what is left before I sign".
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('signsteps.pdf', { pages: 2, label: 'steps page' });

const steps = async () => {
  await h.mode('collaborate');
  await h.card('Simple Sign');
  return page.evaluate(() => [...document.getElementById('signSteps').children].map((r) => ({
    label: r.querySelector('.signstep-label').textContent,
    state: r.dataset.state,
    need: r.dataset.need,
  })));
};
const stateOf = (rows, label) => rows.find((r) => r.label === label)?.state;

test('the checklist renders every declared step, with its need', async () => {
  const rows = await steps();
  assert.ok(rows.length >= 10, `the checklist rendered ${rows.length} rows — the list is the feature`);
  assert.ok(rows.some((r) => r.need === 'required'), 'no step is marked required');
  assert.ok(rows.some((r) => r.state === 'untracked'),
    'no step is shown as untracked. Nib cannot observe several of these, and the dash is how it says so rather than showing an empty circle that reads as "not done yet"');
});

test('a step flips to done only when the thing is actually done', async () => {
  // The stimulus, and the direction that matters: it must read NOT done first, or "it says done
  // after" is true of a row that always said done.
  const before = await steps();
  assert.equal(stateOf(before, 'Open the document'), 'todo',
    'the checklist says a document is open before one has been opened');
  assert.equal(stateOf(before, 'Finalize & sign'), 'todo',
    'the checklist says the document is signed before anything has been signed');

  await h.openDocument(DOC, 2);
  const after = await steps();
  assert.equal(stateOf(after, 'Open the document'), 'done',
    'a document is open and the checklist still shows that step as outstanding — the list is not reading live state, so every tick in it is decoration');
  // …and the steps that are NOT done must not have moved with it.
  assert.equal(stateOf(after, 'Finalize & sign'), 'todo',
    'opening a document marked "Finalize & sign" as done. A checklist that ticks early is worse than no checklist');

  h.answerDialogs(true);
  await h.closeDocument();
});

test('a step link goes where it says', async () => {
  // The rows are links; a link that lands nowhere is the failure this catches. Finalize lives in
  // Secure → Sign & Timestamp, so clicking its row must leave the app there with the card open.
  const rows = await steps();
  const i = rows.findIndex((r) => r.label === 'Finalize & sign');
  assert.ok(i >= 0, 'the Finalize step is missing from the checklist');
  await page.evaluate((n) => document.getElementById('signSteps').children[n].click(), i);
  await page.waitForFunction(() => document.body.dataset.tab === 'secure');
  const cardOpen = await page.evaluate(() => {
    const head = [...document.querySelectorAll('#commands .sbhead.groupcard')]
      .find((x) => x.textContent.trim() === 'Sign & Timestamp');
    return head?.getAttribute('aria-expanded') === 'true';
  });
  assert.equal(cardOpen, true,
    'the Finalize row switched mode but did not open the card holding the command — the link lands in the right room and leaves you looking for the door');
});
