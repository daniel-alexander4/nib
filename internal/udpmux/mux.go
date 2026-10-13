// Package udpmux separates QUIC session traffic from BitTorrent-DHT (KRPC)
// traffic arriving on ONE UDP socket, so both can share a single local IP:port.
//
// It exists because caveat 7 requires it: a NAT mapping — learned or requested —
// is a function of the INTERNAL IP:port, so a mapping obtained on the DHT socket
// is useless for a session that listens elsewhere, however well-behaved the NAT.
// The mapped port, the DHT self-address probe and the live session must therefore
// all be the same socket, and two protocols on one port must be separated before
// either library sees them.
//
// # The cheap discriminator does not work
//
// The obvious separator is the leading byte, and it is wrong on the steady state
// rather than on an edge. Every KRPC message is a bencode DICTIONARY, so its first
// byte is always 'd' = 0x64 = 0b0110_0100: QUIC's header-form bit (0x80) is clear
// and QUIC's fixed bit (0x40) is SET, which is exactly the encoding of a QUIC
// SHORT-header packet — the steady state of every established connection. quic-go's
// own passthrough hook is built on precisely this test
// (wire.IsPotentialQUICPacket(b) = b&0x40 > 0), which is why that hook cannot carry
// KRPC and why this package exists instead of a call into the library.
//
// TestTheCheapDiscriminatorIsWrong asserts the collision so nobody re-derives it.
//
// # What this routes on instead
//
// Two rules, in order:
//
//  1. A LONG-header packet (0x80 set) is QUIC. This is unambiguous in the other
//     direction: a bencode dictionary's 'd' can never have the top bit set, and a
//     KRPC message is always a dictionary at the top level. Long headers are how a
//     QUIC connection announces itself, so this is the rule that bootstraps an
//     INBOUND connection from an address never seen before.
//  2. Otherwise, a datagram from an address this process has SENT QUIC to is QUIC.
//     This is the rule that carries the steady state, where rule 1 says nothing.
//
// Anything else is KRPC and goes to the DHT.
//
// # Why the peer table is learned from our own writes, and never from theirs
//
// Rule 1 could equally well ADD the sender to the peer table, and that would be a
// mistake: the table would then grow from unauthenticated remote input, and a flood
// of forged long-header datagrams from spoofed addresses would grow it without
// bound. Learning only from outbound writes bounds the table by this process's own
// connections. Rule 1 still routes such a packet to QUIC — which is correct, because
// deciding whether an Initial packet is genuine is quic-go's job and it is equipped
// to do it — it just does not remember the sender.
//
// # Known limitations, both recorded rather than defended
//
//   - A DHT node at the same IP:port as an active QUIC peer is routed to QUIC. In a
//     ceremony the QUIC peer is the other party and the DHT nodes are strangers, so
//     the collision is not reachable in practice; if it ever were, the loss is DHT
//     traffic, not session traffic.
//   - A QUIC short-header packet that carries no connection id of ours — a stateless
//     reset from a peer this process never contacted — is handed to the DHT, which
//     fails to parse it as bencode and discards it. That is the same outcome as
//     dropping it. (A stale packet for a CLOSED connection still carries an id of ours
//     and goes to quic-go, which is the one equipped to answer it.)
//
// # A connection id is recognised, not remembered
//
// Rule 2 is refined once a connection-id generator is installed: a short header is QUIC
// exactly when its destination connection id is one this Mux ISSUED, and that is decided
// by a keyed check on the id's own bytes (NewConnectionID, knownCID) — there is no table
// of issued ids, so nothing expires and nothing grows. See NewConnectionID for why.
package udpmux

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"hash/maphash"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/safe"
)

