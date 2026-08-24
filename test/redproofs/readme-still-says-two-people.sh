# docs/red-proofs.md, tier 1: "The trust page goes back to describing exactly two people" (P07.S08, v1.117.144)
#
# The defect: one paragraph reverts to the two-party wording. The clause this guards is the
# NEGATIVE half of S08 acceptance 1, and it is load-bearing precisely because the positive
# half — "the page describes a ceremony of N" — is a substring check over text this package
# wrote and cannot realistically fail.
#
# It asserts against the RENDERED page (api.ExtractContent, runs joined and whitespace
# collapsed), not against the Go constant: digitorus/pdf was measured to return one run per
# GLYPH with spaces expressed as positioning, so a naive extraction reports every phrase
# absent and this row would go green against an unfixed page.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestRenderedReadmeNoLongerSaysTwoParty -count=1"
EXPECT="the rendered readme still says"
