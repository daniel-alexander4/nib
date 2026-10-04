package server

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net"
	"net/netip"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"nib/internal/rendezvous"
)

// /pending 699 — no test in this package reaches the public DHT.
//
// CONTRIBUTING.md names `build/dhtlive.sh` as the one harness that leaves the machine. This
// package's ceremonies opened their rendezvous through production's `rendezvous.Open` over an
// EMPTY node cache (`armedCeremony` gives each one a fresh config directory), and an empty cache
// is exactly the case in which `Open` starts from the shipped seeds. So every test that bootstraps
// for real — `TestTheAddedLatencyToTheDHTTierIsMeasured` is the plainest — sent KRPC queries to
// strangers on every `go test ./...`: measured with strace before this fix, datagrams to all five
// shipped IPv4 seeds from three tests in `lazybootstrap_test.go` alone.
//
// # The fix: one hermetic opener for the whole package
//
// `TestMain` points `openRendezvous` — the one opener the server's rendezvous sites call — at
// `openHermeticRendezvous`, which opens through `rendezvous.OpenAdmittingLoopback`: loopback is the
// whole address rule, for the cache AND for every query the socket sends, and the shipped seeds are
// never consulted. A test that wants a DHT to talk to names a loopback node in its cache, as the
// rendezvous-switch sink does.
//
// # The guard: a counter, not a hope
//
// A query aimed off-host is refused at the socket and counted (`Stats.RefusedSends`). Every
// rendezvous the package opened is remembered with the test that opened it, and after the run
// `reportOffHostDHTQueries` fails the binary if any of them tried — naming the test. A refused query
// left nothing on the wire, so this is the attempt being reported, which is what a reviewer needs:
// the next test that goes looking for the internet fails here rather than succeeding quietly.

var hermeticDHT struct {
	mu   sync.Mutex
	open []hermeticEntry
}

type hermeticEntry struct {
	rz *rendezvous.Server
	by string
}

// openHermeticRendezvous is openRendezvous for this package's tests.
func openHermeticRendezvous(conn net.PacketConn, dir string) (*rendezvous.Server, error) {
	rz, err := rendezvous.OpenAdmittingLoopback(conn, dir)
	if err != nil {
		return nil, err
	}
	hermeticDHT.mu.Lock()
	hermeticDHT.open = append(hermeticDHT.open, hermeticEntry{rz: rz, by: openingTest()})
	hermeticDHT.mu.Unlock()
	return rz, nil
}

// openingTest names the Test function on the opener's stack, or says it could not see one (a
// goroutine the test started has no Test frame of its own).
func openingTest() string {
	pcs := make([]uintptr, 64)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		name := f.Function[strings.LastIndex(f.Function, ".")+1:]
		if strings.HasPrefix(name, "Test") && strings.HasSuffix(f.File, "_test.go") {
			return name
		}
		if !more {
			return "(a goroutine with no Test frame)"
		}
	}
}

// offHostDHTQueries sums what every hermetic rendezvous refused to send, by opening test.
func offHostDHTQueries() map[string]uint64 {
	hermeticDHT.mu.Lock()
	defer hermeticDHT.mu.Unlock()
	out := map[string]uint64{}
	for _, e := range hermeticDHT.open {
		if n := e.rz.Stats().RefusedSends; n > 0 {
			out[e.by] += n
		}
	}
	return out
}

// reportOffHostDHTQueries is the run's guard: 0 when no test aimed a DHT query off-host, 1 and the
// offenders otherwise.
func reportOffHostDHTQueries(w io.Writer) int {
	got := offHostDHTQueries()
	if len(got) == 0 {
		return 0
	}
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(w, "FAIL: tests in internal/server aimed DHT queries at addresses off this machine "+
		"(/pending 699). Tier 1 is hermetic — only build/dhtlive.sh leaves the machine. The "+
		"queries were refused at the socket; give the test a loopback node in its cache instead:")
	for _, n := range names {
		fmt.Fprintf(w, "  %s: %d quer(ies)\n", n, got[n])
	}
	return 1
}

