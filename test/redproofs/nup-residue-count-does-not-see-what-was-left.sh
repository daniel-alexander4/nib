# docs/red-proofs.md, tier 1: "The carry counts what it left behind" (ADR-045, /pending 562)
#
# The defect: an annotation this door does not carry is skipped without being counted.
#
# ADR-045's declared gap is that anything which is not a `/Text` is still dropped and still without
# a sentence, and the door's answer is to COUNT it — so the residue is a number somebody can ask
# for rather than a silence. A count that does not see the residue is the same silence one level in.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheNoteCarryCountsWhatItLeftBehind -count=1"
EXPECT="want 0 and 2"
