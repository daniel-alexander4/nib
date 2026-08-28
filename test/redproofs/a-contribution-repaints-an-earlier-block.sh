# docs/red-proofs.md, tier 3: "A signature repaints a mark already signed for" (/pending 302, v1.117.212)
#
# The defect: `stackPlacement`'s gap goes from 12 pt to -9 pt, so each block overlaps the one
# below it by 9 pt. `Contribute`'s premise is that a later party's contribution is an
# incremental update that "never disturbs the first party's signature" — asserted structurally
# today, in that `sign.Verify` still reports every signer, and never visually.
#
# 189 pixels of the fifth block are repainted by the sixth. The overlap is small deliberately:
# at -42 the fill assertion fires first and this one is never reached, so the row that proves
# THIS assertion has to leave the fill assertion satisfied.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="inside the five EARLIER blocks"
