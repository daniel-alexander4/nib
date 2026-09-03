# docs/red-proofs.md, tier 2: "the consent screen shows every party who has already signed"
# (P06.S07, D27, v1.117.346)
#
# The defect D27 names: the screen naming the CONNECTED PEER and nobody else. Under a carry route
# that peer is a non-signing convener, so the one person shown is the wrong one — and at hop 6 the
# user is joining five signatures they were never shown. `renderConsentSigners` has drawn the whole
# list since v1.117.220 and nothing drove it until this slice.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentroster.test.mjs"
EXPECT="signer row(s) for a document carrying three signatures"
