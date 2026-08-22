# docs/red-proofs.md, tier 1: "A co-signature decline writing nothing to the wire"
# (P05 sweep, v1.117.38)
#
# The defect: `Receive` returned on a decline without writing anything, and the server then
# closed the connection. The initiator's `readFrame` got EOF and answered its user
# `502 co-signing did not complete: receive co-signed document: EOF`.
#
# What it costs: a refusal is reported as a network fault, which invites the retry a refusal
# must not invite — asking the same person again. The transfer path has had an explicit
# declined byte since it was written, and its own doc says why, so the two halves of one
# feature disagreed about what a refusal is.
#
# Note the guard: `TestSessionReceiverDeclines` used to assert only `err != nil`, and passed
# against this defect for its whole life. It asserts WHICH error now.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run 'TestSessionReceiverDeclines|TestARefusalTellsThePeerWHICHRefusalItWas'"
EXPECT="want ErrCoSignDeclined"
