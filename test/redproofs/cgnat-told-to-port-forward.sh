# docs/red-proofs.md, tier 1: "A CGNAT/double-NAT user is told to port-forward" (P05.S11 T02, v1.117.98)
#
# The defect: D19 cause 3's advice offers "enable UPnP / a port-forward on your router" unconditionally,
# instead of only when the user CONTROLS a NAT. D9's whole pin is that this is exactly the wrong advice
# for a carrier-grade-NAT subscriber (no router to open a port on) or a double-NAT (the router answered
# but its own external is private) — the futile instruction that makes the ladder's diagnosis worse than
# a generic timeout. The correct map is: carrier space (SharedAddressSpace) OR answered-unroutable
# (mapUnroutable) ⇒ VPN only; no answer AND not carrier space ⇒ port-forward may work. The patch removes
# the split, and the cause-3 CGNAT/double-NAT rows get the forbidden advice.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestD19ClassifierTable -count=1"
EXPECT="D9 forbids offering a port-forward"
