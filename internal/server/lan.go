package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
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

// lanAnnounceWindow caps how long the armed side ANNOUNCES on the link, independently of how long it
// LISTENS (P05.S09b). S09b extends a ceremony arm's listen window to ceremony scope — up to D33's
// 30-day MaxCeremonyLife — so a multi-party signer is not disarmed before the baton arrives. Left
// coupled, this 500ms ticker would then broadcast a never-rotating six-word name for 30 days:
// 2/s × 86400 × 30 ≈ 5.2M multicast datagrams per ceremony, the D6 privacy harm errLoopbackBind
// exists against and a criterion-14 violation ("nothing emits at full rate for the whole deadline").
// So the LAN fast-path stays what it effectively is today — five minutes, ~600 datagrams — and a
// baton that arrives later is discovered over the DHT, which the connect arm feeds for the whole
// window. The socket is released when this closes, not held idle until disarm.
const lanAnnounceWindow = 5 * time.Minute

// hopAnnounceWindow is how long the DIALLING side announces itself while a ceremony hop is in
// flight (P07.S05c).
//
// **Its whole job is to be heard by one browse, so it is measured against that browse and not
// against the arm.** `findPeerOnLAN` listens for `browseWindow` — two seconds — and closes; an
// announcement that outlives the browse it was sent for is a standing beacon by another name,
// which is exactly what `lanAnnounceWindow` above refuses at scale. Ten seconds is five browses'
// worth of margin for a scheduler, and it ends when the hop does either way: the caller closes the
// announcer on return.
//
// It is a variable rather than a constant so a test can drive a hop whose window has ALREADY
// expired, which is the only case this mechanism exists for — a hop inside the window passes
// without exercising anything, and a test that waited five real minutes for the interesting case
// would not be run.
var hopAnnounceWindow = 10 * time.Second

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
func startAnnouncing(myCertPEM []byte, ln announceable, window time.Duration) (*lanAnnouncer, error) {
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
		// safe.Recover, like every other detached goroutine in this package. runSession's
		// recover does not cover the goroutine it spawns, and an unrecovered panic in ANY
		// goroutine kills the whole desktop process with the user's unsaved documents —
		// the outcome safe.Recover's own doc says it exists to prevent.
		//
		// **This used to claim it was the one `go func` in the package without it.** That
		// was true when written and false from P05.S03, which added four more and gave none
		// of them a recover — a sentence that describes a census nobody re-ran. The census
		// is now a guard: TestEveryDetachedGoroutineIsRecovered.
		//
		// **Deferred FIRST, so it runs LAST** — safe.Recover's own doc says "at the very
		// top" and that the goroutine's other defers "still run as the stack unwinds".
		// `defer close(a.done)` used to sit above it, which inverted that: behaviourally
		// identical here, but it made the package's one stated ordering rule false at one
		// of the two sites that had a recover at all.
		defer safe.Recover("lan announcer")
		defer close(a.done)
		t := time.NewTicker(announceEvery)
		defer t.Stop()
		// The LAN fast-path window (P05.S09b T02). It bounds ANNOUNCING, never listening: when it
		// fires the socket is released and this returns, while the arm keeps accepting and dialing
		// over the DHT for the rest of the (now ceremony-scoped) window.
		windowC := time.After(window)
		for {
			// The outcome is counted by discovery.Socket.Stats() — Sent and SendErrors,
			// at the same moments — and printed by `nib discover`. A second pair here
			// counted the same fact and nothing read it; see below.
			_, _ = sock.Announce(ann)
			select {
			case <-a.stop:
				return
			case <-windowC:
				a.sock.Close() // release the LAN presence; Close's own sock.Close is idempotent
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
		return []candidate{{Addr: address, Transport: transport, Source: sourceTyped}}, true
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
// racedConn is what the racer needs of a won connection: to be closed if it loses the drain. The
// racer is generic over it so ONE implementation serves both the stream-opened dial (*p2p.Conn,
// the manual/co-sign/send paths) and the handshaked dial the symmetric-racing coordinator needs
// (*p2p.HandshakedConn, P05.S09), rather than a second copy of its cap/dedup/trickle/drain logic
// (ADR-009).
type racedConn interface{ Close() error }

func raceCandidates[T racedConn](parent context.Context, in <-chan candidate, dial func(context.Context, candidate) (T, error)) (T, error) {
	var zero T
	// A cancellable child: cancelling on a win is what stops the losers, and it must not
	// disturb the caller's context.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	type result struct {
		conn T
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
	startedBySource := map[candidateSource]int{}
	var dropped atomic.Int64
	var droppedBySource sync.Map // candidateSource -> *atomic.Int64

	go func() {
		// **First, before anything else defers**, so it runs last and catches a panic
		// after `close(results)` has already released the consumer. P05.S03 added four
		// goroutines to this file with no recover, under a comment two hundred lines up
		// asserting there were none left — and an unrecovered panic in ANY goroutine
		// takes the desktop process with the user's unsaved documents.
		defer safe.Recover("race feeder")
		defer func() { wg.Wait(); close(results) }()
		for {
			var c candidate
			// **The ctx arm is what lets this goroutine end on a WIN.**
			//
			// This used to be `for c := range in`, which ends only when the INPUT channel
			// closes. `raceCandidates` cancels on a win but cannot close `in` — it is the
			// caller's channel — so against a trickle source (D16: candidates join as they
			// arrive) the feeder blocked here forever, its `close(results)` never ran, and
			// the drain goroutine below never returned: two goroutines and everything they
			// reference, leaked per race. `dialAny` hid it by closing the channel it builds,
			// and S03's trickle test drove only the LOSS path, where the deadline ends the
			// race anyway. Found by grilling P05.S04, which is the slice that first feeds
			// this from a source that stays open for the whole connect deadline.
			select {
			case got, ok := <-in:
				if !ok {
					return
				}
				c = got
			case <-ctx.Done():
				return
			}
			// `ck`, not `key` — the identity key is a parameter of this function and
			// shadowing it here compiled into a type error rather than a silent bug,
			// which is luck rather than design.
			ck := raceKey(c)
			if seen[ck] {
				continue
			}
			seen[ck] = true
			// **Per source first, then the global law.** The global figure is D16's and is
			// law; the per-source figure is what stops one source spending it. Checking the
			// global one alone is first-come, which is won by whoever emits fastest — the
			// capture attack `maxLANCandidates` closed at the browse level, re-opened one
			// layer up the moment a second source exists.
			if startedBySource[c.Source] >= maxCandidatesPerSource || started >= maxRaceCandidates {
				// **Dropped AND reported** — D16's pin says "drops and reports; it never
				// fails the ceremony", and a drop with no reader is half the clause. The
				// count reaches the user in the failure sentence below, which is the only
				// surface a dialing-side diagnostic has until S11 builds the tier report.
				//
				// Counted per source as well as in total: a lumped counter reads backwards
				// exactly when it matters, because "16 dropped" cannot say which tier was
				// starving the other.
				dropped.Add(1)
				n, _ := droppedBySource.LoadOrStore(c.Source, &atomic.Int64{})
				n.(*atomic.Int64).Add(1)
				continue
			}
			started++
			startedBySource[c.Source]++
			wg.Add(1)
			go func(c candidate) {
				defer safe.Recover("race dial")
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				conn, err := dial(ctx, c)
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
				return zero, raceFailure(tried, dropped.Load(), dropReport(&droppedBySource), last)
			}
			r = got
		case <-parent.Done():
			// **The connect deadline, or the caller giving up.** Without this arm the
			// loop waits on a channel that a trickle source keeps open forever.
			return zero, raceFailure(tried, dropped.Load(), dropReport(&droppedBySource), context.Cause(parent))
		}
		tried++
		if r.err != nil {
			if errors.Is(r.err, errUnknownTransport) {
				return zero, r.err // a caller error, not a peer's
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
			defer safe.Recover("race drain")
			for extra := range results {
				if extra.err == nil {
					// Wrapped in a literal rather than `go extra.conn.Close()`: a bare
					// `go f()` has nowhere to hang a defer, so the recover has nowhere to
					// go — and a Close reaching into crypto/tls or quic-go is exactly the
					// call whose panic this is protecting the process from.
					c := extra.conn
					go func() {
						defer safe.Recover("race loser close")
						c.Close()
					}()
				}
			}
		}()
		return r.conn, nil
	}
}

// raceFailure is the one sentence a lost race produces, so the two exits above cannot
// describe the same outcome differently.
//
// `bySource` names WHICH tier was capped, because "dropped 8" cannot say whether the
// local network flooded the budget or the rendezvous did — and under D6 an attacker
// supplies one of them. The per-source counters exist to be read here; a split nobody
// reports is the same as no split.
func raceFailure(tried int, dropped int64, bySource string, last error) error {
	if last == nil {
		last = errors.New("no candidate addresses")
	}
	if dropped > 0 {
		return fmt.Errorf("tried %d address(es) and dropped %d over the cap (%s), "+
			"none answered as the pinned peer: %w", tried, dropped, bySource, last)
	}
	return fmt.Errorf("tried %d address(es), none answered as the pinned peer: %w", tried, last)
}

// dropReport renders the per-source drop counts in a stable order, so one race's sentence
// can be compared with another's.
func dropReport(m *sync.Map) string {
	var parts []string
	for _, src := range allCandidateSources() {
		if v, ok := m.Load(src); ok {
			if n := v.(*atomic.Int64).Load(); n > 0 {
				parts = append(parts, fmt.Sprintf("%d from %s", n, src))
			}
		}
	}
	if len(parts) == 0 {
		return "source unknown"
	}
	return strings.Join(parts, ", ")
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
	return raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
		return dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, peerFP, lanDialTimeout, nil)
	}) // no ceremony: fresh sockets
}

// raceKey is the identity of a race candidate: one endpoint, one key.
//
// **The address is normalised through netip and the raw string is not the key.** An IPv6
// address has many spellings for one endpoint — `[2001:db8::1]:443`, `[2001:DB8::1]:443` and
// `[2001:db8:0:0:0:0:0:1]:443` are the same host — and string equality makes them three
// candidates. This is the only place candidates are compared, so a peer publishing one
// address in three spellings burnt three of `maxRaceCandidates` and three of its per-source
// allowance, on a budget whose whole purpose is to bound what one source can spend.
//
// **The zone is kept, deliberately.** It is not noise here the way it is in a candidate
// record: LAN discovery's `resolve` builds `fe80::…%wlp1s0` from the arrival interface
// because a link-local address is not dialable without it, and `%wlp1s0` and `%eth0` on the
// same link-local bytes are genuinely different endpoints. Normalising it away would merge
// two peers into one. (A zone on a GLOBAL address cannot get this far — `addrscope.Target`
// refuses it at both candidate-record doors — so nothing arrives here wearing a meaningless
// one.)
//
// An unparseable address keeps its raw string, because the typed-address source takes what
// the user wrote and that may be a hostname. Two spellings of one hostname stay two keys,
// which is the pre-existing behaviour and costs at most a duplicate dial the handshake
// settles; inventing a resolution here would put DNS on the dedupe path.
func raceKey(c candidate) string {
	addr := c.Addr
	if ap, err := netip.ParseAddrPort(addr); err == nil {
		addr = ap.String()
	}
	return addr + "\x00" + c.Transport
}

// answerHopSeekers is the armed side of P07.S05c: it listens for the peer this session is armed
// for, and re-announces when it hears them.
//
// # Why this exists, and why it is the cheap half
//
// A ceremony arm outlives its own announcement by up to thirty days. `lanAnnounceWindow` above
// caps the announcement at five minutes and says why — keeping the 500 ms ticker for the arm's
// life is ≈5.2M multicast datagrams per ceremony — so from the fourth party onward a same-room
// ceremony silently runs over the public DHT, which is the leak D6 exists against.
//
// The plan's own sentence for this was *"the LAN tier is re-announced when the convener's dial for
// hop k begins"*, and no such mechanism can exist: `startAnnouncing` runs on the armed side and
// `browsePeers` only listens, so a dialler has nothing it can send that makes a remote peer speak.
//
// **What settles the design is an asymmetry rather than a trade-off: listening is free.** Every
// beacon shape pays per datagram; a browse held open pays nothing at all. So the party who must be
// found listens indefinitely, and the party who knows a hop is starting sends one bounded burst
// (`hopAnnounceWindow`, on the dialling side). This is the listening half.
//
// # Why it answers ONE fingerprint, and why that is what stops an amplification loop
//
// It answers only sightings that resolve to `peerFP` — the peer this arm was raised for. Under
// D22 a ceremony is a hub, so for a party that is the convener and nobody else, and the convener
// is dialling rather than armed-and-browsing. Two armed parties therefore never answer each
// other, which is the loop a "re-announce on hearing any pinned peer" rule would create: A hears
// B, answers; B hears A, answers; forever, between two machines with nothing to say.
//
// The rate limit is the second half of the same argument and is not redundant with it: a convener
// that re-dials, or a link that duplicates a datagram, must not start a second announcer while the
// first is live.
//
// # L1 is untouched
//
// An answer discloses exactly what this party's own arm announcement already discloses — the
// six-word name and the port. `Matches(fp, name)` recomputes the name from a fingerprint the
// receiver ALREADY HOLDS, and `pairing.decode` is unexported precisely so a name never resolves to
// a pin. So a stranger's announcement cannot make this party answer, and an answer cannot tell a
// stranger anything they could not already have heard.
// `wanted` reports whether this arm still wants to be found. Once the hop has signed, a peer that
// reaches us can only be re-delivering and already holds the address, so answering would put a
// stale candidate on the link for the next ceremony's browse to pick up (/pending 300).
func (s *Server) answerHopSeekers(ctx context.Context, cert []byte, ln announceable, peerFP []byte, wanted func() bool, sighted, watching func(time.Time)) {
	v := s.unlockedVault()
	if v == nil {
		return
	}
	var pins []vault.PinnedPeer
	for _, p := range v.PinnedPeers() {
		if bytes.Equal(p.Fingerprint, peerFP) {
			pins = append(pins, p)
		}
	}
	if len(pins) == 0 {
		return // nothing to recognise, so nothing to answer
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return
	}
	sock, err := discovery.Open(nonce)
	if err != nil {
		return // no usable interface: the DHT and the accept still race, as everywhere else here
	}
	defer sock.Close()
	// The watch has begun. Recorded AFTER the socket opens and the pins are known, because an arm
	// that could not open a socket is not watching the link and must not hold the DHT tier on a
	// silence it was never in a position to hear (S05e).
	if sighted != nil {
		watching(time.Now())
	}

	var answering *lanAnnouncer
	defer func() {
		if answering != nil {
			answering.Close()
		}
	}()
	// `stop` runs on EVERY loop iteration, not only when a sighting resolves — which is the whole
	// point of it being separate from the answer callback. An answer runs for `hopAnnounceWindow`,
	// and once the hop has signed nothing is announcing to us any more, so a check that only fired
	// on a resolved sighting would never fire at all: measured, the last party of a relay went on
	// advertising for the full window after signing, and was still doing so on a second probe four
	// seconds later. Closing the arm announcer alone took six stale candidates to two; this takes
	// the two.
	stop := func() {
		if answering != nil {
			answering.Close()
			answering = nil
		}
	}
	answerLoop(ctx, sock, pins, time.Now, wanted, stop, sighted, func(candidate) bool {
		ann, aerr := startAnnouncing(cert, ln, hopAnnounceWindow)
		if aerr != nil {
			return false // loopback bind or no interface — never fatal to the arm
		}
		if answering != nil {
			answering.Close()
		}
		answering = ann
		return true
	})
}

// answerLoop is answerHopSeekers' POLICY, separated from its socket so it can be driven.
//
// **The separation is the point, not tidiness.** The case this whole mechanism exists for is a
// peer arriving AFTER the arm's own five-minute announcement has expired — and every hop of every
// run so far falls inside that window, so the mechanism shipped with nothing exercising the only
// state it was built for. That is the vacuous-green shape: code that is correct, wired, and
// unproven, with a passing suite that says nothing about it. A real socket cannot drive it without
// either five minutes of wall clock or a knob in the shipped binary; a fake clock and a fake
// browser drive it in microseconds.
//
// `now` is injected for the same reason: the rate limit is a duration, and a test that had to
// sleep through it would be a test nobody runs.
//
// `answer` reports whether it actually announced. A false — a loopback bind, no usable interface —
// must NOT count as an answer, or the rate limit would silence a party that never spoke.
//
// **It is handed the RESOLVED candidate, and production ignores it.** The answer is an ordinary
// announcement of this arm's own endpoint, so nothing about the peer is needed to make it. It is
// passed because a test that cannot see WHICH sighting was answered cannot tell "answered my peer"
// from "answered a stranger" — measured: deleting the `resolve` gate below left the first version
// of `TestTheArmAnswersItsOwnPeerAndNobodyElse` green, because a stranger's sighting and the
// peer's produce the same COUNT. L1 is the property here and a count cannot see it.
// `sighted` is called for every resolved sighting of this arm's own peer, before the answer rate
// limit — see the comment at the call site for why the two must not share a gate.
//
// `wanted` reports whether this arm still wants to be found; `stop` releases whatever it is
// announcing when that goes false. Both are checked once per iteration rather than per sighting,
// because the state this is about — the hop has signed — is exactly the state in which nothing is
// announcing to us any more.
func answerLoop(ctx context.Context, b browser, pins []vault.PinnedPeer, now func() time.Time, wanted func() bool, stop func(), sighted func(time.Time), answer func(candidate) bool) {
	var lastAnswer time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		// **Checked here, acted on AFTER the read.** A `continue` at this point would skip the
		// only blocking call in the loop and spin a long-lived goroutine at full CPU — the read's
		// one-second deadline is what paces this. So the state is captured, the announcer released
		// immediately, and the answering suppressed below.
		//
		// It keeps looping rather than returning: the arm outlives the hop, and a re-race can put
		// it back in a state where being found matters again.
		idle := wanted != nil && !wanted()
		if idle && stop != nil {
			stop()
		}
		// A short read deadline rather than a long one, so ctx cancellation is noticed promptly at
		// disarm instead of after a multi-minute blocking read. The cost is a wakeup per second on
		// a quiet link, which is a timer and not a datagram.
		seen, rerr := b.Read(now().Add(time.Second))
		if rerr != nil {
			continue // a timeout is the ordinary case; ctx is what ends this loop
		}
		if idle {
			continue // signed: a reconnect needs the listener, not another advertisement
		}
		c, ok := resolve(pins, seen)
		if !ok {
			continue // not the peer this arm is for — see the amplification note above
		}
		// **The sighting, reported BEFORE the answer rate limit and not after it (S05e).**
		//
		// The gate below is `hopAnnounceWindow` and exists so a re-dial cannot stack a second
		// announcer — a rule about ANNOUNCING. Observing is a different rate over the same
		// stream, and hanging it off the same gate would make it silently inherit that period:
		// the hold would stop renewing during exactly the stretch in which the peer is most
		// present on the link, which is the opposite of what the evidence says.
		//
		// It is past `resolve`, so it is the arm's OWN expected peer and never a stranger — the
		// screen is pins, not wire bytes (L1), and it is the same screen the answer already
		// trusts.
		if sighted != nil {
			sighted(now())
		}
		if !lastAnswer.IsZero() && now().Sub(lastAnswer) < hopAnnounceWindow {
			continue // one answer per window: a re-dial must not stack a second announcer
		}
		if answer(c) {
			lastAnswer = now()
		}
	}
}

