# ADR-222 — A soft mask is a clip to a move, and a request body decoded in another package is in the field census

Status: accepted, 2026-11-10 (PLAN-image-edit P02 phase close). Extends ADR-221 §1; supersedes ADR-221 §4's
rationale and its title's second half.

## Context

The P02 phase-close review found three things ADR-221 said or assumed that the code did not hold.

- ADR-221 §1 refuses a picture under a clip that does not leave the whole page open, because a move under one is cut
  out of sight. An ExtGState `/SMask` hides part of the page the same way and has no box the walk keeps. A picture
  drawn under one was listed movable.
- The clip test compared the clip's box with the page's exactly. A page-sized clip reached through a scaled `cm` can
  land a rounding error inside the page, and every picture under it was refused `clipped`.
- ADR-221 §4 said the route read no `box` because `requestfields_test.go` refuses a field no client sends. That
  guard never saw the route. It followed only bodies decoded inside `internal/server`, and this route hands its
  body to `objedit.DecodeEdits`. `/api/tags/edit` and `/api/tags/commit` (through `tagwrite`) were outside it too.
  All three contributed no pair at all, which reads exactly like a route whose every field is sent.

## Decision

1. **A `gs` whose dictionary sets `/SMask` (anything but `/None`) marks the clip in force rough** (`runWalker.softMasked`,
   in the walk's `gs` arm), so a picture under it is `clipped`. `Q` takes it back with the rest of the state. Alpha
   and blend without a mask change nothing.
2. **The clip leaves the page open to `displayEpsilon` of the page's size** (`clipLeavesPageOpen`). The readers
   that ask whether a run or shape is clipped (`boxWithin`) are not movability and keep their own test.
3. **The request-field census follows a body handed to another `internal/` package** (`foreignDecodeFields`): a
   call passing `r.Body` to `pkg.Func`, where `Func` builds a `json.NewDecoder` from a parameter and decodes into a
   local. On the client side it follows an object a caller built under a name and passed by it, and a caller that only
   forwards its own parameter, two hops (`callersSend`). `/api/tags/edit rect` is declared cli-only (`nib tag edit`
   reads the same shape from a file). Four (route, field) pairs are pinned in the population so the reach cannot
   silently go.
4. ADR-221's route has read `box` since v2.58.0 (P02.S03); its title's "the window's route waits for the window"
   describes a state that ended there.

## Consequences

- Fewer pictures are listed movable: those under a soft mask. The corpus `OBJECTMOVE` line counts them under
  `clipped`.
- A new route that decodes its body in another package is in the census without a change to it. A decode helper
  outside `internal/` is still not followed, which under-reports on the silent side, as before.
