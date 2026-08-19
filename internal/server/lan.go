package server

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"nib/internal/discovery"
	"nib/internal/pairing"
	"nib/internal/sign"
	"nib/internal/vault"
)

// announceEvery is how often an armed session repeats its announcement.
//
// Repetition rather than a single shout, because the browsing side may not have joined
// the group yet: it browses for D16's 2 s, and a peer that announced once just before
// that window opened is invisible for the whole of it. Half a second gives a browse
// four chances inside its budget while putting almost nothing on the link.
const announceEvery = 500 * time.Millisecond

// lanAnnouncer is the armed side's presence on the link. It exists only while a session
// is armed — that is the whole of what the plan's egress enumeration authorises, and it
// is also what keeps the exposure small: a Nib that is not expecting anybody says
// nothing at all.
type lanAnnouncer struct {
	sock  *discovery.Socket
	stop  chan struct{}
	done  chan struct{}
	sent  atomic.Uint64
	fails atomic.Uint64
}

// startAnnouncing begins announcing this user's name and the given port.
//
// **It never fails the session.** A host with no usable interface, or a firewall that
// swallows the group, must still be able to run a ceremony over a typed address — the
// LAN tier is the first rung of D8's ladder, not a prerequisite for the others. So the
// error is returned for a diagnostic to report and the caller carries on.
func startAnnouncing(myCertPEM []byte, port int) (*lanAnnouncer, error) {
	name, err := ownName(myCertPEM)
	if err != nil {
		return nil, err
	}
	if port <= 0 || port > 0xffff {
		return nil, fmt.Errorf("cannot announce port %d", port)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	sock, err := discovery.Open(nonce)
	if err != nil {
		return nil, err
	}
	a := &lanAnnouncer{sock: sock, stop: make(chan struct{}), done: make(chan struct{})}
	ann := discovery.Announcement{Name: name, Port: uint16(port), Nonce: nonce}

	go func() {
		defer close(a.done)
		t := time.NewTicker(announceEvery)
		defer t.Stop()
		for {
			if n, err := sock.Announce(ann); err != nil || n == 0 {
				a.fails.Add(1)
			} else {
				a.sent.Add(uint64(n))
			}
			select {
			case <-a.stop:
				return
			case <-t.C:
			}
		}
	}()
	return a, nil
}

// Close stops announcing and releases the socket. Idempotent, because the accept
// goroutine's defer and an explicit disarm can both reach it.
func (a *lanAnnouncer) Close() {
	if a == nil {
		return
	}
	select {
	case <-a.stop:
	default:
		close(a.stop)
	}
	<-a.done
	a.sock.Close()
}

// Sent and Failed report what the announcer did, for a diagnostic that has to explain
// silence. Sent counts per-interface writes, so one announcement on a two-interface
// host adds two.
func (a *lanAnnouncer) Sent() uint64   { return a.sent.Load() }
func (a *lanAnnouncer) Failed() uint64 { return a.fails.Load() }

// ownName is this user's six-word pairing name, derived from their identity — the
// only thing about themselves an announcement ever carries.
func ownName(certPEM []byte) (string, error) {
	fp, err := sign.Fingerprint(certPEM)
	if err != nil {
		return "", err
	}
	return pairing.Name(fp)
}

// errNoPeerOnTheLink reports a browse that heard nothing useful.
var errNoPeerOnTheLink = errors.New("that peer is not announcing on this network")

// findPeerOnLAN browses the link for a pinned peer and returns an address to dial.
//
// This is tier 1 of D8's ladder and it is deliberately the whole of it for now: the
// browse either produces a candidate inside D16's 2 s budget or it does not, and the
// caller reports that rather than falling through to a tier that does not exist yet.
//
// The returned address is the peer's OBSERVED source plus its ANNOUNCED port; the
// fingerprint that will be pinned at the handshake is the one from the vault. An
// announcer who lies reaches a TLS handshake that rejects it, which is L1's promise
// spent exactly where it was meant to be.
func findPeerOnLAN(v *vault.Vault, peerFP []byte) (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	sock, err := discovery.Open(nonce)
	if err != nil {
		return "", fmt.Errorf("could not listen on this network: %w", err)
	}
	defer sock.Close()

	// Only the peer being dialled. Browsing against every pin would return candidates
	// the caller did not ask for, and resolving is the only place a name becomes an
	// identity — so the narrower the pin list, the narrower that step.
	var pins []vault.PinnedPeer
	for _, p := range v.PinnedPeers() {
		if bytes.Equal(p.Fingerprint, peerFP) {
			pins = append(pins, p)
		}
	}
	if len(pins) == 0 {
		return "", errors.New("that peer isn't pinned")
	}

	for _, c := range browsePeers(sock, pins, browseWindow) {
		return c.Addr, nil
	}
	return "", fmt.Errorf("%w (listened for %s on %v)", errNoPeerOnTheLink, browseWindow, sock.Interfaces())
}

// portOf reads the bound port off a listener, for the announcement to carry.
func portOf(ln interface{ Addr() net.Addr }) int {
	if ln == nil {
		return 0
	}
	a, ok := ln.Addr().(*net.UDPAddr)
	if ok {
		return a.Port
	}
	if t, ok := ln.Addr().(*net.TCPAddr); ok {
		return t.Port
	}
	return 0
}

// peerAddress returns the address to dial, browsing the link when none was supplied.
//
// It writes the HTTP error itself and reports ok=false, matching the shape of the other
// helpers on these routes. The distinction it preserves is worth stating: "you did not
// tell me where they are and they are not on this network" is a different message from
// "I could not reach the address you gave me", and a user can act on each differently —
// the first says try the manual path, the second says check the address.
func (s *Server) peerAddress(w http.ResponseWriter, v *vault.Vault, address string, peerFP []byte) (string, bool) {
	if address != "" {
		return address, true
	}
	found, err := findPeerOnLAN(v, peerFP)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return "", false
	}
	return found, true
}
