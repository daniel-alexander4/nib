// Combine's picker `accept` and Open…'s tooltip come from the server's format table (status.formats; ADR-184,
// ADR-215), not from a literal in the page. Handed a table the page has never heard of, both follow it — which a page
// carrying its own list could not do. (The Import… picker this file also read is gone: Import folded into Open….)
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';

const formats = [
  { key: 'qqq', label: 'Qqq', exts: ['qqq'], mime: ['application/x-qqq'], available: false, tool: 'Qqqtool', kind: 'Office documents' },
  { key: 'rrr', label: 'Rrr', exts: ['rrr'], mime: ['application/x-rrr'], available: true, tool: 'Rrrtool', kind: 'Office documents' },
  { key: 'zzz', label: 'Zzz', exts: ['zzz', 'zz'], mime: ['application/x-zzz'], available: true, kind: 'Images' },
  { key: 'png', label: 'PNG image', exts: ['png'], mime: ['image/png'], available: true, image: true, kind: 'Images' }, // Office rows come first and Zzz before PNG: the tooltip sorts both
];
const h = await boot({ routes: { '/api/status': { ...BOOT_ROUTES['/api/status'], formats } } });
const { document: doc } = h;
await h.settle();

// The panel's lines, as text: a group heading, then `• label (why)`, the why's colour class beside it.
const panelLines = () => [...doc.querySelectorAll('#openFormats > p, #openFormats li')].map((n) => {
  if (n.tagName === 'P') return n.textContent;
  const why = n.querySelector('.hpwhy');
  return `• ${n.textContent} [${why ? why.className.replace('hpwhy ', '') : 'none'}]`;
});

test('Open…\'s panel lists every format under its group, both alphabetical, each saying whether its program is installed', () => {
  assert.deepEqual(panelLines(), [
    'Open a PDF, or a file Nib converts to PDF:',
    'Images',
    '• PNG image (built in) [builtin]',
    '• Zzz (built in) [builtin]',
    'Office documents',
    '• Qqq (needs Qqqtool, not found on this computer) [missing]',
    '• Rrr (Rrrtool is installed) [found]',
  ]);
});

test('Open… is described by the panel, not by a browser tooltip', async () => {
  const fs = await import('node:fs');
  const html = fs.readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
  assert.match(html, /<button id="openMenuItem" aria-describedby="openFormats">/, 'Open… lost its description, or carries a title again');
  assert.match(html, /<div id="openFormats" class="hoverpanel" role="tooltip" hidden><p class="hptitle">Open a PDF, or a file Nib converts to PDF<\/p><\/div>/,
    'before the status answers, the panel does not say what Open… opens');
});

test('the combine picker offers PDF and the table\'s images', () => {
  assert.equal(doc.getElementById('combineInput').accept, 'application/pdf,image/png');
});

test('the page source carries no list of the extensions', async () => {
  const fs = await import('node:fs');
  const html = fs.readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
  for (const id of ['combineInput']) {
    const tag = html.match(new RegExp(`<input id="${id}"[^>]*>`))[0];
    assert.ok(!/accept=/.test(tag), `${id} carries a literal accept — the table owns it`);
  }
});
