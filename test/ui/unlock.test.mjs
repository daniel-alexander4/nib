// M2's unlock click-through, in a real browser — the last convertible entry on the
// sandbox-blind front-end ledger.
//
// **The blocker this item carried was wrong**, and checking it is what closed the item.
// The entry said driving the unlock overlay "needs a SECOND nib with an enrolled vault"
// because uirepro.sh starts exactly one server and enrols it out of band. That is true of
// reaching the overlay by *navigating* — the vault unlocks once per process, there is no
// Lock control in the UI, and every document route is behind requireUnlocked so the
// harness has to enrol before the browser opens. It is not true of reaching the overlay at
// all: `applyStatus` branches on `st.state` and nothing else, so `/api/status` IS the
// overlay's whole input, and tier 3 has intercepted routes since v1.109.19. Same shape as
// M3's stamp-placement blocker, which also dissolved the first time anyone checked it.
//
// ── Why tier 3 and not tier 2 ────────────────────────────────────────────────
// firstrun.test.mjs already drives `setup` in jsdom, and it declares the ceiling that sent
// this file here: the original M2 defect was the intro overlay at z-index 200 SWALLOWING
// clicks aimed at the form beneath it, and jsdom has no layout and no hit-testing, so it
// cannot see a click land on the wrong element. Every interaction below is a real
// Playwright click or fill, which fails when the target is covered, invisible, or
// zero-sized — that actionability check IS the assertion this tier adds. Nothing here
// dispatches a synthetic event, because a synthetic event reaches a swallowed control
// perfectly well.
//
// ── What this file does NOT prove ────────────────────────────────────────────
// The server's own unlock state machine. `/api/status` is intercepted here, so these tests
// say what the client does with each state, not that the server ever produces it — that
// half is Go-tested (internal/vault, internal/server). Saying so beats letting the next
// reader take a green here as end-to-end unlock coverage.
import test from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const KEY_PATH = '/home/u/.ssh/id_ed25519';

// A status the server sends when the enrolled key is passphrase-protected.
const locked = { state: 'key-locked', keyPath: KEY_PATH, version: 'test' };
// …and when the key file is gone from where it was enrolled.
const missing = { state: 'key-missing', keyPath: KEY_PATH, version: 'test' };

const json = (body, status = 200) => (route) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });

test('a passphrase-protected key shows the unlock prompt, and the prompt takes a real click', async () => {
  const posted = [];
  const h = await launch({
    waitFor: '#authOverlay',
    routes: {
      '**/api/status': json(locked),
      '**/api/ssh/unlock': async (route) => {
        posted.push(JSON.parse(route.request().postData()));
        // The wrong passphrase first: the error path is the one a user actually meets,
        // and a test that only drives the happy path cannot tell "the error is shown"
        // from "the error element is never populated".
        if (posted.length === 1) {
          return json({ error: 'that passphrase did not open the key' }, 400)(route);
        }
        return json({ state: 'ready', version: 'test' })(route);
      },
    },
  });
  const { page, browser } = h;
  try {
    // SETUP: the overlay is up and it is the key-locked face of it. Without this the
    // assertions below could all be satisfied by the setup form, which is a different
    // flow with a different route.
    await page.waitForFunction(() => !document.getElementById('authOverlay').hidden);
    assert.equal(await page.textContent('#authTitle'), 'Enter your key passphrase');
    assert.ok((await page.textContent('#authHint')).includes(KEY_PATH),
      'the hint does not name the key being asked about, so a user with several keys cannot tell which passphrase is wanted');
    // The key-choice block belongs to setup/migrate and must not be offered here —
    // a locked vault is not a chance to enrol a different key.
    assert.ok(await page.locator('#keyChoice').isHidden(), 'the key-choice block is offered on a locked vault');

    // THE CLICK-THROUGH. fill() and click() both refuse a covered or invisible target,
    // which is exactly the defect class this tier exists for.
    await page.fill('#authPw', 'wrong-passphrase');
    await page.click('#authSubmit');
    await page.waitForFunction(() => document.getElementById('authError').textContent.length > 0);
    assert.equal(await page.textContent('#authError'), 'that passphrase did not open the key');
    assert.deepEqual(posted[0], { passphrase: 'wrong-passphrase' },
      'the passphrase did not reach /api/ssh/unlock as the request body the server reads');
    // Still locked — an error must not let the user past.
    assert.ok(!(await page.locator('#authOverlay').isHidden()),
      'a rejected passphrase dismissed the overlay, so a wrong answer unlocks the UI');

    // And the way out.
    await page.fill('#authPw', 'right-passphrase');
    await page.click('#authSubmit');
    await page.waitForFunction(() => document.getElementById('authOverlay').hidden);
    assert.equal(posted.length, 2);
    assert.deepEqual(posted[1], { passphrase: 'right-passphrase' });
    // The deliberate 400 above makes the browser log a resource error, so the assertion
    // is on everything ELSE — a blanket "no console errors" here would either fail on the
    // test's own stimulus or have to be dropped, and dropping it loses the check.
    const unexpected = h.consoleErrors.filter((e) => !e.includes('400'));
    assert.deepEqual(unexpected, [], 'the unlock flow logged console errors beyond the deliberate 400');
  } finally {
    await browser.close();
  }
});

