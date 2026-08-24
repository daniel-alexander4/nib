# docs/red-proofs.md, tier 1: "a digest-rule skew accuses instead of saying so" (P07.S02, v1.117.153)
#
# The defect: CheckDocument stops reading the record's DigestVersion, so a document hashed under a
# different content-digest rule falls through to the hash comparison and reports "these are not the
# same document" — a tampering accusation produced by a Nib point release.
#
# ContentDigestVersion's own doc comment claims it prevents exactly this. It could not: it was bound
# INTO the digest and carried nowhere beside it (three occurrences in the tree, no reader), and
# binding a version inside a hash changes the number without giving any reader something to compare.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestADigestVersionSkewSaysSoRatherThanAccusing -count=1"
EXPECT="want ErrDigestVersion"
