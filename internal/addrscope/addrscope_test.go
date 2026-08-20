package addrscope

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This package was extracted so two callers could share one table. It owns that table, so
// it owns the tests — before this, its only coverage was internal/rendezvous's delegate
// test, which exercises Routable and never Target, MinPort or SharedSpace.

func TestRoutableRefusesWhatItMust(t *testing.T) {
	refuse := []string{
		// the ordinary shapes
		"0.0.0.0", "127.0.0.1", "10.0.0.1", "192.168.1.1", "172.16.0.1",
		"169.254.1.1", "224.0.0.1", "255.255.255.255", "100.64.0.1",
		"192.0.2.1", "198.51.100.1", "203.0.113.1", "198.18.0.1",
		"192.88.99.1", "240.0.0.1",
		"::1", "::", "fe80::1", "ff02::1", "fc00::1", "fd12:3456::1",
		"2001:db8::1", "2001::1", "2002::1", "64:ff9b::1", "100::1", "5f00::1",
		// FAMILY-CROSSING, both forms. Prefix.Contains is false across families, so each
		// of these clears every v4 prefix in the table unless it is handled.
		"::ffff:127.0.0.1", "::ffff:192.168.1.1", "::ffff:240.0.0.1",
		"::7f00:1", "::a00:1", "::c0a8:101", "::e000:1", "::6440:1", "::1.2.3.4",
		"2001:20::1",
	}
	for _, s := range refuse {
		if Routable(netip.MustParseAddr(s)) {
			t.Errorf("Routable(%s) = true — this becomes a punch target, and both sides send "+
				"hundreds of packets at every candidate", s)
		}
	}
}

func TestRoutableAcceptsOrdinaryPublicAddresses(t *testing.T) {
	for _, s := range []string{"93.184.216.34", "8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !Routable(netip.MustParseAddr(s)) {
			t.Errorf("Routable(%s) = false — an ordinary public address was refused, which "+
				"would silently remove a working tier", s)
		}
	}
}

func TestTheTableIsNotEmpty(t *testing.T) {
	// Stimulus: the loops above pass trivially if the table were empty and the Go
	// predicates alone were doing the work. They are not — that is the whole reason this
	// table exists — so assert it has entries.
	if len(reserved) < 10 {
		t.Fatalf("the reserved table has %d entries; the checks above would be resting on "+
			"Go's predicates alone, which is what this table exists because they do not do",
			len(reserved))
	}
}

