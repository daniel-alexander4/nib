# docs/red-proofs.md, tier 1: "a ceremony pin renames a pin the user made" (P07.S02b, v1.117.157)
#
# The defect: `addPinned` assigns the label unconditionally, which is what it did until this
# slice. `AddCeremonyPeer`'s own doc said an existing pin is never downgraded, and the code
# honoured that for `Ceremony` and not for `Label` — so accepting an invitation overwrote the
# user's private nickname for a peer they had pinned themselves with whatever label the convener
# published. A stranger editing this machine's peer list by inviting it.
#
# Invisible until this slice because `AddCeremonyPeer` had no production caller at all. The guard
# also asserts the two controls that stop the fix becoming "labels are frozen": the USER renaming
# their own peer still works, and an unnamed pin may be filled in.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestACeremonyPinNeverRenamesAPinTheUserMade -count=1"
EXPECT="the ceremony renamed the user"
