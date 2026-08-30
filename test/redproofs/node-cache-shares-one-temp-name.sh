# docs/red-proofs.md, tier 1: "two node-cache writers share one temp name" (/pending 316, v1.117.261)
#
# The defect: `writeNodes` goes back to its hand-rolled `os.WriteFile` into a FIXED
# `dht-nodes.tmp` followed by `os.Rename`, outside `internal/atomicfile`.
#
# Writer B opens the fixed name before A renames it, so both hold a descriptor on the same inode;
# after A's rename that inode IS the published cache, and B's truncate-and-write lands on the live
# file. The rename's atomicity is defeated and so is writeNodes' own "do NOT truncate a good cache"
# rule. Reachable because `nodeCacheDir` is one path per config dir and three sites open a
# rendezvous.Server against it — one per ceremony — each saving on Close(), plus the second Nib
# this repo deliberately permits.
#
# The check asserts the CACHE, not the error: a failed write costs one cold start, a torn cache is
# silent and can parse into node addresses assembled from the wrong offsets (see cacheMagic's doc).
# Measured with the defect applied: 211/600 writes failed, 22/20001 reads torn.
#
# The patch also drops the `atomicfile` import — without that the package fails to COMPILE, which
# redproof.sh reports distinctly and which would be red for the wrong reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/rendezvous/ -run TestTwoWritersCannotTearTheNodeCache -count=1"
EXPECT="neither writer's file"
