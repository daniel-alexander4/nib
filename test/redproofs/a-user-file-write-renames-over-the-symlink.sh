# docs/red-proofs.md, tier 1: "a user-file write renames over the symlink" (/pending 515)
#
# The defect, verbatim as it stood until /pending 515: `ReplaceDurable` stats for the mode and hands
# straight to `WriteDurable`, which ends in `os.Rename(tmp, path)` — and a rename over a symlink
# REPLACES THE LINK. A user who keeps a working name pointing at a document in a synced folder,
# opens it in Nib and saves, got a regular file where her link was, the real document still holding
# the bytes from before the edit, and no message about either.
#
# `internal/cli`'s `writeNamed` had carried the resolution since /pending 504 and the GUI's two save
# doors had never had it — the ADR-009 shape exactly: one rule, reaching one of its sites.
#
# The PROVE line runs both tiers of the same defect: the door's own pair in `internal/atomicfile`,
# and the end-to-end `/api/save` in `internal/server` that proves the route reaches that door.
TIER="tier 1 — go test"
PROVE="go test ./internal/atomicfile/ ./internal/server/ -run 'Symlink|FollowsAChain' -count=1"
EXPECT="replaced the user's symlink with a regular file"
