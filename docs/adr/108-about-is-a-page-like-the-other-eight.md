# ADR-108 — About is a page like the other eight, and its two documents open in place

**Status:** accepted
**Date:** 2026-10-08
**Context:** Dan: *"About should use the same tab as the rest of the settings pages"*. ADR-104 made
eight of the nine Settings entries pages and left About a dialog (§9), because that dialog swapped
three views inside itself — the account, the licence, the notices — and was the fixture
`test/ui/dialogfocus.test.mjs` and a red proof were recorded against. ADR-107 then put every page in
one tab, which left About the one Settings entry that opened something else. The About page
(`#settingsAboutPage`) in `web/index.html`; `showAboutDoc` / `toggleAboutDoc` in `web/app.js`;
`#aboutMain`, `#aboutDocs` and `.aboutdoc` in `web/style.css`.
**Supersedes:** one decision and nothing else — ADR-104 §9's *"About opens the About dialog"* (and the
phrase in its *Supersedes* note, *"one id (`aboutBtn`) is left in the pane"*: no entry has an id).
§9's second sentence, about *Identity & Keys*, stands, and so does everything else in ADR-104 as
ADR-107 left it.
**Extends:** ADR-104 (how a page is registered), ADR-107 (the one tab).
**Applies:** the About page; any later page that shows a long fetched text.

## Decision

**1. About is a page, registered the way ADR-104 says and by nothing else.** A `.sheet.apppage`
section (`data-group="Settings"`, `data-menu="settings"`, `data-title="About"`), its entry a
`.pageentry` button with `data-apppage`. It opens in the one page tab, in place of whatever page
was there; the tab reads *Settings · About*; focus goes to its heading; Escape does nothing to it.
`#aboutModal` and its dialog-only controls (`aboutBtn`, `aboutTitle`, `aboutBackBtn`, `aboutClose`,
`aboutDocText`) are gone. No JavaScript was written to open it.

**2. The words did not change.** Everything `#aboutMain` held is on the page, in the same order
bar one: the opening sentence leads, as a page's lead does, and the version line follows it. Each
`h4` section is one of the page's blocks (`.setrow` under an `h3.rowtitle`). The muted colours the
dialog used are `--text`, as on every page (ADR-104's stylesheet note: the muted tokens are under
AA for small text in the light theme).

**3. `#aboutMain` is still the block the trust-claim guard reads.**
`internal/p2p/readme_test.go`'s `TestAboutCopyContainsTrustClaims` isolates it from
`<div id="aboutMain">` to the `<div id="aboutDocs"` that follows, and reads its text. **Keep both
ids, in that order, with nothing but the account between them** — and do not write either opening
tag out in a comment above them: the pattern takes the first it finds.

**4. The licence and the notices open in place, each under its own button, and are fetched when
asked for.** A button shows its document in a scrolling box below it and puts it away again, and
says which (`aria-expanded`, `aria-controls`). Focus stays on the button; the box is a tab stop so
the keyboard can scroll it. A document put away is emptied, and asked for again is fetched again.
Nothing is fetched before a button is pressed — the notices are some 3,500 lines.

The dialog swapped its whole body for the document and offered *Back*. That was refused for the
page: it is a second view inside a page, with a heading that changes under a tab that does not, and
a place for focus to be lost on the way back. In place there is no second view.

**5. A fetched document goes in as text.** `showAboutDoc` writes `textContent`, as it always did.
It is a file Nib ships, and it is still not parsed.

## What this costs, and what it does not cover

- **The dialog-focus tier's fixture is the autofill profile editor now** (`#profileModal`, opened by
  `#editProfileBtn` in Mark Up → Detect & Fill Fields): it opens with no document, from a menu, is
  named by its own heading, and Escape goes through its Cancel. Every assertion is kept. The two
  jsdom focus tests that unhid `#aboutModal` directly unhide `#profileModal`.
- **The red proof `about-dialog-deleted-claims-survive` keeps its name** and was re-recorded against
  the page; its defect — `#aboutMain` deleted and the six claims left in a comment — is the same.
- **Both documents can be open at once**, one above the other, each in its own box.
- **A document open when the page is left is still open when it comes back.** It is not re-fetched.
- **Comments in `internal/server` still say "the About dialog"** (the `/legal/` route and the
  version field). They are comments about who reads the route, and were left so that this change
  touches no server code.
- **No Go change outside a test's pattern and messages, no new request, no stored state.**
- **Covered by** `test/jsdom/apppages.test.mjs` (nine entries, nine pages; About in the one tab; no
  dialog; the two documents fetched on request, as text, and put away) and
  `test/ui/apppages.test.mjs` (the version from the real server; the documents by mouse and by
  keyboard; 414 and 375 px with both documents showing).
