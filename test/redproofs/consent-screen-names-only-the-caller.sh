# docs/red-proofs.md, tier 1: "The consent screen names one person" (P07.S07c, D27 item 3, v1.117.221)
#
# The defect: `sessionConfirmer.Confirm` builds the consent view without `signersSoFar`, so the
# screen names exactly one identity — whoever connected. Under a carry route that is a
# NON-SIGNING convener, and at hop 6 it is not any of the five parties whose signatures the user
# is about to join.
#
# **The guard asserts ROUTING, and the red proof is why.** The two behavioural tests beside it
# call `signersSoFar` directly and stayed GREEN under this patch — they prove the function works
# and say nothing about whether the screen is ever handed its result. Same shape as the L3 gate's
# own guard, same remedy (ADR-009).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheConsentGateRoutesThroughTheSignerList"
EXPECT="without reading who has already signed"
