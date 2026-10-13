// Sign now, pressed in a real window (/pending 876; ADR-135, ADR-147).
//
// The convener's own turn is two presses in the rail: the first quotes the signature block and draws the
// page it goes on with the place outlined, the second signs. It was built and driven in jsdom, which lays
// nothing out, and in the tiers that run more than one process a convener still signs through the dial.
// So nothing had pressed the button where a person does. Pressed there, it signed — and two things were
// wrong that only a window shows: the preview page was drawn 160 px wide in a box 143 px wide, so its
// right edge was cut off, and the preview opened above the button and pushed the outlined block, Sign and
// Not now below the fold.
//
// A real ceremony, convened through the route the wizard posts to, on a real server, twice: this machine is
// the only signer each time, and the other parties are pinned fingerprints who do not sign.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown, WORK } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const page = h.page;

// This tier's server is shared by every file, and a ceremony still waiting on somebody is a fact about the
// server: the file after this one switches ceremonies off and waits for the panel to go, and it timed out behind
// a first version of this file that left one waiting for a second signer. Stopping it here is no way out — a
// stop tells the parties, and these parties are fingerprints nobody answers for. So every ceremony this file
// convenes is one its own signature FINISHES: this machine is the only signer. The party after a convener is
// tier 6's (`build/ceremonyrepro.sh`, CLAUSE 24), over a real connection.
after(async () => { await shutdown(h); });

const api = (path, opt) => page.evaluate(async ({ path, opt }) => {
  const r = await nibFetch(path, opt);
  let body = null;
  try { body = await r.json(); } catch { /* an empty answer */ }
  return { status: r.status, body };
}, { path, opt });
const post = (path, body) => api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const tabs = () => page.$$eval('#tabstrip .tab[aria-controls^="viewerContainer-"]', (els) => els.length);

const BOB = 'b1'.repeat(32), CAROL = 'c2'.repeat(32);

// convened convenes over a fresh document and leaves the window on the rail's next-step view, where Sign
// now is. `cold` closes everything first and opens the ceremony's own copy from disk, as a user does the
// next morning.
async function convened(name, intent, roster, { cold = false } = {}) {
  // From an empty window each time: the document a test before this one signed is not this one's.
  await api('/api/close', { method: 'POST' });
  await page.reload();
  await page.waitForSelector('#empty', { state: 'attached' });
  await h.openDocument(writeFixture(name, { pages: 2, label: name }), 2);
  const me = (await api('/api/peers')).body.fingerprint;
  for (const [fp, label] of [[BOB, 'Bob'], [CAROL, 'Carol']]) {
    assert.equal((await post('/api/peers/pin', { fingerprint: fp, label })).status, 200, `setup: could not pin ${label}`);
  }
  const res = await post('/api/ceremony/convene', {
    roster: roster(me), intent, expires: new Date(Date.now() + 86400e3).toISOString(), convenerSigns: true,
  });
  assert.equal(res.status, 200, `setup: convene refused: ${JSON.stringify(res.body)}`);
  // The window did not convene this — the route was posted to directly — so it reads the document again.
  await page.reload();
  await page.waitForFunction(() => document.getElementById('viewerWrap').classList.contains('has-doc'));
  if (cold) {
    // The convened document is longer than the fixture by the ceremony's own pages; the window says how long.
    await page.waitForFunction(() => /^\/ \d+$/.test(document.querySelector('.pageCount').textContent));
    const pages = Number((await page.$eval('.pageCount', (el) => el.textContent)).slice(2));
    assert.ok(pages > 2, `setup: the convened document has ${pages} pages`);
    assert.equal((await api('/api/close', { method: 'POST' })).status, 200, 'setup: could not close the convened document');
    await page.reload();
    await page.waitForSelector('#empty', { state: 'attached' });
    await h.openDocument(`${WORK}/home/nib/ceremonies/${res.body.ceremony}/document.pdf`, pages);
  }
  await page.click('.modetab[data-tab="collaborate"]');
  await page.click('.sbhead[data-panel="ceremony"]');
  const card = page.locator('#ceremonyList .cercard').filter({ hasText: intent }).first();
  await card.locator('.cernextbtn').click();
  await card.locator('.cersign').waitFor();
  return { card, id: res.body.ceremony };
}

// inView reports whether el's whole box is inside the window and is what a click at its middle would reach.
const inView = (loc) => loc.evaluate((el) => {
  const r = el.getBoundingClientRect();
  const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
  // One pixel of grace: a wrapped line's box ends on a fraction of a pixel.
  return r.width > 0 && r.top >= -1 && r.bottom <= innerHeight + 1 && r.left >= -1 && r.right <= innerWidth + 1 && !!hit && (hit === el || el.contains(hit));
});

async function pressAndRead(card) {
  await card.locator('.cersign').click();
  await page.waitForFunction((el) => el.textContent === 'Sign', await card.locator('.cersign').elementHandle());
  return card.locator('.cerblockpreview').evaluate((p) => {
    const box = p.getBoundingClientRect();
    const canvas = p.querySelector('canvas').getBoundingClientRect();
    const mark = p.querySelector('.srvblock').getBoundingClientRect();
    const pic = p.querySelector('.srvblockpic');
    return {
      box: { left: box.left, right: box.right, top: box.top, bottom: box.bottom },
      canvas: { left: canvas.left, right: canvas.right, top: canvas.top, bottom: canvas.bottom, width: canvas.width },
      mark: { left: mark.left, right: mark.right, top: mark.top, bottom: mark.bottom, width: mark.width, height: mark.height },
      picture: !!pic && pic.complete && pic.naturalWidth > 0,
    };
  });
}

