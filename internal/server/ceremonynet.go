package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
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

// publishCandidates advertises where this armed session can be reached.
//
// **What it publishes is what the probe OBSERVED, never what the socket is bound to.** The
// bind is `0.0.0.0:0` on the LAN path and a private address everywhere else; the only address
// a peer on the far side of a NAT can use is the one strangers saw us from — which is the
// whole reason the probe and the session must share a socket (caveat 7), and why this is the
// slice where that started to matter.
//
// **This comment was attached to `publishableEndpoints`, two functions up, until P08.S05f's
// commit gate parsed for it.** A doc block with no blank line before the next function's own
// comment binds to that function, so a reader of the endpoint helper got a paragraph about the
// publish and the publish had no doc at all. It is the second instance of that defect found in
// one slice — `openRendezvous`'s was the first — which is why the check is now a parse rather
// than a reading.
func (c *ceremonyID) publishCandidates(armCtx context.Context, transport string) error {
	if c == nil || c.rz == nil {
		return errNoCeremony
	}
	// The DHT's first contact with the network, through its one door (S05d). ProbeSelf below is
	// itself off-link traffic, so this is not merely "warm the table first" — both callers reach
	// here only after the LAN window, and a ceremony the link answered never arrives at all.
	if berr := c.ensureBootstrapped(armCtx); berr != nil {
		return fmt.Errorf("could not reach the rendezvous network: %w", berr)
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
		// **Close it even though nothing was obtained.** Since v1.117.120 the mapper records a
		// delete handle for every request that LEFT this host, and `close()` is the only thing
		// that drains them — so returning here dropped the handles for a mapping the router may
		// well have created. The sharpest instance is on the UPnP path: AddPortMapping answers
		// 200, the mapping EXISTS, GetExternalIPAddress then fails, and the whole obtain reports
		// a miss. That is the exact leak /pending 257 was built to close, re-opened at a
		// different door by 257's own change. The screened-out path below already knew to do
		// this; this one did not.
		if mapper.refusal() != nil {
			// The router answered and said no. D19 must not then tell this user that Nib
			// "couldn't get an answer from your router" and suggest enabling UPnP — it is on,
			// and answering, which is why they got a code instead of silence (/pending 263).
			c.markMapRefused()
		}
		mapper.close()
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
func (c *ceremonyID) feedCandidates(ctx context.Context, out chan<- candidate, peerFP []byte, label, name string, hold time.Duration, interval func(time.Duration) time.Duration) {
	defer safe.Recover("candidate feed")
	defer close(out)
	if c == nil || c.rz == nil {
		return
	}
	seed, err := c.inv.HopSeed(c.hop)
	if err != nil {
		return
	}
	// **The LAN window, before the first fetch (S05d).** `publishLoop` has always taken this delay
	// as its `first` parameter and says why; the FETCH did not, so a LAN-local ceremony read from
	// the public DHT before anyone knew the link would answer. Both halves of the leak had to close
	// together: a lazy bootstrap with an unwindowed fetch immediately after it moves the first
	// off-link packet by microseconds. The cost is that the DHT tier starts `hold` later, which
	// D8's ladder races concurrently and is built to absorb.
	//
	// `hold` is `browseWindow` when nobody knows whether the link will answer, and
	// `lanFirstBudget` when the browse already FOUND the peer there — see feedCeremonyRace. A
	// fixed 2 s made the criterion a race against the hop, and the hop won often enough to emit
	// 105 packets on a nine-party relay.
	if !c.holdDHT(ctx, hold) {
		return
	}
	// The DHT's first contact with the network, through its one door. A failure is not fatal — the
	// fetch below simply finds nothing and D19 cause 2 is the sentence for it.
	_ = c.ensureBootstrapped(ctx)
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

// hasLANCandidate reports whether the browse found the peer on the local link.
//
// It reads the SOURCE rather than the address: an address that merely looks link-local may have
// been typed by the user or published to the DHT by the peer, and neither is evidence that this
// machine heard the peer announce in this room.
func hasLANCandidate(cands []candidate) bool {
	for _, c := range cands {
		if c.Source == sourceLAN {
			return true
		}
	}
	return false
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
func publishWhenSlow(ctx context.Context, cer *ceremonyID, transport string, hold time.Duration) {
	defer safe.Recover("dial publish")
	// Each republish re-reads the mapper's CURRENT endpoints, so a refreshed lease that kept
	// its external port is carried automatically. A lease that MOVED port is not this loop's
	// to fix: the peer's gate has already admitted the old address and its cap has no room for
	// the new one, which is item 20 and needs a decision this loop cannot make. `portMoved`
	// still has no reader; the republish does not silently become one.
	// The hold is taken HERE rather than as publishLoop's `first`, because it is renewable on the
	// arm (S05e) and publishLoop's parameter is a duration. publishLoop keeps its parameter — its
	// tests drive it with small durations and counting, which is the only way to drive a period
	// with no fake clock in this package — and is handed zero once the hold has been served.
	if !cer.holdDHT(ctx, hold) {
		return
	}
	publishLoop(ctx, 0, republishEvery(), func(ctx context.Context) {
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

	// **The feed is cancelled AND JOINED before this returns (/pending 355).** It used to discard
	// the WaitGroup — `in, _ :=` — where `connect`, the sibling that drives the same feed, cancels
	// and waits, and says why: `feedCandidates` calls `cer.gate.Accept`, which is not
	// concurrent-safe, so the writer must be quiesced before anything else touches the gate.
	//
	// Here the hazard is a step worse than a gate race, because both callers own the ceremony's
	// lifetime and end it on return. `deliverToParty`'s `defer cer.close()` fires the moment this
	// function returns, and `close()`'s own doc says tearing down `rz`/`end` under a live publish
	// can take the process with it — so the discarded WaitGroup was the difference between a
	// bounded wait and a use-after-close. One rule, both doors.
	feedCtx, feedCancel := context.WithCancel(ctx)
	in, feedWG := s.feedCeremonyRace(feedCtx, cer, cands, peerFP, label, name)
	// The ceremony QUIC dial goes out the shared endpoint (S08, caveat 7).
	conn, err := raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
		return dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, peerFP, lanDialTimeout, cer.end)
	})
	feedCancel()
	feedWG.Wait()
	return conn, err
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
	// **The dial side already knows, and this is where it says so (P07.S05d).** `peerAddresses`
	// browses BEFORE this runs, so a LAN candidate in `cands` is the link having answered — not a
	// guess about whether it will. Holding the DHT tier on a fixed `browseWindow` instead made
	// D6's suppression a race between a 2 s timer and the hop; the hop won often enough that a
	// nine-party LAN relay emitted 105 off-link packets with the lazy bootstrap already in place.
	hold := browseWindow
	if hasLANCandidate(cands) {
		hold = lanFirstBudget
	}
	// The gate writer is `feedCandidates` (`cer.gate.Accept`), and the gate is not concurrent-safe.
	// The caller (connect) cancels and JOINS this WaitGroup before returning, so the re-race loop's
	// next connect does not spawn a second feed while this one's writer is still running — two
	// overlapping feeds would race on the gate (P05.S11). All four feed goroutines join it.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer safe.Recover("candidate feed")
		defer wg.Done()
		cer.feedCandidates(ctx, dht, peerFP, label, name, hold, rendezvousInterval)
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
		publishWhenSlow(ctx, cer, transportQUIC, hold)
	}()
	punchCh := make(chan candidate, maxRaceCandidates)
	wg.Add(1)
	go func() {
		defer safe.Recover("dial punch")
		defer wg.Done()
		punchLoop(ctx, cer.end.Punch, cer.punchBudget(s), punchCh, punchInterval)
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
	// **No bootstrap here (S05d).** This runs BEFORE `peerAddresses` browses the link, so at this
	// point nobody knows whether the LAN will answer — and bootstrapping is off-link traffic. It is
	// deferred to `ensureBootstrapped`, which the two DHT verbs call after the LAN window. A
	// bootstrap failure still means the DHT tier finds nothing while the LAN and manual tiers are
	// unaffected; D19 cause 2 is the sentence for it and S11 renders it.
	return cer, nil
}

// recordOutlivesBudget is the ONE deadline rule, with the reservation as a parameter (ADR-009,
// P08.S04a).
//
// **Two callers, two budgets, and the budgets are not a preference.**
//
//   - The INITIATOR reserves a whole hop (`ceremonyHopBudget()`, 29m20s), because it is about to
//     start one and D16's nesting rule says the outer clock must reserve the inner one's worst case.
//   - The SIGNING party reserves **ZERO** — it refuses only a deadline already past.
//
// **Why zero, since a reservation looks safer.** It is not: at the receiver a reservation refuses
// HONEST hops. The worst-case lag from the convener's dial to the signer's gate is
// `bootstrapBudget(20s) + connectDeadline(300s) + exchangeDeadline(360s) + the spoken-check gate
// (300s, which no connection deadline bounds because no I/O happens during it) +
// exchangeDeadline(360s)` = **22m20s**, against an initiator that admitted the hop at 29m20s. The
// margin is **7m00s, and that figure IS the tolerable clock skew between two machines with no time
// sync**. Reserving even eight minutes at this end makes it minus one minute, so a hop the convener
// correctly admitted is refused at the far end for arithmetic reasons.
//
// The arithmetic was got wrong twice before it was re-derived at the line — once by omitting one of
// `Receive`'s two `exchangeDeadline` arms, once by omitting the spoken gate as well — which is why
// `TestTheHopBudgetNestsTheReceiversWorstCaseLag` exists to make it falsifiable rather than argued.
//
// The cost of zero, stated: the nesting property at the receiver is DELEGATED to the convener. That
// guard is what makes the delegation checked instead of assumed.
func recordOutlivesBudget(rec ceremony.Record, now time.Time, budget time.Duration) error {
	if rec.Expires.After(now.Add(budget)) {
		return nil
	}
	if budget == 0 {
		return fmt.Errorf("%w: this ceremony ended at %s", p2p.ErrCeremonyEnded,
			rec.Expires.UTC().Format(time.RFC3339))
	}
	return fmt.Errorf("this ceremony ends at %s, which is less than one hop (%s) away — "+
		"starting a hop now would ask somebody to consent to a signature on a "+
		"proceeding that has already ended by the time it completes",
		rec.Expires.UTC().Format(time.RFC3339), budget)
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
	// **Reserve a whole HOP, not one phase of one (fixed 2026-08-24, P07.S02a, C20).**
	//
	// This reserved `p2p.ExchangeBudget()` — 6 minutes — against a hop that can spend
	// 29m20s. exchangeDeadline's own doc says it is "the budget for one PHASE of a session —
	// never for the whole of it", and this read it as the whole. Measured consequence: with
	// `Expires = now+7m` the check PASSED and the far party's consent landed thirteen minutes
	// after the ceremony had ended — verbatim the harm the paragraph above says it prevents.
	//
	// The guard for it was self-referential too: ceremonydeadline_test.go derived its own
	// expectation from the same call, so it could not fail for the reservation being wrong.
	//
	// Still ONE hop rather than every REMAINING hop: this function is handed a document and a
	// clock and does not know the local party's position in the roster, so "how many hops are
	// left" is not answerable here. Convene reserves the whole ceremony up front — which is C20's
	// clause — so a document that reached this gate was admitted against a deadline covering every
	// hop; refining this to the remaining hops needs the hop index and is S05's carry route.
	//
	// **What Convene reserves is no longer `hops × ceremonyHopBudget()` (P08.S05b).** It is now
	// `hops × (hopBudget + deliveryBudget)`, because the delivery round costs a leg per party and
	// had nothing reserved for it. That makes the up-front reservation strictly larger, so the
	// one-hop reservation here remains sound — but the sentence above used to name the old figure,
	// and a justification that cites a number the code no longer uses is the doc-vs-code shape this
	// repo keeps finding.
	return recordOutlivesBudget(rec, now, ceremonyHopBudget())
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
