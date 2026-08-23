# docs/red-proofs.md, tier 1: "an XFDF checkbox read with the CSV cell reader" (/pending 9, v1.117.128)
#
# The defect: an XFDF checkbox <value> is the field's EXPORT-VALUE NAME — whatever name the form chose,
# with Off the one name PDF reserves for unchecked — and Nib read it with `truthy`, a CSV cell reader
# accepting eight English words. A form whose on-state was anything else came back UNCHECKED. The row
# carries a text field on purpose: with only the checkbox present an unchecked result changes nothing and
# pdfcpu refuses the whole fill, so the defect would surface as an ERROR. Its real shape is silent — the
# other fields apply, the fill reports success, and the box is quietly wrong.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAnXFDFCheckboxValueIsAnExportNameNotABoolean -count=1"
EXPECT="came back unchecked"
