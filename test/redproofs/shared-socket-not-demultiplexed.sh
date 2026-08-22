# docs/red-proofs.md, tier 1: "the shared socket stops demultiplexing" (caveat 7, P05.S04)
#
# The defect: `udpmux.route`'s final arm sends everything to the QUIC view instead of the DHT
# view. The socket is still ONE socket and every address in the test still matches — what
# stops being true is that two consumers are being served by it.
#
# This row exists because the assertion it replaced could not fail. `TestTheDHTAndThe
# ArmedListenerShareOneSocket` used to compare `ln.Addr().String()` with
# `cer.end.LocalAddr().String()`, and both are `e.mux.LocalAddr()` on the SAME mux
# (p2p/quic.go:237, p2p/endpoint.go:68, udpmux/mux.go:202) — a value compared with itself, in
# any address family, for any bind string. It was S04's ledgered evidence for caveat 7's
# probe-and-session half.
#
# What one shared socket means is a DEMULTIPLEX, not an address: a QUIC-shaped datagram and a
# KRPC-shaped datagram sent to the same address by the same stranger must reach different
# views. That is a counter, and this is the defect that moves it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDHTAndTheArmedListenerShareOneSocket"
EXPECT="did not reach the DHT view"
