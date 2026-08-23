# docs/red-proofs.md, tier 1: "every version skew reads as 'newer'" (/pending 247 prerequisite,
# v1.117.118)
#
# The defect: a PREFIX TEST where a comparison belongs. Anything starting `nib-invite-v` that was not
# this build's exact prefix answered "made by a newer version of Nib", so the first bump of
# InvitationVersion would have told a user holding an ORDINARY older invitation that it came from the
# future — D32's own class of defect, inside the machinery D32 put there.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAVersionSkewNamesTheDirection -count=1"
EXPECT="want this invitation was made by an older version"
