// P01.S01 — tier 3, and it has to be tier 3.
//
// The claim is that a REAL window, in a REAL browser, declares itself to nib and stops
// declaring itself when it goes away. Tier 1 can prove the arithmetic — it does, in
// `internal/server/window_test.go` — but it proves it with a Go client cancelling a
// context, which is a model of a closing window rather than a closing window. Tier 2
// cannot help either: jsdom has no EventSource wired to a live server, and the whole
// point is that the socket's fate follows the page's.
//
// **The observable is a log line, deliberately.** A tier-3 test drives the shipped
// binary and cannot read a Go field, and the alternative — publishing the count in
// `/api/status` — would put internal state into the client's public shape for a test's
// convenience. The literals here are the seam inventory's *Emitted string* for rows P1
// and S1, and they are asserted against captured output rather than against the
// server's vocabulary: greping for a function name that never appears in its own log
// line returns 0 over a log full of events.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { launch, WORK } from './harness.mjs';

const LOG = join(WORK, 'nib.log');
const readLog = () => readFileSync(LOG, 'utf8');

// countOf is a substring count rather than a regex over the whole line, so a change to
// the "(N open)" suffix does not silently stop matching.
const countOf = (text, needle) => text.split(needle).length - 1;

// waitForLog polls because the server writes its line when the socket event reaches it,
// which is not ordered against Playwright's promise resolving.
async function waitForLog(needle, atLeast) {
  const deadline = Date.now() + 10000;
  for (;;) {
    const n = countOf(readLog(), needle);
    if (n >= atLeast) return n;
    if (Date.now() > deadline) {
      assert.fail(`saw ${n} of "${needle}" in nib.log, wanted at least ${atLeast}`);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
}

test('a real window declares itself, and stops when it closes', async () => {
  // Baselines taken BEFORE the window opens. The harness shares one nib across every
  // tier-3 file, so this log already contains other files' windows — an absolute count
  // would pass or fail depending on which files ran first, which is the kind of green
  // that means nothing.
  const before = readLog();
  const connectedBefore = countOf(before, 'window connected');
  const goneBefore = countOf(before, 'window gone');

  const { browser, page, consoleErrors } = await launch();

  // The page is up; its stream should have reached the server.
  await waitForLog('window connected', connectedBefore + 1);

  // The stimulus is asserted before the response is graded: if opening a page did not
  // add a connect, the close assertion below would be measuring nothing.
  assert.equal(
    countOf(readLog(), 'window gone'),
    goneBefore,
    'a window that is still open must not have been reported gone',
  );

  await browser.close();

  // The whole slice, at tier 3: the window went away and nib noticed, without anyone
  // telling it and without watching the browser process.
  await waitForLog('window gone', goneBefore + 1);

  assert.deepEqual(consoleErrors, [], 'the page logged errors');
  void page;
});
