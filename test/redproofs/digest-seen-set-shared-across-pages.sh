# docs/red-proofs.md, tier 1: "The #again set scoped per document instead of per call" (/pending 488)
#
# The defect: `ContentDigest` builds ONE `seen` map and hands it to every page's `hashObject`
# instead of letting each top-level call start fresh.
#
# It is the exact mistake /pending 488 warns about — *"the `#again` marker semantics are per-call
# and must be preserved exactly, or the fix is a `ContentDigestVersion` bump"* — and it is the
# tempting one, because it is faster again and every structural property of the digest survives it.
# `hashObject`'s own doc comment records that this was measured WRONG once already: per-document
# scope "changes the digest of ordinary documents, measured on two controls immediately".
#
# What makes it worth a row is that it is silent. The digest is still deterministic, still
# injective over the fixtures, still stable across a pdfcpu rewrite — it is simply a DIFFERENT
# number, which by ADR-013 is reported to a counterparty as "these are not the same document".
# Only a stored output from before the change can see it, which is what the golden corpus is.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestContentDigestIsByteIdenticalAcrossTheExternalCorpus -count=1"
EXPECT="ADR-013: the digest's OUTPUT is a commitment"
