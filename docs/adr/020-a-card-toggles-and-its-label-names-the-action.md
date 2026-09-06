# ADR-020 — a card header toggles, and its label names the action

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan, on v1.123.0: *"the sidebar pills are not all expanding/collapsing on click. Make
the titles of the pills clear and explanatory to the actions within, ie Pages -> Arrange Pages"*
**Amends:** ADR-018's *"exactly one card expanded at a time"* — it is **at most** one. The rest of
ADR-018 stands.
**Applies:** anything added to the sidebar.

## Decision

**Every card header toggles: a click on the open card closes it, whichever kind of card it is.**
ADR-018 gave the sidebar two kinds — command groups, which use `.open`, and content panels, which
use `.active` because the tab wiring already spoke it — and only the group half ever grew the close
branch. Measured in a real browser at v1.123.0, in File mode: *Form data* and *Multiple documents*
toggled, *Pages* and *Outline* re-opened on every click and could not be put away. Half the pills
answered a second click by doing nothing, which is what Dan was looking at.

**Showing a panel is not clicking its header, and it gets one door.** `showPanel(name)` clicks only
when the panel is not already open. Two callers mean *show*, not *toggle* — the marker fill path
sending the user to the Library, and `syncSidebarForMode` landing a mode on its own first surface —
and with the header now a real toggle a bare `.click()` at either site closes the card it was
supposed to open. ADR-009: the rule is written once and every site calls it. The tier-3 harness's
`panel()` helper is the same door for tests, and `gestures`/`stamplace` were routed through it
because both clicked a header directly and one of them clicked the mode's own landing panel.

**A card's label names the action it contains, verb-led.** *Pages* → **Arrange Pages**, *Outline* →
**Jump to Section**, *Certify* → **Sign & Timestamp**, *Page setup* → **Size & Number Pages**. The
noun labels were inherited from the toolbar, where a group sits beside its buttons and the buttons
supply the verb; a card is collapsed by default, so its label is all a user has to decide whether to
open it — the noun made them open cards to find out what was in them.

This also retires the collision ADR-018 had to route around: *Pages* the panel and *Pages* the
command group could not both be one-word tabs, and neither is one word any more.

## What was measured

Every pill, in Chromium at 1400×900, before and after: click once, read whether the body renders,
click again, read it again. Before, 2 of 4 pills in File mode collapsed; after, every pill in all
five modes opens and collapses. Label width was measured the same way rather than estimated —
`scrollWidth` against `clientWidth` on the 200px column — because the longest new label, *Place
Signatures & Stamps*, is nearly twice the word count of the one it replaces. Nothing overflows and
nothing wraps: every header is one line, 31–32px, unchanged from before the rename.

## What this does not decide

**Whether a mode should land with a card open at all.** It still opens the mode's first command
group, per ADR-018. That a user can now close it and see nothing but headers is the point of the
toggle, not a default anyone chose.
