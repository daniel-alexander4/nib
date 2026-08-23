# docs/red-proofs.md, tier 1: "the PCP delete names a mapping that never existed" (/pending 257,
# v1.117.120)
#
# The defect: PCP identifies a mapping by (nonce, protocol, internal port), and Unmap minted a FRESH
# nonce — so every PCP delete was a silent no-op, on the SUCCESS path as much as the error path. Nothing
# could see it: the mock echoes the nonce and validates nothing, and the one delete test drives NAT-PMP,
# which has no nonce at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestThePCPDeleteCarriesTheMappingsOwnNonce -count=1"
EXPECT="names one that never existed"
