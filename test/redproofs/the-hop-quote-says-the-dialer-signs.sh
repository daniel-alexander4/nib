# docs/red-proofs.md, tier 1: "the hop quote says the dialer signs" (/pending 517)
#
# The defect in the shape it had until /pending 517: `handleCeremonyHopQuote` re-derived
# `Contributes` from a SECOND `ContributionProgress` walk after the `mine` branch returned —
# `out.Contributes = i == pr.Done` over this machine's own place in the order — and then minted an
# attestation and returned its `lines`, `rect` and `when`.
#
# The mint could never run. `mine` is false past that point by construction and `mine` IS
# `me == Order[Done]`, so the first `i` matching this machine is never `pr.Done`: the loop could
# only ever write the `false` the field already held, and the `if !out.Contributes` below it
# returned every time. Dead code in a signing path, and it read as a guarantee — a route that could
# quote a block for a machine that signs at a hop it is dialling, when no such machine exists.
#
# `mine && next.Signs` is the whole answer, and this patch drops the `mine &&` to put a true
# `contributes` back on a dialled hop — which is what the deleted branch would have acted on.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestADialledHopQuotesNoBlockForThisMachine' -count=1"
EXPECT="says this machine SIGNS at a hop it is dialling"
