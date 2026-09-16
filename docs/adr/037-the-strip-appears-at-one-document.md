# ADR-037 — the document switcher appears at one document, and the close controls do not

**Status:** accepted
**Date:** 2026-09-16
**Context:** Dan: *"Even if only one page is open it should open in a tab."* `syncTabs` in
`web/app.js`; `#tabstrip` in `web/index.html`; `web/style.css`'s switcher rules.
**Supersedes:** the appear-at-two rule stated in those three files and in `test/ui/tabs.test.mjs`,
`test/jsdom/restore.test.mjs` and `test/jsdom/perview.test.mjs`. It was never written as an ADR —
it lived as a comment in three places, which is why this record says where it was.

## Decision

**The strip is shown whenever a document is open, and hidden only when none is.** One open
document gets one tab, with its name and its ×, exactly as two do.

**The close controls keep the appear-at-two threshold**, and that is the half of this decision that
is easy to miss. `syncTabs` had ONE predicate — `views.length > 1` — doing three jobs: the strip's
visibility, `#closeBtn`'s label (*Close* vs *Close view*), and whether `#closeAllBtn` shows. They
are now two predicates, because they answer different questions:

| | test | question |
| --- | --- | --- |
| the strip | `views.some((v) => v.pdfDocument)` | is there a document to put in a tab |
| the close controls | `views.length > 1` | do *Close view* and *Close all* mean different things |

With one document open, "close this view" and "close everything" *are* the same act, and offering
both is still two buttons for one outcome. Nothing about showing a tab changes that.

## The predicate is not `views.length >= 1`, and that is the trap

`views` always holds one view — the empty one the app launches with (`const views = [view]`). So
`views.length >= 1` is true **with nothing open**, and the obvious reading of "show it at one"
would have put an empty strip on the launch screen, under a viewer reading *"Open a PDF to begin."*

The question is whether any view holds a **document**, which is what `#viewerWrap.has-doc` and the
toolbar's document title already key on. A closed view has its `pdfDocument` nulled
(`web/app.js`, the close and reset paths), so the test is sound in both directions.

## What it costs, stated

**The document name is now in two places at once.** `#docTitle` in the toolbar and the tab both
carry it whenever one document is open; before this they alternated. That is a real duplication
and it is accepted: the title also carries the unsaved dot and survives at widths where the strip
scrolls, and ADR-024 put it there deliberately.

**Every launch-state assertion had to be re-read, not just flipped.** Three tests asserted
`tabstrip.hidden === true` and each meant something different by it: *one document is open*
(`tabs.test.mjs`, `perview.test.mjs`, `restore.test.mjs`) and *nothing is open*
(`restore.test.mjs`'s all-stale case, `perview.test.mjs`'s launch-state case). The first group
becomes a tab **count** — which is the stronger assertion, and the one the old rule made
impossible: with the strip hidden below two, the tab count at one document was 0 whether the app
held one document or none, so those tests could not tell the right outcome from the worst one.
`restore.test.mjs` says so in its own comment, and that comment is the reason this ADR exists in
the form it does.

**The focus fallback fires less often.** `syncTabs` moves focus to the menubar's first control when
the strip disappears under a keyboard user who just closed a tab. Closing down to one document no
longer disappears the strip, so that path now runs only on closing the last document — the
assertion in `test/jsdom/clientdoors.test.mjs` moves with it rather than being deleted.

## What this does not decide

**Whether the toolbar title should yield to the strip.** It could hide itself once a tab carries
the same name, and it deliberately does not: that is a second change to the same surface, and this
one is Dan's instruction taken literally and no further.
