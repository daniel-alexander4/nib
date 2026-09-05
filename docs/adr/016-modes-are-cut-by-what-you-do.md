# ADR-016 — the modes are cut by what you do to the document

**Status:** accepted
**Date:** 2026-09-05
**Context:** `/pending 306`'s mode re-cut, deferred once and then reopened; `web/index.html`'s five
`.tbtab` panes; `SIDEBAR_FOR` in `web/app.js`; `test/jsdom/modes.test.mjs`.
**Applies:** every control added to the toolbar from now on, and every future change to the mode
set. It decides which pane a new control belongs in, which is a question its author will otherwise
answer by proximity.

## Decision

**Five modes, each one a kind of thing you do to the document:**

| Tab | id | What belongs here |
| --- | --- | --- |
| **File** | `file` | getting the document in and out — open, save, export, print, page navigation, zoom, find |
| **Mark Up** | `markup` | putting things ON the page — form fields, annotations, drawn marks, the signature/stamp Library |
| **Document** | `edit` | changing the document itself — its pages, and the content already on them |
| **Secure** | `secure` | removing, protecting and proving — redaction, passwords, and Certify |
| **Ceremony** | `collaborate` | the multi-party signing flow |

**Undo/Redo belong to none of them** and sit in `#toolbar`'s fixed area, beside the sidebar toggle.

Two ids deliberately do not match their labels: `edit` for **Document** and `collaborate` for
**Ceremony**. Renaming them would touch about eight test files each for no behavioural gain.
`sign` **was** renamed to `markup`, because that word now means the Ceremony and a
`data-tab="sign"` on the annotation tab is the drift that costs the next reader an hour.

## Why the old cut was wrong

"Edit" held four unrelated jobs — things you add to the page, the page structure, the existing
content, and forms — and no other product in the category files them together. Acrobat puts
Text/Highlight/Draw/Note under **Comment** and reserves "Edit PDF" for altering existing content;
Preview calls the same toolbar **Markup**. Adding a layer on top of a page is the opposite operation
from changing what is under it.

The grouping shipped in ADR-015 did not fix this. Group labels are deliberately kept out of the bar
— captions would add height at exactly the widths the fold exists to shrink — so they surface only
inside `⋯ More`, which never opens above 850px. At the widths this product is actually used at, Edit
remained a flat wall of 32 controls.

**Form filling is in Mark Up, not Document**, and that is the decision most likely to be re-litigated:
Detect fields and Autofill are the primary user's commonest task, and under "Document" — a word
meaning *change the pages* — nobody would look for them.

## Why five and not six

The first plan split Edit three ways and left "Edit" holding five controls. Measured tab-strip widths
in real Chromium, cloning a tab node so padding and font match: **408px before**, **476px at six
tabs**, **392px at five after trimming "Signing Ceremony" to "Ceremony"** (149px → 96px, the single
highest-leverage edit in the change). Six tabs would have moved the menubar wrap point ~70px and
pushed the mode-tab dropdown up into the 680–1366 band this product is used in. Five tabs made the
strip *narrower* than before the re-cut.

A **Pages** tab was also killed on a name collision: File already has a group labelled **"Page"**
(Previous · page box · Next). One word, one letter apart, two meanings.

## Two silent failures this change found, and the guard that now covers them

Adding or renaming a mode means changing four lists together, and **two of them fail without any
symptom**:

- **`SIDEBAR_FOR`.** `syncSidebarForMode` reads `SIDEBAR_FOR[tab] || []`, hides every sidebar nav
  button because none is in the empty list, then skips its own re-activate branch because
  `panels.length` is 0 — so the *previous* mode's panel stays on screen with no tab above it. No
  throw, no warning.
- **`[data-modejump]`.** Below the fold threshold `.modetabs` is `display: none`, so a mode absent
  from the dropdown is simply unreachable at small widths.

`test/jsdom/modes.test.mjs` asserts all four lists agree. It needs no layout engine, which is why it
is tier 2.

## What the empty state taught

The fold threshold is **719px**, and the number is set by the state with **no document open**: the
signature badge reads "no document" rather than "Unsigned" — 23px wider — and the menubar wraps at
700 where it fits at 720. Every test in the responsive suite opened a document first, so the guard
spoke only for the populated state: a population bias in the instrument, not in the code. The suite
now sweeps the empty state too, because it is the first screen a user ever sees.

The root cause is that badge, and shrinking or hiding it would let the threshold drop again. That is
a product decision about what the chrome says when nothing is open, and it is deliberately not taken
here.

## What this does not decide

**The Flags panel stays in Ceremony.** It is markup and belongs beside Library, and moving it would
make `fillMarker`'s cross-mode click same-mode by construction — but removing Flags from
`collaborate` makes the ceremony panel that mode's landing screen, which is exactly the arrangement
a documented product decision reversed after eight tier-3 tests went red. Reopening a settled
product decision inside a taxonomy change is how a re-cut becomes a regression.
