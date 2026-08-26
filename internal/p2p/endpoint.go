package p2p

import (
	"fmt"
	"net"

	"github.com/quic-go/quic-go"

	"nib/internal/udpmux"
)

// SharedEndpoint is ONE UDP socket carrying both a QUIC transport and a DHT view.
//
// # Why this type exists (caveat 7)
//
// A NAT mapping is a function of the internal `IP:port`. So the socket the DHT probes its own
// public address from, and the socket the session is actually reachable on, have to be the
// same one — otherwise the mapping that gets published belongs to a socket nobody listens on,
// and, as `rendezvous.Open`'s refusal puts it, "nothing downstream could tell".
//
// **The mechanism has been in the tree since P02.S03 and nothing ever composed it.**
// `udpmux.Mux` hands out two views of one socket — `QUIC()` and `DHT()` — and
// `internal/udpmux/interop_test.go` drives a real `anacrolix/dht` server and real quic-go over
// one mux in four tests. But in production `QUICDial` and `QUICListen` each bound their own
// socket and used only the QUIC view, and `nib rendezvous` bound its own and used only the DHT
// view. Two halves of a design, never joined.
//
// # What it does NOT do, and the limit is structural rather than an omission
//
// It is **QUIC-only**. A TCP listener is a `net.Listener` over a different IP protocol, and a
// NAT keeps separate mapping tables per protocol — which is exactly why D15 requires both UDP
// and TCP to be mapped when both are offered. So an arm on the TCP transport gets a DHT on a
// socket of its own, and caveat 7's clause is not satisfied for it. That is stated here rather
// than discovered at S06, which is the slice that first asks a router for a mapping.
//
// It also does not cover the racing DIALER. That half of the criterion belongs with the punch
// (S08), which is the first thing whose correctness depends on the dial's source port, and
// with S09, which is the first slice where the dialing side has a listener at all.
type SharedEndpoint struct {
	mux *udpmux.Mux
	tr  *quic.Transport
}

// NewSharedEndpoint binds one UDP socket and puts a QUIC transport on it.
//
// The connection-ID generator is installed here, at construction, because quic-go reads
// `ConnectionIDGenerator` exactly once inside its `init()` — assigning it later has no effect,
// and without it the mux falls back to routing short-header packets by peer ADDRESS, which
// over-claims for QUIC and swallows a DHT node that shares an `IP:port` with a live peer.
func NewSharedEndpoint(bind string) (*SharedEndpoint, error) {
	sock, err := net.ListenPacket("udp", bind)
	if err != nil {
		return nil, err
	}
	mux := udpmux.New(sock)
	return &SharedEndpoint{
		mux: mux,
		tr:  &quic.Transport{Conn: mux.QUIC(), ConnectionIDGenerator: newCIDGen(mux)},
	}, nil
}

// DHT is the packet view to hand to the rendezvous server. Opaque bytes: the DHT does not
// know what a QUIC packet is and must not have to.
func (e *SharedEndpoint) DHT() net.PacketConn { return e.mux.DHT() }

// Stats is the mux's routing tally, exposed here because the endpoint is the only handle a
// caller outside this package holds.
//
// **Its reader is the proof that this endpoint is ONE socket serving two consumers.** Address
// equality cannot show that: every accessor here and on the QUIC listener bottoms out in the
// same `m.pc.LocalAddr()`, so comparing them compares a value with itself. What distinguishes
// one shared socket from two is that a QUIC-shaped datagram and a KRPC-shaped one, sent to the
// same address, are demultiplexed to different views — and that is a counter, not an address.
func (e *SharedEndpoint) Stats() udpmux.Stats { return e.mux.Stats() }

// LocalAddr is the one address this endpoint is reachable on — the address a mapping would be
// requested for, the address the probe observes, and the address the listener answers on.
func (e *SharedEndpoint) LocalAddr() net.Addr { return e.mux.LocalAddr() }

// Punch sends one NAT-opening datagram to addr on the SHARED socket (P05.S08b, tier 4). Its only
// job is to open THIS host's NAT binding toward addr — the peer receiving it is incidental — so
// the payload is minimal and chosen to be DISCARDED cleanly by the peer's mux (grill 7b): the
// high bit is clear, so quic-go does not treat it as a long-header connection attempt, and it is
// not valid bencode, so the DHT view drops it rather than a parser choking. It goes out the same
// socket the QUIC session dials and listens on (caveat 7), which is the whole point: the hole is
// opened for the port the peer learned.
//
// The DHT view's write does not register addr as a QUIC peer (learns=false), so punching a
// candidate does not perturb the mux's routing of a real session with that host.
func (e *SharedEndpoint) Punch(addr net.Addr) error {
	_, err := e.mux.DHT().WriteTo(punchPayload, addr)
	return err
}

// punchPayload is one byte: high bit clear (not a QUIC long header) and not a bencode start
// (bencode begins with a digit, 'd', 'l' or 'i'), so the peer's mux routes it to the DHT view
// and the DHT drops it.
var punchPayload = []byte{0x00}

// Close tears the endpoint down. **The caller must close whatever is using it FIRST**, and the
// ordering is not a style preference: if the mux closes while a `dht.Server` is still reading
// its view, that read returns `net.ErrClosed`, the library's own `serveUntilClosed` sees an
// error with its `closed` flag unset, and it calls `panic(err)` on a goroutine nothing of ours
// is on. Process death, at shutdown, on a path a user reaches by quitting.
//
// `quic.Transport.Close` does NOT close the socket here — quic-go only does that for a
// connection it created itself, and this one is supplied — so the mux close below is what
// actually releases the fd.
func (e *SharedEndpoint) Close() error {
	if e == nil {
		return nil
	}
	e.tr.Close()
	return e.mux.Close()
}

// QUICListenOn arms a pinned-peer QUIC listener on an endpoint the CALLER owns.
//
// The difference from QUICListen is ownership and nothing else: this listener closes its own
// `quic.Listener` and leaves the transport and socket alone, because they outlive it and
// something else — the DHT — is using them. Closing the transport here would destroy every
// connection on the socket (`Transport.Close` "abruptly terminates all existing connections"),
// and closing the mux would panic the DHT's goroutine as described above.
func QUICListenOn(e *SharedEndpoint, identityCertPEM, identityKeyPEM, pinnedSPKI []byte) (Listener, error) {
	if e == nil {
		return nil, fmt.Errorf("p2p: no shared endpoint")
	}
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, true)
	if err != nil {
		return nil, err
	}
	cfg.NextProtos = sessionALPN
	ln, err := e.tr.Listen(cfg, quicConfig())
	if err != nil {
		return nil, err
	}
	return newQUICListener(e.mux, e.tr, ln, false), nil
}
