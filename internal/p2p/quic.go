package p2p

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"time"

	"github.com/quic-go/quic-go"

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
// transport closes it. It is deliberately longer than the session core's own
// exchangeDeadline: the core's deadline is what should end a stalled session, with a
// message about the session, and an idle timeout firing first would replace that with
// a transport error.
const quicIdle = 5 * time.Minute

// closeGrace bounds how long the LISTENING side waits for the peer to finish reading
// what it already wrote. It returns as soon as the peer closes, so this is the ceiling
// for a peer that has vanished, not the usual cost.
const closeGrace = 5 * time.Second

// quicConfig is the same on both ends.
func quicConfig() *quic.Config {
	return &quic.Config{MaxIdleTimeout: quicIdle}
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
func QUICDial(addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte, timeout time.Duration) (*Conn, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	return &quicListener{mux: mux, tr: tr, ln: ln}, nil
}

type quicListener struct {
	mux *udpmux.Mux
	tr  *quic.Transport
	ln  *quic.Listener
}

func (l *quicListener) Addr() net.Addr { return l.mux.LocalAddr() }

func (l *quicListener) Close() error {
	err := l.ln.Close()
	l.tr.Close()
	if e := l.mux.Close(); err == nil {
		err = e
	}
	return err
}

func (l *quicListener) Accept() (*Conn, error) {
	// Deliberately unbounded, and it is the opposite of what it looks like.
	//
	// quic-go's Listener.Accept yields only connections whose handshake COMPLETED;
	// one that fails — a peer that is not the pinned identity — is dropped inside the
	// library and never surfaces here. So a bounded context does not bound a bad
	// peer's handshake: it bounds how long we wait for a GOOD one, and a wrong peer
	// then costs the full timeout out of the arm window before we even loop. Measured
	// at 30 seconds, against TCP's immediate "peer identity does not match".
	//
	// The arm window is already bounded by the caller, which closes the listener; that
	// is what ends this wait, and it arrives as ErrServerClosed below. handshakeTimeout
	// still bounds the peer that completes a handshake and then says nothing — see the
	// stream accept.
	qc, err := l.ln.Accept(context.Background())
	if err != nil {
		// A closed listener must report net.ErrClosed so the caller's accept loop
		// ends rather than spinning: quic-go reports its own error for that, and a
		// loop that could not tell "closed" from "this peer failed" would busy-loop
		// on a disarmed session.
		if errors.Is(err, quic.ErrServerClosed) {
			return nil, net.ErrClosed
		}
		return nil, err
	}
	closeConn := gracefulClose(qc, nil, false)

	// Accepting the STREAM as well as the connection, here rather than in the caller.
	// It unblocks when the dialer's first frame arrives, which is the commitment — so
	// a peer that completes the handshake and then says nothing is bounded, which is
	// where handshakeTimeout actually earns its keep on this transport.
	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()
	st, err := qc.AcceptStream(ctx)
	if err != nil {
		closeConn()
		return nil, err
	}
	ch, err := quicChannel(qc, st)
	if err != nil {
		closeConn()
		return nil, err
	}
	// The listening side waits: it is the one that writes last.
	return &Conn{Channel: ch, closer: gracefulClose(qc, st, true)}, nil
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
