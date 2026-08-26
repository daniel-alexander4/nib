# docs/red-proofs.md, tier 1: "a refusal is reported as a connect failure" (P07.S03b, v1.117.165)
#
# The defect: both lifts go — the one in `handleSessionInitiate` and the one in `connectFailure` —
# so a contribution refusal falls through to `writeConnectDiagnosis`, which renders a **502**
# wrapped in *"could not connect to peer"* and picks a **D19 network cause**, for an exchange in
# which the peer connected perfectly well and said no.
#
# **This is P07.S03a's wire fix undone one layer up, and it was measured at tier 4 rather than
# reasoned about**: the refusal crossed correctly and came out of the API as
# `{"error":"could not connect to peer: a co-signature takes exactly one prior signer"}`.
# `verify.go` already states the harm for its own case in words that apply unchanged — "could not
# connect" invites a retry, and a retry is the wrong advice for every one of these — and
# `connectFailure` already lifts `ClockSkewError` for the same reason.
#
# BOTH lifts are in the patch because a rule enforced at one of two doors is the ADR-009 shape:
# `diagnosis.go` reaches `connectFailure` independently of the handler.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARefusalIsNotReportedAsAConnectFailure -count=1"
EXPECT="a refusal wearing a transport sentence"
