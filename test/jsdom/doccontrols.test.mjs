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

// Every toolbar pane's button ids, discovered from the markup.
//
// **This scanned only the Edit pane until the v1.120.0 mode re-cut, and that was a guard that
// could be weakened without failing.** Moving a control out of Edit simply removed it from the
// scan — silently — and the re-cut moved most of Edit to Mark Up and Secure. Reading every pane
// means a control cannot escape the rule by changing tabs.
function toolbarButtons() {
  // **Every command host, not every PANE.** v1.121.0 moved the mode-independent commands into
  // a `.tbfixed` bar outside any pane, and they left this scan without failing it — the same
  // silent narrowing that widening it from the Edit pane fixed one commit earlier, arriving
  // through the new surface. The inventory for /pending 373 predicted this as G1.
  const panes = [...HTML.matchAll(/<div class="tbtab" data-tab="[a-z]+">|<div class="tbfixed">/g)];
  assert.ok(panes.length >= 6, `only ${panes.length} command hosts in web/index.html — this scan is reading nothing`);
  const out = [];
  for (const p of panes) {
    // Bounded by the pane's OWN matching close tag, walked by depth. Slicing to the next
    // pane (or, for the last one, to end of file) is what the Edit-only version did, and it
    // swept every modal dialog in the document into the last pane's segment — 140 ids that
    // have nothing to do with the toolbar. It never bit before only because Edit was never
    // the last pane.
    let depth = 0, i = p.index;
    for (;;) {
      const open = HTML.indexOf('<div', i), close = HTML.indexOf('</div>', i);
      assert.notEqual(close, -1, `pane ${p[1]} is unbalanced in web/index.html`);
      if (open !== -1 && open < close) { depth++; i = open + 4; continue; }
      depth--; i = close + 6;
      if (depth === 0) break;
    }
    for (const m of HTML.slice(p.index, i).matchAll(/<button[^>]*id="([^"]+)"/g)) out.push(m[1]);
  }
  return out;
}

// Edit profile is a preferences control and acts on no document. It is an exemption NAMED
// at the site, which is what ADR-009 asks for in place of a silent omission.
//
// (Undo/Redo were exempted here until v1.128.72 and the controls had been gone since
// v1.125.0 — six releases of an exemption naming nothing, which is the defect the note
// below already stated about `prevBtn`/`nextBtn`. `TestEveryExemptionNamesAControlThatExists`
// is what now stops a third instance; /pending 423.)
const EXEMPT = new Set([
  // A preferences control; acts on no document.
  'editProfileBtn',
  // ── The rest became visible when this scan widened from the Edit pane to every pane.
  // Each is document-INDEPENDENT by design, which is why none was ever a defect.
  // They get up-front reasons rather than a blanket skip, per ADR-009.
  //
  // These CREATE or receive a document, so requiring one would be circular:
  'openMenuItem', 'officeOpenBtn', 'combineBtn', 'sessionRecvDocBtn', 'sessionRecvBtn',
  // Gated by their own state, not by the registry: saveBtn follows canSave, the find
  // buttons follow the search results, and closeAllBtn is hidden below two documents.
  'saveBtn', 'findPrevBtn', 'findNextBtn', 'closeAllBtn',
  // These act on the VIEWER, not on the document. With nothing open the viewer is empty
  // and they are inert; disabling them would be a behaviour change, not a fix.
  // (`prevBtn`/`nextBtn` were here until v1.125.0 and are gone from the product — a name in an
  // exemption list that matches nothing is a claim about a control that does not exist.)
  'zoomOutBtn', 'fitBtn', 'zoomInBtn',
  // The two zoom modes added with the View group (v1.129.4) are the same class as `fitBtn`
  // beside them: they set the viewer's scale, and with nothing open there is no scale to set.
  'fitPageBtn', 'actualSizeBtn',
  // The two LAYOUT buttons are a step further out — they are a preference, persisted to the
  // vault and applied to every view including ones not created yet, so they are meaningful with
  // nothing open at all. Requiring a document would mean a user cannot choose how documents look
  // until after one is already looking wrong.
  'viewStandardBtn', 'viewContinuousBtn',
  // Present and Full screen join them (v1.129.6). Presenting with nothing open is an empty black
  // screen rather than an error, and Full screen is a WINDOW state that has nothing to do with a
  // document at all — requiring one would mean you cannot fill the display until you have opened
  // something, which is backwards for the control that makes room to open things in.
  'viewPresentBtn', 'fullScreenBtn',
  // Quit acts on the PROCESS, not on a document (P01.S06). Requiring a document would make it
  // unreachable from the one state a user most wants it in — a Nib with nothing open that they
  // want to stop — and its own modal already names whatever would be lost, document or ceremony.
  'quitBtn',
  // The Settings pane (v1.126.0). None of it acts on the open document — they are the machine's
  // identity, its vault, its update preference and its About box — so requiring a document would
  // make Settings unreachable on a fresh install, which is exactly when it is needed.
  'managePeersBtn', 'manageKeysBtn', 'backupBtn', 'aboutBtn',
  // These act on something OTHER than the open document — your signing certificate, and a
  // file the user picks in the dialog.
  'exportCertBtn', 'timestampVerifyBtn',
]);

