# docs/red-proofs.md, tier 2: "The panel accuses over an upgrade" (P07.S09c, D32, v1.117.227)
#
# The defect: the client stops reporting an unreadable signature and lets the disagreement line
# fire — the state that shipped, seen from the surface a user actually reads.
#
# Go publishing `tagVersion` is only half the fix: the sentence a reader sees is the half that
# matters, and without this branch a document whose ONLY anomaly is that one party has a newer Nib
# is reported as "not produced by a single agreed proceeding".
#
# The row's own file carries both controls, because the branch stands in FRONT of the disagreement
# line: a ceremony this build can read in full must keep its proceeding verdict, and a genuine
# disagreement must still be reported. A fix that suppressed the check whenever any tagVersion was
# present would silence it on every ordinary ceremony in the product.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="was reported as not being one proceeding"
