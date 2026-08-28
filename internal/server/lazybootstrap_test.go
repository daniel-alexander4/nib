package server

import (
	"context"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
	"time"
)

// P07.S05d — the DHT's first contact with the network is LAZY, and it has ONE door.
//
// P03's exit criterion says a LAN ceremony completes with no outbound internet traffic. Measured
// in the namespace with an nft counter: a two-party LAN ceremony emitted 0 packets and a
// four-party LAN relay emitted 120. Three sites bootstrapped eagerly — the dialer at construction
// and BOTH arm paths, which are different functions — so the criterion was false for every
// ceremony carrying an invitation, which is every ceremony P07 builds. The two-party `--lan` run
// could not see it, because it has no invitation and therefore no ceremony object at all.

// TestTheDHTBootstrapHasExactlyOneDoor is ADR-009's shape, and it is the guard that matters most
// here: the defect was not a wrong bootstrap, it was a THIRD site nobody had counted.
//
// It asserts routing through the door rather than the text at each site — "eight copies checked
// for agreement say nothing about a ninth site added without one". A fourth caller of
// `rz.Bootstrap` added to this package fails here, whatever it looks like.
func TestTheDHTBootstrapHasExactlyOneDoor(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var callers []string
	var sawEnsure bool
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}
				if fn.Name.Name == "ensureBootstrapped" {
					sawEnsure = true
				}
				ast.Inspect(fn, func(m ast.Node) bool {
					call, ok := m.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Bootstrap" {
						return true
					}
					callers = append(callers, fn.Name.Name+" ("+path+")")
					return true
				})
				return true
			})
		}
	}
	// SETUP: the scan can see this package's functions at all. Without this the assertion below
	// is satisfied by a parse that found nothing, which is the vacuous green this repo keeps
	// finding in its own guards.
	if !sawEnsure {
		t.Fatal("setup: the scan did not find ensureBootstrapped, so it is not reading this " +
			"package and an empty caller list proves nothing")
	}
	if len(callers) != 1 || !strings.HasPrefix(callers[0], "ensureBootstrapped ") {
		t.Fatalf("rz.Bootstrap must be called from ensureBootstrapped and nowhere else "+
			"(ADR-009: a rule gets one door, and its guard checks the door). Callers: %v",
			callers)
	}
}

// TestTheFeedDoesNotTouchTheDHTInsideTheLANWindow is the other half of the fix, and without it
// the first half buys nothing: a bootstrap deferred to first use, with an unwindowed fetch
// immediately after it, moves the first off-link packet by microseconds.
//
// `publishLoop` has always taken `browseWindow` as its `first` parameter and says why. The FETCH
// did not. This drives `feedCandidates` for a fraction of the window and asserts the door was
// never reached.
func TestTheFeedDoesNotTouchTheDHTInsideTheLANWindow(t *testing.T) {
	cer, peerFP := armedCeremony(t)

	// SETUP: the flag starts false, or "still false" below is true of a ceremony that never ran.
	if cer.bootstrapDone.Load() {
		t.Fatal("setup: bootstrapDone was already true before the feed started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan candidate, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		cer.feedCandidates(ctx, out, peerFP, "", "", browseWindow, rendezvousInterval)
	}()
	go func() {
		for range out { //nolint:revive // drain
		}
	}()

	// A fraction of the window. If the feed reaches the DHT eagerly it does so immediately, so
	// this does not need to be close to the boundary to catch it.
	slice := browseWindow / 10
	time.Sleep(slice)
	if cer.bootstrapDone.Load() {
		t.Fatalf("the candidate feed reached the DHT %s into a %s LAN window: a ceremony the "+
			"local link is about to answer emitted off-link traffic anyway, which is the "+
			"whole of P03's exit criterion (S05d)", slice, browseWindow)
	}
	cancel()
	<-done
}

// TestTheBootstrapDoorSetsItsFlagEvenWhenTheBootstrapFAILS is acceptance clause 3: a lazy
// bootstrap must not make `bootstrapDone` lie.
//
// The flag gates D19's arm-side diagnosis — until the bootstrap has had its chance, zero DHT
// responses means "still warming up" rather than "unreachable", and cause 2 then is a false
// alarm on a healthy machine. A flag set only on SUCCESS inverts that: the machine whose network
// is actually dead is the one that never gets told, because the diagnosis it needs is gated on
// the thing that failed.
func TestTheBootstrapDoorSetsItsFlagEvenWhenTheBootstrapFAILS(t *testing.T) {
	cer, _ := armedCeremony(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the bootstrap cannot succeed against a dead context
	err := cer.ensureBootstrapped(ctx)

	// SETUP: the attempt really did fail. If it somehow succeeded, "the flag is set" says
	// nothing about the failure path this test exists for.
	if err == nil {
		t.Skip("the bootstrap succeeded against a cancelled context; nothing to assert here")
	}
	if !cer.bootstrapDone.Load() {
		t.Fatal("the bootstrap failed and bootstrapDone stayed false, so D19's arm-side " +
			"diagnosis is gated forever on the machine that most needs it (clause 3)")
	}

	// Once per ceremony object — which is exactly the attempt count the three eager calls had.
	// The cached error is how a later caller learns what happened without a second attempt.
	if again := cer.ensureBootstrapped(context.Background()); again != err {
		t.Fatalf("the door bootstrapped twice for one ceremony: first %v, then %v", err, again)
	}
}

// armedCeremony builds a real ceremony with a live rendezvous over a loopback socket — the same
// path `sharedsocket_test.go` uses, because a fake rendezvous cannot answer the question these
// tests ask, which is whether the DHT was CONTACTED.
func armedCeremony(t *testing.T) (*ceremonyID, []byte) {
	t.Helper()
	invs, certs, fps := threeParty(t)
	peerFP, err := hex.DecodeString(fps[1])
	if err != nil {
		t.Fatal(err)
	}
	cer, err := ceremonyFor(mustEncode(t, invs[fps[1]]), certs[0], nil, peerFP)
	if err != nil {
		t.Fatal(err)
	}
	lnCert, lnKey, err := newIdentity(t)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := cer.openRendezvous(transportQUIC, "127.0.0.1:0", t.TempDir(), lnCert, lnKey, peerFP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close(); cer.close() })
	if cer.rz == nil {
		t.Fatal("setup: the arm produced no rendezvous server, so there is no DHT to reach")
	}
	return cer, peerFP
}

// TestTheAddedLatencyToTheDHTTierIsMeasured is acceptance clause 2: a ceremony with no LAN peer
// still reaches the DHT tier, and the cost of waiting is a NUMBER rather than an assumption.
//
// The cost is one-sided by construction. A ceremony the link answers pays nothing and emits
// nothing; a ceremony with no LAN peer waits out the window it was always going to wait out on
// the publish side, and now waits it out on the fetch side too. D8's ladder races the tiers
// concurrently, so this is added latency on ONE rung and not on the ceremony.
//
// Asserted as a band, not a point: a floor because a fetch that beat the window would be the
// leak back, and a ceiling because a tier that never starts is not a delayed tier.
func TestTheAddedLatencyToTheDHTTierIsMeasured(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the full LAN window")
	}
	cer, peerFP := armedCeremony(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*browseWindow)
	defer cancel()
	out := make(chan candidate, 8)
	go func() {
		for range out { //nolint:revive // drain
		}
	}()
	started := time.Now()
	go cer.feedCandidates(ctx, out, peerFP, "", "", browseWindow, rendezvousInterval)

	var reached time.Duration
	for reached == 0 {
		if cer.bootstrapDone.Load() {
			reached = time.Since(started)
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("with no LAN peer the DHT tier was never reached at all within %s: the "+
				"window deferred the tier instead of delaying it (clause 2)", 4*browseWindow)
		case <-time.After(5 * time.Millisecond):
		}
	}

	t.Logf("no LAN peer: the DHT tier is reached %s after the hop starts (LAN window %s) — the "+
		"added latency this slice introduces, on one rung of D8's concurrent ladder",
		reached.Round(time.Millisecond), browseWindow)

	if reached < browseWindow {
		t.Fatalf("the DHT was reached %s into a %s window, so the fetch is not behind the "+
			"window after all", reached, browseWindow)
	}
	if ceiling := browseWindow + bootstrapBudget; reached > ceiling {
		t.Fatalf("the DHT tier took %s to start, past %s: that is not the window's cost",
			reached, ceiling)
	}
}

