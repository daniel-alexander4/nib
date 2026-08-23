package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/addrscope"
	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/portmap"
	"nib/internal/rendezvous"
	"nib/internal/safe"
)

// This file is where the DHT stops being a diagnostic and becomes a candidate source.
//
// Two directions, one per role at a hop. The ARMED side publishes where it can be reached;
// the DIALING side fetches that and feeds the racer. Both go through the ceremony's gate, so
// a record that is not this hop's, not this ceremony's, or not signed by the expected party
// never reaches a dial — which is what keeps L1 true with a stranger supplying the addresses.

// ceremonyTransport maps the server's transport name onto the candidate record's wire byte.
//
// The third vocabulary for one idea, and it is the shape ADR-010 already established rather
// than a new one: a wire format owns a byte, internal/p2p owns a string, and this layer is the
// one that holds both. `transportOf`/`announcedTransport` do exactly this for the LAN
// announcement; these two do it for the candidate record.
func ceremonyTransport(t string) ceremony.Transport {
	if t == transportQUIC {
		return ceremony.TransportQUIC
	}
	return ceremony.TransportTCP
}

func serverTransport(t ceremony.Transport) string {
	if t == ceremony.TransportQUIC {
		return transportQUIC
	}
	return transportTCP
}

// candidateLife is how long a published record claims to be valid.
//
// **Derived, not chosen.** The record has to outlive the PEER's whole race, and the peer's
// race is bounded by `connectDeadline`. On top of that sit the two rendezvous budgets that
// bracket it: our publish traversal can take up to `PublishBudget` before the record is
// anywhere, and the peer's final fetch can take another `PublishBudget` before it is read. A
// clock disagreement between the two machines is real on this path — D19's fifth cause exists
// because of it — so a further allowance is added rather than assumed away.
//
// *Not to be copied from the one prior example:* `nib rendezvous --self-test` used
// `now + 5 minutes`, which is exactly `connectDeadline` with ZERO margin, so a record built
// that way expires while the peer is still reading it. Fine there — its publish and fetch are
// the next two statements — and wrong for a ceremony.
//
// The sum stays well inside `ceremony.MaxCandidateLife`, which is the reader-side ceiling and
// the only thing that caps a publisher's generosity.
const candidateSkewAllowance = 90 * time.Second

// rendezvousInterval steps the DHT fetch cadence down over the life of an arm.
//
// **The constant was sized for a 300 s race and P05.S09b gave it a thirty-day one.** The receive
// arm's window is `MaxCeremonyLife`, and `feedCandidates` polled at a flat 5 s for the whole of
// it — and a "poll" here is not one datagram but a full iterative DHT traversal fanned out to
// the routing table, so the ceiling is on the order of millions of `get` queries aimed at
// strangers, at a BEP-44 target only two parties ever touch. `lan.go` computed exactly this harm
// for the LAN announcer (5.2M multicast datagrams) and capped the announcer at five minutes; the
// DHT half of the same arm never got the same treatment.
//
// It steps rather than caps, and that distinction is load-bearing: `lan.go`'s cap is only safe
// BECAUSE it delegates late discovery to this loop ("a baton that arrives later is discovered
// over the DHT, which the connect arm feeds for the whole window"). Stopping this loop would
// leave a long arm with no discovery at all.
//
// The first tier is unchanged at `candidateFetchEvery`, so the dialing side — bounded by
// `connectDeadline` — behaves exactly as it did. The ceiling is `candidateLife()/2`, which is
// the same figure the republish uses: a side that re-fetches no slower than its peer republishes
// cannot miss a generation of the peer's record.
func rendezvousInterval(elapsed time.Duration) time.Duration {
	switch {
	case elapsed < connectDeadline:
		return candidateFetchEvery
	case elapsed < 30*time.Minute:
		return 30 * time.Second
	default:
		return candidateLife() / 2
	}
}

// republishEvery is how often an armed side refreshes the record that says where to reach it.
//
// **A published record lives eight minutes and the arm lives thirty days.** `candidateLife()` is
// bounded above by `ceremony.MaxCandidateLife`, a READER-side ceiling of one hour that every
// peer enforces — so no value of the record's own expiry can cover an arm, and republishing is
// the only mechanism that can. Without it the arm is un-findable for 29 days 23 hours 52 minutes
// of a 30-day window: a peer dialling at hour three finds nothing, and D19 tells them the other
// side "hasn't started their ceremony yet" about a machine that has been listening for hours.
//
// Half the record's life, so a generation is always in place before the last one expires. It is
// deliberately NOT the fetch cadence: a republish is only ever needed before the record expires,
// while a fetch is a poll for something that may not exist yet, and tying them would republish
// every five seconds during a race.
func republishEvery() time.Duration { return candidateLife() / 2 }

func candidateLife() time.Duration {
	return connectDeadline + 2*rendezvousPublishBudget + candidateSkewAllowance
}

