# ADR-143 — rule 6 follows every kind of resource a page binds, and a digest-rule skew says which build is behind

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-080 (a record is checked by the digest rule it names) with its first bump under that policy, and
ADR-013 (`DocHash` is a hop-1 anchor). Supersedes ADR-080's readable set only: the rules this build computes are
5 and 6, and rule 4 is retired.
**Context:** /pending 691 parts R5-5 and R5-6 (the P07 phase-close review, widened by the P03 one), decided by
/discuss on 2026-10-12: one `ContentDigestVersion` bump, and a ceremony begun on a retired rule gets a plain
"started with an older Nib" sentence, never a tampering report.
**Applies:** `internal/pdfops` (`ContentDigest`), `internal/ceremony` (`digestRuleSkew`), `internal/p2p` (two
refusal codes), `internal/server` (`GET /api/document/ceremony-copy`), `web/app.js` (that route's causes).

## Measured

**The gap.** Rules 1 to 5 followed a page's resources through a list — `/Font` and `/XObject` — under a comment
saying every other kind that changes what is drawn "does so through a stream these two already cover". Six pairs of
documents, each differing in ONE resource with the page's content stream byte-identical, digest alike under rule 5:

- an `/ExtGState` whose `/ca` goes 1 → 0 (the text is painted fully transparent; no stream is involved at all);
- the form under an `/ExtGState`'s `/SMask /G`;
- a tiling `/Pattern`'s stream;
- an Indexed `/ColorSpace`'s palette;
- a `/Shading`'s colours;
- a `/Properties` membership dictionary (`/OCMD /P /AnyOn` → `/AllOff`) that turns marked content off.

The window is the one ADR-013 names: before the first signature, where `DocHash` is the only anchor.

**The cost**, because the digest runs at the arrival gate on a request path (/pending 359). Rule 5 against rule 6 in
one process, the rules alternated inside each repetition, a fresh read each time, best of N. "Call" is the read plus
the walk — what a caller of `ContentDigestAt` waits for; "walk" is `digestRule` alone. Per document: size, pages;
the bytes the RESOURCE walk feeds the hash under rule 6 against rule 5 and the streams decoded (both deterministic);
then the call in thread CPU time, best of 31.

- `irs-f1040.pdf` (Designer) 220 KB, 2 pages: bytes +0.21%, decodes 12 → 12; call −1.7%
- `cdc-mm7201-H.pdf` (InDesign) 532 KB, 20 pages: bytes +5.61%, decodes 50 → 54; call −0.2%
- `fda-82395.pdf` (Word) 864 KB, 42 pages: bytes +0.05%, decodes 33 → 33; call +1.3%
- `irs-p15.pdf` (Antenna House) 1.7 MB, 59 pages: bytes +0.04%, decodes 35 → 35; call +1.9%
- `census-p60-280.pdf` (InDesign) 2.7 MB, 67 pages: bytes +9.46%, decodes 81 → 93; call +1.3%
- `MPN-flattened.pdf` (a scan) 5.0 MB, 14 pages: bytes +0.00%, decodes 14 → 14; call +1.0%
- `fda-170357.pdf` (Acrobat) 6.1 MB, 45 pages: bytes +0.06%, decodes 319 → 319; call −4.7%

Wall time of the call, best of 41 with the machine at load 4, on the three documents where rule 6 does the most
extra work: −2.7%, +2.2%, +0.9%.

**The walk alone could not be timed to better than the machine's noise.** Other work ran on the machine throughout
(load 4 to 17). On `census-p60-280.pdf`, the one document where rule 6 decodes more, four runs gave the walk −3%,
+8%, +14% and +29% — of 29 to 79 ms inside a call of 600 to 1,500 ms — while documents whose two rules do the SAME
work to within 0.1% read anywhere from −12% to +10%. The deterministic figures above are the bound to rely on. The
stop rule set before building was 15% on the digest's wall time on any document; the call is under it everywhere.

## Decision

1. **Rule 6 follows EVERY kind the page's effective resource dictionary holds** (`hashEveryResource`): kinds taken
   from the dictionary and sorted, each dictionary kind hashed name by name, sorted, each name its own top-level
   walk; a kind that is not a dictionary (`/ProcSet`) hashed as the object it is. No list of kinds exists for rule
   6 to fall behind. Nothing else differs from rule 5.
2. **Rule 5 is the legacy arm, byte for byte.** Its goldens (`digest-generated-v5.golden`,
   `digest-external-v5.golden`) are the files it shipped with, unregenerated; rule 6 has `-v6` files. Each of the
   six rows asserts both halves: rule 5 still digests the pair alike, rule 6 tells it apart.
3. **Rule 4 is retired**, by ADR-080's policy that a bump retires the oldest readable rule. Its arm, its goldens
   and `pdfread.PageContentAsPdfcpu` (its one caller's door to pdfcpu's bare join) are removed.
4. **A digest-rule skew says which build is behind** (`ceremony.digestSkew`). A record under a rule this build has
   RETIRED: "this ceremony was started with an older Nib … finish the ceremony on the Nib it was started with, or
   convene it again". A record under a rule this build does not compute YET: "… started with a newer Nib … update
   Nib to take part in it". Both end "a version difference, not a changed document". Both are
   `ceremony.ErrDigestVersion` to `errors.Is`.
5. **The skew crosses the wire by name.** Refusal codes 18 (`ErrCeremonyRuleRetired`) and 19
   (`ErrCeremonyRuleTooNew`), appended; the skew error IS the p2p sentinel for its direction, so the arrival gate's
   refusal needs no mapping at the site. Two codes because the remedies are opposite and one byte cannot say which
   side is behind.
6. **The ceremony-copy route names it**: 422 causes `older-nib` and `newer-nib`, where it answered `unreadable`.

## Consequences

- **A rule-5 ceremony keeps rule 5's holes until it ends**, as ADR-080 said of rule 4. Rule 6 protects ceremonies
  convened on rule 6.
- **A ceremony convened under rule 4 and still in flight is halted on this build**, with the older-Nib sentence.
  That is the retirement, and it is the owner's decision (2026-10-12).
- **Across one release, in both directions:** a rule-5 record on this build is checked by rule 5. A rule-6 record
  reaching the previous release is a skew there, and that build has no code for it — its dialler still reads a bare
  EOF. Codes 18 and 19 help from this build on; a build that does not know them reads `ErrRefusedUnknown`.
- **"Every kind" is, in practice, ISO 32000-1 Table 33's eight.** The digest reads a validated context and pdfcpu's
  validator deletes any other key from a resource dictionary before the walk sees it — the validator's reading, as
  ADR-129 says of a font it empties (which still hashes as `#nil`, under rule 6 as under rule 5). No content operator
  can name such a key. Pinned by `TestRuleSixTakesItsKindsFromTheDictionary`.
- Rule 6 holds across a rewrite: the no-op page write over the generated and real-producer corpora
  (`TestANoOpWalkKeepsTheDigest`) and a pdfcpu write and `Optimize` of a document using all six kinds
  (`TestRuleSixHoldsAcrossARewrite`).
- **Not covered, as before:** the AcroForm `/XFA` stream (ADR-080).
- **Declared gap in the tests:** rule 5's rows (`digestrule5_test.go`) were two-sided against rule 4 — rule 4 had to
  stay blind to each. With rule 4 gone that half cannot be asserted; the rows now hold under rules 5 and 6.
