package rendezvous

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

func ob(t *testing.T, from, self string) Observation {
	t.Helper()
	f, err := netip.ParseAddr(from)
	if err != nil {
		t.Fatal(err)
	}
	s, err := netip.ParseAddrPort(self)
	if err != nil {
		t.Fatal(err)
	}
	return Observation{From: f, Self: s}
}

// TestClassifyIsThreeValuedAndCountsSources.
//
// classify is pure and separate from the query that feeds it for the reason writeNodes
// is separate from saveNodes: every case that matters is one a live DHT cannot be put
// into on demand — a liar, a genuinely symmetric NAT, two nodes that turn out to be one
// destination.
func TestClassifyIsThreeValuedAndCountsSources(t *testing.T) {
	for _, c := range []struct {
		name    string
		obs     [][2]string
		want    Mapping
		addr    string
		sources int
		why     string
	}{
		{
			name: "nothing answered", obs: nil, want: MappingUnknown, sources: 0,
			why: "the zero value must be the claim-nothing one; a bool would say 'your NAT is fine'",
		},
		{
			name: "one node agreeing with itself", want: MappingUnknown, sources: 1,
			obs: [][2]string{{"9.9.9.9", "203.0.113.9:1000"}},
			why: "one observation is not corroboration, and D19 turns on TWO different nodes",
		},
		{
			name: "two nodes in one /24 are ONE source", want: MappingUnknown, sources: 1,
			obs: [][2]string{{"9.9.9.1", "198.18.9.9:1000"}, {"9.9.9.2", "198.18.9.9:1000"}},
			why: "two nodes behind one NAT are one destination; counting them as two would " +
				"resolve the clause with a number that cannot answer it",
		},
		{
			name: "two distinct prefixes agreeing", want: MappingEndpointIndependent,
			addr: "198.18.9.9:1000", sources: 2,
			obs: [][2]string{{"9.9.9.1", "198.18.9.9:1000"}, {"8.8.8.1", "198.18.9.9:1000"}},
			why: "the two-observation case D19 describes",
		},
		{
			name: "two distinct prefixes disagreeing", want: MappingEndpointDependent, sources: 2,
			obs: [][2]string{{"9.9.9.1", "198.18.9.9:1000"}, {"8.8.8.1", "198.18.9.9:2000"}},
			why: "a different mapping per destination is the finding, and there is no single address to have",
		},
		{
			name: "a genuinely symmetric NAT: every source sees a different port",
			want: MappingEndpointDependent, sources: 5,
			obs: [][2]string{
				{"9.9.9.1", "198.18.9.9:1"}, {"8.8.8.1", "198.18.9.9:2"}, {"7.7.7.1", "198.18.9.9:3"},
				{"6.6.6.1", "198.18.9.9:4"}, {"5.5.5.1", "198.18.9.9:5"},
			},
			why: "no group exceeds one, so no majority — the classification must not need a tie-break to get this right",
		},
		{
			name: "one liar among four is outvoted", want: MappingEndpointIndependent,
			addr: "198.18.9.9:1000", sources: 4,
			obs: [][2]string{
				{"9.9.9.1", "198.18.9.9:1000"}, {"8.8.8.1", "198.18.9.9:1000"},
				{"7.7.7.1", "198.18.9.9:1000"}, {"6.6.6.1", "198.51.100.7:53"},
			},
			why: "the liar picks the reflection target; a strict majority is what stops us publishing it",
		},
		{
			name: "one liar against ONE honest node wins nothing", want: MappingEndpointDependent,
			sources: 2,
			obs:     [][2]string{{"9.9.9.1", "198.18.9.9:1000"}, {"6.6.6.1", "198.51.100.7:53"}},
			why:     "with two sources and no majority the honest answer is that we cannot tell, and refusing to publish is the safe direction",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var obs []Observation
			for _, p := range c.obs {
				obs = append(obs, ob(t, p[0], p[1]))
			}
			got := classify(obs, false)
			if got.Mapping != c.want {
				t.Errorf("Mapping = %v, want %v — %s", got.Mapping, c.want, c.why)
			}
			if got.Sources != c.sources {
				t.Errorf("Sources = %d, want %d — %s", got.Sources, c.sources, c.why)
			}
			if c.addr != "" && got.Addr.String() != c.addr {
				t.Errorf("Addr = %v, want %v — %s", got.Addr, c.addr, c.why)
			}
			if c.addr == "" && got.Addr.IsValid() {
				t.Errorf("Addr = %v on a %v classification — an address that is only "+
					"meaningful under endpoint-independence must not be handed out under "+
					"any other", got.Addr, got.Mapping)
			}
		})
	}
}