// TestATestCeremonyNeverStartsFromTheShippedSeeds drives the case /pending 699 measured: a real
// ceremony over an empty cache, bootstrapping for real.
//
// Its stimulus is the bootstrap reaching the door and the traversal starting at all; its assertions
// are that the shipped seeds were never consulted and that no query was aimed off-host.
func TestATestCeremonyNeverStartsFromTheShippedSeeds(t *testing.T) {
	cer, _ := armedCeremony(t)
	_ = cer.ensureBootstrapped(t.Context())
	if !cer.bootstrapDone.Load() {
		t.Fatal("setup: the bootstrap door was never reached, so nothing below was exercised")
	}
	st := cer.rz.Stats()
	if st.Seeds != 0 {
		t.Errorf("a test ceremony over an empty node cache started from %d shipped seed(s) — the "+
			"public DHT, on every go test run (/pending 699)", st.Seeds)
	}
	if st.RefusedSends != 0 {
		t.Errorf("a test ceremony aimed %d DHT quer(ies) off this machine (/pending 699)", st.RefusedSends)
	}
}

// TestTheOffHostReportNamesTheTest proves the run-level guard can fail: a hermetic rendezvous
// pointed at a public address must be reported, by name.
func TestTheOffHostReportNamesTheTest(t *testing.T) {
	cer, _ := armedCeremony(t)
	// 192.0.2.0/24 is TEST-NET-1: documentation space, never routed, and outside loopback — so the
	// socket must refuse it, which is the refusal the report counts.
	cer.rz.Seed([]netip.AddrPort{netip.MustParseAddrPort("192.0.2.1:6881")})
	_ = cer.ensureBootstrapped(t.Context())
	if cer.rz.Stats().RefusedSends == 0 {
		t.Fatal("a rendezvous seeded with a public address sent it no query — either it reached the " +
			"network (the opener is not hermetic) or nothing was attempted, and the report below " +
			"would be checking nothing")
	}
	var b strings.Builder
	if reportOffHostDHTQueries(&b) != 1 || !strings.Contains(b.String(), t.Name()) {
		t.Fatalf("the run-level guard did not report an off-host query by this test: %q", b.String())
	}
	// This test's own refusal is the stimulus, not a finding: forget it so the run stays green.
	hermeticDHT.mu.Lock()
	kept := hermeticDHT.open[:0]
	for _, e := range hermeticDHT.open {
		if e.by != t.Name() {
			kept = append(kept, e)
		}
	}
	hermeticDHT.open = kept
	hermeticDHT.mu.Unlock()
}

// TestEveryTestRendezvousOpensHermetically is the routing half (ADR-009's shape): a test file that
// calls `rendezvous.Open` itself, or points `openRendezvous` anywhere, walks around the guard above.
// `live_test.go` is the named exemption — its tests skip unless the live-DHT variable is set, which
// only `build/dhtlive.sh` does.
func TestEveryTestRendezvousOpensHermetically(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	exempt := map[string]string{
		"live_test.go": "gated on the live-DHT variable, run only by build/dhtlive.sh",
	}
	var files int
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			files++
			if _, ok := exempt[path]; ok {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && id.Name == "rendezvous" &&
						strings.HasPrefix(x.Sel.Name, "Open") && path != "hermeticdht_test.go" {
						t.Errorf("%s: %s opens a rendezvous around openRendezvous — go through "+
							"openRendezvous, which TestMain makes hermetic (/pending 699)",
							fset.Position(x.Pos()), "rendezvous."+x.Sel.Name)
					}
				case *ast.AssignStmt:
					for _, l := range x.Lhs {
						if id, ok := l.(*ast.Ident); ok && id.Name == "openRendezvous" && path != "helpers_test.go" {
							t.Errorf("%s: assigns openRendezvous — TestMain owns it, and a test that "+
								"restores it restores production's seeds (/pending 699)", fset.Position(x.Pos()))
						}
					}
				}
				return true
			})
		}
	}
	if files < 100 {
		t.Fatalf("setup: parsed only %d test files — the census is not reading this package", files)
	}
}
