# docs/red-proofs.md, tier 1: "a failed write is still acknowledged" (P08.S05a, C10, v1.117.290)
#
# The defect: Accept swallows saveReceived s error, so ReceiveDocument writes ackOK and the sender
# is told the document arrived. This is the behavioural half of the ordering row above — the write
# can be in the right place and still not be believed.
#
# The failure is injected by making ~/nib a FILE, which makes MkdirAll refuse deterministically on
# every platform and as any user; a chmod fixture is a no-op for root and on Windows.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAFailedReceivedWriteIsNotAnAcknowledgement -count=1"
EXPECT="reported the document accepted while its durable write had failed"
