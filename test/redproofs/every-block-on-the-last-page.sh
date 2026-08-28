# docs/red-proofs.md, tier 1: "Every block stacks on the last page, indexed by a signature count"
# (P07.S06, v1.117.210)
#
# The defect: the shipped rule. `stackPlacement(PageCount(pdf), len(Verify(pdf).Signers))` puts
# every block on the LAST page indexed by a running count, so after S02 allocated the signature
# pages a ceremony of nine landed all nine on signature page 2 — page 1 receiving nothing — and
# block 8 topped out at 892 on an 842 pt A4 page. `sigpages.go` says so in its own comment:
# "It allocates pages; it does not place blocks on them."
#
# Two assertions fail differently on purpose. The PAGE arm catches the pile-up; the BOX arm
# catches the block that climbs off the media box; and a third arm catches an allocated page that
# receives nothing, because "every block is inside its page" is satisfied by putting them all on
# one and leaving the other blank.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestEveryBlockLandsOnItsOwnAllocatedPageAndInsideIt"
EXPECT="is on page"
