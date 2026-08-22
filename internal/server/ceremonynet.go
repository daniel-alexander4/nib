package server

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
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
func (c *ceremonyID) publishCandidates(ctx context.Context, transport string) error {
	if c == nil || c.rz == nil {
		return errNoCeremony
	}
	self, err := c.rz.ProbeSelf(ctx)
	if err != nil {
		return fmt.Errorf("could not learn this machine's public address: %w", err)
	}
	addr := self.V4.Addr
	if !addr.IsValid() {
		addr = self.V6.Addr
	}
	if !addr.IsValid() {
		// Not a failure of this path: it is D19's cause 3 or 4, and the ladder's other
		// tiers are unaffected. Publishing nothing is the honest outcome — a record naming
		// an address we do not have would send the peer somewhere that is not us.
		return fmt.Errorf("no public address was established, so there is nothing to publish")
	}

	rec := ceremony.CandidateRecord{
		CeremonyID: c.inv.ID,
		Hop:        c.hop,
		Expires:    time.Now().Add(candidateLife()),
		Addrs:      []ceremony.Endpoint{{Addr: addr, Transport: ceremonyTransport(transport)}},
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
	return c.rz.Publish(ctx, seed, c.gate.PublishSalt(), sealed)
}

// feedCandidates fetches the peer's records and sends what the gate admits to the racer.
//
// **It closes `out`, and that is the contract that lets the race end.** `raceCandidates`
// watches its parent context, but its feeder also has to stop consuming — and P05.S03's leak
// was exactly this shape one layer down. The caller owns the context; this owns the channel.
func (c *ceremonyID) feedCandidates(ctx context.Context, out chan<- candidate, peerFP []byte, label, name string) {
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
	for {
		sealed, _, ferr := c.rz.Fetch(ctx, seed, c.gate.Salt())
		if ferr == nil {
			// The gate is the only door. It opens, verifies the signature, checks the
			// author against this hop's expected party and the roster, checks the ceremony
			// and the hop, and screens every address — so nothing reaches a dial that a
			// stranger merely asserted.
			_ = c.gate.Accept(sealed, time.Now())
		}
		// `Candidates()` is the race set IN ARRIVAL ORDER and `Accept` only ever appends,
		// so the tail past what we have already sent IS the diff — no second `seen` set,
		// and a wrong diff would only be wasteful because the racer dedupes on
		// (address, transport) anyway.
		all := c.gate.Candidates()
		for _, e := range all[min(sent, len(all)):] {
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
		case <-time.After(candidateFetchEvery):
		case <-ctx.Done():
			return
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = netip.AddrPort{}
var _ = errors.New

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
		cer.stopNet = cancel

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
		pctx, pcancel := context.WithTimeout(ctx, rendezvousPublishBudget)
		defer pcancel()
		_ = cer.publishCandidates(pctx, transport)
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

	in := make(chan candidate, maxRaceCandidates)
	if cer == nil || cer.rz == nil {
		// No ceremony: the fixed set, exactly as before, through the same racer.
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
		return raceCandidates(ctx, in, cert, key, peerFP)
	}

	dht := make(chan candidate, maxRaceCandidates)
	go cer.feedCandidates(ctx, dht, peerFP, label, name)
	go func() {
		defer safe.Recover("candidate merge")
		defer close(in)
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
		}
	}()
	return raceCandidates(ctx, in, cert, key, peerFP)
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
