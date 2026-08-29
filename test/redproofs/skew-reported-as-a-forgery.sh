# docs/red-proofs.md, tier 1: "a version-skewed ceremony is reported as a forgery" (P08.S03, v1.117.243)
#
# The defect: `ReadStored` drops the `ErrVersion` branch, so a record written by a NEWER Nib falls
# through to `LoadUnverifiable` and the user is told their ceremony "does not verify" — the
# vocabulary of forgery, for a file that is perfectly intact and belongs to them.
#
# It is not hypothetical. `Record.Verify` checks the version FIRST, so every skewed record also
# fails the checks below it; `FormatVersion` has already moved three times mid-project, and Nib
# self-updates. Without the branch, updating Nib mid-ceremony reports every live ceremony on the
# machine as unverifiable — and, because the prune must first establish that a ceremony ended, also
# leaves them permanently unremovable.
#
# The guard asserts the CLASS and the SENTENCE, because the class alone would pass with a message
# that still accuses somebody.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAVersionSkewIsNotReportedAsDamageOrForgery -count=1"
EXPECT="want version-skew"
