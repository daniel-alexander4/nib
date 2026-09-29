package rendezvous

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"

	"nib/internal/addrscope"
	"nib/internal/udpmux"
)

// goodCache is n cached nodes at in-scope public addresses.
//
// **Nothing here ever sends them a packet** (/pending 699: tests stay off the public DHT).
// StartingNodes is read only by a traversal, and no test below bootstraps, publishes or
// fetches; each asserts `Bootstrapped == 0` so that stays true rather than assumed.
func goodCache(n int) []krpc.NodeInfo {
	out := make([]krpc.NodeInfo, 0, n)
	for i := 0; i < n; i++ {
		var ni krpc.NodeInfo
		ni.ID[0], ni.ID[1], ni.ID[19] = byte(i>>8), byte(i), 0x77
		ni.Addr = krpc.NodeAddr{IP: net.IPv4(45, 1, byte(i>>8), byte(i)+1), Port: 6881}
		out = append(out, ni)
	}
	return out
}

// openProduction opens a Server through `Open` — production's cache rule, not the tests'.
func openProduction(t *testing.T, cached []krpc.NodeInfo) *node {
	t.Helper()
	dir := t.TempDir()
	if len(cached) > 0 {
		if _, err := writeNodes(dir, cached); err != nil {
			t.Fatal(err)
		}
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := udpmux.New(pc)
	rz, err := Open(m.DHT(), dir)
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { rz.Close(); m.Close() })
	return &node{mux: m, rz: rz, dir: dir, addr: m.LocalAddr().(*net.UDPAddr)}
}

// TestOneInboundPingDoesNotReplaceAGoodNodeCache is /pending 707, the reviewer's measurement
// turned into its own regression.
//
// anacrolix adds any node that queries us to the routing table, BEP-42 exempts loopback and
// private sources, and `saveNodes` wrote the table OVER the cache. Under ADR-011's lazy bootstrap
// a ceremony the link answers never traverses, so Open → one inbound query → Close is the
// ordinary shape — and it left the cache holding one node: the stranger. Measured before the
// fix: Loaded=200, Bootstrapped=0, persisted 1 node, `127.0.0.1:<port>`.
func TestOneInboundPingDoesNotReplaceAGoodNodeCache(t *testing.T) {
	good := goodCache(200)
	victim := openProduction(t, good)
	stranger := newNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := stranger.rz.Ping(ctx, victim.addr); err != nil {
		t.Fatalf("setup: the stranger's ping was not answered: %v", err)
	}

	// STIMULUS: the cache loaded whole, nothing traversed, and the stranger IS in the table —
	// without the last, a save that writes nothing new would pass for the wrong reason.
	st := victim.rz.Stats()
	if st.Loaded != len(good) || st.Bootstrapped != 0 || st.Seeds != 0 {
		t.Fatalf("setup: Loaded=%d Bootstrapped=%d Seeds=%d, want %d/0/0", st.Loaded, st.Bootstrapped, st.Seeds, len(good))
	}
	if st.Nodes == 0 {
		t.Fatal("setup: the inbound ping did not put the stranger in the routing table, so the save under test has nothing hostile to write")
	}

	if err := victim.rz.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := (&Server{dir: victim.dir}).loadNodes()
	if err != nil {
		t.Fatalf("the cache does not load after the close: %v", err)
	}
	have := make(map[netip.AddrPort]bool, len(after))
	for _, ni := range after {
		ap, _ := nodeAddrPort(ni)
		have[ap] = true
	}
	kept := 0
	for _, ni := range good {
		if ap, _ := nodeAddrPort(ni); have[ap] {
			kept++
		}
	}
	if kept != len(good) {
		t.Errorf("after one inbound ping the persisted cache holds %d of the %d good nodes (%d total) — "+
			"a stranger's single datagram replaced the list the next run bootstraps from", kept, len(good), len(after))
	}
	strangerAP := netip.MustParseAddrPort(stranger.addr.String())
	if have[strangerAP] {
		t.Errorf("the stranger %v was written to the node cache — a loopback address the bootstrap rule "+
			"(addrscope.Seed) refuses everywhere else", strangerAP)
	}
}

