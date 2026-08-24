# docs/red-proofs.md, tier 1: "A body past the page renders without complaint" (P07.S08, v1.117.144)
#
# The defect: RenderReadme stops refusing. Measured before the guard existed — a 61-line body
# computes a last baseline of -189, and pdfcpu CLAMPS what it emits (a requested y of -50 and
# of -5000 both land at 421.0, A4s centre), so the surplus lines stack on ONE baseline as an
# illegible smear while err is nil and PageCount stays 1.
#
# This is why the guard reads the COMPUTED baseline: the rendered position saturates, so forty
# overflow lines and four hundred are indistinguishable from it.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestRenderReadmeRefusesAnOverflowingBody -count=1"
EXPECT="want ErrReadmeOverflow"
