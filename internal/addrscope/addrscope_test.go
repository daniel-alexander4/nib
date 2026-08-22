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

// TestLoopbackIsOneRule pins the predicate that replaced five of them. The rows that matter
// are the disagreements: 127.0.0.2 was loopback to two of the old five and not to the other
// three, and `loopbackOnly` requires two of those that disagreed.
func TestLoopbackIsOneRule(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
		why  string
	}{
		{"127.0.0.1:8080", true, "the ordinary case"},
		{"127.0.0.1", true, "no port — a Host header may omit it"},
		{"localhost:8080", true, "what a browser sends when the user typed it"},
		{"localhost", true, ""},
		{"::1", true, "v6 loopback, unbracketed"},
		{"[::1]:8080", true, "v6 loopback as a Host header carries brackets"},
		{"[::1]", true, "bracketed with no port"},
		{"127.0.0.2:8080", true, "the whole 127/8 block is loopback; three of the five old rules said no"},
		{"127.255.255.254", true, "still 127/8"},
		{"0.0.0.0:8080", false, "the wildcard is not loopback — binding it exposes nib to the network"},
		{"192.168.1.5:8080", false, "a LAN address"},
		{"10.0.0.1", false, ""},
		{"::", false, "the v6 wildcard"},
		{"evil.example:8080", false, "a name that is not localhost"},
		{"localhost.evil.example", false, "a prefix attack on the name arm"},
		{"evil.localhost", false, "a SUFFIX attack — a HasSuffix here would admit any name an attacker controls"},
		{"notlocalhost", false, "and a bare substring"},
		{"", false, ""},
		{"::ffff:127.0.0.1", true, "v4-mapped loopback: net.IP.IsLoopback unmaps, and a Host header can carry this"},
		{"::ffff:192.168.1.5", false, "v4-mapped LAN must not become loopback by changing family"},
	} {
		if got := Loopback(tc.in); got != tc.want {
			t.Errorf("Loopback(%q) = %v, want %v — %s", tc.in, got, tc.want, tc.why)
		}
	}
}

// TestAZoneCannotSmuggleAnAddressPastTheReservedTable — a live bypass, measured.
//
// `netip.Prefix.Contains` is false for any address carrying a zone, and `Routable` walks
// every entry in `reserved` through it. So a zone cleared the whole table while leaving the
// byte-test predicates above it working — which is why the hole was invisible: `::1%eth0`
// and `fe80::1%eth0` stayed refused, and those are the cases anyone would think to try.
//
// The classes below are the table's own reason for existing. `::/96` carries an IPv4
// address in the low 32 bits, so `::c0a8:101` IS 192.168.1.1 — the entry's own comment
// records that measurement and calls it "the one on this list that reaches a real machine
// somebody else owns". 6to4 encodes an arbitrary IPv4 in bits 16..48. NAT64 is the one that
// bites hardest: on an IPv6-only carrier — standard on mobile networks — `64:ff9b::/96` is
// translated to any IPv4 address, including RFC1918 and loopback, at the translator.
//
// The attacker is an in-roster counterparty, which is inside this design's declared threat
// model, and what it buys them is aiming Nib's dials — and later the punch — at hosts on
// the victim's own network.
func TestAZoneCannotSmuggleAnAddressPastTheReservedTable(t *testing.T) {
	// Each pair is the same address twice: bare, and wearing a zone. The bare form is the
	// CONTROL — without it a table that refused everything would pass, and refusing
	// everything breaks every tier that ends in a dialable address.
	for _, tc := range []struct{ bare, zoned, what string }{
		{"::c0a8:101", "::c0a8:101%eth0", "192.168.1.1 inside ::/96"},
		{"::7f00:1", "::7f00:1%eth0", "127.0.0.1 inside ::/96"},
		{"2001:db8::1", "2001:db8::1%eth0", "documentation space"},
		{"2002:c0a8:101::1", "2002:c0a8:101::1%9", "6to4, which encodes an arbitrary IPv4"},
		{"64:ff9b::7f00:1", "64:ff9b::7f00:1%eth0", "NAT64, translated to any IPv4 at the translator"},
		{"2001:20::1", "2001:20::1%eth0", "ORCHIDv2"},
		{"100::1", "100::1%eth0", "the discard prefix"},
	} {
		bare := netip.MustParseAddr(tc.bare)
		// SETUP: the bare form really is refused. If it is not, the table has lost the
		// entry and the zoned assertion below would pass for the wrong reason.
		if Routable(bare) {
			t.Fatalf("setup: %s (%s) is Routable even without a zone — the reserved table "+
				"has lost this entry, so the zone assertion proves nothing", tc.bare, tc.what)
		}
		zoned := netip.MustParseAddr(tc.zoned)
		if Routable(zoned) {
			t.Errorf("%s is Routable but %s is not — the same address, and the only "+
				"difference is a zone. %s. netip.Prefix.Contains is false for any zoned "+
				"address, so a zone clears every entry in `reserved` while the byte tests "+
				"above keep working.", tc.zoned, tc.bare, tc.what)
		}
		// And through the door the candidate record actually uses.
		if Target(netip.AddrPortFrom(zoned, 5000)) {
			t.Errorf("Target accepts %s:5000 — a candidate record naming it would be sealed, "+
				"published, opened and dialled", tc.zoned)
		}
	}

	// The other direction, so this cannot be satisfied by refusing every zoned address in a
	// way that breaks something real: a zone on a genuinely global address is meaningless
	// but harmless, and stripping it must leave the address's own verdict intact.
	global := netip.MustParseAddr("2606:4700:4700::1111")
	if !Routable(global) {
		t.Fatal("setup: a global unicast v6 address is not Routable, so the check below " +
			"cannot distinguish stripping a zone from refusing everything")
	}
	if !Routable(netip.MustParseAddr("2606:4700:4700::1111%eth0")) {
		t.Error("a zone made a genuinely global address unroutable; the zone must be " +
			"stripped for the comparison, not treated as disqualifying — otherwise a " +
			"legitimate candidate learned on an interface stops being dialable")
	}
}

