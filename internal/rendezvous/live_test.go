package rendezvous

import (
	"context"
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
