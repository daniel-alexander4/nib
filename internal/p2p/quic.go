package p2p

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"time"

	"github.com/quic-go/quic-go"

	"nib/internal/safe"
	"nib/internal/udpmux"
)

// alpn is the ALPN protocol identifier Nib's QUIC sessions negotiate.
//
// It is wire-visible and versioned for that reason: an ALPN mismatch is a clean,
// immediate handshake failure naming the protocol, which is the right outcome when two
// Nibs of incompatible session versions meet. The alternative — negotiating and then
// failing somewhere inside the exchange — is the confusing one.
//
// quic-go REQUIRES a non-empty NextProtos, so this is not optional decoration.
const alpn = "nib/1"

// quicIdle is how long a QUIC connection may sit with nothing on it before the
// transport closes it, and quicKeepAlive is why it never gets there during a ceremony.
//
// The doc here used to say quicIdle was "deliberately longer than the session core's own
// exchangeDeadline". It was 5 minutes against the core's 6 — **shorter**, and the sentence
// had been false since the constant was written. With KeepAlivePeriod left at zero
// (quic-go: "If set to 0, then no keep alive is sent") a QUIC session emitted NOTHING
// while a user read the four words on screen, and a five-minute prompt ended the
// connection with a transport error instead of the session's own message. On the
// receiving side that lands after coSignExchange has already used the key — the exact
// signature-exists-but-the-write-fails outcome postConsentDeadline was written to prevent,
// one layer down where that fix cannot see it.
//
// **The fix is the keep-alive, not a bigger number**, and the reason is a coupling in
// another package: `udpmux.peerTTL` is also 5 minutes, and a connection that outlives its
// mux state starts routing short headers to the DHT — which `UseConnectionIDs`'s own doc
// calls "the worse failure of the two". Raising quicIdle past peerTTL trades this defect
// for that one. A keep-alive fixes both at once: the packets keep the connection up AND
// refresh the mux's connection-id entry, so the two five-minute constants stop being a
// pair that has to be kept in step across a package boundary.
//
// A ceremony is never actually idle — the humans are thinking. This says so on the wire.
const (
	quicIdle      = 5 * time.Minute
	quicKeepAlive = 30 * time.Second
)

// closeGrace bounds how long the LISTENING side waits for the peer to finish reading
// what it already wrote. It returns as soon as the peer closes, so this is the ceiling
// for a peer that has vanished, not the usual cost.
const closeGrace = 5 * time.Second

// quicConfig is the same on both ends.
func quicConfig() *quic.Config {
	return &quic.Config{MaxIdleTimeout: quicIdle, KeepAlivePeriod: quicKeepAlive}
}

// QUICDial opens a QUIC session to a pinned peer and returns it with its Channel
// established — the same contract as Dial, over the other transport D14 keeps.
//
// The socket is bound here and handed to quic-go through internal/udpmux rather than
// letting quic-go open its own. That is caveat 7: a NAT mapping is a function of the
// internal IP:port, so the mapped port, the DHT self-address probe and the live session
// must all be the same socket. Nothing is attached to the mux's DHT view yet — P04 does
// that — and wiring it now costs one expression and means P04 attaches a DHT to a
// socket the session is already sharing, instead of changing this code to make room.
// **ctx cancels the DIAL and nothing after it (P05.S03).** quic-go builds the connection
// with `context.WithoutCancel(ctx)` (`quic-go/transport.go`, in `doDial`), so the dial
// context governs establishment only and an established connection is not tied to it. A
// racer may therefore cancel every loser without touching the winner.
//
// **No goroutine outlives a successful dial**, and that is a property rather than an
// observation: `tr.Dial` and `OpenStreamSync` both RETURN on cancellation, so the three
// error paths below are the whole of the cancellation handling. A watcher goroutine
// attached to ctx.Done() would reach into the winner and tear down its mux and socket.
func QUICDial(ctx context.Context, addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte, timeout time.Duration) (*Conn, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, false)
	if err != nil {
		return nil, err
	}
	cfg.NextProtos = []string{alpn}

	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	sock, err := net.ListenPacket("udp", localWildcardFor(remote))
	if err != nil {
		return nil, err
	}
	mux := udpmux.New(sock)
	tr := &quic.Transport{Conn: mux.QUIC(), ConnectionIDGenerator: newCIDGen(mux)}

	// One closer for the whole stack, so a caller that defers Close does not have to
	// know a QUIC session owns a transport and a socket as well as a connection.
	shutdown := func(qc *quic.Conn) func() error {
		return func() error {
			if qc != nil {
				qc.CloseWithError(0, "")
			}
			tr.Close()
			return mux.Close()
		}
	}

	// The caller's ctx, bounded by the per-dial floor. Derived rather than replaced: the
	// floor is what stops a deadline-less context from becoming an unbounded dial.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	qc, err := tr.Dial(ctx, remote, cfg, quicConfig())
	if err != nil {
		shutdown(nil)()
		return nil, err
	}
	// The dialing side always speaks first in every one of the four entry points — the
	// commitment frame in Initiate and SendDocument — so opening the stream here and
	// letting the peer accept it on first data is the right way round.
	st, err := qc.OpenStreamSync(ctx)
	if err != nil {
		shutdown(qc)()
		return nil, err
	}
	ch, err := quicChannel(qc, st)
	if err != nil {
		shutdown(qc)()
		return nil, err
	}
	// The dialing side gets the same graceful stream close, wrapped in the socket
	// teardown it also owns.
	// The dialing side does not wait: it reads last, so it owes the peer nothing, and
	// its prompt close is what releases the listener's wait.
	inner := gracefulClose(qc, st, false)
	return &Conn{Channel: ch, closer: func() error {
		inner()
		tr.Close()
		return mux.Close()
	}}, nil
}

