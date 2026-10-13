# ADR-141 — nib checks at most 64 signatures on one document, and says so past that

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-058 (the sweep runs before the library), ADR-059 (`could-not-check`), ADR-067 (a refusal on
cost names itself in `Unchecked`).
**Context:** /pending 735, decided by Dan 2026-10-12. The signature library hashes every signature record's
byte ranges, so `sign.Verify` costs records × signed bytes, and it runs on every install, mutation and undo.
The gate in front of the library (`libraryWouldOverread`) holds each record to the file's size and says
nothing about how many there are: M records each naming `[0 size]` pass it one at a time. Measured before
this, on a ~2.2 MB file whose extra records were 60-110 bytes each and shared one real PKCS#7 by reference:
0.58 s at 100 records, 2.1 s at 400, 9.9 s at 1,600, 19.1 s at 2,000 — the verdict still `valid`. No bound
on the total exists that an honest document passes and that is tighter than records × size, because each
signature of an honest N-signature document covers nearly the whole file.
**Applies:** `internal/sign/revisions.go` (`maxLibrarySignatures`, `libraryRecords`), `verify.go`
(`verifySwept`, `UncheckedTooManySignatures`), `internal/cli/commands.go` (`uncheckedWords`), `web/app.js`
(`UNCHECKED_WORDS`).

## Decision

**1. A document that would hand the library more than 64 signature records is refused before the library
reads anything.** `maxLibrarySignatures = 64`; the check is the first thing `verifySwept` does, so it holds
for `Verify`, `Revisions` and the boundary walk's prefixes alike.

**2. It counts the records the LIBRARY processes (`libPos >= 0`), never every swept record.** A record the
library drops before reading a range — a blob that does not parse, a filter it does not know, a `/Perms`
signature with no `/SigFlags` — costs it nothing. Counting those would let 65 inert dictionaries, about
7 KB, turn a document with one honest signature `Invalid`.

**3. The refusal has its own cause: `Unchecked: too-many-signatures`.** The verdict is `Invalid`, with
`AddedAfter` warning `could-not-check` (nothing measured the coverage — the invariant every `Unchecked` cause
keeps), `Refused` and `Timestamps` as swept, and **no signer listed**: none was checked, and the first 64
would read as the document's signers. `could-not-check` alone was refused as the answer — a reader given
only that cannot tell this document from a damaged one. The CLI and the details panel each have a sentence
for it, held to the constant by the two existing censuses.

**4. `libraryWouldOverread` keeps its per-record sum; no cross-record total is added.** The plan named one
(the sum "resets per record"). With the ceiling in place it would be inert: at most 64 records arrive, each
held to `size`, so the total is already at most 64 × size — a running total compared with that product
refuses nothing new, and compared with anything smaller it refuses honest documents. The per-record sum
stays because the library holds one record's ranges at a time, so it is what bounds memory.

## Consequences

- Measured after, 2 MB file, records naming `[0 2000000]` and sharing one blob: 64 library records verify
  in 0.49 s (`valid`); 65, 401 and 2,001 are refused in 10 ms, 78 ms and 363 ms — the residue is the sweep
  itself, which reads every record.
- **Declared: an honest document with more than 64 signatures reads `Invalid`** with a sentence saying why,
  and cannot be co-signed, carried or finalized through nib. The most on any document measured here is 30.
- **Declared: the worst case is still 64 × the file's size hashed per verification.** ADR-142 takes the
  allocation per signature from about six times the signed bytes to about one; the hashing is not reducible
  without a streaming PKCS#7.
- The number is a product limit and Dan's. Raising it is a one-line change with a measurable price; it is
  not a stored or wire format.
