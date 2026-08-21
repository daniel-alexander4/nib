package p2p

import (
	"context"
	"testing"
	"time"

	"nib/internal/sign"
)

// TestManyCompletedHandshakesDoNotStarveTheNextPeer — P05.S02, both transports.
//
// Until P05.S03's racer, at most ONE connection ever completed a pinned handshake against an
// armed listener, because the server's dialer walked candidates serially and returned on the
// first success. A racing dialer completes all of them against a dual-stack or multi-homed
// peer, keeps one, and drops the rest — so this listener's behaviour under N simultaneous
// completed handshakes stops being hypothetical.
//
// The defect it drives: a completed handshake used to park on an UNBUFFERED `ready` holding
// one of `maxConcurrentHandshakes` slots, released only by the deferred `<-l.sem` **after**
// that send resolved. Closing the connection from the far end does not end a channel send, and
// the read deadline is cleared before the park — so N abandoned connections took N slots for
// as long as the session lived. At N >= maxConcurrentHandshakes the accept loop itself blocks
// acquiring a slot and **the next peer is never handshaked at all**.
//
// So the assertion is not "the losers were cleaned up" — that is unobservable from here. It is
// **the next peer still gets in**, which is the only thing the starvation actually costs.
func TestManyCompletedHandshakesDoNotStarveTheNextPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("opens maxConcurrentHandshakes+ connections per transport")
	}
	eachTransport(t, func(t *testing.T, tr transport) {
		certA, keyA, err := sign.GenerateIdentity("Alice")
		if err != nil {
			t.Fatal(err)
		}
		certB, keyB, err := sign.GenerateIdentity("Bob")
		if err != nil {
			t.Fatal(err)
		}
		fpA, err := sign.Fingerprint(certA)
		if err != nil {
			t.Fatal(err)
		}
		fpB, err := sign.Fingerprint(certB)
		if err != nil {
			t.Fatal(err)
		}

		ln, err := tr.listen("127.0.0.1:0", certB, keyB, fpA)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		// **Accept must be running first.** The loop starts lazily from Accept
		// (`start()`), so a listener nobody has accepted on does not accept sockets at
		// all — dialling into one times out for a reason that has nothing to do with
		// this test. The server's real shape is exactly this: `runSession` calls Accept
		// and then serves what it gets, for the length of a ceremony.
		served := make(chan *Conn, 1)
		go func() {
			c, aerr := ln.Accept()
			if aerr != nil {
				served <- nil
				return
			}
			served <- c
		}()
		first, err := tr.dial(context.Background(), ln.Addr().String(), certA, keyA, fpB, 10*time.Second)
		if err != nil {
			t.Fatalf("setup: the first pinned dial failed: %v", err)
		}
		defer first.Close()
		// **The dialer must SPEAK for a QUIC connection to be accepted at all.** A QUIC
		// stream is invisible to the peer until data crosses it (`quic.go`'s stream
		// accept), so a connection that completes its handshake and says nothing is
		// never accepted — which is precisely the abandoned-loser shape below, and
		// precisely why it had to come off the accept path. The setup connection is the
		// one that must NOT be a loser, so it writes.
		if werr := writeFrame(first.Stream, []byte("hello")); werr != nil {
			t.Fatalf("setup: could not write the first frame: %v", werr)
		}
		select {
		case c := <-served:
			if c == nil {
				t.Fatal("setup: Accept errored on the first connection")
			}
			defer c.Close()
		case <-time.After(15 * time.Second):
			t.Fatal("setup: the first connection was never accepted, so nothing is " +
				"occupying the listener and the starvation below cannot occur")
		}

		// MORE than the pool, so the defect exhausts it. With the parked-send bug every
		// one of these takes a slot and never gives it back while the listener lives —
		// and nobody is calling Accept again, because the server is busy with the
		// session it just accepted.
		const extra = 4
		n := maxConcurrentHandshakes + extra
		for i := 0; i < n; i++ {
			c, derr := tr.dial(context.Background(), ln.Addr().String(), certA, keyA, fpB, 10*time.Second)
			if derr != nil {
				// Two readings, and which one applies is decided by WHERE it stopped.
				// Failing at exactly maxConcurrentHandshakes+1 is not a setup problem —
				// it IS the starvation, arriving one dial early: the pool is full of
				// connections nobody will accept, so the accept loop is blocked
				// acquiring a slot and this handshake never completes.
				if i+1 > maxConcurrentHandshakes {
					t.Fatalf("abandoned dial %d of %d did not complete a handshake: %v — "+
						"that is dial number %d against a pool of %d, so every %s handshake "+
						"slot is held by a connection nobody will ever accept and the accept "+
						"loop is blocked acquiring one. The next real peer cannot get in.",
						i+1, n, derr, i+1, maxConcurrentHandshakes, tr.name)
				}
				t.Fatalf("setup: abandoned dial %d of %d did not complete a handshake: %v "+
					"— it stopped BELOW the pool size, so this is not the starvation and "+
					"the assertion below would pass for the wrong reason", i+1, n, derr)
			}
			// Abandoned exactly as a racer abandons a loser: closed, never spoken on.
			c.Close()
		}

		// THE ASSERTION. One more pinned peer, after the pool has been hammered by
		// connections nobody will ever accept.
		done := make(chan error, 1)
		go func() {
			c, derr := tr.dial(context.Background(), ln.Addr().String(), certA, keyA, fpB, 20*time.Second)
			if c != nil {
				c.Close()
			}
			done <- derr
		}()
		select {
		case derr := <-done:
			if derr != nil {
				t.Fatalf("after %d abandoned handshakes the next pinned peer could not "+
					"connect: %v — every %s handshake slot is held by a connection nobody "+
					"will ever accept, which is the whole pool spent on our own dialer",
					n, derr, tr.name)
			}
		case <-time.After(25 * time.Second):
			t.Fatalf("after %d abandoned handshakes the next pinned peer's dial never "+
				"returned — the accept loop is blocked acquiring a handshake slot", n)
		}
	})
}
