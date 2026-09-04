# docs/red-proofs.md, tier 1: "the listing names who convened it" (/pending 353, v1.117.351)
#
# The defect: `ReadStored` sets `Convener` from `Me` instead of reading it off the record through
# `Record.Convener`.
#
# It passes on every ceremony this machine convened — the two ARE the same party there — and fails
# on every mirrored one, which is precisely the population the panel needs it for. The test's
# second arm is a ceremony this machine did not convene, and it is the only thing that tells a
# field read from the record apart from one echoed off the marker.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheListingNamesWhoConvenedIt"
EXPECT="did not convene"
