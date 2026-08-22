package rendezvous

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/anacrolix/dht/v2/krpc"

	"nib/internal/udpmux"
)

// TestBootstrapResolvesNoHostname is D6's clause as a guard, and it is a guard rather
// than a counter on purpose.
//
// The first draft of this file carried a `Resolutions` counter incremented by
// `Add(0)` — a number that could only ever be zero, with a comment arguing that made
// it worth asserting. That is the vacuous instrument exactly: asserting a constant
// proves nothing about the code that is supposed to keep it constant.
//
// What can actually fail is a reference. D6 forbids hostname bootstrap because a DNS
// lookup is a third party learning who starts a ceremony and when, and on a network
// that blocks or rewrites DNS it is the single point where the ladder fails before it
// begins. So: this package must not name a resolver, and must not reach the library's
// hostname bootstrap.
func TestBootstrapResolvesNoHostname(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0) // no ParseComments — a comment must not be able to satisfy this
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["rendezvous"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("no non-test files parsed — the checks below would pass on nothing")
	}

	// Stimulus: this package really does construct a DHT server, so "it never
	// resolves" is a fact about a bootstrap path and not about an empty package.
	constructs := false
	forbidden := map[string]string{
		"LookupHost":             "a DNS lookup on the bootstrap path",
		"LookupIP":               "a DNS lookup on the bootstrap path",
		"ResolveUDPAddr":         "resolves a host:port string, which may be a hostname",
		"ResolveTCPAddr":         "resolves a host:port string, which may be a hostname",
		"GlobalBootstrapAddrs":   "the library's hostname bootstrap — exactly what D6 forbids",
		"NewDefaultServerConfig": "the default config, whose StartingNodes IS the hostname bootstrap",
		// AddNode fires `go s.Ping(addr)` for any zero-id node (server.go:391-394), with a
		// context Ping discards — one uncancellable goroutine per seed, all at once. An
		// invitation may carry eight, and a hostile one would carry eight it chose.
		// StartingNodes is the path Open already uses and is evaluated lazily.
		"AddNode": "spawns an uncancellable ping goroutine per node; seeds go through StartingNodes",
	}
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "NewServer" {
				constructs = true
			}
			if why, bad := forbidden[sel.Sel.Name]; bad {
				t.Errorf("%s calls %s — %s", name, sel.Sel.Name, why)
			}
			return true
		})
	}
	if !constructs {
		t.Fatal("nothing here constructs a DHT server; the bootstrap checks are vacuous")
	}
}

// TestTheDHTIsNeverGivenANilConn is caveat 7 as a guard.
//
// dht.NewServer opens its OWN socket when Conn is nil (server.go:1046) — so the
// failure this prevents is not a compile error or a panic, it is a DHT that works
// perfectly on a socket the session will never use, and a self-address probe that
// measures a NAT mapping belonging to nothing.
func TestTheDHTIsNeverGivenANilConn(t *testing.T) {
	if _, err := Open(nil, t.TempDir()); err == nil {
		t.Fatal("Open accepted a nil connection — dht.NewServer would have opened its own " +
			"socket, and caveat 7 is that the mapped port, the DHT probe and the live " +
			"session must be ONE socket")
	}
}

// TestTheNodeCacheRoundTrips — D6's "populated on first contact" needs the cache to
// survive a restart, which is the half a single run cannot show.
// Renamed at P04.S03 from TestTheNodeCacheRoundTrips, which is what it claimed and not
// what it did: the body asserts one thing — that a missing cache is not an error — and
// would have stayed green with writeNodes and the whole load path deleted. It also opened
// a UDP socket it never used, the fingerprint of a rewrite that dropped the body. The
// round trip it was named for IS covered, by TestTheCacheCarriesIPv6 and
// TestTheNodeCacheSurvivesARestart; what was missing was a test whose name matched it.
func TestAMissingNodeCacheIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	s := &Server{dir: dir}
	// A first run with no cache must not be an error — it is the ordinary first run.
	if _, err := s.loadNodes(); !os.IsNotExist(err) {
		t.Fatalf("a missing cache reported %v, want a not-exist error — a first run is not a "+
			"failure, and treating it as one would make Open refuse to start", err)
	}
}

