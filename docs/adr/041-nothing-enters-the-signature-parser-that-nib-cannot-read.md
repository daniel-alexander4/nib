# ADR-041 — nothing enters the signature parser that nib's own parser cannot read

**Status:** accepted

## Context

`digitorus/pdf` v0.1.2 lexes object-stream content past its end. `buffer.readByte` returns `'\n'`
forever once the underlying reader is exhausted (`lex.go:71-82`), and two of the token readers have
no end-of-file check:

- `readLiteralString` (`lex.go:229`) appends every byte it sees, so an unterminated `(` allocates
  until the process dies — **`fatal error: out of memory`**;
- `readHexString` (`lex.go:190`) skips whitespace with `goto Loop`, so an unterminated `<`
  **spins** on a core forever, allocating nothing.

`readToken` itself *does* check `b.eof`, so this only happens inside a buffer with `allowEOF` set —
and on the verify path exactly one place sets it: the object-stream reader at `read.go:890`, which
is reached whenever `Reader.Resolve` is asked for an object stored in an `/ObjStm`.

**Neither failure is a panic.** The library's own `recover` cannot catch them, `internal/safe.Recover`
cannot catch them, and Go offers no per-goroutine allocation cap. A process that reaches either is
gone — and on the p2p arm there is no per-request recover to lose either, because there is no
request. The bytes come from **another party**: `p2p.ContributionProgress` (`l3.go:287`) asks
`sign.Verify` what is already on a document that just arrived, and `cosign.go:19,48` do the same.

/pending 453 had already reduced the surface: a document with no `/ByteRange` is `Unsigned` by
inspection and never enters the parser. That left the residue this ADR closes — a **damaged**
document that does carry a signature.

### What was measured, because the original finding was a sample

Every single-bit flip of a freshly signed 3,719-byte fixture, at **every** offset and both `0x01`
and `0x80` — 7,438 runs, each in its own process under a 3 GiB address-space cap so a runaway
landed in seconds instead of taking the machine:

- **12** ended in `fatal error: out of memory`. The earlier pass reported "4 of 2,480" because it
  stepped `off += 3`; the density is the same, the class is three times larger than recorded.
- All 12 lie at offsets 233–238 — the six bytes of the word `Filter` in
  `7 0 obj <</Filter/FlateDecode/First 15/Length 192/N 3/Type/ObjStm>>`. Damaging the **key** means
  the stream is never inflated, so the lexer is handed raw deflate bytes, which sooner or later
  contain a `(` with no `)`.
- The crash is in `verify.Verify` alone. `signatureBlobPresent`, `trailingContentAfterLastSignature`
  and `hasCertificationSignature` were driven over the same damaged bytes and all three returned
  normally — /pending 502 had already fixed those.

**Whether a given flip runs away is luck, and that matters for testing.** The same semantic damage
crashed one fixture and was shrugged off by the next, because the fixture is signed with a fresh
random key each run and the runaway depends on what the compressed bytes happen to contain. A
bit-flip corpus is the right instrument for *finding* this and the wrong one for *guarding* it.

## Decision

**`sign.Verify` asks nib's own PDF parser whether it can read the document, and refuses the
document if it cannot, before `digitorus/pdfsign` is handed a byte.**

`pdfcpuCanRead` is that door. It sits between the existing `/ByteRange` byte scan and
`verify.Verify`, in that order: the scan first so an unsigned document pays nothing, the read
second because it is the only thing standing between a damaged object stream and the process.

**It is one door for 27 call sites.** `verify.Verify` has exactly one caller in this repo, and
`sign.Verify` has twenty-seven across eight packages. A gate written "on the receive path" would
have had to be written at each of them and would have missed the twenty-eighth. This is ADR-009 as
`handoff.go` states it — the *check* is unified, not the sentence each surface prints.

**The refusal is `Invalid`, never `Unsigned`.** A document carrying `/ByteRange` that no parser can
read is "something is wrong with a signed document". `scanForSignatureBlob`'s own comment already
settled which way to err: *telling a user a broken document may carry a signature costs them a look,
where telling them a tampered signed document was never signed costs them the thing they were
relying on*. `p2p.ContributionProgress` already renders that state as "this document carries a
signature that cannot be read".

**Relaxed validation is pinned, not inherited.** `model.NewDefaultConfiguration()` returns the
user's `config.yml` when one exists, so a machine configured for strict validation would give this
gate a stricter rule than the one measured — and a gate stricter than measured refuses documents nib
can read. The question here is only ever *can this be parsed at all*.

