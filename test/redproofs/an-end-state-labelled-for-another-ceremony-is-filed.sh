# docs/red-proofs.md, tier 1: "the same defect at the receive path: a relabelled end state is accepted and written, write-once, into the other ceremony's folder"
# (/pending 686, v1.166.7)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnEndStateLabelledForAnotherCeremonyIsNotAccepted -count=1"
EXPECT="was accepted — its save writes it into"
