# docs/red-proofs.md, tier 3: "a chosen hue never reaches the cards" (ADR-025, v1.126.0)
#
# The rule that paints a card from the chosen hue is deleted. The picker still stores the choice,
# the vault still returns it, `<html>` still carries `data-cardhue` — and the sidebar keeps its
# six-accent rotation, so every part of the feature reports success and nothing changes on screen.
#
# It is tier 3 because the assertion is the RENDERED colour: `color-mix()` over a per-theme token
# is the browser's arithmetic, and no source scan reaches it. Tier 2 measures the ladder's contrast
# from the stylesheet and compares the three lists that name the hues; neither can see paint.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="picking a hue changed nothing on screen"
