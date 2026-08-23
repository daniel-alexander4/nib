# docs/red-proofs.md, tier 1: "a router that answered and refused is told it stayed silent"
# (/pending 263, v1.117.133)
#
# The defect: cause 3 had no branch for a gateway that ANSWERED and refused, so those users got the
# learned-nothing sentence — "Nib couldn't get an answer from your router… enabling UPnP may help".
# Every clause is false: the router answered by code, and UPnP is not merely on but talking. The advice
# also inverts, which is what makes it worth a branch rather than a hedge — a refusal proves the router
# is the user's and reachable, so a manual port-forward is the thing that works.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARefusedGatewayIsNotToldItStayedSilent -count=1"
EXPECT="is told Nib got no answer from it"
