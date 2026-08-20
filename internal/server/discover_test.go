package server

import (
	"bytes"
	"crypto/sha256"
	"go/parser"
	"go/token"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/discovery"
	"nib/internal/pairing"
	"nib/internal/vault"
)

// announcementFrom builds a Seen the way the wire does: a real Announcement, really
// ENCODED and really PARSED, so the resolution is driven against bytes that crossed a
// parser rather than against a struct someone filled in by hand.
func announcementFrom(t *testing.T, fp []byte, port uint16, from string) discovery.Seen {
	t.Helper()
	name, err := pairing.Name(fp)
	if err != nil {
		t.Fatal(err)
	}
	a := discovery.Announcement{Name: name, Port: port}
	b, err := a.Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := discovery.Parse(b)
	if err != nil {
		t.Fatalf("the fixture does not survive its own parser: %v", err)
	}
	return discovery.Seen{Announcement: parsed, From: &net.UDPAddr{IP: net.ParseIP(from), Port: discovery.Port}}
}

func fpOf(seed byte) []byte {
	h := sha256.Sum256([]byte{seed})
	return h[:]
}

// TestAPinnedPeerResolvesAndAnUnpinnedOneDoesNot is the acceptance clause, and both
// halves are here because only the pair distinguishes matching from accepting: a
// resolver that returned every announcement would satisfy the first half alone.
func TestAPinnedPeerResolvesAndAnUnpinnedOneDoesNot(t *testing.T) {
	pinned := fpOf(1)
	stranger := fpOf(2)
	pins := []vault.PinnedPeer{{Fingerprint: pinned, Label: "Bea"}}

	// The pinned peer.
	got, ok := resolve(pins, announcementFrom(t, pinned, 8443, "192.168.1.9"))
	if !ok {
		t.Fatal("a pinned peer's announcement did not resolve; nothing below is meaningful")
	}
	if !bytes.Equal(got.Fingerprint, pinned) {
		t.Errorf("resolved to %x, want the pinned %x", got.Fingerprint, pinned)
	}
	if got.Label != "Bea" {
		t.Errorf("label %q, want the pinned label", got.Label)
	}
	// The address is the OBSERVED source plus the ANNOUNCED port — the announcement
	// carries no address of its own, deliberately.
	if got.Addr != "192.168.1.9:8443" {
		t.Errorf("addr %q, want 192.168.1.9:8443 (observed source, announced port)", got.Addr)
	}

	// The stranger. Same shape of announcement, same everything, different identity.
	if c, ok := resolve(pins, announcementFrom(t, stranger, 8443, "192.168.1.9")); ok {
		t.Errorf("an UNPINNED peer resolved to %+v — discovery would be introducing a peer, "+
			"which L1 forbids and which the invitation path exists to do instead", c)
	}
}

// TestTheResolvedFingerprintIsThePinnedOneNotTheAnnouncedName guards the property that
// makes a lying announcer harmless.
//
// An announcement carries a NAME, which is 66 bits of a fingerprint — so two different
// fingerprints could in principle produce the same name. What must never happen is the
// candidate carrying anything derived from the wire: the fingerprint that goes to the
// TLS pin has to be the one in the vault, byte for byte.
func TestTheResolvedFingerprintIsThePinnedOneNotTheAnnouncedName(t *testing.T) {
	pinned := fpOf(3)
	pins := []vault.PinnedPeer{{Fingerprint: pinned, Label: "Bea"}}
	seen := announcementFrom(t, pinned, 9000, "10.0.0.5")

	got, ok := resolve(pins, seen)
	if !ok {
		t.Fatal("setup: the announcement did not resolve")
	}
	// Not merely equal — a DIFFERENT backing array. A candidate that aliased the
	// caller's slice would let one peer's identity be rewritten by another's.
	if &got.Fingerprint[0] == &pins[0].Fingerprint[0] {
		t.Error("the candidate aliases the vault's slice rather than copying it")
	}
	got.Fingerprint[0] ^= 0xff
	if bytes.Equal(pins[0].Fingerprint, got.Fingerprint) {
		t.Error("mutating the candidate changed the pin")
	}
}