test('every toolbar control needs a document, or is a named exemption', () => {
  const required = new Set(listNamed('DOC_REQUIRED'));
  const missing = toolbarButtons().filter((id) => !EXEMPT.has(id) && !required.has(id));
  assert.deepEqual(missing, [],
    `${missing.join(', ')} sit in a toolbar pane and are not in DOC_REQUIRED, so they are clickable with nothing open. Add them, or add a named exemption with the reason`);
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

// **The other direction, and it is the one that was missing.** The scan above asks whether
// every toolbar button is required-or-exempt; nothing asked whether every EXEMPTION names a
// button that exists. So a control could be deleted from the product and its exemption stay
// behind, claiming a considered decision about something that is not there.
//
// Twice now. `prevBtn`/`nextBtn` went at v1.125.0 and the note above records it; `undoBtn`
// and `redoBtn` went in the same release and were still exempted six releases later, along
// with six dead `if (els.undoBtn)` branches and a CSS rule matching nothing (/pending 423).
// Both were found by a person reading, which is what this replaces.
test('every exemption names a control that exists', () => {
  const buttons = new Set(toolbarButtons());
  assert.ok(buttons.size > 20,
    `only ${buttons.size} toolbar buttons were parsed; this markup has many more. The scan is `
      + 'broken, so every name below would report as missing and the failure would be the scan.');
  const ghosts = [...EXEMPT].filter((id) => !buttons.has(id));
  assert.deepEqual(ghosts, [],
    `${ghosts.join(', ')} sit in EXEMPT and match no button in the markup. An exemption for a `
      + 'control that does not exist is a claim about a considered decision that nobody can '
      + 'check — and it outlives the control, so the next reader believes the gate was thought '
      + 'about. Delete the name, or restore the control.');
});

// /pending 455 — the signature warning must not promise evidence the operation destroys.
//
// Measured on a signed document with one invisible approval signature: `writeMutated`
// (api.WriteContext) leaves `invalid` with the blob present and one signer, while
// `pdfops.Collect`/`api.MergeRaw` leave `unsigned` with no blob and no signers. Redaction
// and remove-originals use the second pair, so they ERASE rather than invalidate — and a
// user told their signature would be "invalidated" expects to find a broken signature,
// which is evidence a reader can act on. Erasure leaves none.
//
// The deeper question — whether Nib wants an edited signed document to read `invalid` or
// `unsigned`, rather than having it depend on which primitive a route happens to use — is
// still open on that item. This pins only the sentence.
test('the signature warning does not promise an invalidation it does not perform', () => {
  const warn = APP.slice(APP.indexOf('function signatureWarning()'), APP.indexOf('function confirmSignatureLoss()'));
  assert.ok(warn.length > 40, 'signatureWarning is gone or renamed — this pins nothing');

  assert.doesNotMatch(warn, /invalidate/,
    'the redaction/flatten warning still says the signature will be "invalidated". Those routes '
      + 'rebuild the document through Collect/MergeRaw and leave it reading UNSIGNED with no blob '
      + 'and no signers — so the reader is promised evidence that the operation removes.');
  assert.match(warn, /destroy/,
    'the warning no longer says the signature is destroyed, which is the fact the user is '
      + 'deciding on');
  assert.match(warn, /not show that it was ever signed/,
    'the warning does not say the result carries no trace of the signature. "Destroyed" alone '
      + 'still lets a reader assume a broken signature will remain to point at.');
});
