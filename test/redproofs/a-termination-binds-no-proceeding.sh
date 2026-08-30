# docs/red-proofs.md, tier 1: "a termination binds no proceeding" (P08.S04b, v1.117.286)
#
# The defect: the `RosterHash` comparison in `Termination.Verify` is disabled. That single field is
# the WHOLE binding — it commits to the id, the DocHash, the intent, the deadline and every roster
# entry — so without it one convener's decline ends any ceremony they are a party to.
#
# **The arm that reddens is the SAME-CONVENER one, and finding that out is the value of this row.**
# The first cut of the check had only cross-convener arms, so `Verify`'s convener-fingerprint test
# caught them and removing the binding entirely left the whole test GREEN. An arm two predicates can
# satisfy cannot say which one is missing — the same hole found hours earlier in /pending 324's exit
# door, in a different file, by the same method.
#
# One convener running two proceedings is also the real attack, and `invitation_test.go` already
# names it in those words: "Alice carried a lease, Bob carried a deed of sale at a different price."
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run ATerminationBindsExactlyOneProceeding -count=1"
EXPECT="ended a DIFFERENT ceremony of theirs"
