# docs/red-proofs.md, tier 1: "the armed screen is blank until the DHT bootstrap has run"
# (P06.S05, D16 amendment, v1.117.338)
#
# The shipped state, and its cause was a gate rather than an omission. `sessionStatus` fills
# `Diagnosis` only `if cer != nil && !inSession && cer.bootstrapDone.Load()` — which is RIGHT for a
# verdict, because a cause computed before the DHT has had its chance would accuse the wrong tier.
#
# The defect is putting the live PROGRESS view behind the same condition. Under ADR-011 nothing
# bootstraps until the local link has had its window, and where a browse has already answered that
# hold is `lanFirstBudget`: thirty seconds. So the product would be deliberately silent for the
# longest stretch of an arm — which is precisely the window D16 says must never be a blank spinner,
# and precisely the window the diagnosis structurally cannot speak in.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheStatusPublishesProgressBeforeTheBootstrap -count=1"
EXPECT="carries NO progress while the bootstrap has not run"
