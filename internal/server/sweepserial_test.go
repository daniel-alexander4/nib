package server

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// TestTwoCeremonySweepsNeverOverlap — /pending 515.
//
// **The overlap is reachable, and the reachability is the whole finding.** `adoptVault` launches its
// sweep whenever the vault goes from nil to open; `handleVaultImport` nils `s.vault` and calls
// `ensureUnlocked` again, so an import landing while the first sweep is still walking
// `~/nib/ceremonies` starts a second one over the same directory. `handleCeremonyAccept` is a third
// trigger. What overlapping costs is exactly what `adoptVault` refuses *inside* one sweep: the
// close-out and the re-arms *"read the same listing and reach opposite conclusions about the same
// ceremony — one arms a rendezvous for it, the other moves its directory out from under that arm"*.
//
// # Why the assertion is a concurrency counter and not `-race`
//
// `-race` reports what it happens to observe on the interleaving it happens to get, and the two
// sweeps race on the FILESYSTEM — a `rename` against a `ListStored` — which the detector cannot see
// at all. A count of how many sweep bodies were inside the door at once is the thing that is true or
// false independently of scheduling luck, and it goes red on every run without the lock.
//
// # HOME is isolated, and it is load-bearing rather than hygiene
//
// The bodies below call the REAL sweep, which reads `~/nib/ceremonies`. Without the isolation this
// test reads the developer's own ceremonies and can move their directories — the measured failure
// that put the `deliveryRearm` gate in front of this sweep in the first place.
func TestTwoCeremonySweepsNeverOverlap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// STIMULUS: with an isolated HOME there are no stored ceremonies, so the real sweep called
	// below touches nothing of the developer's. A non-empty listing here means this test is
	// reading a real directory and its green result would mean nothing.
	if got, err := ceremony.ListStored(defaultOutputDir(), time.Now()); err != nil || len(got) != 0 {
		t.Fatalf("setup: an isolated HOME still lists %d stored ceremon(ies) (err=%v)", len(got), err)
	}

	s := &Server{configDir: t.TempDir()}
	s.EnableDeliveryRearm()

	var inside, peak atomic.Int64
	var ran atomic.Int64
	bump := func() {
		n := inside.Add(1)
		for {
			was := peak.Load()
			if n <= was || peak.CompareAndSwap(was, n) {
				break
			}
		}
	}

	const sweeps = 4
	// Buffered and collected with a deadline rather than a WaitGroup, because a door that DROPS a
	// sweep never reaches the body at all — a `wg.Done` inside it would turn that outcome into a
	// hang and report it as the wrong thing.
	finished := make(chan struct{}, sweeps)
	for i := 0; i < sweeps; i++ {
		s.runCeremonySweep("test sweep", func() {
			bump()
			// The real work, so the isolated HOME above is doing something: a sweep over an
			// empty ceremonies directory, exactly as an unlock runs it with no vault yet.
			s.closeOutEnded(nil, time.Now())
			s.rearmDeliveries(nil)
			// Wide enough that an unserialized run overlaps on every scheduler this repo is
			// built on; the real bodies take milliseconds of disk and a socket bind.
			time.Sleep(20 * time.Millisecond)
			ran.Add(1)
			inside.Add(-1)
			finished <- struct{}{}
		})
	}
	timeout := time.After(30 * time.Second)
collect:
	for i := 0; i < sweeps; i++ {
		select {
		case <-finished:
		case <-timeout:
			break collect
		}
	}

	if got := peak.Load(); got != 1 {
		t.Errorf("%d sweeps of ~/nib/ceremonies were running at once (want 1). Two sweeps read the "+
			"same listing and reach opposite conclusions about the same ceremony — one arms a "+
			"rendezvous for it while the other moves its directory out from under that arm", got)
	}
	// STIMULUS, and the other half of the disposition. A door that DROPPED sweeps instead of
	// queueing them — "skip if one is already running" — holds the peak at 1 and passes the
	// assertion above. It is still wrong: an import brings a DIFFERENT identity, with a different
	// fingerprint and a different set of ceremonies to arm for, so its sweep is not a duplicate of
	// the one in flight. The same count catches a sweep wedged behind another, which is the cost a
	// mutex can impose and the reason the deadline above is generous rather than tight.
	if got := ran.Load(); got != sweeps {
		t.Errorf("%d of %d sweeps ran to completion. Serialized means queued, not coalesced and "+
			"not wedged: a dropped sweep leaves an imported identity unarmed until the next "+
			"unlock, and a wedged one is a ceremony that silently never re-arms", got, sweeps)
	}
}

// TestEveryCeremonySweepGoesThroughTheOneDoor — ADR-009's second half: *"the guard asserts routing
// through the door, not the text each site prints"*.
//
// The lock is worth nothing if a fourth trigger is added next to the three that exist and launches
// its own `go func`. The counter test above would stay green, because it drives the door directly.
// So this asserts the shape instead: `sweepMu` is taken in exactly one function, and both production
// triggers reach it by name.
func TestEveryCeremonySweepGoesThroughTheOneDoor(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	takers, scanned := map[string]int{}, 0
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		raw, rerr := os.ReadFile(n)
		if rerr != nil {
			t.Fatal(rerr)
		}
		scanned++
		if c := strings.Count(stripLineComments(string(raw)), "sweepMu.Lock()"); c > 0 {
			takers[n] = c
		}
	}
	// STIMULUS: a scan that read the wrong directory produces the same clean result as a correct
	// one, and this package is large enough that three files would already be wrong.
	if scanned < 10 {
		t.Fatalf("setup: scanned %d source file(s) — this guard is not reading internal/server", scanned)
	}
	if len(takers) != 1 || takers["ceremonyarm.go"] != 1 {
		t.Errorf("sweepMu is taken in %v, want exactly one place (ceremonyarm.go, runCeremonySweep). "+
			"A second taker is a second answer to \"is a sweep running\", which is the duplicate "+
			"derivation ADR-009 refuses", takers)
	}

	for _, f := range []string{"auth.go", "ceremonyarm.go"} {
		raw, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if !strings.Contains(stripLineComments(string(raw)), "s.runCeremonySweep(") {
			t.Errorf("%s no longer routes its sweep through runCeremonySweep. It reads "+
				"~/nib/ceremonies, moves directories and binds a socket; a trigger that launches "+
				"its own goroutine runs all of that beside a sweep already doing it", f)
		}
	}
}
