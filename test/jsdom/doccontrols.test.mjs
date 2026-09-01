// /pending 331 — a control that needs a document must be disabled without one, and the
// rule has to reach every control on the row rather than the first three of it.
//
// Five drawing tools (Border, Note, Dropdown, Radio, Shapes) sat beside Text/Highlight/
// Draw in the Edit tab and were in NEITHER list: not `DOC_REQUIRED`, so they were live on
// a clean vault with nothing open; and not `EDITING_TOOLS`, so they stayed armable after
// signing mode toasted "the document is in signing mode and can no longer be edited".
// The second is the worse one and was found while fixing the first.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The lists themselves, and the partition below — which is the part that catches a SIXTH
// tool added later. Asserting the five by name would go green the day someone adds a
// sixth, which is exactly how these five arrived.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// Whether a disabled button is actually unclickable in a real engine, and whether the
// tool arms. Tier 3 drives the real UI.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');
const HTML = fs.readFileSync(path.join(REPO, 'web', 'index.html'), 'utf8');

// The declared list, read out of the source rather than restated here — a copy of the
// list in the test is a second list to keep in step, and it agrees with itself forever.
function listNamed(name) {
  const m = APP.match(new RegExp(`const ${name} = \\[([\\s\\S]*?)\\n\\];`));
  assert.ok(m, `${name} is not in web/app.js in the shape this scan reads — the guard is reading nothing`);
  return [...m[1].matchAll(/'([A-Za-z][A-Za-z0-9]*)'/g)].map((x) => x[1]);
}

// The Edit tab's own button ids, discovered from the markup.
function editRowButtons() {
  const i = HTML.indexOf('<div class="tbtab" data-tab="edit">');
  assert.notEqual(i, -1, 'the Edit toolbar tab is not in web/index.html — this scan is reading nothing');
  const j = HTML.indexOf('<div class="tbtab"', i + 10);
  const seg = HTML.slice(i, j === -1 ? undefined : j);
  return [...seg.matchAll(/<button[^>]*id="([^"]+)"/g)].map((m) => m[1]);
}

// Undo/Redo are gated by reflectUndoControls off canUndo/canRedo, not by the open/closed
// state — enabling them for an open document with no history would be wrong. Edit profile
// is a preferences control and acts on no document. Both are exemptions NAMED at the
// site, which is what ADR-009 asks for in place of a silent omission.
const EXEMPT = new Set(['undoBtn', 'redoBtn', 'editProfileBtn']);

test('every Edit-tab control needs a document, or is a named exemption', () => {
  const required = new Set(listNamed('DOC_REQUIRED'));
  const missing = editRowButtons().filter((id) => !EXEMPT.has(id) && !required.has(id));
  assert.deepEqual(missing, [],
    `${missing.join(', ')} sit in the Edit tab and are not in DOC_REQUIRED, so they are clickable with nothing open. Add them, or add a named exemption with the reason`);
});

test('every drawing tool is switched off by signing mode', () => {
  // The tools that put CONTENT on the page. Derived from DOC_REQUIRED's own drawing
  // block rather than hand-listed, so the two stay in step.
  const drawing = ['textToolBtn', 'highlightToolBtn', 'drawToolBtn',
    'borderBtn', 'noteBtn', 'dropdownBtn', 'radioBtn', 'shapeBtn'];
  const editing = new Set(listNamed('EDITING_TOOLS'));
  const missing = drawing.filter((id) => !editing.has(id));
  assert.deepEqual(missing, [],
    `${missing.join(', ')} draw content onto the page and are not in EDITING_TOOLS, so signing mode leaves them armable — while the app has just said the document "can no longer be edited"`);
});

test('EDITING_TOOLS stays a strict subset of DOC_REQUIRED', () => {
  // app.js:2372 states this as a fact and orders two calls by it. Nothing checked it, and
  // an id in EDITING_TOOLS but not DOC_REQUIRED would be re-enabled by the editing pass
  // on a document that is not open.
  const required = new Set(listNamed('DOC_REQUIRED'));
  const stray = listNamed('EDITING_TOOLS').filter((id) => !required.has(id));
  assert.deepEqual(stray, [],
    `${stray.join(', ')} are in EDITING_TOOLS but not DOC_REQUIRED — app.js's ordering comment says the first is a strict subset of the second, and setEditingEnabled would re-enable them with nothing open`);
});