// publishCandidates advertises where this armed session can be reached.
//
// **What it publishes is what the probe OBSERVED, never what the socket is bound to.** The
// bind is `0.0.0.0:0` on the LAN path and a private address everywhere else; the only address
// a peer on the far side of a NAT can use is the one strangers saw us from — which is the
// whole reason the probe and the session must share a socket (caveat 7), and why this is the
// slice where that started to matter.
// publishableEndpoints turns the probe's observations into the endpoints a record carries.
//
// **BOTH families, not the better of the two (P05.S05, D8 tier 2).**
//
// This used to be inline and read `addr := self.V4.Addr; if !addr.IsValid() { addr = self.V6.Addr }`,
// then built a ONE-element slice. That fallback reads like dual-stack support and is the exact
// opposite of it: a host with a working IPv4 address never reached the v6 line, so the only
// case that ever published a v6 address was the case where v4 had already failed. A dual-stack
// peer could therefore never be dialled over IPv6 — which is precisely what tier 2 is.
//
// Order is v4 first, and it is not a preference the racer honours (D8 races every candidate
// concurrently); it is so that two readings of one record list them the same way.
//
// It is a free function taking the observation rather than a method reaching for the network,
// because the rule worth testing is "how many endpoints, from which observations" and reaching
// it through `ProbeSelf` would need a live DHT to ask a question that has nothing to do with
// one. `MaxCandidates` is 8; this returns at most 2.
func publishableEndpoints(self rendezvous.SelfAddress, transport string) []ceremony.Endpoint {
	var out []ceremony.Endpoint
	for _, a := range []netip.AddrPort{self.V4.Addr, self.V6.Addr} {
		if a.IsValid() {
			out = append(out, ceremony.Endpoint{Addr: a, Transport: ceremonyTransport(transport)})
		}
	}
	return out
}

func (c *ceremonyID) publishCandidates(armCtx context.Context, transport string) error {
	if c == nil || c.rz == nil {
		return errNoCeremony
	}
	self, err := c.rz.ProbeSelf(armCtx)
	if err != nil {
		return fmt.Errorf("could not learn this machine's public address: %w", err)
	}
	// Retain the probe for the D19 diagnosis BEFORE the len(addrs)==0 early return below — cause 3 is
	// exactly that early-return case, so a store at end-of-function would never run when it is needed
	// (P05.S11 grill). The class and shared-space are what publishableEndpoints throws away.
	c.setSelf(self)
	addrs := publishableEndpoints(self, transport)
	// The router port-mapping tier (D15, tier 3), appended to the reflexive candidates. It is
	// obtained HERE and not inside `publishableEndpoints`, which is a pure function of a DHT
	// observation with no network reach — a live mapping call has no place in it (grill F5).
	// A miss or an unroutable answer leaves `addrs` untouched: an ordinary tier miss, never an
	// error (D15/D16, grill F8), and never routed through `Sign`'s all-or-nothing screen, which
	// would drop the good reflexive candidates alongside a bad mapped one (grill F1/#1).
	addrs = c.appendMappedCandidate(armCtx, addrs)
	if len(addrs) == 0 {
		// Not a failure of this path: it is D19's cause 3 or 4, and the ladder's other
		// tiers are unaffected. Publishing nothing is the honest outcome — a record naming
		// an address we do not have would send the peer somewhere that is not us.
		return fmt.Errorf("no public address was established, so there is nothing to publish")
	}

	rec := ceremony.CandidateRecord{
		CeremonyID: c.inv.ID,
		Hop:        c.hop,
		Expires:    time.Now().Add(candidateLife()),
		Addrs:      addrs,
	}
	if err := rec.Sign(c.certPEM, c.keyPEM); err != nil {
		return err
	}
	key, err := c.inv.RecordKey(c.hop)
	if err != nil {
		return err
	}
	// **Sealed and published at OUR salt, which is not the one the gate reads.** A hop has
	// two parties and both publish, at two targets; `PublishSalt` exists because until it did
	// the only salt the gate exposed was the read one, so the only value to reach for was
	// wrong.
	sealed, err := rec.Seal(key, c.gate.PublishSalt(), c.hop)
	if err != nil {
		return err
	}
	seed, err := c.inv.HopSeed(c.hop)
	if err != nil {
		return err
	}
	// The publish gets its OWN budget off the arm ctx; Publish self-caps at PublishBudget
	// regardless, and this keeps the arm ctx (which the refresh rides) uncancelled.
	pctx, pcancel := context.WithTimeout(armCtx, rendezvousPublishBudget)
	defer pcancel()
	return c.rz.Publish(pctx, seed, c.gate.PublishSalt(), sealed)
}

