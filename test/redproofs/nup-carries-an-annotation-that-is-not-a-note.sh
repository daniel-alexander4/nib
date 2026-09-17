# docs/red-proofs.md, tier 1: "An n-up carries /Text and nothing else" (ADR-045)
#
# The defect: `captureNoteAnnots` accepts every annotation subtype instead of `/Text` alone.
#
# **This mutation SURVIVED its first probe and the survivor was a real coverage hole.** The widget
# test stayed green, because a filled form's widgets carry indirect appearance streams and
# `copyAnnot` refuses those whatever the subtype rule says — so that fixture cannot tell the two
# reasons apart. `linkFixture` closes it: a `/Link` whose every key is direct is fully copyable, so
# the subtype test is the only thing between it and the sheet.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAnNUpLeavesACopyableNonNoteAnnotationBehind -count=1"
EXPECT="this door carries /Text alone"
