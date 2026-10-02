# ADR-072 — a refusal about a document is a 422 naming its cause, and a fact about returned bytes rides in a header

**Status:** accepted. `PLAN-returned-document.md` P02.S03 (2026-10-02). Extends ADR-004 (409 is "not that document")
and ADR-054 (every route behind the session).

## Context

`GET /api/document/revision?signer=<fingerprint>` returns the version of the open document that a signer signed — or a
reason there is none, of five: `no-signature`, `resaved`, `not-your-signature`, `prefix-failed-reverify`,
`could-not-check` (the fifth parked for Dan as an amendment to D7). Two questions had no answer in the repo:

- **What status a refusal is.** 409 is taken: ADR-004 makes it "the document you named is no longer open", and
  `apiFetch` answers every 409 by reconciling the open tabs with the server — a refusal sent as 409 would drop a
  perfectly open document's state. 404 is the empty state (nothing open). The refusal is a fact about the document's
  contents, valid request and valid document both.
- **Where the facts about returned bytes go.** The body IS the signed version — raw PDF bytes that go straight to the
  compare pipeline (`getDocument({data})`). What is known about them — which signature held, whether the version came
  from an earlier revision, which object a later revision redefined, whether the search for a later version was cut,
  the working copy's recorded history — has nowhere else to ride.

## Decision

1. **A refusal about a document's contents is `422 Unprocessable Entity` with a JSON body naming its `cause`** (and here
   `refused[]`, every signature nib could not read, and `attributed`). Never 409 (ADR-004's, and it triggers a
   reconcile) and never 404 (the empty state). `apiFetch` passes 422 through untouched, as it already did for
   `/api/identity/external`'s wrong passphrase.
2. **Facts about returned bytes ride in one response header, as compact JSON** — here `X-Nib-Revision` — built field
   by field from the producing type (a projection, so every field has a named reader), each list capped with a
   `truncated` flag so a hostile file cannot grow the header past what a browser accepts.
3. **A malformed request parameter is 400**, normalised first where the producer's form is case-insensitive (a
   fingerprint is lowercased; an unnormalised one would match nothing and read as a false `not-your-signature`).

## Consequences

- The client tells the five causes apart by `cause` alone (`fetchSignedRevision`), and a 422 never disturbs the tabs.
- The header form is JSON rather than a `k=v` grammar: one parser, already on both sides.
- `history` reports RECORDED history only — `undo`, `evicted`, `none` — and `none` does not mean "the bytes as they
  arrived": a barrier operation clears undo without marking it evicted, and a save replaces the bytes with no history.
  The surface (P03) words it so.