function assertPreview(p, who) {
  const within = (a, b) => a.left >= b.left - 0.5 && a.right <= b.right + 0.5 && a.top >= b.top - 0.5 && a.bottom <= b.bottom + 0.5;
  assert.ok(within(p.canvas, p.box), `${who}: the page is drawn outside its preview box, so part of it is cut off: ${JSON.stringify(p)}`);
  assert.ok(within(p.mark, p.canvas), `${who}: the outlined place is not on the page drawn: ${JSON.stringify(p)}`);
  assert.ok(p.mark.width >= 20 && p.mark.height >= 5, `${who}: the outline is too small to be seen: ${JSON.stringify(p.mark)}`);
  assert.ok(p.picture, `${who}: the server's picture of the block did not load into the outline`);
}

test('the first press shows the place on a whole page with the buttons in view, Not now signs nothing, the second press signs in the same tab', async () => {
  const { card } = await convened('signnow-first.pdf', 'We confirm the inventory of 4 Mill Lane', (me) => [
    { fingerprint: me, label: 'Alice', signs: true },
    { fingerprint: BOB, label: 'Bob', signs: false },
    { fingerprint: CAROL, label: 'Carol', signs: false },
  ]);
  assert.match(await card.innerText(), /It is your turn to sign \(1 of 1 signing\)/);
  const before = await tabs();

  assertPreview(await pressAndRead(card), 'first press');
  for (const sel of ['.cersign', '.cersignback', '.cercallstate']) {
    assert.ok(await inView(card.locator(sel)), `after the first press ${sel} is not on screen — the step opened below the fold`);
  }
  // The outline takes no pointer (it lies over the page it marks), so it is asked for its place alone.
  const mark = await card.locator('.srvblock').evaluate((el) => { const r = el.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight; });
  assert.ok(mark, 'after the first press the outlined place is not on screen');
  // Nothing is signed by the first press.
  assert.deepEqual((await api('/api/attestations')).body.attestations || [], [], 'the first press signed');

  // Not now puts the step back and signs nothing; pressed again, it quotes again.
  await card.locator('.cersignback').click();
  assert.equal(await card.locator('.cersign').innerText(), 'Sign now');
  assert.equal(await card.locator('.cerblockpreview').isHidden(), true);
  assert.deepEqual((await api('/api/attestations')).body.attestations || [], [], 'Not now signed');
  assertPreview(await pressAndRead(card), 'quoted a second time');

  await card.locator('.cersign').click();
  await page.waitForFunction(() => /Everyone has signed\. This ceremony is finished\./.test(document.getElementById('ceremonyList').innerText));
  const signed = (await api('/api/attestations')).body.attestations;
  assert.equal(signed.length, 1, 'the document does not carry exactly this party’s signature');
  assert.ok(signed[0].valid && /\[NibRoster:\d+:[0-9a-f]{64}\]/.test(signed[0].reason), `the signature is not a valid ceremony signature: ${JSON.stringify(signed[0]).slice(0, 300)}`);
  assert.equal(await tabs(), before, 'the signed document opened beside the one that was signed');
  assert.equal(await page.locator('#ceremonyList .cersign').count(), 0, 'Sign now is still offered after signing');
});

test('the only signer, on the ceremony’s copy opened cold from disk: signs in that tab and the ceremony is finished', async () => {
  const { card } = await convened('signnow-sole.pdf', 'I confirm the schedule of works', (me) => [
    { fingerprint: me, label: 'Alice', signs: true },
    { fingerprint: BOB, label: 'Bob', signs: false },
  ], { cold: true });
  assert.match(await card.innerText(), /It is your turn to sign \(1 of 1 signing\)/);
  assert.equal(await tabs(), 1, 'setup: more than the one cold-opened document is open');

  assertPreview(await pressAndRead(card), 'only signer');
  await card.locator('.cersign').click();
  await page.waitForFunction(() => /Everyone has signed\. This ceremony is finished\./.test(document.getElementById('ceremonyList').innerText));
  const signed = (await api('/api/attestations')).body.attestations;
  assert.equal(signed.length, 1);
  assert.ok(signed[0].valid, 'the signature does not verify');
  assert.equal(await tabs(), 1, 'the signed document opened in a tab of its own beside the cold-opened one');
});

test('the two buttons above the list fit the sidebar, and nothing was logged', async () => {
  const fits = await page.$$eval('.ceractions > button', (els) => {
    // The sidebar ends where the view begins; the panel's own box is no measure, it overflowed with the button.
    const edge = document.getElementById('viewerWrap').getBoundingClientRect().left;
    return els.map((el) => { const r = el.getBoundingClientRect(); return { text: el.textContent, ok: r.left >= 0 && r.right <= edge + 0.5 }; });
  });
  assert.equal(fits.length, 2);
  for (const f of fits) assert.ok(f.ok, `"${f.text}" runs outside the sidebar`);
  assert.deepEqual(h.consoleErrors, []);
});
