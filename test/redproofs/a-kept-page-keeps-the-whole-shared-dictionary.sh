# docs/red-proofs.md, tier 1: "a kept page carries only what its drawing names" (/pending 688)
#
# The defect: `selectPages` hands every kept page the WHOLE inherited — or shared — `/Resources`, and
# pdfcpu writes by reachability, so the form and the image only the redacted page drew are written
# into the redacted file through a page that never draws them. Measured by the P07 phase-close review
# (R5-1): the raster replaced page 1 and page 1's form text was still in the file.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestRedactionLeavesNothingOnlyTheRedactedPageDrew -count=1"
EXPECT="still carries the form only page 1 drew"
