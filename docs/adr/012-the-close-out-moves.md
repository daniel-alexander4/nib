# ADR-012 — The close-out moves a ceremony's folder; nothing deletes it

**Status:** accepted
**Date:** 2026-09-02
**Context:** D29's lifecycle pin (*end state → delivery round → close-out*);
`PLAN-signing-ceremony.md` P08.S06, C09 and C11; D28's four end states.
**Applies:** [ADR-009](009-one-door-per-rule.md) — the one-door rule is the mechanism
here, not the decision, and is not restated.

## Decision

**A ceremony that has ended is MOVED out of the live set, never deleted.**
`ceremony.CloseOutMirror` renames `~/nib/ceremonies/<id>/` to `~/nib/ended/<id>/` — one
`os.Rename`, same filesystem — and the record and any termination travel with the
document.

Three parts, and all three bind:

- **The mirror is preserved; the vault stores are destroyed.** Pins, invitation secrets
  and the stored invitation are key material and go, through one door
  (`closeOutStores`, four stores including the invitee-side invitation). The mirror
  holds no key material by D29's own design, so there is nothing in it to destroy.
- **`RemoveMirror` is the ROLLBACK's verb, not the close-out's.** It stays an
  `os.RemoveAll` and keeps its one production caller, `unconvene`: a convene whose commit
  failed never produced a contribution, so there is nothing to preserve. One door with a
  verb parameter would be two behaviours wearing one name.
- **Nothing in the tree removes what was moved.** `~/nib/ended/` grows by one directory
  per ceremony this machine took part in. That is a decision and is filed as
  `/pending 361`, not an oversight.

**A close-out always leaves a local receipt** — `{ceremony, state, observed_at}`,
**unattested**, written into the moved directory — and it is **write-once on the state**:
the first thing this machine observed is the one that stands.

## Why

**On every machine but the convener's, `~/nib/ceremonies/<id>/document.pdf` is the only
place that party's own signature exists.** A declined, expired or abandoned ceremony has
no delivery round, so nothing has carried it anywhere else. A close-out that deleted would
destroy a user's signature as an act of tidying — and `RemoveMirror`'s own doc comment
invited exactly that, naming P08.S06 as its caller in a sentence written before the
preservation rule existed.

**The receipt exists because `Termination` deliberately has no `When`.** That exclusion is
P08.S04b's finding: a convener-chosen timestamp driving other machines' retention would
hand a convener control of when they prune. So retention counts from a local observation,
and the receipt is where that observation lives. It is the only unattested artifact in the
ceremony set, because it is the only one that never leaves the machine that wrote it — and
it is the only one that can carry `expired` or `abandoned`, which nobody can sign.

**Write-once, because the four states are not equal in standing.** `declined` and
`completed` come from a verified termination; `expired` and `abandoned` are conclusions
drawn from a clock, and the sweep produces `abandoned` past the grace for anything with no
end state. Without the rule, a re-sweep replaces *"they declined on the 2nd"* with
*"nothing ever said"* — the better answer destroyed by the worse, leaving no trace.

## What this binds

- **No code may delete a ceremony directory except `unconvene`.** A new destructive site
  is a new decision and supersedes this ADR rather than joining it.
- **Every store that holds ceremony-scoped material goes through `closeOutStores`.** It is
  four today. A fifth store added anywhere is added there, not at a call site — the count
  was three in the plan and four in the code, and building to the plan would have left the
  ceremony secret on disk.
- **A destructive close-out refuses a non-absolute root before it acts.**
  `defaultOutputDir` returns a bare `"nib"` when `os.UserHomeDir()` fails and reaches
  twenty-four call sites; the guard is at the one destructive door, not the resolver.
- **`nib verify` may name an end state only from local records, under a heading that says
  so.** The document does not carry one and cannot: D25 forbids a structural write after a
  signature. A verifier that printed an end state as though the document said it would be
  making a claim the file does not support, and the same file on another machine produces
  no such line at all.

## What was considered and rejected

- **Copy the document out, then `RemoveAll`.** Two syscalls, a window where both copies
  exist, and a partial copy on a full disk. `os.Rename` is atomic and total.
- **Keep the folder in place and mark it ended.** The live set is what `ListStored` walks
  and what the sweep and the re-arm read; a marker means every reader grows a filter, and a
  reader that forgets one treats an ended ceremony as live. Moving makes the exit
  structural.
- **A retention sweep with the close-out.** Deleting a user's own signed contribution on a
  timer is materially bigger than anything else in the lifecycle and is the one step a user
  cannot undo. `/pending 361`.
