# docs/red-proofs.md, tier 1: "the consent view drops the signer's block" (P02.S03, v1.128.8)
#
# The defect: `blockFor` is correct and reaches nobody. The field is `omitempty`, so a view that
# stops setting it is indistinguishable at the client from a document whose placement could not be
# computed — the box simply never appears, on the screen where a signer is told where their
# signature goes.
#
# A source scan, because the behavioural reach needs a live session with a peer mid-handshake and
# no tier below 4 arranges one. The scan proves the line is present; the equality row below proves
# it is right.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheConsentViewSendsTheBlock -count=1"
EXPECT="no longer carries the signer's own block"
