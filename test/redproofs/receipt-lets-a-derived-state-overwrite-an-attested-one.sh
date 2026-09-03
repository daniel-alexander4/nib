# docs/red-proofs.md, tier 1: "a re-sweep overwrites a signed end state with a guessed one"
# (P08.S06, D28, v1.117.330)
#
# The receipt's four states are not equal in standing. `declined` and `completed` come from a
# termination somebody signed; `expired` and `abandoned` are conclusions this machine drew from a
# clock, and `closeOutReason` returns `abandoned` past the grace for anything it cannot find an end
# state for. The sweep reaches a ceremony more than once — an interrupted close-out, a second
# unlock — so without write-once, *"they declined on the 2nd"* is replaced by *"nothing ever said"*,
# the better answer destroyed by the worse with no trace that it had been there.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheReceiptKeepsTheFirstThingThisMachineObserved -count=1"
EXPECT="overwrote an attested"
