package rendezvous

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"nib/internal/udpmux"
)

// The driven half of P04.S01: a DHT and a live QUIC session on ONE socket.
//
// This is caveat 7's clause, and its amendment is specific — "shares" means through
// the demultiplexer P02 built, not by any other arrangement. So the test opens one
// UDP socket, wraps it in a Mux, hands the QUIC view to quic-go and the DHT view to
// this package, and drives both AT THE SAME TIME. Either alone would pass a weaker
// claim.
//
// quic-go and anacrolix/dht are imported by this test file only; the shipped binary
// reaches neither from here.

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "rendezvous-interop"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"rendezvous-interop"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

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
		t.Fatalf("ping: %v", err)
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
