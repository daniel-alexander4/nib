# ADR-027 — the sign checklist ticks only what it can see

**Status:** accepted
**Date:** 2026-09-07
**Context:** Dan, after asking for the steps of a simple sign: *"Under Simple Sign, Let's add each
of those items as links and provide some way to show if the item is 1) optional/required
2) completed or not."*
**Extends:** ADR-026 (Simple Sign is one card).
**Applies:** the checklist, and anything that adds a step to it.

## Decision

**The Simple Sign card lists the steps of signing a document, in order, each row a link to the tool
that does it**, with two attributes: `required` or `optional`, and done / not done / **not
tracked**.

**It is a checklist, not a wizard.** Nothing is enforced and nothing is skipped for you. The order
is advice — but it is advice containing two one-way doors, applying a redaction and signing, and
saying so out loud is the reason the surface earns its place.

**A step is ticked only where Nib can observe it.** `done` is a probe or it is `null`. Null renders
as "—" and says on hover that Nib cannot tell. Nib genuinely cannot know whether you ran a
hidden-content scan, or emailed the file, or where you put an `.ots` — and a tick not backed by a
signal is worse than no tick, because the entire value of the list is answering *what is left*.

Seven steps have real probes today: the key is enrolled, a signature image exists, the autofill
profile is filled, a document is open, a stamp is placed, flags are planted, the document is
signed. Eight are honestly untracked.

**`required` means required to the SPINE** — open a document, seal it, keep the result — not to
your document. Everything else is optional because some documents need it and some do not.

## Why the list is declared, not written in the markup

Every row's state is computed, so a row hard-coded in `index.html` would be a claim about progress
that nothing checks. The container is empty and `renderSignSteps()` fills it, re-running on the
same funnel that already fires for every load, client edit and save.

## What this cost

**A temporal-dead-zone trap, walked into with the warning in front of me.** `renderSignSteps()`
was called from the early boot block, above the module-scope `const SIGN_STEPS` it reads — so it
threw, and took every later line of `app.js` with it. It presented as an empty checklist and an
Open dialog that did nothing. `buildSidebarAccordion`'s own comment names this exact trap for its
accent list, six hundred lines away.

**And a caption that rendered nothing.** The seam above Nib's own transport commands was written as
`.menucap`, which is `display: none` inside `#commands` — the card header carries the name there
(ADR-018). It has its own class now.
