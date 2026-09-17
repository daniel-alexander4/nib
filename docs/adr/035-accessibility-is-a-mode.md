# ADR-035 — accessibility is a mode, because it was a concern spread across three

**Status:** accepted
**Date:** 2026-09-16
**Context:** Dan: *"Accessibility needs to be its own main menu item with everything that falls
underneath that in the sidebar."* `web/index.html`'s `.modetab` and `[data-modejump]` lists;
`SIDEBAR_FOR` in `web/app.js`; `test/jsdom/modes.test.mjs`.
**Extends:** ADR-016 (the modes are cut by what you do to the document) and ADR-025 (Settings is a
mode). Neither is reversed — see *What this does not supersede*.
**Applies:** every accessibility control added from now on, and every future change to the mode set.

## Decision

**Accessibility is the seventh mode**, sitting between Page Functions and Secure, and it holds every
control that is about what a screen reader will make of the document:

| | was | now |
| --- | --- | --- |
| Tag structure… (`#tagsBtn`) | Page Functions, *Tag Structure* card | Accessibility, same card |
| Check accessibility (PDF/UA)… (`#uaBtn`) | Secure, *Protect & Inspect* card | Accessibility, *Check Accessibility* card |
| Review Structure Tree (`#tagtree`) | `SIDEBAR_FOR.edit[1]` | `SIDEBAR_FOR.accessibility[1]` |

**It is cut differently from the other six, and that is the decision.** ADR-016 cuts a mode by *what
you do to the document* — put things on the page, change the pages, remove and protect. Accessibility
is not a verb of that kind; it is a **property of the document** that marking up, changing pages and
inspecting each affect. Filed by ADR-016's own rule, its three controls landed in three different
modes, which is exactly what happened: a user fixing a document's accessibility had to know that
proposing tags was under Page Functions, that checking the result was under Secure, and that
correcting an element was a panel hanging off the first of those.

**ADR-025 is the precedent, not an analogy.** Settings is also not a thing you do to the document,
and it earned a mode on the same instruction and for the same reason — its items were scattered
behind a gear. This is that shape applied a second time, which is why it needs no new mechanism: a
seventh entry in four lists that already exist.

**The landing surface is its commands, and the structure tree is second.** `syncSidebarForMode`
activates `panels[0]`, so the order in `SIDEBAR_FOR` decides where the mode lands; the tree was
Page Functions' second surface and stays second here. Listing it first would make the tree the
landing screen for a document that may have no tree at all.

## What this does not supersede

**ADR-016's rule stands for the six modes it cut.** A control that *acts on the pages* still belongs
in Page Functions and a control that *protects* still belongs in Secure; this ADR does not license
a mode per concern. The test it adds is narrow: a concern earns a mode when its controls are
otherwise **forced apart by ADR-016's own cut** and no single one of the six is their home.

**`PLAN-accessibility.md` D10's placement is superseded, its reasoning is not.** D10 put tagging in
`edit` *per ADR-016*, and against five modes that was the right answer — Page Functions is where
you change the document's pages, and a structure tree is about the pages. What D10 could not choose
was an option that did not exist. The plan's phases are closed and its coordinates are history; this
changes where the controls live, not what they do.

## What it costs, stated

**A seventh tab is a wider strip**, and ADR-016 killed a sixth tab on exactly that number: 408px at
five, 476px at six, against a 719px fold threshold below which `.modetabs` goes `display: none` and
the dropdown takes over. Six shipped anyway with ADR-025. The width with a seventh is **measured at
tier 3, in real Chromium, at 719 / 1024 / 1366** — jsdom has no layout engine, so no tier-2
assertion about the strip can fail. `/pending 532` carries it until the measurement is recorded.

**Page Functions loses a card and Secure loses a button.** Neither mode is emptied: Page Functions
keeps Split, Compose, Page setup, Rotate and Combine & Compare; Secure keeps Redact, Sign &
Timestamp and the rest of Protect & Inspect. `test/ui/cardhue.test.mjs` counts cards in Settings, not
in Page Functions, so its six-step ladder is unaffected.

**Four lists change together or two of them fail silently** — the tab strip, the `[data-modejump]`
dropdown, the `.tbtab` pane and `SIDEBAR_FOR`. `test/jsdom/modes.test.mjs` asserts all four agree and
needs no layout engine, which is why it is tier 2.
