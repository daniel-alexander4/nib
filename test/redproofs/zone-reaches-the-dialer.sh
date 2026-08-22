# docs/red-proofs.md, tier 1: "A peer-chosen zone reaches the dialer" (sweep 10, v1.117.44)
#
# The defect: `dialable` does not refuse a zone, so a zoned GLOBAL v6 address passes both
# candidate doors. `Routable` strips the zone for the DECISION — that is what makes the
# reserved table work — but the address keeps it: parseCandidate stores what ParseAddrPort
# returned, the canonical re-encode re-emits it, and ceremonynet hands `Endpoint.Addr.String()`
# to the racer, which dials it verbatim. Measured: on Linux the kernel ignores sin6_scope_id
# for a global destination, so this is attacker-chosen bytes handed to a syscall that can
# never act on them — plus a free axis for multiplying spellings past maxRaceCandidates.
#
# Distinct from `zone-bypasses-reserved`, which is about the reserved TABLE and a PRIVATE
# address wearing a zone. This row is about a genuinely global address, which that table has
# nothing to say about.
TIER="tier 1 — go test"
PROVE="go test ./internal/addrscope/ -run TestAZoneOnAGlobalAddressNeverReachesTheDialer"
EXPECT="Target accepts"
