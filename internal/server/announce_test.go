package server

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
)

// boundOn is a real listener plus the transport it stands for, which is what
// `announceable` asks for. A bare net.Listener cannot answer the second question, and
// that ambiguity is the whole of ADR-010.
type boundOn struct {
	net.Listener
	transport string
}

func (b boundOn) Transport() string { return b.transport }

// TestALoopbackBindIsNotAnnouncedOnTheLink.
//
// `runSession` announced `portOf(ln)` and nothing looked at the host that port belongs to.
// Arm with `{"bind":"127.0.0.1:8443"}` and Nib broadcast *armed on 8443* on every joined
// interface every 500 ms while the listener answered only on loopback: a peer on the link
// resolves that to `<our LAN address>:8443` and cannot connect, and a user who deliberately
// bound loopback has their six-word name — a stable function of a never-rotating
// fingerprint — put on every attached segment anyway.
//
// **The second half of this test is the one that could go vacuous.** A `startAnnouncing`
// that refused everything passes the loopback assertion and breaks the LAN tier outright,
// which is the whole product feature P03 built. So the wildcard case asserts the refusal
// does NOT fire. It deliberately does not assert that a socket opened: `discovery.Open`
// joins multicast groups and legitimately fails in a restricted environment, and this test
// is about which addresses reach that call — not about whether the link is usable here.
func TestALoopbackBindIsNotAnnouncedOnTheLink(t *testing.T) {
	cert, _, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		bind     string
		announce bool
	}{
		{"ipv4 loopback", "127.0.0.1:0", false},
		// 127.0.0.2 is loopback by definition and is the address the repo's five
		// competing loopback rules used to disagree about (addrscope.Loopback's own
		// doc). A bind here is exactly as unreachable from the link as 127.0.0.1.
		{"ipv4 loopback, not .1", "127.0.0.2:0", false},
		{"ipv6 loopback", "[::1]:0", false},
		{"ipv4 wildcard", "0.0.0.0:0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", tc.bind)
			if err != nil {
				t.Skipf("setup: cannot bind %s here: %v", tc.bind, err)
			}
			defer ln.Close()
			// STIMULUS: the listener really did bind the family and host this case is
			// about. Without it a silently-rewritten bind would make the assertion
			// below true for a reason that has nothing to do with the rule.
			if host, _, serr := net.SplitHostPort(ln.Addr().String()); serr != nil {
				t.Fatalf("setup: unreadable listener address %q", ln.Addr())
			} else if ip := net.ParseIP(strings.Trim(host, "[]")); ip == nil {
				t.Fatalf("setup: listener address %q is not an IP", host)
			} else if ip.IsLoopback() == tc.announce {
				t.Fatalf("setup: bound %s, which is loopback=%v — this case wants the opposite",
					ln.Addr(), ip.IsLoopback())
			}

			ann, err := startAnnouncing(cert, boundOn{ln, "tcp"}, lanAnnounceWindow)
			if ann != nil {
				ann.Close()
			}
			refused := errors.Is(err, errLoopbackBind)
			if tc.announce && refused {
				t.Errorf("bind %s is reachable from the link and was refused as loopback: %v", ln.Addr(), err)
			}
			if !tc.announce && !refused {
				t.Errorf("bind %s is loopback and was announced on every joined interface "+
					"(err=%v); a peer resolves that to our LAN address and cannot connect, "+
					"and our name is on every attached segment", ln.Addr(), err)
			}
		})
	}
}

// TestAnnouncingHasExactlyOneDoor is ADR-009's half of the fix.
//
// The rule "a loopback bind is not announced" is worth nothing if a second call site can
// build an announcer without passing through it. Asserting the TEXT of `startAnnouncing`
// would say nothing about a `&lanAnnouncer{...}` written somewhere else — which is the
// exact shape ADR-009 was written about ("eight copies checked for agreement say nothing
// about a ninth site added without one").
//
// So this asserts the routing: the composite literal that MAKES an announcer occurs once
// in the package, inside the function that holds the rule.
func TestAnnouncingHasExactlyOneDoor(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sites []string
	var scanned int
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, d := range af.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				id, ok := cl.Type.(*ast.Ident)
				if !ok || id.Name != "lanAnnouncer" {
					return true
				}
				sites = append(sites, fn.Name.Name)
				return true
			})
		}
	}
	// STIMULUS: the scan really read this package. A glob that matched nothing would
	// report zero sites and read as a pass.
	if scanned == 0 {
		t.Fatal("setup: the scan parsed no non-test files, so it could not have found a site")
	}
	if len(sites) == 0 {
		t.Fatalf("setup: found no lanAnnouncer literal at all in %d files — the scan is "+
			"looking for the wrong thing, not proving there is one door", scanned)
	}
	if len(sites) != 1 || sites[0] != "startAnnouncing" {
		t.Errorf("an announcer is built at %v; it must be built only in startAnnouncing, "+
			"which is where the loopback rule lives (ADR-009)", sites)
	}
}

// TestTheAnnouncerStopsAtItsWindow — P05.S09b T02, criterion 14 driven over a SIMULATED window.
// S09b extends a ceremony arm's LISTEN window to ceremony scope (up to 30 days); left coupled, the
// 500ms announce ticker would then emit for 30 days — ~5.2M multicast datagrams of a never-rotating
// name. The window caps ANNOUNCING independently of listening. Driven with a short window: the count
// must STOP climbing once the window passes, or the cap does nothing and the ceremony is a cannon.
func TestTheAnnouncerStopsAtItsWindow(t *testing.T) {
	cert, _, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "0.0.0.0:0") // wildcard, so not refused as loopback
	if err != nil {
		t.Skipf("setup: cannot bind a routable listener here: %v", err)
	}
	defer ln.Close()

	const window = 700 * time.Millisecond // several announceEvery ticks, then the cap fires
	ann, err := startAnnouncing(cert, boundOn{ln, "tcp"}, window)
	if err != nil {
		t.Skipf("announcer did not start (multicast unavailable here): %v", err)
	}
	defer ann.Close()

	// STIMULUS: it must actually have announced, or "it stopped" is equally true of a socket that
	// never sent. Multicast joins fail in restricted sandboxes, so a genuine zero is a SKIP, not a
	// pass — the same honesty the loopback test keeps about discovery.Open failing.
	time.Sleep(window + 400*time.Millisecond)
	s1 := ann.sock.Stats().Sent
	if s1 == 0 {
		t.Skip("the announcer never emitted (multicast unavailable), so the window cap cannot be observed here")
	}
	// After the window it must be SILENT: a second reading a full announceEvery later is unchanged.
	time.Sleep(3 * announceEvery)
	s2 := ann.sock.Stats().Sent
	if s2 != s1 {
		t.Errorf("the announcer emitted %d more datagram(s) AFTER its %s window — the cap does not "+
			"fire, so an arm extended to ceremony scope emits at full rate for the whole deadline "+
			"(criterion 14, the ~5.2M-datagram harm)", s2-s1, window)
	}
	// And it did not overshoot the window wildly: bounded by roughly window/announceEvery interfaces.
	if maxExpected := uint64(window/announceEvery+2) * uint64(ann.sock.Stats().Interfaces+1); s1 > maxExpected {
		t.Errorf("emitted %d datagrams in a %s window (announceEvery=%s) — more than the cap should allow",
			s1, window, announceEvery)
	}
}
