package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/rendezvous"
	"nib/internal/sign"
)

// ceremonyID is the ceremony identity an armed session carries: which proceeding this is,
// which hop of it, and the material every rendezvous derivation needs.
//
// **This is the import that did not exist.** Until P05.S04 `internal/server` imported neither
// `internal/ceremony` nor `internal/rendezvous`, so the whole of P04's output was reachable
// only from `nib rendezvous --self-test`. An arm knew a fingerprint, a bind, a mode and a
// transport, and nothing about the proceeding it was part of.
//
// It is OPTIONAL on the arm. A session without one is the manual and LAN path this route has
// always served, which D9 demotes rather than deletes.
type ceremonyID struct {
	inv  ceremony.Invitation
	hop  int
	gate *ceremony.CandidateGate
	// me and peer are the two ends of this hop, hex. Kept because the gate holds them
	// privately and the publish side needs its own.
	me, peer string
	// certPEM/keyPEM sign this party's candidate records. Held here because a record is
	// signed by the identity, not by the ceremony, and the publish path needs both.
	certPEM, keyPEM []byte

	// end is the ONE socket the DHT and the armed listener share, and rz is the rendezvous
	// server on its DHT view. Both nil on the TCP transport — see openRendezvous.
	end *p2p.SharedEndpoint
	rz  *rendezvous.Server
	// mu guards stopNet and portMap, both written by the armed background goroutine and read by
	// close() on the teardown goroutine. **stopNet was a live data race before S07** (grill C5):
	// written at `startArmedRendezvous`'s third statement, read by close(), with no lock and
	// nothing but a tight window keeping -race quiet.
	mu sync.Mutex
	// stopNet cancels the arm's background rendezvous work — bootstrap, the LAN wait, the
	// publish. Set by startArmedRendezvous and called by close, so the goroutine ends with
	// the session rather than outliving it. Guarded by mu.
	stopNet context.CancelFunc
	// portMap is the router port-mapping lease for this arm (S07): obtained at publish,
	// refreshed while armed, deleted by close(). nil when the tier obtained nothing. Guarded
	// by mu.
	portMap *portMapper
	// closed guards the narrow window where close() runs BEFORE the arm goroutine has stored
	// stopNet/portMap (diff-grill #4): a setter arriving after close acts immediately rather
	// than storing state nothing will ever tear down.
	closed bool
	// reDelivery caches THIS hop's co-signed output so a reconnect after the signature but before
	// the initiator read it re-delivers the SAME bytes instead of signing again (P05.S10, D18/D24).
	// Keyed on sha256 of the INBOUND (the peer's already-signed document): keying on the hop alone
	// would hand a reconnect with a different document the wrong signature. The ceremonyID is
	// already per-hop, so the hop is implicit. Guarded by mu; cleared by close, so no signed bytes
	// outlive the ceremony. In-memory only — the process-kill persistence D24 names is P08's.
	reDelivery map[string][]byte
}

// setStopNet and setPortMap store the two shared fields under the lock.
func (c *ceremonyID) setStopNet(cancel context.CancelFunc) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel() // close() already ran; do not store a canceller nothing will call
		return
	}
	c.stopNet = cancel
	c.mu.Unlock()
}

// reDeliverKey hashes the inbound document — the idempotency key for re-delivery (P05.S10).
func reDeliverKey(inbound []byte) string {
	sum := sha256.Sum256(inbound)
	return string(sum[:])
}

// Cached returns THIS hop's previously co-signed output for `inbound`, or nil if this hop has not
// signed that document (a cache miss runs the fresh exchange). Nil after close, so a re-delivery
// races a teardown to a clean miss rather than a signature off a dead ceremony.
func (c *ceremonyID) Cached(inbound []byte) []byte {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	return c.reDelivery[reDeliverKey(inbound)]
}

// Store records THIS hop's co-signed output for `inbound`, so a later reconnect re-delivers it. A
// Store arriving after close is dropped (the closed-flag pattern setStopNet/setPortMap use), so no
// signed bytes are stored on a ceremony nothing will tear down.
func (c *ceremonyID) Store(inbound, final []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.reDelivery == nil {
		c.reDelivery = map[string][]byte{}
	}
	c.reDelivery[reDeliverKey(inbound)] = final
}

// hasSigned reports whether this hop has produced any signature — the receiver's "am I past the
// gate" signal, since a lost writeback is otherwise indistinguishable from a clean completion.
func (c *ceremonyID) hasSigned() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reDelivery) > 0
}

func (c *ceremonyID) setPortMap(pm *portMapper) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		pm.close() // close() already ran; delete this mapping rather than leak it
		return
	}
	c.portMap = pm
	c.mu.Unlock()
}

// nodeCacheDir is where the DHT's node list lives.
//
// **The config directory, not `~/nib`.** A node cache is a list of IP addresses this machine
// has spoken to, and `~/nib` is the DOCUMENT output directory — the folder a small practice is
// most likely to put in Dropbox or a network share, because it is where their finished client
// files land. It is also created 0755 by the save path, so `writeNodes`' own 0700 MkdirAll is
// a no-op there and the file's existence and mtime are world-readable on a shared machine:
// a per-user signal of *this person does remote signings, most recently at T*. The config
// directory is 0700 and is where the vault already lives.
func nodeCacheDir(configDir string) string { return filepath.Join(configDir, "dht-nodes") }

