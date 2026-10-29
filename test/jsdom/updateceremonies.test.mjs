// An update that strands a ceremony says so in the download popup (ADR-175).
//
// The server sends `openCeremonies` only where the update crosses the digest flag day (ADR-174 §1), so
// the page's half is small: say it when the count is there, say nothing when it is not, never ask a
// question (ADR-102: the click on the pill IS the request), and never name a ceremony — a count only.
// Which updates carry the count is `internal/server/update_test.go`.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let check;
const base = { current: '2.2.18', latest: '2.4.0', updateAvailable: true, url: 'https://example.test/r', downloadUrl: 'https://example.test/d/nib' };

const h = await boot({
  routes: {
    '/api/status': () => ({ state: 'ready', version: '2.2.18', autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false }),
    '/api/update/check': () => check,
    '/api/update/download': () => ({ status: 'started', name: 'nib', path: '/x/nib', dir: '/x' }),
    '/api/update/download/cancel': () => ({ cancelled: true }),
  },
});
const { document: doc, settle } = h;
const warn = () => doc.getElementById('dlCeremonies');

async function pressPill(c) {
  check = c;
  doc.getElementById('updateGet').click();
  await settle();
}
async function closePopup() {
  doc.getElementById('dlCancel').click();
  await settle();
}

test('with open ceremonies the popup says so, in numbers and not names, and still asks nothing', async () => {
  await pressPill({ ...base, openCeremonies: 3 });
  assert.equal(doc.getElementById('downloadModal').hidden, false, 'setup: the popup did not open, so nothing below is about it');
  assert.equal(warn().hidden, false, 'three ceremonies have not ended and the popup does not say so');
  assert.match(warn().textContent, /^3 ceremonies on this computer have not ended\./);
  assert.match(warn().textContent, /v2\.2\.18/, 'it does not name the version that can finish them');
  assert.match(warn().textContent, /v2\.4\.0/, 'it does not name the version that cannot');
  assert.match(warn().textContent, /Finish them on v2\.2\.18/);
  assert.equal(h.confirms.length, 0, 'the warning raised a confirm() — the pill must not ask (ADR-102)');
  assert.match(doc.getElementById('updateGet').getAttribute('title') || doc.getElementById('updateGet').getAttribute('aria-label') || '', /have not ended/,
    'the pill\'s own description does not carry the warning');
});

test('one ceremony reads in the singular', async () => {
  await closePopup();
  await pressPill({ ...base, openCeremonies: 1 });
  assert.match(warn().textContent, /^A ceremony on this computer has not ended\./);
  assert.match(warn().textContent, /Finish it on v2\.2\.18/);
});

test('no count means no warning: an ordinary update says nothing of ceremonies', async () => {
  await closePopup();
  await pressPill({ ...base });
  assert.equal(doc.getElementById('downloadModal').hidden, false, 'setup: the popup did not open');
  assert.equal(warn().hidden, true, 'an update with no open ceremonies shows a ceremony warning');
  assert.equal(warn().textContent, '', 'the warning text is left over from the last update');
});