// appendMappedCandidate obtains a router port mapping for the shared endpoint's own port and,
// if the answer screens as a dialable public address, appends it to addrs. It returns addrs
// unchanged on any miss.
//
// **Caveat 7:** the internal port is the shared endpoint's, `c.end.LocalAddr()` — the single
// UDP socket the QUIC session actually answers on (`p2p.SharedEndpoint`), not a fresh one. The
// tier is therefore UDP/QUIC-only, and that is correct rather than a gap: this whole publish
// path runs only for a QUIC arm (`c.rz` is nil on TCP), and PCP/NAT-PMP speak UDP to the
// gateway anyway, so D15's "both UDP and TCP" has no call site here until a TCP publish path
// exists. Recorded the way S05 recorded its scoping (grill F4).
//
// **Lifecycle (S07).** The mapping is a managed lease: obtained here, refreshed on the arm
// context by `portMapper`, and DELETED by `close()` on teardown. So the normal case leaves
// nothing on the router. The residue is the CRASH case, and the grill's C8 corrected what it
// costs: a killed process deletes nothing and the mapping lives until the router-GRANTED lease
// expires — which is `portmap.DefaultLeaseSec` (120 s) only if the router honours the request; a router
// granting more (a tested value is 7200 s) leaves the hole open that long. Bounded by the grant,
// not by the 120 s we ask for — recorded rather than claimed as 120 s.
func (c *ceremonyID) appendMappedCandidate(armCtx context.Context, addrs []ceremony.Endpoint) []ceremony.Endpoint {
	if c.end == nil {
		return addrs // no shared endpoint (the TCP path never sets one) — no mapping tier
	}
	ua, ok := c.end.LocalAddr().(*net.UDPAddr)
	if !ok {
		return addrs // no shared UDP socket — not the QUIC path, so no mapping tier
	}
	// A missing gateway is not the end of the tier: UPnP self-discovers over SSDP, so the
	// client is built either way and only the socket protocols are skipped (grill #3).
	client := &portmap.Client{TryUPnP: true}
	if gw, err := portmap.DefaultGateway(); err == nil {
		client.Gateway = netip.AddrPortFrom(gw, portmap.GatewayPort)
	}

	// The mapping is now a MANAGED lease (S07): obtained synchronously here (the record needs the
	// address), refreshed on the ARM context, and deleted by close(). The obtain is bounded by
	// its own 3 s budget; the refresh rides armCtx.
	mapper := newPortMapper(client, portmap.UDP, uint16(ua.Port))
	mapCtx, cancel := context.WithTimeout(armCtx, portMapBudget)
	defer cancel()
	ap, ok := mapper.obtain(mapCtx)
	if !ok {
		return addrs // ErrNoMapping, or the arm was cancelled — swallowed (grill F8)
	}
	ep, ok := screenedMappedEndpoint(ap.Addr(), ap.Port())
	if !ok {
		// A private/CGNAT/low-port answer: we will not publish it, and a mapping we do not
		// publish must not be left on the router — delete it now rather than leak it to lease
		// expiry (grill F1 for the screen, D15 for the delete).
		c.markMapUnroutable() // the router answered but is behind another NAT — D19 cause 3 → VPN, not port-forward
		mapper.close()
		return addrs
	}
	c.setPortMap(mapper)  // stored under the lock, so close() can reach it (grill C6)
	mapper.startRefresh() // self-contained; stopped by close() (diff-grill #1)
	return append(addrs, ep)
}

// screenedMappedEndpoint applies `addrscope.Target` to a gateway-supplied external address and
// returns the endpoint only if it passes. It is the grill's F1 fix as a pure function, so the
// screen is testable without a live router: a CGNAT (100.64/10), double-NAT (RFC-1918) or
// sub-1024 external — all of which a real gateway legitimately returns — is dropped, rather
// than reaching `preimage`'s all-or-nothing check and taking the whole record down with it.
func screenedMappedEndpoint(ext netip.Addr, port uint16) (ceremony.Endpoint, bool) {
	ap := netip.AddrPortFrom(ext, port)
	if !addrscope.Target(ap) {
		return ceremony.Endpoint{}, false
	}
	// **Always QUIC, hard-coded, not the caller's transport (ADR-010, grill F2/#2).** A PCP/
	// NAT-PMP mapping is a UDP pinhole by construction, so it is a QUIC endpoint whatever the
	// arm's transport string says. Threading `transport` here would let a non-QUIC value — which
	// cannot occur today, but a guard is not for today — publish a UDP mapping labelled TCP,
	// the signed-lie-about-the-endpoint that format version 2 exists to prevent.
	return ceremony.Endpoint{Addr: ap, Transport: ceremony.TransportQUIC}, true
}

