# docs/red-proofs.md, tier 6: "a document already under a live ceremony is convened again"
# (P08.S09, C13, C15, v1.117.331)
#
# `ErrAlreadyConvened` has existed since P07.S02a and was driven at the PACKAGE, then at the route
# inside one process. C15 says every criterion in this phase is driven by the multi-instance
# harness, and this one was not: the refusal is about a DOCUMENT's state across convene calls, and a
# single-process test shares that state by construction rather than establishing it. The mutation
# drops the 409 mapping, so the second convene falls through to 400 — telling a user their roster is
# wrong when the truth is that this document is already part of a ceremony, which is a different
# action entirely.
TIER="tier 6 — ./build/ceremonyrepro.sh"
PROVE="./build/ceremonyrepro.sh"
EXPECT="C13 second convene"
