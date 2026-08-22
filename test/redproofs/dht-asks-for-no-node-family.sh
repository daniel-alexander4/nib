# docs/red-proofs.md, tier 1: "the DHT asks for no node family" (D8 tier 2, P05.S05)
#
# The defect: `DefaultWant` is dropped from our `dht.ServerConfig`, which is the state the tree
# shipped in until v1.117.43.
#
# `dht.NewServer` does not default it — only `NewDefaultServerConfig` does, and caveat 7 forbids
# that because it opens its own socket. So `find_node` goes out with `Want: nil`, BEP-32 says a
# responder answers with the family of the query SOURCE, and every seed we ship is an IPv4
# literal. v4 in, v4 out, permanently: the routing table cannot learn an IPv6 node, so
# `SelfAddress.V6` is structurally the zero value and D8's tier 2 cannot work.
#
# Measured 2026-08-22, over IPv4, one host: without `want`, all three shipped seeds returned
# nodes4 and zero nodes6. With `want=[n4,n6]`, 87.98.162.88 returned 304 bytes of nodes6.
#
# This is the SECOND field silently missed from a caller-supplied config; `Exp` was the first,
# and it deleted our own published record the first time anyone read it. The guard is written
# against the class rather than either instance: it reflects over what the library defaults and
# demands our literal set each one or name it with a reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/rendezvous/ -run TestOurServerConfigAnswersEveryFieldTheLibraryDefaults"
EXPECT="sets DefaultWant and our ServerConfig does not"
