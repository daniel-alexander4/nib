package server

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/safe"
	"nib/internal/sign"
)

// TestTheFeedStopsWhenTheRaceIsWon — P05.S03's leak, found by grilling P05.S04.
//
// `raceCandidates` used to read its input with `for c := range in`, which ends only when
// the INPUT channel closes. The racer cancels on a win but CANNOT close `in` — it is the
// caller's channel — so against a trickle source (D16: candidates join as they arrive) the
// feeder goroutine blocked on the receive forever. Its deferred `close(results)` never ran,
// so the drain goroutine started on the win path never returned either: two goroutines, plus
// the whole graph they reference, leaked on every winning race.
//
// **Why no existing test could see it.** `dialAny` closes the channel it builds
// (`lan.go`), so every test that goes through it exits the loop the ordinary way. The one
// test that does drive an open channel — TestATrickleSourceThatNeverClosesStillHitsTheDeadline
// — drives the LOSS path, where the deadline ends the race and the same `defer cancel()`
// tidies up. The win path with an open channel had no test at all, and it is exactly the
// shape P05.S04 introduces: a DHT feed that stays open for the whole 300 s connect deadline
// while the LAN tier wins in three seconds.
//
// **The observable is the feeder still CONSUMING, not a goroutine count.** A count is flaky
// under a parallel suite and cannot say which goroutine leaked. Offering one more candidate
// after the race has returned answers exactly the question: a live feeder takes it, a
// returned one leaves it in the channel.
func TestTheFeedStopsWhenTheRaceIsWon(t *testing.T) {
	aCert, aKey, err := sign.GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	bCert, bKey, err := sign.GenerateIdentity("B")
	if err != nil {
		t.Fatal(err)
	}
	aFP, err := sign.Fingerprint(aCert)
	if err != nil {
		t.Fatal(err)
	}
	bFP, err := sign.Fingerprint(bCert)
	if err != nil {
		t.Fatal(err)
	}

	live := liveListener(t, bCert, bKey, aFP)

	// UNBUFFERED and never closed — a trickle source. Buffering would let the probe's send
	// succeed into the buffer with no feeder alive at all, which is the vacuous version of
	// this test: green whether the fix is there or not.
	in := make(chan candidate)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() { in <- candidate{Addr: live, Transport: "tcp", Source: sourceLAN} }()

	conn, rerr := raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
		return dialPeerWithin(ctx, c.Transport, c.Addr, aCert, aKey, bFP, lanDialTimeout, nil)
	})
	// STIMULUS: the race really won. Without this the probe below passes against a race
	// that failed instantly and whose feeder exited for an entirely different reason.
	if rerr != nil || conn == nil {
		t.Fatalf("setup: the race did not produce a winner (%v), so there is no win path "+
			"to observe and the probe below would pass for the wrong reason", rerr)
	}
	defer conn.Close()

	// Let the feeder observe the cancel. Generous, because the failure this is looking for
	// is a goroutine that never leaves — not one that is slow.
	time.Sleep(250 * time.Millisecond)

	select {
	case in <- candidate{Addr: blackHole, Transport: "tcp", Source: sourceLAN}:
		t.Fatal("a candidate offered AFTER the race returned was consumed, so the feed " +
			"goroutine is still running: it is blocked on the input channel with no ctx arm, " +
			"its close(results) will never run, and the drain goroutine leaks with it. Under " +
			"P05.S04 that is a DHT feed still fetching from strangers for the rest of the " +
			"connect deadline after the LAN tier already won.")
	case <-time.After(time.Second):
		// Nobody is reading: the feeder returned on ctx.Done, as it must.
	}
}

