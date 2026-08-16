// Tier 3 smoke: the real nib binary, in a real browser.
//
// Two jobs. First, prove the harness reaches a real engine at all — without that,
// every tier-3 test below it is a slower, more fragile tier 2. Second, prove the
// two tiers agree where they overlap, because that agreement is what makes tier
// 2's "tier 3 covers that" delegations trustworthy rather than hopeful.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, BASE } from './harness.mjs';

const { browser, page, consoleErrors } = await launch();
after(() => browser.close());

// The row that justifies the whole tier. Every one of these is false or absent in
// jsdom, and each is something nib's UI actually depends on: layout drives the
// fit-widest-page scale, canvas draws thumbnails and bakes redactions, and the
// media query backs the device-pixel-ratio heal.
test('the harness reaches a real rendering engine', async () => {
  const caps = await page.evaluate(() => ({
    canvas2d: !!document.createElement('canvas').getContext('2d'),
    layout: document.body.clientWidth,
    mediaQueries: typeof matchMedia === 'function' && matchMedia('(min-width: 1px)').matches,
    dpr: window.devicePixelRatio,
  }));
  assert.equal(caps.canvas2d, true, 'no 2d canvas — this tier would be a slower jsdom');
  assert.ok(caps.layout > 0, `body has no width (${caps.layout}) — layout is not running`);
  assert.equal(caps.mediaQueries, true, 'media queries do not evaluate');
  assert.ok(caps.dpr > 0, 'no device pixel ratio');
});

test('it is driving the real nib binary, not a mock', async () => {
  const st = await page.evaluate(async () => (await fetch('/api/status')).json());
  // `ready` proves uirepro.sh's key enrollment actually took: an un-enrolled
  // server answers `setup`, and every document route would be behind the overlay.
  assert.equal(st.state, 'ready', `expected an unlocked vault, got ${st.state}`);
  assert.ok(String(BASE).startsWith('http://127.0.0.1:'), 'nib must be loopback-only');
});

// Where the tiers overlap they must agree — same assertions as
// test/jsdom/smoke.test.mjs, now against the real engine and the real server.
test('the launch state matches what tier 2 asserts', async () => {
  const s = await page.evaluate(() => ({
    empty: document.getElementById('empty').textContent,
    wrap: document.getElementById('viewerWrap').className,
    badge: document.getElementById('sigBadge').textContent,
    badgeClass: document.getElementById('sigBadge').className,
    pageCount: document.querySelector('.pageCount').textContent,
    closeDisabled: document.getElementById('closeBtn').disabled,
    saveDisabled: document.getElementById('saveBtn').disabled,
  }));
  assert.equal(s.empty, 'Open a PDF to begin.');
  assert.equal(s.wrap, '');
  assert.equal(s.badge, 'no document');
  assert.equal(s.badgeClass, 'badge badge-none');
  assert.equal(s.pageCount, '/ 0');
  assert.equal(s.closeDisabled, true);
  assert.equal(s.saveDisabled, true);
});

test('the app boots without console errors it should not have', async () => {
  // The vendored pdf.js references image assets that are not shipped (a known,
  // filed defect — 31 of 32 are missing), so those 404s are expected noise here
  // and are filtered rather than allowed to mask a real error. When that item is
  // fixed this filter should start finding nothing, and can go.
  const unexpected = consoleErrors.filter((e) => !/vendor\/pdfjs\/images\//.test(e));
  assert.deepEqual(unexpected, [], 'unexpected console errors during boot');
});
