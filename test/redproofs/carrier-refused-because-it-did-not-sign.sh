# docs/red-proofs.md, tier 1: "the receiver refuses a carrier because it did not sign" (P07.S05, v1.117.172)
#
# The defect: `coSignExchange`'s channel binding goes back to comparing the document's last signer
# against the TLS-pinned **wire peer** — the state the product was in, and correct only while every
# carrier also signs.
#
# Measured at the grill: a three-party roster with a `signs:false` convener, A having signed, the
# convener carrying to B — `coSignExchange` answered *"the document was not signed by the connected
# peer"* while `AdmitContribution` answered `<nil>` and named B as the next contributor. The two
# checks disagreed about the same hop.
#
# What replaces it is not a relaxation: **L3 subsumes both old checks**, against the record this
# party verified at ARM time rather than against a claim in the document. What L3 does not say is
# that the party on the SOCKET belongs to this proceeding, so `InRoster` is asked and nothing else.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAFourPartyCeremonyCompletesOverTheCarryRoute -count=1"
EXPECT="the signer refused the carrier"