// TestFamiliesAreClassifiedApartNotAgainstEachOther.
//
// A v4 and a v6 observation are two candidates, not a disagreement. Comparing them would
// report endpoint-dependent mapping on a dual-stack host with no NAT at all — and IPv6 is
// the topology where there is usually no mapping to classify.
func TestFamiliesAreClassifiedApartNotAgainstEachOther(t *testing.T) {
	obs := []Observation{
		ob(t, "9.9.9.1", "198.18.9.9:1000"),
		ob(t, "8.8.8.1", "198.18.9.9:1000"),
		ob(t, "2001:4860::1", "[2001:db9::1]:1000"),
		ob(t, "2001:4870::1", "[2001:db9::1]:1000"),
	}
	v4, v6 := classify(obs, false), classify(obs, true)
	if v4.Mapping != MappingEndpointIndependent || v4.Addr.String() != "198.18.9.9:1000" {
		t.Errorf("v4 = %v %v, want endpoint-independent 198.18.9.9:1000", v4.Mapping, v4.Addr)
	}
	if v6.Mapping != MappingEndpointIndependent || v6.Addr.String() != "[2001:db9::1]:1000" {
		t.Errorf("v6 = %v %v, want endpoint-independent [2001:db9::1]:1000", v6.Mapping, v6.Addr)
	}
	if v4.Sources != 2 || v6.Sources != 2 {
		t.Errorf("sources v4=%d v6=%d, want 2 and 2 — one family is counting the other's "+
			"responders, so each is classified on a population that is not its own",
			v4.Sources, v6.Sources)
	}
}

// TestABogonSelfAddressIsRefusedUnderItsOwnCause — the counters are split for a reason.
func TestABogonSelfAddressIsRefusedUnderItsOwnCause(t *testing.T) {
	for _, c := range []struct {
		name, host string
		port       int
		raw        []byte
		length     uint64
		portC      uint64
		scope      uint64
		shared     bool
	}{
		{name: "a private address", host: "10.1.2.3", port: 6881, scope: 1},
		{name: "loopback", host: "127.0.0.5", port: 6881, scope: 1},
		{name: "link-local", host: "169.254.1.1", port: 6881, scope: 1},
		{name: "documentation", host: "192.0.2.7", port: 6881, scope: 1},
		{name: "carrier NAT", host: "100.100.1.2", port: 6881, scope: 1, shared: true},
		{name: "port zero", host: publicLiteral, port: 0, portC: 1},
		{name: "a FOUR-byte ip field", raw: []byte{1, 2, 3, 4}, length: 1},
		{name: "a two-byte ip field", raw: []byte{1, 2}, length: 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			n := newNode(t)
			var f *fakeNode
			if c.raw != nil {
				f = newFakeNode(t, "127.9.0.1", rawIP(c.raw))
			} else {
				f = newFakeNode(t, "127.9.0.1", reports(c.host, c.port))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := n.rz.Ping(ctx, f.addr); err != nil {
				t.Fatalf("setup: the fake node did not answer a ping (%v) — nothing below "+
					"would be a refusal, it would be an absence", err)
			}
			got, err := n.rz.ProbeSelf(ctx)
			if err != nil {
				t.Fatal(err)
			}
			st := n.rz.Stats()
			if st.Observed != 0 {
				t.Errorf("Observed = %d, want 0 — the address was BELIEVED, and it is the "+
					"one we publish and both sides punch at", st.Observed)
			}
			if st.RejectedLength != c.length || st.RejectedPort != c.portC || st.RejectedScope != c.scope {
				t.Errorf("rejected length=%d port=%d scope=%d, want %d/%d/%d — refused, but "+
					"filed under the wrong cause, and a 4-byte field (silent corruption) must "+
					"never be summed with a bogon (somebody aiming us at a victim)",
					st.RejectedLength, st.RejectedPort, st.RejectedScope, c.length, c.portC, c.scope)
			}
			if got.SharedAddressSpace != c.shared {
				t.Errorf("SharedAddressSpace = %v, want %v — D19's amended cause 3 turns on "+
					"this exact bit, and re-probing to recover it is what it exists to avoid",
					got.SharedAddressSpace, c.shared)
			}
		})
	}
}

