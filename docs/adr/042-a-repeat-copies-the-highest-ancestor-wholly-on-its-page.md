# ADR-042 — a repeat copies the highest ancestor wholly on its page, and never the `/Document`

**Status:** accepted

## Context

`Collect` may name a page more than once — `DuplicatePage(pdf, p)` *is*
`Collect(pdf, ["1-p", "p-"])`, and a user may type `1,1,1`. Since P02.S04b the selection carries the
source structure tree onto the pages it keeps, and the carry copies the elements a repeated page
needs.

**It copied the wrong thing, and every gate said it was fine.** A `/ParentTree` row names a LEAF —
a `/Lbl`, an `/LBody`, a `/TD` — so a carry driven from the parent tree copies leaves and nothing
else. Grouping elements have no MCID and therefore no row, so `/L`, `/LI`, `/Table` and `/TR` were
never copied, and each copied leaf was attached beside its original, under the ORIGINAL page's
parent.

Measured on a tagged Markdown document (`DuplicatePage(1)`, 15 elements in, 26 out — not 30):

- the source's one `/L` and its three `/LI` had no counterpart on page 2;
- page 2's `/Lbl` and `/LBody` were children of page 1's `/LI`, which then held **two** `/Lbl` and
  **two** `/LBody` and read `"••first item of the list"`;
- **page 2 had no list structure at all.**

Measured on veraPDF's `7.5 Tables/7.5-t01-pass-a.pdf`, which is the louder case and the one that
decides the design: the header `/TR` of a five-column table came out with **ten** cells, reading
`"Index Index Failure Condition Failure Condition Section Section Type Type How How"`, and the
duplicated page had no `/Table` and no `/TR` — a reader on it hears no table.

**Every existing check scored that output as correct.** Every element was anchored, every MCID
resolved, `structureCarriedCompletely` was empty, the operation reported `carried`, there were zero
orphans, and veraPDF passed the file. Nothing in the package compares a row's width to its table's,
and nothing asks whether a grouping element describes content on one page or two. So this is not a
defect a gate missed; it is a defect the gates cannot express.

## Decision

**A repeated page's carry copies the HIGHEST ancestor whose whole subtree lies on that page**, and
the copy is spliced in as a block after the original's, rather than element by element.

Three parts, each load-bearing:

1. **Climb, don't refuse.** The climb is monotone and floored at the leaf, so a legal clone root
   always exists. The alternative considered — clone the highest wholly-within element and REFUSE
   the carry where none exists — was refuted by measurement: a table spanning the page boundary has
   a wholly-within ancestor at every leaf, so refusing buys it nothing and costs it its entire tree.
   `span` refuses on anything the copy would drop (an inline child, an OBJR with no cloned
   annotation, an unresolvable `/K`), not merely on a second page.

2. **Stop at `/Document`.** A file holds one; repeating a page repeats a division of content, not
   the document. Without the stop, a naive climb duplicates the `/Document` element itself. The rule
   is self-limiting as well as stated: a multi-page `/Document` spans pages, so `span` would stop
   the climb anyway — the explicit stop is what makes a single-page document behave like every
   other one.

3. **Splice the copy as a BLOCK.** Copies are grouped per parent and inserted after the last
   original this carry is copying, or after any copy an earlier repeat already placed there. That
   is what makes the reading order page-then-page instead of element-then-element, and it is also
   what fixes three repeats coming out in reverse order, which the old insert-one-at-a-time code
   recorded as a known defect in its own comment.

**The cost is flat and was measured, not projected.** The climb is 1.5–3.0 µs per row leaf,
independent of document size, because `span` bails at the first page disagreement — a spanning
ancestor costs two kids, not a subtree — and the `/Document` stop is checked before `span`, so the
whole-tree walk never happens. End to end, `DuplicatePage` is within noise at 8/37/73/146 pages, and
a whole-document doubling is +9–18%, most of which is the extra elements now actually being copied.

## Consequences

**A duplicated page is a duplicated page.** Its list is a list, its table is a table, and the
original's `/LI` holds one `/Lbl` again.

**`/P` must be an indirect reference.** ISO 32000-1 Table 323 says so, and the old code mutated a
direct `/P` dict, attaching the copy to nothing. A `/P` that is not indirect now routes to the root.

**The declared gap: the carried tree is ordered against the SOURCE, not the output's page
sequence.** `Collect(["1","2","1","2"])` yields a tree over output pages 1, 3, 2, 4, and
`Collect(["3","1","2"])` yields 2, 3, 1. That is pre-existing — the prune appends in source order
and nothing re-sorts the root's `/K` — and closing it means ordering the whole tree against output
page order, which is a different job. It is stated here rather than left to be rediscovered.

**What this does NOT do is raise the completeness gate.** The gate scored the broken and the
repaired carry identically and still would; it asks whether every element is anchored and every MCID
resolves, which was true of both. The reader that can tell them apart is the tag tree read out, and
that is what the tests for this decision assert.
