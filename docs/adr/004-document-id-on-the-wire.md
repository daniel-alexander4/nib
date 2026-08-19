# ADR-004: Document identity travels on the wire, in an `X-Nib-Doc` header

**Status:** Accepted
**Date:** 2026-08-16

## Context

Nib's server used to hold exactly one working copy — `s.doc` — and every
document-touching route acted on it. Holding several documents at once cannot be
solved on the client: a client-side view registry in front of a server that still
holds one working copy means every server operation performed in one tab rewrites
the others. The identity has to reach the server, which makes it a wire concern.

Roughly 30 of nib's ~68 registered routes touch the document, and they take three
different body shapes — bodyless POST (`/api/undo`, `/api/redo`), JSON, and
multipart. Any scheme carried in the body would have had to edit three schemas and
every call site that builds them.

## Decision

**Server-side, `s.doc` becomes an ordered registry plus an active id**, reached
through a single `activeDoc()` accessor, so the pre-existing `doc := …; if doc ==
nil` guard shape is preserved at all ~14 sites rather than rewritten.

**On the wire, the document id travels in an `X-Nib-Doc` request header.** A header
crosses all three body shapes without touching a single body schema, and it matches
the existing `X-CSRF-Token` convention.

Four rules complete it, and each exists because of a specific failure:

1. **`/api/pdf` takes a query parameter instead, and that is not an inconsistency to
   tidy away.** That URL is fetched by **pdf.js**, not by nib's own `apiFetch` —
   `getDocument({ url: '/api/pdf?…' })`. The fetch belongs to the library, so a
   header would mean opting into pdf.js's `httpHeaders` plumbing to buy a uniformity
   nobody reads. The URL already carries a cache-buster; the id joins it. **Recorded
   here so it is not "fixed" later by someone who did not know why it exists.**

2. **The id is optional and defaults to the active document — for the CLI and the
   pre-existing Go tests only.** Around 20 Go tests `GET /api/pdf` with no id, and
   nib's CLI verbs address a single document by construction; requiring an id would
   edit twenty tests to say nothing new.

   **The web client always sends one, and that is enforced by the transport rather
   than by discipline.** `apiFetch` attaches the calling view's captured id to every
   document-touching route, so a call site cannot omit it by forgetting. This is the
   dangerous half of the rule: a pinned call and an unpinned call differ by the
   *absence* of a header — no error, no log line, and nothing in review distinguishes
   "correctly defaulted" from "forgot". A call site that simply forgets gets
   "whatever the server currently thinks is active", which during exactly the switch
   ADR-001 exists to survive is the wrong document, and it commits having passed no
   check because it never made one.

3. **An id naming a document the server no longer holds is `409`, never `404`.** 404
   already means "no document open"; a closed tab is a different fact, and the client
   must tell them apart to remove the tab rather than blank the app.

4. **The id carries a per-process epoch (`<nonce>:<counter>`), and a mismatched epoch
   is also 409.** ADR-001 makes ids monotonic and never reused *within a process*; a
   restart restarts the counter at 1. That is usually harmless because the default
   `NIB_ADDR` is `127.0.0.1:0` and a restart takes a different port, so a surviving
   browser tab cannot reconnect. But `NIB_ADDR` pins a fixed port for headless and
   remote runs, which is a documented supported mode — and there a stale tab
   reconnects and can pin to an id the new process has since reassigned to a
   different document. That is ADR-001's exact failure crossing a process boundary.
   A mismatched epoch is 409 because it is the same fact as a closed document: the
   document you named is not one I hold.

## Consequences

- **A new document-touching route must accept the header**, and a new client call
  site must go through `apiFetch` rather than `fetch`. Neither is enforced by the
  compiler; both are enforced by the guard below.
- *Guarded by* `test/jsdom/pinning.test.mjs`, which walks every `apiFetch` call to a
  document route and asserts `X-Nib-Doc` is present — proven red by removing the
  attachment in `apiFetch`. No other criterion covers it: the failure is an absence,
  and absences are invisible to tests that assert on what is sent.

## Alternatives considered

- **A body field.** Rejected: three body shapes, ~30 routes, and two of them have no
  body at all.
- **A required id everywhere.** Rejected: it edits ~20 existing Go tests and every
  CLI path to state something that is true by construction there.
- **Making `/api/pdf` uniform with the rest.** Rejected — see rule 1. The uniformity
  is real and buys nothing, and the cost is pdf.js header plumbing.

## Provenance

Settled as decisions D6 and D15 in the multiple-open-documents plan, with the
who-may-omit-the-id rule and the per-process epoch added by that plan's reviews.
Built as P03.S02/S03 (v1.103.14). The plan itself is retired; this ADR is the
surviving record.
