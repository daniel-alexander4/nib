# docs/red-proofs.md, tier 2: "the watcher stops when the round does" (/pending 370, v1.117.354)
#
# The defect: the poll's stop is not called from the round's `finally`.
#
# A watcher that outlives its round keeps asking about a ceremony nobody is delivering, on a 1500 ms
# timer, forever — and it survives the error paths as well as the happy one, which is why the stop
# is in a `finally` rather than after the success branch.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonydeliver.test.mjs"
EXPECT="kept polling after the round returned"