// TestACacheOfOutOfScopeNodesIsAColdStart is the load half: a cache already poisoned — by the
// pre-707 save, or by anything else that wrote the file — reads as empty, so the shipped list is
// consulted and its rot alarm (`Bootstrapped == 0 while Seeds > 0`) can fire. Before the fix it
// loaded as a warm cache: `Seeds` 0, and every run started from the stranger's address.
func TestACacheOfOutOfScopeNodesIsAColdStart(t *testing.T) {
	var poisoned []krpc.NodeInfo
	for i, a := range []string{"127.0.0.1:39340", "192.168.1.10:6881", "10.0.0.1:6881", "45.1.2.3:80"} {
		var ni krpc.NodeInfo
		ni.ID[0] = byte(i + 1)
		ap := netip.MustParseAddrPort(a)
		ni.Addr = krpc.NodeAddr{IP: net.IP(ap.Addr().AsSlice()), Port: int(ap.Port())}
		poisoned = append(poisoned, ni)
	}
	n := openProduction(t, poisoned)
	st := n.rz.Stats()
	if st.Loaded != 0 {
		t.Errorf("Loaded = %d from a cache of only loopback, private and low-port nodes — a cache the "+
			"bootstrap rule refuses entry by entry was read as a warm one", st.Loaded)
	}
	if st.Seeds == 0 {
		t.Error("Seeds = 0 over an all-out-of-scope cache — the shipped list is never reached, and the " +
			"rot alarm can never fire, on the machine a poisoned cache has stranded")
	}
	if st.Bootstrapped != 0 {
		t.Fatalf("Bootstrapped = %d — this test must not traverse (/pending 699)", st.Bootstrapped)
	}
}

// TestTheCacheSaveMergesAndCaps pins cacheSet directly: a public stranger in the table is kept
// (it may be a real node, and it is in scope) but removes nothing; out-of-scope entries on EITHER
// side are dropped; duplicates collapse; the cap holds; and a table with nothing in scope reports
// fresh == 0 so the caller leaves the file alone.
func TestTheCacheSaveMergesAndCaps(t *testing.T) {
	prev := goodCache(200)
	var stranger krpc.NodeInfo
	stranger.ID[0] = 0xee
	stranger.Addr = krpc.NodeAddr{IP: net.IPv4(46, 2, 3, 4), Port: 6881}
	var loop krpc.NodeInfo
	loop.Addr = krpc.NodeAddr{IP: net.IPv4(127, 0, 0, 1), Port: 39340}

	out, fresh := cacheSet([]krpc.NodeInfo{stranger, loop, prev[0]}, append(prev, loop), addrscope.Seed)
	if fresh != 2 {
		t.Errorf("fresh = %d, want 2 (the public stranger and prev[0]; the loopback node is out of scope)", fresh)
	}
	if len(out) != 201 {
		t.Errorf("merged cache holds %d nodes, want 201 — the 200 previous plus one stranger, deduplicated", len(out))
	}
	for _, ni := range out {
		if ap, _ := nodeAddrPort(ni); !addrscope.Seed(ap) {
			t.Errorf("out-of-scope node %v survived the save", ap)
		}
	}

	if _, fresh := cacheSet([]krpc.NodeInfo{loop}, prev, addrscope.Seed); fresh != 0 {
		t.Errorf("a table holding only a loopback node reported fresh = %d — the save would rewrite the cache for nothing it may keep", fresh)
	}

	big, _ := cacheSet(goodCache(300), goodCache(600), addrscope.Seed)
	if len(big) != maxCachedNodes {
		t.Errorf("merged cache holds %d nodes, want the cap %d", len(big), maxCachedNodes)
	}
}