// A zone on a GLOBAL v6 address is refused by the two predicates that judge an address
// somebody else supplied — the counterpart to the test above, and a different claim.
//
// That test is about the reserved TABLE: a zone cleared every prefix, so a private address
// wearing one read as public. `Routable` fixed it by stripping, which makes the verdict
// right and leaves the address alone. This test is about what happens to the address AFTER
// the verdict. `[2606:4700:4700::1111%eth0]:5000` is genuinely global, so the table has
// nothing to say about it; it passed `Target`, was stored by `parseCandidate` with its zone
// intact, survived the canonical re-encode (which re-emits the zone rather than filtering
// it), and reached `net.Dialer` verbatim through `Endpoint.Addr.String()`.
//
// **Measured before this existed**, so the fix is not aimed at a guess: on Linux/Go that
// address dials — from the ordinary global source, in ~20ms — and so do `%docker0`, `%99`
// and `%nosuchif`. The kernel ignores `sin6_scope_id` for a global-scope destination, and
// `Dialer.Control` confirms the zone reaches the syscall regardless. So no source-interface
// steering was demonstrated on this platform, and none is claimed for Windows or macOS.
//
// The rule stands on the simpler ground: a zone means something only on a link-local
// address, link-local is refused whatever its zone says, so a zone reaching here is bytes
// an attacker chose that Nib can never act on and hands to the kernel anyway.
func TestAZoneOnAGlobalAddressNeverReachesTheDialer(t *testing.T) {
	const bare = "2606:4700:4700::1111"
	// SETUP, and it is the whole discriminator: the bare form must PASS both predicates.
	// Without this the test is satisfied by a predicate that refuses everything, which is
	// exactly how the reserved-table version of this bug could have been "fixed" wrongly.
	if !Target(netip.AddrPortFrom(netip.MustParseAddr(bare), 5000)) {
		t.Fatal("setup: the bare global address is not a Target, so refusing its zoned " +
			"form proves nothing about zones")
	}
	if !Seed(netip.AddrPortFrom(netip.MustParseAddr(bare), 5000)) {
		t.Fatal("setup: the bare global address is not a Seed, so refusing its zoned " +
			"form proves nothing about zones")
	}
	for _, zone := range []string{"eth0", "lo", "docker0", "1", "99", "nosuchif"} {
		zoned := netip.MustParseAddr(bare).WithZone(zone)
		if Target(netip.AddrPortFrom(zoned, 5000)) {
			t.Errorf("Target accepts %s:5000 — a candidate record naming it is sealed, "+
				"published, opened, and its zone is handed to the kernel by the racer", zoned)
		}
		if Seed(netip.AddrPortFrom(zoned, 5000)) {
			t.Errorf("Seed accepts %s:5000 — an invitation's bootstrap list naming it "+
				"reaches the DHT's dialer with a zone the peer chose", zoned)
		}
	}
}

// Both zone-judging predicates route through the one door, so a third predicate added
// later cannot quietly grow its own copy of the rule (ADR-009).
//
// It reads the source rather than comparing behaviour, because eight cases agreeing says
// nothing about a ninth call site written without the check — which is the failure ADR-009
// exists for, and which this repo has shipped.
func TestTargetAndSeedShareTheZoneDoor(t *testing.T) {
	src, err := os.ReadFile("addrscope.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	// SETUP: the door exists and is the only place the zone is read for a verdict.
	if strings.Count(body, "a.Zone() != \"\"") != 1 {
		t.Fatalf("setup: expected exactly one zone verdict in addrscope.go, found %d — "+
			"a second one is a copy of the rule, which is what this guard forbids",
			strings.Count(body, "a.Zone() != \"\""))
	}
	for _, fn := range []struct{ name, decl string }{
		{"Target", "func Target(ap netip.AddrPort) bool {"},
		{"Seed", "func Seed(ap netip.AddrPort) bool {"},
	} {
		i := strings.Index(body, fn.decl)
		if i < 0 {
			t.Fatalf("setup: %s's declaration has changed shape; this guard cannot see it", fn.name)
		}
		rest := body[i:]
		end := strings.Index(rest, "\n}\n")
		if end < 0 {
			t.Fatalf("setup: cannot find the end of %s", fn.name)
		}
		if !strings.Contains(rest[:end], "dialable(") {
			t.Errorf("%s does not call dialable — it judges an address somebody else "+
				"supplied and must go through the one door that refuses a zone", fn.name)
		}
	}
}
