# ADR-001: Operation pinning — no operation acts on a document it did not capture

**Status:** Accepted
**Date:** 2026-08-16

## Context

Nib held exactly one open document for its whole life, so "the document" was
unambiguous: any code that wanted it read `pdfDocument` on the client or `s.doc`
on the server and was necessarily right. Making Nib hold several documents removes
that guarantee without removing the reads.

A `/deepdive` over the client measured the damage before any code was written:
**38 sites read `pdfDocument` or `docMeta` after an `await`, and 13 of those can
corrupt.** `save()` is the worst of them — it captures document A's bytes, then
after the await reads B's `docMeta.canSave`, and POSTs A's content to a server
whose working document is B. The result is A's content written over B's file, past
the signature guard, with no error and nothing on screen to suggest it happened.

The failure needs no unusual timing. Any switch during any in-flight operation
does it, and every one of Nib's document operations is in-flight for as long as a
PDF takes to process.

## Decision

**No operation may act on a document it did not capture at its start.**

Every asynchronous operation captures its document and that document's id *before*
its first `await`, carries the id to the server, and discards a reply whose id no
longer matches what it captured. The server refuses an operation whose id is not
the document it names.

**Document ids are a monotonic counter for the life of the process. They are never
an index into the registry and are never reclaimed when a document closes.**

That second paragraph is not an implementation note; it is the load-bearing half.
The whole law reduces to an id comparison, so an id that can be reused defeats it
**silently and completely**: close document 3, open another that is also assigned
3, and an operation pinned to the *old* 3 passes its check and commits to the new
document — the exact corruption this ADR exists to prevent, now wearing a guard
that reported success.

## Consequences

- Every async document operation gains a capture at its top and a comparison at
  its end. There is no way to opt a call site out; a site that does not pin is a
  site that can corrupt.
- The wire grows a document id on every document-touching route. `/api/undo` and
  `/api/redo` stop being bodyless requests.
- The safety is **invisible**. Nothing a user can see changes, which is precisely
  why it cannot be deferred until something breaks — the breakage is silent data
  loss in someone else's file.
- Ids being monotonic means a long-running process assigns large numbers. This
  costs nothing; they are opaque and never displayed.
- The guard is cheap to get right now and **impossible to detect later**, because
  a reused id makes the failure look like a correct comparison. There is no log
  line, no exception, and no way to tell the corrupted save from a legitimate one
  after the fact.

## Alternatives considered

- **Serialize operations — one document operation at a time, globally.** Rejected:
  it makes the app feel broken (no switching while an OCR runs), and it does not
  actually close the hole, since a switch between an operation's start and its
  commit is still possible unless switching is also blocked.
- **Re-read the document after the await and bail if it changed.** Rejected as
  the same idea done less reliably: it checks identity at one point instead of
  carrying it, so any site that forgets the re-read is silently wrong, and the
  wrongness is invisible in review. Capturing at the top makes the correct shape
  the readable one.
- **Recycle ids from a free list.** Rejected — this is the failure described
  above, and it is attractive precisely because it looks tidy.
- **Compare by document pointer rather than id.** Works on the client, fails on
  the wire: the server needs a value it can receive and compare, and a pointer is
  not one.

## Provenance

Settled as decision D7 in the multiple-open-documents plan (2026-08-15), from a
`/deepdive` that measured the 38 sites; the id-reuse pin was added by that plan's
`/plan-review` as its single **critical** finding. Recorded here because the plan
is a temporary artifact and this law outlives it.
