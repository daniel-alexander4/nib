# docs/red-proofs.md, tier 1: "the GUI split has no source to compare against, so it replaces the
# open document's own file" (/pending 569, v1.135.x)
#
# The defect: `handleSplitPages`/`handleSplitBookmarks` hand `writeSplitParts` an empty source, which
# is what they had before this change — the handlers never resolved the addressed document because
# they work from posted bytes and had nothing to ask it for. `OutputOverwritingSource` answers "no
# collision" for an empty src by contract, so the door is called, returns false, and the part lands
# on the user's document.
#
# **It is the interesting half of /pending 569**, because the CLI's defect is visible in the loop and
# this one is visible only in what the handler never asked for. The item's own entry says the GUI
# door "has the same shape, established by READING" — the shape is the same and the mechanism is
# not, and driving it is what showed the loss is real: 1,239 bytes became 1,076 at status 200.
#
# The patch is one argument per handler, which is the size of the hole.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheGUISplitNeverWritesOverTheDocumentItIsSplitting -count=1"
EXPECT="replaced the open document's own file"
