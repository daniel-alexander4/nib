# docs/red-proofs.md, tier 1: "a signed arm keeps advertising a hop that is over"
# (/pending 300, v1.117.201)
#
# The defect: an arm that has SIGNED goes on announcing itself for the whole post-signing
# re-delivery window. A re-delivery is a reconnect by a peer that already holds the address, so the
# window needs the listener and not the advertisement — and announcing through it leaves a stale
# candidate on the link.
#
# **What that costs, measured.** After a four-party QUIC relay completed, the convener still heard
# all three parties announcing QUIC endpoints: six candidates, on both probes four seconds apart. A
# later ceremony on the same machines then browses a link full of them, and if its own fresh
# announcement is missed inside the two-second browse the candidate set is entirely stale —
# all-QUIC, so the glare path is taken and the dial goes to endpoints that will not serve this
# ceremony. It surfaced as *"Couldn't reach the rendezvous network"*, a D19 verdict about the DHT,
# for a peer on the link and announcing.
#
# **The patch removes the PER-ITERATION check specifically, because that is the half that was hard
# to get right.** An earlier fix closed the arm announcer at signing and took six stale candidates
# to two; the two that remained came from `answerHopSeekers`, whose gate only ran when a sighting
# resolved — and once a hop has signed, nothing is announcing to it, so the gate never ran at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestASignedArmStopsAnnouncingAndStopsAnswering -count=1"
EXPECT="never released its announcer after signing"
