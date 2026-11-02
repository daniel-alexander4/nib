// The two file pickers' `accept` come from the server's format table (status.formats; P01.S01, ADR-184), not from
// a literal in the page. Handed a table the page has never heard of, the pickers follow it — which a page
// carrying its own list could not do.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';

const formats = [
  { key: 'png', label: 'PNG image', exts: ['png'], mime: ['image/png'], available: true, image: true },
  { key: 'zzz', label: 'Zzz', exts: ['zzz', 'zz'], mime: ['application/x-zzz'], available: true },
  { key: 'qqq', label: 'Qqq', exts: ['qqq'], mime: ['application/x-qqq'], available: false },
];
const h = await boot({ routes: { '/api/status': { ...BOOT_ROUTES['/api/status'], formats } } });
const { document: doc } = h;
await h.settle();

test('the import picker offers the available documents of the table, and not an image or a missing tool', () => {
  assert.equal(doc.getElementById('officeInput').accept, '.zzz,.zz');
});

test('the combine picker offers PDF and the table\'s images', () => {
  assert.equal(doc.getElementById('combineInput').accept, 'application/pdf,image/png');
});

test('the page source carries no list of the extensions', async () => {
  const fs = await import('node:fs');
  const html = fs.readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
  for (const id of ['officeInput', 'combineInput']) {
    const tag = html.match(new RegExp(`<input id="${id}"[^>]*>`))[0];
    assert.ok(!/accept=/.test(tag), `${id} carries a literal accept — the table owns it`);
  }
});
