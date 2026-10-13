// The lost-accessibility-structure notice (ADR-031, law 2's `dropped-with-notice`).
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the commit door NOTICES — that the flag is set exactly when a claim disappears,
// that it is sticky across later edits, and that it reaches the wire. They cannot see whether
// anything is drawn, whether it survives the next edit on screen, or whether dismissing it works.
//
// ── Why a persistent banner and not a toast, asserted rather than commented ──
// `index.html`'s own note about the other persistent notice says it: *"`toast` cannot carry them: it
// clears itself after 2500 ms."* A user who lost their document's accessibility structure has to
// still be able to see that at the moment they save, which may be ten minutes later. So the test
// that matters most here is the one where a SECOND edit arrives and the banner is still up.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { viewersMade } from './stub-viewer.mjs';

const DOC = '/tmp/nib-harness/tagged.pdf';
let dropped = false;
let label = '';
let tagged = false;

const h = await boot({
  routes: {
    '/api/open': () => ({
      id: 'test-epoch:1', name: 'tagged.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
      taggingDropped: dropped, conformanceLabel: label, tagged,
    }),
    // The route a page rotation actually takes — driven through the real button below, because a
    // bare fetch would not re-render anything and would prove nothing about the banner.
    '/api/pages': () => ({
      id: 'test-epoch:1', name: 'tagged.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: true, canRedo: false,
      taggingDropped: dropped, conformanceLabel: label, tagged,
    }),
  },
});
const { document: doc, settle } = h;

const notice = () => doc.getElementById('tagNotice');
const shown = () => { const n = notice(); return !!n && !n.hidden; };

// anEdit drives a real page rotation through the real control, so what is asserted is what the app
// does on an ordinary edit rather than what a hand-made fetch does.
async function anEdit() {
  doc.getElementById('rotateLeftBtn').click();
  await settle();
}

async function openDocument() {
  setNextDocument({ numPages: 2, outline: null });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}

test('a document that never lost tagging shows nothing', async () => {
  dropped = false;
  await openDocument();
  assert.equal(shown(), false,
    'the lost-accessibility banner is up on a document that never had tagging — it would fire on '
    + 'nearly every edit in the product and teach the user to ignore the one time it matters');
});

test('losing the structure raises a persistent banner, not a toast', async () => {
  dropped = true;
  await openDocument();
  assert.equal(shown(), true,
    'nothing is shown after an edit removed the document accessibility structure. The file is '
    + 'honest — it no longer claims to be tagged — and the person who could re-make it is told '
    + 'nothing, which is the whole of law 2\'s `with-notice` half');
  const n = notice();
  assert.equal(n.getAttribute('role'), 'status',
    'the banner carries no role, so a screen-reader user — the person this notice is most for — '
    + 'is not told at all');
  assert.equal(n.getAttribute('aria-live'), 'polite',
    'the banner is not a live region, or is assertive; assertive is spent on the signature-loss '
    + 'notice and interrupting for this would devalue that one');
  const text = doc.getElementById('tagNoticeText').textContent;
  assert.match(text, /screen reader/i,
    `the banner says ${JSON.stringify(text)} — "tagging" alone means nothing to the small-practice `
    + 'user this product is for, so it has to name what was lost and what it was for');
  assert.match(text, /re-make|source/i, 'the banner does not say the one thing the user can do about it');
});

test('it survives the next edit, which is the whole reason it is not a toast', async () => {
  // A toast would be gone 2.5 seconds later; the tagging is still gone ten minutes later, and the
  // moment this has to be on screen is the SAVE. The server keeps the flag sticky per document —
  // this asserts the client does not drop it on the next response.
  assert.equal(shown(), true, 'setup: the banner is not up, so surviving an edit proves nothing');
  await anEdit();
  assert.equal(shown(), true,
    'an ordinary later edit took the banner down. The tagging is still gone, and the user stops '
    + 'being told before the moment they save');
});

test('dismissing it is remembered, so the user is not nagged', async () => {
  assert.equal(shown(), true, 'setup: nothing to dismiss');
  doc.getElementById('tagNoticeDismiss').click();
  await settle();
  assert.equal(shown(), false, 'Dismiss did not take the banner down');
  await anEdit();
  assert.equal(shown(), false,
    'the banner came back after a dismissal — a user who read it and decided they do not care is '
    + 'nagged for the rest of the session');
});

