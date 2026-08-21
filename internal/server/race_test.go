package server

import (
	"errors"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// blackHole is an address that accepts nothing and refuses nothing — a TEST-NET-3 host, which
// drops rather than resets, so a dial to it hangs for its full timeout instead of failing fast.
// That is what makes it a usable stand-in for a candidate the ladder will really produce: a
// published endpoint behind a firewall.
const blackHole = "203.0.113.1:9"

// liveListener arms a real pinned listener and returns its address.
func liveListener(t *testing.T, cert, key, peerFP []byte) string {
	t.Helper()
	ln, err := p2p.Listen("127.0.0.1:0", cert, key, peerFP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c
		}
	}()
	return ln.Addr().String()
}

// TestTheRaceDoesNotWaitForADeadCandidateAheadOfALiveOne — criterion 10, and the assertion
// that separates a race from a walk.
//
// **The vacuous version this is built to kill** is an ordered collection over concurrent
// dials: launch every goroutine, then read their results in slice order. Every dial is
// genuinely concurrent and a test that only measures "did it connect" is green — the dead
// candidate still errors, just later. Putting the dead one FIRST and measuring the clock is
// what tells the two apart: serially it costs `lanDialTimeout` before the live one is even
// attempted.
func TestTheRaceDoesNotWaitForADeadCandidateAheadOfALiveOne(t *testing.T) {
	if testing.Short() {
		t.Skip("spends real wall-clock against a black-holed address")
	}
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	bCert, bKey, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	aFP, _ := sign.Fingerprint(aCert)
	bFP, _ := sign.Fingerprint(bCert)
	live := liveListener(t, bCert, bKey, aFP)

	// STIMULUS: the black hole really does hang rather than refuse. Without this the test
	// is green against a serial walk too, because a fast refusal costs nothing.
	start := time.Now()
	if c, derr := dialAny([]candidate{{Addr: blackHole, Transport: "tcp"}}, aCert, aKey, bFP); derr == nil {
		c.Close()
		t.Fatal("setup: the black-holed address answered — it is not a black hole here")
	}
	if hang := time.Since(start); hang < 2*time.Second {
		t.Fatalf("setup: the black-holed address failed in %v rather than hanging; a walk "+
			"would not be slowed by it either, so the timing assertion below proves nothing", hang)
	}

	start = time.Now()
	conn, err := dialAny([]candidate{
		{Addr: blackHole, Transport: "tcp"}, // FIRST, as a walk would suffer it
		{Addr: live, Transport: "tcp"},
	}, aCert, aKey, bFP)
	if err != nil {
		t.Fatalf("the race did not reach the live candidate: %v", err)
	}
	conn.Close()
	elapsed := time.Since(start)
	if elapsed >= lanDialTimeout {
		t.Errorf("the race took %v with a dead candidate first and a live one second; one "+
			"dial's timeout is %v, so it waited for the dead one instead of racing past it",
			elapsed, lanDialTimeout)
	}
}

// TestALateCandidateJoinsTheRaceInFlight — criterion 11, verbatim.
//
// **The vacuous version is drain-then-race**: read the input until it is quiet, then start
// every dial. A test whose "late" candidate arrives a few milliseconds after the first would
// be green against it. Two things kill it here: the late candidate arrives only after the
// first dial is already in flight AND STILL BLOCKED, and **the channel is never closed** — a
// drain-then-race implementation would wait for a close that never comes and this test would
// time out rather than pass.
func TestALateCandidateJoinsTheRaceInFlight(t *testing.T) {
	if testing.Short() {
		t.Skip("spends real wall-clock against a black-holed address")
	}
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	bCert, bKey, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	aFP, _ := sign.Fingerprint(aCert)
	bFP, _ := sign.Fingerprint(bCert)
	live := liveListener(t, bCert, bKey, aFP)

	in := make(chan candidate) // deliberately never closed
	go func() {
		in <- candidate{Addr: blackHole, Transport: "tcp"}
		// Long enough that the first dial is unambiguously in flight, far short of its
		// 6 s timeout so the win cannot be explained by that dial having finished.
		time.Sleep(750 * time.Millisecond)
		in <- candidate{Addr: live, Transport: "tcp"}
	}()

	start := time.Now()
	done := make(chan *p2p.Conn, 1)
	errc := make(chan error, 1)
	go func() {
		c, rerr := raceCandidates(in, aCert, aKey, bFP)
		if rerr != nil {
			errc <- rerr
			return
		}
		done <- c
	}()
	select {
	case c := <-done:
		c.Close()
		elapsed := time.Since(start)
		if elapsed >= lanDialTimeout {
			t.Errorf("the late candidate won after %v; one dial's timeout is %v, so the "+
				"race waited for the dead candidate rather than admitting the late one",
				elapsed, lanDialTimeout)
		}
	case rerr := <-errc:
		t.Fatalf("the race failed instead of admitting the late candidate: %v", rerr)
	case <-time.After(lanDialTimeout + 3*time.Second):
		t.Fatal("the race never returned — it is waiting for the candidate channel to " +
			"close, which is drain-then-race and not a race at all")
	}
}