const (
	// maxDatagram bounds one read from the socket. A datagram larger than this is
	// truncated by the kernel exactly as it would be without the mux in the path.
	maxDatagram = 65535

	// queueDepth is how many datagrams may await a consumer that is not reading.
	// A full queue DROPS, which is UDP's own semantics — the alternative is to
	// block the read loop and so let one stalled consumer starve the other, which
	// on this socket means a stalled DHT lookup silently killing a live session.
	queueDepth = 256

	// peerTTL is how long an address stays routed to QUIC after the last datagram
	// this process sent it. It is refreshed by every write, so it expires only
	// after a connection has been idle or closed for this long — at which point
	// the address is free to be an ordinary DHT node again.
	peerTTL = 5 * time.Minute

	// sweepEvery is the least interval between two full sweeps of the peer table
	// (/pending 711 R5-2). Sweeping on EVERY insert made an insert cost the table's size
	// under the write lock, so filling it was quadratic: measured, 60k destinations took
	// 84.6 s to learn, one lock hold of about 1 ms each at the end, with the read loop
	// waiting on the same lock for every inbound datagram. (The connection-id table that
	// shared this rule is gone — see NewConnectionID.) Swept at most this often, an insert is
	// O(1) and a sweep's O(n) is paid once per interval. An entry therefore outlives its
	// expiry by at most this long IN THE MAP, never in effect: every lookup compares the
	// expiry itself, so a stale entry routes nothing.
	sweepEvery = peerTTL / 8

	// ConnectionIDLen is how long the connection ids this Mux issues are: cidNonce random
	// bytes, then the keyed tag over them that knownCID recomputes (see NewConnectionID).
	//
	// Sixteen bytes, of QUIC's twenty: the tag is 64 bits, so a bencode dictionary or a stray
	// datagram reading as one of ours by chance is 2^-64 — the same figure the eight random
	// bytes of the table this replaced gave — and the 64-bit nonce keeps two issued ids from
	// colliding. The cost is eight more bytes in every short header the PEER sends us. A
	// connection id is opaque to the peer and any length up to twenty is the protocol's, so a
	// peer on an older build sees nothing it has to understand.
	ConnectionIDLen = 16
	cidNonce        = 8
)

// Mux owns one UDP socket and hands out two net.PacketConn views of it.
type Mux struct {
	pc net.PacketConn

	quic *side
	dht  *side

	mu    sync.RWMutex
	peers map[netip.AddrPort]time.Time
	// peersSweep is when the peer table may next be swept (sweepDue). Guarded by mu.
	peersSweep time.Time

	// cidSeed keys the tag inside every connection id this Mux issues. Drawn once at
	// construction and never written again, so NewConnectionID and knownCID read it with no
	// lock. cidsOn says a generator is installed (UseConnectionIDs), which is what switches
	// short-header routing off the address rule.
	cidSeed maphash.Seed
	cidsOn  atomic.Bool

	// now is fixed at construction so the read loop never observes it changing.
	// A test that reassigned it on a live Mux would race with route().
	now func() time.Time

	closeOnce sync.Once

	// Seam-inventory counters (class 1). Read by the mux's own tests and by
	// Stats, which is what a diagnosability route would report.
	longHeader atomic.Uint64
	byCID      atomic.Uint64
	byPeer     atomic.Uint64
	toDHT      atomic.Uint64
	learned    atomic.Uint64
	expired    atomic.Uint64
	panicked   atomic.Uint64
}

// Stats reports what the router did. Every field is monotonic for the life of
// the Mux except Peers, which is the live size of the peer table.
type Stats struct {
	// RoutedLongHeader — datagrams sent to QUIC by rule 1 (0x80 set).
	RoutedLongHeader uint64
	// RoutedByCID — datagrams sent to QUIC because their destination connection ID
	// is one this endpoint issued. Exact, and the rule that removes the address
	// rule's one collision.
	RoutedByCID uint64
	// RoutedByPeer — datagrams sent to QUIC by the ADDRESS rule, which applies only
	// while no connection ID has ever been registered. **Non-zero after a QUIC
	// transport is running is a wiring failure**: it means the connection-ID
	// generator was never installed, and the mux is back on the rule whose collision
	// swallows a DHT reply from a session peer.
	RoutedByPeer uint64
	// RoutedToDHT — datagrams that matched neither rule.
	RoutedToDHT uint64
	// Learned — addresses added to the peer table by an outbound QUIC write.
	Learned uint64
	// Expired — addresses swept from the peer table after peerTTL.
	Expired uint64
	// Peers — the live peer-table size, which may include entries past their expiry for up
	// to sweepEvery (they route nothing; see sweepEvery).
	Peers int
	// DroppedQUIC, DroppedDHT — datagrams discarded because that side's queue
	// was full. Non-zero means a consumer is not keeping up, not a routing bug.
	DroppedQUIC, DroppedDHT uint64
	// Panicked — datagrams dropped because routing or delivering ONE of them panicked.
	// Unlike every other counter here, **non-zero is a bug in this package**, never a
	// network condition and never a slow consumer: it means a datagram reached a path
	// `route` does not handle. The read loop survives it (see `handle`), so this is the
	// only trace such a datagram leaves.
	Panicked uint64
}

