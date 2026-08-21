# ADR-008 — The open-document byte cap binds every door that grows a document

**Status:** accepted
**Date:** 2026-08-20
**Context:** supersedes nothing; extends **ADR-005**. Found by the v1.116.18 full-repo review.

## Decision

ADR-005's aggregate byte ceiling is a property of **the open documents**, not of one
entry point. It is enforced wherever `doc.data` is written to a value the user's
operation produced — `addDocCapped` at open, and both commit doors (`commitMutation`,
`commitBarrier`) for a document already open. The check sums every *other* open
document plus the incoming bytes, so a write that makes a document **smaller** can
never be refused: the bound is on the total after the write, never on the delta.

Undo and redo are deliberately **not** gated. They restore a state this server already
held and already counted; refusing an undo because the document it returns to is large
would leave the user unable to get back to a document they were just looking at, and
those bytes are ADR-003's pool anyway.

A refusal from any of these doors is **409**, carrying `ErrTooManyBytes`'s own sentence.

## Why

ADR-005 says the cap "bounds count AND aggregate bytes". The count half was true of
every door — a ninth document is refused wherever it comes from. The byte half was
true only of `addDocCapped`, and five writers of `doc.data` went straight past it.

The consequence was not theoretical. An OCR text layer, an N-up, a scan import and an
attachment all grow a document in place. Two 200 MiB documents plus a third an
attachment grows to 300 MiB is 700 MiB against a 512 MiB ceiling, refused by nothing —
and the ADR's sentence read as though it could not happen. This is the repo's most
repeated defect shape and not a new one: **a claim wider than what enforces it.**

## Why 409 and not a status of its own

The five user-initiated install routes have always answered 409 for `ErrTooManyBytes`.
A second status for the same fact would mean the same refusal reads differently
depending on which door the user reached it through — which is the drift this ADR
exists to remove, reintroduced one level out.

ADR-004's rule is that a document the server no longer holds is 409 and **never 404**;
it does not reserve 409 for that one fact. The client's 409 hook reconciles and then
shows the message either way, so the cost of sharing the code is one extra
`GET /api/docs` on a cap refusal, which finds nothing changed.

## Consequences

- The two commit doors return `error`, not `bool`. They now have two reasons to refuse
  and a caller mapping one boolean onto two sentences cannot tell the user which
  happened. `errDocClosed` names the other.
- **One mapping, `wroteCommitFailure`.** Eight call sites hand-mirroring a two-arm rule
  is eight chances to have seven — which is exactly how `ErrStampTextUnrepresentable`
  came to be mapped at two producers of three (v1.117.5). `TestACommitFailureIsAlwaysA409`
  was rewritten to guard *routing through the door* rather than the string each branch
  prints, and keeps its floor of eight.
- A test may lower the ceiling through `Server.maxDocBytes`, on the same terms as
  `maxHistoryBytes`. The first draft of the growth test drove the real 512 MiB and
  spent over ten minutes allocating before its first assertion.
