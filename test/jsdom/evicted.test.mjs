// /pending 462 — the answer to a Ctrl+Z that finds nothing, when the reason is an eviction.
//
// ADR-003's history budget is GLOBAL and drops an inactive document's history WHOLE, because a
// partially-trimmed history is a silent loss by construction. The ADR's own condition is that
// eviction is observable or it is not eviction — and `canUndo: false` reads identically for "you
// have made no edits" and "your edits are no longer undoable", which is precisely the ambiguity the
// whole-history rule exists to resolve.
//
// **It had no standing surface.** The notice used to be delivered on the ↶ control, and v1.125.0
// deleted the ↶/↷ buttons: `undoAny` is reached from the key handler alone, and the branch that set
// the button's title sat behind `if (els.undoBtn)` — permanently false — for six releases, until
// /pending 423 removed it. What survived is a toast fired at the MOMENT of eviction, which is the
// delivery that site's own comment rejects: the budget is global, so an eviction happens while the
// user is working on another document and is gone before they look at this one.
//
// So the answer is given at the press. What this tier can see is exactly that: which keystroke
// produces which sentence, and that the silent case stays silent.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let nextOpen = null;

const { document: doc, settle } = await boot({
  routes: {
    '/api/open': () => nextOpen,
    '/api/scan': { hidden: [] },
  },
});

const toastText = () => {
  const t = doc.getElementById('toast');
  return t ? t.textContent : '';
};

function pressUndo() {
  // Cleared, never removed: `toast()` caches the element in a module-level `toastEl`, so a
  // removed node stays the one it writes to and every later assertion reads an empty document.
  const t = doc.getElementById('toast');
  if (t) t.textContent = '';
  const ev = new doc.defaultView.KeyboardEvent('keydown', {
    key: 'z', ctrlKey: true, bubbles: true, cancelable: true,
  });
  doc.defaultView.dispatchEvent(ev);
}

function pressRedo() {
  // Cleared, never removed: `toast()` caches the element in a module-level `toastEl`, so a
  // removed node stays the one it writes to and every later assertion reads an empty document.
  const t = doc.getElementById('toast');
  if (t) t.textContent = '';
  const ev = new doc.defaultView.KeyboardEvent('keydown', {
    key: 'y', ctrlKey: true, bubbles: true, cancelable: true,
  });
  doc.defaultView.dispatchEvent(ev);
}

async function open(meta) {
  nextOpen = {
    id: meta.id, name: meta.name, path: '/tmp/nib-harness/' + meta.name, canSave: true,
    signature: { state: 'unsigned' },
    canUndo: false, canRedo: false, historyEvicted: !!meta.evicted,
  };
  setNextDocument({ numPages: 2 });
  doc.getElementById('pathInput').value = nextOpen.path;
  doc.getElementById('openGo').click();
  await settle();
}

// Declared FIRST, because opening an evicted document toasts once on its own (the event notice
// that survives) and that toast must not be what a later assertion reads.
test('a document with history left alone says nothing on a fruitless undo', async () => {
  await open({ id: 'ev:1', name: 'quiet.pdf', evicted: false });
  pressUndo();
  await settle();
  assert.equal(toastText(), '',
    'a Ctrl+Z that finds nothing now toasts even when nothing was evicted — which replaces the '
    + 'ambiguity ADR-003 names with a message on every fruitless keypress and resolves neither half');
});

test('a Ctrl+Z that finds nothing says so when the history was evicted', async () => {
  await open({ id: 'ev:2', name: 'evicted.pdf', evicted: true });
  // The stimulus: the open itself has to have landed on a view reporting the eviction, or the
  // press below is aimed at a document that was never in this state.
  assert.match(toastText(), /released to stay within the memory budget/,
    'the server-reported eviction was not announced at all on open, so this harness is not in '
    + 'the state the test is about');
  pressUndo();
  await settle();
  assert.match(toastText(), /nothing left to undo/,
    'Ctrl+Z on a document whose history was evicted is silent, so the user has no way at all to '
    + 'learn why their undo stopped reaching — the ↶ control that used to carry this was deleted '
    + 'at v1.125.0 and every banner corner is spoken for');
  assert.match(toastText(), /while you were working elsewhere/,
    'the sentence does not say WHERE the loss happened. The budget is global, so an eviction is '
    + 'always something that happened to a document the user was not looking at — without that '
    + 'clause it reads as "Nib lost your work" rather than as a memory ceiling');
});

test('Ctrl+Y answers the same question, because eviction takes both stacks', async () => {
  await open({ id: 'ev:3', name: 'both.pdf', evicted: true });
  pressRedo();
  await settle();
  assert.match(toastText(), /nothing left to undo/,
    'redo is silent on an evicted document. ADR-003 bounds the undo+redo PAIR and drops a history '
    + 'WHOLE, so a fruitless Ctrl+Y after an eviction is the same question and owes the same answer');
});
