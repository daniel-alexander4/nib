# docs/red-proofs.md: "The stamp faces keep upstream's name" (/pending 494, 2026-09-16)
#
# The defect: the rename token is upstream's own, so `nibFaceName` is the identity and `renamedFace`
# rewrites each name record into itself. Everything still installs and every width is still right —
# and pdfcpu's watermark matcher (`stamp.go` `createFontResForWM`) sees the office suite's own
# `BAAAAA+LiberationSans` under the same name as nib's face again, so `unreusableFace` refuses it and
# the stamp is drawn in Base-14 Helvetica.
#
# Measured on the fixture the check builds: the converted document fails `7.1 t8 / 7.1 t10`, and with
# this applied the stamp over it adds `7.21.4.1 t1` — the clause /pending 494 is about, back.
TIER="tier 1 — go test (LibreOffice + veraPDF)"
PROVE="go test ./internal/pdfops/ -run TestAStampOnAnOfficeDocumentEmbedsItsFace -count=1"
EXPECT="still fails PDF/UA 7.21.4.1"
