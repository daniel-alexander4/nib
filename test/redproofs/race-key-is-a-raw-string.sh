# docs/red-proofs.md, tier 1: "One endpoint, several race candidates" (sweep 10, v1.117.44)
#
# The defect: `raceKey` returns the raw `Addr` string, so IPv6's several spellings of one
# endpoint are several keys. `[2001:db8::1]:443`, `[2001:DB8::1]:443` and
# `[2001:db8:0:0:0:0:0:1]:443` are the same host and were three candidates — three of
# maxRaceCandidates and three of one source's allowance, on a budget whose entire purpose is
# to bound what a single source can spend. This is the only place candidates are compared.
#
# The check's two setup rows are what make it a proof rather than a coincidence: two distinct
# endpoints and two link-locals differing only by zone must BOTH still be two, so a key that
# collapses everything cannot pass in place of one that normalises.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestOneIPv6EndpointIsOneRaceCandidateHoweverItIsSpelled"
EXPECT="spelled three ways was tried"
