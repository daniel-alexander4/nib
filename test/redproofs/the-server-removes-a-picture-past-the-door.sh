# docs/red-proofs.md, tier 1: "The server removes a picture without the objedit door (PLAN-image-edit P01.S02, ADR-219)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test . -count=1 -run 'TestEveryPageObjectEditRoutesThroughTheDoor$'"
EXPECT="(surface server) calls pdfops.EditPageObjects directly"
