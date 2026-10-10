# ADR-114 — a file derived from veraPDF's source says so, and takes its MPL-2.0 option

**Status:** accepted
**Date:** 2026-10-09
**Context:** `/pending 638`, a licensing pass over every way veraPDF enters the repository. veraPDF never ships —
the tests run it as an oracle — but some of nib's own files are derived from its source: `rules_table.go` follows
`GFSETable.checkTable` "step for step", several rules are veraPDF's tests transcribed, and `cff_tables.go` is
generated from `CFFPredefined.java`. veraPDF's own file headers offer it under **GPLv3+ or MPLv2+** (read from
`GFSETable.java` and `CFFPredefined.java` upstream, 2026-10-09); nib is AGPLv3. Nothing in the repository said
on what terms that derived code was held. Dan chose the dual label (`/discuss`, 2026-10-09).
**Applies:** every non-test Go file in `internal/`, `cmd/` and `mdpdf/`.

## Decision

**1. A file derived from veraPDF's source starts with `// SPDX-License-Identifier: MPL-2.0 OR AGPL-3.0-only`**
and a paragraph naming veraPDF, its copyright line and the option taken. MPL-2.0 is file-scoped and its section
3.3 names the AGPL as a Secondary License, so the file stays available under MPL-2.0 and is distributed as part
of nib under the AGPL. That closes the derivative-work question without having to win it.

**2. "Derived" is what the file says about itself.** It names a veraPDF source file (`X.java`), or it says a
veraPDF test, predicate or algorithm is transcribed or ported. **Reproducing behaviour measured by running
veraPDF is not derivation** and earns no header — that is most of the checker. Test files are outside the rule.
Twelve files when written: eleven in `internal/uacheck` and `internal/fontcode/codespace.go`. (`/pending 638`
said eight; `/discuss` said eleven from a grep that matched `d.javaInt` and missed two files.)

**3. `-only`, not `-or-later`.** Nib states its licence as AGPLv3 with no "or any later version" grant, and a
header must not offer twelve files on wider terms than the rest of the program. The decision put to Dan read
`-or-later`; this is narrower, can be widened by him at any time, and could not be narrowed after release.

**4. The notices say it, from the grep.** `build/gen-notices.sh` emits a `## veraPDF` section listing every file
that carries the header — the generator's own search, never a typed list — with the copyright line, the two
licences offered and the one taken, and the non-endorsement.

**5. The six rule summaries that were word-for-word veraPDF's profile descriptions are in nib's own words.**
Clause ids and counts are unchanged. The accurate engineering comments ("ported step for step", the `.java`
line ranges) stay: the header is what covers them.

**6. Corpus files never enter the repository** (`CONTRIBUTING.md`).

## Consequences

- Those twelve files may be reused by anyone under MPL-2.0's weaker copyleft. They are a PDF/UA rule checker.
- A new file that cites veraPDF's source without the header, or carries the header with no stated derivation,
  fails `TestEveryFileDerivedFromVeraPDFSaysSo` (both directions; each was removed and went red, as did a file
  dropped from the notices).
- Left as it is: nine summaries that paraphrase veraPDF's descriptions and twelve output strings that track its
  messages. `/pending 638` measured them and asked only for the six.
