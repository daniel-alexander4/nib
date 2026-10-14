# ADR-172 — a ticked box of a Yes/No pair is the answer the window saves

**Status:** accepted
**Date:** 2026-10-14
**Extends:** ADR-167 (the vendored viewer carries a marked patch, pinned by hash and re-applied at every bump).
**Context:** `/pending 893`, found by `build/formrt.sh` once it judged every field kind (/pending 889 part 1). A check
box field whose two widgets carry different on-states — `Yes` and `No`, the pair Acrobat builds for a yes/no
question, and which pdfcpu exports as a radio group — lost its answer when the window saved it: **20 of 180 such
answers in five real documents** (two are copies; one a government loan form) read back from pdfcpu as no option.
Traced on the saved bytes, the ticked widget's `/AS` is right and the field's `/V` is `Off`. The window reopens it
ticked (pdf.js reads each widget's `/AS`), so the user sees nothing wrong; every reader of `/V` — pdfcpu's export,
`nib export-xfdf`, a mail-merge, the other party's software — reads no answer. pdfcpu's own fill of the same fields
is right.

The mechanism is pdf.js's worker (6.1.200; the same code in 6.4.299). `_saveCheckbox` calls `setValue` for an
UNTICKED widget too — `_saveRadioButton` guards the same call with the widget's state — and `setValue` writes the
parent field's `/V` only for the first widget saved (`if(!n.has(r))`). Whichever box is saved first decides, and in
a pair whose No box comes second, ticking No stored `Off`.
**Applies:** `web/vendor/pdfjs/pdf.worker.min.mjs`, `web/vendor/pdfjs/NOTICE.md`, `vendorpatch_test.go`,
`test/ui/checkpair.test.mjs`.

## Decision

**1. The worker carries one marked span** (`Nib (ADR-172)`), in `setValue`: when the parent's `/V` has already been
put for this save and is `Off`, the widget being saved now replaces it. A ticked box therefore wins whatever order
the boxes are saved in; an unticked one never overwrites a ticked one's state (the `Off` test is the half a looser
fix breaks — mutation-red in the tier-3 test's second case); a single box being unticked still writes `Off` (its
put is the first); radio buttons call `setValue` only when chosen and text values are never the name `Off`, so
neither is touched.

**2. Pinned as ADR-167's patch is.** `vendorpatch_test.go` holds the worker to its published hash with the span
put back, and fails until the patch is re-applied (or dropped where upstream has fixed it) at a bump.
`test/ui/checkpair.test.mjs` is red on the worker as published.

**3. Refused:** repairing `/V` server-side after the window saves (a second writer over bytes pdf.js wrote, and a
pass over every save for one field shape); clearing the unticked sibling's storage before the save (its `/AS`
would then stay `On`, and both boxes read ticked); waiting for upstream (6.4.299 is unchanged). Reporting it upstream
is Dan's to send, as mozilla/pdf.js #22179 was.

## Consequences

The window's saves of such pairs store the answer the user ticked. A document saved with the published worker
before this keeps its `Off`; the window shows it ticked, and re-saving it in Nib writes `/V` again. Not covered:
a check box with more than two on-states (none in the local corpus), and a box ticked in a document that is
already signed (ADR-140's refusals stand).
