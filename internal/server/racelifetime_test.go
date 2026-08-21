package server

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	conn, rerr := raceCandidates(ctx, in, aCert, aKey, bFP)
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
	_, rerr := raceCandidates(ctx, in, cert, key, make([]byte, 32))
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
	_, rerr := raceCandidates(ctx, in, cert, key, make([]byte, 32))
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

// TestEveryDetachedGoroutineIsRecovered is the census, made into a guard.
//
// `lan.go` used to assert in a comment that its announcer was "the one `go func` in
// internal/server without it". That was true when written and false from P05.S03, which
// added four more and recovered none — a sentence describing a census nobody re-ran. A
// comment cannot notice a fifth goroutine; this can.
//
// It asserts the ROUTING (ADR-009): every `go` statement in the package launches a function
// literal whose first statement is `defer safe.Recover(...)`. It deliberately does NOT check
// the label text — eight sites checked for agreement say nothing about a ninth added without
// one, and the label is not the property.
func TestEveryDetachedGoroutineIsRecovered(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["server"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("setup: internal/server did not parse — every check below would pass on nothing")
	}

	// **A bare `go f()` is not automatically a finding, and the first draft of this guard
	// said it was.** `go s.runSession(...)` is recovered — one frame down, at the top of
	// runSession itself — so a check that only looks at the `go` statement reports a defect
	// where there is none, and a guard that cries wolf gets relaxed rather than obeyed. So
	// first collect which package-level functions recover themselves, then resolve callees
	// against that. It is one hop, not a call graph: a goroutine two frames from its recover
	// is a shape nobody should be able to write without saying why.
	recovers := map[string]bool{}
	for _, f := range pkg.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || len(fd.Body.List) == 0 {
				continue
			}
			if def, ok := fd.Body.List[0].(*ast.DeferStmt); ok && isSafeRecover(def.Call) {
				recovers[fd.Name.Name] = true
			}
		}
	}
	// STIMULUS for the map itself: if this were empty every bare `go f()` below would be
	// reported, and the guard would fail loudly for the wrong reason.
	if len(recovers) == 0 {
		t.Fatal("setup: no function in internal/server defers safe.Recover first, so the " +
			"callee resolution below cannot have worked")
	}

	var found, viaCallee int
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			g, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			found++
			pos := fset.Position(g.Pos())
			lit, ok := g.Call.Fun.(*ast.FuncLit)
			if !ok {
				// A bare call: recovered only if the callee recovers itself.
				if calleeRecovers(g.Call.Fun, recovers) {
					viaCallee++
					return true
				}
				t.Errorf("%s:%d launches `go %s(...)`, and that function does not defer "+
					"safe.Recover as its first statement. A bare call has nowhere to hang a "+
					"defer, so either the callee recovers itself or the call is wrapped in a "+
					"function literal that does.",
					filepath.Base(name), pos.Line, types.ExprString(g.Call.Fun))
				return true
			}
			if len(lit.Body.List) == 0 {
				t.Errorf("%s:%d launches an empty goroutine", filepath.Base(name), pos.Line)
				return true
			}
			first, ok := lit.Body.List[0].(*ast.DeferStmt)
			if !ok || !isSafeRecover(first.Call) {
				t.Errorf("%s:%d launches a goroutine whose first statement is not "+
					"`defer safe.Recover(...)`. An unrecovered panic in ANY goroutine takes "+
					"the desktop process down with the user's unsaved documents, and "+
					"safe.Recover's own doc requires it at the very top so the goroutine's "+
					"other defers still run as the stack unwinds.",
					filepath.Base(name), pos.Line)
			}
			return true
		})
	}

	// STIMULUS, both directions. Without the first, a walk that parsed nothing reports a
	// clean bill; without the second, a package that had stopped using goroutines entirely
	// would too.
	if found == 0 {
		t.Fatal("setup: no `go` statement found in internal/server — this guard walked nothing")
	}
	if found < 5 {
		t.Fatalf("setup: found only %d `go` statements; P05.S03 alone added four to lan.go "+
			"and the announcer makes five, so this walk is not seeing the whole package", found)
	}
	// And the callee arm must have been EXERCISED. Without this, a rewrite that turned
	// every bare `go f()` into a literal would leave the resolution above dead — carried
	// forever, never run, and wrong the day somebody relies on it.
	if viaCallee == 0 {
		t.Error("no `go f()` resolved through a self-recovering callee, so that arm of this " +
			"guard ran against nothing. If the last such call site was deliberately rewritten " +
			"as a literal, delete the resolution rather than leaving it unexercised.")
	}
}

// calleeRecovers reports whether a bare `go f()` or `go x.f()` names a package-level
// function that defers safe.Recover as its own first statement.
func calleeRecovers(fun ast.Expr, recovers map[string]bool) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return recovers[f.Name]
	case *ast.SelectorExpr:
		return recovers[f.Sel.Name]
	}
	return false
}

// isSafeRecover reports whether a call is `safe.Recover(...)`, matched on the selector
// rather than on the rendered text so a renamed import does not silently pass.
func isSafeRecover(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Recover" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "safe"
}

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

	var keyed int
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			// **An ELIDED literal counts.** `[]candidate{{Addr: ...}}` has a nil Type,
			// because the element type comes from the slice — and the typed-address
			// producer is written exactly that way. The first draft of this guard matched
			// only `candidate{...}`, found one producer of two, and its own stimulus
			// assertion said so before it ever guarded anything. Same case `candidateLit`
			// handles in l1_test.go, for the same reason.
			switch typ := cl.Type.(type) {
			case nil: // elided inside a []candidate literal
			case *ast.Ident:
				if typ.Name != "candidate" {
					return true
				}
			default:
				return true
			}
			if len(cl.Elts) == 0 {
				return true
			}
			// An EMPTY `candidate{}` is a not-found sentinel, not a producer — skipped by
			// the len check above. A populated one is a producer.
			var hasSource, hasAddr bool
			for _, e := range cl.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					// Unkeyed: refused outright rather than parsed positionally. A
					// positional literal reorders silently when a field is inserted.
					t.Errorf("%s:%d builds a candidate with unkeyed fields; keep it keyed so "+
						"a new field cannot be absorbed by position",
						filepath.Base(name), fset.Position(cl.Pos()).Line)
					return true
				}
				switch k, _ := kv.Key.(*ast.Ident); k.Name {
				case "Source":
					hasSource = true
				case "Addr":
					hasAddr = true
				}
			}
			if !hasAddr {
				return true // not a dialable candidate
			}
			keyed++
			if !hasSource {
				t.Errorf("%s:%d builds a dialable candidate without a Source. It will be "+
					"accounted to the zero-value source, so one tier spends another tier's "+
					"share of the race and the per-source cap reports the wrong tier as the "+
					"one flooding.", filepath.Base(name), fset.Position(cl.Pos()).Line)
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
