package rendezvous

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"

	"github.com/anacrolix/dht/v2/krpc"
	"nib/internal/addrscope"
)

// probeBudget is D16's clock-1 allowance for the DHT self-address probe.
//
// Measured against the public DHT on 2026-08-19: with a warm table, two agreeing
// observations arrive in 0.12–0.32 s and sixteen inside a second. Eight seconds is
// roughly twenty-five times what the common case needs, which is the right shape for a
// gather deadline — D16 is explicit that a source missing its budget contributes no
// candidate and is not an error.
const probeBudget = 8 * time.Second

// probeNodes is how many distinct-prefix nodes a probe asks.
//
// Also measured, and the number is not arbitrary: at four nodes a trial got one reply
// and never reached two observations inside the budget; at eight and sixteen every
// trial reached two inside a third of a second. The reply rate is around 60%, and about
// four fifths of those carry an `ip` field, so sixteen asks for roughly eight answers to
// need two.
const probeNodes = 16

// Mapping is RFC 4787's mapping-behaviour axis, collapsed to the one bit a DHT probe can
// actually resolve.
//
// RFC 4787 §4.1 names three behaviours — endpoint-independent, address-dependent, and
// address-and-port-dependent. Comparing what two nodes at two different addresses report
// separates the first from the other two and **cannot separate those two from each
// other**: that needs a second observation from the SAME address on a DIFFERENT port,
// which is what RFC 5780's STUN check gets from OTHER-ADDRESS and what the DHT has no
// way to ask for. D19's amendment of 2026-08-19 says so rather than implying the whole
// axis is being read.
//
// Filtering behaviour is not measured here at all, and it is the other half of whether a
// punch succeeds.
type Mapping uint8

const (
	// MappingUnknown is the ZERO VALUE on purpose.
	//
	// The obvious shape is a bool, and its zero value is `false` — which reads as
	// endpoint-independent, the optimistic answer. A probe that learned nothing would
	// then tell the tier-preference logic "your NAT is fine" while telling the user
	// D19 cause 4, two readers of one field disagreeing. Three values, and the one
	// you get for free is the one that claims least.
	MappingUnknown Mapping = iota
	// MappingEndpointIndependent — every responder saw the same address and port.
	MappingEndpointIndependent
	// MappingEndpointDependent — responders disagreed, so the mapping depends on
	// where we are sending. Merges RFC 4787's address-dependent and
	// address-and-port-dependent; see the type comment.
	MappingEndpointDependent
)

func (m Mapping) String() string {
	switch m {
	case MappingEndpointIndependent:
		return "endpoint-independent"
	case MappingEndpointDependent:
		return "endpoint-dependent"
	default:
		return "unknown"
	}
}

// Observation is one node's report of where our packets appeared to come from.
type Observation struct {
	// From is the node that said it. Never trusted, only counted.
	From netip.Addr
	// Self is what that node claims our public endpoint is.
	Self netip.AddrPort
}

// Class is the classification for one address family.
type Class struct {
	Mapping Mapping
	// Addr is our public endpoint, and is meaningful **only** when Mapping is
	// endpoint-independent. Under an endpoint-dependent mapping there is no single
	// address to have, which is the whole content of the finding.
	Addr netip.AddrPort
	// Sources is how many distinct source prefixes reported at all.
	Sources int
	// Agreed is how many of them were in the winning group.
	Agreed int
}

// SelfAddress is one probe's whole result.
type SelfAddress struct {
	Observations []Observation
	V4, V6       Class
	// SharedAddressSpace is true when a responder placed us inside 100.64.0.0/10.
	//
	// It is here because D19's amended cause 3 turns on exactly this: telling someone
	// to enable port mapping on their router is precisely the wrong advice for a
	// carrier-NAT subscriber, who has no router to enable it on. The observation is
	// refused as an address — it cannot be dialled from outside — but throwing away
	// the fact would make the message layer re-probe to learn what this probe already
	// knew.
	SharedAddressSpace bool
}

