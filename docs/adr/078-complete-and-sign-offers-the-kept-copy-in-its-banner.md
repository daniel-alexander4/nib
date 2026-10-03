# ADR-078 — Complete & sign offers the kept copy, in its banner

**Status:** accepted. `/pending 814` (2026-10-03). Supersedes ADR-073 decision 1 in part — the clause "Complete &
sign … offer[s] no tick". The rest of ADR-073 stands unchanged, including that the CLI offers none.

## Context

`/api/finalize` has two web callers: the Finalize modal, which ADR-073 gave "Keep a copy for my records", and the
recipient's Complete & sign (`completeAndSign`), a one-press flow that posted fixed params and so never sent `keep`.
A recipient who signed that way kept nothing — and the recipient's signature is the one a dispute over a returned
document turns on: they are the party who must later show what they signed when the sender's file comes back changed.

The question was where the tick lives without turning a one-press flow into a modal. The flow already has one
surface on screen for its whole life: the signing banner (`#signBanner`), which carries both of Complete & sign's
entry points — the primary "Mark complete & sign" once every field is filled, and "Finish & sign" while any is not.

## Decision

1. **The tick sits in the signing banner**, beside both buttons, as "Keep a copy for my records" with the Finalize
   modal's disclosure beneath it. The disclosure has **one source**: the banner's hint is filled from the modal's
   `#fzKeepHint` at start-up, so the two ticks cannot say different things about the same copy.
2. **Off at every opening, never remembered** (ADR-073 D13, unchanged): cleared when the banner is shown for a
   document, when the active document changes (a tick is for the document it was made on), and after a signing.
3. **Same request, same refusal.** Complete & sign sends `keep` and `name` exactly as the modal does — `name` is the
   name its Save As offers, without the preparer's `-for-signing` — and a `copy-not-kept` refusal is worded by the
   one function the modal also calls (`keptRefusal`, ADR-009). The banner stays up with the tick, so unticking and
   signing again is one press away, as the modal's staying open makes it there. A kept copy is announced by the same
   function (`announceKept`), which also re-asks the checklist row.

## Consequences

- No modal is added: the flow stays one press for a recipient who does not want a copy, and one tick plus one press
  for one who does.
- Everything ADR-073 says a kept copy proves and does not prove applies unchanged. For a recipient in a counterparts
  signing it is their counterpart, not the executed original — every surface still calls it "a copy kept when you
  signed".
- The CLI still offers no tick; it already writes where the user names.
