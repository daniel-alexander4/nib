# ADR-179 — the digest leaves a font's subset tag out, and rule 7 stays computable

**Status:** accepted
**Date:** 2026-10-30
**Extends:** ADR-174 (the digest is taken over pdfcpu's optimized graph), ADR-143 (a rule is bound into the digest), ADR-175.
**Context:** `/pending 927`. `ContentDigest` differed between reads of the same bytes for 3 of 2,013 readable real documents
(and 5 of 2,022 in the F11 comparison). pdfcpu's optimize pass registers the first font it meets under a name and rewrites
a later one that `EqualObjects` calls equal to point at it (`optimize.go:407`, a `range` over a map); `equalFontNames`
(`model/equal.go:237`) ignores everything up to the first `+` in `BaseFont`, `FontName` and `Name` of two `/Type /Font`
dictionaries. The survivor's `ABCDEF+` tag is what rule 7 hashed, so it varied with map order: 10 distinct digests in 12
reads of one document, 1 with validation alone. A ceremony recomputes the digest on every party over the same bytes.
Measured on the corpus (counts only): validated-only read 0 unstable / 422 digests move; strip every font-name tag 0 / 1,444;
strip colliding names only 0 / 206.

**Decision.**

1. **Rule 8.** In a dictionary whose `/Type` is `/Font`, the values of `/BaseFont`, `/FontName` and `/Name` are hashed with
   everything up to the first `+` (not at index 0) removed. A `/FontDescriptor` keeps its tag: pdfcpu compares it strictly.
   `ContentDigestVersion` is 8. Nothing else in the walk moves.
2. **Rule 7 stays computable, byte for byte,** for the records written under it (`ContentDigestRuleReadable` is 7 and 8;
   `ContentDigestRuleRetired` stays 5 and 6). No ceremony is stranded by the move, so ADR-175's popup and boundary are not
   extended. A rule-7 record over a document whose fonts collide by tag still matches only some of the time: unchanged, and
   declared here. New ceremonies are written under rule 8.
3. **The simplest rule won over the one that moves fewer digests,** because with rule 7 kept the number of digests that move
   strands nothing, and a rule that strips only colliding names would need a pre-optimize collision pass and a hook in
   `readOptimized`. No pdfcpu setting skips only the font merge.

**Declared.** The measured population is one local corpus. Images, forms and content streams compare strictly in `equal.go`,
so their survivors differ only in object numbers, which are never hashed (read, not measured). `opsunreach` moved on one
document whose digest did not; its cause is not established.
