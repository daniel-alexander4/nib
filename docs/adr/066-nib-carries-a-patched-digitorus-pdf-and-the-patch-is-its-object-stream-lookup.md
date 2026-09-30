# ADR-066 — nib carries a patched `digitorus/pdf`, and the patch is its object-stream lookup alone

**Status:** accepted. /pending 758. Supersedes ADR-041 in part: its refusal of option A (a patched v0.1.2
behind a `go.mod` `replace`). ADR-041's decision — pdfcpu reads the document before the signature parser
does — stands unchanged.

## Context

ADR-041 refused to fork `digitorus/pdf` because a fork "is a standing tax and a supply-chain surface nib
then owns", and kept the option open "if pdfcpu and digitorus ever diverge". The defect it faced was a
CRASH, and the pdfcpu gate could remove it: a document pdfcpu cannot read never reaches the library.

/pending 751 then found a COST. `(*Reader).resolve` finds an object-stream member by decoding the stream
from its start and lexing its `id offset` header pairs until the id matches, then seeking forward to the
member — for every member, caching nothing. The revision sweep and pdfsign's verify each resolve every
xref entry (ADR-058: the same enumeration), so a stream of N members costs N²/2 header pairs and N²/2
decoded bytes per pass. /pending 751 could only bound it from outside (`libraryLookupCost`, refusing a
document past a ceiling), and set the ceiling where it did because an **honest** file already paid it:

- InDesign's `census-p60-280.pdf` (2.7 MB, `~/nib/producers/indesign/`) — **3.26 s a pass**, two passes
  per verdict, on every install, mutation and undo.
- A signed file with one 20,000-member stream — 66 s a pass; the verdict took 129 s, and /pending 751
  had to refuse it `Invalid` although nothing about it is wrong.

**The gate cannot remove a cost, only a crash.** A document pdfcpu reads is handed to the library, and
the library then pays the quadratic; the only lever outside the library is refusal, and refusal is
`Invalid` — a false "modified" for an honest document. That is the divergence ADR-041 kept the option
open for: pdfcpu reads these files in milliseconds, digitorus in seconds to minutes.

## Decision

**`go.mod` replaces `github.com/digitorus/pdf` v0.1.2 with `third_party/digitorus-pdf`, a copy of v0.1.2
that differs from upstream in `read.go` alone**, plus `NOTICE.nib` saying what changed and why.

The patch is **faithful by construction, not by test**: a per-`Reader` cache per object stream holding
the decoder, kept open, with the bytes decoded SO FAR, and the header pairs lexed SO FAR. Both halves are
lazy — extended only as far as the lookup in hand needs — so a lookup reaches exactly what upstream's
reached and nothing past it:

- a corrupt zlib tail or a short `/Length` after the wanted member is never decoded, and the member
  resolves, as upstream resolved it;
- an `/N` over-claiming the header, with a bad token later, still yields the members listed before it;
- first listing wins, and ids truncate to `uint32`, as upstream's comparison did; past the header's end
  every pair reads as `(0, 0)`, as upstream's loop read them;
- a panic part-way (lexer or decoder) is kept and raised again by any later lookup that needs to go past
  it, as upstream's fresh decode would reach the same point again.

The member is read from its own offset over the cached bytes; the stream dictionary's checks
(`/Type`, `/N`, `/First`, `/Extends`) are still made per lookup from a fresh read, and the `/Extends`
walk is unchanged. The cache key is every input the decoded bytes depend on — the stream's own objptr
(which decryption keys on), its data offset, and its raw `/Length`, `/Filter` and `/DecodeParms` — so
two streams share an entry only where they decode identically. No mutex: every `Reader` is one call's.

**One divergence, declared.** Upstream read the member by seeking the header's own buffer; a `/First`
pointing behind that buffer's current 4 KB chunk is a backward seek, which set a negative index and
panicked. The patched reader resolves it. No producer writes such a `/First`.

**The gate stays, re-fitted to the patched reader** (`internal/sign/objstm.go`): header pairs and decoded
bytes are charged once per stream, as far as the furthest lookup reached; each lookup is charged a
dictionary read for every stream it visits, which is the quadratic the patch did not remove (an
`/Extends` chain times its members). `maxLookupWork` keeps its value as the backstop. The decoded bytes
are also what the reader now HOLDS for a pass, so the byte term caps its memory (~57 MiB).

## Measured

One pass = resolve every xref entry on a fresh `Reader`, best of three (base = `ff591413`).

| file | base | patched |
|---|---|---|
| `memberDoc` 1,000 members | 103 ms | 11.8 ms |
| 5,000 | 3.18 s | 46 ms |
| 10,000 | 15.3 s | 113 ms |
| 20,000 | 66.4 s | 173 ms |
| `census-p60-280.pdf` | 3.26 s | 0.50 s |

The 20,000-member file now verifies `valid` in about a second; on base it was refused `Invalid`
(could-not-check), as was every file from 5,000 members up.

All 36 producers, one pass (base → patched; the 22 files with no object stream stay at 0.06–2.6 ms either way):
the dearest were `census-p60-280.pdf` 3.26 s → 0.49 s, `irs-p15.pdf` 730 → 105 ms, `fda-170357.pdf` 536 → 71 ms,
`fda-82395.pdf` 280 → 44 ms, `irs-f1040.pdf` 155 → 35 ms. Verify on the synthetic files, patched: 43 ms (1,000),
146 ms (5,000), 258 ms (10,000), 446 ms (20,000), all `valid`.

The cost figure is above the measured in-stream time on every file (1.1x–3.6x): the 36 producers, the
four synthetic sizes, `/Extends` chains 100x100 to 2,000x500, and a 16 MiB array member. A 1,000-stream
chain under 1,000 members (6.2 s a pass patched) is past the ceiling.

**Differential**: `sign.Verify`'s State, signer count, each signer's position, fingerprint and validity,
`AddedAfter` and its cause, `Unchecked` and `Timestamps` — over the 36 producers and the 403 distinct
documents `internal/sign`'s own tests hand `Verify` (captured on base and replayed on both trees) —
**identical on 437 of 439 rows.** The two that differ are the intended ones: the 8,000- and 20,000-member
files, which base refused `Invalid` (`could-not-check`) at /pending 751's ceiling, now read `valid` with their one
signer and nothing added after it.

## Consequences

- nib owns a copy of a PDF parser: **re-apply the patch at every `digitorus/pdf` bump.**
  `TestTheLibraryPatchIsReadGoOnlyAsItsNoticeSays` fails when the require line leaves v0.1.2, when the
  replace stops pointing at the copy, when any file but `read.go` differs from the module-cache
  original, or when `read.go` no longer carries the cache.
- `build/gen-notices.sh` heads a directory-replaced module "modified by nib" and names its NOTICE.nib,
  and refuses to generate if that notice is missing.
- ADR-041's crash surface is unchanged — the patch lexes exactly the tokens upstream lexed — so the
  pdfcpu gate stays where it is.
- Not charged, declared: a member's own extent per lookup (two ids whose header pairs name one offset
  each read that member), and anything a stream dictionary's keys REFER to.
- Reporting the defect upstream is Dan's call.
