# docs/red-proofs.md, tier 1: "the self-overwrite check compares path strings, so a link walks
# straight past it" (/pending 569, v1.135.x)
#
# The defect: `pdfops.OutputOverwritingSource` compares the SPELLING of the two paths instead of the
# file they name. That is the shape the obvious fix takes, it passes the headline test, and it is
# wrong — `internal/cli`'s write door resolves a symlink before writing (/pending 515), so a link in
# the output folder named as a part sends the write to whatever it points at. `<out>/foo1-2.pdf` and
# `<real>/doc.pdf` are two names for one file and no string comparison can see it.
#
# This row exists because the defect is INVISIBLE to the other four: with the string compare in
# place, `TestASplitNeverWritesOverTheDocumentItIsSplitting`, the before-the-first-write test, the
# mail-merge test and the GUI test are all green, and only the link case is red. A fix that ships
# with four of five green is the reason this one is recorded separately.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestASplitSeesThroughASymlinkToItsOwnInput -count=1"
EXPECT="sent the part through it"