// New takes ownership of pc and starts routing. The caller must not read from pc
// again; it reads from QUIC() and DHT() instead. Closing the Mux closes pc.
func New(pc net.PacketConn) *Mux { return newWithClock(pc, time.Now) }

func newWithClock(pc net.PacketConn, now func() time.Time) *Mux {
	m := &Mux{
		pc:      pc,
		peers:   make(map[netip.AddrPort]time.Time),
		now:     now,
		cidSeed: maphash.MakeSeed(),
	}
	m.quic = newSide(m, true)
	m.dht = newSide(m, false)
	go m.readLoop()
	return m
}

// QUIC returns the view to hand to quic.Transport{Conn: ...}. Writes through it
// teach the router that the destination is a QUIC peer.
func (m *Mux) QUIC() net.PacketConn { return m.quic }

// DHT returns the view to hand to dht.ServerConfig{Conn: ...}. Writes through it
// teach the router nothing.
func (m *Mux) DHT() net.PacketConn { return m.dht }

// Stats reports the router's counters. Safe to call concurrently.
func (m *Mux) Stats() Stats {
	m.mu.RLock()
	n := len(m.peers)
	m.mu.RUnlock()
	return Stats{
		RoutedLongHeader: m.longHeader.Load(),
		RoutedByCID:      m.byCID.Load(),
		RoutedByPeer:     m.byPeer.Load(),
		RoutedToDHT:      m.toDHT.Load(),
		Learned:          m.learned.Load(),
		Expired:          m.expired.Load(),
		Peers:            n,
		DroppedQUIC:      m.quic.dropped.Load(),
		DroppedDHT:       m.dht.dropped.Load(),
		Panicked:         m.panicked.Load(),
	}
}

// Close shuts down both views and closes the underlying socket.
func (m *Mux) Close() error {
	var err error
	m.closeOnce.Do(func() {
		err = m.pc.Close()
		m.quic.shut(net.ErrClosed)
		m.dht.shut(net.ErrClosed)
	})
	return err
}

// LocalAddr is the address both views share — the point of the whole package.
func (m *Mux) LocalAddr() net.Addr { return m.pc.LocalAddr() }

// route decides which view a datagram belongs to. The two rules are documented on
// the package; the order matters, because rule 1 is the only one that can speak
// for an address never seen before.
func (m *Mux) route(p []byte, addr net.Addr) *side {
	if len(p) > 0 && p[0]&0x80 != 0 {
		m.longHeader.Add(1)
		return m.quic
	}
	if known, any := m.knownCID(p); known {
		m.byCID.Add(1)
		return m.quic
	} else if any {
		// A generator IS installed and this is not an id it issued, so the packet
		// is not for any connection this endpoint owns. That is a decision the
		// address rule cannot make, and it is the whole point: a KRPC reply from a
		// host we also have a QUIC session with reaches the DHT.
		m.toDHT.Add(1)
		return m.dht
	}
	if m.isQUICPeer(addr) {
		m.byPeer.Add(1)
		return m.quic
	}
	m.toDHT.Add(1)
	return m.dht
}

// UseConnectionIDs tells the mux that this endpoint's connection ids come from
// NewConnectionID, and switches short-header routing onto them.
//
// **The address rule survives only until this is called**, and that asymmetry is
// deliberate. Routing on the connection ID is exact; routing on the address over-claims
// for QUIC and swallows a DHT reply from a session peer — the collision P02.S03
// documented and P04.S01 hit on its first driven test. But a generator that a later
// change forgets to install would send EVERY short header to the DHT and break the
// session, which is the worse failure of the two. So the mux falls back to the address
// rule while it has been told nothing, and stops the moment it has been told something.
//
// Stats().RoutedByPeer is what makes a missed wiring visible rather than silent.
func (m *Mux) UseConnectionIDs() { m.cidsOn.Store(true) }

