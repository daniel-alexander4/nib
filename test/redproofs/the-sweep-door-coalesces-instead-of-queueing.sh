# docs/red-proofs.md, tier 1: "the sweep door coalesces instead of queueing" (/pending 515)
#
# The near-miss fix, recorded because it is the one a reviewer reaches for and it passes the
# overlap assertion: `TryLock`, skip when a sweep is already running. Peak concurrency stays at 1
# and the race is gone.
#
# It is still wrong. The second sweep exists because `handleVaultImport` opened a DIFFERENT vault —
# a different fingerprint, a different answer to "am I the convener", a different set of ceremonies
# to arm for — so its sweep is not a duplicate of the one in flight, and dropping it leaves that
# identity unarmed until the next unlock. The same count catches the opposite failure, a sweep
# wedged behind another, which is the cost a mutex can impose.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTwoCeremonySweepsNeverOverlap -count=1"
EXPECT="sweeps ran to completion"