// QUICDialOn dials `addr` on the SHARED endpoint's transport — caveat 7's racing-dialer half
// (P05.S08), and S03's T03 "ONE mux and ONE quic.Transport for the whole race" as it was
// written. Unlike QUICDial it binds no socket and creates no transport; it dials on `e.tr`, the
// one the arm's mapping/probe measured and the punch (S08b) opens the hole for. So the dial's
// source port is the port the peer learned, which is the whole point.
//
// **The returned Conn's Close is CloseWithError-ONLY — non-owning.** `e.tr` and `e.mux` outlive
// this connection and the DHT is on them, so closing a loser must not touch them: `tr.Close()`
// abruptly terminates every connection on the socket (the winner included) and `mux.Close()`
// makes the DHT read `net.ErrClosed`, which anacrolix/dht turns into a panic. The owning
// teardown lives on `SharedEndpoint.Close()`, called once at ceremony teardown. This is the dial
// counterpart of `QUICListenOn`'s `ownsEndpoint=false`.
//
// Several of these may run concurrently on one `e.tr` (the racer dials N candidates at once):
// quic-go registers a distinct random CID per dial and the mux routes inbound by destination CID
// (2⁻⁶⁴ collision), so concurrent dials on one transport do not cross.
func QUICDialOn(ctx context.Context, e *SharedEndpoint, addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte, timeout time.Duration) (*Conn, error) {
	if e == nil {
		return nil, errors.New("p2p: no shared endpoint")
	}
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, false)
	if err != nil {
		return nil, err
	}
	cfg.NextProtos = []string{alpn}
	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	qc, err := e.tr.Dial(ctx, remote, cfg, quicConfig())
	if err != nil {
		return nil, err // non-owning: nothing of ours to tear down on failure
	}
	st, err := qc.OpenStreamSync(ctx)
	if err != nil {
		qc.CloseWithError(0, "") // the connection only; leave the shared transport/mux
		return nil, err
	}
	ch, err := quicChannel(qc, st)
	if err != nil {
		qc.CloseWithError(0, "")
		return nil, err
	}
	// gracefulClose does st.Close() + qc.CloseWithError and NOTHING to the shared transport/mux.
	return &Conn{Channel: ch, closer: gracefulClose(qc, st, false)}, nil
}

// localWildcardFor picks the wildcard bind matching the remote's address family, so a
// v6 peer is not dialled from a v4 socket.
func localWildcardFor(remote *net.UDPAddr) string {
	if remote.IP.To4() == nil && remote.IP != nil {
		return "[::]:0"
	}
	return "0.0.0.0:0"
}

// QUICListen arms a QUIC listener that drops any peer but the pinned one during the
// TLS handshake, exactly as the TCP listener does — the pinned-peer callback in
// SessionTLS is reused unchanged, and P02.S02's spike measured that it fires under QUIC
// on both ends.
func QUICListen(addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte) (Listener, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, true)
	if err != nil {
		return nil, err
	}
	cfg.NextProtos = []string{alpn}

	sock, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}
	mux := udpmux.New(sock)
	tr := &quic.Transport{Conn: mux.QUIC(), ConnectionIDGenerator: newCIDGen(mux)}
	ln, err := tr.Listen(cfg, quicConfig())
	if err != nil {
		tr.Close()
		mux.Close()
		return nil, err
	}
	return newQUICListener(mux, tr, ln, true), nil
}

