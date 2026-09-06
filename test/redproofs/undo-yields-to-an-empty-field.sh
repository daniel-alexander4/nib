# docs/red-proofs.md, tier 3: "undo yields to an empty field" (v1.123.3)
#
# The Ctrl+Z guard goes back to `isTypingTarget`, so ANY focused text field takes the keystroke —
# the state the product shipped in until 2026-09-06. Placing a note focuses its empty
# `textarea.note-text`, whose native undo stack holds nothing, so the shortcut did nothing at all
# and the note stayed. The Undo BUTTON was enabled throughout, which is why this reads as "Ctrl+Z
# is broken" rather than as "there is no history": the history was there and stepping back through
# it worked — three rotations, three undos, exactly back to start.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="Ctrl+Z did nothing to a note that was just placed"
