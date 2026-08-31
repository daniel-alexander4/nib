# docs/red-proofs.md, tier 1: "a failed write is reported as a transport failure" (P08.S05a, C10, v1.117.290)
#
# The defect: the send route drops the ErrNotStored arm, so the outcome falls through to
# httpError and the browser toasts "could not send: …" — the sentence a DEAD PEER produces. The
# transport worked, the peer is fine, a human said yes, and the action this calls for (ask them to
# arm again and resend) is not the action an unreachable peer calls for.
#
# It is the MIRROR of the defect ackNotStored was added to fix: that one collapsed a disk failure
# into a decline, this one collapses it into a connection fault.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestSendReportsNotStoredAsItsOwnOutcome -count=1"
EXPECT="want notStored"
