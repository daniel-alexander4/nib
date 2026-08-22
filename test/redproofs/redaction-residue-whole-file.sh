# docs/red-proofs.md, tier 1: "The residue check that could not see the whole file" (sweep 11, v1.117.48)
#
# The defect: the whole-file residue scanner resumes at `start+k`, the offset of `endstream` —
# which CONTAINS `stream` — so after the first object every real payload lands inside a
# misaligned span that fails to inflate and is silently skipped. The whole-file check then
# PASSES for the wrong reason: the fixture's secret is in the first stream (control finds it)
# and everything after is invisible (redacted file scans clean). A vacuous green caught by the
# discriminating test that asserts the page-content and whole-file checks genuinely differ.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheTwoResidueChecksDifferAndTheDifferenceIsThePoint"
EXPECT="did NOT find a flate stream"
