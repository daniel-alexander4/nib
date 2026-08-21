package server

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"nib/internal/addrscope"
	"nib/internal/discovery"
	"nib/internal/p2p"
	"nib/internal/pairing"
	"nib/internal/safe"
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
	sock *discovery.Socket
	stop chan struct{}
	done chan struct{}
	// once, not a check-then-close. The doc on Close claims idempotence because two
	// callers can reach it, and a select/default pair is exactly the shape where two
	// concurrent callers both take default and the second close() panics. Socket.Close
	// in the same phase already used sync.Once, so the right shape was to hand.
	once sync.Once
}

// errLoopbackBind reports a listener that is not reachable from the link, so there is
// nothing truthful to announce about it.
var errLoopbackBind = errors.New("the armed listener is bound to loopback, so it is not announced")

// startAnnouncing begins announcing this user's name and the port ln is bound to.
//
// **It never fails the session.** A host with no usable interface, or a firewall that
// swallows the group, must still be able to run a ceremony over a typed address — the
// LAN tier is the first rung of D8's ladder, not a prerequisite for the others. So the
// error is returned for a diagnostic to report and the caller carries on.
//
// # It takes the LISTENER, not a port, and that is the fix
//
// It used to take an int, so the only fact it could check was that the number was in
// range. `runSession` passed `portOf(ln)` and nothing anywhere looked at the HOST that
// port belongs to — so arming with `{"bind":"127.0.0.1:8443"}` broadcast *armed on 8443*
// on every joined interface every 500 ms while the listener answered only on loopback.
// Two harms, and the second is the one that does not announce itself: a peer on the link
// resolves that to `<our LAN address>:8443` and cannot connect, and a user who
// deliberately bound loopback has their presence — a stable six-word name derived from a
// never-rotating fingerprint — put on every attached segment anyway.
//
// The rule lives HERE rather than at the call site because this is the door (ADR-009).
// A guard on `runSession` would say nothing about the second caller, and the ladder's
// later tiers are exactly where a second caller comes from.
func startAnnouncing(myCertPEM []byte, ln interface{ Addr() net.Addr }) (*lanAnnouncer, error) {
	name, err := ownName(myCertPEM)
	if err != nil {
		return nil, err
	}
	if ln == nil {
		return nil, errors.New("cannot announce a nil listener")
	}
	// Before the port, because a loopback bind is a deliberate choice and not an error:
	// the caller carries on either way, and the two want different words in a diagnostic.
	//
	// The listener's RESOLVED address, never the requested bind string — `0.0.0.0:0` is
	// what the LAN path asks for and the kernel is what says which port it got.
	if addrscope.Loopback(ln.Addr().String()) {
		return nil, fmt.Errorf("%w (%s)", errLoopbackBind, ln.Addr())
	}
	port := portOf(ln)
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
		// safe.Recover, like every other detached goroutine in this package. runSession's
		// recover does not cover the goroutine it spawns, and an unrecovered panic in ANY
		// goroutine kills the whole desktop process with the user's unsaved documents —
		// the outcome safe.Recover's own doc says it exists to prevent. This was the one
		// `go func` in internal/server without it.
		defer safe.Recover("lan announcer")
		t := time.NewTicker(announceEvery)
		defer t.Stop()
		for {
			// The outcome is counted by discovery.Socket.Stats() — Sent and SendErrors,
			// at the same moments — and printed by `nib discover`. A second pair here
			// counted the same fact and nothing read it; see below.
			_, _ = sock.Announce(ann)
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
	a.once.Do(func() { close(a.stop) })
	<-a.done
	a.sock.Close()
}

// Sent and Failed are GONE, and the deletion is the fix rather than a reader being added.
//
// They had zero callers anywhere, production or test, and their doc said they existed "for
// a diagnostic that has to explain silence" — a diagnostic that did not exist. That made
// them the asymmetric-pair defect at its worst: "announced fine" and "could not announce"
// both unread, so a default-deny host whose firewall swallows every multicast write is
// indistinguishable from one that announced successfully, which is exactly the failure
// CONTRIBUTING's tier-5 section says happens "with no error at either end".
//
// They are deleted rather than wired up because `discovery.Socket.Stats()` already counts
// the same two events at the same moments (Sent / SendErrors, mcast.go), and `nib discover`
// already prints them. Adding a reader here would have made two counters for one fact —
// the duplicate-derivation defect — where one already has a reader.

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
func findPeerOnLAN(v *vault.Vault, peerFP []byte) ([]string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	sock, err := discovery.Open(nonce)
	if err != nil {
		return nil, fmt.Errorf("could not listen on this network: %w", err)
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
		return nil, errors.New("that peer isn't pinned")
	}

	// EVERY address, in the order they were heard. One peer can legitimately announce
	// from two addresses (dual stack), and two different hosts can claim one name —
	// the name is public. Returning only the first meant an attacker who announced
	// faster than the real peer denied the tier outright, with the genuine address
	// discarded where no caller could reach it.
	var addrs []string
	for _, c := range browsePeers(sock, pins, browseWindow) {
		addrs = append(addrs, c.Addr)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w (listened for %s on %v)", errNoPeerOnTheLink, browseWindow, sock.Interfaces())
	}
	return addrs, nil
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
func (s *Server) peerAddresses(w http.ResponseWriter, v *vault.Vault, address string, peerFP []byte) ([]string, bool) {
	if address != "" {
		return []string{address}, true
	}
	found, err := findPeerOnLAN(v, peerFP)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return nil, false
	}
	return found, true
}

// lanDialBudget bounds the WHOLE walk, not each hop.
//
// `sessionDialTimeout` bounds one dial; nothing bounded the loop. With N candidates that is
// N×30 s inside an HTTP handler on a server with no WriteTimeout, and N was unbounded until
// `maxLANCandidates` — see browsePeers, where an on-link host produces a fresh candidate per
// datagram by varying the announced port. Capping N was the other half; this is the half
// that holds even if the cap is later raised, because it bounds the thing the user
// experiences (a wedged request) rather than the thing the attacker controls (a count).
const (
	lanDialBudget = 20 * time.Second
	// lanDialTimeout is per candidate, and it is much shorter than sessionDialTimeout on
	// purpose. That constant (30 s) is sized for an address a user TYPED — a peer across
	// the internet behind a slow path, where patience is the whole point. These candidates
	// are link-local: the peer is on the same segment and answers in milliseconds, so six
	// seconds is already generous and thirty is only ever spent on something that is not
	// there. It is the count multiplier in the attack above.
	lanDialTimeout = 6 * time.Second
)

// dialAny tries each candidate in order and returns the first connection that
// establishes. Trying only the first was the defect: on a link where anything else
// announces the same name, the genuine peer may not be first.
func dialAny(transport string, addrs []string, cert, key, peerFP []byte) (*p2p.Conn, error) {
	var last error
	deadline := time.Now().Add(lanDialBudget)
	var tried int
	for _, a := range addrs {
		if tried > 0 && time.Now().After(deadline) {
			// `tried > 0` so a slow clock or a long TLS handshake can never make this
			// return without having dialled anything at all.
			last = fmt.Errorf("gave up after %v and %d address(es): %w", lanDialBudget, tried, last)
			break
		}
		tried++
		conn, err := dialPeerWithin(transport, a, cert, key, peerFP, lanDialTimeout)
		if err == nil {
			return conn, nil
		}
		if errors.Is(err, errUnknownTransport) {
			return nil, err // a caller error, not a peer's; do not try the rest
		}
		last = err
	}
	if last == nil {
		last = errors.New("no candidate addresses")
	}
	return nil, fmt.Errorf("tried %d address(es), none answered as the pinned peer: %w", len(addrs), last)
}