// openRendezvous gives this ceremony a socket the DHT and the listener share, and returns the
// listener built on it.
//
// **QUIC only, and the limit is structural rather than an omission.** A TCP listener is a
// `net.Listener` over a different IP protocol, and a NAT keeps separate mapping tables per
// protocol — which is exactly why D15 requires both UDP and TCP to be mapped when both are
// offered. So on TCP the listener binds its own socket and this returns no endpoint: the
// ceremony still runs, and caveat 7's clause is simply not satisfied for that transport.
// Stated here rather than discovered at S06, the slice that first asks a router for a mapping.
// setupSharedEndpoint opens the ceremony's shared QUIC socket and its rendezvous WITHOUT a
// listener (P05.S09): the symmetric-racing coordinator owns the single handshaked listener, and a
// transport permits only one. It is openRendezvous's QUIC branch minus the QUICListenOn — the arm
// and the dialer now set the endpoint up the same way, and connect arms the listener on top.
func (c *ceremonyID) setupSharedEndpoint(bind, configDir string) error {
	end, err := p2p.NewSharedEndpoint(bind)
	if err != nil {
		return err
	}
	rz, err := rendezvous.Open(end.DHT(), nodeCacheDir(configDir))
	if err != nil {
		end.Close()
		return err
	}
	c.end, c.rz = end, rz
	return nil
}

func (c *ceremonyID) openRendezvous(transport, bind, configDir string, cert, key, peerFP []byte) (p2p.Listener, error) {
	if transport != transportQUIC {
		ln, err := listenPeer(transport, bind, cert, key, peerFP)
		return ln, err
	}
	end, err := p2p.NewSharedEndpoint(bind)
	if err != nil {
		return nil, err
	}
	rz, err := rendezvous.Open(end.DHT(), nodeCacheDir(configDir))
	if err != nil {
		end.Close()
		return nil, err
	}
	ln, err := p2p.QUICListenOn(end, cert, key, peerFP)
	if err != nil {
		// Teardown order matters even on the failure path: the rendezvous server first,
		// then the socket. Closing the mux while the DHT still reads its view makes that
		// read return net.ErrClosed, which anacrolix/dht turns into a panic on a goroutine
		// nothing of ours is on.
		rz.Close()
		end.Close()
		return nil, err
	}
	c.end, c.rz = end, rz
	return ln, nil
}

// close tears the ceremony's network down IN ORDER, and the order is the whole of it.
//
// rendezvous first, then the socket. Three of the six plausible orderings panic the process:
// any that closes the mux while the DHT server is still reading it. The one existing call site
// in the tree gets this right only by accident of `defer` LIFO, with nothing naming the rule.
func (c *ceremonyID) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	stop := c.stopNet
	mapper := c.portMap
	c.reDelivery = nil // drop the signed outputs with the ceremony (D6: no signed bytes at rest)
	c.mu.Unlock()
	if stop != nil {
		// Cancel the background work FIRST, so Close's own join has something finite to
		// wait for rather than a publish still holding its 45 s budget.
		stop()
	}
	// The mapping is released before the sockets close. Its delete opens its OWN socket to the
	// gateway (portmap.Client.Unmap dials fresh) on a FRESH context (grill C2), so it neither
	// needs c.end nor is cancelled by the stop() above; it joins the refresh goroutine first so
	// nothing re-creates the mapping after the delete (grill C3).
	if mapper != nil {
		mapper.close()
	}
	if c.rz != nil {
		c.rz.Close()
	}
	if c.end != nil {
		c.end.Close()
	}
}

// errNoCeremony reports an arm with no invitation — not an error, the ordinary case.
var errNoCeremony = errors.New("this session has no ceremony identity")

// ceremonyFor builds the identity from an invitation and the two parties.
//
// The hop is DERIVED, never supplied: it is the roster's own order (D22 makes connectivity a
// sequence of pairs, and Party's doc says the roster order is the signing order), read off an
// artifact both ends already hold. A hop passed in a request would be a number the two sides
// have to agree on, and there is nothing to make them.
func ceremonyFor(text string, myCertPEM, myKeyPEM []byte, peerFP []byte) (*ceremonyID, error) {
	if text == "" {
		return nil, errNoCeremony
	}
	inv, err := ceremony.ParseInvitation(text)
	if err != nil {
		return nil, fmt.Errorf("that invitation could not be read: %w", err)
	}
	myFP, err := sign.Fingerprint(myCertPEM)
	if err != nil {
		return nil, err
	}
	me := hex.EncodeToString(myFP)
	peer := hex.EncodeToString(peerFP)

	hop, err := inv.Hop(me, peer)
	if err != nil {
		// The three ways this fails are all worth distinguishing for a user, and `Hop`'s
		// own error already does: not in the roster at all, the same party twice, or two
		// parties who are not adjacent and therefore share no hop.
		return nil, fmt.Errorf("this invitation does not put you and that peer on one hop: %w", err)
	}
	gate, err := ceremony.NewCandidateGate(inv, hop, me, peer)
	if err != nil {
		return nil, err
	}
	return &ceremonyID{inv: inv, hop: hop, gate: gate, me: me, peer: peer, certPEM: myCertPEM, keyPEM: myKeyPEM}, nil
}
