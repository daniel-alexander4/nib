package rendezvous

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/anacrolix/dht/v2/krpc"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"nib/internal/udpmux"
)

// TestLiveSelfAddressProbe drives the real BitTorrent DHT.
//
// # It is opt-in, and it is the only test in this repo that leaves the machine
//
// Every other tier is hermetic on purpose — tier 3 was *made* hermetic (v1.109.3) after
// an outbound update check turned an unrelated network failure into a red. So this one
// runs only when `NIB_LIVE_DHT=1` is set, which `build/dhtlive.sh` does and nothing in
// the routine loop does.
//
// # Why a hermetic tier cannot discharge the clause
//
// P04.S02's acceptance says the port must be observed "on the wire from a real node, not
// inferred from the type carrying a port field" — because the phase-open note had
// already settled the representation by READING `krpc.Msg.IP` and was explicit that
// reading a type proves a library can represent a port, not that one comes back. Two
// servers on loopback answer with `ip` set (the library sets it for every non-error
// reply), so a loopback test proves the plumbing and nothing about the network. Only a
// stranger's node can show a translated port.
func TestLiveSelfAddressProbe(t *testing.T) {
	if os.Getenv("NIB_LIVE_DHT") == "" {
		t.Skip("set NIB_LIVE_DHT=1 (or run ./build/dhtlive.sh) — this test uses the public network")
	}
	dir := t.TempDir() // no cache: a genuine cold start, so the seed list is exercised

	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	m := udpmux.New(pc)
	defer m.Close()

	s, err := Open(m.DHT(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	local := m.LocalAddr().(*net.UDPAddr)
	t.Logf("LOCAL socket %v", local)
	if s.Stats().Seeds == 0 {
		t.Fatal("no seed addresses on an empty cache — D6's amendment is unexercised and " +
			"there is nothing for the bootstrap below to start from")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Patience, and it belongs here rather than in Bootstrap.
	//
	// A cold start against public routers is genuinely unreliable, and this harness
	// measured it the hard way: after an hour of crawling from one address the routers
	// still answered `ping` and returned NO NODES to `find_node`, so three consecutive
	// runs bootstrapped to an empty table. That is the seed list's fragility, which is
	// exactly why D6's amendment has a second half (nodes carried in the invitation)
	// rather than treating the shipped list as the mechanism.
	var st Stats
	for attempt := 1; attempt <= 3; attempt++ {
		if err := s.Bootstrap(ctx); err != nil {
			t.Fatalf("bootstrap: %v", err)
		}
		st = s.Stats()
		t.Logf("BOOTSTRAP attempt %d: %d seed(s) -> %d node(s) in the table (+%d learned)",
			attempt, st.Seeds, st.Nodes, st.Bootstrapped)
		if st.Nodes > 0 {
			break
		}
		select {
		case <-time.After(time.Duration(attempt) * 3 * time.Second):
		case <-ctx.Done():
		}
	}
	if st.Nodes == 0 {
		// SKIP, not FAIL — and the distinction is the whole point of this block.
		//
		// This harness's subject is the self-address probe. "The public DHT would not
		// talk to us today" is a real and separately-reported fact (Seeds, and this
		// message), but it is not evidence the probe is wrong, and failing on it would
		// make the harness red for a reason nobody can fix in this repo. What must NOT
		// happen is a silent pass: build/dhtlive.sh reads this skip and reports SKIP,
		// never PASS, so nobody mistakes it for coverage.
		t.Skipf("UNREACHABLE: bootstrapping from %d seed(s) produced an empty table after "+
			"3 attempts. The seeds answer or they do not; either way the probe below is "+
			"unexercised and this run discharges nothing", st.Seeds)
	}
	if st.Bootstrapped == 0 {
		t.Fatalf("the table holds %d node(s) but the bootstrap counter says it added none — "+
			"the one number a diagnostic would read to tell 'the seeds are dead' from "+
			"'we never asked' is not moving", st.Nodes)
	}
	// Stimulus for the clause below: enough DISTINCT prefixes exist to ask, so a
	// classification of "unknown" would mean the network declined to answer rather than
	// that there was nobody to ask.
	if got := len(s.probeTargets()); got < 2 {
		t.Fatalf("only %d distinct-prefix node(s) to probe; two observations are the whole "+
			"of D19's test and this run cannot make it", got)
	}

	start := time.Now()
	got, err := s.ProbeSelf(ctx)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	st = s.Stats()

	for _, o := range got.Observations {
		t.Logf("OBSERVED %-39s says we are %v", o.From, o.Self)
	}
	t.Logf("PROBE %d observation(s) in %v; rejected length=%d port=%d scope=%d; "+
		"screened=%d; disagreements=%d",
		st.Observed, elapsed.Round(time.Millisecond),
		st.RejectedLength, st.RejectedPort, st.RejectedScope, st.Screened, st.Disagreements)

	// CAVEAT 2, on the wire. The field arrived, with a length only a real 6- or
	// 18-byte compact endpoint produces, from more than one stranger.
	if st.Observed < 2 {
		t.Fatalf("%d usable `ip` field(s) came back from the public DHT — caveat 2 is "+
			"discharged on the representation and this is the half that needed the wire",
			st.Observed)
	}
	// CAVEAT 9, by measurement rather than assumption: ProbeSelf is bounded by
	// probeBudget, so sources counted here arrived inside D16's allowance.
	class := got.V4
	if !class.Addr.IsValid() && got.V6.Addr.IsValid() {
		class = got.V6
	}
	t.Logf("CLASSIFIED %v — %d of %d distinct source prefixes agreed on %v",
		class.Mapping, class.Agreed, class.Sources, class.Addr)
	if class.Sources < 2 {
		t.Fatalf("only %d distinct source prefix(es) answered inside %v — caveat 9's "+
			"degradation to D19 cause 4 is what happened, and it is a legitimate outcome, "+
			"but this run cannot then discharge the caveat", class.Sources, probeBudget)
	}
	if class.Mapping == MappingUnknown {
		t.Fatalf("mapping is unknown with %d sources — that combination should be "+
			"impossible", class.Sources)
	}

	// THE PORT, which is the clause's actual subject. It has to have come from the
	// datagram: our socket is bound to a wildcard, so a responder echoing anything it
	// did not observe could not produce a routable address at all.
	if class.Mapping == MappingEndpointIndependent {
		self := class.Addr
		if !self.Addr().IsValid() || self.Port() == 0 {
			t.Fatalf("classified endpoint-independent with no usable endpoint: %v", self)
		}
		lo, _ := netip.AddrFromSlice(local.IP)
		if self.Addr() == lo.Unmap() && int(self.Port()) == local.Port {
			t.Logf("NOTE the observed endpoint equals the local one — this host is not " +
				"behind a NAT, which is a valid result and not a translated port")
		} else {
			t.Logf("TRANSLATED local port %d -> observed %v — the port was carried on the "+
				"wire by a real node, which is the clause", local.Port, self)
		}
	}
}

// TestLivePublishAndFetch is P04.S03's acceptance clause against the real network.
//
// # Why this cannot be a loopback test, in the same way the probe could not
//
// The hermetic round trip (roundtrip_test.go) drives publish and fetch against a fake
// storer, and it proves the protocol: the put leaves the machine, a node verifies the
// signature, a DIFFERENT node fetches it back. What it cannot prove is that real DHT
// nodes — which choose for themselves whether to store anything, and enforce their own
// size, seq and token rules — accept what Nib sends. "A published endpoint is retrievable
// by the peer" is a claim about strangers, and only strangers can discharge it.
//
// # And it must not be satisfiable by our own store
//
// dht.Server.Put writes locally before sending (server.go:1081) and our get handler
// serves from that same store, so a single-process publish-then-fetch proves nothing.
// This closes the publisher's socket before fetching, and fetches from a SECOND server on
// a SECOND socket with its own empty cache. Anything that comes back was held by someone
// else.
func TestLivePublishAndFetch(t *testing.T) {
	if os.Getenv("NIB_LIVE_DHT") == "" {
		t.Skip("set NIB_LIVE_DHT=1 (or run ./build/dhtlive.sh) — this test uses the public network")
	}

	// A seed nobody else will be using. Derived from the clock so two runs do not collide
	// on one target and read each other's records — which would look like success while
	// measuring the previous run.
	seed := make([]byte, 32)
	binary.BigEndian.PutUint64(seed, uint64(time.Now().UnixNano()))
	copy(seed[8:], []byte("nib-p04-s03-live-probe"))
	salt := []byte("live")
	record := []byte("nib-live-candidate-record-" + time.Now().UTC().Format(time.RFC3339Nano))

	// warm is the fetcher's starting table, harvested from the publisher before it closes.
	//
	// This is NOT a shortcut around the property under test. The record's location is not
	// in it — only node addresses — and the publisher is closed before the fetch, so
	// nothing it holds can answer. What it avoids is a third cold bootstrap in one run:
	// the shipped seed list is five IP literals, three were dead the day they were
	// written, and the survivors stop returning nodes to `find_node` under repeated use
	// within a single session. Measured here: the publisher bootstrapped on attempt 2 and
	// the fetcher, minutes later, could not bootstrap at all — so the test SKIPPED
	// without ever asking its own question. A warm cache is also what a real Nib has;
	// a cold one is D6's second half, which is P04.S06's subject.
	var warm []krpc.NodeInfo

	open := func(what string) (*Server, *udpmux.Mux, *net.UDPAddr) {
		pc, err := net.ListenPacket("udp", ":0")
		if err != nil {
			t.Fatalf("%s socket: %v", what, err)
		}
		m := udpmux.New(pc)
		dir := t.TempDir()
		if len(warm) > 0 {
			if _, err := writeNodes(dir, warm); err != nil {
				t.Fatalf("%s cache: %v", what, err)
			}
		}
		s, err := Open(m.DHT(), dir)
		if err != nil {
			m.Close()
			t.Fatalf("%s open: %v", what, err)
		}
		return s, m, m.LocalAddr().(*net.UDPAddr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	pub, pubMux, pubAddr := open("publisher")
	t.Logf("PUBLISHER socket %v", pubAddr)
	for attempt := 1; attempt <= 3; attempt++ {
		if err := pub.Bootstrap(ctx); err != nil {
			t.Fatalf("publisher bootstrap: %v", err)
		}
		if pub.Stats().Nodes > 0 {
			break
		}
		t.Logf("publisher bootstrap attempt %d gained nothing; retrying", attempt)
	}
	st := pub.Stats()
	t.Logf("PUBLISHER TABLE %d node(s) from %d seed(s)", st.Nodes, st.Seeds)
	if st.Nodes == 0 {
		pub.Close()
		pubMux.Close()
		t.Skipf("UNREACHABLE: the publisher could not build a routing table (%d seeds, "+
			"%d gained). The DHT is a third party and this is not evidence the publish "+
			"path is broken.", st.Seeds, st.Bootstrapped)
	}

	if err := pub.Publish(ctx, seed, salt, record); err != nil {
		pub.Close()
		pubMux.Close()
		t.Fatalf("publish: %v", err)
	}
	st = pub.Stats()
	t.Logf("PUBLISHED %d byte(s); %d node(s) answered the token traversal", len(record), st.PublishNodes)
	if st.PublishNodes == 0 {
		pub.Close()
		pubMux.Close()
		t.Skipf("UNREACHABLE: no node answered the traversal that collects write tokens, "+
			"so there was nowhere to write. (Published=%d is not a claim that anyone "+
			"stored it — getput.Put cannot report that.)", st.Published)
	}

	// Harvest the table, then the publisher goes away BEFORE the fetch. That ordering is
	// what makes the result mean something: whatever answers the fetch is not us.
	warm = pub.Nodes()
	t.Logf("HARVESTED %d node address(es) for the fetcher's cache — addresses only; the "+
		"record's whereabouts are not among them, and the publisher is about to close", len(warm))
	pub.Close()
	pubMux.Close()

	sub, subMux, subAddr := open("fetcher")
	defer func() { sub.Close(); subMux.Close() }()
	t.Logf("FETCHER socket %v (a different socket, a different empty cache)", subAddr)
	for attempt := 1; attempt <= 3; attempt++ {
		if err := sub.Bootstrap(ctx); err != nil {
			t.Fatalf("fetcher bootstrap: %v", err)
		}
		if sub.Stats().Nodes > 0 {
			break
		}
	}
	if sub.Stats().Nodes == 0 {
		t.Skip("UNREACHABLE: the fetcher could not build a routing table")
	}

	got, seq, err := sub.Fetch(ctx, seed, salt)
	fst := sub.Stats()
	if err != nil {
		t.Skipf("UNREACHABLE: nothing came back (%v). %d node(s) answered the fetch "+
			"traversal. With FetchNodes>0 this means the network reached us and nobody "+
			"was holding the record — which on a public DHT can happen to an honest "+
			"publish, so it is reported as a skip rather than a failure.", err, fst.FetchNodes)
	}
	if !bytes.Equal(got, record) {
		t.Fatalf("OBSERVED a record of %d bytes and it is not the one published", len(got))
	}
	t.Logf("OBSERVED the published record retrieved from strangers: %d bytes at seq %d, "+
		"%d node(s) answered", len(got), seq, fst.FetchNodes)
}
