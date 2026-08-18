// Every image the vendored pdf.js references is accounted for.
//
// The vendored tree references 32 images and ships ONE. Most of the missing 31 belong to
// pdf.js's own default viewer chrome — its toolbar, sidebar tree and message bars — which
// Nib replaces wholesale and correctly never loads. Seven were on paths Nib's UI DOES
// reach, and each was a live 404 with a missing visual: the editing cursors for the Text,
// Draw and Highlight tools, the two per-annotation toolbar icons (which rendered blank),
// and the page loading indicator. Found by reading the browser console during a live
// verification, which is not a method that scales.
//
// **This file is the half that matters**, and the reason is in how the gap appeared: it
// drifted because NOTHING checked it, and a pdf.js bump can add a reference exactly as
// quietly. TestNoticesUpToDate is the in-repo precedent for the shape — a check that
// closes a drift class rather than fixing one instance.
//
// Every referenced image must be in exactly one of three states, each with a reason:
//   vendored  — present in web/vendor/pdfjs/images/
//   supplied  — Nib overrides pdf.js's own hook for it (see web/style.css)
//   unused    — pdf.js default-viewer chrome Nib does not render
// A NEW reference is in none of them and fails, which is the point.
//
// Pure source scan: no boot.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const VENDOR = path.join(REPO, 'web', 'vendor', 'pdfjs');
const css = fs.readFileSync(path.join(VENDOR, 'pdf_viewer.css'), 'utf8');
const mjs = fs.readFileSync(path.join(VENDOR, 'pdf_viewer.mjs'), 'utf8');
const appCss = fs.readFileSync(path.join(REPO, 'web', 'style.css'), 'utf8');

const referenced = [...new Set(
  [...(css + mjs).matchAll(/images\/([a-zA-Z0-9_.-]+\.(?:svg|gif|png))/g)].map((m) => m[1]),
)].sort();

// pdf.js's own default-viewer chrome. Nib builds its whole UI itself — its toolbar, its
// sidebar, its dialogs — so nothing in the app renders these and vendoring them would add
// weight for no pixel. Listed by name rather than matched by prefix: a prefix rule would
// silently absorb a future asset that IS reachable.
const UNUSED = new Set([
  'altText_add.svg', 'altText_disclaimer.svg', 'altText_done.svg', 'altText_spinner.svg',
  'altText_warning.svg', 'checkmark.svg', 'comment-closeButton.svg',
  'comment-popup-editButton.svg', 'messageBar_closingButton.svg', 'messageBar_info.svg',
  'messageBar_warning.svg', 'pages_closeButton.svg', 'pages_selected.svg',
  'pages_viewArrow.svg', 'pages_viewButton.svg', 'toolbarButton-currentOutlineItem.svg',
  'toolbarButton-editorHighlight.svg', 'toolbarButton-menuArrow.svg',
  'toolbarButton-pageDown.svg', 'toolbarButton-viewAttachments.svg',
  'toolbarButton-viewLayers.svg', 'toolbarButton-viewOutline.svg',
  'toolbarButton-zoomIn.svg', 'treeitem-collapsed.svg', 'treeitem-expanded.svg',
]);

// The seven Nib supplies. loading-icon.gif is supplied as a CSS animation rather than a
// file, because pdf.js hard-codes its url() in a rule instead of behind a custom property
// — so it is checked by its override rule, not by a path.
const SUPPLIED = new Map([
  ['cursor-editorFreeText.svg', 'img/pdfjs/cursor-editorFreeText.svg'],
  ['cursor-editorInk.svg', 'img/pdfjs/cursor-editorInk.svg'],
  ['cursor-editorFreeHighlight.svg', 'img/pdfjs/cursor-editorFreeHighlight.svg'],
  ['cursor-editorTextHighlight.svg', 'img/pdfjs/cursor-editorTextHighlight.svg'],
  ['editor-toolbar-delete.svg', 'img/pdfjs/editor-toolbar-delete.svg'],
  ['editor-toolbar-edit.svg', 'img/pdfjs/editor-toolbar-edit.svg'],
  ['loading-icon.gif', null], // replaced by the .page.loadingIcon::after override
]);

test('the scan reads the vendored tree it claims to', () => {
  // The stimulus. A regex that matched nothing would report every image accounted for.
  assert.ok(referenced.length >= 30,
    `only ${referenced.length} image references found in the vendored pdf.js — the scan is not reading it`);
  assert.ok(referenced.includes('loading-icon.gif'),
    'loading-icon.gif is not among the references, so this scan is missing the hottest one — it is set on every page while it renders');
});

test('every image the vendored pdf.js references is vendored, supplied, or documented unused', () => {
  const unaccounted = referenced.filter(
    (img) => !UNUSED.has(img) && !SUPPLIED.has(img)
      && !fs.existsSync(path.join(VENDOR, 'images', img)),
  );
  assert.deepEqual(unaccounted, [],
    `the vendored pdf.js references these and Nib neither ships them, supplies a replacement, nor records them as chrome it does not render — each is a live 404 in the browser and a missing visual: ${unaccounted.join(', ')}. Add the file, add an override in web/style.css and an entry to SUPPLIED, or add it to UNUSED with the reason it is never rendered.`);
});

test('every supplied replacement exists and is wired up', () => {
  const missingFile = [];
  const missingWiring = [];
  for (const [img, rel] of SUPPLIED) {
    if (rel && !fs.existsSync(path.join(REPO, 'web', rel))) missingFile.push(rel);
    if (rel && !appCss.includes(rel)) missingWiring.push(rel);
  }
  assert.deepEqual(missingFile, [],
    `SUPPLIED names replacement assets that do not exist: ${missingFile.join(', ')}`);
  assert.deepEqual(missingWiring, [],
    `these replacements exist but web/style.css never points pdf.js at them, so the vendored url() still wins and the 404 is unchanged: ${missingWiring.join(', ')}`);
  // The one that is not a file.
  assert.ok(/\.page\.loadingIcon::after\s*\{[^}]*animation:/.test(appCss),
    'loading-icon.gif is supplied as a CSS animation, and the .page.loadingIcon::after override that provides it is gone — the vendored rule then requests a GIF that is not there, on every page render');
});

test('the UNUSED list does not name anything that is actually referenced nowhere', () => {
  // The reverse direction: an entry excusing an image pdf.js no longer references is a
  // stale excuse, and a stale excuse is where the next reachable asset gets parked.
  const stale = [...UNUSED].filter((img) => !referenced.includes(img)).sort();
  assert.deepEqual(stale, [],
    `these are excused as unused pdf.js chrome but the vendored tree no longer references them at all — a pdf.js bump dropped them, and the list is now describing nothing: ${stale.join(', ')}`);
});