// NewConnectionID issues a connection id this Mux will recognise for as long as it lives.
//
// **The id carries its own proof, and the mux keeps no record of it** (/pending 669 #2,
// /pending 691 R7-3). An id is cidNonce random bytes followed by a tag over them, keyed with
// a seed only this Mux holds; knownCID recomputes the tag from the packet. It replaced a
// table of issued ids with a five-minute expiry refreshed on a match, which was wrong at
// both ends, for one reason — **quic-go tells nobody when a connection ends or an id is
// retired** (`ConnectionIDGenerator` is two methods, and neither names a connection), so a
// table can only guess an id's lifetime from a clock:
//
//   - Too SHORT for a live connection. quic-go issues three spare ids at the handshake and
//     the peer moves to the next one every 5,000-15,000 packets it sends
//     (conn_id_manager.go, `shouldUpdateConnID`). A spare is never matched until then, so it
//     aged out while the session sat on the safety words or the consent prompt — the keep-alive
//     refreshes only the id IN USE — and the first rotation of the document transfer that
//     followed put every later packet in the DHT's queue. Measured over real quic-go on two
//     muxes with the clock moved six minutes (three 2-minute steps, a byte each): a 40 MiB
//     write stalled at about 13,000 packets with 270 datagrams routed to the DHT.
//   - Too LONG for a stranger. Every Initial quic-go accepts draws an id before any
//     pin check, so each cost a table entry for five minutes.
//
// With the proof in the id there is no lifetime to get wrong and nothing a stranger's packet
// can grow.
//
// **It is a discriminator and not an authenticator, on purpose.** All it decides is which of
// two parsers sees a datagram. A forged tag would send a datagram to quic-go — which is where
// a single set bit (rule 1) already sends anything — and quic-go authenticates every packet
// itself. So the tag is `hash/maphash`, a keyed 64-bit hash, rather than a MAC: measured on
// the per-datagram path, about 8 ns and no allocation against about 90 ns for the table's
// read-lock, map index and clock read (3,000,000 iterations, five rounds, same run; one AES
// block was about 77 ns and two allocations). Nothing may come to rest on this check as
// evidence of who sent a packet.
func (m *Mux) NewConnectionID() ([]byte, error) {
	b := make([]byte, ConnectionIDLen)
	if _, err := rand.Read(b[:cidNonce]); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint64(b[cidNonce:], maphash.Bytes(m.cidSeed, b[:cidNonce]))
	m.cidsOn.Store(true)
	return b, nil
}

// knownCID reports whether a short-header packet carries an ID this Mux issued, and whether
// a generator is installed at all. The second return is what keeps the address rule alive
// until the generator is wired.
//
// **No lock and no clock**, which is also what keeps `handle`'s recover sound (/pending 512):
// a read loop that carries on after a recovered panic is only safe if nothing on the
// per-datagram path can panic while holding a lock, and this path now holds none. The slice
// expressions are inside the length check above them.
func (m *Mux) knownCID(p []byte) (known, any bool) {
	if !m.cidsOn.Load() {
		return false, false
	}
	if len(p) < 1+ConnectionIDLen {
		return false, true
	}
	id := p[1 : 1+ConnectionIDLen]
	return maphash.Bytes(m.cidSeed, id[:cidNonce]) == binary.BigEndian.Uint64(id[cidNonce:]), true
}

// sweepDue reports whether the peer table may be swept now, and if so books the next
// sweep. The caller holds the table's write lock.
func sweepDue(now time.Time, next *time.Time) bool {
	if now.Before(*next) {
		return false
	}
	*next = now.Add(sweepEvery)
	return true
}

// peerKey reduces an address to the comparable key the table is built on. It is a
// netip.AddrPort and not addr.String() because String allocates, and this runs once
// per datagram in each direction. IPv4-in-IPv6 is unmapped so the same peer keys the
// same however the two sides of the socket happened to represent it.
func peerKey(addr net.Addr) (netip.AddrPort, bool) {
	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		return netip.AddrPort{}, false
	}
	ap := ua.AddrPort()
	if !ap.IsValid() {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), true
}

func (m *Mux) isQUICPeer(addr net.Addr) bool {
	k, ok := peerKey(addr)
	if !ok {
		return false
	}
	m.mu.RLock()
	exp, found := m.peers[k]
	m.mu.RUnlock()
	return found && m.now().Before(exp)
}

// learn records a QUIC destination, and sweeps expired entries while it holds the
// lock — at most once per sweepEvery, so an insert does not cost the table's size. Sweeping here rather than on a timer keeps the package free of a goroutine
// whose only job is to delete map keys: the table only grows on a write, so a write
// is exactly when it is worth looking.
func (m *Mux) learn(addr net.Addr) {
	k, ok := peerKey(addr)
	if !ok {
		return
	}
	now := m.now()

	// The common case by far is a datagram to a peer already in the table and
	// nowhere near expiry, and that case must not take the write lock: learn runs
	// on EVERY outbound QUIC datagram, and the write lock it would take is the one
	// the read loop is contending for on every INBOUND datagram. Refreshing at half
	// the TTL keeps the entry comfortably alive while touching the lock once per
	// peer per two and a half minutes instead of once per packet.
	m.mu.RLock()
	exp, found := m.peers[k]
	m.mu.RUnlock()
	if found && now.Add(peerTTL/2).Before(exp) {
		return
	}

	m.mu.Lock()
	if _, seen := m.peers[k]; !seen {
		m.learned.Add(1)
	}
	m.peers[k] = now.Add(peerTTL)
	if sweepDue(now, &m.peersSweep) {
		for other, exp := range m.peers {
			if other != k && !now.Before(exp) {
				delete(m.peers, other)
				m.expired.Add(1)
			}
		}
	}
	m.mu.Unlock()
}

