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
# The guard also holds the proceeding lookup CONDITIONAL on a signature naming a ceremony, so an
# ordinary document pays no pdfcpu parse per request (CLAUDE.md's hot-path rule).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheAttestationsRouteDoesNotReVerify -count=1"
EXPECT="which verifies the whole"
