# ADR-104 — a menu entry that holds settings or a workflow opens a page with its own tab

**Status:** accepted
**Date:** 2026-10-08
**Context:** Dan: *"in the Settings menu, I would like each button to popup a nice page instead of
expanding in the menu."* Asked whether that meant popups or tabbed pages: *"I prefer new tabs/pages
over popups"*, for Settings and for Signing. Settings was nine accordion cards like every other
mode's, and a card is the sidebar's 200px column: *Updates* held a text field, a line saying which
folder is in use and a refusal; *Advanced features* and *Main menu* each held a paragraph and a list
of boxes. `web/index.html` (the Settings pane; the `.apppage` sections in `#viewerCol`); the *App
pages* block, `syncTabs`, `setCardOpen`, `syncSidebarForMode` and `flushDownloadDir` in `web/app.js`;
`.apppage` / `.pagebody` / `.setrow` and `.pagetab` in `web/style.css`.
**Superseded in part by [ADR-107](107-the-pages-share-one-tab.md)** (the title's and §2's *"a tab of its own"* and §2's *"Several pages can be open"*: the pages share ONE tab, and opening another page replaces what it shows. Every other decision here stands.)
**Superseded in part by [ADR-108](108-about-is-a-page-like-the-other-eight.md)** (§9's *"About opens the About dialog"*, and *"one id (`aboutBtn`) is left in the pane"* below: About is a page like the other eight. §9's sentence about *Identity & Keys* stands.)
**Supersedes:** three phrases and nothing else —
- ADR-025's *"with its items as sidebar cards"* and *"each id is a named exemption"*: the items are
  entries that open pages, and one id (`aboutBtn`) is left in the pane. Settings being a mode, the
  gear being gone, and everything it says about Colours stand.
- ADR-036's *"changed from a **Main menu** card in Settings"*: it is a page. Its rules — a separate
  switch from Advanced features, Settings never hideable — stand.
- ADR-020's *"a card toggles"* does not reach an entry, which is not a card: it has no open state.
  Every other sidebar header still toggles.
**Extends:** ADR-037 and ADR-103 (the strip and its row): every child of the strip is still a tab,
and a tab is now a document's or a page's.
**Applies:** anything added to Settings; the Signing pages, which are to be built on this; any later
surface that opens in the main area.

## Decision

**1. An entry opens a page; it does not expand in the menu and it is not a popup.** In the markup an
entry is a `.tbgroup` marked `data-entry` that holds **one button**. The nine Settings names are the
nine entries, in the order they had. **Do not put a setting in the pane**: it goes on a page.
Entering Settings opens nothing — a mode lands on its first *card*, and an entry is not one.

**2. A page stands in the main area and has a tab of its own.** It is a `<section class="sheet
apppage">` in `#viewerCol`, shown in place of the viewer as the two sheets are. Its tab is in
`#tabstrip` after the documents' tabs, built by the one function that builds the strip
(`syncTabs`), in the same shape: a `role="tab"` div with a real × inside. Several pages can be open;
an entry clicked again brings its page forward and never opens a second copy.

**3. A page is never a `view`.** `openAppPages` (what is open) and `activeAppPage` (what is in
front) sit *beside* `views`. Operation pinning (ADR-001), `X-Nib-Doc` (ADR-004), the budgets
(ADR-003/005) and the three-mutators law all key on a view being a document, and none of them sees
a page. In `syncTabs`, `several` and `anyDoc` still read `views` alone: **Close all** appears at two
*documents*, closes documents, and leaves pages in their tabs. Only whether the row shows asks about
pages too, because a page can be open with no document.

**4. One door for what the main area shows.** `syncMainArea` is the only writer of
`#viewerWrap.hidden` and of a page's `hidden`. It derives: a page in front, else a sheet that is
up, else the viewer. The two sheets were folded in rather than left as two more writers — each
still writes its own `hidden` with the state that goes with it, and calls the door. A page coming
forward closes the returned-document sheet and parks a ceremony setup, through their own doors; a
sheet opening leaves the page.

**5. A page in front is, for every control, no document.** `docShowing()` is the one answer to "is a
document on screen to act on". `setDocControls`, `applySignLock` and the keyboard shortcuts that act
on the document read it, so Save, the `DOC_REQUIRED` set and the editing tools are inert and a tool
cannot be armed against a document nobody can see. The fixed bar's document groups — Find, Page,
History, View, Zoom, Save, Print — go `inert` (`reflectDocShowing`): zoom, the page box and
read-aloud are not in `DOC_REQUIRED`, and each would change a document behind a page unseen. The
document is quiesced as on a tab switch (a
drag, reading aloud, a dialog about that document). What was armed on it stays armed on *it* — armed
state is the view's — and is there when it comes back, with its scroll and zoom.

**6. The ways out are the tabs.** A document's tab shows that document (`showDocument`: the page
yields, *then* the view is activated — `activateView` returns at once for the view already active,
which is exactly the tab clicked to get back). The page's × closes it; closing the page in front
shows the document again. Opening a document shows it. **Leaving the page's menu leaves the page**
(`data-menu`), as it parks a ceremony setup: the new mode's tools act on the document. **Escape
does nothing to a page** — it is not a dialog, and the Escape contract is the dialogs'.

