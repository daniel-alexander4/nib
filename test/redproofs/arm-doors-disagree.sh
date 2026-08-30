# docs/red-proofs.md, tier 1: "the two arm doors disagree" (/pending 311, v1.117.259)
#
# The defect: `arm()` asks only `se.ln != nil`. A connect-based ceremony arm sets `se.cer` and
# leaves `se.ln` nil, so a following manual/TCP arm sees no listener and succeeds — overwriting
# `se.cer` with no `close()` on the ceremony it displaced, orphaning that ceremony's rendezvous
# server, shared UDP socket, router port-mapping lease (whose refresh goroutine only `close()`
# stops), in-memory document and invitation secret.
#
# TWO checks fire and they are different halves. The behavioural one catches the orphaning. The
# structural one catches it as an ADR-009 violation — `arm` deciding armedness without routing
# through `armedLocked()` — and it is the half that would catch a FOURTH site added later, which
# is what comparing three copies for agreement cannot do.
#
# Note the predicate-COUNT assertion deliberately stays green here: the reverted door writes
# `se.ln != nil`, not the full predicate, so counting copies never sees it. That is why the guard
# asserts routing as well as counting.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestASecondArmCannotOrphanALiveCeremony|TestTheArmedPredicateHasOneImplementation' -count=1"
EXPECT="a manual/TCP arm succeeded while a ceremony arm was live"
