# docs/red-proofs.md, tier 1: "The announcer could emit for the whole 30-day arm" (P05.S09b T02, v1.117.80)
#
# The defect: the LAN announce loop ignores its window and emits every announceEvery (500ms) until
# disarm — the pre-S09b behaviour. Coupled to S09b's ceremony-scoped LISTEN window (up to D33's
# 30-day MaxCeremonyLife), that is 2/s × 86400 × 30 ≈ 5.2M multicast datagrams of a never-rotating
# six-word name per ceremony (the D6 privacy harm, and criterion 14's "nothing emits at full rate
# for the whole deadline"). The patch drops the window case, so the announcer never stops and the
# count keeps climbing past the window.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheAnnouncerStopsAtItsWindow -count=1"
EXPECT="the cap does not fire"