// ── What signing does to a conformance label (ADR-147, /pending 520) ──────────
//
// No signing path checks a PDF/UA or PDF/A label: an unsigned document loses it before the signature, a signed
// one keeps it. The server says which and EVERY signing surface says it before the user signs — Finalize,
// Co-sign and Simple Sign in the markup, from one painter. The words are `nib sign`'s, held equal here.
const labelNotes = () => [...doc.querySelectorAll('.signlabelnote')];

test('every signing surface says what signing does to a conformance label, in nib sign\'s words', async () => {
  const { readFileSync } = await import('node:fs');
  const go = readFileSync(new URL('../../internal/pdfops/uaid.go', import.meta.url), 'utf8');
  const sentence = (name) => {
    const m = new RegExp(name + String.raw`\s*=\s*((?:"[^"]*"\s*\+?\s*)+)`).exec(go);
    assert.ok(m, `internal/pdfops/uaid.go no longer declares ${name}`);
    return [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]).join('');
  };
  const homes = ['finalizeModal', 'cosignModal', 'signingStepsPage'];
  for (const id of homes) {
    assert.ok(doc.getElementById(id)?.querySelector('.signlabelnote'),
      `#${id} has no place for the note: a labelled document is signed there with nothing said`);
  }

  label = '';
  await openDocument();
  assert.ok(labelNotes().every((n) => n.hidden && !n.textContent),
    'a document with no label is given a note about one');

  label = 'removed';
  await openDocument();
  for (const n of labelNotes()) {
    assert.equal(n.hidden, false, 'an unsigned labelled document is signed with nothing said about its label');
    assert.equal(n.textContent, sentence('LabelRemovedBySigning'),
      'the window and `nib sign` say different things about the same document');
  }

  label = 'kept';
  await openDocument();
  for (const n of labelNotes()) {
    assert.equal(n.textContent, sentence('LabelKeptBySigning'),
      'a signed labelled document does not say its label is kept unchecked');
  }

  label = '';
  await openDocument();
  assert.ok(labelNotes().every((n) => n.hidden), 'the note outlived the document it was about');
});

// ── A crop on a tagged document (/pending 577, ADR-147) ──────────────────────
//
// A crop hides; it does not remove, and the tags still describe the whole page (ADR-047). The window says so
// once, where the crop happened — and never in #tagNotice, which says the tags were LOST.
test('cropping a tagged document says the hidden part is still read aloud, and names Redact', async () => {
  // A real crop through the real controls: arm the tool, drag a box on the page, confirm. jsdom lays nothing
  // out, so the one page is given a rectangle and the stub viewer is told that page is it.
  const win = doc.defaultView;
  const ev = (type, x, y) => new win.MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0 });
  const crop = async () => {
    const box = doc.querySelector('.viewerContainer:not([hidden])');
    const page = doc.createElement('div');
    page.className = 'page';
    page.getBoundingClientRect = () => ({ left: 0, top: 0, right: 500, bottom: 700, width: 500, height: 700 });
    box.appendChild(page);
    for (const v of viewersMade) v.getPageView = (i) => (i === 0 ? { div: page } : null);
    doc.getElementById('cropBtn').click();
    page.dispatchEvent(ev('pointerdown', 50, 50));
    doc.getElementById('viewerWrap').dispatchEvent(ev('pointerup', 300, 400));
    assert.equal(doc.getElementById('cropModal').hidden, false, 'setup: the drag did not reach the confirm step');
    const before = doc.getElementById('cropTaggedNote');
    const asked = before && !before.hidden ? before.textContent : '';
    doc.getElementById('cropGo').click();
    await settle();
    page.remove();
    return { said: doc.getElementById('toast')?.textContent || '', asked };
  };
  dropped = false; label = ''; tagged = false;
  await openDocument();
  const plain = await crop();
  assert.match(plain.said, /cropped to the box/, `setup: the crop did not report: ${plain.said}`);
  assert.doesNotMatch(plain.said, /read aloud/, 'an untagged document is told about a screen reader it has no tags for');
  assert.equal(plain.asked, '', 'the confirm step tells an untagged document about a screen reader');

  tagged = true;
  await openDocument();
  const { said, asked } = await crop();
  assert.match(asked, /still in the file and will still be read aloud/,
    `the confirm step does not say it BEFORE the crop: ${asked}`);
  assert.match(said, /cropped to the box/, `the crop's own result is gone: ${said}`);
  assert.match(said, /still in the file and will still be read aloud/,
    `a tagged page was cropped and nothing says the hidden part is still read: ${said}`);
  assert.match(said, /use Redact/, `the sentence does not name the tool that removes: ${said}`);
  assert.equal(shown(), false, 'the crop raised #tagNotice, which says the tags were lost — the opposite');
  tagged = false;
});
