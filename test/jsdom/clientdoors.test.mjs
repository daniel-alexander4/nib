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
    // Closing the LAST document is a close-all: `closeView` hands off to `requestClose`, which
    // posts here. Reached since ADR-037, because the strip now survives down to one document and
    // the focus-fallback case moved to closing that one.
    '/api/close': { status: 'ok' },
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

  // Down to ONE document, which since ADR-037 still has a strip and still has a tab — so focus
  // stays in the strip rather than falling back. This step used to be the fallback case, and it
  // is now the last chance to prove the ordinary path once more.
  const x2 = tabs()[1].querySelector('.tabclose');
  x2.focus();
  x2.click();
  await settle(30);
  assert.equal(tabs().length, 1,
    'closing down to one document emptied the strip — ADR-037 shows it at one, and the tab that '
    + 'remains is the open document');
  assert.ok(doc.activeElement && doc.activeElement.classList.contains('tab'),
    `focus fell to ${doc.activeElement && doc.activeElement.tagName} after closing down to one `
    + 'document, where a tab is still there to hold it');

  // And when the strip itself disappears — which is now closing the LAST document — focus lands on
  // the menubar. The property is unchanged; the point at which it fires moved by one close.
  const x3 = tabs()[0].querySelector('.tabclose');
  x3.focus();
  x3.click();
  await settle(30);
  assert.equal(tabs().length, 0, 'setup: the strip did not go away when the last document closed');
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

// ── The missing converter says so (ADR-040) ─────────────────────────────────────────────────
//
// boot.mjs answers /api/status with `libreoffice: false`, so this file is already booted into
// the state under test. Before ADR-040 the ONLY consequence of that flag was `officeInput.accept`
// narrowing to Markdown — a user with a .docx met the missing converter as their file not
// appearing in the file dialog, and nothing named the reason.
test('with no LibreOffice, the File card says so and offers a way back', async () => {
  // Stimulus: the status this assertion depends on was actually asked for and applied.
  assert.ok(h.calls.some((c) => c.url.endsWith('/api/status')),
    'setup: /api/status was never fetched, so loAvailable is its declared default rather than the server\'s answer');
  assert.equal(doc.getElementById('officeInput').accept, '.md,.markdown',
    'setup: the picker was not narrowed, so this boot is not in the LibreOffice-absent state');

  const shown = (sel) => { const el = doc.querySelector(sel); return !!el && !el.hidden; };
  assert.ok(shown('#officeMissing'),
    'nothing tells the user why their .docx cannot be converted — the absence is communicated only '
    + 'by the file picker quietly narrowing, which is what ADR-040 exists to end');
  assert.ok(shown('#officeRecheck'),
    'there is no way to re-ask after installing LibreOffice, so the only remedy is restarting Nib '
    + '— which hands off to the running instance and is not a thing a user can easily do');

  // It is a standing condition, not an event: it must not evaporate the way a toast does.
  await settle();
  assert.ok(shown('#officeMissing'), 'the explanation disappeared on its own');

  // The link is authored in index.html, never composed from a response body (ADR-039's
  // reasoning applied to navigation), so it is here to be read rather than built.
  const link = doc.querySelector('#officeMissing a');
  assert.ok(link && /^https:\/\//.test(link.getAttribute('href') || ''),
    'the explanation carries no https link, so it names a remedy the user cannot reach');
  assert.equal(link.getAttribute('rel'), 'noopener', 'the outbound link has no rel="noopener"');

  // The button is always offered, because Markdown needs no converter. app.js claimed for
  // years that it was "hidden otherwise" and nothing ever implemented that.
  assert.ok(shown('#officeOpenBtn'),
    'the convert button was hidden — Markdown converts in pure Go, so hiding it removes a '
    + 'feature that works in order to report one that does not');
});

test('with no Ghostscript, a refused PDF/A says a tool would have converted it', () => {
  // Census rather than a driven click, and deliberately: reaching the real branch needs an open
  // document and the modal, and this file's document state belongs to the tab-closing test above.
  // The branch is what matters — before it, `#pdfaGsGo` was revealed ONLY when gs was present
  // (`if (!gs && gsAvailable)`), so a user WITHOUT it was told the document was refused and never
  // that a tool exists which would have converted it. The refusal at internal/server/pdfa.go was
  // literally unreachable from the GUI.
  const run = bodyOf('async function runPdfa(engine) {');
  assert.ok(run, 'setup: runPdfa is gone');
  assert.match(run, /\} else if \(!gs\) \{/,
    'runPdfa has no branch for "the pure-Go path refused AND Ghostscript is absent" — that user is '
    + 'told only that their document was refused, with no mention of the tool that would convert it');
  // Plain "could not", not "couldn't": this reads app.js as SOURCE TEXT, where an apostrophe inside
  // a single-quoted string is escaped (`couldn\'t`) and a naive /couldn't/ can never match. Caught by
  // this assertion failing on its first run — the string was right and the probe was wrong.
  assert.match(run, /could not find it/,
    'the Ghostscript-absent branch does not say Nib could not FIND it — ADR-040 turns on that wording, '
    + 'because exec.LookPath cannot distinguish "absent" from "installed off PATH"');

  // And the line that carries it must announce itself: it changes while the dialog is open, which
  // is SC 4.1.3. It had neither role nor aria-live, so a screen-reader user heard nothing at all.
  const status = doc.getElementById('pdfaStatus');
  assert.ok(status, 'setup: #pdfaStatus is gone');
  assert.equal(status.getAttribute('role'), 'status',
    '#pdfaStatus carries no role, so the refusal reason is written into a silent node');
  assert.equal(status.getAttribute('aria-live'), 'polite',
    '#pdfaStatus is not a live region, so a textContent update inside an already-open dialog is '
    + 'announced to nobody');
});

