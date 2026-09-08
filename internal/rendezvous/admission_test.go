package rendezvous

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCloseAndEnterCompose — /pending 387, defect 2, and **this is not the proof**.
//
// # What it is, stated first because the honest label matters more than the coverage
//
// It drives `enter`/`leave` against `Close` under contention and asserts the two compose: no
// panic, no deadlock, no operation admitted after `Close` has returned. It is a smoke test with
// a stimulus floor, and it is kept for what it would catch LATER — a teardown reordering that
// deadlocks, or an admission that outlives the join.
//
// **It was measured against the unfixed `enter` and did not go red: 0 of 10 runs, in three
// successive shapes** — one-shot racers, spinning racers, and spinning racers with a warm-up
// barrier so `Close` lands in a steady state rather than at goroutine-start. (The second shape
// went red 1 in 10 and that red was the STIMULUS floor firing, not the assertion; the warm-up is
// what fixed it, and fixing it made the test greener rather than redder — which is the tell.)
//
// # Why no black-box test can prove this ordering
//
// The window is `stopLive()` to `inFlight.Wait()`: a handful of instructions. To be caught, a
// racer must be descheduled between reading `live.Err()` and calling `inFlight.Add(1)` for that
// entire span. Widening it enough to hit reliably means a synchronisation hook in production
// code, which is a worse trade than the defect. So the ordering is asserted where it is
// observable — in the source, by `TestAdmissionAndCloseShareOneLock` below, which DOES go red
// when the fix is reverted. This repo already uses that shape where a property is structural
// rather than behavioural (`p2p`'s SetDeadline count, `docattach_test.go`).
func TestCloseAndEnterCompose(t *testing.T) {
	const rounds, racers = 60, 6

	admissions := 0
	admittedAfterClose := 0

	for r := 0; r < rounds; r++ {
		n := nodeSeeded(t)
		s := n.rz

		var closeReturned atomic.Bool
		var late, ok atomic.Uint64
		var stop atomic.Bool

		// **The racers SPIN rather than each calling enter once**, and that is what makes this
		// test able to fail. A one-shot racer finishes its `enter` long before `Close` reaches
		// `inFlight.Wait()` — measured: 0 reds in 10 runs against the unfixed code — because the
		// window between `stopLive()` and the wait is a few instructions and nothing was aimed at
		// it. Spinning keeps the in-flight counter oscillating through zero for the whole of
		// Close's run, so a `Wait` really can observe zero and block, and a racer really can
		// `Add` behind it.
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for !stop.Load() {
					_, leave, err := s.enter(context.Background())
					if err != nil {
						return // closed: this racer is done, which is the correct outcome
					}
					ok.Add(1)
					if closeReturned.Load() {
						late.Add(1)
					}
					leave()
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// **Warm-up before Close, and the first shape of this test had none.** Without it
			// `Close` reached `inFlight.Wait()` while the racer goroutines were still being
			// scheduled, so it won every start and the stimulus floor below fired instead of the
			// assertion — a flaky red that says nothing. Close must land in a steady state where
			// the in-flight counter is genuinely oscillating through zero.
			for warm := 0; warm < 64 && ok.Load() == 0; warm++ {
				runtime.Gosched()
			}
			_ = s.Close()
			closeReturned.Store(true)
		}()

		close(start)
		wg.Wait()
		stop.Store(true)

		admissions += int(ok.Load())
		admittedAfterClose += int(late.Load())
	}

	// STIMULUS. A run where `Close` always won the start and every `enter` was refused would
	// report zero violations while never having entered the contested window at all — the
	// vacuous green this test is most likely to produce, and the one it actually produced in
	// its first shape.
	if admissions == 0 {
		t.Fatalf("not one admission succeeded across %d rounds — `Close` won every start, so the "+
			"check-then-act window was never entered and a clean result here says nothing", rounds)
	}
	if admittedAfterClose > 0 {
		t.Errorf("%d operation(s) of %d admitted were registered after Close() had already "+
			"returned, so they run against a torn-down DHT and Close's join promise is false. "+
			"enter's `live.Err()` check must be atomic with its `inFlight.Add(1)`", admittedAfterClose, admissions)
	}
}