// ProbeSelf learns this host's public endpoint from DHT responses alone.
//
// L1: nothing this returns may influence which peer is accepted. It changes messages and
// tier preference; the pin check never sees it.
func (s *Server) ProbeSelf(ctx context.Context) (SelfAddress, error) {
	ctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()

	targets := s.probeTargets()
	if len(targets) == 0 {
		return SelfAddress{}, errors.New("rendezvous: no nodes to probe — the routing " +
			"table is empty, so this is a cold start rather than a network failure")
	}

	var (
		mu  sync.Mutex
		out SelfAddress
		wg  sync.WaitGroup
	)
	for _, addr := range targets {
		wg.Add(1)
		go func(a *net.UDPAddr) {
			defer wg.Done()
			res := s.dht.Query(ctx, dht.NewAddr(a), "ping", dht.QueryInput{})
			if res.ToError() != nil {
				return
			}
			from, ok := netip.AddrFromSlice(a.IP)
			if !ok {
				return
			}
			obs, shared, ok := s.observation(from.Unmap(), res.Reply.IP)
			mu.Lock()
			defer mu.Unlock()
			if shared {
				out.SharedAddressSpace = true
			}
			if ok {
				out.Observations = append(out.Observations, obs)
			}
		}(addr)
	}
	wg.Wait()

	out.V4 = classify(out.Observations, false)
	out.V6 = classify(out.Observations, true)
	s.disagreements.Add(uint64(disagreements(out.Observations, out.V4) +
		disagreements(out.Observations, out.V6)))
	return out, nil
}

// observation validates one reply's `ip` field before anything downstream believes it.
//
// Every refusal is counted under its own cause, because they mean different things and a
// sum would hide the interesting one.
func (s *Server) observation(from netip.Addr, ip krpc.NodeAddr) (Observation, bool, bool) {
	// LENGTH FIRST, and it is not defensive tidiness.
	//
	// krpc.NodeAddr.UnmarshalBinary takes the last two bytes as the port and the rest
	// as the address, with no length check — so a FOUR-byte `ip` value decodes with no
	// error into a two-byte address and a port that was never sent. That is a
	// plausible-looking answer built from a message that did not contain one, and it is
	// precisely the reading caveat 2 asks us not to trust. A 6-byte wire value gives 4
	// address bytes, an 18-byte one gives 16; nothing else is a real endpoint.
	//
	// netip.AddrFromSlice IS that check — it accepts 4 and 16 and nothing else. A first
	// draft tested len(ip.IP) explicitly as well, and the red-proof pass found that
	// branch could never run: two ways to refuse the same bytes, one of them
	// unreachable and therefore untestable. One door, and it is provably the door.
	addr, ok := netip.AddrFromSlice(ip.IP)
	if !ok {
		s.rejectedLength.Add(1)
		return Observation{}, false, false
	}
	addr = addr.Unmap()
	if ip.Port <= 0 || ip.Port > 0xffff {
		s.rejectedPort.Add(1)
		return Observation{}, false, false
	}
	shared := isSharedAddressSpace(addr)
	if !isPublishable(addr) {
		s.rejectedScope.Add(1)
		return Observation{}, shared, false
	}
	s.observed.Add(1)
	return Observation{From: from, Self: netip.AddrPortFrom(addr, uint16(ip.Port))}, shared, true
}

// isPublishable refuses an address we must never treat as our own public endpoint.
//
// **The rule moved to internal/addrscope at P04.S04** and this is a one-line delegate. It
// is not only ours any more: P04.S04 needs the same predicate for the candidate addresses a
// PEER publishes, which are parsed in internal/ceremony — a package this one may never
// import and which may not import this one back. A second copy would have drifted, and the
// tree already demonstrates that (internal/server/fetch.go carries a weaker one).
//
// The stake, unchanged: this address gets published as a candidate, and D8/D16/D33 have
// both sides punch at every candidate. A node that reports our address as a stranger's has
// aimed that at them, and D6's own pin is explicit that L1 does not cover it — L1 stops a
// lying rendezvous substituting a *signer*, and says nothing about it making Nib emit
// traffic at a victim.
func isPublishable(a netip.Addr) bool { return addrscope.Routable(a) }

func isSharedAddressSpace(a netip.Addr) bool { return addrscope.SharedSpace(a) }

