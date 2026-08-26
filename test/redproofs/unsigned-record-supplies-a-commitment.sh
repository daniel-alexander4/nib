# docs/red-proofs.md, tier 1: "an unsigned record supplies a commitment" (P07.S04, v1.117.169)
#
# The defect: `ProceedingOf` reads the record with `Extract` instead of `CheckRecord`, so a
# document carrying a record **nobody signed** still supplies a commitment — and signatures
# committing to it are reported as one proceeding. Nothing binds that roster to a convener, so the
# ✓ vouches for a ceremony anyone could have written into the document.
#
# **Found by mutation, not by review.** Neither of the two arms that existed went red against it:
# both compare against a DIFFERENT hash, so a commitment lifted from an unverified record still
# failed to match and the tests stayed green. The third arm — same roster, same id, record
# unsigned — is what reaches it.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAnUnsignedRecordIsNotAProceeding -count=1"
EXPECT="commits to a record NOBODY SIGNED"