// feedCandidates fetches the peer's records and sends what the gate admits to the racer.
//
// **It closes `out`, and that is the contract that lets the race end.** `raceCandidates`
// watches its parent context, but its feeder also has to stop consuming — and P05.S03's leak
// was exactly this shape one layer down. The caller owns the context; this owns the channel.
func (c *ceremonyID) feedCandidates(ctx context.Context, out chan<- candidate, peerFP []byte, label, name string, interval func(time.Duration) time.Duration) {
	defer safe.Recover("candidate feed")
	defer close(out)
	if c == nil || c.rz == nil {
		return
	}
	seed, err := c.inv.HopSeed(c.hop)
	if err != nil {
		return
	}
	sent := 0
	started := time.Now()
	for {
		sealed, _, ferr := c.rz.Fetch(ctx, seed, c.gate.Salt())
		if ferr == nil {
			// The gate is the only door. It opens, verifies the signature, checks the
			// author against this hop's expected party and the roster, checks the ceremony
			// and the hop, and screens every address — so nothing reaches a dial that a
			// stranger merely asserted.
			_ = c.gate.Accept(sealed, time.Now())
			// Snapshot the D19 cause signals here, in the gate's only writer, so diagnose() (called
			// concurrently from the live-status path) reads atomics instead of the racy gate. The
			// Refused()/EmptyRecords counters are monotonic, so Store is idempotent across fetches.
			gs := c.gate.Stats()
			c.recordRefused.Store(gs.Refused() > 0)
			c.recordEmpty.Store(gs.EmptyRecords > 0)
		}
		// `Candidates()` is the race set IN ARRIVAL ORDER and `Accept` only ever appends,
		// so the tail past what we have already sent IS the diff — no second `seen` set,
		// and a wrong diff would only be wasteful because the racer dedupes on
		// (address, transport) anyway.
		all := c.gate.Candidates()
		for _, e := range all[min(sent, len(all)):] {
			c.peerSeen.Store(true) // a peer-published address reached the race — D19 cause-1 signal (P05.S11)
			select {
			case out <- candidate{
				// **The pin is the PINNED fingerprint, never the record's.** The record
				// carries an SPKI and `Fingerprint()` derives from it, which is a pin a
				// stranger chose. The gate has already refused any record whose author is
				// not this hop's expected party, and the vault is still what says who that
				// is — L1: the network says where, the vault says who.
				Fingerprint: peerFP,
				Label:       label,
				Name:        name,
				Addr:        e.Addr.String(),
				Transport:   serverTransport(e.Transport),
				Source:      sourceDHT,
				Hop:         c.hop,
			}:
				sent++
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-time.After(interval(time.Since(started))):
		case <-ctx.Done():
			return
		}
	}
}

// startArmedRendezvous warms the DHT and, if the LAN does not answer first, publishes.
//
// # Why the publish WAITS (criterion 10's 2026-08-21 amendment)
//
// D8 races every tier at once, and for the DIAL that is right. For the PUBLISH it is a pure
// cost in the case D8's own "why LAN first" calls the most common: two people in the same
// office, where the LAN tier was always going to win. Publishing anyway hands ~8 storing nodes
// and dozens of traversal nodes both parties' office IP, their permanent SPKI, and — the part
// that is worse than the IP — a target only these two ever touch, so a node holding it can see
// that one other specific address came looking. For a small practice, who is signing with whom
// is frequently the privileged fact.
//
// **The signal is this arm's OWN listener, not another tier's gathering**, which is what makes
// it implementable and what keeps criterion 11 intact. The publishing side is the arm, and the
// arm ANNOUNCES rather than browses — it has no browse result to wait on. What it does have is
// an inbound socket: if a peer reaches it within the LAN window, the ceremony is local and
// nothing needs publishing.
//
// The cost is one-sided and bounded: about `browseWindow` added to the remote path, against a
// 300 s connect deadline, and nothing published at all in the local one.
func (s *Server) startArmedRendezvous(cer *ceremonyID, transport string, inbound *atomic.Bool) {
	if cer == nil || cer.rz == nil {
		return
	}
	go func() {
		defer safe.Recover("armed rendezvous")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cer.setStopNet(cancel) // under the lock (grill C5)

		bctx, bcancel := context.WithTimeout(ctx, bootstrapBudget)
		defer bcancel()
		if err := cer.rz.Bootstrap(bctx); err != nil {
			// Not fatal to the ceremony: the LAN and manual tiers are untouched, and D19
			// cause 2 exists to say so to the user. S11 renders it.
			return
		}

		// The LAN window. A peer that reaches this listener inside it makes the publish
		// unnecessary, and `reached` is already the thing that records "a connection put
		// something in front of the local user".
		select {
		case <-time.After(browseWindow):
		case <-ctx.Done():
			return
		}
		if inbound != nil && inbound.Load() {
			return // the local network answered; nothing leaves this machine
		}
		// S08b: the arm PUNCHES too — symmetric-send (D17). It fetches the peer's published
		// tier-4 address (through the gate) and opens THIS listener's NAT toward it, so the
		// peer's QUIC Initial can land. Concurrent with the publish below and bound to the arm
		// ctx, so it runs for the whole ceremony and stops at teardown.
		punchCh := make(chan candidate, maxRaceCandidates)
		go cer.feedCandidates(ctx, punchCh, nil, "", "", rendezvousInterval)
		go punchLoop(ctx, cer.end.Punch, &punchBudget{}, punchCh, punchInterval)

		// The ARM ctx, not a publish-budget child: the port-mapping REFRESH lives as long as the
		// arm, and binding it to the 45 s publish budget would kill it mid-race (grill C4).
		// publishCandidates bounds its own DHT publish internally; ProbeSelf and Publish each
		// self-cap.
		_ = cer.publishCandidates(ctx, transport)

		// **Keep the goroutine alive until teardown.** Its `defer cancel()` fires on return, and
		// the fetch+punch above are bound to `ctx` — returning after the publish would cancel
		// them the instant they started, the S07-C4 shape. stopNet (called by close()) is what
		// ends the ceremony; wait for it.
		<-ctx.Done()
	}()
}

// hopScoped drops any candidate that does not belong to the hop this race is for.
//
// **Criterion 19, as a property rather than a discipline.** The clause is "a convener holding
// candidates for a later party never dials them during this hop", and until now it held only
// because of which slice the caller happened to pass: `CandidateGate.Candidates()` returns
// bare endpoints, so the hop is dropped at the gate's boundary, and `raceCandidates` takes one
// `peerFP` for the whole race. A stray candidate from another hop would fail the PIN — but it
// would have been DIALLED first, which is exactly what the criterion forbids.
//
// LAN and typed candidates belong to no hop and carry zero; they are matched against the hop
// the race is for only when they came from a hop in the first place.
func hopScoped(c candidate, hop int) bool {
	return c.Source != sourceDHT || c.Hop == hop
}

// publishWhenSlow publishes this ceremony's address after the LAN window, unless a faster tier
// has already won (ctx cancelled) — the dial-side mirror of the arm's LAN-window suppression
// (grill CONFIRMED-3). Without the wait, every LAN-local ceremony would leak the dialer's
// publish-write to the DHT, the correlation handle the arm was built to suppress.
func publishWhenSlow(ctx context.Context, cer *ceremonyID, transport string) {
	defer safe.Recover("dial publish")
	// Each republish re-reads the mapper's CURRENT endpoints, so a refreshed lease that kept
	// its external port is carried automatically. A lease that MOVED port is not this loop's
	// to fix: the peer's gate has already admitted the old address and its cap has no room for
	// the new one, which is item 20 and needs a decision this loop cannot make. `portMoved`
	// still has no reader; the republish does not silently become one.
	publishLoop(ctx, browseWindow, republishEvery(), func(ctx context.Context) {
		_ = cer.publishCandidates(ctx, transport)
	})
}

// publishLoop is the publish, then the REPUBLISH that keeps it alive (/pending 256).
//
// Split out with its periods as parameters, and for the reason `startAnnouncing` takes its
// window: there is no fake clock anywhere in this package, so "driven" means driving this
// function with small durations and counting, not waiting on wall time.
//
// The first wait is the D6 suppression and it is not optional: a ceremony that completes over
// the LAN inside `browseWindow` must never write to the DHT at all, because the write is itself
// the correlation handle the arm exists to suppress. A republish loop that fired before that
// window would re-open exactly what criterion 10's amendment closed — so the loop cannot start
// until the first publish has earned its way past it.
func publishLoop(ctx context.Context, first, every time.Duration, publish func(context.Context)) {
	select {
	case <-time.After(first):
	case <-ctx.Done():
		return // a faster tier won inside the LAN window — do not publish
	}
	publish(ctx)
	for {
		select {
		case <-time.After(every):
			if ctx.Err() != nil {
				return
			}
			publish(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// raceWithRendezvous runs a race fed by the LAN candidates already in hand AND, when the
// ceremony has a rendezvous, by the peer's records as they arrive.
//
// This is the join D16 asks for — "a candidate arriving late joins the race in flight; no tier
// waits on another tier's gathering" — and it is the first production use of the trickle path.
// `dialAny` closes the channel it builds, so every caller until now handed the racer a fixed
// set and the open-channel case existed only in tests.
func (s *Server) raceWithRendezvous(cer *ceremonyID, cands []candidate, cert, key, peerFP []byte,
	label, name string) (*p2p.Conn, error) {

	// The caller owns the deadline and the cancel. Cancelling on return is what stops the
	// feed goroutine — P05.S03's leak was this shape with the arm missing.
	ctx, cancel := context.WithTimeout(context.Background(), connectDeadline)
	defer cancel()

	if cer == nil || cer.rz == nil {
		// No ceremony: the fixed set, exactly as before, through the same racer.
		in := make(chan candidate, maxRaceCandidates)
		go func() {
			defer safe.Recover("candidate feed")
			defer close(in)
			for _, c := range cands {
				select {
				case in <- c:
				case <-ctx.Done():
					return
				}
			}
		}()
		return raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
			return dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, peerFP, lanDialTimeout, nil)
		}) // no ceremony: fresh sockets
	}

	in, _ := s.feedCeremonyRace(ctx, cer, cands, peerFP, label, name)
	// The ceremony QUIC dial goes out the shared endpoint (S08, caveat 7).
	return raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
		return dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, peerFP, lanDialTimeout, cer.end)
	})
}

