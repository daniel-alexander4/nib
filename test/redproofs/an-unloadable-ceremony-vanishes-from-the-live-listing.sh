# docs/red-proofs.md, tier 6: "a ceremony folder damaged by hand vanishes from the listing"
# (P08.S09, C12, C15, v1.117.331)
#
# The sibling of `an-unloadable-ceremony-vanishes`, one tier up: that row drives `ListStored`
# directly over directories the test built, this one damages a directory a REAL Nib wrote, under a
# REAL Nib that is still running and will answer the route about it — which is C12's actual case, a
# user tidying `~/nib` by hand. A ceremony Nib will not admit exists is one whose only remedy is
# finding and deleting the folder by hand, which is where the user already is.
TIER="tier 6 — ./build/ceremonyrepro.sh"
PROVE="./build/ceremonyrepro.sh"
# **Not "C12" (/pending 505).** That string is printed by CLAUSE 11's two SETUP failures
# (`no "C12 setup" …`) and by its PASS line (`… degrades ONLY its own entry (C12)`), so a run
# that failed anywhere else while this clause passed or never set up re-proved this row. The
# token is the sentence only the vanished-entry branch prints.
EXPECT="VANISHED from the listing rather than degrading"
