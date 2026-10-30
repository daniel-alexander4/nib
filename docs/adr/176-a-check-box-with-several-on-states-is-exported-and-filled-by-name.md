# ADR-176 — a check box with several on-states is exported and filled by name

**Status:** accepted
**Date:** 2026-10-30
**Extends:** ADR-174 §4 (pdfcpu reads a Yes/No pair as one check box), ADR-172, ADR-009.
**Context:** `/pending 922`. pdfcpu v0.16.2 reads a check box as a boolean: its export writes `Yes` for any `/V` but `Off`
(`form/form.go:493`) and its fill turns a boolean into `/V` (`form/fill.go:693-727`). So a Yes/No pair answered No was
exported as ticked, as one answered Yes is; a single box whose on-state is `Ja` was exported as `Yes`; an edit from No to
Yes was a silent no-op (the requested boolean equals the held one); and a bare `true` onto an unanswered pair ticked both
boxes.

**Decision.**

1. **One door reads a check box's on-states** (`checkStatesOf`, checkstate.go): the names each widget's `/AP /N` draws but
   `Off`, and the field's `/V` name. pdfcpu stays unpatched (ADR-174).
2. **The exports carry the name beside the boolean.** XFDF writes the on-state the field holds, for every check box (the
   spec's export value); CSV writes it for a field with several on-states and `true`/`false` for one with one; JSON adds
   `"state"` to an answered box with several on-states and is otherwise pdfcpu's bytes.
3. **The fills read it.** A value that is one of the on-states' names (any case) selects it; an off word clears the box; a
   bare yes onto an answered field keeps the answer; a bare yes onto an UNANSWERED field with several on-states is refused
   by name. pdfcpu is handed the boolean the document already holds, so it changes nothing, and `applyCheckPicks` writes
   `/V` and each widget's `/AS`.

**Declared.** A field whose `/V` is on a kid rather than the field is read as pdfcpu reads it (the field's). A widget with no
`/AP /N` on-state key draws no state, so a pick naming it sets `/V` and leaves that widget as it was.