// newQUICListener wires a quic listener onto the shared termination protocol.
//
// `shut` is the one piece QUIC does not share: it closes the quic listener always, and the
// transport and socket beneath it only when this listener bound them. Closing them when it
// did not is what `ownsEndpoint` exists to prevent, and putting that decision here keeps it
// in one place across both constructors.
func newQUICListener(mux *udpmux.Mux, tr *quic.Transport, ln *quic.Listener, owns bool) *quicListener {
	l := &quicListener{mux: mux, tr: tr, ln: ln, ownsEndpoint: owns}
	l.listenerCore = newListenerCore()
	l.loop = l.acceptLoop
	l.shut = func() error {
		err := l.ln.Close()
		if l.ownsEndpoint {
			l.tr.Close()
			if e := l.mux.Close(); err == nil {
				err = e
			}
		}
		return err
	}
	return l
}

// quicListener mirrors tlsListener's shape, and P05.S02 is why.
//
// It used to accept a connection and then wait for its first STREAM **on the accept path**,
// under `handshakeTimeout` — the exact head-of-line block `tlsListener.Accept`'s own doc
// records as fixed for TCP, and which was never fixed here. A connection that completes its
// QUIC handshake and opens no stream is not hypothetical under a racing dialer (P05.S03): it
// is every candidate the racer did not keep, and each one held this serial loop for up to 30
// seconds while the winner waited unaccepted. `runSession` re-arms its timer to the REMAINING
// window, so stalled losers burn real arm window and can disarm the session with the winner
// still queued.
//
// The argument in Accept's old comment — that a bounded outer wait bounds the wrong thing,
// because a FAILED handshake never surfaces from quic-go — is still true and is kept below.
// It simply does not reach these connections, whose handshakes succeed.
type quicListener struct {
	// listenerCore is the termination protocol, shared verbatim with tlsListener and
	// written once — see listenercore.go. What is left here is what is actually QUIC's:
	// the socket it sits on, the transport, the listener, and who owns them.
	listenerCore

	mux *udpmux.Mux
	tr  *quic.Transport
	ln  *quic.Listener
	// ownsEndpoint says whether Close may take the transport and socket down with it.
	//
	// True for QUICListen, which bound them. FALSE for QUICListenOn, where the endpoint is
	// shared with a DHT and outlives this listener — and where closing them would do two
	// distinct kinds of damage: `Transport.Close` "abruptly terminates all existing
	// connections" on that socket, and closing the mux makes the DHT's own read return
	// net.ErrClosed, which its library turns into a panic on a goroutine nothing of ours
	// is on.
	ownsEndpoint bool
}

func (l *quicListener) Addr() net.Addr { return l.mux.LocalAddr() }

func (l *quicListener) Transport() string { return TransportQUIC }

// loop accepts connections and hands each to its own stream-accepter.
func (l *quicListener) acceptLoop() {
	for {
		// Deliberately unbounded, and it is the opposite of what it looks like.
		//
		// quic-go's Listener.Accept yields only connections whose handshake COMPLETED;
		// one that fails — a peer that is not the pinned identity — is dropped inside the
		// library and never surfaces here. So a bounded context does not bound a bad
		// peer's handshake: it bounds how long we wait for a GOOD one, and a wrong peer
		// then costs the full timeout out of the arm window before we even loop. Measured
		// at 30 seconds, against TCP's immediate "peer identity does not match".
		//
		// The arm window is bounded by the caller, which closes the listener; that is what
		// ends this wait, and it arrives as ErrServerClosed.
		qc, err := l.ln.Accept(context.Background())
		if err != nil {
			if !errors.Is(err, quic.ErrServerClosed) {
				l.setCloseErr(err)
			}
			_ = l.Close()
			return
		}
		select {
		case l.sem <- struct{}{}:
		case <-l.done:
			gracefulClose(qc, nil, false)()
			return
		}
		go func() {
			defer safe.Recover("quic stream accept")
			defer func() { <-l.sem }()
			closeConn := gracefulClose(qc, nil, false)

			// The STREAM, off the accept path. It unblocks when the dialer's first frame
			// arrives — the commitment — so a peer that completes a handshake and then
			// says nothing is bounded here and costs no other peer anything.
			ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
			defer cancel()
			st, err := qc.AcceptStream(ctx)
			if err != nil {
				closeConn()
				return
			}
			ch, err := quicChannel(qc, st)
			if err != nil {
				closeConn()
				return
			}
			// The listening side waits: it is the one that writes last.
			conn := &Conn{Channel: ch, closer: gracefulClose(qc, st, true)}
			// Buffered hand-off, like the TCP side — see tlsListener.loop.
			select {
			case l.ready <- conn:
				// Buffered, so this does not block and the slot is released at once.
			case <-l.done:
				conn.Close()
			default:
				// The queue is full: `maxConcurrentHandshakes` connections are already
				// waiting for a serial accept loop that serves one session. This one is
				// not going to be served, and holding it would cost the slot that lets
				// the NEXT peer handshake at all.
				//
				// **What is dropped here is a PINNED peer's connection**, not a stranger's
				// — nothing else can reach this line, because a failed handshake returned
				// above. That sounds worse than it is and the argument is worth stating:
				// the only thing that can fill this queue is a peer connecting far faster
				// than one session can be served, which is our own racer (P05.S03) and is
				// bounded well below the buffer. A genuine peer that is dropped sees a
				// closed connection and redials, against a listener that is still armed.
				conn.Close()
			}
		}()
	}
}

