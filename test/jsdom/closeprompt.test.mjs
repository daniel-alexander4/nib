// P01.S05 — the close prompt is armed ONLY when something would be lost (D5, D8).
//
// # Why the prompt is conditional at all
//
// Browsers deliberately ignore custom `beforeunload` text, so this can require confirmation and
// cannot say why. A prompt on every close therefore trains the user to dismiss it — and then it is
// worth nothing on the close that mattered. D5's answer is to arm it only when there is something
// to lose, and to put the real wording in Quit Nib (P01.S06), where Nib owns the modal.
//
// # What this tier can see
//
// jsdom has no browser to actually prompt, so the assertion is on the DOOR — `closeWouldLose()` —
// which is the slice's third acceptance clause in its own words: "the armed/unarmed decision has
// one door, not one per condition." The handler is a two-line reader of that door and is asserted
// structurally below.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { boot, REPO } from './boot.mjs';

const h = await boot({});
const APP_SRC = readFileSync(join(REPO, 'web', 'app.js'), 'utf8');

test('nothing to lose does not arm the prompt', () => {
  // **The real event, not the door.** `closeWouldLose` is module-scope and not reachable from the
  // test, and dispatching is the better assertion anyway: it exercises the listener, the door and
  // the preventDefault together, which is what a browser will do.
  const e = new h.window.Event('beforeunload', { cancelable: true });
  h.window.dispatchEvent(e);
  assert.equal(
    e.defaultPrevented, false,
    'a window with nothing open and no ceremony armed prompts on close. A prompt on every close '
      + 'trains the user to dismiss it, and then it is worth nothing on the close that mattered '
      + '(D5).',
  );
});

test('a ceremony armed DOES arm the prompt', () => {
  // **The other half, and it was untestable at this tier until boot stubbed EventSource.** jsdom
  // has none, so app.js's constructor threw, its own try/catch swallowed it, and `ceremonyArmed`
  // could never leave its initial false — a green test that had never met the case.
  const delivered = h.pushWindowEvent('armed', JSON.stringify({ armed: true }));
  assert.ok(
    delivered,
    'no window stream was listening, so this pushed nothing and the assertion below would pass '
      + 'against a build where the armed event is never subscribed to at all.',
  );

  const e = new h.window.Event('beforeunload', { cancelable: true });
  h.window.dispatchEvent(e);
  assert.equal(
    e.defaultPrevented, true,
    'a window closed with a ceremony armed did not prompt. The arm dies with the process, and D5 '
      + 'names exactly this as something that would be lost.',
  );

  // And it comes back down, or the prompt sticks on for the life of the window after one ceremony.
  h.pushWindowEvent('armed', JSON.stringify({ armed: false }));
  const after = new h.window.Event('beforeunload', { cancelable: true });
  h.window.dispatchEvent(after);
  assert.equal(
    after.defaultPrevented, false,
    'the prompt stayed armed after the ceremony disarmed, so every later close prompts for '
      + 'nothing — which is the training-to-dismiss failure D5 refuses.',
  );
});

