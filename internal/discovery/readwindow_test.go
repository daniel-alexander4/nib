package discovery

import (
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
)

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
	if el := time.Since(start); el > window+200*time.Millisecond {
		t.Errorf("the window ran %s, well past its %s deadline", el, window)
	}
}
