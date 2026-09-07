# docs/red-proofs.md, tier 3: "an unfolded card lands at the end of the pane" (v1.125.4)
#
# `moveCommandsHome` puts every unfolded group back with `insertBefore(g, more)` — in front of the
# ⋯ More wrapper, which is AFTER every header — instead of after its own header. This is the state
# the product shipped in, and it is Dan's report in as many words: *"the content for an expanded
# pill shows below all of the pills and not directly under the header."*
#
# **It only appears after the sidebar has been shut once.** Collapsing moves the mode's cards into
# ⋯ More (ADR-017) and reopening brings them back; opening and closing cards never moves anything,
# which is why clicking pills all afternoon could not reproduce it. Measured with the defect in
# place: the open card's content began 245px below its own header.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="below its own header rather than directly under it"
