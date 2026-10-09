// Focus restore when a dialog closes — the one half of /pending 281 that jsdom cannot see.
//
// **This is tier 3 because tier 2 passes on the bug.** Measured in this repo's jsdom:
// `.focus()` SUCCEEDS on an element inside a `hidden` container, and hiding a container
// does not blur its focused descendant. So a jsdom assertion that "focus was restored to
// the trigger" is green against the exact defect it would exist to catch — a trigger that
// is `display: none` by the time the dialog closes. Only a real browser's activeElement
// can tell the difference.
//
// The defect that makes this worth a whole tier: most dialogs are launched from a File
// menu item, and `closeMenu()` collapses the dropdown in the same click that opens the
// dialog. So by the time the dialog closes, the saved opener is inside a `display: none`
// subtree, `.focus()` is a silent no-op, and the browser drops focus to <body> — which is
// the same SC 2.4.3 harm the change exists to remove, just relocated to the close.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

test('closing a dialog opened from a menu leaves focus somewhere real', async () => {
  // The dialog is the autofill profile editor, opened the way a user opens it: Mark Up, the
  // Detect & Fill Fields card, its button. **It was About until About became a page (ADR-108).**
  // The subject is the dialog focus contract and not either dialog, and the editor has what the
  // fixture needs: it opens with no document (`editProfileBtn` acts on none), from a menu, it is
  // named by its own heading, and Escape goes through its Cancel. The property under test is
  // unchanged: focus must not be restored to an opener that is no longer on screen.
  await h.mode('markup');
  await h.card('Detect & Fill Fields');
  assert.equal(await page.evaluate(() => !!document.getElementById('viewerWrap').classList.contains('has-doc')), false,
    'setup: a document is open — this dialog is opened with none');
  await page.click('#editProfileBtn');
  await page.waitForSelector('#profileModal:not([hidden])');

  const opened = await page.evaluate(() => {
    const m = document.getElementById('profileModal');
    return {
      role: m.getAttribute('role'),
      ariaModal: m.getAttribute('aria-modal'),
      labelledby: m.getAttribute('aria-labelledby'),
      labelText: (document.getElementById(m.getAttribute('aria-labelledby')) || {}).textContent,
      focusInside: m.contains(document.activeElement) || document.activeElement === m,
    };
  });
  assert.equal(opened.role, 'dialog', 'the profile dialog does not announce itself as a dialog');
  assert.equal(opened.ariaModal, 'true', 'the profile dialog is not marked aria-modal');
  assert.ok(opened.labelText && opened.labelText.trim(),
    `aria-labelledby="${opened.labelledby}" does not resolve to text, so the dialog announces no name`);
  assert.ok(opened.focusInside,
    'focus stayed outside the profile dialog when it opened, so a keyboard user is still behind the scrim');

  // Escape goes through the dialog's own Cancel, as the Escape handler requires.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#profileModal', { state: 'hidden' });

  const after_ = await page.evaluate(() => {
    const el = document.activeElement;
    return {
      tag: el ? el.tagName : null,
      id: el ? el.id : null,
      // getClientRects, not opacity: a display:none element reports opacity 1 quite
      // happily. This is the same pair test/ui/pageops.test.mjs uses, and for the same
      // reason — the first draft of that one read opacity and passed with the defect in.
      laidOut: el ? el.getClientRects().length > 0 : false,
      isBody: el === document.body,
    };
  });
  assert.ok(!after_.isBody,
    'focus fell to <body> when the dialog closed — the user is teleported to the top of the tab order, which is the defect this change exists to remove');
  assert.ok(after_.laidOut,
    `focus was restored to an element that is not rendered (${after_.tag}#${after_.id}) — almost certainly the File-menu item that collapsed when the dialog opened, where .focus() is a silent no-op`);
});
