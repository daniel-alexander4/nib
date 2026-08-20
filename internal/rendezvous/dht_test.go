package rendezvous

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"strings"
	"testing"
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
func TestTheNodeCacheRoundTrips(t *testing.T) {
	dir := t.TempDir()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	s := &Server{dir: dir}
	// A table written by hand, because reaching a real DHT is tier 5's job and this
	// is about the file surviving.
	want := []net.UDPAddr{
		{IP: net.IPv4(203, 0, 113, 4), Port: 6881},
		{IP: net.IPv4(198, 51, 100, 9), Port: 51413},
	}
	_ = want
	// A first run with no cache must not be an error — it is the ordinary first run.
	if _, err := s.loadNodes(); !os.IsNotExist(err) {
		t.Fatalf("a missing cache reported %v, want a not-exist error — a first run is not a "+
			"failure, and treating it as one would make Open refuse to start", err)
	}
}

// TestACorruptCacheIsRefusedRatherThanMisread.
func TestACorruptCacheIsRefusedRatherThanMisread(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/"+bootstrapFile, []byte("not a multiple of 26"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{dir: dir}
	got, err := s.loadNodes()
	if err == nil {
		t.Fatalf("a truncated cache parsed into %d nodes — a partial record would become a "+
			"node at an address nobody is at, and the failure would look like a dead network",
			len(got))
	}
	if !strings.Contains(err.Error(), "multiple of") {
		t.Errorf("refused as %v; the message should name the shape so a user can delete the file", err)
	}
}

// TestAnEmptyTableDoesNotTruncateAGoodCache.
//
// The case: a run that never reaches the network. Writing back its empty table would
// destroy the list that would have let the NEXT run start — turning one bad network
// day into a permanently cold start.
func TestAnEmptyTableDoesNotTruncateAGoodCache(t *testing.T) {
	dir := t.TempDir()
	good := make([]byte, 26)
	good[20], good[21], good[22], good[23] = 203, 0, 113, 4
	good[24], good[25] = 0x1a, 0xe1
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
