// Settings → Helper Programs (ADR-218): each outside program Nib can use, with what it is, what Nib uses it for,
// whether Nib found it and a link to its site. Open…'s panel shows on hover or focus, and a line whose program was
// not found opens this page at that program.
//
// ## What this cannot see
//
// - Whether the server's flags are right: `/api/status` is stubbed. `TestStatusCarriesTheFormatTable` holds the
//   rows' `tool`, `toolKey` and `kind`; each `*Available` probe has its own Go tests.
// - The panel's placement and scrolling: jsdom has no layout. Tier 3's `test/ui/apppages.test.mjs` opens the page.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';

const formats = [
  { key: 'png', label: 'PNG image', exts: ['png'], mime: ['image/png'], available: true, image: true, kind: 'Images' },
  { key: 'docx', label: 'Word', exts: ['docx'], mime: ['application/x'], available: false, tool: 'LibreOffice', toolKey: 'libreoffice', kind: 'Office documents' },
  { key: 'ps', label: 'PostScript', exts: ['ps'], mime: ['application/postscript'], available: true, tool: 'Ghostscript', toolKey: 'ghostscript', kind: 'PostScript' },
];
let statusCalls = 0;
const status = { ...BOOT_ROUTES['/api/status'], formats, libreoffice: false, ghostscript: true, heif: false, browser: false, svgExport: false, scanner: true }; // every flag differs from a neighbour's, so a crossed key shows
const h = await boot({ routes: { '/api/status': () => { statusCalls++; return status; } } });
const { document: doc, settle } = h;
await settle();
const $ = (id) => doc.getElementById(id);
const panel = $('openFormats');

test('each helper on the page says whether Nib found it, and the state is coloured by the class it carries', () => {
  for (const [key, found] of [['libreoffice', false], ['ghostscript', true], ['heif', false], ['browser', false], ['pdftocairo', false], ['scanner', true]]) {
    const mark = doc.querySelector(`.helperstate[data-helper="${key}"]`);
    assert.ok(mark, `the page has no indicator for ${key}`);
    assert.equal(mark.textContent, found ? 'Installed' : 'Not found on this computer', `${key}'s indicator`);
    assert.equal(mark.classList.contains(found ? 'found' : 'missing'), true, `${key}'s indicator is not coloured as ${found ? 'found' : 'missing'}`);
  }
});

test('every helper has a what, a how, and an https link that opens apart from Nib', () => {
  const sections = [...doc.querySelectorAll('#settingsHelpersPage .helper')];
  assert.equal(sections.length, 6, 'the page does not list the six helpers');
  for (const s of sections) {
    assert.ok(s.querySelectorAll('.sethelp').length >= 3, `${s.id} lacks its summary, its use or its link`);
    const a = s.querySelector('a');
    assert.match(a.getAttribute('href'), /^https:\/\//, `${s.id}'s link is not https`);
    assert.equal(a.getAttribute('rel'), 'noopener');
    assert.equal(a.getAttribute('target'), '_blank');
  }
});

test('Check again asks the server again and repaints', async () => {
  const before = statusCalls;
  status.libreoffice = true;
  $('helpersRecheck').click();
  await settle();
  assert.ok(statusCalls > before, 'Check again did not ask for the status');
  assert.equal(doc.querySelector('.helperstate[data-helper="libreoffice"]').textContent, 'Installed', 'the page did not repaint');
  status.libreoffice = false;
  $('helpersRecheck').click();
  await settle();
});

test('the panel shows while the mouse is on Open… and goes on Escape; focus shows it too', () => {
  assert.equal(panel.hidden, true, 'setup: the panel is up before anything asked for it');
  $('openMenuItem').dispatchEvent(new h.window.Event('mouseenter'));
  assert.equal(panel.hidden, false, 'hovering Open… did not show the panel');
  doc.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'Escape' }));
  assert.equal(panel.hidden, true, 'Escape did not put the panel away');
  doc.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'Tab' }));
  $('openMenuItem').dispatchEvent(new h.window.FocusEvent('focusin'));
  assert.equal(panel.hidden, false, 'tabbing to Open… did not show the panel');
  doc.body.dispatchEvent(new h.window.Event('pointerdown', { bubbles: true }));
  assert.equal(panel.hidden, true, 'a press elsewhere left the panel over the page');
});

test('focus given back by a dialog after a click does not raise the panel over the document', () => {
  doc.body.dispatchEvent(new h.window.Event('pointerdown', { bubbles: true }));
  $('openMenuItem').dispatchEvent(new h.window.FocusEvent('focusin'));
  assert.equal(panel.hidden, true, 'focus that came from a click (a dialog restoring it) showed the panel, which then covers the page');
});

test('a program that was not found is a link to its place on the Helper Programs page; a found one is not a link', async () => {
  $('openMenuItem').dispatchEvent(new h.window.Event('mouseenter'));
  assert.equal(panel.hidden, false, 'setup: the panel is not up, so "it went away" below would say nothing');
  const links = [...panel.querySelectorAll('a.hpwhy')];
  assert.deepEqual(links.map((a) => a.textContent), ['(needs LibreOffice, not found on this computer)'], 'the red lines are not the links, or a green one is');
  links[0].click();
  await settle();
  const page = $('settingsHelpersPage');
  assert.equal(page.hidden, false, 'the link did not open Settings → Helper Programs');
  assert.equal(doc.activeElement, $('helper-libreoffice-name'), 'the page opened, but not at LibreOffice');
  assert.equal(panel.hidden, true, 'the panel stayed up over the page it opened');
});
