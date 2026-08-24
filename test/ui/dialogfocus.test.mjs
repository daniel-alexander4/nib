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
import { launch } from './harness.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

test('closing a dialog opened from a menu leaves focus somewhere real', async () => {
  // Open About the way a user does — through the settings menu, so the opener IS a
  // dropdown item and the dropdown collapses behind it. Playwright refuses a
  // display:none target, which is the same fact that makes restoring to it a no-op.
  await page.click('.menu.settings > .menutop');
  await page.waitForSelector('#aboutBtn', { state: 'visible' });
  await page.click('#aboutBtn');
  await page.waitForSelector('#aboutModal:not([hidden])');

  const opened = await page.evaluate(() => {
    const m = document.getElementById('aboutModal');
    return {
      role: m.getAttribute('role'),
      ariaModal: m.getAttribute('aria-modal'),
      labelledby: m.getAttribute('aria-labelledby'),
      labelText: (document.getElementById(m.getAttribute('aria-labelledby')) || {}).textContent,
      focusInside: m.contains(document.activeElement) || document.activeElement === m,
    };
  });
  assert.equal(opened.role, 'dialog', 'the About dialog does not announce itself as a dialog');
  assert.equal(opened.ariaModal, 'true', 'the About dialog is not marked aria-modal');
  assert.ok(opened.labelText && opened.labelText.trim(),
    `aria-labelledby="${opened.labelledby}" does not resolve to text, so the dialog announces no name`);
  assert.ok(opened.focusInside,
    'focus stayed outside the About dialog when it opened, so a keyboard user is still behind the scrim');

  // Escape goes through the dialog's own Close, as the Escape handler requires.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#aboutModal', { state: 'hidden' });

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
