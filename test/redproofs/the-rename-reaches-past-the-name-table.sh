# docs/red-proofs.md: "The rename reaches past the name table" (/pending 494, 2026-09-16)
#
# The defect: `renamedFace` also rewrites two bytes of `hmtx` — the first glyph's advance width. It is
# the cheapest possible stand-in for the class the rename must not be able to enter, because `hmtx` is
# every width nib measures with: the fit verdict, the shrink, the wrap and the client's preview are
# all computed from the face pdfcpu parsed out of these bytes.
#
# A rename that alters the widths produces a face that is still installable, still named correctly and
# still draws — and lays text out differently from what the user was shown. Nothing but a comparison
# against the original bytes can see it, which is why the check is a table-by-table diff AND a
# field-by-field comparison of what pdfcpu parses; this defect trips both.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheRenamedStampFaceIsTheSameFace -count=1"
EXPECT="the rename is not confined to the name"