test('a missing key offers recovery, and retrying is a status re-check rather than an enrol', async () => {
  let statusHits = 0;
  const enrols = [];
  const h = await launch({
    waitFor: '#authOverlay',
    routes: {
      '**/api/status': (route) => {
        statusHits += 1;
        // The second read is the retry, and by then the user has put the key back.
        return json(statusHits === 1 ? missing : { state: 'ready', version: 'test' })(route);
      },
      // Must never be reached. `key-missing` recovery is a re-check, not an enrolment:
      // re-enrolling would re-key a vault whose key was only ever MISPLACED.
      '**/api/ssh/enroll': (route) => {
        enrols.push(route.request().postData());
        return json({ state: 'ready' })(route);
      },
    },
  });
  const { page, browser } = h;
  try {
    await page.waitForFunction(() => !document.getElementById('authOverlay').hidden);
    assert.equal(await page.textContent('#authTitle'), 'Unlock key not found');
    // SETUP: the recovery row is the thing under test and it must be reachable.
    assert.ok(await page.locator('#repointRow').isVisible(),
      'the recovery row is not visible, so the only offered way out is a path that may be meaningless');
    assert.equal(await page.inputValue('#repointPath'), KEY_PATH,
      'the recovery field is not prefilled with the path Nib was set up with');
    assert.equal(await page.textContent('#authSubmit'), 'Retry');

    await page.click('#authSubmit');
    // Read the error line FIRST. The recovery branch used to sit after the key-mode
    // block, so with `#keyChoice` hidden and `keySelect` never populated the submit
    // handler returned early with "No key selected." — an error about a control the user
    // cannot see, and no status re-read at all. Asserted here rather than left to the
    // overlay wait below, because a defect that surfaces as a 30 s Playwright timeout
    // tells you nothing about what broke.
    await page.waitForFunction(
      () => document.getElementById('authOverlay').hidden ||
            document.getElementById('authError').textContent.length > 0);
    assert.equal(await page.textContent('#authError'), '',
      'Retry reported an error instead of re-checking the status — the key-missing screen offers no key choice, so any validation of one is validating a control the user cannot see');
    await page.waitForFunction(() => document.getElementById('authOverlay').hidden);
    assert.equal(enrols.length, 0,
      'retrying a missing key hit /api/ssh/enroll — recovery must be a status re-check, not a re-key of a vault whose key was merely misplaced');
    assert.ok(statusHits >= 2, `the retry did not re-read /api/status (${statusHits} read(s))`);
    assert.deepEqual(h.consoleErrors, [], 'the recovery flow logged console errors');
  } finally {
    await browser.close();
  }
});
