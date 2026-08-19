package discovery

import (
	"crypto/rand"
	"errors"
	"fmt"
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
