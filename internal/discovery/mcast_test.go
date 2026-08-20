package discovery

import (
	"crypto/rand"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// liveOrSkip opens a real discovery socket and proves the link can carry a datagram at
// all, by announcing and waiting for this process's OWN loopback copy.
//
// **Why a skip and not a failure.** The environment, not the code, decides whether a
// multicast datagram on Nib's port survives: a loopback copy traverses INPUT, so a
// default-deny host swallows it with no error at either end — measured on the
// development machine, where 224.0.0.251:5353 delivers and the same group on another
// port times out. A test that failed there would be reporting the firewall.
//
// **Why this is not a test excusing itself.** The skip is the diagnostic Nib itself
// will show a user in the same situation, and it is the reason ErrOwn exists rather
// than being swallowed: hearing your own copy is the only firewall-independent proof
// the send path works. The two-process acceptance below does NOT skip — it runs inside
// a network namespace where the host's rules do not apply, which is what makes the
// clause driven rather than hoped for.
func liveOrSkip(t *testing.T) *Socket {
	t.Helper()
	var nonce [nonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	s, err := Open(nonce)
	if err != nil {
		t.Skipf("no usable interface for discovery: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	a := Announcement{Name: aName(t, 7), Port: 8443, Nonce: nonce}
	sent, err := s.Announce(a)
	if err != nil {
		t.Skipf("could not announce on any interface: %v (%s)", err, s.Describe())
	}
	if sent == 0 {
		t.Skipf("announced on no interface (%s)", s.Describe())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, err := s.Read(deadline)
		if errors.Is(err, ErrOwn) {
			return s // the link carries our datagrams; the caller can rely on it
		}
		if err != nil && (errors.Is(err, os.ErrDeadlineExceeded) || strings.Contains(err.Error(), "timeout")) {
			break
		}
	}
	t.Skipf("sent %d announcements on %v and never heard our own loopback copy — this host "+
		"swallows multicast on port %d (a default-deny firewall does exactly this, silently). "+
		"Interfaces: %s", sent, s.Interfaces(), Port, s.Describe())
	return nil
}

// TestTheSocketJoinsTheInterfacesItChose — the acceptance clause that the joined set is
// asserted rather than left to whatever the stdlib picked.
func TestTheSocketJoinsTheInterfacesItChose(t *testing.T) {
	s := liveOrSkip(t)

	joined := s.Interfaces()
	if len(joined) == 0 {
		t.Fatal("the socket reports no joined interfaces yet it heard its own announcement")
	}
	all, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, ifi := range chooseInterfaces(all, false) {
		want[ifi.Name] = true
	}
	for _, name := range joined {
		if !want[name] {
			t.Errorf("joined %q, which the selection would not have chosen — the socket is not "+
				"joining what chooseInterfaces decided (%s)", name, s.Describe())
		}
	}
	if st := s.Stats(); st.Interfaces != len(joined) {
		t.Errorf("Stats says %d interfaces, Interfaces() lists %d", st.Interfaces, len(joined))
	}
	// The Windows fact, recorded wherever the suite runs: a nil control message with
	// a nil error is what x/net gives there, and this is where that shows up.
	if s.Stats().NoControlMessage {
		t.Logf("this platform supplies no arrival interface (x/net SetControlMessage " +
			"unimplemented) — nothing filters on it, which is why that is survivable")
	}
}

// TestOwnAnnouncementsAreFilteredByNonceNotAddress.
func TestOwnAnnouncementsAreFilteredByNonceNotAddress(t *testing.T) {
	s := liveOrSkip(t)

	// Stimulus: liveOrSkip already heard one own-copy, so Own is non-zero and the
	// filter demonstrably fires. Without that this asserts nothing.
	if s.Stats().Own == 0 {
		t.Fatal("no own announcement was recognised, so the self-filter never ran")
	}
	before := s.Stats()

	// An announcement with a DIFFERENT nonce from the same host must NOT be filtered
	// — that is the whole point of using the nonce rather than the source address,
	// since both datagrams arrive from the same local address.
	other := Announcement{Name: aName(t, 11), Port: 9999}
	if _, err := rand.Read(other.Nonce[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Announce(other); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var seen *Seen
	for time.Now().Before(deadline) && seen == nil {
		got, err := s.Read(deadline)
		if err == nil {
			g := got
			seen = &g
		} else if !errors.Is(err, ErrOwn) && !errors.Is(err, ErrNotOurs) && !errors.Is(err, ErrMalformed) {
			break
		}
	}
	if seen == nil {
		t.Fatal("an announcement with a different nonce, from this same host, was never " +
			"returned as a peer — the self-filter is matching on something other than the nonce")
	}
	if seen.Port != 9999 {
		t.Errorf("read the wrong announcement: port %d", seen.Port)
	}
	if seen.From == nil {
		t.Error("no source address — the candidate to dial is the source address plus the " +
			"announced port, so a nil here makes the announcement useless")
	}
	if st := s.Stats(); st.Peers <= before.Peers {
		t.Errorf("Peers did not advance: %d then %d", before.Peers, st.Peers)
	}
}

// TestTwoSocketsCanShareThePort is the SO_REUSEADDR assertion, and it is the one that
// would catch the darwin gap.
//
// net.ListenPacket does not set SO_REUSEADDR — the stdlib only does when the bind
// address is multicast, and this binds the wildcard — so without the ListenConfig
// control this fails and same-host discovery is impossible. darwin and the BSDs need
// SO_REUSEPORT as well; rather than ship a //go:build file per platform with a no-op
// sibling (the shape that already produced one silent defect here), the gap is named
// and this test is what fails loudly there.
func TestTwoSocketsCanShareThePort(t *testing.T) {
	var n1, n2 [nonceLen]byte
	n1[0], n2[0] = 1, 2
	a, err := Open(n1)
	if err != nil {
		t.Skipf("no usable interface: %v", err)
	}
	defer a.Close()
	b, err := Open(n2)
	if err != nil {
		t.Fatalf("a second socket could not share port %d: %v\n"+
			"On darwin/BSD this is SO_REUSEPORT, which is deliberately not set — see Open. "+
			"Two Nibs on one machine cannot discover each other until it is.", Port, err)
	}
	defer b.Close()
	if a.LocalPort() != b.LocalPort() {
		t.Fatalf("the two sockets bound different ports (%d, %d) — they are not sharing",
			a.LocalPort(), b.LocalPort())
	}
}

// TestTwoProcessesDiscoverEachOther is P03.S02's first acceptance clause: two
// processes, real multicast, driven.
//
// It re-execs this test binary rather than using goroutines, because the clause says
// processes and because a goroutine pair would share the socket options and the
// membership table — precisely the state whose per-process behaviour is in question.
//
// Run it under build/mcastrepro.sh, which supplies a network namespace with a dummy
// interface. Outside one it skips, for the firewall reason liveOrSkip documents.
func TestTwoProcessesDiscoverEachOther(t *testing.T) {
	if role := os.Getenv("NIB_MCAST_ROLE"); role != "" {
		runRole(t, role)
		return
	}
	if os.Getenv("NIB_MCAST_NETNS") != "1" {
		t.Skip("not in a prepared network namespace — run build/mcastrepro.sh, which creates " +
			"one with a dummy interface so the host's firewall rules do not decide the result")
	}

	self := os.Args[0]
	browse := exec.Command(self, "-test.run", "^TestTwoProcessesDiscoverEachOther$", "-test.v")
	browse.Env = append(os.Environ(), "NIB_MCAST_ROLE=browse")
	out, err := browse.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := browse.Start(); err != nil {
		t.Fatal(err)
	}
	defer browse.Process.Kill()

	time.Sleep(300 * time.Millisecond) // let the browser join before anyone announces

	announce := exec.Command(self, "-test.run", "^TestTwoProcessesDiscoverEachOther$", "-test.v")
	announce.Env = append(os.Environ(), "NIB_MCAST_ROLE=announce")
	aOut, aErr := announce.CombinedOutput()
	if aErr != nil {
		t.Fatalf("the announcing process failed: %v\n%s", aErr, aOut)
	}

	buf := make([]byte, 8192)
	n, _ := out.Read(buf)
	got := string(buf[:n])
	_ = browse.Wait()

	// The stimulus and the result are asserted separately: the announcer must have
	// SENT (its own output says how many), and the browser must have SEEN a peer.
	if !strings.Contains(string(aOut), "ANNOUNCED") {
		t.Fatalf("the announcer never reported sending, so the browser's silence proves "+
			"nothing:\n%s", aOut)
	}
	if !strings.Contains(got, "DISCOVERED") {
		t.Fatalf("the browsing process never discovered the announcer.\nbrowser:\n%s\nannouncer:\n%s",
			got, aOut)
	}
	// Echoed, not just consumed. The first version asserted on `got` and threw it
	// away on success, so the harness around this test could not tell a pass about
	// discovery from a pass about anything else — caught by that harness's own
	// "the pass above is not about discovery" guard. Evidence a check consumed
	// privately is evidence nobody else can audit.
	t.Logf("browser: %s", strings.TrimSpace(got))
	t.Logf("announcer: %s", strings.TrimSpace(firstLine(string(aOut))))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func runRole(t *testing.T, role string) {
	var nonce [nonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	s, err := Open(nonce)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	switch role {
	case "announce":
		a := Announcement{Name: aName(t, 21), Port: 8443, Nonce: nonce}
		for i := 0; i < 12; i++ { // repeat: the browser may still be joining
			sent, err := s.Announce(a)
			if err != nil {
				t.Fatalf("announce: %v (%s)", err, s.Describe())
			}
			if sent == 0 {
				t.Fatalf("announced on no interface (%s)", s.Describe())
			}
			fmt.Printf("ANNOUNCED %d on %v\n", sent, s.Interfaces())
			time.Sleep(150 * time.Millisecond)
		}
	case "browse":
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			seen, err := s.Read(deadline)
			if err == nil {
				fmt.Printf("DISCOVERED %q port %d from %s\n", seen.Name, seen.Port, seen.From)
				return
			}
		}
		fmt.Printf("NOTHING (%+v) %s\n", s.Stats(), s.Describe())
		t.Fatal("browsed for 8s and discovered nothing")
	}
}

// TestNothingDecidesOnTheArrivalInterface is the first Windows divergence as a guard.
//
// x/net's SetControlMessage is unimplemented on Windows — control_windows.go, both
// families, is a TODO returning errNotImplemented — and Windows compiles
// payload_nocmsg.go, whose ReadFrom returns a nil control message with a NIL ERROR.
// So a filter written the natural defensive way,
//
//	if cm != nil && cm.IfIndex != want { continue }
//
// silently accepts everything there, and one written without the nil guard panics.
// Neither shows up in any test that runs on Linux.
//
// The handling is to decide nothing on it: the self-filter is the nonce, in the
// payload. This asserts that stays true, because the tempting change — "only accept
// announcements from interfaces we joined" — looks like a security improvement and is
// a no-op on the platform where it matters.
func TestNothingDecidesOnTheArrivalInterface(t *testing.T) {
	// Parsed, not grepped. The first version used strings.Contains over the source and
	// matched the WORD "IfIndex" inside the comment in mcast.go that explains why
	// nothing uses it — a guard failing on its own documentation. That is the third
	// instance of this hole in this repo (the .deb guard satisfied by a comment,
	// published.test.mjs satisfied by a doc comment), so it is parsed here: a selector
	// expression is in the AST and a comment is not.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0) // no ParseComments: comments are not in the tree at all
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["discovery"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("no non-test files parsed — the check below would pass on nothing")
	}

	// Stimulus: the package really does read from the socket, so "it never touches the
	// arrival interface" is a fact about a reader and not about an empty package.
	reads := false
	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "ReadFrom" {
				reads = true
			}
			return true
		})
	}
	if !reads {
		t.Fatal("nothing in this package reads from a socket; the guard below is vacuous")
	}

	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "IfIndex" {
				t.Errorf("%s uses the arrival interface (IfIndex). On Windows the control "+
					"message is nil with a NIL ERROR, so any decision made on it silently "+
					"accepts everything there. The self-filter is the nonce, in the payload.",
					name)
			}
			return true
		})
	}

	// And EVERY SetControlMessage error must be checked, not merely one of them.
	//
	// The first version asked whether such a check existed anywhere in the package.
	// There are two calls — v4 and v6 — so discarding one left the other satisfying
	// the guard, and a probe that did exactly that came back GREEN. Counted now:
	// every call site must sit in an if-with-init, which is what puts its error into
	// Stats().NoControlMessage.
	calls, checked := 0, 0
	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetControlMessage" {
				calls++
			}
			ifst, ok := n.(*ast.IfStmt)
			if !ok || ifst.Init == nil {
				return true
			}
			as, ok := ifst.Init.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, rhs := range as.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetControlMessage" {
					checked++
				}
			}
			return true
		})
	}
	if calls == 0 {
		t.Fatal("nothing calls SetControlMessage; the count below would pass on an absence")
	}
	if checked != calls {
		t.Errorf("%d of %d SetControlMessage calls have their error checked. On Windows that "+
			"error is the only signal the arrival interface will be unavailable, and one "+
			"unchecked call is enough to make Stats().NoControlMessage wrong.", checked, calls)
	}
}

