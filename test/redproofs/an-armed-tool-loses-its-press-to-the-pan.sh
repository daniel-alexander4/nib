# docs/red-proofs.md, tier 2: "A drag with a drawing tool armed moves the page as well (drag to pan, v1.183.0)"
#
# The defect: the pan does not ask whether a tool owns the pointer, so a Redact drag both draws its
# box and drags the page out from under it.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="scrolled the page instead of drawing"
