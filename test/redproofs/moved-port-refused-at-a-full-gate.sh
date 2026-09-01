# docs/red-proofs.md, tier 1: "a moved port is refused at a full gate" (/pending 20, v1.117.312)
#
# The replacement branch is disabled, restoring the shipped state. D15 refreshes a port mapping
# while armed and `portMapper` asks for the SAME external port — a router may hand back a different
# one. Each republish re-reads the mapper's current endpoints, so the moved address DOES arrive; it
# arrives at a full gate whose `addrs` is append-only, and there is no room.
#
# What it costs is the whole connection: the race spends the rest of its budget dialling an endpoint
# that has expired, and reports D19 cause 4 — "couldn't connect" — for a peer that was reachable
# throughout. The item carried that consequence for four sweeps.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAMovedPortReplacesItsHostRatherThanBeingRefused -count=1"
EXPECT="refused at a full gate"
