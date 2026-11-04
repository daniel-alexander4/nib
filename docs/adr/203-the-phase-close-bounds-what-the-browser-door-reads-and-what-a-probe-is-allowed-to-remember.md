# ADR-203 — The P04 close: a probe remembers only a verdict, and a book is bounded in total

Status: accepted, 2026-11-04 (PLAN-import-export P04 phase close, v2.44.1). Extends ADR-198, ADR-199, ADR-200, ADR-201, ADR-202.

1. **A cached probe holds a verdict, never a moment.** The Ghostscript, HEIC and browser probes cache a result per binary only when it is
   success or the tool's own refusal (`ErrGhostscriptNotConfined`/`TooOld`, `ErrHEIFCannotRead`, a browser that wrote the wrong page).
   A timeout, an over-memory stop, a shutdown, `ErrNotServed` or a cancelled context is returned and not remembered
   (`probeUnfinished`): one slow first start used to disable the format until the binary changed.
2. **The origin's own end exists.** The origin refuses a `peercred.Gone` socket: it has just accepted the connection, so "no such
   socket" is a far end that closed before the lookup, which is how another user would spend the single use. `Unknown` (off Linux) is
   still served, the declared gap. ADR-157's trade door keeps `Gone` as allowed: a different door with a different question.
3. **An EPUB is bounded in total.** A spine item is read once, and the chapters' sanitised output together is capped at
   `epubOutputBytes` (96 MiB): each chapter's picture budget is its own, so 300 chapters could write 300 budgets.
4. **A converter's stderr is a tail.** `tailBuffer` keeps the last 16 KiB; a hostile program writes to stderr for as long as the
   timeout lets it, and the buffer lives in Nib's own process.
5. **A message's style sheet does not share a page with Nib's header block.** `.eml` drops the message's `<style>` elements.
6. **Declared, not closed:** the Ghostscript probe tries a file read and a pipe, not the `uniprint` device switch of CVE-2024-29510, so
   a stock upstream gs below 10.03.1 passes floor and probe (/pending 943). The sanitiser's recursion is bounded by the parser's own
   512-node open-stack limit, pinned by a test rather than re-implemented.
