# docs/red-proofs.md, tier 1: "A page selection reveals a hidden layer" (/pending 525, v1.134.x)
#
# The defect: `carryOptionalContent` returns nil, so `/OCProperties` is dropped by the catalog
# allowlist exactly as it was from the first `Collect` until this item.
#
# It is the one entry on that drop list whose loss costs the AUTHOR rather than the reader, and in
# the direction nobody checks. A layer's content lives in the page's own content stream, bracketed
# `/OC /L0 BDC … EMC`, and the page's `/Resources /Properties` names the group — both travel with the
# page dictionary. The only thing saying the group is OFF is the catalog key. Measured on this
# fixture at 40 dpi, page 1 went from 39 dark pixels to 18,855 under Ghostscript 10.02.1 and from 6
# to 18,598 under poppler's pdftoppm: the box the author hid, printed on the page the user is about
# to hand over. `RedactPages` builds its runs of untouched pages through `collectWithoutStructure`,
# so the reveal landed inside a redaction.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAHiddenLayerIsStillHiddenAfterASelection -count=1"
EXPECT="the output has no /OCProperties"
