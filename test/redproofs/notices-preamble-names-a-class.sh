# docs/red-proofs.md, tier 1: "The notices preamble asserts a licence class" (sweep 10, v1.117.45)
#
# The defect: THIRD-PARTY-NOTICES.md's preamble summarises the whole component set as "under a
# permissive license (BSD, MIT, or Apache-2.0)". It is not, and the same file says so 3,100
# lines later: github.com/anacrolix/dht/v2 is MPL-2.0, compatible through MPL 2.0 §3.3 rather
# than through being permissive, and the vendored web assets add SIL OFL and ISC.
#
# It is a distribution-facing legal document, and the preamble is the part a reader takes as the
# summary. The guard is on the CONSTRUCTION rather than on the stale names — replacing three
# licence names with five goes stale the same way on the next dependency.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestTheNoticesPreambleNamesNoLicenseClass"
EXPECT="names the license class"
