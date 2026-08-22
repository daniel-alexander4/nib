# docs/red-proofs.md, tier 1: "A dial-won ceremony receive cannot consent" (P05.S09 T05/C4, v1.117.67)
#
# The defect: the consent gate's stale-goroutine guard keys ONLY on the armed listener
# (se.ln == ln). A symmetric-racing hop whose RECEIVE role wins by DIALING holds no listener, so
# its ceremony-anchored consent can never park and the exchange hangs at the consent gate — the
# grill's biggest hole (C4). The patch drops consentAnchor.current's ceremony branch, restoring the
# listener-only guard, and a ceremony-anchored consent can no longer park while its ceremony is
# armed.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestACeremonyHopConsentAnchorsOnTheCeremonyNotAListener -count=1"
EXPECT="dial-won receive role would hang here"
