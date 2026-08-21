package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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
		c, rerr := raceCandidates(context.Background(), in, aCert, aKey, bFP)
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

// TestTheTwoLawFiguresBindTheRace — D16's size bound and concurrency bound, driven.
//
// D16's plan-review pin makes these **law, not tuning**: "the backoff bounds how fast the
// race emits and nothing bounds how much, and under the D6 pin an attacker supplies the
// candidates". Until this test they were two constants nothing read at runtime and nothing
// asserted — which is how a law figure becomes a number somebody later "tunes".
//
// Both are observed through the clock and the failure sentence rather than by inspecting the
// racer, because a test that reads the racer's own bookkeeping would pass against a racer
// that ignored it.
func TestTheTwoLawFiguresBindTheRace(t *testing.T) {
	if testing.Short() {
		t.Skip("spends two dial timeouts against black-holed addresses")
	}
	cert, key, err := sign.GenerateIdentity("Ada")
	if err != nil {
		t.Fatal(err)
	}
	// More candidates than the cap, every one black-holed so each costs a full timeout.
	over := 4
	var cands []candidate
	for i := 0; i < maxRaceCandidates+over; i++ {
		cands = append(cands, candidate{Addr: fmt.Sprintf("203.0.113.%d:9", i+1), Transport: "tcp"})
	}
	// STIMULUS: the fixture really does exceed the cap, and really does need more than one
	// wave at the concurrency bound. Both assertions below are vacuous otherwise.
	if len(cands) <= maxRaceCandidates {
		t.Fatalf("setup: %d candidates does not exceed the cap of %d", len(cands), maxRaceCandidates)
	}
	if maxRaceCandidates <= maxConcurrentDials {
		t.Fatalf("setup: the cap (%d) is not above the concurrency bound (%d), so a single "+
			"wave would dial everything and the timing assertion cannot see batching",
			maxRaceCandidates, maxConcurrentDials)
	}

	start := time.Now()
	_, derr := dialAny(cands, cert, key, make([]byte, 32))
	elapsed := time.Since(start)
	if derr == nil {
		t.Fatal("the race connected to TEST-NET-3")
	}

	// **The size bound**: the excess is dropped AND reported. D16's pin says "drops and
	// reports; it never fails the ceremony" — a drop with no reader is half the clause.
	want := fmt.Sprintf("dropped %d over the %d-candidate cap", over, maxRaceCandidates)
	if !strings.Contains(derr.Error(), want) {
		t.Errorf("the failure says %q; it must contain %q — the cap dropped candidates and "+
			"said nothing, which is the half of D16's pin that has no reader", derr, want)
	}

	// **The concurrency bound**: with the cap above the bound, dialling everything takes
	// more than one wave. One wave would mean the bound is not applied at all.
	if elapsed < lanDialTimeout+time.Second {
		t.Errorf("%d candidates finished in %v, which is about one dial timeout (%v) — they "+
			"were all dialled at once, so the concurrency bound of %d is not being applied "+
			"and our own racer can occupy a peer's whole handshake pool",
			maxRaceCandidates, elapsed, lanDialTimeout, maxConcurrentDials)
	}
}