// TestAdmissionAndCloseShareOneLock — /pending 387, defect 2, and this one is the proof.
//
// The property is an ORDERING between two functions, not a behaviour either produces, so it is
// asserted over the source. `TestCloseAndEnterCompose` above records why: the window is a few
// instructions and no black-box test reached it in 30 measured runs.
//
// What must hold: `enter` reads `live.Err()` and calls `inFlight.Add(1)` under `admit`, and
// `Close` performs `stopLive()` under the same lock. Either half alone is useless — a check
// under a lock that the transition does not take is exactly the defect wearing a mutex.
func TestAdmissionAndCloseShareOneLock(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "dht.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	bodies := map[string]string{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		var buf bytes.Buffer
		if perr := printer.Fprint(&buf, fset, fd.Body); perr != nil {
			t.Fatal(perr)
		}
		bodies[fd.Name.Name] = buf.String()
	}

	// STIMULUS: both functions were found and parsed. A rename leaves every assertion below
	// vacuously true against an empty string.
	for _, name := range []string{"enter", "Close"} {
		if len(bodies[name]) < 80 {
			t.Fatalf("%s's body did not parse out of dht.go (%d chars) — this guard is reading nothing",
				name, len(bodies[name]))
		}
	}

	// Asserted as index comparisons rather than as a text span, because `enter` unlocks TWICE —
	// once on the refusal path and once after registering — so "between the Lock and the first
	// Unlock" ends before `inFlight.Add(1)` and would fail against correct code. It did, on the
	// first run of this guard.
	at := func(body, needle string) int { return strings.Index(body, needle) }

	enter := bodies["enter"]
	lock, lastUnlock := at(enter, "s.admit.Lock()"), strings.LastIndex(enter, "s.admit.Unlock()")
	errCheck, add := at(enter, "s.live.Err()"), at(enter, "s.inFlight.Add(1)")
	if lock < 0 {
		t.Fatal("enter does not take s.admit at all — its `live.Err()` check and its " +
			"`inFlight.Add(1)` are a check-then-act against Close's `stopLive(); inFlight.Wait()`, " +
			"which admits work behind the join or panics the WaitGroup (/pending 387)")
	}
	if !(lock < errCheck && errCheck < lastUnlock) {
		t.Errorf("enter reads s.live.Err() outside s.admit (lock@%d, check@%d, unlock@%d), so the "+
			"refusal is not atomic with Close's transition and a caller can observe a live server "+
			"that is already closing", lock, errCheck, lastUnlock)
	}
	if !(lock < add && add < lastUnlock) {
		t.Errorf("enter calls s.inFlight.Add(1) outside s.admit (lock@%d, add@%d, unlock@%d), so an "+
			"operation can register after Close has begun waiting — the WaitGroup-misuse panic this "+
			"lock exists to prevent", lock, add, lastUnlock)
	}

	closing := bodies["Close"]
	cLock, cUnlock := at(closing, "s.admit.Lock()"), at(closing, "s.admit.Unlock()")
	stop, wait := at(closing, "s.stopLive()"), at(closing, "s.inFlight.Wait()")
	if cLock < 0 || !(cLock < stop && stop < cUnlock) {
		t.Errorf("Close performs stopLive() outside s.admit (lock@%d, stopLive@%d, unlock@%d). A "+
			"check under a lock the transition does not take is the original defect wearing a "+
			"mutex: enter can still read a live server, be descheduled, and register behind "+
			"inFlight.Wait()", cLock, stop, cUnlock)
	}
	// And the wait is NOT under the lock, or every would-be caller blocks for the length of a
	// traversal instead of being refused immediately.
	if wait > 0 && cUnlock > 0 && wait < cUnlock {
		t.Errorf("Close holds s.admit across inFlight.Wait() (wait@%d, unlock@%d), so a caller "+
			"arriving during shutdown blocks for the length of an in-flight traversal rather than "+
			"being refused at once", wait, cUnlock)
	}
}
