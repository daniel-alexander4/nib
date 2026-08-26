# docs/red-proofs.md, tier 1: "L3 stops comparing the prefix against the roster" (P07.S03, v1.117.159)
#
# The defect: `NextContributor` no longer checks that signature i was made by the roster's i-th
# signer. The signing ORDER is then unenforced — which is the whole of D23 — and a document whose
# blocks arrived in any order at all is admitted.
#
# **The row's assertion is the SENTENCE, not merely the refusal, and that is the finding.** With
# the identity check off the gate still refuses — it counts the signatures and computes a
# different "next" — so a test that only asked "was it refused?" would stay green. What it says is
# *"it is not this party's turn"*, which sends the user to wait for a turn that will never come,
# when the truth is that the document's signatures are not this ceremony's order at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheGateRefusesEachThingByName -count=1"
EXPECT="want the signatures on this document are not the ceremony's signing order"
