# docs/red-proofs.md, tier 3: "undo drains one stack then the server" (ADR-023, v1.124.1)
#
# `undoAny` goes back to draining nib's overlay stack and then falling through to the server ring
# — the state the product shipped in until 2026-09-06. pdf.js's annotation-editor stack is then
# reachable only while one of ITS tools is armed, so drawing something and then arming a nib tool
# leaves the drawing beyond the reach of Ctrl+Z entirely.
#
# It is the defect as reported: *"if I draw lines and then draw shapes, ctrl-z will not undo the
# previous set of lines."* Measured before the fix — two editor changes on screen, four presses,
# nothing undone.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="did not reach the DRAWING"