// TestOneSourceCannotSpendTheWholeRaceBudget — P05.S03's per-source acceptance bullet,
// which shipped unmet and unledgered.
//
// `maxRaceCandidates` is D16's law and is global. Checking it ALONE is first-come, and
// first-come is won by whoever emits fastest — so with two tiers feeding one channel the
// flooding one takes every slot and the genuine peer is never dialled. That is the capture
// attack `maxLANCandidates` closed at the browse level, re-opened one layer up, and under
// D6 an attacker supplies one of the sources.
//
// Invisible until now because there was exactly one source, and with one source a global
// cap and a per-source cap are the same number.
func TestOneSourceCannotSpendTheWholeRaceBudget(t *testing.T) {
	cert, key, err := sign.GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the flood must be bigger than the per-source cap, or the assertion below is
	// true of an unfixed racer too.
	flood := maxCandidatesPerSource + 4
	if flood > maxRaceCandidates {
		t.Fatalf("setup: %d floods the GLOBAL cap of %d as well, so this test cannot tell a "+
			"per-source bound from the global one", flood, maxRaceCandidates)
	}

	in := make(chan candidate, flood)
	for i := 0; i < flood; i++ {
		// Distinct addresses: the dedupe would otherwise do the capping and the test would
		// pass with no cap at all.
		in <- candidate{Addr: fmt.Sprintf("203.0.113.%d:9", i+1), Transport: "tcp", Source: sourceLAN}
	}
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	_, rerr := raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
		return dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, make([]byte, 32), lanDialTimeout, nil)
	})
	if rerr == nil {
		t.Fatal("setup: every candidate is a black hole, so the race must lose")
	}
	// `tried` in the sentence is what was actually dialled.
	want := fmt.Sprintf("tried %d address(es)", maxCandidatesPerSource)
	if !strings.Contains(rerr.Error(), want) {
		t.Errorf("one source offered %d candidates and the race reported %q; it must dial at "+
			"most %d from a single source, or a flooding tier spends the whole budget and the "+
			"honest tier is never reached", flood, rerr.Error(), maxCandidatesPerSource)
	}
	// And the drop must SAY which source, because "dropped 4" cannot tell a user or a
	// reviewer which tier was flooding.
	if !strings.Contains(rerr.Error(), sourceLAN.String()) {
		t.Errorf("the failure sentence is %q; a per-source cap whose report does not name the "+
			"source is a split nobody can read", rerr.Error())
	}
}

// TestTwoSourcesEachGetTheirShare is the other half, and without it the test above is
// satisfied by a racer that simply lowered the global cap to eight.
func TestTwoSourcesEachGetTheirShare(t *testing.T) {
	cert, key, err := sign.GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	in := make(chan candidate, 2*maxCandidatesPerSource)
	for i := 0; i < maxCandidatesPerSource; i++ {
		in <- candidate{Addr: fmt.Sprintf("203.0.113.%d:9", i+1), Transport: "tcp", Source: sourceLAN}
		in <- candidate{Addr: fmt.Sprintf("198.51.100.%d:9", i+1), Transport: "tcp", Source: sourceTyped}
	}
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	_, rerr := raceCandidates(ctx, in, func(ctx context.Context, c candidate) (*p2p.Conn, error) {
		return dialPeerWithin(ctx, c.Transport, c.Addr, cert, key, make([]byte, 32), lanDialTimeout, nil)
	})
	if rerr == nil {
		t.Fatal("setup: every candidate is a black hole, so the race must lose")
	}
	want := fmt.Sprintf("tried %d address(es)", 2*maxCandidatesPerSource)
	if !strings.Contains(rerr.Error(), want) {
		t.Errorf("two sources offered %d each and the race reported %q; each source is entitled "+
			"to its own %d, so a racer that capped the TOTAL at %d would starve the second tier",
			maxCandidatesPerSource, rerr.Error(), maxCandidatesPerSource, maxCandidatesPerSource)
	}
}

// TestSafeRecoverActuallyRecovers — internal/safe had no test of any kind.
//
// This matters because of the guard below it: an AST check that every detached goroutine
// defers safe.Recover is satisfied by the NAME. Gut the function body to nothing and the
// guard stays green while every goroutine in the tree is unprotected — the
// "satisfied by a word in a comment" shape, one level down. So the guard polices the
// routing and this polices what the routing reaches.
func TestSafeRecoverActuallyRecovers(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// **The outer recover is what makes this red for its own reason.** Without it, a gutted
	// safe.Recover lets the panic escape, the test binary dies on a raw stack, and the run
	// aborts — a red that names nothing and takes the rest of the package with it. That is
	// `redproof.sh`'s third failure mode, and it is worth six lines to avoid.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the panic escaped safe.Recover and reached this frame (%v). Every "+
				"detached goroutine in internal/server defers safe.Recover as its first "+
				"statement, and the AST guard below is satisfied by that NAME — so if the "+
				"function itself does not swallow a panic, every one of those goroutines is "+
				"unprotected while the guard stays green.", r)
		}
	}()

	survived := func() (ok bool) {
		defer func() { ok = true }()
		defer safe.Recover("unit under test")
		panic("a hostile peer's malformed frame")
	}()

	if !survived {
		t.Fatal("safe.Recover did not swallow the panic")
	}
	// And it must SAY which goroutine, or a recovered panic is an invisible one.
	if !strings.Contains(buf.String(), "unit under test") {
		t.Errorf("recovered, but the log line is %q — it must name the label, because the "+
			"whole point of recovering rather than crashing is that somebody can find out "+
			"afterwards what failed", buf.String())
	}
	if !strings.Contains(buf.String(), "a hostile peer's malformed frame") {
		t.Errorf("the log line is %q — it must carry the panic value", buf.String())
	}
}

