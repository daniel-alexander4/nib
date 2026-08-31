# docs/red-proofs.md, tier 6: "a transfer the sender believes completed leaves nothing on the
# receiver s disk" (P08.S05a, C10, v1.117.290)
#
# The transfer route had NO harness coverage above tier 1 before this slice: /api/session/send and
# the armed mode:"receive" path were driven only inside one process, sharing one vault and one
# identity. So "the document reaches the other machine and lands on its disk" was asserted by
# nothing that crosses a process boundary.
#
# The check makes the receiver s durable write fail and requires the SENDER to learn it, over both
# transports, through two real binaries.
TIER="tier 6 — ceremonyrepro.sh"
PROVE="./build/ceremonyrepro.sh"
EXPECT="the transfer did not complete"
