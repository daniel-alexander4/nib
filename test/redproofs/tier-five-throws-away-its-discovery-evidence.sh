# docs/red-proofs.md, tier 5: "the evidence a check consumed privately" (/pending 518)
#
# The defect: `TestTwoProcessesDiscoverEachOther` stops echoing the browsing process's output.
# It still asserts on it — `strings.Contains(got, "DISCOVERED")` is untouched — it just consumes
# the evidence and throws it away, which is what the test did before v1.110.1.
#
# **`go test ./...` is GREEN with this applied, and so is every tier below 5.** The test passes;
# the assertion it makes is correct; nothing compiles differently. What breaks is the harness's
# ability to tell a pass ABOUT DISCOVERY from a pass about anything else, and `mcastrepro.sh` is
# the only thing in the tree that asks that question:
#
#	grep -q "DISCOVERED" "$OUT" || fail "no process reported discovering another; the pass
#	                                     above is not about discovery"
#
# That guard was written against this exact defect (docs/red-proofs.md's vacuous-green table
# records it) and had never been replayed — tier 5 had two prose rows and no replayable one, so
# its own greps had never been made to fail through the harness. This is that row.
#
# The token is the harness's own failure sentence, not "DISCOVERED" and not a test name: the
# namespace log carries `=== RUN` and `--- PASS:` lines for every test the tier names, so a token
# built from one of those would re-prove on any red at all. `TestNoRedProofTokenIsItsTestsName`
# refuses that construction for harness rows now.
TIER="tier 5 — ./build/mcastrepro.sh, two processes over real multicast in a network namespace"
PROVE="./build/mcastrepro.sh"
EXPECT="the pass above is not about discovery"
