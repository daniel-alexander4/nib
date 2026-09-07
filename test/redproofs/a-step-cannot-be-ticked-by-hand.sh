# docs/red-proofs.md, tier 3: "a step cannot be ticked by hand" (v1.128.0)
#
# The checklist's markers stop responding, so a step Nib cannot observe can never be marked done.
#
# Eight of the fifteen steps are untracked by design — Nib cannot know whether you ran a
# hidden-content scan or emailed the file — so without the manual tick those rows are permanently
# dashes and the list can never read as finished. The tick is also recorded as the USER's claim
# (`data-by="hand"`), which is the other half: her judgement and Nib's observation are different
# kinds of evidence and the list is worth less if they look identical.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="an optional step cannot be checked off at all"
