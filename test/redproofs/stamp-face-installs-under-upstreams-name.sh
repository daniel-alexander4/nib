# docs/red-proofs.md: "A stamp face installs under upstream's name" (/pending 494, 2026-09-16)
#
# The defect: `stampFaceBytes` still computes the renamed face and then hands pdfcpu the ORIGINAL
# bytes under nib's name. pdfcpu names a `.gob` by the PostScript name it reads out of the file and
# ignores the name it was handed (`installTrueTypeRep`), so the install "succeeds" and writes
# `LiberationSans.gob` — a face nothing can then refer to, since every reference asks for `NibSans`.
#
# It is the shape mdpdf's `installFallbacks` already refuses in so many words, and the reason it
# refuses: the next symptom is a panic from deep inside pdfcpu, several frames from the cause.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestEveryStampFaceInstallsUnderTheNameNibAsksFor -count=1"
EXPECT="installed under some other name"
