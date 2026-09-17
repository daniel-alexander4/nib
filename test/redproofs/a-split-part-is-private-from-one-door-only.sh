# docs/red-proofs.md, tier 1: "a split part's mode depends on which surface produced it"
# (/pending 570, v1.135.x)
#
# The defect: the GUI's split writer hard-codes 0600 for a new part while the CLI's writes 0644 —
# the state this tree was actually in. Same operation, same re-derivable output, and nobody chose
# it: each door inherited whatever mode its helper happened to pass, exactly as the durability
# difference did (/pending 550).
#
# It is user-visible in a way durability is not: a part written by the GUI is unreadable by another
# account and one written by the CLI is not, so a folder of "the same" parts is readable or not
# depending on where the user clicked. Recorded because nothing catches a literal that disagrees
# with a literal one package over — the two doors compiled, tested and shipped green while
# disagreeing, which is how it survived.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestASplitPartIsReadableByTheAccountTheUserGaveItTo -count=1"
EXPECT="a part written by the GUI is"
