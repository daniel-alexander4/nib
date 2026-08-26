// A ceremony document with NO signatures reports how many parties are obliged.
//
// ── The defect this is written against ──────────────────────────────────────
// C18's completeness sentence is rendered by `augmentSigDetails`, which lives behind the
// signature-details modal. Two doors closed that modal on a document with no signatures:
//
//   * `updateBadge` set `sigDetailsBtn.hidden = !signers.length`, and
//   * `openSigDetails` began `if (!signers.length) return;`
//
// Both are the right rule for a modal that LISTS signatures. Neither is right for a
// **convened but unsigned** document, which is C18's extreme case — two obliged signers,
// none of whom has signed. The server published `obliged: 2, signed: 0` and no user could
// open it. Found at tier 6 (P07.S05a), where the route was fetched directly and answered
// correctly while the surface was unreachable.
//
// ── Why the control case is half the test ───────────────────────────────────
// Un-hiding the button for every document would pass the first assertion and destroy the
// rule the gate exists for: an ordinary unsigned PDF has nothing to say in a signature
// panel, and offering one is a control that opens onto an empty box. So the second case
// drives the SAME app with `inCeremony` absent and requires the button to stay hidden.
// Without it, `hidden = false` unconditionally is a passing implementation.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

// An unsigned document. `state: 'unsigned'` is what internal/sign emits (verify.go), and
// `signers: []` is the whole point — this is the document nobody has signed yet.
const unsigned = (inCeremony) => ({
  id: 'test-epoch:1', name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf',
  canSave: false, canUndo: false, canRedo: false,
  ...(inCeremony ? { inCeremony: true } : {}),
  signature: { state: 'unsigned', signers: [] },
});

// The attestations route answers as the real one does for a convened document: no
// attestations at all (nobody has signed), and the roster's obliged count beside them.
let openResp = unsigned(true);
let atts = { attestations: [], obliged: 2, signed: 0 };

const { document: doc, settle } = await boot({
  routes: {
    '/api/attestations': () => atts,
    '/api/open': () => openResp,
  },
});

async function open() {
  setNextDocument({ numPages: 1 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
  doc.getElementById('openGo').click();
  await settle();
}

test('a convened but unsigned document offers the panel and says none have signed', async () => {
  await open();

  // STIMULUS: the document really opened as an UNSIGNED one. Without this the assertions
  // below could be read off a boot that never got a document, where the button is also
  // hidden-or-not for reasons having nothing to do with the ceremony.
  assert.equal(doc.getElementById('sigBadge').textContent, 'Unsigned',
    'the fixture did not open as an unsigned document, so this case is not the one under test');

  assert.equal(doc.getElementById('sigDetailsBtn').hidden, false,
    'the details button is hidden on a ceremony document, so C18 has no surface at all');

  const body = doc.getElementById('sigDetailsBody');
  body.innerHTML = '';
  doc.getElementById('sigDetailsBtn').click();
  for (let i = 0; i < 80 && !/obliged/.test(body.textContent); i++) {
    await new Promise((r) => setTimeout(r, 5));
  }
  const txt = body.textContent.replace(/\s+/g, ' ');
  assert.match(txt, /0 of 2 obliged signer\(s\) have signed/,
    `the panel did not report the ceremony's completeness: ${txt.slice(0, 300)}`);
  assert.match(txt, /Incomplete/,
    `a ceremony nobody has signed was not reported as incomplete: ${txt.slice(0, 300)}`);
});

test('an ordinary unsigned document is offered no signature panel', async () => {
  openResp = unsigned(false);
  atts = { attestations: [] };
  await open();

  assert.equal(doc.getElementById('sigBadge').textContent, 'Unsigned',
    'the control fixture did not open as an unsigned document');
  assert.equal(doc.getElementById('sigDetailsBtn').hidden, true,
    'an ordinary unsigned PDF offers a signature-details button, which opens onto nothing — ' +
    'the ceremony case was fixed by removing the gate rather than by narrowing it');
});
