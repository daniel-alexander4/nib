# ADR-163 — a signer's text is handed to the signing library already escaped, and read back as typed

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-062 (a signature nib made is read back), ADR-009 (one door). ADR-142 is kept exactly: the
library's `sign/` is not touched. Closes the gap ADR-158 declared (a block drawn for a script whose signing
was then refused): `TestAContributionInAnyScriptIsSignedAsWritten` (internal/p2p) signs one through
`Contribute`.
**Context:** `/pending 879`. `github.com/digitorus/pdfsign` writes the signature dictionary's `/Name`,
`/Reason`, `/Location` and `/ContactInfo` through `pdfString` (`sign/helpers.go:39-48`,
`sign/pdfsignature.go:148-166`). Its ASCII arm escapes `\`, `(`, `)` and a carriage return. Its other arm
writes the raw bytes of the UTF-16BE encoding between parentheses and escapes nothing, and a PDF literal string
gives four bytes a meaning (ISO 32000-1 7.3.4.2). Measured on the tree before this, through `Sign`,
`SignApproval` and `SignExternal`, as the name and as the reason:

| text | the bytes | what happened |
|---|---|---|
| 山田 太郎 | U+5C71 is `\q` | refused by nib's read-back: `invalid escape sequence \q` |
| محمد العربي | U+0628 is `(`, never closed | refused: `EOF` |
| مدينة | U+0629 is `)` | refused: `unexpected delimiter ')'` |
| Müller :-) | `)` is 0x00 0x29 | refused: `unexpected delimiter ')'` |
| naïve C:\dir | `\` is 0x00 0x5C | refused: `invalid escape sequence` |
| 就这样签署 | U+5C31 is `\1`, an octal escape | **signed**, with a `/Reason` of nine bytes of noise, read the same by nib and by pdfcpu |

The refusals were ADR-062's read-back failing on a file the sweep could not parse. The last row parses, so
nothing refused it: a signature over text nobody typed. 1,503 of the 63,488 BMP characters hold a `(`, `)` or
`\` byte in one half of their UTF-16 unit; 497 more hold a carriage return (to be read as a line feed by the
standard; nib's reader and pdfcpu's leave it, so that one is the rule and not a measured failure); 32,512
supplementary characters hold one in a surrogate half. Latin was never exempt — only ASCII was.
**Applies:** `internal/sign` (`librarytext.go`, `runSign`, `signedAsIntended`).

## Decision

**1. Every text nib gives the library for the signature dictionary goes through one door, `libraryText`,
inside `runSign`** — the door every production signature already passes — so `Sign`, `SignApproval` and
`SignExternal` (whose name may be an imported certificate's own) cannot differ.

**2. A text that is not all ASCII is handed over as its own escaped form.** Each byte of the true UTF-16BE
encoding becomes a three-digit octal escape, `\ddd`. Those four ASCII bytes are two UTF-16 units — U+5C30..33
and U+3030..3737 — so the escape is carried as two characters, which the library's unescaped arm encodes back
to exactly `\ddd`. What reaches the file is `(` BOM `\ddd…` `)`: a conforming literal in which every byte but
the library's own BOM is a backslash or a digit 0-7. Every non-ASCII text takes this shape, not only one
holding an affected character, so the common case exercises what the rare one needs. Eight bytes of file per
UTF-16 unit. An ASCII text is the library's to escape and is passed as it came.

**3. The new signature's `/Name` and `/Reason` are read back and must be the text asked for**
(`textAsAsked`, `signedAsIntended`'s fifth conjunct), from the sweep's own record of it — no second walk of
the file (ADR-058). They are inside the signature's coverage. Anything else
is `ErrSignedOutputUnreadable`, naming the field, and nothing is returned.

**4. Refused: patching `pdfString`.** It is the direct fix and three lines. It widens ADR-142's patch from the
verify path to the sign path and is one more thing to re-apply at every bump, for a result decision 2 reaches
with the library byte-identical to upstream outside `verify/`. **Refused: turning the text away where it is
typed** (ADR-158's door for an undrawable character) — nothing is undrawable or unsignable here; that door
would have refused ordinary names (山田, العربي) for a defect in escaping.

**5. Not looked at: upstream v0.9.0** (2026-08-06, four months past the pin). Only its `.info` and `.mod`
are in the module cache; its source was not fetched. If it escapes the string, decision 2 is double escaping
on that version and decision 3 turns every non-ASCII signing into a refusal until `libraryText` is removed.

## Consequences

- A name, a recital or a pinned label in any script is signed, and reads back as typed in nib and in pdfcpu
  (`TestTextInAnyScriptIsSignedAsTyped`, which signs every affected BMP character at once through all three
  entry points). poppler's `pdfsig` 24.02 validates the result; it does not print a reason, so no third
  reader of the TEXT was available on the machine this was built on.
- `TestWhatTheLibraryIsHandedIsAConformingLiteral` reads the bytes: `\ddd` and nothing else.
- **At a pdfsign bump, re-run both.** The guard that the library is unchanged outside `verify/` is ADR-142's
  and is untouched.
- The read side had no defect: nib's verifier reports a reason another tool wrote as a hex string (either
  case), as a literal with escaped bytes, or as octal escapes
  (`TestAnotherToolsTextIsReadWhicheverWayItIsWritten`). One difference from the standard stays, declared: a
  raw carriage return inside another tool's literal is reported as a carriage return, not a line feed.
- `Info.Name` is also what the library draws when a visible signature has no image. nib never asks for that
  (the block is nib's own picture, ADR-158); a text drawn there would be the escapes.
- Signatures made before this are unchanged. A document signed by an earlier Nib with a text like the table's
  last row carries the noise for good; nothing can tell it from a reason typed that way.
- Reporting the defect upstream is Dan's call, as in ADR-142.