// feedCeremonyRace builds the ceremony's candidate stream on the caller's ctx: the fixed
// candidates first, then the hop-scoped DHT candidates as they arrive, teed to the punch loop;
// alongside, it publishes this side's own address when the LAN window is slow and punches toward
// the peer. Cancelling ctx (a race winner, or the connect deadline) stops every goroutine and
// suppresses the late publish and further punches. Extracted from raceWithRendezvous so the
// symmetric-racing coordinator (P05.S09) drives the SAME feed under a ctx it shares with its
// accept loop — the dial and the accept must cancel together on a glare win.
func (s *Server) feedCeremonyRace(ctx context.Context, cer *ceremonyID, cands []candidate, peerFP []byte, label, name string) (<-chan candidate, *sync.WaitGroup) {
	in := make(chan candidate, maxRaceCandidates)
	dht := make(chan candidate, maxRaceCandidates)
	// The gate writer is `feedCandidates` (`cer.gate.Accept`), and the gate is not concurrent-safe.
	// The caller (connect) cancels and JOINS this WaitGroup before returning, so the re-race loop's
	// next connect does not spawn a second feed while this one's writer is still running — two
	// overlapping feeds would race on the gate (P05.S11). All four feed goroutines join it.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer safe.Recover("candidate feed")
		defer wg.Done()
		cer.feedCandidates(ctx, dht, peerFP, label, name, rendezvousInterval)
	}()

	// S08b: the dial side is symmetric — it PUBLISHES its own address (so the arm can punch
	// toward it) and PUNCHES toward the peer's candidates. Both are suppressed by the same
	// LAN-window logic: `ctx` is cancelled when the race returns a winner, so if a faster tier
	// wins inside the browse window neither the late publish (D6 privacy) nor further punch
	// packets fire. The QUIC transport is fixed — the punch is QUIC-only (D8).
	wg.Add(1)
	go func() {
		defer safe.Recover("dial publish")
		defer wg.Done()
		publishWhenSlow(ctx, cer, transportQUIC)
	}()
	punchCh := make(chan candidate, maxRaceCandidates)
	wg.Add(1)
	go func() {
		defer safe.Recover("dial punch")
		defer wg.Done()
		punchLoop(ctx, cer.end.Punch, &punchBudget{}, punchCh, punchInterval)
	}()

	wg.Add(1)
	go func() {
		defer safe.Recover("candidate merge")
		defer wg.Done()
		defer close(in)
		defer close(punchCh)
		for _, c := range cands {
			select {
			case in <- c:
			case <-ctx.Done():
				return
			}
		}
		for c := range dht {
			if !hopScoped(c, cer.hop) {
				continue
			}
			select {
			case in <- c:
			case <-ctx.Done():
				return
			}
			// Tee the DHT candidate to the punch loop too (one gate accessor — the gate is not
			// concurrent-safe, so we fan out this stream rather than fetch twice).
			select {
			case punchCh <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	return in, &wg
}

// dialerCeremony gives the DIALING side a ceremony identity and a DHT to fetch from.
//
// It opens a socket of its own rather than sharing with a listener, because at this slice the
// dialing side has none: S09 is where both sides listen. So caveat 7's racing-dialer clause is
// NOT satisfied here and is not claimed to be — that half belongs with the punch (S08), which
// is the first thing whose correctness depends on the dial's source port, and with S06, whose
// scope already says "caveat 7 decides where the request is sent FROM".
func (s *Server) dialerCeremony(text string, cert, key, peerFP []byte) (*ceremonyID, error) {
	cer, err := ceremonyFor(text, cert, key, peerFP)
	if err != nil {
		return nil, err
	}
	end, err := p2p.NewSharedEndpoint("0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	rz, err := rendezvous.Open(end.DHT(), nodeCacheDir(s.configDir))
	if err != nil {
		end.Close()
		return nil, err
	}
	cer.end, cer.rz = end, rz
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapBudget)
	defer cancel()
	if berr := rz.Bootstrap(ctx); berr != nil {
		// Bootstrapping failed: the DHT tier will find nothing, the LAN and manual tiers
		// are unaffected, and the race below still runs over whatever it was handed. D19
		// cause 2 is the sentence for this and S11 renders it.
		return cer, nil
	}
	return cer, nil
}

// checkCeremonyDeadline refuses to START a hop that the ceremony cannot outlive.
//
// **D16's Stage 6 pin, and it is a nesting rule rather than a comparison.** A hop admitted one
// second before the ceremony deadline still gets `exchangeDeadline`'s six minutes, so the
// ceremony outlives its own expiry by up to that much — and the party at that hop is asked to
// consent to a signature on a proceeding that has already ended. The plan states the rule in as
// many words: *"no hop starts unless the ceremony deadline exceeds now plus one full exchange
// budget."* The outer clock must reserve the inner one's worst case, not merely be larger.
//
// **This is where clock 3 stops being a field nobody reads.** `Record.Expires` was labelled
// "the ceremony deadline (D16's clock 3)" and read at exactly one line in the repo — the roster
// preimage — and never compared to a clock anywhere. `MaxCeremonyLife` (P05.S04 T05) caps how far
// ahead it may be SET, which is a different question; this is the one the field is for.
//
// A document with no record is the ordinary two-party co-sign and has no deadline to honour, so
// the absence is not an error. A record that is present must verify: an unverified `Expires` is
// a number a stranger chose.
func checkCeremonyDeadline(pdf []byte, now time.Time) error {
	rec, err := ceremony.Extract(pdf)
	if errors.Is(err, ceremony.ErrNoRecord) {
		return nil // no ceremony attached; nothing to bound
	}
	if err != nil {
		return fmt.Errorf("this document carries a ceremony record that cannot be read: %w", err)
	}
	if err := rec.Verify(now); err != nil {
		return fmt.Errorf("this document's ceremony record does not verify: %w", err)
	}
	if !rec.Expires.After(now.Add(p2p.ExchangeBudget())) {
		return fmt.Errorf("this ceremony ends at %s, which is less than one exchange budget "+
			"(%s) away — starting a hop now would ask somebody to consent to a signature on a "+
			"proceeding that has already ended by the time it completes",
			rec.Expires.UTC().Format(time.RFC3339), p2p.ExchangeBudget())
	}
	return nil
}

// hsResult is a handshaked dial's outcome, carried on connect's dial channel so a total failure
// can report the racer's rich reason rather than a bare nil.
type hsResult struct {
	conn *p2p.HandshakedConn
	err  error
}

// glareSettleWindow bounds how long connect holds a formed connection while waiting for the glare
// PARTNER — the OTHER of the two connections symmetric racing forms. The survivor is deterministic
// (the lower-fingerprint party's dial, glareKeepsDial), and the two ends of each connection are
// one handshake, so when the survivor forms both ends see it within an RTT and converge on it.
// When the survivor never forms — an asymmetric NAT that blocks that one direction — both ends
// fall back, after this window, to the connection that did form, which is again the same one. So
// convergence holds for any value both sides share; the window only trades latency in the
// asymmetric case. Pinned at one second: generous for a WAN RTT. The fallback is fail-SAFE, not
// cost-free: if one side's own survivor dial completes at the peer but takes longer than this
// window to confirm back locally (a lossy link), the two ends can briefly promote different
// connections — each of which then has one end closed by the other, so both Promote calls fail
// and the hop re-races (S10) rather than running a split channel. Correctness holds; an
// over-long window costs a wasted re-race round in that case, not just latency.
const glareSettleWindow = 1 * time.Second

// connect is the symmetric-racing coordinator (P05.S09). Over the ONE shared endpoint it both
// DIALS the peer's candidates and ACCEPTS a dial from the peer, resolves the glare so both ends
// keep the SAME connection, and opens that connection's stream by the ROSTER role (`initiator`) —
// not by who dialled, which is the deadlock S09a fixed. It returns the ready channel; the caller
// runs Initiate or Receive on it by the same role.
//
// QUIC-only, and that is not a limitation being papered over: the shared socket (caveat 7), the
// punch (S08b) and the glare's deferred stream are all QUIC, and a ceremony publishes QUIC
// endpoints because its endpoint IS the QUIC one. A non-QUIC candidate has no place in this race
// and is filtered out; a ceremony that offered only TCP would fail to connect here, loudly, rather
// than silently dialling the wrong way round.
//
// The accept loop is PRIVATE — a QUICListenHandshakeOn the caller armed on cer.end, not s.sess.arm — so it carries no
// consent, announce or disarm of its own; the consent gate lives on s.sess and is reached through
// the ceremony anchor (T05). The listener and the losing connection are closed before return.
func (s *Server) connect(ctx context.Context, cer *ceremonyID, hl *p2p.HandshakeListener, cands []candidate, cert, key, peerFP, myFP []byte, initiator bool, label, name string) (*p2p.Conn, error) {
	keepDial := glareKeepsDial(myFP, peerFP)

	// One child context for BOTH the dial race and the accept, so the glare winner cancels the
	// loser's side — the dial's feed goroutines and the accept — in a single stroke (S03's leak was
	// a feed that outlived its race).
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dialCh := make(chan hsResult, 1)
	go func() {
		defer safe.Recover("glare dial")
		feedCtx, feedCancel := context.WithCancel(cctx)
		in, feedWG := s.feedCeremonyRace(feedCtx, cer, cands, peerFP, label, name)
		conn, derr := raceCandidates(cctx, filterQUIC(feedCtx, in), func(ctx context.Context, c candidate) (*p2p.HandshakedConn, error) {
			// Handshake only — the stream is deferred until the role is known after glare (S09a).
			return p2p.QUICDialHandshakeOn(ctx, cer.end, c.Addr, cert, key, peerFP, lanDialTimeout)
		})
		// Stop the feed and JOIN it before reporting: this iteration's gate WRITER (feedCandidates,
		// which calls the non-concurrent-safe cer.gate.Accept) must be quiesced before the re-race
		// loop's NEXT connect spawns a second feed, or the two would race on the gate (P05.S11).
		feedCancel()
		feedWG.Wait()
		dialCh <- hsResult{conn, derr}
	}()

	acceptCh := make(chan *p2p.HandshakedConn, 1)
	go func() {
		defer safe.Recover("glare accept")
		conn, aerr := hl.Accept(cctx)
		if aerr != nil {
			acceptCh <- nil
			return
		}
		acceptCh <- conn
	}()

	var dialed, accepted *p2p.HandshakedConn
	var dialErr error
	var haveDial, haveAccept, dialDone, acceptDone bool
	var settle <-chan time.Time

	decide := func(settleExpired bool) glareChoice {
		return glareDecide(keepDial, haveDial, haveAccept, settleExpired, dialDone && acceptDone)
	}

	for {
		settleExpired := false
		select {
		case r := <-dialCh:
			dialDone, dialed, dialErr = true, r.conn, r.err
			haveDial = r.conn != nil
			if settle == nil {
				settle = time.After(glareSettleWindow)
			}
		case a := <-acceptCh:
			acceptDone, accepted = true, a
			haveAccept = a != nil
			if settle == nil {
				settle = time.After(glareSettleWindow)
			}
		case <-settle:
			settleExpired = true
		case <-ctx.Done():
			cancel()
			closeHandshaked(dialed)
			closeHandshaked(accepted)
			// A side still in flight when the caller gave up will send its result after the
			// cancel; drain it so a connection that races in past the cancel is closed, not leaked.
			s.drainHandshaked(&dialDone, nil, dialCh)
			s.drainHandshaked(&acceptDone, acceptCh, nil)
			return nil, ctx.Err()
		}

		switch decide(settleExpired) {
		case glareWait:
			continue
		case glareDial:
			cancel()
			closeHandshaked(accepted) // synchronous loser close (T04, both kinds)
			s.drainHandshaked(&acceptDone, acceptCh, nil)
			return dialed.Promote(ctx, initiator)
		case glareAccept:
			cancel()
			closeHandshaked(dialed)
			s.drainHandshaked(&dialDone, nil, dialCh)
			return accepted.Promote(ctx, initiator)
		default: // glareFail
			cancel()
			// settle arms on the FIRST arrival, so it can fire while the OTHER side is still
			// racing — and that side may select a winner after this cancel and send it into a
			// buffered channel nobody would otherwise read. Drain both so a late connection is
			// closed rather than leaked open to the peer (diff-grill: the drain contract holds on
			// every exit, not only the two that keep a survivor).
			s.drainHandshaked(&dialDone, nil, dialCh)
			s.drainHandshaked(&acceptDone, acceptCh, nil)
			if dialErr != nil {
				return nil, dialErr // the racer's rich failure sentence
			}
			return nil, errors.New("could not reach the ceremony peer on any address, and the peer did not reach us")
		}
	}
}

// closeHandshaked closes a handshaked connection if one formed. Nil-safe: a race that lost or an
// accept that never came is nil here.
func closeHandshaked(hc *p2p.HandshakedConn) {
	if hc != nil {
		hc.Close()
	}
}

// drainHandshaked closes the LATE arrival on whichever side connect did not use, so a connection
// that lands just after the glare is resolved is not leaked. Exactly one of the two channels is
// passed; the other is nil. The done flag says the value was already consumed by the main loop.
func (s *Server) drainHandshaked(done *bool, accept <-chan *p2p.HandshakedConn, dial <-chan hsResult) {
	if *done {
		return
	}
	go func() {
		defer safe.Recover("glare drain")
		if accept != nil {
			closeHandshaked(<-accept)
		}
		if dial != nil {
			closeHandshaked((<-dial).conn)
		}
	}()
}

// filterQUIC passes only QUIC candidates through to the coordinator's dial race — the shared
// endpoint speaks QUIC, and a non-QUIC candidate cannot be handshake-dialled on it (see connect).
func filterQUIC(ctx context.Context, in <-chan candidate) <-chan candidate {
	out := make(chan candidate, maxRaceCandidates)
	go func() {
		defer safe.Recover("quic candidate filter")
		defer close(out)
		for c := range in {
			if c.Transport != transportQUIC {
				continue
			}
			select {
			case out <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// quicEndpointAnnounce adapts a shared QUIC endpoint's address to announceable, so a
// symmetric-racing ceremony arm can announce on the LAN (runCeremonyReceive) without the
// coordinator's HandshakeListener presenting as a full p2p.Listener — which ADR-009's
// termination-protocol guard would then require to embed listenerCore, and it deliberately does
// not (it is a minimal handshaked accept, not the shared accept-and-teardown protocol).
type quicEndpointAnnounce struct{ addr net.Addr }

func (q quicEndpointAnnounce) Addr() net.Addr    { return q.addr }
func (q quicEndpointAnnounce) Transport() string { return transportQUIC }
