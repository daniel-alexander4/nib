# docs/red-proofs.md, tier 1: "The About dialog is deleted and the claim strings survive elsewhere" (P07.S08, v1.117.144)
#
# The defect: #aboutMain is removed outright and the six trust claims are left behind in an
# HTML comment. This is the row that could NOT be recorded before S08 — measured, the previous
# guard (strings.Contains over the whole of web/index.html) returned true for ALL SIX claims
# against exactly this mutation, so the sole discharge of P07 C08 was green over a dialog that
# no longer existed.
#
# docs/red-proofs.md records this shape as instances two, three and four; this was the fifth.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAboutCopyContainsTrustClaims -count=1"
EXPECT="could not locate the About dialog"
