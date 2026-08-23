# docs/red-proofs.md, tier 1: "a misaddressed mutation reaches whatever is active" (/pending 15's
# behavioural suite, v1.117.124)
#
# The defect: docFor falls back to the active document when the addressed one is unknown — ADR-001's
# whole failure mode, where document A's bytes land in document B's file past the signature guard with a
# "Saved" toast and no error anywhere. The jsdom pinning guard is a SOURCE scan and cannot see this; it
# proves the client sends an id, not that the server refuses a stale one. Eleven routes, driven.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryMutatingRouteRefusesAMisaddressedDocument -count=1"
EXPECT="want 409"
