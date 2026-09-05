# ADR-014 — reloading a changed file is a mutation of the SAME document

**Status:** accepted
**Date:** 2026-09-05
**Context:** `/pending 333`'s remedy half; `internal/server/diskstate.go`'s `handleReload`;
`web/app.js`'s `reloadFromDisk` and `recheckDisk`; the six existing writers of `doc.data`.
**Applies:** every future door that installs bytes from a path into an open document. It
constrains what may be built on document identity, which is why it is a decision record
rather than a comment on the route.

## Decision

**Re-reading the file under an open document replaces that document's bytes under its
EXISTING id, and commits through `commitMutation` like any other edit.**

Two halves, and both are load-bearing:

- **The id does not change.** A reload is not a new document.
- **It goes through the commit door**, so it inherits ADR-008's byte cap, D29's ceremony
  freeze, the registration re-test under the write's own lock, and the undo ring. The route
  writes no predicate of its own for any of those.

**The reload is therefore undoable.** That is not a side effect to be tidied away later: the
client fires this route *without the user asking*, on return-to-foreground, and an automatic
action that replaces what she was looking at owes her a way back.

## Why this reverses what shipped

The `Reload from disk` button introduced with `/pending 333`'s detection half went through
`POST /api/open`, producing a **new** document in a **new** view and then closing the old
one. Its comment argued the new id was *honest* — "the bytes are different, so it is a
different document, and pinning it as the same one would be the lie."

**The tree refutes that.** Six sites already replace `doc.data` under a stable id:
`handleUndo`, `handleRedo`, `handleSave`, `commitMutation`, `commitBarrier` and
`installCeremonyResult`. Different bytes under one id is what this package already means by
an edit; a reload is the seventh instance of an established shape, not a novel claim.

The open-then-close version also had a defect nobody had noticed. `handleOpen` computes
`dup := s.docForPath(path) != nil` **before** the install, and the button opened before it
closed — so the old view still held the path and **every press reported `sameFileOpen`**,
toasting *"That file is already open in another tab — these are two separate copies, and
saving one will refuse to overwrite the other"* about a tab that was closed one line later.
A false sentence, on the happy path, every time.

Automating that path would have fired it with no user action at all, alongside three costs
that are tolerable for a button and not for a background action: the tab moves to the end of
the strip, the reading position resets, and at the eight-document cap (ADR-005) the reload
fails outright because open-before-close needs a ninth slot.

## What the automatic half may do, and may not

The client reloads **by itself** only where doing so can cost nothing: the document reports
`diskChanged`, has **no unsaved work**, and is **not in a ceremony**. A document with unsaved
work keeps the banner and the button, because a reload replaces the bytes and would destroy
exactly what the banner exists to protect. The ceremony condition is the server's rule, not a
second copy of it — `commitMutation` refuses a convened document at the freeze, and the client
check only avoids a request whose answer it already knows.

**The trigger stays event-driven — `focus` and `visibilitychange`, never a poll.** Nib has no
watcher and acquires none here: `fsnotify` appears in `go.sum` transitively and is imported
nowhere, and `web/app.js` has no `setInterval`. The uncovered case is Nib in the foreground
and idle while the file changes; it is left uncovered deliberately, because a recurring
request against every open document to answer a question whose answer is almost always "no"
is a poll added to a local-first app, and on a network mount it is the hung-read case
`docResponse` already takes its own stat outside the lock to avoid.

## What this does not decide

**The reading position is not preserved, and this record does not claim it is.** Three
implementations were written and each was measured inert — `scrollTop` 1363 before the reload
and 25 after, identically with the restore present and deleted. Every one of the fifteen
in-place callers of `setDocumentFromServer` lands at the top the same way, so the fix belongs
to that sink and to all of them at once, not to this route (`/pending 372`).

Zoom **is** preserved, structurally rather than by test: `setDocumentFromServer` calls
`setUserScale` only when the arriving id differs from the one the view holds, so a same-id
reload leaves a user-set scale alone.
