# docs/red-proofs.md, tier 1: "the watch rewrite writes through a planted symlink" (/pending 639, v1.169.4)
#
# The defect: watchTransform writes its result through writeAtomic, whose ReplaceDurable resolves a
# symlink and writes to its TARGET — so a link swapped into the watched directory while a rewrite ran
# sent that rewrite to a file outside the directory.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run '^TestTheWatchRewriteReplacesTheEntryRatherThanWritingThroughALink$' -count=1"
EXPECT="the rewrite followed a symlink planted in the watched directory"
