# docs/red-proofs.md, tier 1: "a borrowed endpoint is torn down by its borrower" (/pending 376, v1.128.28)
#
# The defect: `ceremonyID.close()` ignores `borrowedEndpoint` and closes the round's shared socket
# and DHT server when the FIRST leg finishes. Every leg after it dials a closed socket.
#
# **This is the one that panics**, per close()'s own doc: three of the six plausible orderings kill
# the process on a goroutine nothing can recover, and an endpoint with two closers is that ordering
# waiting for a schedule.
#
# **The first version of the reader could not see it.** It compared `LocalAddr()` before and after —
# and a closed socket still reports the address it was bound to, so the assertion was vacuous and
# this mutation SURVIVED it. The observable has to be a write.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestABorrowedEndpointOutlivesTheLegThatUsedIt -count=1"
EXPECT="will not send"