// TestTheWinnerOutlivesTheRaceContext — criterion 17, in the only form that can fail.
//
// The clause is *"letting the connect deadline elapse in full leaves both the exchange budget
// and the ceremony deadline undiminished"*. **The obvious test of it is vacuous**: every
// entry point calls `SetDeadline(time.Now().Add(exchangeDeadline))` unconditionally
// (`internal/p2p/session.go`), so "burn the connect deadline, then read the exchange budget"
// compares two literals and passes with the racer deleted, or never written.
//
// The hazard the clause actually guards is the race's context bleeding into the session that
// follows it. `raceCandidates` bounds itself with `connectDeadline` and **cancels on return**
// — so if a dialer ever attached a teardown to that context, the winner would be destroyed
// microseconds after being chosen, or at t=300 s mid-ceremony with the local user's signature
// already spent. Both libraries detach deliberately (`crypto/tls` documents it; quic-go
// passes `context.WithoutCancel`), and this is the guard that keeps it true here.
//
// So the assertion is: **the connection still carries bytes after the race that produced it
// has been cancelled.** Its red proof is wiring the race context into the established conn.
func TestTheWinnerOutlivesTheRaceContext(t *testing.T) {
	for _, tc := range []struct{ name, transport string }{
		{"tcp", "tcp"},
		{"quic", "quic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
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

			var ln p2p.Listener
			if tc.transport == "quic" {
				ln, err = p2p.QUICListen("127.0.0.1:0", bCert, bKey, aFP)
			} else {
				ln, err = p2p.Listen("127.0.0.1:0", bCert, bKey, aFP)
			}
			if err != nil {
				t.Skipf("no %s listener available here: %v", tc.transport, err)
			}
			defer ln.Close()
			echoed := make(chan []byte, 1)
			go func() {
				c, aerr := ln.Accept()
				if aerr != nil {
					return
				}
				buf := make([]byte, 5)
				if _, rerr := io.ReadFull(c.Stream, buf); rerr == nil {
					echoed <- buf
				}
			}()

			conn, err := dialAny([]candidate{{Addr: ln.Addr().String(), Transport: tc.transport}},
				aCert, aKey, bFP)
			if err != nil {
				t.Fatalf("the race found no listener: %v", err)
			}
			defer conn.Close()

			// STIMULUS: the race really has ended, so its context really is cancelled.
			// `raceCandidates` cancels on return, so holding the winner here means we are
			// past that point — which is the whole condition under test. If the race were
			// still running, a live connection would prove nothing.
			//
			// **A beat before asserting, and it is not padding.** A teardown attached to
			// the race context runs on its own goroutine, so writing immediately races it
			// — and on TCP the write won, which made this assertion pass against the very
			// defect it exists to catch. Found by running the red proof: QUIC failed and
			// TCP did not. Waiting lets any such teardown happen first, so liveness here
			// means the connection is genuinely detached rather than merely not-yet-killed.
			time.Sleep(250 * time.Millisecond)

			// THE ASSERTION: bytes cross AFTER that cancellation.
			if _, werr := conn.Stream.Write([]byte("hello")); werr != nil {
				t.Fatalf("writing to the winner failed after the race context was cancelled: "+
					"%v — the race's context reached the established connection, so a ceremony "+
					"would die the moment the race ended, or at the connect deadline with the "+
					"local user's signature already spent", werr)
			}
			select {
			case got := <-echoed:
				if string(got) != "hello" {
					t.Errorf("the peer read %q, want %q", got, "hello")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the peer never received the bytes written after the race ended — " +
					"the winner did not survive its own race's cancellation")
			}
		})
	}
}

// TestATrickleSourceThatNeverClosesStillHitsTheDeadline — the defect this slice's own review
// found, and the one a trickle source makes reachable.
//
// The first racer read results with `for r := range results`, which ends only when the INPUT
// channel closes. **A trickle source stays open for the whole race by design** — D16 has
// candidates joining as they arrive, and S04's DHT feed will hold that channel open for the
// full connect deadline. So with every candidate failing, the race never returned: each dial
// was bounded and the race was not, and `/api/session/initiate` hung forever with the local
// user's document already signed.
//
// `dialAny` could never see it — it closes the channel it builds. Only a trickle source can,
// which is why no existing test caught it.
func TestATrickleSourceThatNeverClosesStillHitsTheDeadline(t *testing.T) {
	cert, key, err := sign.GenerateIdentity("Ada")
	if err != nil {
		t.Fatal(err)
	}
	// Short, because the point is the deadline being observed at all — `connectDeadline`
	// is five minutes and the caller owns it precisely so this is testable.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	in := make(chan candidate) // never closed, like a live gather
	go func() {
		in <- candidate{Addr: "127.0.0.1:1", Transport: "tcp"} // refused at once
		// and then nothing, forever — the shape of a gather that found one dead address
	}()

	done := make(chan error, 1)
	go func() {
		_, rerr := raceCandidates(ctx, in, cert, key, make([]byte, 32))
		done <- rerr
	}()
	select {
	case rerr := <-done:
		if rerr == nil {
			t.Fatal("the race succeeded against a refused address")
		}
		// STIMULUS: it really did attempt the candidate, so the return is the deadline
		// arriving rather than an empty race short-circuiting.
		if !strings.Contains(rerr.Error(), "address(es)") {
			t.Errorf("the failure does not name what was tried: %v", rerr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the race never returned — it is waiting on a channel a trickle source " +
			"holds open for the whole ceremony, so the connect deadline bounds each dial " +
			"and not the race, and the HTTP handler hangs with the document already signed")
	}
}