// TestACorruptCacheIsRefusedRatherThanMisread — both doors, because they are different
// failures with different messages.
func TestACorruptCacheIsRefusedRatherThanMisread(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"no magic at all", "not a cache", "does not begin with"},
		{"our magic, a truncated record", cacheMagic + "half a record", "multiple of"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(dir+"/"+bootstrapFile, []byte(c.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := (&Server{dir: dir}).loadNodes()
			if err == nil {
				t.Fatalf("a corrupt cache parsed into %d nodes — a partial record becomes a "+
					"node at an address nobody is at, and the failure looks like a dead network",
					len(got))
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("refused as %v; the message should say %q so a user can act on it",
					err, c.want)
			}
		})
	}
}

// TestTheOldCacheLayoutIsRefusedRatherThanReinterpreted.
//
// The reason the file carries a magic string at all. The previous layout was 26 bytes
// per record and this one is 38, and those collide at 494 bytes — so a cache holding
// exactly 19 old records is a whole number of new ones and parses without complaint into
// 13 node addresses read from the wrong offsets.
//
// A refusal would be merely wasteful. Silent misparsing is worse, and specifically so:
// the resulting list is NON-EMPTY, which is the condition under which Open declines to
// use the seed addresses — so the run bootstraps from 13 fictions and cannot recover
// from a cold start it would otherwise have survived.
func TestTheOldCacheLayoutIsRefusedRatherThanReinterpreted(t *testing.T) {
	const oldRecord = 26
	dir := t.TempDir()
	body := make([]byte, 19*oldRecord) // 494 bytes: 19 old records, 13 new ones exactly
	for i := 0; i < 19; i++ {
		body[i*oldRecord+20], body[i*oldRecord+21] = 203, 0
		body[i*oldRecord+22], body[i*oldRecord+23] = 113, byte(i+1)
		body[i*oldRecord+24], body[i*oldRecord+25] = 0x1a, 0xe1
	}
	// Stimulus: the length really is the collision, so a refusal below is the magic
	// doing its job and not an off-by-one in the fixture.
	if len(body)%nodeRecord != 0 {
		t.Fatalf("setup: %d bytes is not a whole number of %d-byte records, so the old "+
			"layout could not be misread as the new one anyway", len(body), nodeRecord)
	}
	if err := os.WriteFile(dir+"/"+bootstrapFile, body, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := (&Server{dir: dir}).loadNodes()
	if err == nil {
		t.Fatalf("19 records in the old layout parsed as %d nodes in the new one — every "+
			"one of them is bytes read from the wrong offsets, and a non-empty list is "+
			"exactly what stops Open from falling back to the seeds", len(got))
	}
}

// TestAnEmptyTableDoesNotTruncateAGoodCache.
//
// The case: a run that never reaches the network. Writing back its empty table would
// destroy the list that would have let the NEXT run start — turning one bad network
// day into a permanently cold start.
func TestAnEmptyTableDoesNotTruncateAGoodCache(t *testing.T) {
	dir := t.TempDir()
	good := append([]byte(cacheMagic), make([]byte, nodeRecord)...)
	rec := good[len(cacheMagic):]
	copy(rec[20:36], net.IPv4(203, 0, 113, 4).To16())
	rec[36], rec[37] = 0x1a, 0xe1
	path := dir + "/" + bootstrapFile
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	// Stimulus: the cache really is readable before the save.
	s := &Server{dir: dir}
	before, err := s.loadNodes()
	if err != nil || len(before) != 1 {
		t.Fatalf("setup: the cache does not load (%d nodes, %v)", len(before), err)
	}

	if n, err := writeNodes(dir, nil); err != nil || n != 0 {
		t.Fatalf("writeNodes(empty) = %d, %v", n, err)
	}
	after, err := s.loadNodes()
	if err != nil || len(after) != 1 {
		t.Fatalf("after saving an empty table the cache holds %d nodes (%v) — a run that "+
			"never reached the network destroyed the list the next run needed", len(after), err)
	}
}

// TestTheCacheCarriesIPv6.
//
// The defect this replaces: the record was the 6-byte compact form BEP-5 uses on the
// wire, so writeNodes skipped every node without an IPv4 form — and the "an empty table
// must not truncate a good cache" rule then meant a **v6-only host wrote no cache at
// all, ever**, and cold-started on every run. Nothing saw it, because the only test that
// exercised the cache ran on 127.0.0.1.
func TestTheCacheCarriesIPv6(t *testing.T) {
	dir := t.TempDir()
	v6 := net.ParseIP("2001:db9::1")
	v4 := net.IPv4(203, 0, 113, 4)
	in := []krpc.NodeInfo{
		{Addr: krpc.NodeAddr{IP: v6, Port: 6881}},
		{Addr: krpc.NodeAddr{IP: v4, Port: 51413}},
	}
	for i := range in {
		in[i].ID[19] = byte(i + 1)
	}
	n, err := writeNodes(dir, in)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("writeNodes wrote %d of 2 nodes — the v6 one was dropped, which on a "+
			"v6-only host means no cache is ever written and every run is a cold start", n)
	}
	out, err := (&Server{dir: dir}).loadNodes()
	if err != nil || len(out) != 2 {
		t.Fatalf("loadNodes = %d nodes, %v; want 2", len(out), err)
	}
	// net.IP.Equal, not bytes: a v4 address survives as its 16-byte v4-mapped form and
	// a byte comparison would call that a different address.
	if !out[0].Addr.IP.Equal(v6) || out[0].Addr.Port != 6881 {
		t.Errorf("v6 node round-tripped as %v:%d, want %v:6881", out[0].Addr.IP, out[0].Addr.Port, v6)
	}
	if !out[1].Addr.IP.Equal(v4) || out[1].Addr.Port != 51413 {
		t.Errorf("v4 node round-tripped as %v:%d, want %v:51413", out[1].Addr.IP, out[1].Addr.Port, v4)
	}
}

