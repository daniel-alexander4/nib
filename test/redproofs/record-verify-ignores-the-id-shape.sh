# docs/red-proofs.md, tier 1: "Record.Verify accepts an unusable ceremony id" (/pending 308, v1.117.264)
#
# The defect is NOT path traversal — `MirrorDir` refuses a bad id at every path site and always
# did. It is WHEN the refusal lands. With this hunk gone, a signed record carrying a hostile id
# passes Verify, passes MatchesRecord (a hostile id equals itself), reaches the user's consent,
# is SIGNED by Contribute, and is refused only at WriteMirror — so the convener holds a real
# signature and the signer is shown "Signed, but not saved — do not close Nib." for a ceremony
# that was never storable.
#
# A pinned peer is exactly who can do this: pinning authenticates WHO, not what they send, and a
# convener signs its own record, so both halves of every comparison are theirs.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestVerifyRefusesAnIDThatCannotNameADirectory -count=1"
EXPECT="want ErrBadID"