test('a machine WITH LibreOffice is told nothing — the explanation is not a standing ornament', () => {
  // The same door, driven the other way. Without this, an element that was never hidden would
  // pass the test above for the wrong reason.
  const apply = bodyOf('function applyStatus(st) {');
  assert.ok(apply, 'setup: applyStatus is gone');
  for (const [re, what] of [
    [/els\.officeMissing\.hidden = loAvailable/, 'the explanation'],
    [/els\.officeRecheck\.hidden = loAvailable/, 'the Check again button'],
  ]) {
    assert.match(apply, re,
      `${what} is not tied to loAvailable in applyStatus, so it is shown to users who have `
      + 'LibreOffice installed and nothing to fix');
  }
});

// ── /pending 513 — every ARM goes through the disarm door too ────────────────────────────────────
//
// /pending 506 gave the lock one door for "nothing is armed". It left eleven other sites each
// writing the same rule out by hand — "arming this one puts down the others" — and the eleven
// disagreed with each other in thirteen places, which is ADR-009's failure exactly: Border left
// Shape armed; Dropdown, Radio and the new-document teardown left Checkbox armed; Note left
// Dropdown, Radio and Checkbox armed; Redact left Edit text armed; four sites left the pdf.js
// Text/Highlight/Draw mode live underneath a box tool; and `setTool`, arming that mode, left Crop,
// Split-by-box and the flag tools alone. Two box tools share one `pointerdown` on `#viewerWrap`,
// and a live pdf.js editor layer takes the pointer outright — so each gap is a tool the user has
// lit and cannot draw with.
//
// **The population is DISCOVERED and not listed**, because ADR-009's own words are that eight
// copies checked for agreement say nothing about a ninth site added without one. A roster typed
// into this file would be that ninth site's blind spot. The scan looks for the act itself: turning
// a drawing mode on.
test('every arm puts the other tools down through the one door, and none keeps its own list', () => {
  const lines = APP.split('\n');
  // Approximate a handler as the region back to the nearest module-level binding — pinning.test.mjs
  // uses the same coarse rule and for the same reason: it over-reports rather than under-reports,
  // and a false positive here is a loud question about a real handler.
  const starts = lines.reduce((acc, l, i) => {
    if (/^(els\.[\w.]+ = |function |const \w+ = (async )?\(|let )/.test(l)) acc.push(i);
    return acc;
  }, [0]);
  const arms = [];
  lines.forEach((l, i) => {
    if (l.trim().startsWith('//')) return;
    const m = /view\.([A-Za-z]+Mode) = (true|on)\b/.exec(l);
    if (!m) return;
    const prev = Math.max(...starts.filter((s) => s <= i));
    arms.push({ mode: m[1], at: i + 1, body: lines.slice(prev, i + 1).join('\n') });
  });
  // The stimulus floor: ten tools arm this way today, and a scan reading none of them would pass
  // every assertion below.
  assert.ok(arms.length >= 9,
    `only ${arms.length} arming sites found — the scan is not reading what it thinks`);

  const unrouted = arms.filter((a) => !a.body.includes('disarmEditingTools()'))
    .map((a) => `${a.at}: view.${a.mode}`);
  assert.deepEqual(unrouted, [],
    'an arm turns its mode on without going through disarmEditingTools, so it puts down whatever '
    + `its own list happens to remember — and every such list has been wrong:\n  ${unrouted.join('\n  ')}`);

  // **That assertion also carries the ORDERING, and it is worth saying why rather than writing a
  // second check that cannot fail.** Each region above ends AT the line that turns the mode on, so
  // a door call found inside it is a door call that runs before the mode is written — which is the
  // property that matters: the door ends in `exitX()` for every tool, so an arm that set its flag
  // first would have the door switch it straight back off, and the tool would light up and disarm
  // in the same click. A separate "is the door earlier than the assignment" check over this slice
  // is true by construction, so it is stated here instead of asserted.

  // The two that cannot join the population above, because neither writes a `<x>Mode = true`: the
  // pdf.js editor modes and the flag tools. Both RE-ENTER the door — it ends by calling each of
  // them — so both are asserted by name rather than trusted to the scan.
  const tool = bodyOf('function setTool(mode) {');
  assert.ok(tool, 'setup: setTool is gone');
  assert.match(tool, /if \(on\) disarmEditingTools\(\);/,
    'setTool arms a pdf.js editor mode over whatever Nib-side tools its own list forgets — it '
    + 'forgot Crop, Split-by-box and every flag tool');
  assert.doesNotMatch(tool, /\bexit[A-Z]\w*\(\)/,
    'setTool still names individual exits, so the door and this list can disagree again');

  const marker = bodyOf('function setMarkerMode(m) {');
  assert.ok(marker, 'setup: setMarkerMode is gone');
  assert.match(marker, /disarmEditingTools\(\);/,
    'arming a flag tool keeps its own list of what to put down — the copy that left the pdf.js '
    + 'editor layer live over the page the flag is being placed on');
  assert.doesNotMatch(marker, /\bexit[A-Z]\w*\(\)/,
    'setMarkerMode still names individual exits, so the door and this list can disagree again');
  // The ordering argument again, and here it is load-bearing in a second way: the door calls
  // `setMarkerMode(null)`, so this function re-enters itself. Writing the mode after the door is
  // what stops that re-entry clearing the mode being armed.
  assert.ok(marker.indexOf('disarmEditingTools();') < marker.indexOf('view.markerMode = m;'),
    'setMarkerMode writes the mode before calling the door, and the door calls setMarkerMode(null) '
    + '— so arming a flag tool clears it in the same call');
});