// probeTargets picks nodes to ask, one per source prefix.
//
// **Per prefix, not per node**, and that is the difference between resolving D19's
// clause and only appearing to. The clause turns on "two different DHT nodes", but two
// nodes behind one NAT — two clients on one home connection, two daemons on one seedbox
// — are ONE destination as far as a mapping is concerned. Counting them as two would
// answer the question with a number that cannot answer it.
func (s *Server) probeTargets() []*net.UDPAddr {
	seen := make(map[netip.Prefix]bool)
	var out []*net.UDPAddr
	for _, ni := range s.dht.Nodes() {
		a, ok := netip.AddrFromSlice(ni.Addr.IP)
		if !ok || ni.Addr.Port <= 0 {
			continue
		}
		a = a.Unmap()
		// An IPv6 link-local node needs a zone, and krpc.NodeAddr has nowhere to carry
		// one — the same gap P03 hit when a discovered `[fe80::…]:8443` came back
		// "connect: invalid argument". The library will accept such a node into the
		// table (BEP-42 treats link-local as local, so the id check passes), and asking
		// it would spend one of at most sixteen slots on a query that cannot be sent.
		// Loopback and private addresses are deliberately NOT excluded: a DHT node on
		// the LAN is legitimate, and the driven tests are built on loopback ones.
		if a.Is6() && a.IsLinkLocalUnicast() {
			continue
		}
		p, ok := sourcePrefix(a)
		if !ok || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, ni.Addr.UDP())
		if len(out) >= probeNodes {
			break
		}
	}
	return out
}

// sourcePrefix is the granularity at which two responders count as different.
func sourcePrefix(a netip.Addr) (netip.Prefix, bool) {
	bits := 48
	if a.Is4() {
		bits = 24
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return netip.Prefix{}, false
	}
	return p, true
}

// classify reduces observations of one family to a mapping class.
//
// Pure, and separate from the query that feeds it, for the reason writeNodes is separate
// from saveNodes: a decision entangled with the thing it queries is a decision no test
// can put into a state. Every interesting case here — a liar, a genuine
// endpoint-dependent NAT, a single node agreeing with itself — is a table row.
func classify(all []Observation, want6 bool) Class {
	groups := make(map[netip.AddrPort]map[netip.Prefix]bool)
	sources := make(map[netip.Prefix]bool)
	for _, o := range all {
		if o.Self.Addr().Is4() == want6 {
			continue // never compare across families: two candidates, not a disagreement
		}
		p, ok := sourcePrefix(o.From)
		if !ok {
			continue
		}
		sources[p] = true
		if groups[o.Self] == nil {
			groups[o.Self] = make(map[netip.Prefix]bool)
		}
		groups[o.Self][p] = true
	}
	c := Class{Sources: len(sources)}
	// One observation agrees with itself, which is not corroboration. Below two
	// distinct sources there is nothing to compare and the honest answer is that we
	// do not know — caveat 9's degradation to D19 cause 4, and the expected outcome
	// on a cold or blocked network rather than a defect.
	if c.Sources < 2 {
		return c
	}
	var best netip.AddrPort
	for addr, srcs := range groups {
		if len(srcs) > c.Agreed || (len(srcs) == c.Agreed && addr.String() < best.String()) {
			c.Agreed, best = len(srcs), addr
		}
	}
	// A STRICT MAJORITY of the sources that answered, so one lying node is outvoted
	// rather than obeyed — and so a genuine endpoint-dependent NAT, where every source
	// sees something different and no group exceeds one, still classifies correctly.
	//
	// What this does NOT defeat, said plainly so it is never over-read: an off-path
	// attacker who can spoof source addresses. Transaction ids come from a
	// process-global sequential varint (server.go:843), the reply is matched on
	// {RemoteAddr, T}, and the first reply wins — so such an attacker wins K races as
	// easily as one. The issuer is not configurable, so this is a limit, not a defence.
	if c.Agreed*2 > c.Sources {
		c.Mapping, c.Addr = MappingEndpointIndependent, best
		return c
	}
	c.Mapping = MappingEndpointDependent
	return c
}

// disagreements counts observations of a family that differed from a majority that
// formed anyway.
//
// Deliberately zero when no majority formed: under an endpoint-dependent classification
// every observation differs from every other, and reporting that as N disagreements
// would drown the case this exists to surface — a consensus with one dissenter, which
// is what a lying node looks like and has no other signal.
func disagreements(all []Observation, c Class) int {
	if c.Mapping != MappingEndpointIndependent {
		return 0
	}
	n := 0
	for _, o := range all {
		if o.Self.Addr().Is4() == c.Addr.Addr().Is4() && o.Self != c.Addr {
			n++
		}
	}
	return n
}
