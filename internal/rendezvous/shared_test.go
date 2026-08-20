package rendezvous

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"nib/internal/udpmux"
)

// Shared fixtures for this package's driven tests.
//
// **The DHT-and-QUIC-on-one-socket clause is NOT here**, and this comment used to say it
// was — it described a test that opens one socket, hands the QUIC view to quic-go and the
// DHT view to this package, and drives both at once. No such test is in this file; it
// lives in internal/udpmux, for the reason set out below. A doc comment describing
// verification that is somewhere else reads exactly like verification that is here, which
// is the shape l1_test.go's header was written to punish. The dead `selfSigned` helper
// that test would have needed was removed with it (P04.S03).

type node struct {
	mux  *udpmux.Mux
	rz   *Server
	dir  string
	addr *net.UDPAddr
}

func newNode(t *testing.T) *node {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := udpmux.New(pc)
	dir := t.TempDir()
	rz, err := Open(m.DHT(), dir)
	if err != nil {
		m.Close()
		t.Fatalf("open rendezvous: %v", err)
	}
	t.Cleanup(func() { rz.Close(); m.Close() })
	return &node{mux: m, rz: rz, dir: dir, addr: m.LocalAddr().(*net.UDPAddr)}
}

// The DHT-and-QUIC-on-one-socket clause is driven in internal/udpmux, not here.
//
// It needs the connection-ID generator the product wires (internal/p2p's cidGen), and
// this package must not import internal/p2p — L1, and the package doc says so. A test
// that hand-rolled its own generator would be driving a stand-in for the wiring, which
// is the part most worth driving. So the rule lives where the real libraries and the
// real generator already meet: TestKRPCFromAQUICPeerReachesTheDHT.

// TestTheNodeCacheSurvivesARestart is D6's "populated on first contact" across the
// boundary a single run cannot show.
func TestTheNodeCacheSurvivesARestart(t *testing.T) {
	a, b := newNode(t), newNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.rz.Ping(ctx, b.addr); err != nil {
		// Both muxes, because "ping timed out" names no side and this failure has
		// already had three different causes. RoutedToDHT on B with nothing on A is a
		// reply that was never sent — which is what a drained send limiter looks like.
		t.Fatalf("ping: %v\n  A mux: %+v\n  B mux: %+v\n  screened: A=%d B=%d",
			err, a.mux.Stats(), b.mux.Stats(),
			a.rz.Stats().Screened, b.rz.Stats().Screened)
	}
	// Stimulus: A really learned a node, so "the cache has one" is not true of a
	// table that was always empty.
	if n := a.rz.Stats().Nodes; n == 0 {
		t.Fatal("A learned no nodes from a successful ping; nothing below is meaningful")
	}
	if a.rz.Stats().Loaded != 0 {
		t.Fatalf("a first run loaded %d nodes from a cache that should not exist yet",
			a.rz.Stats().Loaded)
	}

	if err := a.rz.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(a.dir + "/" + bootstrapFile); err != nil {
		t.Fatalf("no cache was written: %v", err)
	}

	// Restart on a NEW socket, same directory — which is what a restart is.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m2 := udpmux.New(pc)
	defer m2.Close()
	again, err := Open(m2.DHT(), a.dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()

	if got := again.Stats().Loaded; got == 0 {
		t.Fatalf("the restart loaded %d nodes from the cache — D6's bootstrap is a cached "+
			"node list, and a cache nothing reads is a hostname bootstrap wearing a "+
			"different name", got)
	}
	t.Logf("RESTART loaded %d node(s) from the cache", again.Stats().Loaded)
}