// TestOffLinkSourcesAreRejected is the ingress scope boundary.
//
// Go binds a multicast listener to the WILDCARD, so this port accepts ordinary unicast
// from any host that can route to it — that was measured, not inferred: a unicast
// datagram sent to 127.0.0.1:8446 parsed and came back as a peer. The hop limit is the
// scope guarantee on the way OUT; on the way IN there was none, so anyone who could land
// a packet here could inject a candidate for any peer whose six-word name they knew, and
// names are public by design.
//
// This does not stop an on-link attacker spoofing a source — nothing at this layer could
// — but it removes every attacker who is not on the link.
func TestOffLinkSourcesAreRejected(t *testing.T) {
	_, lan, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	s := &Socket{nets: []*net.IPNet{lan}}

	for _, tc := range []struct {
		ip   string
		want bool
		why  string
	}{
		{"192.168.1.9", true, "on a joined link's subnet"},
		{"127.0.0.1", true, "loopback — two Nibs on one machine is a case the design serves, " +
			"and tier 4's LAN run depends on it"},
		{"::1", true, "loopback, v6"},
		{"fe80::1", true, "link-local: what the family is for, and a peer may have no other " +
			"address yet"},
		{"8.8.8.8", false, "off-link — the remote injection case"},
		{"192.168.2.9", false, "a private address on a DIFFERENT subnet is still not on our link"},
		{"2606:4700::1111", false, "a routable v6 address off-link"},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			got := s.onLink(&net.UDPAddr{IP: net.ParseIP(tc.ip), Port: 1})
			if got != tc.want {
				t.Errorf("onLink(%s) = %v, want %v — %s", tc.ip, got, tc.want, tc.why)
			}
		})
	}
	// A nil address is not on any link. The read loop can produce one if the source is
	// not a UDPAddr, and defaulting that to "on-link" would be the wrong direction.
	if s.onLink(nil) {
		t.Error("a nil source counted as on-link")
	}
	// And with NO joined networks nothing routable is on-link — the failure direction
	// must be closed, not open.
	empty := &Socket{}
	if empty.onLink(&net.UDPAddr{IP: net.ParseIP("192.168.1.9")}) {
		t.Error("with no joined links, a routable address counted as on-link — a socket that " +
			"joined nothing must accept nothing, not everything")
	}
}

