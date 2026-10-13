# ADR-142 — nib carries a patched pdfsign, and the patch is one buffer per signature

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-066 (nib's first patched module, and its maintenance rule), ADR-141 (the ceiling on how many
signatures reach the library).
**Context:** /pending 735, decided by Dan 2026-10-12. `sign.Verify` runs on every install, mutation and undo,
and for each signature `github.com/digitorus/pdfsign`'s `processByteRange` read every `/ByteRange` pair with
`io.ReadAll` into a buffer of its own and then appended that buffer to `p7.Content`. Measured through
`sign.Verify` before this (goprobe, best of 3, a document padded to size and signed three times, the cost of
each signature after the first): **6.3, 6.1 and 7.1 times the file** at 1, 4 and 16 MB. With ADR-141's 64
signatures that is about 400 times the file allocated per verification.
**Applies:** `third_party/digitorus-pdfsign/` (`verify/signature.go`, `verify/verify.go`, `NOTICE.nib`),
`go.mod`, `librarypatch_test.go`, `THIRD-PARTY-NOTICES.md`.

## Decision

**1. `go.mod` replaces `github.com/digitorus/pdfsign` with `third_party/digitorus-pdfsign`,** a copy of the
pinned version that differs from it in two files and adds `NOTICE.nib`. The require line keeps the pin. This is
ADR-066's shape, held by a guard of the same shape
(`TestTheSignatureLibraryPatchIsItsVerifyPathOnlyAsItsNoticeSays`): the pin, the replace, the patch still
present, and nothing else differing from the module-cache original.

**2. The patch is `processByteRange` reading the pairs straight into `p7.Content`, grown once** to the bytes
the pairs can yield in a file of the size `VerifyWithOptions` was given. `verify/verify.go` changes by that
one argument. Measured after, the same way: **1.3, 1.1 and 1.0 times the file** per signature.

**3. What is read, in what order, and every error stay upstream's.** Each pair still goes through
`io.NewSectionReader` and is read to its end; the file's size chooses a capacity and nothing else, so a wrong
one costs an allocation and cannot change a byte. `libraryWouldOverread` is written against upstream's
behaviour on hostile ranges and stays exactly as it is — including its refusal of an indirect `/ByteRange`,
which the patched function no longer re-parses per pair but which conjunct (1) refuses as a record anyway.

**4. `p7.Content` stays one whole slice.** `digitorus/pkcs7` hashes it with a single `h.Write(p7.Content)` and
has no streaming entry point. Hashing the ranges without holding them needs a patch to that module too, and
that is refused here: a third patched module for the last 1× is not worth its maintenance.

## Consequences

- The copy carries the whole module — `sign/`, which nib signs through, unchanged, and upstream's tests and
  test PDFs (about 1.2 MB) — because the guard's meaning is "identical to upstream except these two files".
- `go.sum` loses the module's two lines, as it did for `digitorus/pdf`: a replaced module is not checksummed.
- **Re-apply the patch at every bump of pdfsign.** The guard fails until it is.
- Declared in `NOTICE.nib`: `/ByteRange` is resolved once per signature (upstream: three times per pair); on a
  read error `p7.Content` holds the failing pair's partial read; a full buffer asks the section for no bytes
  to learn it has ended.
- `TestVerifyingASignatureAllocatesAboutWhatItCovers` (internal/sign) holds each signature after the first to
  twice the file; upstream's function measures five to eight there.
- The CPU half is unchanged: every signature's covered bytes are still hashed, at most 64 times (ADR-141).
- Reporting upstream is Dan's call.