// TestAnAnnouncementWithoutAPortOrSourceResolvesToNothing — the two fields the
// candidate is assembled from. Either missing makes a candidate that cannot be dialled,
// and returning one anyway would hand the caller an address like ":0".
func TestAnAnnouncementWithoutAPortOrSourceResolvesToNothing(t *testing.T) {
	fp := fpOf(4)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "Bea"}}

	full := announcementFrom(t, fp, 8443, "10.0.0.5")
	if _, ok := resolve(pins, full); !ok {
		t.Fatal("setup: a complete announcement does not resolve")
	}

	noSource := full
	noSource.From = nil
	if c, ok := resolve(pins, noSource); ok {
		t.Errorf("an announcement with no source resolved to %+v — the address half of the "+
			"candidate comes from the source, so this one cannot be dialled", c)
	}
	noPort := full
	noPort.Port = 0
	if c, ok := resolve(pins, noPort); ok {
		t.Errorf("an announcement with no port resolved to %+v", c)
	}
}

// fakeBrowser replays a script of reads. Errors are in the script too, because the real
// socket returns them constantly — our own loopback copies, foreign traffic, malformed
// datagrams — and a browse loop that mishandled them would spin or stop early.
type fakeBrowser struct {
	script []struct {
		seen discovery.Seen
		err  error
	}
	i      int
	closed bool
}

func (f *fakeBrowser) Read(deadline time.Time) (discovery.Seen, error) {
	if f.i >= len(f.script) {
		f.closed = true
		// Past the script: behave like a socket with nothing on it — block until
		// the deadline, then time out. This is what makes the window assertion real
		// rather than a script that happens to end.
		if d := time.Until(deadline); d > 0 {
			time.Sleep(d)
		}
		return discovery.Seen{}, os.ErrDeadlineExceeded
	}
	s := f.script[f.i]
	f.i++
	return s.seen, s.err
}

func TestBrowseCollectsEachPeerOnceAndSurvivesNoise(t *testing.T) {
	a, b := fpOf(5), fpOf(6)
	stranger := fpOf(7)
	pins := []vault.PinnedPeer{{Fingerprint: a, Label: "Ada"}, {Fingerprint: b, Label: "Bea"}}

	fb := &fakeBrowser{script: []struct {
		seen discovery.Seen
		err  error
	}{
		{err: discovery.ErrOwn}, // our own loopback copy
		{seen: announcementFrom(t, a, 8443, "10.0.0.1")},
		{err: discovery.ErrNotOurs},                      // something else on the link
		{seen: announcementFrom(t, a, 8443, "10.0.0.1")}, // a repeat: peers announce often
		{err: discovery.ErrMalformed},
		{seen: announcementFrom(t, stranger, 8443, "10.0.0.9")}, // not pinned
		{seen: announcementFrom(t, b, 9443, "10.0.0.2")},
	}}

	got := browsePeers(fb, pins, 500*time.Millisecond)

	// Stimulus: the whole script was consumed, so the loop did not stop at the first
	// error — which is what a browse over a shared link would do to itself.
	if !fb.closed {
		t.Fatalf("the browse stopped after %d of %d reads — an error on a shared link is "+
			"ordinary and must not end the window", fb.i, len(fb.script))
	}
	if len(got) != 2 {
		t.Fatalf("browse returned %d candidates, want 2 (Ada once despite two announcements, "+
			"Bea once, the stranger never): %+v", len(got), got)
	}
	if !bytes.Equal(got[0].Fingerprint, a) || got[0].Addr != "10.0.0.1:8443" {
		t.Errorf("first candidate is %+v, want Ada at 10.0.0.1:8443", got[0])
	}
	if !bytes.Equal(got[1].Fingerprint, b) || got[1].Addr != "10.0.0.2:9443" {
		t.Errorf("second candidate is %+v, want Bea at 10.0.0.2:9443", got[1])
	}
	for _, c := range got {
		if bytes.Equal(c.Fingerprint, stranger) {
			t.Error("the unpinned stranger became a candidate")
		}
	}
}

