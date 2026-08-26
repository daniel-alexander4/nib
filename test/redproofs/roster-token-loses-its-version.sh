# docs/red-proofs.md, tier 1: "the roster token loses its format version" (P07.S04, v1.117.169)
#
# The defect: the commitment is written without the record format version it was computed under.
#
# `FormatVersion` is the FIRST substantive axis of `rosterPreimage`, so two builds at different
# versions digest the IDENTICAL roster to different hashes. The commitments then disagree, and the
# client's honest reading of that is *"This document was not produced by a single agreed
# proceeding"* — an accusation about two people who agreed on everything, caused by one of them
# updating Nib. That is D32's forbidden shape arriving through the one surface D32 excused.
#
# With the version present the client says which it is, in words that name both numbers.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheRosterTokenIsWellFormedWhereItIsActuallyBuilt -count=1"
EXPECT="does not carry the token in its documented shape"
