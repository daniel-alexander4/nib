# docs/red-proofs.md, tier 1: "the consent card describes the wrong signature" (P07.S05a, v1.117.178)
#
# The defect: `coSignExchange` hands the Confirmer `ats[0]` — the party who signed FIRST — rather
# than `ats[len(ats)-1]`, the party whose contribution this hop is being asked to build on. At two
# signatures they differ, and at nine they differ badly: the user is asked to consent to a document
# described by a signature seven hops back.
#
# **It used to be protected sideways, and P07.S05 removed that protection.** The same `peer`
# variable fed the channel bindings, so a wrong index failed a binding — which is what the retired
# `channel-binding-reads-the-first-signer` row proved. S05 replaced those bindings with L3 inside a
# ceremony, so the index now decides only what the consent card says, and nothing checked it. The
# old row was retired rather than left standing over a claim it no longer proved; this is the row
# for the claim that replaced it.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheConsentGateIsGivenTheRightSignature -count=1"
EXPECT="want the LAST signer"
