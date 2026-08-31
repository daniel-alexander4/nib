# docs/red-proofs.md, tier 1: "the durable write runs after the acknowledgement" (P08.S05a, C10, v1.117.290)
#
# The defect as SHIPPED until P08.S05a: saveReceived ran from serveOneSession, AFTER
# ReceiveDocument had already written ackOK, best-effort and returning nothing. A party whose disk
# failed was recorded as delivered, never retried and never told — and saveReceived s own comment
# said the sender "will not send it again".
#
# The check asserts ROUTING (ADR-009): the write has one door and it is sessionAccepter.Accept,
# which is the last thing before the frame. A new call site elsewhere fails by name.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheReceivedWriteHasOneDoor -count=1"
EXPECT="calls saveReceived"
