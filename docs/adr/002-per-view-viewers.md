# ADR-002: One PDFViewer per document, hidden rather than destroyed

**Status:** Accepted
**Date:** 2026-08-16

## Context

With several documents open, switching between them has to preserve what the user
has done to each: scroll position, zoom, current page, filled form fields, and —
the hard part — the typed contents of Nib's own overlay widgets.

The obvious cheap design is one viewer that is re-pointed on every switch, with
each document's state serialized on the way out and rebuilt on the way in. It was
tried on paper and it does not survive contact with where the state actually
lives.

**An overlay's value lives in the DOM, not in Nib's model.** A text overlay's
content is `f.el.value`; a checkbox's is `f.el.checked`. And pdf.js's
`setDocument()` calls `_resetView()`, which does `viewer.textContent = ""` — so
re-pointing the viewer destroys every overlay element and everything typed into
one. Preservation would therefore mean a serialize/rebuild round-trip written by
hand for **twelve** overlay kinds, where a single missed property loses the user's
typing with no error and no way to notice until they look.

## Decision

Each open document owns its own `PDFViewer` and its own `#viewerContainer` /
`#viewer` pair, nested inside the stable `#viewerWrap`. Inactive views are hidden
with `display: none`. **The page DOM is never torn down.**

Preservation is then the browser's default rather than something Nib implements
per overlay kind: a hidden element keeps its value because nothing removed it.

## Consequences

- **Correct by construction for state Nib does not know about.** Anything living
  in the DOM survives a switch whether or not Nib has a model for it, including
  overlay kinds added later. That is the whole argument.
- Hidden views cost memory: each keeps its rendered page DOM. This is bounded by
  the open-document cap and is the price of the property above.

  **(correction, 2026-08-17, P05 phase-close review — the bound named above does
  not exist.)** The decision stands; this consequence overstated what backs it.
  There is no open-document cap in the code, in count or in bytes: `Server.addDoc`
  appends with no length test and nothing but a full close removes an entry, and
  client-side `views.push(arrival)` is equally unbounded. D9's cap is planned for
  P06 (`PLAN.md`), so the sentence describes an intended state in the present
  tense. Measured cost meanwhile: roughly 35 MiB of canvas and ~2,100 nodes per
  hidden 300-page document, on top of the page DOM, and one more arrives with every
  completed co-signature. Recorded as a dated note rather than an edit, because an
  ADR's text is the record of what was decided and when — see `_index.md`.

  **(resolved, 2026-08-17, P06.S01 + P06.S04 — the bound now exists.)** D9's cap is
  built: **eight documents** (`maxOpenDocs`, P06.S01, which is the slice that made
  unbounded growth reachable by making Open ADD) **and 512 MiB of aggregate document
  bytes** (`maxOpenBytes`, P06.S04, chosen against a measurement recorded in
  `PLAN.md`), refusing on whichever binds first. The sentence above is true again.

  **And the figure this note quoted was wrong.** "Roughly 35 MiB of canvas and ~2,100
  nodes per hidden 300-page document" was an estimate; P06.S04 measured it in a real
  browser across 3-, 50-, 150- and 300-page documents and found the canvas cost is set
  by the **viewport**, not by the document's length — about 12–22 MiB per view whether
  it holds 3 pages or 300, because pdf.js renders only what is near the viewport and a
  hidden view keeps what it had rendered. The node count did scale with pages (~2.2 per
  page on the near-blank measurement fixtures, so a floor rather than a typical figure),
  reaching ~690 for 300 pages rather than 2,100. The practical consequence is the
  opposite of what the estimate implied: **the client is bounded by the COUNT cap, not
  by the byte one**, because per-view cost barely varies with document size.
- `display: none` has measurement consequences that must be handled explicitly —
  a hidden container reports `clientWidth` 0, so a view that loads while hidden
  gets no scale and must be re-fit when it is activated. Verified empirically
  before the decision, not assumed.
- Anything that resolves "the viewer" or "the overlay fields" at call time becomes
  a hazard: a command recorded against one view and executed against another
  corrupts the wrong document. Those resolutions must become per-view.
- Document-wide DOM sweeps become wrong. `document.querySelectorAll('.splitmark')`
  reaches into hidden views and removes marks from documents the user is not
  looking at; cleanup has to be view-scoped.

## Alternatives considered

- **One viewer, snapshot overlays on switch and rebuild on return.** Rejected —
  the reasoning above. Twelve hand-written round-trips, each of which loses user
  data silently when it is subtly wrong, against a design where the data is never
  removed in the first place. Recorded as decision D4 in the plan.
- **Server holds N documents, client renders one and reloads on switch.**
  Rejected: it discards transient overlay state on every switch — the state that
  has not been baked into the PDF yet is exactly the state a user has just typed.
- **Destroy inactive viewers, keep only a serialized model.** The same objection
  as the first, plus it re-renders every page on every switch, which is the cost
  this design exists to avoid.

## Provenance

Settled as decision D3 in the multiple-open-documents plan (2026-08-15), with D4
recording the rejected alternative. The `display: none` behaviours were verified
empirically during the deepdive that preceded the plan rather than reasoned about.
