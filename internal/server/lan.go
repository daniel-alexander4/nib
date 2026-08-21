package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
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

// announceable is the part of an armed listener an announcement is built from: where it
// is bound, and which kind of socket that is. Both are needed and neither is enough —
// see the Transport note on p2p.Listener.
//
// An interface narrower than p2p.Listener so a test can drive this without a session.
type announceable interface {
	Addr() net.Addr
	Transport() string
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
func startAnnouncing(myCertPEM []byte, ln announceable) (*lanAnnouncer, error) {
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
	// The transport is ASKED OF THE LISTENER, never passed alongside it: a port and a
	// transport carried separately are two facts that can disagree, and the listener
	// is the only thing that knows which socket it actually opened (ADR-010).
	ann := discovery.Announcement{
		Name:      name,
		Port:      uint16(port),
		Transport: announcedTransport(ln.Transport()),
		Nonce:     nonce,
	}

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

// announcedTransport maps p2p's transport name onto the announcement's wire byte. The
// inverse of transportOf, and here for the same reason: internal/discovery may not import
// internal/p2p, so this layer owns the join.
func announcedTransport(t string) discovery.Transport {
	if t == p2p.TransportQUIC {
		return discovery.TransportQUIC
	}
	return discovery.TransportTCP
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
func findPeerOnLAN(v *vault.Vault, peerFP []byte) ([]candidate, error) {
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
	// The CANDIDATES, not their address strings. Flattening to []string here threw
	// away the announced transport, so the dialer fell back to whatever the caller's
	// own request said and a QUIC-armed peer was dialled over TCP (ADR-010).
	found := browsePeers(sock, pins, browseWindow)
	if len(found) == 0 {
		return nil, fmt.Errorf("%w (listened for %s on %v)", errNoPeerOnTheLink, browseWindow, sock.Interfaces())
	}
	return found, nil
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
func (s *Server) peerAddresses(w http.ResponseWriter, v *vault.Vault, address, transport string, peerFP []byte) ([]candidate, bool) {
	if address != "" {
		// A typed address carries no announcement, so the REQUEST names the
		// transport — the only case where it still does. It is validated here rather
		// than left to the dialer so a typo is refused before a browse or a dial,
		// with the 400 it deserves instead of a 502 about an unreachable peer.
		if err := checkTransport(transport); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return nil, false
		}
		return []candidate{{Addr: address, Transport: transport}}, true
	}
	found, err := findPeerOnLAN(v, peerFP)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return nil, false
	}
	return found, true
}

// lanDialTimeout is per candidate. Link-local candidates answer in milliseconds, so six
// seconds is already generous and anything longer is only ever spent on something that is
// not there. The whole-race budget is `connectDeadline` (D16); this is the floor under one
// dial inside it.
//
// Its sibling `lanDialBudget` — a total budget for the WHOLE walk — was deleted at P05.S03
// along with the walk it bounded. The racer needs no such cut-short: N dead candidates cost
// one timeout rather than N, so concurrency is the bound.
const lanDialTimeout = 6 * time.Second

// raceCandidates dials every candidate concurrently and returns the first that answers as
// the pinned peer, closing the rest.
//
// # Why a race and not a walk (D8, D16)
//
// The walk this replaces tried candidates in slice order, blocking, one at a time. D8's
// ladder attempts every tier at once and takes the first to complete; D16 adds that
// candidates **join as they arrive** and that gathering never blocks the race. A walk cannot
// express either: a dead candidate delays a live one behind it by a full `lanDialTimeout`,
// and a candidate that arrives after the walk began is simply not in the slice.
//
// # Losers are CLOSED, and that is correctness rather than hygiene
//
// A connection the racer abandons but leaves OPEN is accepted by the peer's serial loop and
// blocks it inside `p2p.Receive`'s verification read for the full `exchangeDeadline` — six
// minutes, four past this side's own connect deadline — while the winner sits queued. P05.S02
// did not change this: it fixed the handshake pool, and `runSession` still runs
// `serveOneSession` inline. A **closed** loser fails that read at once, so closing is both
// necessary and sufficient, and it is what makes the abandoned connection invisible to the
// peer's user (`runVerification` does the wire exchange before it shows anybody anything).
//
// Closing is therefore initiated at the moment of decision — but not serially in front of the
// winner. A QUIC close is a goroutine round-trip (`tr.Close()` joins quic-go's read loop), so
// N−1 closes on the return path would be a new head-of-line block on the request racing was
// meant to speed up. Each loser closes in its own goroutine.
//
// # The context governs the DIAL and nothing after it
//
// Both libraries detach an established connection from its dial context — `crypto/tls`
// documents it, and quic-go passes `context.WithoutCancel(ctx)` into the connection. So the
// winner is safe from the cancel that stops the losers, and `raceCandidates` may cancel
// everything as it returns. The parent is `context.Background()` and NOT the request's:
// `buildCoSigned` runs before the dial, so by the time a race starts the local user has
// already signed, and a closed browser tab must not be able to cancel a ceremony carrying
// that signature the day someone plumbs a context into `Initiate`.
// **The caller owns the deadline, and that is not a style choice.** The first version made
// its own from `connectDeadline` and then read results with `for r := range results` — which
// ends only when the INPUT channel closes. A trickle source stays open for the whole race by
// design (D16: candidates join as they arrive), so if every candidate failed, this function
// never returned: each dial was bounded and the race was not, and an HTTP handler hung
// forever. Found by reviewing this slice's own diff. The deadline is now the caller's
// context and the loop watches it.
func raceCandidates(parent context.Context, in <-chan candidate, cert, key, peerFP []byte) (*p2p.Conn, error) {
	// A cancellable child: cancelling on a win is what stops the losers, and it must not
	// disturb the caller's context.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	type result struct {
		conn *p2p.Conn
		err  error
	}
	// Buffered to the candidate cap so a late winner never blocks on a send nobody is
	// reading — the shape `tlsListener` needed for the same reason.
	results := make(chan result, maxRaceCandidates)
	sem := make(chan struct{}, maxConcurrentDials)

	var wg sync.WaitGroup
	// Keyed on ADDRESS AND TRANSPORT. ADR-010 exists because a port without its transport
	// names two different sockets, so deduping on the address alone silently drops one of
	// them — and which one depends on arrival order.
	seen := map[string]bool{}
	var started int
	var dropped atomic.Int64

	go func() {
		defer func() { wg.Wait(); close(results) }()
		for c := range in {
			// `ck`, not `key` — the identity key is a parameter of this function and
			// shadowing it here compiled into a type error rather than a silent bug,
			// which is luck rather than design.
			ck := c.Addr + "\x00" + c.Transport
			if seen[ck] {
				continue
			}
			seen[ck] = true
			if started >= maxRaceCandidates {
				// **Dropped AND reported** — D16's pin says "drops and reports; it never
				// fails the ceremony", and a drop with no reader is half the clause. The
				// count reaches the user in the failure sentence below, which is the only
				// surface a dialing-side diagnostic has until S11 builds the tier report.
				dropped.Add(1)
				continue
			}
			started++
			wg.Add(1)
			go func(c candidate) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				conn, err := dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, peerFP, lanDialTimeout)
				results <- result{conn, err}
			}(c)
		}
	}()

	var last error
	var tried int
	for {
		var r result
		select {
		case got, ok := <-results:
			if !ok {
				// Every dial finished and the input closed: the race is exhausted.
				return nil, raceFailure(tried, dropped.Load(), last)
			}
			r = got
		case <-parent.Done():
			// **The connect deadline, or the caller giving up.** Without this arm the
			// loop waits on a channel that a trickle source keeps open forever.
			return nil, raceFailure(tried, dropped.Load(), context.Cause(parent))
		}
		tried++
		if r.err != nil {
			if errors.Is(r.err, errUnknownTransport) {
				return nil, r.err // a caller error, not a peer's
			}
			// **A clock-skew refusal outranks whatever else lost.** `connectFailure` lifts
			// `*p2p.ClockSkewError` out with `errors.As` and reports D19's fifth cause,
			// naming the direction and size of the disagreement. Under a walk the last
			// error was the only error; under a race "last" is whichever goroutine
			// happened to finish last, so a skewed peer reachable at one address and
			// skewed at another would report a generic failure roughly half the time.
			// Cause 5 is actionable — check the clock — and cause 4 is not, so the
			// specific one wins.
			var skew *p2p.ClockSkewError
			if last == nil || errorsAs(r.err, &skew) {
				last = r.err
			}
			continue
		}
		// A winner. Cancel the rest and drain what is already in flight, closing each in
		// its own goroutine — see the note above on why not serially.
		cancel()
		go func() {
			for extra := range results {
				if extra.conn != nil {
					go extra.conn.Close()
				}
			}
		}()
		return r.conn, nil
	}
}

// raceFailure is the one sentence a lost race produces, so the two exits above cannot
// describe the same outcome differently.
func raceFailure(tried int, dropped int64, last error) error {
	if last == nil {
		last = errors.New("no candidate addresses")
	}
	if dropped > 0 {
		return fmt.Errorf("tried %d address(es) and dropped %d over the %d-candidate cap, "+
			"none answered as the pinned peer: %w", tried, dropped, maxRaceCandidates, last)
	}
	return fmt.Errorf("tried %d address(es), none answered as the pinned peer: %w", tried, last)
}

// errorsAs is errors.As, named so the preference above reads as one condition rather than
// two statements. It exists only to keep that `if` honest.
func errorsAs(err error, target any) bool { return errors.As(err, target) }

// dialAny races a fixed set of candidates. It is raceCandidates with the trickle removed,
// for the callers that already hold every candidate they will ever have.
func dialAny(cands []candidate, cert, key, peerFP []byte) (*p2p.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectDeadline)
	defer cancel()
	in := make(chan candidate, len(cands))
	for _, c := range cands {
		in <- c
	}
	close(in)
	return raceCandidates(ctx, in, cert, key, peerFP)
}
