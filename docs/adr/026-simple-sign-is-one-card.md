# ADR-026 — Simple Sign is one card, and role belongs to the ceremony

**Status:** accepted
**Date:** 2026-09-07
**Context:** Dan: *"[the Originate/Receive toggle] should be in it's own collapsible header called
Simple Sign and it should not be separated into Originate/Receive. That will be managed in the
Ceremony pill. Place Signing Flags should be the top pill above Simple Sign and Ceremony"*
**Extends:** ADR-024 (the sidebar's two sections).
**Applies:** the Ceremony mode, and anything that adds a peer-to-peer command.

## Decision

**Collaborate's palette is one card, `Simple Sign`.** The Originate/Receive toggle split seven
buttons in half, so reaching a command meant first answering which half you were in — and the two
halves are the two ends of one act. `Identity & peers…` appeared in BOTH halves and appears once
now; it only looked like two commands because you could never see both at the same time.

**Role belongs to a CEREMONY, not to the tool palette.** A proceeding has sides; a list of things
you can do does not. That distinction is the Signing Ceremonies panel's to make.

**Place Signing Flags is the top pill**, above Simple Sign and Signing Ceremonies. It is the first
thing a document goes through, and it is the mode's landing panel — so it now leads the column
rather than trailing it.

## What moving a panel to the front exposed

**A mode that lands on a PANEL must not then auto-open a card.** `syncSidebarForMode` opened the
active pane's first `.tbgroup` after landing, and `openCard` deactivates every content panel — so
Collaborate stopped landing on Flags the moment its pane had a card at all. The line was inert for
as long as that pane's palette was the role containers, and went live with this change. It is now
scoped to modes whose first panel IS `commands`. Caught by `tablist.test.mjs`.

**Two geometry guards were reading the column wrong**, and neither could have been noticed before:

- The flush-and-rounded check walked headers and open CARD bodies but not open PANELS. A panel is
  an open body too, and one sitting between two headers is as much a gap as a card's — safe only
  while every content panel was last in the column, which flags no longer is. It reported the
  Flags panel as a 684px gap.
- The same check then reported that panel as a pill with square corners, because it classified a
  row as a body by asking "is it a `.tbgroup`". A row is a body when it is not a HEADER.

Both are the same lesson: a guard written when one arrangement held encodes that arrangement.
