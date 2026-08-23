# docs/red-proofs.md, tier 1: "a gateway refusal flattened into 'nothing answered'" (/pending 263,
# v1.117.133)
#
# The defect: all three mechanisms wrap ErrResultCode, and all three ended as a bare ErrNoMapping — so
# "the router refused" and "no router answered" became the same value before the diagnosis ever saw
# them. This patch reverts the INNER carry, in tryGatewayProtocols, which is where the first draft of
# the fix missed it: carrying the refusal out of mapWithSuggestion alone leaves it flattened one level
# down, and the outer check then sees ErrNoMapping forever. The test caught that.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestARefusalIsCarriedOutNotFlattened -count=1"
EXPECT="indistinguishable from no gateway at all"
