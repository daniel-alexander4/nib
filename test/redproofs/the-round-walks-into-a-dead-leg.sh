# docs/red-proofs.md, tier 1: "a round with a dead caller names what stopped it" (/pending 355,
# v1.117.345)
#
# The other half, and it needs its own row because the two fail differently. With the context
# already dead at the top of the party loop, the round used to mint the party's invitation and
# enter a race it knew was over — reporting `tried 0 address(es), none answered as the pinned peer:
# context canceled`, a sentence about dialling for something that never dialled. The guard names
# the one thing that actually stopped it.
#
# Deleting this guard leaves the SIBLING case green, because there the context is alive when the
# loop is entered and only the race's cancellation ends the leg. That is why both are asserted.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARoundStopsWhenTheRequestThatStartedItGoesAway -count=1"
EXPECT="It walked into the leg anyway"
