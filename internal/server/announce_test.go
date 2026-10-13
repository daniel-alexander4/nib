package server

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"nib/internal/discovery"
	"nib/internal/vault"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nib/internal/pairing"
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

			ann, err := announcingServer(t).startAnnouncing(cert, boundOn{ln, "tcp"}, lanAnnounceWindow, discovery.HopNone)
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
	ann, err := announcingServer(t).startAnnouncing(cert, boundOn{ln, "tcp"}, window, discovery.HopNone)
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

// announcingServer is a server whose user has switched local-network discovery ON.
//
// **Every test in this file is about the ANNOUNCER, and none is about the switch.** Since v1.129.5
// `startAnnouncing` refuses when discovery is off (`/pending 451`), and off is the default — so
// without this every case here would be testing the refusal and reporting it as a fact about
// binding, hops or windows. The switch itself is driven in `advanced_test.go`.
func announcingServer(t *testing.T) *Server {
	t.Helper()
	srv, v := unlockedServer(t)
	if err := v.UpdateSettings(func(s *vault.Settings) {
		s.Advanced = &vault.Advanced{Discovery: true}
	}); err != nil {
		t.Fatal(err)
	}
	return srv
}

// TestTwoArmsOfOneIdentityShareNoIdentifierOnTheLink — ADR-151, through the product's own
// announcer and two real sockets.
//
// The announcement used to carry the six-word name, one constant for the life of an identity:
// anything on the segment could tell that two arms, on two days or two networks, were the same
// party without knowing who. Two real announcers of ONE certificate are started here and heard
// by a third socket. What must hold of what was actually heard:
//
//   - both are recognised by a Nib that has that identity PINNED — the feature still works;
//   - neither is recognised by a Nib that has somebody else pinned;
//   - the two share neither salt nor tag, so a listener holding no fingerprint cannot join them.
//
// The arms are told apart by the PORT each announces, which is the listener each was started on.
func TestTwoArmsOfOneIdentityShareNoIdentifierOnTheLink(t *testing.T) {
	cert, _, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	srv := announcingServer(t)

	hear, err := discovery.Open([8]byte{0xB0, 1, 2, 3, 4, 5, 6, 7})
	if err != nil {
		t.Skipf("no usable interface for discovery here: %v", err)
	}
	defer hear.Close()

	ports := map[uint16]bool{}
	for i := 0; i < 2; i++ {
		ln, err := net.Listen("tcp", "0.0.0.0:0") // wildcard, so not refused as loopback
		if err != nil {
			t.Skipf("setup: cannot bind a routable listener here: %v", err)
		}
		defer ln.Close()
		ann, err := srv.startAnnouncing(cert, boundOn{ln, "tcp"}, lanAnnounceWindow, discovery.HopNone)
		if err != nil {
			t.Skipf("announcer did not start (multicast unavailable here): %v", err)
		}
		defer ann.Close()
		ports[uint16(portOf(ln))] = true
	}
	if len(ports) != 2 {
		t.Fatalf("setup: the two arms share a port (%v), so they cannot be told apart", ports)
	}

	heard := map[uint16]discovery.Seen{}
	deadline := time.Now().Add(5 * time.Second)
	for len(heard) < 2 && time.Now().Before(deadline) {
		s, err := hear.Read(deadline)
		if err != nil || !ports[s.Port] {
			continue // a timeout, or some other Nib on this machine's link
		}
		if prev, dup := heard[s.Port]; dup && (prev.Salt != s.Salt || prev.Tag != s.Tag) {
			t.Fatalf("one announcer changed its salt or tag between two announcements (%x/%x then %x/%x) "+
				"— a browse that matched the first would not match the second", prev.Salt, prev.Tag, s.Salt, s.Tag)
		}
		heard[s.Port] = s
	}
	if len(heard) < 2 {
		t.Skipf("heard %d of the 2 arms in 5s (multicast does not loop back here), so nothing can be compared", len(heard))
	}

	mine := []vault.PinnedPeer{{Fingerprint: fp, Label: "Alice"}}
	other := []vault.PinnedPeer{{Fingerprint: fpOf(99), Label: "Somebody else"}}
	var seen []discovery.Seen
	for port, s := range heard {
		c, ok := resolve(mine, s, discovery.HopNone)
		if !ok || c.Label != "Alice" {
			t.Errorf("the arm on port %d was not recognised by a Nib that has its identity pinned", port)
		}
		if c, ok := resolve(other, s, discovery.HopNone); ok {
			t.Errorf("the arm on port %d resolved to %q for a Nib that has only somebody else pinned", port, c.Label)
		}
		seen = append(seen, s)
	}
	if seen[0].Salt == seen[1].Salt {
		t.Errorf("two arms of one identity announced the same salt %x — it was reused across announcers", seen[0].Salt)
	}
	if seen[0].Tag == seen[1].Tag {
		t.Errorf("two arms of one identity announced the same tag %x: anything on the link can tell they "+
			"are one party without holding its fingerprint", seen[0].Tag)
	}
}