// TestTheBrowseWindowBoundsTheWait asserts D16's 2 s is a BOUND, not a sleep.
//
// A browse that always waited the full window would make the LAN tier cost two seconds
// even when it succeeds instantly, which is the opposite of why D16 gives it the
// shortest budget of any tier. And one that returned early on an empty link would
// break the tier's only job. So: the empty case takes about the window, and it does not
// take appreciably longer.
func TestTheBrowseWindowBoundsTheWait(t *testing.T) {
	pins := []vault.PinnedPeer{{Fingerprint: fpOf(8), Label: "Ada"}}
	fb := &fakeBrowser{}

	start := time.Now()
	got := browsePeers(fb, pins, 300*time.Millisecond)
	elapsed := time.Since(start)

	if len(got) != 0 {
		t.Fatalf("an empty link produced %d candidates", len(got))
	}
	if elapsed < 250*time.Millisecond {
		t.Errorf("the browse returned after %v, well inside its %v window — a tier that gives "+
			"up early cannot find a peer that answers late", elapsed, 300*time.Millisecond)
	}
	if elapsed > 2*time.Second {
		t.Errorf("the browse took %v for a %v window — the budget is a bound and this one "+
			"is not bounding", elapsed, 300*time.Millisecond)
	}
}

// TestBrowseWindowIsD16sTwoSeconds pins the constant against the decision that fixes it.
func TestBrowseWindowIsD16sTwoSeconds(t *testing.T) {
	if browseWindow != 2*time.Second {
		t.Errorf("browseWindow is %v; D16's table fixes the multicast browse at 2 s, and the "+
			"ladder's other budgets are written against it", browseWindow)
	}
}

// TestResolutionLivesOutsideTheDiscoveryPackage records WHY this file is here, and it
// checks the INVARIANT rather than a proxy for it.
//
// The first version asserted that ../discovery/announce_test.go *contained the string*
// "nib/internal/vault" — satisfied by a comment, by a leftover "we used to guard against
// nib/internal/vault" after the guard was deleted, or by the string anywhere at all. That
// is the strings-in-a-file hole, and it is the FOURTH instance in this repo: the .deb
// guard satisfied by a comment, published.test.mjs satisfied by a doc comment,
// TestNothingDecidesOnTheArrivalInterface matching "IfIndex" in its own explanation — and
// then this, written one file away from the third fix.
//
// So it parses the discovery package and asserts what actually matters: that package
// cannot see a pin, which is L1 made structural and the reason resolution lives here.
func TestResolutionLivesOutsideTheDiscoveryPackage(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../discovery", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["discovery"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("could not parse internal/discovery — the check below would pass on nothing")
	}
	for name, f := range pkg.Files {
		for _, imp := range f.Imports {
			switch strings.Trim(imp.Path.Value, `"`) {
			case "nib/internal/vault", "nib/internal/sign", "nib/internal/p2p":
				t.Errorf("%s imports %s. internal/discovery cannot be allowed to see a pin — "+
					"that is L1 made structural, and it is the whole reason this resolution "+
					"lives in internal/server instead", name, imp.Path.Value)
			}
		}
	}
}

// TestTwoHostsClaimingOneNameBothBecomeCandidates.
//
// The name is broadcast in the clear every 500 ms and is displayed beside a signature —
// it is not a secret. So two different HOSTS can claim one peer's name, and the earlier
// dedupe (by fingerprint alone) threw the loser away: an attacker announcing faster than
// the real peer captured the browse outright, and the genuine address was discarded
// where no caller could reach it. The dial still failed at the pin, so identity held —
// but the LAN tier was deniable by anyone who knew a name.
func TestTwoHostsClaimingOneNameBothBecomeCandidates(t *testing.T) {
	fp := fpOf(61)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "Bea"}}

	// The impostor announces first, as an attacker would — faster and earlier.
	fb := &fakeBrowser{script: []struct {
		seen discovery.Seen
		err  error
	}{
		{seen: announcementFrom(t, fp, 8443, "10.0.0.99")}, // not the real Bea
		{seen: announcementFrom(t, fp, 8443, "10.0.0.2")},  // the real one, second
		{seen: announcementFrom(t, fp, 8443, "10.0.0.99")}, // repeats are still deduped
	}}

	got := browsePeers(fb, pins, 400*time.Millisecond)

	// Stimulus: the script really was consumed, so "two candidates" is not one arrival
	// counted twice.
	if !fb.closed {
		t.Fatalf("the browse stopped after %d of %d reads", fb.i, len(fb.script))
	}
	if len(got) != 2 {
		t.Fatalf("browse returned %d candidates, want 2 — one address per host, deduped per "+
			"address rather than per peer: %+v", len(got), got)
	}
	if got[0].Addr != "10.0.0.99:8443" || got[1].Addr != "10.0.0.2:8443" {
		t.Errorf("candidates are %q and %q, want the impostor first (heard first) and the "+
			"real one second — order is first-heard, and BOTH must survive",
			got[0].Addr, got[1].Addr)
	}
	// Both carry the pinned fingerprint: the wire never supplies one, so an impostor's
	// candidate is still pinned to the real identity and fails at the handshake.
	for _, c := range got {
		if !bytes.Equal(c.Fingerprint, fp) {
			t.Errorf("candidate %q carries a fingerprint from somewhere other than the vault", c.Addr)
		}
	}
}

