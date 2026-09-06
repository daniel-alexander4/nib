# docs/red-proofs.md, tier 3: "a reload leaves the document dirty" (v1.123.5)
#
# `reloadFromDisk` stops clearing `dirty`. Every arrival through `setDocumentFromServer` sets it,
# which is right for the twenty operations that reach that sink and wrong for a reload: the bytes
# now MATCH the file, so there is nothing unsaved to lose.
#
# The symptom is a close prompting about work that no longer exists — a dialog asking the user to
# confirm discarding something she has already discarded. Nothing about the rendered document
# looks wrong, which is why it is asserted through the close rather than through the page.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="closing after a reload prompted about unsaved work"
