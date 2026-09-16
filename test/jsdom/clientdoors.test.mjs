// /pending 506 — the web client's doors that a keyboard user, or a wrong passphrase, could not get
// through.
//
// Driven here: a wrong certificate passphrase is reported (it used to be a 401 that `apiFetch`
// swallowed as "locked"), a library card can be used from the keyboard, and closing a tab leaves focus
// somewhere a keyboard user can continue from. Held by census: the one door that disarms every tool,
// the one door that makes a click-only element keyboard-operable, and the fillable-form bake.
//
// ## What this cannot see
//
// - Whether a screen reader announces the new names and roles. No tier runs AT.
// - The sign lock BEHAVIOURALLY: locking needs a placed flag, and placing one needs page geometry
//   jsdom does not have. The census below proves every tool's exit is called from the lock's door;
//   that an armed Border's crosshair actually goes away is tier 3's.
// - The fillable form's OUTPUT (which marks are in the authored PDF): that needs overlay fields
//   placed on real pages and the server's bake. The census proves the bake is called with the
//   exclusion; the bytes are tier 1/3's.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const h = await boot({
  routes: {
    '/api/images': [{ id: 'img1', name: 'My signature', builtin: true }],
    '/api/identity/external': (opts) => (opts.method === 'POST'
      ? new Response(JSON.stringify({ error: 'wrong passphrase, or not a PKCS#12 file' }), {
        status: 422, headers: { 'Content-Type': 'application/json' },
      })
      : { present: false }),
    '/api/peers': { self: 'f'.repeat(64), peers: [] },
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      const name = p.split('/').pop();
      return { id: 'cd:' + name, name, path: p, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
    },
  },
});
const { document: doc, window: win, settle } = h;
const toastText = () => (doc.getElementById('toast') || { textContent: '' }).textContent;

test('a wrong certificate passphrase says so, and puts the user back in the passphrase field', async () => {
  const file = doc.getElementById('extP12File');
  const pass = doc.getElementById('extP12Pass');
  assert.ok(file && pass, 'setup: the certificate import controls are not in index.html');
  Object.defineProperty(file, 'files', { configurable: true, value: [new win.File([new Uint8Array([1])], 'id.p12')] });
  pass.value = 'not it';

  const rejections = [];
  const onRej = (e) => rejections.push(e);
  process.on('unhandledRejection', onRej);
  doc.getElementById('extP12Import').click();
  await settle();
  process.off('unhandledRejection', onRej);

  // Stimulus: the import actually went out.
  assert.ok(h.calls.some((c) => c.url.endsWith('/api/identity/external') && c.method === 'POST'),
    'setup: the import never reached the server');
  assert.match(toastText(), /Wrong passphrase/,
    'a wrong passphrase showed nothing — the handler\'s branch never ran');
  assert.equal(doc.activeElement, pass, 'focus was not returned to the passphrase field to try again');
  assert.deepEqual(rejections, [], 'the import left an unhandled rejection behind');
});

test('a library card is a keyboard control: focusable, named, and Enter does what a click does', async () => {
  const card = doc.querySelector('#imageGrid .libimg');
  assert.ok(card, 'setup: the library rendered no card from /api/images');
  assert.equal(card.tabIndex, 0, 'the card is not in the tab order — Place a signature has no keyboard path');
  assert.equal(card.getAttribute('role'), 'button', 'the card has no role, so a reader announces an image and a name, not a control');
  assert.equal(card.getAttribute('aria-label'), 'Place My signature', 'the card is not named for what pressing it does');

  // With no document open, activating a card says so — which is the observable that the ACTION ran.
  card.dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  await settle();
  assert.match(toastText(), /Open a PDF first/, 'Enter on a focused card did nothing — the click handler has no keyboard twin');
});