// TestDialAnyTriesEveryCandidate — the other half. Returning both is useless if only the
// first is dialled.
func TestDialAnyTriesEveryCandidate(t *testing.T) {
	// Two addresses that cannot connect; what matters is that BOTH were attempted, which
	// the error reports. A single-candidate dialAny would say "1 address(es)".
	_, err := dialAny("tcp", []string{"127.0.0.1:1", "127.0.0.1:2"}, nil, nil, nil)
	if err == nil {
		t.Fatal("dialling two dead addresses succeeded")
	}
	if !strings.Contains(err.Error(), "2 address(es)") {
		t.Errorf("error says %q — it must name how many were tried, or a silent "+
			"first-only dial is indistinguishable from a real exhaustion", err)
	}
	if _, err := dialAny("tcp", nil, nil, nil, nil); err == nil {
		t.Error("dialling an empty candidate list succeeded")
	}
}

// TestARealAnnouncementResolvesToACandidate closes the acceptance clause end to end:
// a real socket, a real datagram on a real group, and a candidate a caller could dial.
//
// It runs only inside the namespace build/mcastrepro.sh creates, for the reason that
// harness documents: a multicast loopback copy traverses INPUT, so a default-deny host
// swallows it silently and this would fail for the firewall's reasons rather than the
// code's.
//
// Two sockets in ONE process rather than two processes, because the property under test
// is the resolution and not the socket — tier 5's discovery test already drives two
// processes. They share the port (proven separately) and differ only in nonce, which is
// also what makes this a real exercise of the self-filter: without it the browser would
// discard the announcer's datagram as its own, since both leave from the same address.
func TestARealAnnouncementResolvesToACandidate(t *testing.T) {
	if os.Getenv("NIB_MCAST_NETNS") != "1" {
		t.Skip("not in a prepared network namespace — run build/mcastrepro.sh")
	}

	var nAnnounce, nBrowse [8]byte
	nAnnounce[0], nBrowse[0] = 0xa1, 0xb2

	peerFP := fpOf(42)
	name, err := pairing.Name(peerFP)
	if err != nil {
		t.Fatal(err)
	}

	announcer, err := discovery.Open(nAnnounce)
	if err != nil {
		t.Fatalf("open announcer: %v", err)
	}
	defer announcer.Close()
	browser, err := discovery.Open(nBrowse)
	if err != nil {
		t.Fatalf("open browser: %v", err)
	}
	defer browser.Close()

	// The announcer plays the peer: its announcement carries the peer's NAME, which
	// is all an announcement ever carries about identity.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		a := discovery.Announcement{Name: name, Port: 8443, Nonce: nAnnounce}
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := announcer.Announce(a); err != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Pinned: the browser holds this fingerprint already. That is the only way a name
	// on the wire can ever become an identity.
	pins := []vault.PinnedPeer{{Fingerprint: peerFP, Label: "Bea"}}
	got := browsePeers(browser, pins, 3*time.Second)

	// One candidate PER ADDRESS, not per peer — a host with three interfaces and two
	// families legitimately announces from several addresses, and all of them are worth
	// keeping so the caller can try each. (This assertion said "want 1" until the
	// dedupe was changed from per-fingerprint to per-address; that change is what stops
	// an impostor's announcement from displacing the real peer's.)
	if len(got) == 0 {
		t.Fatalf("browsed a real link and resolved nothing (stats %+v, %s)",
			browser.Stats(), browser.Describe())
	}
	for _, c := range got {
		if !bytes.Equal(c.Fingerprint, peerFP) {
			t.Errorf("candidate %q carries %x, want the pinned %x", c.Addr, c.Fingerprint, peerFP)
		}
		if c.Label != "Bea" {
			t.Errorf("candidate %q has label %q, want the pinned label", c.Addr, c.Label)
		}
		// Every one must be dialable — including the link-local ones, which is where
		// the missing-zone defect lived.
		if _, err := net.ResolveUDPAddr("udp", c.Addr); err != nil {
			t.Errorf("candidate %q does not resolve: %v", c.Addr, err)
		}
	}
	host, port, err := net.SplitHostPort(got[0].Addr)
	if err != nil {
		t.Fatalf("candidate address %q is not dialable: %v", got[0].Addr, err)
	}
	if port != "8443" {
		t.Errorf("candidate port %q, want the announced 8443", port)
	}
	ip := net.ParseIP(host)
	if i := strings.IndexByte(host, '%'); i >= 0 {
		ip = net.ParseIP(host[:i])
	}
	if ip == nil {
		t.Errorf("candidate host %q is not an address — it comes from the datagram's source, "+
			"which is the half an announcement deliberately does not carry", host)
	}
	if ip != nil && ip.IsLinkLocalUnicast() && !strings.Contains(host, "%") {
		t.Errorf("candidate host %q is link-local with NO ZONE — the kernel cannot pick an "+
			"interface for it and the dial fails with 'invalid argument'. On a link-local "+
			"discovery protocol this is the common case, not an edge", host)
	}
	t.Logf("RESOLVED %s at %s (stats %+v)", got[0].Label, got[0].Addr, browser.Stats())

	// And the other half, on the same real link: an announcement whose name matches
	// NOTHING pinned resolves to nothing. Without this the test above is satisfied by
	// a resolver that returns every announcement it hears.
	none := browsePeers(browser, []vault.PinnedPeer{{Fingerprint: fpOf(99), Label: "Nobody"}},
		1*time.Second)
	if len(none) != 0 {
		t.Errorf("an unpinned peer resolved to %+v on a real link", none)
	}
}