// TestALANFoundPeerHoldsTheDHTPastTheBrowseWindow is the half the lazy bootstrap did not fix,
// and it is a two-arm test because one arm alone proves nothing.
//
// **Measured, and it is why `browseWindow` was the wrong figure.** With the bootstrap already
// lazy and both DHT verbs already behind a 2 s window, a nine-party LAN relay still emitted 105
// off-link packets. The stack trace named `publishWhenSlow`: its gate was a 2 s timer racing the
// hop, and hops take 1–3 s, so the timer won often enough to leak. The dial side does not have to
// guess — `peerAddresses` browses BEFORE this runs, so a `sourceLAN` candidate is the link having
// already answered.
//
// The control arm matters as much: a hold that applied to every dial would be a DHT tier that
// never starts, which is not a fix, it is a different outage.
func TestALANFoundPeerHoldsTheDHTPastTheBrowseWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the browse window twice")
	}
	past := browseWindow + browseWindow/2 // comfortably past the old gate, far short of the new one

	t.Run("found on the link: the DHT still has not been touched", func(t *testing.T) {
		cer, peerFP := armedCeremony(t)
		s := &Server{epoch: "test-epoch"}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out, wg := s.feedCeremonyRace(ctx, cer,
			[]candidate{{Addr: "10.9.0.2:7777", Source: sourceLAN}}, peerFP, "", "")
		go func() {
			for range out { //nolint:revive // drain
			}
		}()
		time.Sleep(past)
		touched := cer.bootstrapDone.Load()
		cancel()
		wg.Wait()
		if touched {
			t.Fatalf("the browse had already found the peer on the link and the dial reached the "+
				"public DHT %s later anyway: D6's suppression was a race between a %s timer and "+
				"the hop, and the hop wins (S05d)", past, browseWindow)
		}
	})

	t.Run("nothing on the link: the DHT tier still opens", func(t *testing.T) {
		cer, peerFP := armedCeremony(t)
		s := &Server{epoch: "test-epoch"}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out, wg := s.feedCeremonyRace(ctx, cer,
			[]candidate{{Addr: "203.0.113.7:7777", Source: sourceTyped}}, peerFP, "", "")
		go func() {
			for range out { //nolint:revive // drain
			}
		}()
		time.Sleep(past)
		touched := cer.bootstrapDone.Load()
		cancel()
		wg.Wait()
		if !touched {
			t.Fatalf("with no LAN candidate the DHT tier had still not started %s in: the hold "+
				"is not conditional on the browse at all, so it is an outage rather than a fix",
				past)
		}
	})
}
