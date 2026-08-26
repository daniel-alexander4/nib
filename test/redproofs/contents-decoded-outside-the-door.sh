# docs/red-proofs.md, tier 1: "a Contents payload is decoded outside the version gate" (P07.S02a, v1.117.155)
#
# The defect: a second unmarshaller of the vault's `Contents` payload, in `builtin.go`, with its
# receiver named `out` and no call to `checkContentsVersion`. Reading a payload there means a
# vault written by a newer Nib is accepted, and the next ordinary save — `AddRecent`, i.e. opening
# any PDF — rewrites it without the keys this build does not know. For a ceremony invitation
# secret that is the only copy, so the loss is unrecoverable. ADR-009: the rule gets ONE door.
#
# **This row is a guard's red proof, and the patch is shaped as the evasion that was measured.**
# The first draft of `TestEveryContentsDecodeGoesThroughTheDoor` counted the literal
# `json.Unmarshal(plain, &c)` in `vault.go` alone — so a decoder whose receiver was spelled
# differently evaded it completely, and `builtin.go` was never read at all. The guard now splits
# every non-test file in the package into functions and asks which of them unmarshal a Contents
# payload, which is the property rather than a spelling.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestEveryContentsDecodeGoesThroughTheDoor -count=1"
EXPECT="these decode a Contents payload outside decodeContents: [builtin.go:recentFromPayload]"
