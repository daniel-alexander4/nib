package rendezvous

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// TestCloseCancelsAndJoinsAnInFlightPublish — P05.S04 T10.
//
// Close used to do neither. It saved the node cache and called `dht.Server.Close`, which
// sets a flag and does `go s.socket.Close()` — so an in-flight traversal kept running against
// a server being torn down, kept writing into the Server's atomics, and ended only on its own
// 45-second budget. On a desktop process that is a shutdown that reports finished while a
// stranger's reply is still being processed.
//
// **The counters cannot see this and that is why the fake blocks.** `PublishAttempts`
// increments before the wire and `Published` after, so `attempts=1, published=0` is produced
// identically by an in-flight publish, a cancelled one, a seq-ceiling refusal and a
// getput error — four states, one signature. The observable has to be goroutine lifetime,
// so the test holds a `put` open and asks whether Close waited.
func TestCloseCancelsAndJoinsAnInFlightPublish(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	// ONE door for the release. Two closers — the timed one below and the cleanup — raced
	// and panicked with "close of closed channel", which killed the whole package run and
	// took eleven unrelated tests with it. A test that leaves work armed does not fail
	// politely; the repo's tier-2 note says the same thing about a hung one.
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	var puts atomic.Int64
	var once atomic.Bool

	f := newFakeNode(t, "127.0.0.72", func(q krpc.Msg, id krpc.ID) []byte {
		if q.A == nil {
			return nil
		}
		r := krpc.Return{ID: id}
		switch q.Q {
		case "ping", "find_node":
		case "get":
			tok := "tok"
			r.Token = &tok
		case "put":
			puts.Add(1)
			if once.CompareAndSwap(false, true) {
				close(arrived)
				// Hold the traversal open until the test lets go. This is the whole
				// instrument: without it, Publish returns before Close is even called and
				// the join assertion is true of a Close that joins nothing.
				<-release
			}
		default:
			return nil
		}
		b, err := bencode.Marshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &r})
		if err != nil {
			return nil
		}
		return b
	})
	// Always released, so a failure cannot hang the package — this repo's tier-2 lesson is
	// that a test which leaves work armed does not fail, it HANGS, and cleanup after an
	// assertion is skipped exactly when it is needed.
	t.Cleanup(letGo)

	n := nodeSeeded(t, f)
	rz := n.rz

	seed := make([]byte, 32)
	seed[0] = 9
	salt := []byte("close-join")

	var pubReturned atomic.Bool
	var pubErr error
	pubDone := make(chan struct{})
	go func() {
		pubErr = rz.Publish(context.Background(), seed, salt, []byte("v"))
		pubReturned.Store(true)
		close(pubDone)
	}()

	// STIMULUS: the publish really reached the network. Without this, everything below is
	// true of a publish that failed locally and never started a traversal at all.
	select {
	case <-arrived:
	case <-time.After(20 * time.Second):
		t.Fatal("setup: no put reached the fake node, so the publish never got to the " +
			"wire and there is nothing in flight for Close to join")
	}
	if pubReturned.Load() {
		t.Fatal("setup: Publish returned before Close was called — the fake did not hold " +
			"the traversal open, so the join assertion below cannot fail")
	}

	closed := make(chan error, 1)
	closeStart := time.Now()
	go func() { closed <- rz.Close() }()

	// Release on a timer rather than immediately. **This is what makes the cancel assertion
	// able to fail**: if Close does not cancel, the publish runs until the fake answers, so
	// Close cannot return before this fires. If it does cancel, the publish comes back at
	// once and Close is finished long before.
	const hold = 3 * time.Second
	go func() { time.Sleep(hold); letGo() }()

	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return within 30s. Either the wait deadlocked — the sync.Once " +
			"trap, where in-flight work re-enters Do — or it is waiting out the publish's " +
			"full 45s budget.")
	}
	took := time.Since(closeStart)

	// **THE CANCEL ASSERTION.** Joining without cancelling makes Close wait for the
	// operation's whole remaining budget — up to 45 seconds, on the path a user reaches by
	// quitting the app. Close must end the work, not outlive it.
	if took >= hold {
		t.Errorf("Close took %v, which is at least as long as the %v the fake held the "+
			"publish for — so it waited the operation out instead of cancelling it. That is "+
			"a hang wearing a clean shutdown.", took, hold)
	}
	// Wait for the publish goroutine to have STORED its error before reading it — otherwise
	// this is both a read-too-early and a data race the detector would report.
	select {
	case <-pubDone:
	case <-time.After(10 * time.Second):
		t.Fatal("the Publish goroutine never returned after Close")
	}
	if !errors.Is(pubErr, context.Canceled) {
		t.Errorf("the in-flight Publish returned %v; want context.Canceled", pubErr)
	}

	// **THE JOIN ASSERTION, and its limit is declared rather than implied.**
	//
	// With cancellation prompt, the publish returns within microseconds of Close cancelling
	// it — so a Close that skipped the wait would *usually* still see this true. This
	// assertion is therefore a regression guard on an invariant the code guarantees
	// structurally, NOT a demonstrated red: removing `inFlight.Wait()` does not reliably
	// fail it, and a flaky red proof is worth less than an honest note.
	//
	// The first version of this test asserted the join by timing and released the publish
	// immediately, which made BOTH red proofs come back green — the stimulus was gone before
	// the question was asked. That is why the cancel assertion above is the one carrying the
	// weight here.
	if !pubReturned.Load() {
		t.Error("Close returned while a Publish was still running")
	}

	if puts.Load() == 0 {
		t.Error("setup: no put was counted, so the fake never carried the operation")
	}
}

// TestWorkStartedAfterCloseIsRefusedRatherThanQueued.
//
// The alternative shapes both fail to terminate: work admitted after the WaitGroup has been
// waited on is either abandoned silently or deadlocks the Close it was admitted behind. A
// refusal is the only answer that ends.
func TestWorkStartedAfterCloseIsRefusedRatherThanQueued(t *testing.T) {
	f := newFakeNode(t, "127.0.0.73", func(q krpc.Msg, id krpc.ID) []byte {
		if q.A == nil {
			return nil
		}
		r := krpc.Return{ID: id}
		if q.Q == "get" {
			tok := "tok"
			r.Token = &tok
		}
		b, err := bencode.Marshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &r})
		if err != nil {
			return nil
		}
		return b
	})
	n := nodeSeeded(t, f)
	rz := n.rz

	seed := make([]byte, 32)
	seed[0] = 11
	salt := []byte("after-close")

	// SETUP: it works BEFORE Close. Without this the refusal below is equally true of a
	// Publish that never worked at all.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rz.Publish(ctx, seed, salt, []byte("v")); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("setup: a publish before Close failed with %v, so the refusal after Close "+
			"proves nothing", err)
	}

	if err := rz.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	start := time.Now()
	err := rz.Publish(context.Background(), seed, salt, []byte("v"))
	if err == nil {
		t.Fatal("a Publish started after Close was accepted. It would run against a torn-down " +
			"DHT server and write into counters nobody will read again.")
	}
	// And refused PROMPTLY: a refusal that first waits out a 45-second budget is a hang
	// wearing an error.
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the refusal took %v — it must be immediate, not the publish budget "+
			"elapsing against a dead server", d)
	}

	// Close is idempotent and must not deadlock on a second call.
	if err := rz.Close(); err != nil {
		t.Errorf("a second Close returned %v", err)
	}
	_ = bep44.Target{}
}