// TestALyingNodeIsOutvotedNotObeyed, driven end to end through the real probe.
func TestALyingNodeIsOutvotedNotObeyed(t *testing.T) {
	n := newNode(t)
	honest := []*fakeNode{
		newFakeNode(t, "127.11.0.1", reports(publicLiteral, 4000)),
		newFakeNode(t, "127.12.0.1", reports(publicLiteral, 4000)),
		newFakeNode(t, "127.13.0.1", reports(publicLiteral, 4000)),
	}
	liar := newFakeNode(t, "127.14.0.1", reports(victimLiteral, 53))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, f := range append(append([]*fakeNode{}, honest...), liar) {
		if err := n.rz.Ping(ctx, f.addr); err != nil {
			t.Fatalf("setup: %s did not answer (%v)", f.addr, err)
		}
	}
	// Stimulus: all four really are distinct sources, so "outvoted" is a majority and
	// not an artefact of the liar never having been asked.
	if got := len(n.rz.probeTargets()); got != 4 {
		t.Fatalf("setup: %d probe targets, want 4 — the liar may not even be in the vote", got)
	}

	got, err := n.rz.ProbeSelf(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.V4.Mapping != MappingEndpointIndependent {
		t.Fatalf("Mapping = %v, want endpoint-independent (%d sources, %d agreed)",
			got.V4.Mapping, got.V4.Sources, got.V4.Agreed)
	}
	if got.V4.Addr.String() != publicLiteral+":4000" {
		t.Errorf("Addr = %v, want a majority address — one node's claim became the candidate "+
			"both sides punch at, which is the packet cannon D6's pin names", got.V4.Addr)
	}
	if got.V4.Agreed != 3 || got.V4.Sources != 4 {
		t.Errorf("agreed %d of %d sources, want 3 of 4", got.V4.Agreed, got.V4.Sources)
	}
	if n.rz.Stats().Disagreements != 1 {
		t.Errorf("Disagreements = %d, want 1 — the liar was outvoted but not counted, and "+
			"a counter nobody can read is what P03 already paid for", n.rz.Stats().Disagreements)
	}
}

// TestTwoNodesAreNeededToClassify — caveat 9's degradation, driven rather than assumed.
func TestTwoNodesAreNeededToClassify(t *testing.T) {
	n := newNode(t)
	f := newFakeNode(t, "127.21.0.1", reports(publicLiteral, 4000))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := n.rz.Ping(ctx, f.addr); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := n.rz.ProbeSelf(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the one node DID answer, so "unknown" is "one is not enough" and not
	// "nothing arrived".
	if n.rz.Stats().Observed != 1 {
		t.Fatalf("setup: Observed = %d, want 1", n.rz.Stats().Observed)
	}
	if got.V4.Mapping != MappingUnknown {
		t.Errorf("Mapping = %v with one source, want unknown — this is caveat 9's "+
			"degradation to D19 cause 4 and the expected outcome, not a defect", got.V4.Mapping)
	}
	if got.V4.Addr.IsValid() {
		t.Errorf("Addr = %v from a single unverified claim", got.V4.Addr)
	}
}

// TestAnEmptyTableIsACollStartNotANetworkFailure.
func TestAnEmptyTableIsAColdStartNotANetworkFailure(t *testing.T) {
	n := newNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := n.rz.ProbeSelf(ctx)
	if err == nil {
		t.Fatal("ProbeSelf succeeded with an empty routing table")
	}
	if want := "cold start"; !strings.Contains(err.Error(), want) {
		t.Errorf("refused as %q; it should name the cold start, because 'no observations' "+
			"and 'nobody to ask' want different advice", err)
	}
}

// publicLiteral and victimLiteral are globally-routable addresses used as DATA ONLY —
// a fake node CLAIMS them, and nothing in this package ever sends them a packet. They
// have to be genuinely routable, because the reserved ranges are exactly what
// isPublishable refuses, and a fixture inside one would make every driven case below
// pass for the wrong reason. victimLiteral is a real service on purpose: the realistic
// lie aims Nib at something that exists.
const (
	publicLiteral = "81.2.69.142"
	victimLiteral = "8.8.8.8"
)

// TestAKRPCErrorReplyIsNotALiveNode.
//
// `QueryResult.Err` is about the transaction — did we get an answer. A node that answers
// "no" leaves it nil, so reading `Err` alone counts a node refusing us as a healthy one,
// and the refusal then shows up as a mysteriously empty routing table rather than as a
// node that said no. `ToError()` is the one that folds in the reply's own error.
func TestAKRPCErrorReplyIsNotALiveNode(t *testing.T) {
	n := newNode(t)
	f := newFakeNode(t, "127.31.0.1", func(q krpc.Msg, _ krpc.ID) []byte {
		return bencode.MustMarshal(krpc.Msg{
			T: q.T, Y: "e",
			E: &krpc.Error{Code: krpc.ErrorCodeGenericError, Msg: "no"},
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stimulus: the node really answers.
	//
	// The first version of this block called net.DialUDP and checked the error — which
	// proves nothing at all. connect(2) on a UDP socket is a purely local operation: it
	// sends no packet, and it succeeds against a closed port, an unbound port, and a
	// fake node whose goroutine has already exited. So: send the bytes, read the reply.
	probe, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	probe.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := probe.WriteTo([]byte("d1:ad2:id20:aaaaaaaaaaaaaaaaaaaae1:q4:ping1:t2:zz1:y1:qe"), f.addr); err != nil {
		t.Fatal(err)
	}
	if _, _, err := probe.ReadFrom(make([]byte, 2048)); err != nil {
		t.Fatalf("setup: the fake node did not answer at all (%v) — a failure below would "+
			"be a timeout, not the error reply this test is about", err)
	}
	err = n.rz.Ping(ctx, f.addr)
	if err == nil {
		t.Fatal("a node that answered with a KRPC error counted as live — the reply arrived, " +
			"so the transaction succeeded, and only the reply's own error says it was a refusal")
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("refused as %v — that is the transaction timing out, so this test is not "+
			"exercising the error-reply path at all", err)
	}
	t.Logf("REFUSED as %v", err)
}

// TestAZonelessLinkLocalNodeIsNotProbed.
//
// krpc.NodeAddr has no room for an IPv6 zone, and `[fe80::1]:6881` without one is
// "connect: invalid argument" — the same gap P03 hit resolving a discovered peer. The
// library will hold such a node happily (BEP-42 counts link-local as local, so the
// security check passes), so it would spend one of at most sixteen probe slots on a
// query that cannot leave the machine.
func TestAZonelessLinkLocalNodeIsNotProbed(t *testing.T) {
	n := newNode(t)
	add := func(ip string, port int) {
		var ni krpc.NodeInfo
		ni.ID[19] = byte(port)
		ni.Addr = krpc.NodeAddr{IP: net.ParseIP(ip), Port: port}
		if err := n.rz.dht.AddNode(ni); err != nil {
			t.Fatalf("AddNode(%s): %v", ip, err)
		}
	}
	add("fe80::1", 6881)
	add("127.40.0.1", 6882)

	// Stimulus: the table really holds both, so an absence below is a decision this
	// code made and not a node that was never there.
	if got := len(n.rz.Nodes()); got != 2 {
		t.Fatalf("setup: the routing table holds %d node(s), want 2", got)
	}
	targets := n.rz.probeTargets()
	if len(targets) != 1 {
		t.Fatalf("probeTargets returned %d target(s), want 1: %v", len(targets), targets)
	}
	if targets[0].IP.To4() == nil {
		t.Errorf("probeTargets kept %v — a zoneless link-local address cannot be dialled, "+
			"and asking it spends a slot on a query that cannot be sent", targets[0])
	}
}

// TestIsPublishableAgainstTheRangesGoDoesNotCover.
//
// `netip.Addr.IsGlobalUnicast` excludes only the unspecified address, loopback,
// multicast and link-local. Every row in the reject half below passes it — which is how
// the first version of isPublishable let 0.0.0.0/8, 240/4 and the 6to4 relay anycast
// prefix through as candidates both sides would punch at.
func TestIsPublishableAgainstTheRangesGoDoesNotCover(t *testing.T) {
	reject := []string{
		"0.1.2.3", "240.1.2.3", "255.255.255.254", "192.88.99.1",
		"192.0.2.7", "198.51.100.7", "203.0.113.7", "198.18.9.9",
		"10.1.2.3", "172.16.0.1", "192.168.1.1", "127.0.0.1", "169.254.1.1",
		"100.100.1.2", "0.0.0.0", "224.0.0.1",
		"2001:db8::1", "3fff::1", "2001:0:1::1", "2002::1",
		"64:ff9b::1", "64:ff9b:1::1", "100::1", "5f00::1",
		"fc00::1", "fe80::1", "::1", "::", "ff02::1",
	}
	accept := []string{
		"8.8.8.8", "81.2.69.142", "1.1.1.1", "203.1.113.7",
		"2606:4700:4700::1111", "2001:4860:4860::8888",
	}
	// Stimulus: the accept list is non-empty and really passes, so a green on the
	// reject list is a filter and not a function that returns false for everything.
	for _, s := range accept {
		if !isPublishable(netip.MustParseAddr(s)) {
			t.Errorf("isPublishable(%s) = false — an ordinary public address was refused, "+
				"and a host there could never publish a candidate at all", s)
		}
	}
	for _, s := range reject {
		a := netip.MustParseAddr(s)
		if isPublishable(a) {
			t.Errorf("isPublishable(%s) = true (IsGlobalUnicast reports %v) — this becomes "+
				"a candidate both sides punch hundreds of packets at", s, a.IsGlobalUnicast())
		}
	}
}