// TestEveryDetachedGoroutineIsRecovered LIVED HERE, and now lives at the repo root in
// `goroutines_test.go`. It was scoped to `pkgs["server"]`, and the law it enforces is not a
// law about this package: `internal/udpmux`'s readLoop, the shared socket's sole reader of
// untrusted inbound datagrams, shipped with no recover because this guard could not see it.
// Moved rather than copied — ADR-009, a rule gets one door — and the root version walks
// `time.AfterFunc` too, which found two more.

// TestEveryCandidateProducerNamesItsSource — the guard for the defect I nearly shipped.
//
// The per-source cap is only as good as the labelling: `resolve` built LAN candidates
// without a Source, so every one of them carried the zero value and was accounted to the
// typed-address source. The tests were green because they set the field by hand. That is
// the vacuous green in its purest form — the fixture supplying what production omits — and
// it would have made the whole split silently wrong the day the rendezvous became the
// second source.
//
// So the rule is checked at the SOURCE, not through a fixture: every keyed `candidate`
// composite literal in this package's non-test files sets Source. P05.S04 adds a third
// producer; this is what stops it inheriting the hole.
func TestEveryCandidateProducerNamesItsSource(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["server"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("setup: internal/server did not parse — this guard walked nothing")
	}

	// **Elided literals are resolved from their PARENT, not assumed.** `[]candidate{{…}}`
	// and `[]ceremony.Endpoint{{Addr: …}}` are indistinguishable by shape — both are an
	// elided literal with an `Addr` key — and treating every elided one as a candidate made
	// this guard fire on `publishCandidates` building a perfectly correct Endpoint. l1_test's
	// own `candidateLit` takes the opposite shortcut and is right to: it keys on
	// `Fingerprint`, which a non-candidate struct does not have, so its false positive costs
	// nothing. This one keys on `Addr`, which they share.
	//
	// So: only literals whose type is spelled reach the check, and a composite whose type is
	// `[]candidate` or `map[K]candidate` lends its type to the elements inside it.
	var keyed int
	check := func(file string, cl *ast.CompositeLit) {
		if len(cl.Elts) == 0 {
			return
		}
		var hasSource, hasAddr bool
		for _, e := range cl.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				t.Errorf("%s:%d builds a candidate with unkeyed fields; keep it keyed so "+
					"a new field cannot be absorbed by position",
					filepath.Base(file), fset.Position(cl.Pos()).Line)
				return
			}
			switch k, _ := kv.Key.(*ast.Ident); k.Name {
			case "Source":
				hasSource = true
			case "Addr":
				hasAddr = true
			}
		}
		if !hasAddr {
			return
		}
		keyed++
		if !hasSource {
			t.Errorf("%s:%d builds a dialable candidate without a Source. It will be "+
				"accounted to the zero-value source, so one tier spends another tier's "+
				"share of the race and the per-source cap reports the wrong tier as the "+
				"one flooding.", filepath.Base(file), fset.Position(cl.Pos()).Line)
		}
	}
	isCandidateType := func(e ast.Expr) bool {
		switch t := e.(type) {
		case *ast.Ident:
			return t.Name == "candidate"
		case *ast.ArrayType:
			id, ok := t.Elt.(*ast.Ident)
			return ok && id.Name == "candidate"
		case *ast.MapType:
			id, ok := t.Value.(*ast.Ident)
			return ok && id.Name == "candidate"
		}
		return false
	}
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok || cl.Type == nil || !isCandidateType(cl.Type) {
				return true
			}
			if id, ok := cl.Type.(*ast.Ident); ok && id.Name == "candidate" {
				check(name, cl)
				return true
			}
			// A slice or map OF candidates: each element is one, elided or not.
			for _, e := range cl.Elts {
				inner := e
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					inner = kv.Value
				}
				if el, ok := inner.(*ast.CompositeLit); ok {
					check(name, el)
				}
			}
			return true
		})
	}

	// STIMULUS: the walk really found producers. Without this, a rename of the type — or a
	// glob that matched nothing — reports a clean bill over zero literals.
	if keyed == 0 {
		t.Fatal("setup: found no keyed candidate literal with an Addr in internal/server, so " +
			"this guard is looking for the wrong thing rather than proving there is nothing wrong")
	}
	if keyed < 2 {
		t.Fatalf("setup: found only %d candidate producer(s); there are at least two — the "+
			"link-local browse and the typed address — so this walk is not seeing them all", keyed)
	}
}
