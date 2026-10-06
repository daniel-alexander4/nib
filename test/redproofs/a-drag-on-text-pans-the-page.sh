# docs/red-proofs.md, tier 2: "A drag that starts on a run of text moves the page (drag to pan, v1.183.0)"
#
# The defect: the pan takes every left-button drag on the page, text included, so text selection —
# copying a clause out of a document — is gone the moment the page overflows its view.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="text can no longer be selected"
