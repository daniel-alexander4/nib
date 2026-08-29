# docs/red-proofs.md, tier 1: "a ceremony that cannot be read is dropped from the listing" (P08.S03, v1.117.243)
#
# The defect: `ListStored` keeps only the entries that loaded. A damaged, absent or version-skewed
# ceremony then disappears from the list entirely rather than appearing as a degraded row — so the
# user's folder holds a ceremony their Nib will not admit exists, and the only remedy is to find and
# delete it by hand. That is what C12 and D34's self-healing rule forbid in as many words.
#
# The guard is written on the COUNT as well as the state, because a test that only checked the
# surviving entry's class would pass against a listing that silently dropped the other one.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestOneUnloadableCeremonyDoesNotCostTheOthers -count=1"
EXPECT="the listing dropped an entry"
