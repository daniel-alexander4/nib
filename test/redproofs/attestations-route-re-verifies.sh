# docs/red-proofs.md, tier 1: "the attestations route re-verifies the whole document" (P07.S04, v1.117.169)
#
# The defect: `/api/attestations` goes back to `ReadAttestations(s.docBytes(doc))`, which verifies
# the whole file again — while `document.sig` sits beside the bytes, computed wherever they were
# installed. That is signature-count × document-SIZE work on a request path, with the answer
# already cached and thrown away.
#
# **The cost is size-driven, and the guard says so rather than asserting a number it cannot see.**
# Measured: nine signatures on a 31 KB document is single-digit milliseconds and scales roughly
# linearly in signature count; the plan's 5.2 s figure needs ~95 MiB. A timing assertion on a small
# fixture would measure noise, and a 95 MiB fixture would cost a minute on every `go test` run. So
# what is checked is whether the handler re-verifies at all — which is what actually regresses.
#
# **The second half of this guard was CORRECTED at P07.S05a, and the patch re-recorded with it.**
# It used to hold the proceeding lookup conditional on a signature naming a ceremony. That gate was
# wrong on its own terms — a convened but UNSIGNED document never names one, and it is exactly the
# case C18 is about — so the lookup is now unconditional and the hot-path property is carried by
# what this row already proves: the route does not re-verify. See `completeness-gated-on-a-signature`
# for the gate's own row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheAttestationsRouteDoesNotReVerify -count=1"
EXPECT="which verifies the whole"
