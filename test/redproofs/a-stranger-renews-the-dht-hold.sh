# docs/red-proofs.md, tier 1: "A stranger's announcement renews the arm's DHT hold"
# (P07.S05e, v1.117.207)
#
# The defect: the sighting hook sits ABOVE `resolve(pins, seen)` rather than below it, so any
# host announcing on the link renews the hold — and a hold is a delay on another party's DHT
# fallback. Any machine on the link could then push a party's fallback out indefinitely by
# shouting, without holding any pin. L1 is what refuses it: the screen is pins the receiver
# already has, never wire bytes.
#
# A COUNT cannot see this — a stranger's sighting and the peer's produce the same one — which
# is why `answerLoop`'s own first version of this discrimination shipped green. The claim has
# to be about WHOSE, and the guard asserts the hold was never renewed at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDHTHoldRenewsOnEvidenceAndLapsesWithout"
EXPECT="a stranger's announcement renewed the DHT hold"