// TestNoAbandonedConnectionIsLeftLiveAtThePeer — the property the whole racer rests on,
// asserted where it matters.
//
// A connection the racer abandons but leaves OPEN is accepted by the peer's serial loop and
// blocks it inside `p2p.Receive`'s verification read for the full `exchangeDeadline` — six
// minutes, four past this side's own connect deadline — while the winner sits queued.
// P05.S02 did not change that: it fixed the handshake pool, and `runSession` still runs
// `serveOneSession` inline.
//
// **The assertion is on the PEER, not on the racer.** "The racer called Close" is
// unobservable from the racer and would be satisfied by a Close that never reached the wire.
// What matters is what the far end's read does.
//
// **The racer has two mechanisms and this test does not choose between them.** A loser whose
// dial is still in flight is CANCELLED and never reaches the peer at all; one that completed
// before the winner was picked is CLOSED. Both satisfy the property — nothing abandoned is
// left live — and which one fires is a race against the network.
//
// **Declared limit, because the count is the honest part:** on loopback the cancel path
// dominates, so this test usually observes one connection arriving rather than several. The
// close path is therefore under-exercised here and the test says how many actually arrived.
// Driving the close path deterministically needs a listener that completes a handshake and
// then stalls the winner, which is S08's punch harness rather than this slice's.
func TestNoAbandonedConnectionIsLeftLiveAtThePeer(t *testing.T) {
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	bCert, bKey, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	aFP, _ := sign.Fingerprint(aCert)
	bFP, _ := sign.Fingerprint(bCert)

	// FOUR live listeners for one pinned identity — a multi-homed peer, which is the
	// ordinary case the racer was built for and the one that produces losers.
	accepted := make(chan *p2p.Conn, 8)
	var addrs []candidate
	for i := 0; i < 4; i++ {
		ln, lerr := p2p.Listen("127.0.0.1:0", bCert, bKey, aFP)
		if lerr != nil {
			t.Fatal(lerr)
		}
		defer ln.Close()
		go func() {
			for {
				c, aerr := ln.Accept()
				if aerr != nil {
					return
				}
				accepted <- c
			}
		}()
		addrs = append(addrs, candidate{Addr: ln.Addr().String(), Transport: "tcp"})
	}

	winner, err := dialAny(addrs, aCert, aKey, bFP)
	if err != nil {
		t.Fatalf("the race found none of the four listeners: %v", err)
	}
	defer winner.Close()

	// Let anything still in flight land or be cancelled.
	time.Sleep(1500 * time.Millisecond)

	var peers []*p2p.Conn
	for done := false; !done; {
		select {
		case c := <-accepted:
			peers = append(peers, c)
		default:
			done = true
		}
	}
	// STIMULUS: the race really connected to somebody. Zero arrivals would make the
	// "at most one live" assertion below true of a race that dialled nothing.
	if len(peers) == 0 {
		t.Fatal("setup: no connection reached any peer, so there is nothing to observe")
	}

	live := 0
	for _, c := range peers {
		_ = c.Stream.SetDeadline(time.Now().Add(2 * time.Second))
		var buf [1]byte
		if _, rerr := c.Stream.Read(buf[:]); rerr == nil {
			live++
		} else if !isTimeout(rerr) {
			// A read that ERRORED is a dead connection — the loser, closed. A read that
			// TIMED OUT is a live connection with nothing on it, which is the winner.
			continue
		} else {
			live++
		}
		c.Close()
	}
	t.Logf("%d connection(s) reached the peers; %d live after the race", len(peers), live)
	if live > 1 {
		t.Errorf("%d of %d connections at the peer are still live after the race; at most "+
			"the winner may be — an abandoned connection left open blocks the peer's accept "+
			"loop inside the verification read for the whole exchange deadline",
			live, len(peers))
	}
}

// isTimeout reports a deadline expiry rather than a dead connection. A live-but-silent
// connection times out; a closed one returns EOF or a reset, and the difference is the whole
// point of the assertion above.
func isTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	var t timeout
	return errors.As(err, &t) && t.Timeout()
}
