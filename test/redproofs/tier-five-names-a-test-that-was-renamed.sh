# docs/red-proofs.md, tier 5: "a named test is renamed out from under the tier" (/pending 518)
#
# The defect: `TestTwoSocketsCanShareThePort` is renamed to `TestTwoSocketsCanShareTheirPort`.
# That is an ordinary, innocent-looking edit — and it is the one `mcastrepro.sh`'s test list was
# built to refuse (/pending 505).
#
# **`go test ./...` is GREEN**: the test still exists and still runs, under its new name. But the
# tier's `-test.run` pattern is built from `DISC_TESTS`, which still names the old one, so inside
# the namespace the test MATCHES NOTHING, runs nothing, prints nothing — and the run exits 0.
# Before the single list, the run pattern and the PASS greps were maintained separately and three
# of the six tests could vanish exactly this way with the tier still green.
#
# The assertion is the per-test loop, and it is the harness's own — no test can make it, because
# the thing it observes is a test that DID NOT RUN:
#
#	grep -qF -- "--- PASS: $t (" "$OUT" || fail "$t did not PASS inside the namespace —
#	                                             renamed, skipped, or not run; …"
#
# The token deliberately starts after the test name. `--- PASS: TestTwoSocketsCanShareThePort (`
# is the grep's own pattern and a test name is printed by `-test.v` on every run whichever way it
# went, so a token containing one is satisfied by any red at all.
TIER="tier 5 — ./build/mcastrepro.sh, two processes over real multicast in a network namespace"
PROVE="./build/mcastrepro.sh"
EXPECT="renamed, skipped, or not run; this tier names it and must see it pass"
