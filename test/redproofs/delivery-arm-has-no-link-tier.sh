# docs/red-proofs.md, tier 4 --lan: "a delivery arm reaches the DHT on a link-local ceremony"
# (P08.S05h, ADR-011, P03's exit criterion, v1.117.323)
#
# The defect: the shipped one. `armForDelivery`'s doc claimed *"everything about the tier ladder is
# reused rather than rebuilt"*, and tier 1 was absent at both ends of a delivery leg. With no
# answerer the arm never sets `cer.watchingLink`, so `holdDHT` takes its `ns == 0` arm — the one
# that returns immediately after a flat `browseWindow` — and `publishWhenSlow` bootstraps the
# public DHT about two seconds after every delivery arm comes up.
#
# **Measured, at both the fix and the commit before it:** a nine-party `--lan` relay emitted **78
# packets destined off the link** where P03's exit criterion says zero, and it emitted exactly 78
# at v1.117.320 as well — so the regression arrived with P08's delivery arm, not with the slice
# that found it. The plan had recorded the criterion green since v1.117.207 and nothing had run
# `--lan -n 9` since.
#
# **Counting publishes finds nothing**, which is why the first probe of this said 0: no candidate
# is ever published in the namespace. The packets are the BOOTSTRAP the publish path performs on
# its way, and a stack trace on `ensureBootstrapped` is what named the caller.
#
# With the answerer restored the count is 102 rather than 78, because the reading now also covers
# the end-state round, which used to run outside the measured window entirely.
TIER="tier 4 --lan — nine real binaries in a namespace"
PROVE="./build/pairrepro.sh --lan -n 9"
EXPECT="packets destined off the link"
