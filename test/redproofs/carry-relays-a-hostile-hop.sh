# docs/red-proofs.md, tier 1: "the cross-binding rule stops firing" (P07.S05, v1.117.172)
#
# The defect: L3's cross-binding check is switched off, so a prefix whose identities are exactly
# right but whose evidence is not — a signature attesting to somebody who never signed — is
# admitted. L3 and D23 both say "each one valid **and cross-bound**", and an identity-only check
# satisfies the sentence while missing half of it.
#
# **The exemption inverted at this slice and the fixture inverted with it.** The rule was "all but
# the LAST", written while `AcceptedPeer` named the successor; under D22's amendment a signature
# accepts its PREDECESSOR, so it is "all but the FIRST". Caught by a four-party carry ceremony
# failing at hop 2 with *"signature 1 attests to a peer who is not a valid signer"* — the first
# signer, exempt under the new direction and not under the old.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheGateRefusesEachThingByName -count=1"
EXPECT="prefix not cross-bound: admitted"