test('the decision has ONE door, and the handler reads only that door', () => {
  // The clause is "one door, not one per condition". A `beforeunload` that grew a second `if` per
  // condition is how one of them silently stops being asked.
  const handler = APP_SRC.slice(APP_SRC.indexOf("addEventListener('beforeunload'"));
  const body = handler.slice(0, handler.indexOf('\n});'));
  assert.ok(body.length > 20, 'the beforeunload handler did not parse out of app.js');
  assert.ok(
    body.includes('closeWouldLose()'),
    'the close handler does not consult closeWouldLose(). The armed/unarmed decision must have '
      + 'one door — this is the slice\'s own third acceptance clause.',
  );
  // No second condition inline: the door composes them, the handler does not.
  assert.ok(
    !/editedViews\(|ceremonyArmed|\.dirty/.test(body),
    'the close handler tests a condition directly instead of going through closeWouldLose(). Two '
      + 'places deciding the same thing is how one of them silently stops being asked.',
  );
});

test('the door asks about EVERY view, not just the active one', () => {
  // `hasUnsavedWork()` is per-view and closing the window ends every view. That exact defect
  // shipped once already for Close-All: "the other document's typed overlays, its overlay undo
  // stack and its server history were discarded with NO prompt at all."
  const door = APP_SRC.slice(APP_SRC.indexOf('function closeWouldLose()'));
  const body = door.slice(0, door.indexOf('\n}'));
  assert.ok(body.length > 10, 'closeWouldLose did not parse out of app.js');
  assert.ok(
    body.includes('editedViews()'),
    'closeWouldLose asks hasUnsavedWork() or a per-view flag. Closing the WINDOW ends every view, '
      + 'so a per-view question discards the other documents\' work with no prompt — the defect '
      + 'app.js:3128 records having shipped once already for Close-All.',
  );
  assert.ok(
    body.includes('ceremonyArmed'),
    'closeWouldLose does not consult the pushed ceremony state, so a window that never armed '
      + 'itself reports nothing-to-lose while a ceremony is running (D5).',
  );
});

test('the armed state comes from the window stream, not from a poll', () => {
  // `pollRecv` starts only when THIS window arms. A door fed from it would be false on every
  // window that did not arm — the policy-armed ceremony D5 most cares about.
  assert.ok(
    /windowStream\.addEventListener\('armed'/.test(APP_SRC),
    'nothing subscribes to the stream\'s armed event, so ceremonyArmed can only ever be its '
      + 'initial false — and the ceremony half of the prompt is inert.',
  );
  assert.ok(
    !/setInterval[^)]*armed/i.test(APP_SRC),
    'the armed state is polled on a timer. It rides the socket every window already holds, on '
      + 'change and never on a clock — a new timer is the cost D1 refused for the window signal.',
  );
});

// P01.S06 — Quit's modal names what it will end, specifically.
//
// D5's shape is that `beforeunload` can require confirmation and cannot say why, so the real
// wording lives here. The clause is "names a live ceremony and an unsaved document SPECIFICALLY,
// not generically" — a user cannot decide about "you have unsaved work".
test('Quit names the ceremony and the document, and does not prompt for nothing', async () => {
  const src = APP_SRC.slice(APP_SRC.indexOf('async function quitNib()'));
  const body = src.slice(0, src.indexOf('\n}\n'));
  assert.ok(body.length > 100, 'quitNib did not parse out of app.js');

  // Named by the ceremony's own words, and by each document's own name.
  // **Interpolated into the MESSAGE, not merely mentioned.** The first version of this asserted
  // `body.includes('ceremonyArmedWhat')` and stayed green against a mutation that removed the name
  // from the sentence while leaving the variable in the ternary that chooses it — a test that
  // checked the word was in the file rather than in the text the user reads.
  assert.ok(
    /\$\{ceremonyArmedWhat\}/.test(body),
    'Quit mentions ceremonyArmedWhat but does not put it in the sentence. "A ceremony is running" '
      + 'is a generality, and the clause asks for the specific one — its intent, in the '
      + 'convener\'s words.',
  );
  assert.ok(
    /originalName/.test(body),
    'Quit does not name the unsaved documents. "You have unsaved work" is the generality the '
      + 'clause refuses.',
  );
  // Every view, not the active one — same reason as the close prompt.
  assert.ok(
    body.includes('editedViews()'),
    'Quit asks about the active view only, so the other documents\' unsaved work goes unnamed and '
      + 'unmentioned — the defect app.js:3128 records for Close-All.',
  );

  // **Nothing to lose does not prompt**, on D5's own argument: a dialog on every quit trains the
  // user to dismiss it, and then it is worth nothing on the quit that mattered.
  const h2 = await boot({});
  let confirmed = 0;
  h2.window.confirm = () => { confirmed += 1; return false; };
  // Nothing armed, nothing edited: the route is called with no dialog.
  assert.ok(
    /if \(lose\.length && !confirm/.test(body),
    'Quit confirms unconditionally. A dialog on every quit trains the user to dismiss it (D5).',
  );
  assert.equal(confirmed, 0, 'sanity: the stub was not called during boot');
});