// TestALinkLocalCandidateCarriesItsZone is deterministic on purpose.
//
// The namespace test resolves whichever family answers first, and on a dual-stack link
// that is usually IPv4 — so the link-local path there is exercised by luck, and an
// assertion reached by luck is one that silently stops being reached. This drives the
// zone directly.
//
// The property: fe80::… is not dialable without the interface it was heard on
// (`dial tcp [fe80::1]:9: connect: invalid argument`, measured). net.UDPAddr keeps Zone
// separately from IP, so a candidate built from IP alone drops it and looks fine.
func TestALinkLocalCandidateCarriesItsZone(t *testing.T) {
	fp := fpOf(51)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "Bea"}}

	seen := announcementFrom(t, fp, 8443, "fe80::1")
	seen.From.Zone = "wlp1s0"

	// Stimulus: the fixture really is link-local, or the assertion below is about
	// an ordinary address and proves nothing.
	if !seen.From.IP.IsLinkLocalUnicast() {
		t.Fatal("the fixture is not a link-local address")
	}

	got, ok := resolve(pins, seen)
	if !ok {
		t.Fatal("a link-local announcement did not resolve at all")
	}
	if !strings.Contains(got.Addr, "%wlp1s0") {
		t.Fatalf("candidate %q dropped the zone — it is link-local, so without the interface "+
			"the kernel cannot pick a link and the dial fails with 'invalid argument'",
			got.Addr)
	}
	// And it is still a well-formed address, not just a string with a % in it.
	ua, err := net.ResolveUDPAddr("udp", got.Addr)
	if err != nil {
		t.Fatalf("candidate %q does not resolve: %v", got.Addr, err)
	}
	if ua.Zone != "wlp1s0" || ua.Port != 8443 {
		t.Errorf("resolved to zone %q port %d, want wlp1s0/8443", ua.Zone, ua.Port)
	}

	// An ordinary address must NOT grow a zone suffix — the other half, without which
	// this passes for a hostOf that appends "%" unconditionally.
	plain := announcementFrom(t, fp, 8443, "192.168.1.9")
	got2, ok := resolve(pins, plain)
	if !ok {
		t.Fatal("an ordinary announcement did not resolve")
	}
	if strings.Contains(got2.Addr, "%") {
		t.Errorf("a non-link-local candidate %q carries a zone", got2.Addr)
	}
}
