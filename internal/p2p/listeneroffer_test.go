package p2p

import "testing"

// TestAHandshakeFinishingAfterCloseIsClosed — /pending 807 R7.
//
// Both handshake goroutines selected between the buffered send to `ready` and `<-l.done`. After
// Close, `done` is closed and `ready` has room, so Go chose uniformly and half the handshakes that
// finished after Close's drain sat in a queue nothing would ever read — a leaked connection, kept
// alive under QUIC by its own keep-alive. Every one must be closed, so this offers many.
func TestAHandshakeFinishingAfterCloseIsClosed(t *testing.T) {
	l := newListenerCore()
	l.shut = func() error { return nil }
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	const n = 64
	closed := 0
	for i := 0; i < n; i++ {
		l.offer(&Conn{closer: func() error { closed++; return nil }})
	}
	if closed != n || len(l.ready) != 0 {
		t.Fatalf("%d of %d handshakes finishing after Close were closed and %d sit queued for an "+
			"accept loop that has ended — each is a leaked connection", closed, n, len(l.ready))
	}

	// The control: on a live listener the connection is QUEUED for Accept, not closed.
	live := newListenerCore()
	shut := false
	live.offer(&Conn{closer: func() error { shut = true; return nil }})
	if shut || len(live.ready) != 1 {
		t.Fatalf("control: a live listener closed=%v and queued %d, want it queued", shut, len(live.ready))
	}
}
