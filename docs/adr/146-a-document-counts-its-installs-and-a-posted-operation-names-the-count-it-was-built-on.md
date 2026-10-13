# ADR-146 — a document counts its installs, and a posted operation names the count it was built on

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-001 (operation pinning), ADR-004 (document identity on the wire; 409 for "not that document"),
ADR-009 (a rule gets one door).
**Context:** /pending 792 and 667 #1. Since /pending 646 a commit whose input the server handed out
(`snapshotBase`) is refused when the document no longer holds those bytes. Two routes could not be covered:
`POST /api/pages` and `POST /api/outline` commit a PDF the WINDOW posts — its own bake of the document plus its
overlays — so the server has nothing to compare, and two such operations built on the same document both landed.
The second replaced the first, and the undo entry it pushed was the state before the first, so undo could not
bring the lost edit back. Slice identity cannot close it: the posted bytes are never a slice the server holds.
**Applies:** `internal/server/undo.go` (`document.install`, `echoedBase`, `commitMutation`), `server.go`
(`document.rev`, `docResponse.Rev`), `pages.go`, `outline.go`; `web/app.js` (`loadedRev`, `reloadIfMoved`).

## Decision

1. **A document carries a revision: the number of times its bytes have been replaced since it was registered.**
   0 as opened, +1 at every install. It is per document and per process, like the id it sits beside (ADR-001); it
   is never stored and means nothing across a restart, where the id's epoch already refuses (ADR-004).

2. **`document.install` is the one writer of `document.data`, and it moves the revision.** The two commit doors,
   the history step (undo and redo), the save and the ceremony install all call it. A census
   (`TestEveryInstallOfDocumentBytesBumpsTheRevision`) fails on any other assignment to a `.data` field in
   `internal/server`, so a sixth door cannot install bytes under a revision that still names the old ones.
   An undo moves the revision forward, not back: the count says "replaced", not "which bytes".

3. **The revision is published as `docResponse.rev` and echoed as the form field `baseRev`** by the two
   posted-bytes routes. The name is not `X-Nib-Revision`: that header is ADR-072's, and carries facts about a
   SIGNED revision of the file — a different thing. A commit whose echo is not the document's revision at the
   moment of the commit is refused with `errDocChanged`, **409**, through `wroteCommitFailure` — the same refusal a
   stale snapshot gets, by one integer compare under the lock the commit already holds. No second status and no
   second sentence: it is the same fact reaching another door (ADR-008's reasoning about one code).

4. **The echo is optional, as `X-Nib-Doc` is (ADR-004 rule 2), and for the same callers.** The CLI and the Go
   tests send none, and a request without one stays last-writer-wins — the declared gap. The window always sends
   it. A `baseRev` that is present and not a whole number is 400, never read as absent.

5. **The window echoes the revision of the bytes its view HOLDS, not of the last answer it read.** `loadedRev` is
   set where the view's pdf.js document is replaced, and after a save (the server then holds exactly what the view
   baked). A refused operation asks `/api/docs` for the document's revision and, when it differs, reloads that one
   view and says so (`reloadIfMoved`) — without that a view the document has moved past would be refused for ever.

## Consequences

- Two windows (or two operations in flight) on one document can no longer silently overwrite each other through
  the page and outline routes; the later one is refused and reloaded.
- **What the reload costs:** anything typed into that view since it loaded (form fills, overlay fields not yet
  baked) was built on the document before, and goes with the reload.
- The refusal shares 409 with "that document is gone", so the window's 409 hook still runs one
  `reconcileWithServer` beside it (/pending 792 I1 — ADR-004's recorded choice, unchanged).
- Residue, declared: `docResponse` reads the revision in a later lock hold than the commit, so an answer can name
  a revision one newer than the commit it answers. For every route that reloads, the bytes fetched are at least
  that new, which is the safe direction. For the save that does not reload it is not: a commit by another window
  landing between the save's install and its answer is then adopted unseen. Microseconds wide; not closed here.
- `commitBarrier` takes no base and is still last-writer-wins against a concurrent commit; nothing here changes it.
