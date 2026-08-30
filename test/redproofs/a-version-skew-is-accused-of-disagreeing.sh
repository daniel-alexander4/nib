# docs/red-proofs.md, tier 1: "a version skew is accused of disagreeing" (/pending 324, v1.117.278)
#
# **The anti-proof, and it guards against the FIX rather than the defect.** Dropping `skew == ""`
# (and `claimed > 0`) from `disagrees()` is the naive form of defect 2's fix, and it was measured
# exiting 2 on a document whose counterparty had merely updated Nib — breaking
# `nib verify x.pdf && echo intact`, the README's own idiom, over something no party did wrong.
# The CLI never had the web's two D32 discriminators; it does now.
#
# It also reddens the zero-signature arm: a convened, unsigned document has `oneProc == false`
# because nothing claims the ceremony, which is not a disagreement.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestTheCeremonyVerdictRefusesWhatItUsedToCallComplete -count=1"
EXPECT="was reported as a disagreement"