// gracefulClose is the QUIC teardown, and it exists because closing a QUIC connection
// is NOT the polite thing closing a TCP one is.
//
// A TCP close flushes: the bytes are already in the kernel's buffer and get delivered
// even though the writer has gone. quic-go's CloseWithError sends CONNECTION_CLOSE
// immediately and abandons anything still unacknowledged — so the listening side of a
// ceremony, which writes the co-signed document and returns, destroys that document in
// its own deferred Close. Measured, not reasoned: the first version failed with
// "receive co-signed document: Application error 0x0 (remote)" — the initiator watching
// the finished document evaporate one frame from the end.
//
// **The wait is asymmetric, and the protocol is what makes that correct.** In all four
// entry points the LISTENING side writes last and the DIALING side reads last:
// Receive writes the co-signed document, ReceiveDocument writes the acknowledgement,
// and Initiate and SendDocument each end on a read. So only the listener has anything
// owed to the peer, and only the listener waits; the dialer closes immediately, which
// is precisely the event the listener is waiting for.
//
// Making both sides wait was the first attempt and it is why this comment is long: each
// waited for the other and every ceremony paid the full grace. Measured at 5.05s per
// session before the asymmetry, milliseconds after.
//
// quic-go exposes no "everything I wrote has been acknowledged" signal — SendStream's
// own Context is cancelled inside Close, before any of it is acked (send_stream.go:574)
// — so the peer going away is the strongest available proof that it is done reading.
func gracefulClose(qc *quic.Conn, st *quic.Stream, awaitPeer bool) func() error {
	return func() error {
		if st != nil {
			st.Close() // FIN: everything written is now owed to the peer
		}
		if awaitPeer {
			select {
			case <-qc.Context().Done():
			case <-time.After(closeGrace):
			}
		}
		qc.CloseWithError(0, "")
		return nil
	}
}

// quicChannel reads the verified peer and the channel binding off a completed QUIC
// handshake. It is TLSChannel's opposite number and takes the same two things from the
// same place — quic-go surfaces a real tls.ConnectionState, including a populated
// exporter (TestSpikeEKMWorksUnderQUIC) and the verified peer chain.
func quicChannel(qc *quic.Conn, st *quic.Stream) (Channel, error) {
	cs := qc.ConnectionState().TLS
	fp, err := verifiedPeerFingerprint(cs)
	if err != nil {
		return Channel{}, err
	}
	return Channel{Stream: st, PeerFP: fp, Export: cs.ExportKeyingMaterial}, nil
}

// Compile-time proof that a QUIC stream satisfies the session core's Stream. If quic-go
// ever drops SetDeadline the failure should be here, naming the reason, rather than at
// whichever call site happens to be compiled first.
var _ Stream = (*quic.Stream)(nil)

// cidLen is how long the connection IDs this endpoint issues are.
//
// Eight bytes: long enough that a bencode dictionary or a stray datagram matching one
// by chance is 2^-64, short enough to cost nothing per packet. quic-go's own default
// is four; the mux looks these up on every short header, so a wider id buys
// separation from the DHT traffic sharing the socket for one extra word of hashing.
const cidLen = 8

// cidGen issues this endpoint's connection IDs and tells the demultiplexer about each
// one — which is what lets the mux route a short header on its DESTINATION connection
// id rather than on the sender's address.
//
// It matters because the address rule over-claims: a DHT node at the same IP:port as
// an active QUIC peer was routed to QUIC and its reply lost. P02.S03 documented that
// and left it; P04.S01's first driven test hit it, because a rendezvous ping to a host
// we also hold a session with is exactly the shape.
//
// Every id this endpoint issues comes through here — dial (transport.go:277), accept
// (server.go:812, :912) and rotation (conn_id_generator.go:138) all call the
// generator — so the table is complete by construction.
type cidGen struct{ mux *udpmux.Mux }

func newCIDGen(m *udpmux.Mux) *cidGen {
	m.UseConnectionIDs(cidLen)
	return &cidGen{mux: m}
}

func (g *cidGen) ConnectionIDLen() int { return cidLen }

func (g *cidGen) GenerateConnectionID() (quic.ConnectionID, error) {
	b := make([]byte, cidLen)
	if _, err := rand.Read(b); err != nil {
		return quic.ConnectionID{}, err
	}
	g.mux.RegisterConnectionID(b)
	return quic.ConnectionIDFromBytes(b), nil
}
