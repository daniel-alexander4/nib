# docs/red-proofs.md, tier 4: "a relay accumulates arrivals instead of replacing the baton"
# (P07.S05b, v1.117.180)
#
# The defect: `installCeremonyResult` stops replacing by ceremony id and adds every hop's result as
# a new document. ADR-005 caps a machine at eight open documents, so a nine-party ceremony — eight
# hops — refuses the convener its OWN ceremony partway through, and the refusal arrives as a 409 on
# a route that looks unrelated.
#
# **Nothing cheaper can see it.** The failure needs a real relay with enough hops to cross the cap,
# and the property is a DELTA rather than a total: eight hops must leave the same footprint as one.
# A unit test over `installCeremonyResult` asserts the function replaces; this asserts that a
# ceremony walked end to end through the real routes actually goes through that door at every hop.
#
# The assertion is a delta and not a count on purpose — a total is contaminated by everything the
# run did before it, and would drift with the harness rather than with the product.
TIER="tier 4 — two real binaries"
PROVE="./build/pairrepro.sh -n 4"
EXPECT="accumulating instead of being replaced"