// TestWhatTwoAnnouncersSendIsRecognisedByAPinAndLinkedByNothingElse — the same property as the
// test above, on a path that needs no multicast: this is the leg that runs on a developer's
// machine, where a default-deny firewall swallows the group and the test above skips.
//
// Two announcements are built by the announcer's own constructor (`announcementOf`, the only
// place a salt is drawn for one), sent from a real UDP socket to a real discovery socket, and
// what comes OFF the wire is what is judged.
func TestWhatTwoAnnouncersSendIsRecognisedByAPinAndLinkedByNothingElse(t *testing.T) {
	cert, _, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	name, err := pairing.Name(fp)
	if err != nil {
		t.Fatal(err)
	}

	hear, err := discovery.Open([8]byte{0xB1, 1, 2, 3, 4, 5, 6, 7})
	if err != nil {
		t.Skipf("no usable interface for discovery here: %v", err)
	}
	defer hear.Close()
	c, err := net.Dial("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(discovery.Port)))
	if err != nil {
		t.Skipf("cannot send a loopback datagram here: %v", err)
	}
	defer c.Close()

	var wires [][]byte
	var heard []discovery.Seen
	for i, port := range []uint16{41001, 41002} {
		a, err := announcementOf(fp, port, discovery.TransportTCP, discovery.HopNone, [8]byte{0xA0, byte(i)})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := a.Encode()
		if err != nil {
			t.Fatalf("the announcer built an announcement its own format refuses: %v", err)
		}
		wires = append(wires, wire)
		if _, err := c.Write(wire); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			s, err := hear.Read(deadline)
			if err == nil && s.Port == port {
				heard = append(heard, s)
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the discovery socket never heard the announcement for port %d sent to it on loopback", port)
			}
		}
	}

	mine := []vault.PinnedPeer{{Fingerprint: fp, Label: "Alice"}}
	other := []vault.PinnedPeer{{Fingerprint: fpOf(99), Label: "Somebody else"}}
	for _, s := range heard {
		got, ok := resolve(mine, s, discovery.HopNone)
		if !ok {
			t.Fatalf("the announcement for port %d was not recognised by a Nib that has its sender pinned", s.Port)
		}
		// What a user is shown is still the six words — from the pin, since the wire has none.
		if got.Name != name || !bytes.Equal(got.Fingerprint, fp) {
			t.Errorf("resolved to %q / %x, want the pinned %q / %x", got.Name, got.Fingerprint, name, fp)
		}
		if c, ok := resolve(other, s, discovery.HopNone); ok {
			t.Errorf("the announcement for port %d resolved to %q for a Nib that has only somebody else pinned", s.Port, c.Label)
		}
	}
	if heard[0].Salt == heard[1].Salt {
		t.Errorf("two announcers of one identity sent the same salt %x — it was reused", heard[0].Salt)
	}
	if heard[0].Tag == heard[1].Tag {
		t.Errorf("two announcers of one identity sent the same tag %x: anything on the link can tell "+
			"they are one party without holding its fingerprint", heard[0].Tag)
	}
	// And byte for byte, over the part of the datagram that speaks of identity — everything
	// after the nonce: the two agree nowhere for as long as four bytes. A run that long would be
	// something derived from the identity alone, whatever field it sat in.
	const identity = pairing.SaltLen + pairing.TagLen
	a, b := wires[0][len(wires[0])-identity:], wires[1][len(wires[1])-identity:]
	for i := 0; i+4 <= len(a); i++ {
		if j := bytes.Index(b, a[i:i+4]); j >= 0 {
			t.Errorf("bytes %x at offset %d of one arm's salt and tag are also in the other's (offset %d)",
				a[i:i+4], i, j)
		}
	}
	// Nor the six words themselves, in either.
	for _, w := range wires {
		for _, word := range strings.Fields(name) {
			if bytes.Contains(w, []byte(word)) {
				t.Errorf("the word %q of the sender's six-word name is in its announcement", word)
			}
		}
	}
}
