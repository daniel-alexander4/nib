# docs/red-proofs.md, tier 1: "an armed listener presents an expired transport leaf" (/pending 662, v1.169.11)
#
# The defect: leafCache never re-mints once it holds a leaf, so a listener armed longer than transportTTL
# presents an expired leaf and every peer dialling in is refused with a sentence blaming its clock.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run '^TestAHeldSessionConfigPresentsAFreshLeaf$' -count=1"
EXPECT="a config held past transportTTL presented a leaf its peer refuses"