// allQUICCandidates reports whether EVERY candidate can be dialled on the ceremony's shared
// endpoint, and that there is at least one.
//
// The glare join races QUIC candidates only (`filterQUIC`), so a path that takes it silently
// discards everything else it was given. That makes the safe question "can it race all of this?"
// rather than "can it race any of this" — and the difference is not academic.
//
// **Measured, and the "any" version was written first and failed on a link.** A four-party LAN
// relay ran QUIC and then TCP over the same instances. On the TCP run the browse heard the party's
// live TCP announcement AND a lingering QUIC one from the run before, so "any" was true, the glare
// path was taken, the only reachable candidate was discarded, and the user was told *"Couldn't
// reach the rendezvous network"* — a D19 cause about the DHT, for a peer that was on the link and
// announcing.
//
// It is asked AFTER the browse because on a link the transport arrives in the announcement
// (ADR-010) and the request names nothing at all.
//
// **What is still owed, stated narrowly because the wide version of it is fixed (/pending 298,
// closed 2026-08-27).** No candidate is dropped any more: the glare path runs only when every
// candidate can be dialled on it, and everything else goes to `raceWithRendezvous`, which dials
// each candidate on its own transport. What a mixed set loses is not a candidate but the GLARE
// JOIN — the symmetric accept that lets a peer who can only reach us by dialling us be joined
// rather than refused. That is a NAT-traversal tier degrading, not a peer becoming unreachable,
// and racing both kinds side by side belongs to P05's coordinator rather than to this predicate.
func allQUICCandidates(cands []candidate) bool {
	if len(cands) == 0 {
		return false
	}
	for _, c := range cands {
		if c.Transport != transportQUIC {
			return false
		}
	}
	return true
}
