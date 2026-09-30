# ADR-068 — the patched library also patches its lexer, because pdfcpu's gate refuses what pdfcpu cannot read, not what the library loops on

**Status:** accepted. /pending 761. **Supersedes in part** ADR-066's rule that `third_party/digitorus-pdf`
differs from v0.1.2 in `read.go` alone, and ADR-067's consequence that "pdfcpu's gate covers Verify" for the
library's endless loops. ADR-041's decision — pdfcpu reads a document before the signature parser does — stands;
what changes is what that gate is said to protect against.

## Context

ADR-041 put pdfcpu in front of `digitorus/pdf` because the library's lexer reads past an object stream's end
for ever (`readByte` answers `'\n'` at EOF) and a `recover` cannot contain an out-of-memory. The shapes it
measured had a damaged `/Filter`, and pdfcpu refuses those. ADR-067 then recorded, unmeasured, that the gate
"covers Verify" and that only paths skipping it were exposed.

/pending 761 built the remaining shapes as fixtures and ran each path in a subprocess under a memory cap:

- an array, hex string or literal string left open at an object stream's **clean** end (intact filter);
- `endobj` inside an array — as an object-stream member, an ordinary object, or a field's value.

**pdfcpu read all four.** Every one took the process through `HasSignatureBlob`, `Verify`, `Revisions`,
`Sign` and `SignApproval` alike: out of memory in 0.3–12 s, or (the hex string) a spin killed at the timeout.
So the gate was never the containment for this class, and gating the one ungated door (`HasSignatureBlob`,
parked on cost) would not have made it one. Reachability: every install route computes `sign.Verify`, so
opening such a file carrying a `/ByteRange` token killed nib; without the token, save, Save As, tag writes
and reflow did.

The first three are the object-stream view's end, which `read.go` owns. The fourth is not: `readObject`
unreads `endobj` and answers null, and `readArray` hands the token back and appends that null for ever,
with no I/O at all. Nothing in `read.go` can reach that loop.

## Decision

- **The patch covers two files: `read.go` and `lex.go`**, and `lex.go` carries one change — `readArray` refuses an
  `endobj` or `endstream` token through the lexer's own `errorf` ("unexpected keyword %q parsing array"). NOTICE.nib
  divergence 4. `readDict` needs none: a keyword where a key belongs already panics.
- **The object-stream view refuses a read told of the stream's end more than 64 times** (`ErrObjStmRunsPastEnd`,
  NOTICE.nib divergence 3), in `read.go`.
- **The library's one signing call is contained** (`containedLibrarySign`): pdfsign's own walk of the reader has no
  recover, so a reader panic in `Sign` took the process once the loops above became panics.
- **The gate is described as what it is**: it keeps out what pdfcpu cannot read. The comments that said it closed the
  library's unbounded reads are corrected in `verify.go`.
- `TestTheLibraryPatchIsReadGoAndLexGoOnlyAsItsNoticeSays` holds the two-file rule (any third differing file fails,
  and either change reverted fails); `TestAMemberLeftOpenAtTheStreamsEndIsRefusedNotRead` holds both classes over all
  five paths in subprocesses. The fuzz test compares neither class, because the original never returns on them.

## Consequences

- The fork's maintenance surface is two files at every bump, not one. Refused: a nib-side pre-scan (a second lexer
  that only helps while it agrees with the first — ADR-041's own reason for choosing pdfcpu), and leaving the gap
  declared (a process kill on open or save, from a file anyone can send).
- Other unbounded loops in the lexer, if any, are not claimed covered. The two classes above are the ones measured.
- Reporting upstream stays Dan's call.