func (m *Mux) readLoop() {
	// The sole reader of the shared socket, processing UNTRUSTED inbound datagrams through
	// route/peerKey/deliver. Every other detached goroutine on this path carries the recover;
	// a panic here (a malformed datagram reaching an unhandled path) would otherwise take the
	// desktop process down with unsaved documents. First statement, so it runs last.
	//
	// **It is the SECOND recover that matters; this one is the backstop** (/pending 512). A
	// recover at the top of a goroutine ENDS the goroutine — the loop stopped, both views went
	// on blocking against a socket nobody was reading, and nothing anywhere said so. A silently
	// dead mux is a worse outcome than the crash it prevents, and calling `shut` here is not the
	// alternative it looks like: anacrolix/dht's `serveUntilClosed` panics on a read error while
	// the socket is open, so shutting turns the recover back into the crash. So the per-datagram
	// work carries its own recover (`handle`) and one bad datagram costs that datagram, exactly
	// as a full queue does.
	//
	// What is left for THIS recover is a panic raised by `m.pc.ReadFrom` itself, which no
	// datagram can cause: every production site hands the Mux a `*net.UDPConn`
	// (`p2p/endpoint.go`, `p2p/quic.go` twice, `cli/rendezvous.go`).
	defer safe.Recover("udpmux read loop")
	buf := make([]byte, maxDatagram)
	for {
		n, addr, err := m.pc.ReadFrom(buf)
		if n > 0 {
			m.handle(buf, n, addr)
		}
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = net.ErrClosed
			}
			m.quic.shut(err)
			m.dht.shut(err)
			return
		}
	}
}

// handle routes and delivers ONE datagram.
//
// It is a function of its own so that `recoverDatagram` has something to return FROM that is
// not the read loop — that is the whole mechanism: a panic unwinds to here, is counted, and the
// loop reads the next datagram.
//
// It takes `buf` and `n` rather than `buf[:n]`, so that the slice expression an impossible `n`
// would panic on is inside the recover's reach as well.
//
// **The defer costs nothing measurable on this path, and that is measured, not assumed** —
// CLAUDE.md's hot-path rule asks before logic is added to one, and a per-datagram `defer` in
// the sole reader of the shared socket is exactly that. Against the same function with the
// `defer` removed: 109.0 ns vs 107.7 ns (median of 5, quiet box), and 152.6 ns vs 152.1 ns
// across five ALTERNATING process pairs on a box at load 10.9 — where the no-recover variant
// was the slower of the two in three pairs of the five. So the cost is under the noise floor,
// ~1 ns on a ~110 ns path, against a datagram that has already crossed the kernel.
func (m *Mux) handle(buf []byte, n int, addr net.Addr) {
	defer m.recoverDatagram()
	p := buf[:n]
	s := m.route(p, addr)
	s.deliver(datagram{b: append([]byte(nil), p...), addr: addr})
}

// recoverDatagram counts and logs a panic that reached it, and returning normally is what
// resumes the read loop.
//
// **Counted as well as logged.** Nib is a desktop app whose log nobody reads by default, so a
// `log.Printf` alone is the shape the tree already calls out — set, never read.
// `Stats().Panicked` is what `nib rendezvous` prints and what a failed ceremony's diagnosis
// reports, which is the difference between "this is not a quiet network" and silence.
func (m *Mux) recoverDatagram() {
	if r := recover(); r != nil {
		n := m.panicked.Add(1)
		log.Printf("udpmux: recovered from a panic routing one datagram (%d dropped this way): %v", n, r)
	}
}

type datagram struct {
	b    []byte
	addr net.Addr
}

// side is one net.PacketConn view of the shared socket.
type side struct {
	m *Mux
	// learns is true for the QUIC view only: an outbound datagram teaches the
	// router that its destination is a QUIC peer.
	learns bool

	in chan datagram

	rd, wd deadline

	mu      sync.Mutex
	closed  bool
	closeCh chan struct{}
	err     error

	dropped atomic.Uint64
}

