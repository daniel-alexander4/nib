# ADR-080 — a record is checked by the digest rule it names, and rule 5 covers what the reader sees

**Status:** accepted. Supersedes in part ADR-013 (its claim that a coverage change "reads as tampering across a
point release") and ADR-056/057 (their exemption list: `ContentDigest` and the accessibility checker are no longer
exemptions from `pdfread.PageContent`). `/pending 718, 719, 720, 578, 616`.

## Context

Four coverage defects in `ContentDigest` (`internal/pdfops/attachments.go`), the digest every ceremony record's
`DocHash` commits to, were filed rather than fixed, each for the same stated reason — changing what is hashed is a
`ContentDigestVersion` bump, and a bump "reads as tampering across a point release" (ADR-013):

- **718** — it hashed pdfcpu's bare `/Contents` join, so `[(A) Tj, ET]` (text shown) and `(A) TjET` (not shown)
  digested alike.
- **720** — an annotation's `/P` (or a link's `/Dest`) led the walk into the page dictionary and its content
  stream's DICTIONARY, so `/Length` and `/Filter` were covered on every page something links to: a no-op re-encode
  moved the digest on 21 of 35 real-producer documents.
- **578** — `/MediaBox`, `/CropBox`, `/Rotate` were read from the page's own dictionary: `/Rotate 90` on a `/Pages`
  node left the digest where `/Rotate 0` had it. (Its attachment half was closed by v4, /pending 725.)
- **616** — `/OCProperties` (which hides content), `/OpenAction`, `/AA`, `/Names /JavaScript`, page `/UserUnit` and
  page `/AA` were outside the digest and outside its declared exclusions.

**The objection was already half-stale.** `Record.DigestVersion` has been carried beside `DocHash`, inside the
convener's signature, since P07.S02, and `digestRuleSkew` reads it at every comparing door — so a bump has not read
as tampering since then. What it read as was a SKEW: *"update Nib"*, which halts every ceremony in flight across a
point release, for all parties on either side of it.

## Decision

1. **A record is checked under the rule its `DigestVersion` names, by a build that still computes that rule.**
   `pdfops.ContentDigestAt(pdf, v)` computes rule `v`; `ContentDigestRuleReadable(v)` says which rules this build
   computes — the current one and the one before it. `ceremony.DocumentHashFor(pdf, r)` is the one door every
   comparing site calls (`CheckDocument`, `ReadMirror`, the arrival gate); `DocumentHash`, the current rule, has one
   production caller, `Convene`, and `TestOnlyConveneHashesUnderTheCurrentRule` holds that. A rule this build does
   not compute is still `ErrDigestVersion`.
2. **The legacy arm is the old rule byte for byte, and the goldens prove it.** v4's
   `digest-generated.golden` and `digest-external.golden` (297 veraPDF corpus files) are the files v4 shipped with,
   unregenerated; rule 4 must keep matching them. v5 has its own `-v5` goldens. The one change to v4's arm: it reads
   pdfcpu's join through `pdfread.PageContentAsPdfcpu`, which is byte-identical below `MaxPageContentBytes` and
   refuses past it — v4 hashed a decode bomb unbounded.
3. **Rule 5** joins page content at token boundaries (718); hashes a page reached from inside the walk as `#page`
   plus its position, never its dictionary (720); hashes each page's EFFECTIVE geometry and resources from pdfcpu's
   inherited attributes — the answer every nib page operation reads — plus page `/UserUnit` and `/AA` (578, 616);
   and hashes the catalog's `/AA`, `/OCProperties`, `/OpenAction` and the `/Names /JavaScript` tree, entry by entry
   and sorted (616). The remaining exclusions are written, with their reasons, in `ContentDigest`'s doc.
4. **The checker reads a divided page through `pdfread.PageContent`** (719), because that is veraPDF's join —
   measured, not assumed: on `["…EMC 0 0 1 1 re", "f"]` and `["…EMC % c", "0 0 1 1 re f"]` veraPDF fails 7.1 t3 at
   `content[1]/contentItem[0]`, exactly as on the single stream `"…EMC 0 0 1 1 re f"`, and passes the fused single
   stream `"…EMC 0 0 1 1 ref"`. pdfcpu's join had made both arrays a false pass
   (`TestTheCheckerJoinsADividedPageAsVeraPDFDoes`).

## Consequences

- A rule bump no longer halts a ceremony in flight: a v4 record is checked by v4 on a v5 build. The other direction
  — a v5 record reaching a v4 build — is still the skew sentence, which is correct: that build cannot compute rule 5.
- **A v4 ceremony keeps v4's holes until it ends.** A record written by the previous release is checked under the
  coverage its convener signed for; rule 5's new axes protect ceremonies convened on rule 5. A convener cannot be
  downgraded by anyone else, because the rule is inside the signed preimage.
- Each future bump retires the oldest readable rule. Keeping more than one prior rule is a choice to make then, with
  the in-flight population in view.
- The page-content census has two exemptions (the n-up carries, ADR-057); `ContentDigest` and the checker left it.
- **Not covered by rule 5**, and named rather than discovered later: the AcroForm `/XFA` stream, which some readers
  render instead of the pages.
