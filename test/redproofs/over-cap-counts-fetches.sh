# docs/red-proofs.md, tier 1: "DroppedOverCap counts fetches, not addresses" (/pending 250, v1.117.114)
#
# The defect: an over-cap candidate is dropped without being recorded in `seen`, so the next fetch
# re-decides and re-counts it. BEP-44 serves the same value to every fetch — ~10 across a 300 s race
# — so one honest peer offering a ninth endpoint inflates the counter ~10x and it reads as attack
# traffic. DroppedOverCap is the only counter that would ever witness item 20.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAnOverCapAddressIsCountedOncePerAddressNotOncePerFetch -count=1"
EXPECT="reporting the fetch cadence"
