# ADR-100 — a search that did not look across every line end says so

**Status:** accepted (2026-10-07). Extends ADR-095.

## Context

ADR-095 made `wrappedMatches` (web/detect.js) the one door for a search match that wraps a line: a stretch of text is
joined to every stretch of the next row that begins left of its end — which of them is the same column is not known —
over up to three lines, and each such chain is searched with each pattern. It recorded the cost as it landed: 170 ms
for 60 lines of 12 stretches, 3.2 s for 200 lines of 20, "quadratic in stretches per line", no real page timed.

Measured (2026-10-07, five patterns, one reading; a machine under other load, so each figure is a floor of three runs):

- **The count of chains is the rule, and it is cubic.** A line of S stretches makes about S²/2 chains of two lines
  and S³/6 of three with the lines after it: 25,894 chains at 60 × 12; 349,192 at 200 × 20; 2.3 million at 100 × 50.
- **Half the time was not the search.** Each chain read every piece on the page to find its next row, built its text a
  character at a time with an object for each, and looked each matched piece up by scanning the list. Rewritten —
  the next row looked up by row, the text put together from each piece's trimmed strings, a match's place in each
  piece worked out from offsets and only for a match that reaches the chain's first and last lines — the same
  matches come back in the same order for: 60 × 12, 172 → 113 ms; 120 × 12, 311 → 200 ms; 200 × 20, 2,177 →
  1,283 ms. What is left is one run of each pattern over each chain's text: 4–5 µs a chain.
- **Real pages are nowhere near it.** The densest page of the accuracy corpus, a tax form with 25 stretches on one
  line, makes 9,386 chains: 56 → 19 ms. The sum over the corpus's 36 mapped pages: 225 → 82 ms.
- **No chain can be dropped without changing what is found.** Whether a pattern matches across a line end depends on
  the pattern; a RegExp gives no way to ask "could this match anything that contains these characters" short of
  running it. So the cubic count cannot be removed and leave the results alone — it can only be bounded.

A page past any figure exists to be written: 100 lines of 50 cells took 8.6 s for one reading, on the window's own
thread, for each page, twice (the estimate's reading and the map's).

## Decision

1. **`WRAP_BUDGET` — 200,000 chains — is the most one call of `wrappedMatches` searches.** The chains are counted
   before any is searched (a sum over the rows; no pattern is run). Past the budget the chains of three lines are left
   out; if those of two lines are past it alone, all are. A match inside one line is the row's own search and is
   never affected.
2. **A call that left chains out says so**: its result carries `skipped`. `matchesInMap` carries it up from either
   layer, `scanTextMatches` names the page (`marks.skipped`), and the Redact text dialog tells the user which pages
   — in the toast when matches were marked, and beside "No matches found" when none were: *"Page 3: check by eye
   for a name or number split across lines — this page is too dense to search across line ends."*
3. **The safe answer for a search is to be told, not to be given more or fewer boxes.** A detector past its budget
   can refuse (ADR-096's `paintedOverBudget` takes a ground as covered and proposes nothing). A redaction search
   cannot refuse the page — the matches inside each line are found and must be marked — and cannot over-cover what it
   did not find. What it must never do is read as complete. So the budget's whole consequence is the sentence.
4. **The rewrite's acceptance is the first version.** `wrappedMatches` as it landed in ADR-095 is kept in
   test/jsdom/wrappedmatch.test.mjs as the oracle, and the two are held to the same matches in the same order over
   400 generated pages and ten patterns.

## Consequences

- The budget is 21 times the densest real page measured and about a second of searching for five patterns. Two tables
  at that edge: 120 lines of 20 cells is past it (206,710 chains; two lines are searched, three are not, and the page
  is named); 60 lines of 20 is inside it (101,710).
- A new reader of `wrappedMatches`' result must carry `skipped` to the user, as the two callers do. The dialog's two
  ends are held by test/jsdom/wrappedmatch.test.mjs.

**Declared gaps.**

- **The sentence has not been seen in the running app**: no document in the corpus or the tier-3 fixtures is past the
  budget, and jsdom holds the wording and both call sites, not the toast on a screen.
- **The budget is per call, not per search.** A document of many pages each just inside it still takes about a second
  a page a reading. Nothing bounds a search as a whole.
- **Inside the budget the cost is still cubic** in stretches per line; the budget is what stops it, not the rewrite.
- The estimate's reading and the map's each count their own chains, so a page can be named by one and not the other;
  it is named once either way.
