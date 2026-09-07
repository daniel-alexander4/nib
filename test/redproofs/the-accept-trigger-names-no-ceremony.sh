# docs/red-proofs.md, tier 1: "the accept trigger names no ceremony" (P02 close, v1.128.16)
#
# The defect: the accept trigger stops passing the id it just accepted, so the sweep falls back to
# listing order.
#
# **A source scan, and it is one because a probe proved nothing else can see it.** With ONE ceremony
# the preference and the listing order pick the same one; with two, the slot is already held by the
# first accept's sweep, so the second's preference cannot decide anything — and making it decide is
# the racing displacement that was tried and backed out. The scan proves the id is passed; the row
# above proves the sweep honours one when it gets one. Neither proves the pair matters on a machine
# holding two live ceremonies, because nothing can.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheAcceptTriggerNamesWhatItAccepted -count=1"
EXPECT="no longer names the ceremony it just accepted"
