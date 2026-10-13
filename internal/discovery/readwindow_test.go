package discovery

import (
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// TestAnAnnouncementEveryInterfaceRefusedIsAnError — /pending 653 part 6.
//
// A send error was recorded only where choosing the interface had SUCCEEDED, so a socket on which
// `SetMulticastInterface` fails for every interface and both families returned (0, nil) with
// `SendErrors` zero: total failure, reported as "nothing was attempted". Driven over a closed
// socket, where choosing the interface fails at once.
func TestAnAnnouncementEveryInterfaceRefusedIsAnError(t *testing.T) {
	ifis, err := net.Interfaces()
	if err != nil || len(ifis) == 0 {
		t.Skipf("no interfaces to name: %v", err)
	}
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback UDP: %v", err)
	}
	s := &Socket{pc: pc, p4: ipv4.NewPacketConn(pc), p6: ipv6.NewPacketConn(pc), ifis: ifis[:1],
		group4Addr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9},
		group6Addr: &net.UDPAddr{IP: net.IPv6loopback, Port: 9}}
	pc.Close()

	n, err := s.Announce(tagged(t, 1, Announcement{Port: 8443, Hop: HopNone}))
	if n != 0 {
		t.Fatalf("setup: %d write(s) succeeded on a closed socket", n)
	}
	if err == nil {
		t.Error("Announce returned (0, nil) when every interface refused — total send failure reported as nothing attempted")
	}
	if got := s.Stats().SendErrors; got != 2 {
		t.Errorf("SendErrors = %d, want 2 (one interface, both families) — the failure is counted nowhere", got)
	}

	// The other side: with no interface to send on, nothing was attempted and nothing failed.
	none := &Socket{}
	if n, err := none.Announce(tagged(t, 1, Announcement{Port: 8443, Hop: HopNone})); n != 0 || err != nil {
		t.Errorf("Announce on a socket with no interfaces = (%d, %v), want (0, nil)", n, err)
	}
}

// TestAFailingSocketDoesNotSpinTheListenWindow — /pending 710 R4-7.
//
// Both listen loops (`nib discover`, `/api/lan/test`) retried at once on any Read error, so a socket
// that fails on every call spun a core for the whole window. Driven over a CLOSED socket, which
// fails at `SetReadDeadline` immediately and forever: the window must cost a handful of reads, not
// thousands.
func TestAFailingSocketDoesNotSpinTheListenWindow(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback UDP: %v", err)
	}
	s := &Socket{pc: pc, p4: ipv4.NewPacketConn(pc)}
	pc.Close()

	var reads int
	var last error
	window := 300 * time.Millisecond
	start := time.Now()
	s.ReadWindow(time.Now().Add(window), func(_ Seen, err error) {
		reads++
		last = err
	})
	if !errors.Is(last, net.ErrClosed) {
		t.Fatalf("setup: the closed socket's read failed with %v, not net.ErrClosed — this is not "+
			"the persistent-failure case", last)
	}
	// 300 ms at a 50 ms pause is about six reads; a spin is tens of thousands.
	if reads > 20 {
		t.Errorf("a socket failing on every read was read %d times in %s — the listen loop spins a "+
			"core for the whole window instead of pausing", reads, window)
	}
	// And each failure is counted (/pending 653 part 7): the window used to end with every
	// counter at zero and nothing saying the socket had not been read once.
	if got := s.Stats().ReadErrors; got != uint64(reads) {
		t.Errorf("ReadErrors = %d after %d failed reads — a dead socket reads as a quiet link", got, reads)
	}
	if el := time.Since(start); el > window+200*time.Millisecond {
		t.Errorf("the window ran %s, well past its %s deadline", el, window)
	}
}
