# docs/red-proofs.md, tier 1: "the consent gate never reconciles the document" (P07.S02b, v1.117.157)
#
# The defect: `sessionConfirmer.Confirm` loses its `checkArrival` call, so a received document is
# put in front of the user without ever being reconciled against the invitation the arm was built
# from. C17's whole clause is the ORDER: by the time an unreconciled document has been read and
# accepted, the party has signed it.
#
# **Guarded on the ROUTING, per ADR-009**, with `//` comments stripped — a scan satisfied by prose
# that merely names the call is how `handleSave`'s freeze guard read its own explanation as proof
# of coverage (v1.117.155). The guard also asserts the gate runs BEFORE `setPending`, and that a
# decline prunes while a consent TIMEOUT does not.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheConsentGateRoutesThroughTheArrivalCheck -count=1"
EXPECT="Confirm does not contain \"checkArrival(\""