// TestAnUnreadableCacheIsAColdStartNotARefusalToRun.
//
// Open's comment said "a corrupt cache is not fatal — it is a first run with extra
// steps" from the day it was written, and the code under it returned an error. So a
// single damaged byte in ~/nib/dht-nodes stopped the rendezvous from starting at all,
// and the comment asserting otherwise is exactly the kind of claim that reads as
// verification.
func TestAnUnreadableCacheIsAColdStartNotARefusalToRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/"+bootstrapFile, []byte("half a record"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Stimulus: the file really is unreadable, so a successful Open below is tolerance
	// and not a cache that happened to parse.
	if _, err := (&Server{dir: dir}).loadNodes(); err == nil {
		t.Fatal("setup: the corrupt cache parsed, so there is nothing to tolerate")
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// NO `defer pc.Close()` here: `udpmux.New` states "New takes ownership of pc… Closing the
	// Mux closes pc" (mux.go), so closing both closes the socket twice. Harmless in fact — the
	// second returns an error nobody reads — and wrong in the way that teaches the next reader
	// the ownership contract does not mean what it says.
	m := udpmux.New(pc)
	defer m.Close()
	s, err := Open(m.DHT(), dir)
	if err != nil {
		t.Fatalf("Open refused to start over a corrupt cache: %v — one damaged byte in a "+
			"bootstrap hint file must not be the reason Nib cannot open a document", err)
	}
	defer s.Close()
	if !s.Stats().CacheRejected {
		t.Error("CacheRejected is false — the run continued, but 'no cache' and 'a cache I " +
			"could not read' are now indistinguishable, and they want different advice")
	}
	if s.Stats().Seeds == 0 {
		t.Error("Seeds = 0 after an unreadable cache — a cold start with nowhere to start is " +
			"the state D6's amendment exists to prevent")
	}
}
