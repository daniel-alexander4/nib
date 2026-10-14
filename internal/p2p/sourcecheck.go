package p2p

import (
	"net"
	"time"

	"golang.org/x/time/rate"
)

// ── Who gets a handshake before proving where they are (ADR-160, /pending 766 part 4) ────────────
//
// A QUIC Initial costs its sender one datagram and costs this endpoint a connection: quic-go derives
// keys, runs the TLS server flight — a key agreement and a signature — sends its reply and holds the
// connection for the handshake timeout, all BEFORE the pinned-peer check, which is the client's
// certificate and so arrives last. `listenerCore`'s pool and ADR-149's per-source share bound the
// TCP side of that; over QUIC nothing did, because the library does that work itself and hands this
// package only handshakes that completed. And a UDP source address is whatever the sender wrote, so
// the cost could be drawn from nowhere, by anybody, without receiving a single reply.
//
// The rule: **an endpoint starts a handshake for an unvalidated source only inside a small budget;
// past it, the source must first prove it can receive at the address it claims** — QUIC's Retry
// (RFC 9000 §8.1.2), which quic-go sends from `Transport.VerifySourceAddress`. A Retry is stateless:
// one token sealed, one datagram out, nothing held. The client answers it by itself, on the same
// socket, so a real peer pays one round trip and a spoofed source pays nothing and gets nothing.
//
// The budget is per ENDPOINT, not per source — the source is exactly the thing not yet known — and
// it is sized so that an honest ceremony never meets it: an arm is dialled by one pinned peer racing
// a handful of candidate addresses, re-raced no faster than `reraceBase` apart. Past the budget the
// honest peer is not refused, only asked: measured with a Retry demanded of EVERY Initial, the glare
// (both sides dialling and listening on one socket each), the deferred-stream handshake and the
// whole package's tests pass unchanged (see ADR-160 for the figures).
//
// What it does not bound, declared: a sender at a REAL address can answer the Retry and still be
// given a handshake that will fail the pin. That one has an address, which is what a per-source
// share would need; it is not built here.
const (
	// unvalidatedBurst is how many handshakes an endpoint starts for unvalidated sources at once.
	unvalidatedBurst = 16
	// unvalidatedPerSecond is how fast that allowance comes back.
	unvalidatedPerSecond = 4
)

// newSourceCheck is the `quic.Transport.VerifySourceAddress` every transport in this package
// installs (`TestEveryTransportChecksItsSources`): it reports whether the source of an Initial that
// carries no valid token must be sent a Retry before it is given a handshake.
func newSourceCheck() func(net.Addr) bool {
	return sourceCheck(unvalidatedBurst, unvalidatedPerSecond, time.Now)
}

// sourceCheck is the rule with its three inputs open, for the tests that move them.
func sourceCheck(burst int, perSecond float64, now func() time.Time) func(net.Addr) bool {
	lim := rate.NewLimiter(rate.Limit(perSecond), burst)
	// The address is not consulted, on purpose: it is unvalidated — quic-go's own note on this
	// callback says it "might be spoofed in case of an attack" — so a budget kept per address
	// would be a budget the attacker picks the keys of.
	return func(net.Addr) bool { return !lim.AllowN(now(), 1) }
}
