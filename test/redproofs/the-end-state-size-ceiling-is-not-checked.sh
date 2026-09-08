# docs/red-proofs.md, tier 1: "the end-state size ceiling is not checked" (/pending 380, v1.128.27)
#
# The defect: `Termination.Seal` stops enforcing `MaxSealedRecord`. The failure it prevents is
# SILENT — `MaxSealedRecord`'s own doc says an over-size value is refused by our own store inside
# `dht.Server.Put` before any datagram is sent, `getput.Put` logs a warning per node and returns
# nil, so the record never leaves the machine and nothing says so.
#
# **Measured, and the margin is a function of a NAME.** A sealed end state is 875 bytes against a
# 996 cap for the 8-character common name production mints (`GenerateIdentity("Nib User")`), and
# 1219 bytes — over — for a 128-character one. **Recorded because it SURVIVED the first pass**: the
# fixture test asserted the honest object fits and only LOGGED the long-name case.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAnOversizeEndStateIsRefusedAtTheSeal -count=1"
EXPECT="far over the cap was accepted"
