# docs/red-proofs.md, tier 1: "A deleted tag's element kid still names the deleted tag as its /P (ADR-124)"
#
# The defect: the kid is listed under the new parent and its `/P` is not re-pointed, so the tree reads one way down and another way up.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestADeletedElementsKidsTakeItsPlace"
EXPECT="the re-homed paragraph's /P is"