**7. Opening a page puts focus on its heading.** The page is a `role="region"` named by that
heading, never `aria-modal`. Its tab is in the strip's roving order and announces what it is:
*"Updates, Settings page"*; on screen the difference from a document's tab is a **word** (the
page's `data-group`), not colour.

**8. The controls did not change, and nothing moves.** Every control has the id and the handler it
had as a card, in a page container that holds it permanently. `applyStatus` and the savers write to
them whether or not the page is open. A setting is saved as it is changed, so a page has no Save and
no Cancel. **The one field that saves on `change` is answered for when its page is left**
(`appPageLeave` → `flushDownloadDir`): a folder typed and not yet saved is saved; one the server
refuses is said in a toast and the box goes back to the stored folder.

**9. About opens the About dialog.** That dialog swaps three views inside itself and is the fixture
the dialog-focus tier and two red proofs are recorded against; making it a page is a separate change.
*Identity & Keys* is a page that says what its two dialogs are for and opens them.

## How a menu registers a page

Markup only.

1. In `#viewerCol`, beside the others: `<section id="X" class="sheet apppage" data-group="Signing"
   data-menu="collaborate" data-title="…" role="region" aria-labelledby="XTitle" hidden>` holding
   `<div class="pagebody">` with an `<h2 id="XTitle" tabindex="-1">`.
2. Anywhere: `<button data-apppage="X">`. For an entry in the menu, the button alone in a
   `.tbgroup[data-entry]`.
3. Only if the page must do something as it is left: `appPageLeave.X = () => …`.

`openAppPage(id)` is the door for code that wants a page shown.

## Why not dialogs, and why not cards re-parented

Dialogs were built first and withdrawn at Dan's word: a setting is something you go to and come back
from with the document still there, and a tab says where it went. Moving the card bodies into a
shared container at open and back at close keeps one copy of the markup too, but adds a state —
*where is this group now* — to a pane that already moves between the sidebar and the toolbar.

## What this costs, and what it does not cover

- **Open pages are the window's, not the vault's.** A reload starts with none.
- **One more click to a setting than the first card had** (it was open on arrival).
- **The Main menu boxes work with no document open.** They share `data-mode` with the annotation
  tools' buttons, and five sweeps written as a bare `[data-mode]` disabled them whenever no document
  was open and wired a click on one to `setTool`. The sweeps read `button[data-mode]` now.
- **`Restore vault…` is a button now**, in front of the hidden file input. It was a `<label>` around
  the input, which the keyboard could not reach.
- **The sheets keep their own state and focus rules.** They sit behind the door; they were not
  rewritten as pages, and neither has a tab.
- **No Go change and no new request.** `foldThresholds` is untouched (ADR-087); the Settings groups
  keep their ranks, which decide nothing by width.
- **Covered by** `test/ui/apppages.test.mjs` (a real document beside a page, focus, a narrow window,
  the typed folder through the real route) and `test/jsdom/apppages.test.mjs` (the entries, one tab
  a page, and the writers of the registry and of `#viewerWrap.hidden`).