func newSide(m *Mux, learns bool) *side {
	s := &side{m: m, learns: learns, in: make(chan datagram, queueDepth), closeCh: make(chan struct{})}
	s.rd.init()
	s.wd.init()
	return s
}

func (s *side) deliver(dg datagram) {
	select {
	case s.in <- dg:
	default:
		s.dropped.Add(1)
	}
}

// shut records why this view stopped and unblocks everyone waiting on it. It is
// idempotent, because both the read loop and Mux.Close can reach it.
func (s *side) shut(err error) {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.err = err
		close(s.closeCh)
	}
	s.mu.Unlock()
}

func (s *side) closeErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	return net.ErrClosed
}

func (s *side) ReadFrom(p []byte) (int, net.Addr, error) {
	// A datagram already queued is delivered even if the view has since been
	// closed — dropping buffered data on close would lose a packet the socket
	// really did receive.
	select {
	case dg := <-s.in:
		return copy(p, dg.b), dg.addr, nil
	default:
	}
	select {
	case dg := <-s.in:
		return copy(p, dg.b), dg.addr, nil
	case <-s.rd.wait():
		// os.ErrDeadlineExceeded and not an error of our own: it satisfies
		// net.Error with Temporary() true, which quic-go's read loop depends on
		// (transport.go listen()). Transport.Close on a user-supplied conn
		// unblocks its loop by SETTING A READ DEADLINE and waits for it to
		// return; an error that is not Temporary would make Close hang forever.
		return 0, nil, os.ErrDeadlineExceeded
	case <-s.closeCh:
		return 0, nil, s.closeErr()
	}
}

func (s *side) WriteTo(p []byte, addr net.Addr) (int, error) {
	select {
	case <-s.closeCh:
		return 0, s.closeErr()
	default:
	}
	select {
	case <-s.wd.wait():
		return 0, os.ErrDeadlineExceeded
	default:
	}
	if s.learns {
		s.m.learn(addr)
	}
	return s.m.pc.WriteTo(p, addr)
}

// Close closes this view ONLY. The shared socket stays open for the other view, which is
// the whole reason a shim is needed: a library handed this conn must not be able to take the
// other library's transport down with it.
//
// **The reason used to be stated for both libraries and is true of only one.**
// `dht.Server.Close` does close the conn it was given
// (`anacrolix/dht/v2@v2.24.0/server.go:1185`). `quic.Transport.Close` does NOT: it gates that
// on the unexported `createdConn` (`quic-go@v0.61.0/transport.go:465`), which is false for
// both of Nib's `&quic.Transport{Conn: …}` literals — it instead calls
// `SetReadDeadline(time.Now())` on the conn. Measured, and it is why the shim is still right:
// `side.SetReadDeadline` sets a deadline on THIS view only, so the QUIC transport closing
// leaves the DHT's reads untouched. One true half is enough to need the shim; stating it of
// both made the tree contradict itself, since `interop_test.go` had it correct.
func (s *side) Close() error {
	s.shut(net.ErrClosed)
	return nil
}

func (s *side) LocalAddr() net.Addr { return s.m.pc.LocalAddr() }

// SetReadBuffer and SetWriteBuffer forward to the real socket. quic-go asks for a
// large receive buffer and logs a warning pointing at its wiki when the conn cannot
// oblige; more to the point, the buffer being sized is the SHARED one, so a shim that
// swallowed the request would leave both protocols on the default and show it as
// datagrams dropped by this package. Both views forward to the same socket, so the
// last caller wins — which is fine, because only quic-go asks and it asks for more.
func (s *side) SetReadBuffer(n int) error {
	if c, ok := s.m.pc.(interface{ SetReadBuffer(int) error }); ok {
		return c.SetReadBuffer(n)
	}
	return errors.ErrUnsupported
}

func (s *side) SetWriteBuffer(n int) error {
	if c, ok := s.m.pc.(interface{ SetWriteBuffer(int) error }); ok {
		return c.SetWriteBuffer(n)
	}
	return errors.ErrUnsupported
}

func (s *side) SetDeadline(t time.Time) error {
	s.rd.set(t)
	s.wd.set(t)
	return nil
}
func (s *side) SetReadDeadline(t time.Time) error  { s.rd.set(t); return nil }
func (s *side) SetWriteDeadline(t time.Time) error { s.wd.set(t); return nil }
