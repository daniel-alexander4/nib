# docs/red-proofs.md, tier 3: "the locked ceremony panel is on top, not merely in the DOM"
# (P06.S07, D29, v1.117.346)
#
# The defect, reintroduced exactly: the panel outside `#authOverlay`, which is `aria-modal` and
# covers it. **This is the reading tier 2 cannot make.** `element.hidden === false` is true of a
# node under a modal, so a jsdom assertion on it passes over the precise state P06.S02 marked met —
# the panel present, attached, and unreachable. The assertion is `document.elementFromPoint` at the
# panel's own centre: what the user hits when they click there.
TIER="tier 3 — a real browser"
PROVE="./build/uirepro.sh"
EXPECT="something else is on top of the locked ceremony panel"
