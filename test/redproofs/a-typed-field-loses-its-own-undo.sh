# docs/red-proofs.md, tier 3: "a typed field loses its own undo" (v1.123.3)
#
# The mirror of `undo-yields-to-an-empty-field`, and the reason the rule is "has an undo of its
# own" rather than the cheaper "is empty": `ownsUndo` stops yielding to text fields at all, so
# Ctrl+Z inside a note the user has typed into deletes the NOTE instead of the last word.
#
# The cheap rule would have passed the first row and failed this one only in a narrow case — type,
# select all, delete, undo — which is why the field is marked on its first input rather than read
# for emptiness at the moment the key arrives.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="removed a note the user had typed into"
