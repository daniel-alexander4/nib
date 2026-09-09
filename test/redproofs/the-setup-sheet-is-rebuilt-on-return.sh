# docs/red-proofs.md, tier 2: "the setup sheet is rebuilt on the way back" (P03.S04, v1.128.45)
#
# The slice's whole content is an ABSENCE: coming back from the document re-shows the sheet and
# does not call `loadPeerPicker()` or `restoreCeremonyDraft()`. Putting them back is the shape a
# later author reaches for whenever a restore looks like the safe thing to do — and it is not:
# `loadPeerPicker` empties `#cerPeerPick` BEFORE its fetch, destroying every checkbox's state and
# every capacity typed and not yet blurred, and where `/api/peers` answers nothing the restore
# matches no row and the next change event posts an empty roster over the saved one.
#
# **The check counts REQUESTS, not appearances.** "Re-entered rather than rebuilt" is not
# observable from the values in the fields — a rebuild that happens to restore the same values
# looks identical — so the assertion is that the return leg makes no GET at all.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="the return leg re-fetched the peers"