### Why pdfcpu rather than the three options the finding listed

| | why not |
|---|---|
| **A — patched v0.1.2 behind a `go.mod` `replace`** | A fork of a PDF parser is a standing tax and a supply-chain surface nib then owns, re-applied at every bump, and carried through `gen-notices.sh`. It is the only option that fixes the defect *at* the defect — which is why it stays on the table if pdfcpu and digitorus ever diverge. |
| **B — verify in a subprocess under a memory cap** | Changes nib's process model on three platforms for one call. There is no `RLIMIT_AS` on Windows; it needs a Job Object, and nib would be shipping a second executable entry point plus a document-sized IPC round trip on a path taken by undo, redo and every document open. |
| **C — a nib-side object-stream lex gate** | A second implementation of the reader it guards, which helps only while the two agree. That is the differential the finding itself warned about, written by hand. |
| **D — pdfcpu, chosen** | Already in the binary, already reads every document nib opens, and is a genuine independent implementation rather than a re-derivation of the one it guards. |

### The differential, measured rather than argued

A guard that disagrees with what it guards is the entire risk, so both directions were measured
over the same 7,438 flips:

- pdfcpu returned a verdict on **7,438 of 7,438**. It neither crashed nor hung on the corpus that
  kills the library it guards.
- pdfcpu refused **12 of the 12** that were fatal.
- The cell that would matter — **pdfcpu refusing a document digitorus reports `valid`** — is
  **empty**. Zero.
- The only behaviour change on the corpus is **10** flips moving `unsigned` → `invalid`, which is
  the direction the code already argues for.

**Encryption was measured separately**, being the obvious place for two libraries to differ: six
shapes (AES-256, AES-128, RC4-128 and RC4-40; owner-only and with a user password). pdfcpu is at
least as permissive as digitorus in **every one**, and the single shape digitorus reads (RC4-40,
owner-only) pdfcpu reads too. Digitorus v0.1.2 refuses AES outright.

**Cost**: 0.15–0.31× the `Verify` call it precedes, over 3.7 KB (1 page) to 94 KB (400 pages). That
is the measured range and it is not extrapolated past it. It is **zero** for an unsigned document.

## Consequences

- A damaged document from a remote party produces a refusal a user can read instead of a process
  that is gone. The failure had no diagnostic at all: no panic, no log line, no exit code worth
  reading.
- **The hang is covered too, and it was not in the original finding.** `readHexString` spins rather
  than allocating, so no memory watch and no OOM report would ever have named it. Both paths are now
  driven deterministically: in an object stream whose `/Filter` name is damaged, a same-length
  payload of `(aaa…` reaches `readLiteralString` and `<444…` reaches `readHexString`. pdfcpu refuses
  both with `decodeObjectStreamObjects: problem decoding object stream 7`.
- **The test forks, because the subject is a process death.** `p2p`'s
  `TestADamagedDocumentFromARemotePartyIsRefusedRatherThanFatal` re-runs this binary with an env
  marker and drives `ContributionProgress` in the child. The child bounds **itself** — an allocation
  watcher for the OOM and a deadline for the spin, since the spin allocates nothing and the watcher
  is blind to it — so neither failure can reach the machine, and both are portable rather than
  resting on `RLIMIT_AS`. Proven red: with the gate inert the child exits 9 (runaway allocation) and
  10 (spin) while still printing `CONTROL-ADMITTED`, so the failure is specific to the damage and
  not a gate that refuses everything.
- **Not addressed, deliberately.** A real-world signed document that pdfcpu alone refuses is badged
  `Invalid` — a false "modified", not a crash. Zero such cases exist in the corpus above and none in
  the encryption sweep, but the corpus is one fixture's flips and one tool's output. A bump of
  either library can move the line; the ordering guard in `verify_test.go` is what fails if the door
  is moved, and this consequence is what to re-measure if the badge is ever wrong.
- The upstream defect is untouched and unreported. v0.2.0 bounds `readLiteralString` and **cannot be
  adopted**: `pdfsign/verify/verify.go:89` calls `Reader.Resolve`, and v0.2.0 has no such method —
  a `grep` for `Resolve` across the whole v0.2.0 module returns one hit, a comment in
  `benchmark_test.go`. Its `readHexString` still spins.