async function openDoc(name) {
  setNextDocument({ numPages: 1 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/' + name;
  doc.getElementById('openGo').click();
  await settle();
}
const tabs = () => [...doc.querySelectorAll('#tabstrip .tab')];

test('closing a tab from its × leaves focus on the tab strip, or on the menubar when the strip goes', async () => {
  await openDoc('one.pdf');
  await openDoc('two.pdf');
  await openDoc('three.pdf');
  assert.equal(tabs().length, 3, 'setup: three documents are not open');

  // Close the middle tab by keyboard: focus its × and press it.
  const x = tabs()[1].querySelector('.tabclose');
  x.focus();
  assert.equal(doc.activeElement, x, 'setup: the × could not take focus');
  x.click();
  await settle(30);
  assert.equal(tabs().length, 2, 'setup: the close did not happen');
  assert.ok(doc.activeElement && doc.activeElement.classList.contains('tab'),
    `focus fell to ${doc.activeElement && doc.activeElement.tagName} after closing a tab — a keyboard user is thrown to the top of the page`);

  // And when the strip itself disappears (one document left), focus lands on the menubar.
  const x2 = tabs()[1].querySelector('.tabclose');
  x2.focus();
  x2.click();
  await settle(30);
  assert.equal(tabs().length, 0, 'setup: the strip did not go away with one document left');
  const menubarFirst = doc.getElementById('menubar')?.querySelector('button');
  assert.equal(doc.activeElement, menubarFirst,
    `focus fell to ${doc.activeElement && doc.activeElement.tagName} when the tab strip went away`);
});

// ── Census ──────────────────────────────────────────────────────────────────────────────────────

function bodyOf(header) {
  const at = APP.indexOf(header);
  if (at === -1) return null;
  const open = APP.indexOf('{', at + header.length - 1);
  let d = 0;
  for (let j = open; j < APP.length; j++) {
    if (APP[j] === '{') d++;
    else if (APP[j] === '}') { d--; if (d === 0) return APP.slice(open, j + 1); }
  }
  return null;
}

test('locking the marks disarms EVERY drawing tool, through one door', () => {
  const lock = bodyOf('function setSignLocked(locked) {');
  assert.ok(lock, 'setup: setSignLocked is gone');
  assert.match(lock, /if \(locked\) disarmEditingTools\(\);/,
    'locking does not go through disarmEditingTools, so it disarms whatever its own list remembers');

  const door = bodyOf('function disarmEditingTools() {');
  assert.ok(door, 'disarmEditingTools is gone');
  // The population is every exit the file defines — so tool number twelve is covered by existing.
  const exits = [...APP.matchAll(/^function (exit\w+)\(\)/gm)].map((m) => m[1]).filter((n) => n !== 'exitFullScreen');
  assert.ok(exits.length >= 8, `only ${exits.length} exit functions found — the census is not reading app.js`);
  const missed = exits.filter((n) => !new RegExp(`\\b${n}\\(\\)`).test(door));
  assert.deepEqual(missed, [], `locking leaves these tools armed on a document it calls uneditable: ${missed.join(', ')}`);
  for (const [re, what] of [
    [/setMarkerMode\(null\)/, 'the flag tools'],
    [/view\.redactMode = false/, 'redaction'],
    [/view\.editMode = false/, 'Edit text'],
    [/if \(view\.activeTool\) setTool\(view\.activeTool\)/, 'the pdf.js Text/Highlight/Draw modes'],
  ]) assert.match(door, re, `disarmEditingTools does not disarm ${what}`);
});

test('the three click-only surfaces go through the keyboard door, and the door does its job', () => {
  const door = bodyOf('function makeActivatable(el, action, label) {');
  assert.ok(door, 'makeActivatable is gone');
  assert.match(door, /el\.tabIndex = 0;/, 'the door does not put the element in the tab order');
  assert.match(door, /setAttribute\('role', 'button'\)/, 'the door gives the element no role');
  assert.match(door, /e\.key === 'Enter' \|\| e\.key === ' '/, 'the door binds no activation key');
  assert.match(door, /if \(e\.target !== el\) return;/,
    'a key pressed on a control inside the element (the card\'s delete ×) would also activate the element');

  const outline = bodyOf('async function buildOutline(gen = view.docGen, owner = view) {');
  assert.match(outline, /makeActivatable\(a, /, 'outline entries are <a> with no href — not focusable — and do not use the door');
  const browse = bodyOf('async function browseDir(path, t = saveAsDirEls(), onFile = null) {');
  assert.match(browse, /if \(onclick\) makeActivatable\(li, onclick\);/, 'folder rows are click-only again');
  assert.doesNotMatch(browse, /li\.onclick = /, 'a folder row sets its own onclick, bypassing the door');
});

test('Save as fillable form bakes the document the user sees, minus only the fields it authors', () => {
  const go = bodyOf('els.fieldNameGo.onclick = async () => {');
  assert.ok(go, 'setup: the fillable-form handler is gone');
  assert.match(go, /await bakedBytes\(opDoc && opDoc\.id, owner, exclude\)/,
    'the fillable form is not built from the baked document, so edits, stamps and notes are dropped');
  // Comments stripped: the handler's own comment names the two calls it no longer makes, and a guard
  // that reads prose as code fails on its documentation (seen on the first run of this test).
  const code = go.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/[^\n]*/g, '$1');
  assert.doesNotMatch(code, /saveDocument\(\)|getData\(\)/,
    'the fillable form reads the raw bytes again, dropping everything Nib draws through the bake');
  assert.match(go, /new Set\(pendingAuthor\.map\(\(f\) => f\.src\)/, 'the exclusion is not the authored fields');
  for (const header of ['function collectFieldsWithSources(owner = view, exclude = null) {', 'function collectStamps(owner = view, exclude = null) {']) {
    const body = bodyOf(header);
    assert.ok(body, `setup: ${header} is gone`);
    assert.match(body, /if \(exclude && exclude\.has\(f\)\) continue;/,
      `${header.split('(')[0]} ignores the exclusion, so an authored field is burned in underneath its own widget`);
  }
});
