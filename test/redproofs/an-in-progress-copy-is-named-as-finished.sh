# docs/red-proofs.md, tier 1: "a party's in-progress copy is not named as the finished document"
# (P06.S09, D28, v1.117.348)
#
# The defect: every arrival was `"co-signed with <peer>.pdf"`. At hop 3 of a nine-party ceremony a
# party's own copy was named as though the proceeding were over, and named after the one peer who
# happened to hand it over, with six signatures still to collect. The name is what a user reads
# most often, because it is the tab.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnInProgressCopyIsNotNamedAsTheFinishedDocument -count=1"
EXPECT="is named"