func TestTargetAppliesThePortRuleAndItsExceptions(t *testing.T) {
	cases := map[string]bool{
		"93.184.216.34:34154": true,
		"93.184.216.34:1024":  true,
		"93.184.216.34:443":   true,  // TCP fallback — D14's whole reason
		"93.184.216.34:80":    true,  // ditto
		"93.184.216.34:53":    false, // DNS reflection
		"93.184.216.34:123":   false, // NTP
		"93.184.216.34:19":    false, // chargen
		"93.184.216.34:0":     false,
		"192.168.1.1:34154":   false, // routable rule still applies
	}
	for s, want := range cases {
		if got := Target(netip.MustParseAddrPort(s)); got != want {
			t.Errorf("Target(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestSharedSpaceIsAnswerableSeparately(t *testing.T) {
	// CGNAT is a distinct fact from unroutable — the self-address probe reports it to the
	// user as a diagnosis — so it stays separately answerable, in both address forms.
	for _, s := range []string{"100.64.0.1", "100.127.255.254", "::ffff:100.64.0.1"} {
		if !SharedSpace(netip.MustParseAddr(s)) {
			t.Errorf("SharedSpace(%s) = false", s)
		}
	}
	for _, s := range []string{"93.184.216.34", "100.63.255.255", "100.128.0.0"} {
		if SharedSpace(netip.MustParseAddr(s)) {
			t.Errorf("SharedSpace(%s) = true", s)
		}
	}
}

// TestTheDialHookRefusesWhatTheStdlibVocabularyMissed is P04's phase-review finding.
//
// Every address here passed BOTH hand-written copies of this hook, which tested Go's
// `IsLoopback || IsPrivate || IsLinkLocalUnicast || IsLinkLocalMulticast || IsUnspecified`
// — the stdlib's vocabulary, not this repo's table — on the path that fetches URLs out of
// untrusted file content.
func TestTheDialHookRefusesWhatTheStdlibVocabularyMissed(t *testing.T) {
	// The exact predicate both copies carried, so this test measures the DIFFERENCE and
	// cannot silently agree with itself.
	stdlibVocabulary := func(host string) bool {
		ip := net.ParseIP(host)
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified())
	}

	missed := []struct{ addr, why string }{
		{"::7f00:1", "IPv4-compatible IPv6 — this IS 127.0.0.1"},
		{"::a00:1", "IPv4-compatible IPv6 — 10.0.0.1"},
		{"::c0a8:101", "IPv4-compatible IPv6 — 192.168.1.1"},
		{"::6440:1", "IPv4-compatible IPv6 — 100.64.0.1"},
		{"100.64.0.1", "shared address space: the carrier's NAT and the subscriber's CPE"},
		{"255.255.255.255", "broadcast"},
		{"0.1.2.3", "0.0.0.0/8"},
		{"240.0.0.1", "240.0.0.0/4, reserved"},
		{"198.18.0.1", "benchmarking range"},
		{"2001:20::1", "ORCHIDv2"},
	}
	for _, m := range missed {
		// STIMULUS: assert the OLD predicate really did admit it. Without this the test
		// grades the new hook against a list that might have been safe all along, and
		// would stay green if the finding had never been real.
		if !stdlibVocabulary(m.addr) {
			t.Errorf("%s (%s): the stdlib vocabulary already refused this, so it is not "+
				"evidence of the gap — remove it or the list overstates the finding",
				m.addr, m.why)
			continue
		}
		if err := RefuseNonPublicDialAddress(net.JoinHostPort(m.addr, "80")); err == nil {
			t.Errorf("%s (%s): admitted by the dial hook", m.addr, m.why)
		}
	}

	// The control. A hook that refuses everything is not a fix, it is an outage — these
	// clients fetch public OpenTimestamps calendars and block explorers.
	for _, ok := range []string{"93.184.216.34", "2606:4700:4700::1111"} {
		if err := RefuseNonPublicDialAddress(net.JoinHostPort(ok, "443")); err != nil {
			t.Errorf("%s: refused a public host — the timestamp paths would stop working: %v",
				ok, err)
		}
	}

	// A malformed dial address must not be admitted by falling through.
	if err := RefuseNonPublicDialAddress("not-an-address"); err == nil {
		t.Error("a malformed dial address was admitted")
	}
	if err := RefuseNonPublicDialAddress("example.com:80"); err == nil {
		t.Error("a NAME was admitted — this hook runs after resolution, and admitting a " +
			"name means it ran somewhere it cannot do its job")
	}
}

// TestBothDialHooksCallTheSharedPredicate is P03's recorded lesson applied: "a guard tested
// a predicate and not that anything called it". The predicate above can be perfect while a
// third copy of the old five-term test sits in a dialer, which is exactly the state this
// phase review found.
func TestBothDialHooksCallTheSharedPredicate(t *testing.T) {
	root := filepath.Join("..", "..")
	var dialers, delegating int
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") ||
			strings.HasSuffix(p, "_test.go") || strings.Contains(p, "node_modules") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			// A dial-control hook by SHAPE, not by name: a rename must not disarm this.
			if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
				return true
			}
			last := fn.Type.Params.List[len(fn.Type.Params.List)-1]
			sel, ok := last.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "RawConn" {
				return true
			}
			dialers++
			body := string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			if strings.Contains(body, "RefuseNonPublicDialAddress") {
				delegating++
			} else {
				t.Errorf("%s: %s is a dial-control hook that does NOT delegate to "+
					"addrscope.RefuseNonPublicDialAddress — a fourth copy of the address "+
					"table, and the copies are how one of them stays weak", p, fn.Name.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The floor. Zero hooks found means the shape matcher stopped matching and every
	// assertion above ran on nothing.
	if dialers < 2 {
		t.Fatalf("found %d dial-control hook(s) in the tree; expected at least the two in "+
			"internal/server and internal/cli — the matcher has gone blind", dialers)
	}
	t.Logf("%d dial-control hook(s), %d delegating", dialers, delegating)
}
