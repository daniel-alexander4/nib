# ADR-180 — An ended ceremony is removed only when the user presses Remove

**Status:** accepted
**Date:** 2026-10-31
**Context:** `/pending 874`, from `/pending 361`'s decision C (Dan, 2026-10-12). Supersedes ADR-012's
"No code may delete a ceremony directory except `unconvene`" for this one route, and nothing else in it.

## Decision

**`POST /api/ceremonies/ended/remove` deletes one closed-out ceremony's folder, and only when the user asks.**
The request names one id; the work is `ceremony.RemoveEnded`, which removes `~/nib/ended/<id>` and any numbered
folder `CloseOutMirror` set aside beside it. Nothing else in Nib removes what a close-out moved: there is still
no timer, no sweep and no setting (`/pending 361`, B declined). The close-out still moves and never deletes.

- **It reaches `ended/` only.** The live folder `ceremonies/<id>` is not named by it, so a proceeding that took
  the same id after a close-out is untouched. The id is `ValidID`'s, the root must be absolute, and a folder that
  is a link is left alone rather than followed.
- **The window asks first, in words that carry the cost.** On every machine but the convener's, `ended/<id>` holds
  the only copy of that party's own signature (ADR-012), and the window cannot tell whether another copy exists, so
  the confirmation says that a copy with their signature is deleted from this computer and may be the only one.
  The server holds no confirmation of its own: the route is behind the session token (ADR-054), and a request is
  the user's act the way every other destructive route's is.
- **A second copy of Nib on this machine refuses it** (409), as it refuses to continue a ceremony: both would write
  to `~/nib`.
- **One door, one caller.** `ceremony.RemoveEnded` has one production caller, the handler
  (`TestRemoveEndedHasOneCaller`); `RemoveMirror` keeps its own, the convene rollback. A new site that deletes a
  ceremony folder is a new decision and supersedes this ADR.

## Why not the alternatives

A timer was declined by Dan (`/pending 361`): deleting a user's own signed contribution on a clock is the one thing
in the lifecycle a user cannot undo. Moving the folder somewhere else on Remove (a trash) was not asked for and
would leave "remove" meaning "keep", which is the kind of word that costs trust the first time someone looks.

## Consequences

The list can shrink. A removed ceremony cannot be re-read by `nib verify` against its record or by the ceremony
copy door (`ReadMirrorFor` falls back to `ended/`, and finds nothing). Declared: the vault stores were already
destroyed at close-out (ADR-012), so a removal leaves nothing behind in the vault to tidy.
