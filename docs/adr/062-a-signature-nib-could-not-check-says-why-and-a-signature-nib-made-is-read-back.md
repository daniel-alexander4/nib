# ADR-062 — a signature nib could not check says why, and a signature nib made is read back before it is returned

**Status:** accepted. /pending 741 and 747 (Dan chose 747's option A, 2026-09-29). Extends ADR-059's vocabulary and
ADR-058's record; follows /pending 733 and 740.

## Context

Since /pending 733 a file whose signatures nib's reader cannot reach (a hybrid `/XRefStm` file, a trailer `/Root`
that is not a catalog, a file pdfcpu refuses) reads `Invalid` — the safe direction — but with `AddedAfter=false`, no
cause and no refused record: the user was told "invalid" and, in the CLI, "modified since signing", which is not
what anyone knew. Separately, nothing checked that a signature nib had just MADE was one nib could read back: the
library writes an unreadable or 0-signer document on inputs its reader diverges on (hybrid files, /pending 740; a
hand-built third-party signature, the `synthSigned` fixture), and nib returned it.

## Decision

**A verdict that rests on no checked signer names why.** `Status.Unchecked` (`hybrid-reference`, `unread`,
`unreadable`) is set on the zero-signer path when the verdict is `Invalid` and no signer, refused record or
timestamp explains it, together with `AddedAfter=true` and `AddedAfterCause=could-not-check`. `State` is unchanged.
The CLI and the page each hold one sentence per cause, and a guard in each fails when a cause has none.

**`runSign` reads back what it signed, without a full `Verify`** (`signedAsIntended`): pdfcpu reads the output; the
revision sweep reads both input and output and the output has exactly one more record; exactly one record reaches
EOF, the library would enumerate it, it is not a timestamp and it names the signing certificate; and one PKCS#7
verify of only that signature over its own ranges. Any failure is `ErrSignedOutputUnreadable` and nothing is
returned. The full-`Verify` form was measured and refused: +230% time and +110% peak heap at 100 MB. This form
measured +20–25% time and +50% memory (400 → 600 MB) at 100 MB, and it catches the `synthSigned` divergence — the
sweep fails on the library's output.

## Consequences

- A document the library would have signed into something unreadable is refused at signing time, on every path
  (`Sign`, `SignApproval`, `SignExternal` all end in `runSign`).
- Signing a 100 MB document holds ~200 MB more at peak. Hashing the ranges in place would save 100 MB; not built,
  because it mutates the bytes being returned.
- A hybrid file on which the library still sees SOME signers gets the ordinary verdict and is not named as hybrid.
