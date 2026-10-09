# ADR-107 — the pages share one tab, and opening another replaces what it shows

**Status:** accepted
**Date:** 2026-10-08
**Context:** Dan, after ADR-104 and ADR-105 gave every Settings entry and two Signing entries a page
with a tab each: *"instead of each button getting it's own tab, let's use a single tab and when I
click on a new button it replaces the content."* Ten entries open a page, so a walk through
Settings left up to eight page tabs beside the documents', each for something looked at once.
`openAppPage`, `showAppPage`, `closeAppPage` and the page-tab loop of `syncTabs` in `web/app.js`.
**Supersedes:** two phrases and nothing else —
- ADR-104's title and §2 *"has a tab of its own"* and §2's *"Several pages can be open"*.
- ADR-105 §1's *"opens a page with its own tab"* (it was ADR-104's mechanism by reference).
Every other decision in both stands: a page is never a `view`; `syncMainArea` and `docShowing()`;
the ways out; focus on the heading; leaving the menu leaves the page; a hidden menu's page closes;
Close all closes documents; Escape does nothing to a page.
**Applies:** every app page, of any menu, and any added later.
**Superseded in part by [ADR-109](109-toggle-features-is-one-page-of-three-sections.md)** (the Settings order: seven entries, with Updates, Advanced features and Main menu as the sections of Toggle Features).

## Decision

**1. At most one page is open, and there is one page tab.** It is the strip's last tab, after the
documents'. One tab in TOTAL — Settings and Signing pages take turns in it, because the instruction
was "a single tab" and a tab per menu would be the same accumulation at a smaller number.

**2. Opening a page that is not the open one replaces it.** `openAppPage` puts the page in the
registry *in place of* what it held and brings it forward; the tab is rebuilt where it was, and its
word, its name, its accessible name (*"Updates, Settings page"*), its `aria-controls` and its ×'s
name follow the page showing. Opening the page that is already open brings it forward. Focus goes
to the new page's heading, as on any open.

**3. The replaced page is LEFT, by the door every page is left by, exactly once.** In front, it is
`activeAppPage` when `showAppPage` runs, and `showAppPage` leaves the page it takes the screen from
(`appPageLeave`: a typed Download folder is saved). Behind a document it was left when the document
came forward, and is not left again. `openAppPage` therefore adds no leave call of its own — **do
not add one**: it would be a second leave for the first case.

**4. `openAppPages` stays, as a registry that never holds more than one.** Every reader
(`syncTabs`, `showAppPage`, `closeAppPage`, `applyModeVisibility`) and the writer guard already
speak of it, and the change is one line. The source guard now also refuses a `push`/`unshift` on
it: nothing ADDS a page beside the one open.

## What this costs, and what it does not cover

- **You cannot keep two pages a click apart.** Going back to the earlier page is its entry again.
  A page holds no unsaved state — settings save as they change, and the one field that does not is
  saved by the leave — so nothing is lost by the replacement.
- **A feature's "switched off" row (ADR-105 §7) is rarely seen now.** Switching ceremonies off from
  the Advanced features page replaces the ceremony page, whose entry is then hidden. The row still
  answers the case where the feature goes off while its page is the one open, and the checklist's
  own off-steps are unaffected.
- **Likewise ADR-105 §8** (a hidden menu's page closes): the Main menu page takes the tab from a
  Signing page before its box can hide Signing. The rule is kept for a menu hidden while its page
  is open by any other route, and is tested by running the box's handler with the page open.
- **No Go change, no new request, no stored state.** A reload still starts with no page.
- **Covered by** `test/jsdom/apppages.test.mjs` (one tab through eight entries; the replaced page
  left once, in front and behind; Settings ↔ Signing; the registry guard) and
  `test/ui/apppages.test.mjs` (beside real documents: the tab's place in the strip and its left
  edge unchanged, the typed folder reaching the real server, focus, Close all leaving the page).