// TestAnOffLinkUnicastIsDroppedByTheReadLoop drives the scope check where it lives.
//
// TestOffLinkSourcesAreRejected tests the PREDICATE. That is not the same claim, and a
// probe proved it: removing the call from Read left that test green, because a predicate
// with no caller passes its own unit test perfectly. This one sends a real datagram from
// a real address on a link the socket did not join.
//
// It needs two subnets, so it runs only in the namespace build/mcastrepro.sh builds.
func TestAnOffLinkUnicastIsDroppedByTheReadLoop(t *testing.T) {
	if os.Getenv("NIB_MCAST_NETNS") != "1" {
		t.Skip("not in a prepared network namespace — run build/mcastrepro.sh")
	}
	all, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	var onIf, offIf net.Interface
	for _, ifi := range all {
		switch ifi.Name {
		case "d0":
			onIf = ifi
		case "d3":
			offIf = ifi
		}
	}
	if onIf.Name == "" || offIf.Name == "" {
		t.Skipf("this namespace lacks d0/d3; the off-link case is unexercised (have %v)",
			describeAll(all))
	}
	offIP := firstV4(t, offIf)

	var nonce [nonceLen]byte
	nonce[0] = 0x5a
	// Joined on d0 ONLY, so d3's address is genuinely off-link for this socket.
	sock, err := open([]net.Interface{onIf}, nonce)
	if err != nil {
		t.Fatalf("open on d0 only: %v", err)
	}
	defer sock.Close()

	send := func(from net.IP, payload []byte) {
		t.Helper()
		c, err := net.DialUDP("udp4",
			&net.UDPAddr{IP: from},
			&net.UDPAddr{IP: firstV4(t, onIf), Port: Port})
		if err != nil {
			t.Fatalf("dial from %s: %v", from, err)
		}
		defer c.Close()
		if _, err := c.Write(payload); err != nil {
			t.Fatalf("write from %s: %v", from, err)
		}
	}

	good, err := Announcement{Name: aName(t, 33), Port: 8443, Nonce: [nonceLen]byte{1}}.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// Stimulus: the SAME datagram from an ON-link source is accepted. Without this,
	// "the off-link one was dropped" is equally true of a socket dropping everything.
	send(firstV4(t, onIf), good)
	if _, err := sock.Read(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("an on-link unicast was not accepted (%v) — the assertion below would pass "+
			"for a socket that accepts nothing", err)
	}
	before := sock.Stats()

	// And now the same bytes from off-link.
	send(offIP, good)
	got, err := sock.Read(time.Now().Add(2 * time.Second))
	if err == nil {
		t.Fatalf("an off-link unicast from %s was returned as a peer: %+v — anyone who can "+
			"route a packet to this port could inject a candidate for any peer whose "+
			"six-word name they know, and names are public by design", offIP, got)
	}
	if st := sock.Stats(); st.OffLink != before.OffLink+1 {
		t.Errorf("OffLink went %d -> %d, want +1 — it was rejected, but not as off-link, so "+
			"the counter a diagnostic reads would not show the attack",
			before.OffLink, st.OffLink)
	}
	t.Logf("OFFLINK %s rejected, on-link %s accepted", offIP, firstV4(t, onIf))
}

func firstV4(t *testing.T, ifi net.Interface) net.IP {
	t.Helper()
	addrs, err := ifi.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return n.IP
		}
	}
	t.Fatalf("%s has no IPv4 address", ifi.Name)
	return nil
}
