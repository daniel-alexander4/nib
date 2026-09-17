# docs/red-proofs.md, tier 1: "a sixth folder-filling writer arrives asking neither question"
# (/pending 569, ADR-009, v1.135.x)
#
# The defect: `runContinuousPagenum`'s named exemption is removed. That site creates a destination
# folder and fills it with names derived from its inputs, exactly like the two splits, and it is
# exempt on a real reason — its output is a SUPERSET of its input, so the collision loses nothing —
# but the reason has to be stated where the next reader will find it.
#
# **The row is about the guard, not about pagenum.** ADR-009 asks that the guard assert routing
# through the door rather than the text each site prints, because "eight copies checked for
# agreement say nothing about a ninth site added without one". Deleting the exemption comment is the
# cheapest way to simulate that ninth site: a writer that fills a user's folder and neither calls
# `pdfops.OutputOverwritingSource` nor says why it need not. Both of /pending 569's measured defects
# entered exactly that way.
TIER="tier 1 — go test"
PROVE="go test . -run TestEveryFolderFillingWriterAsksWhetherItIsAboutToWriteItsOwnSource -count=1"
EXPECT="creates a destination folder and fills it with derived names"
