# docs/red-proofs.md, tier 1: "A refused peer record reads as 'hasn't started'" (P05 close, v1.117.106)
#
# The defect: classifyD19 cause 1 ("hasn't started their ceremony yet") was keyed solely on
# !peerSeen, and peerSeen is set only when the gate ADMITS a candidate. A peer who published but
# whose record the gate refused (stale/forged/wrong-ceremony) or that carried no address collapsed
# into cause 1 — a confident-false statement, this repo's worst class.
#
# What it costs: a user whose peer HAS started is told to ask them to open the ceremony, sending
# troubleshooting at the wrong party. Removing the causePeerRecordUnusable branch reintroduces it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestD19ClassifierTable"
EXPECT="record refused -> unusable, not 'not started': cause"
