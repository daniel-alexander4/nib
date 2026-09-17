# docs/red-proofs.md, tier 1: "two ceremony sweeps run at once" (/pending 515)
#
# The defect, verbatim as it stood until /pending 515: the sweep of `~/nib/ceremonies` is launched on
# a bare `go func` with nothing serialising it. `adoptVault` starts one whenever the vault goes from
# nil to open, and `handleVaultImport` sets `s.vault` back to nil and calls `ensureUnlocked` again —
# so an import landing while the first sweep is still walking the directory starts a second one over
# the same files, with a different identity. `handleCeremonyAccept` is a third trigger.
#
# What the overlap costs is exactly what `adoptVault` refuses INSIDE one sweep, in its own words:
# the close-out and the re-arms *"read the same listing and reach opposite conclusions about the same
# ceremony — one arms a rendezvous for it, the other moves its directory out from under that arm"*.
# Ordering two calls inside one goroutine says nothing about a second goroutine running the pair.
#
# `-race` cannot see it: the two sweeps race on the FILESYSTEM, a rename against a listing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestTwoCeremonySweepsNeverOverlap|TestEveryCeremonySweepGoesThroughTheOneDoor' -count=1"
EXPECT="were running at once"
