package server

import (
	"bufio"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// TestTheWindowStreamCarriesTheArmedState — P01.S05 T01, and the reason it exists at all.
//
// `beforeunload` cannot fetch, so the close prompt's ceremony half has to be sitting in the client
// before the user closes. `pollRecv` cannot supply it — that poller starts only when THIS window
// arms, so a window that never armed would report "nothing to lose" while a ceremony was running,
// which is the policy-armed case D5 most cares about.
func TestTheWindowStreamCarriesTheArmedState(t *testing.T) {
	ts, s := startServerWith(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/window", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open window stream: %v", err)
	}
	defer resp.Body.Close()
	lines := streamLines(bufio.NewReader(resp.Body))

	// The value is pushed on CONNECT, not only on change: a window that opens while a ceremony is
	// already armed must not wait for the next change to learn it.
	if got := readArmedEvent(t, lines); got.Armed {
		t.Errorf("a fresh stream pushed armed=%v, want false — nothing is armed yet", got)
	}

	// STIMULUS: the state really changes. Without arming something, the read below would return
	// the same value whether the push worked or not.
	if !s.sess.armIn(&arm{kind: armInteractive, addr: "127.0.0.1:0"}) {
		t.Fatal("setup: could not arm, so the change below is not a change")
	}
	if !s.sess.Armed() {
		t.Fatal("setup: armed a slot and Armed() still reports false")
	}
	if got := readArmedEvent(t, lines); !got.Armed {
		t.Errorf("after arming, the stream pushed armed=%v, want true. A window that never armed "+
			"itself would report nothing-to-lose while a ceremony was running (D5)", got)
	}

	// And the other direction, or the prompt would stick on forever after one ceremony.
	s.sess.disarm()
	if got := readArmedEvent(t, lines); got.Armed {
		t.Errorf("after disarming, the stream pushed armed=%v, want false — the prompt would stay "+
			"armed for the life of the window after any ceremony", got)
	}
}

// streamLines reads a stream's lines onto a channel, ONCE per stream.
//
// **One goroutine for the whole test and not one per read**, which the first version got wrong:
// each `readArmedEvent` call started its own reader on the same `bufio.Reader`, so two goroutines
// raced the same stream and the second call lost lines the first had already taken. It failed
// against CORRECT code, which is the worst kind of test bug — it looks like the product.
func streamLines(br *bufio.Reader) <-chan lineOrErr {
	out := make(chan lineOrErr, 32)
	go func() {
		for {
			s, err := br.ReadString('\n')
			out <- lineOrErr{strings.TrimRight(s, "\r\n"), err}
			if err != nil {
				return
			}
		}
	}()
	return out
}

type lineOrErr struct {
	s   string
	err error
}

// readArmedEvent reads one `event: armed` frame's data field off an already-running reader.
//
// **The deadline is a select, not a loop condition.** The first version checked the clock BETWEEN
// reads, and `ReadString` on a stream that sends nothing blocks forever — so a build that stopped
// pushing on change did not fail this test, it hung it, and the failure arrived as `panic: test
// timed out` forty-five seconds later with no sentence about what was wrong. A test whose red is a
// hang is a test whose red nobody reads.
func readArmedEvent(t *testing.T, lines <-chan lineOrErr) armedEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	seenEvent := false
	for {
		select {
		case <-deadline:
			t.Fatal("no armed event arrived within the deadline — the stream is not pushing the " +
				"armed state, so a window's close prompt never learns a ceremony is running (D5)")
			return armedEvent{}
		case l := <-lines:
			if l.err != nil {
				t.Fatalf("read stream: %v", l.err)
			}
			switch {
			case l.s == "event: armed":
				seenEvent = true
			case seenEvent && strings.HasPrefix(l.s, "data: "):
				// **Decoded rather than string-compared, and the format change is why.** P01.S05
				// pushed a bare bool and this read `== "true"`; S06 made it an object so the
				// modal could name the ceremony, and a string compare would have gone quietly
				// false rather than failing. It failed, which is the shape that gets noticed.
				var ev armedEvent
				if err := json.Unmarshal([]byte(strings.TrimPrefix(l.s, "data: ")), &ev); err != nil {
					t.Fatalf("armed event did not decode: %v (%q)", err, l.s)
				}
				return ev
			}
		}
	}
}

// TestEveryWriteToTheArmsTableAnnouncesIt — P01.S05, and the guard is the point.
//
// The armed state reaches a window's close prompt by a PUSH, so a site that writes `arms` without
// announcing it leaves every open window's prompt stale — and nothing else would notice, because
// the prompt is correct in the common case and wrong only for the window that closes next.
//
// **Asserted over the source rather than by behaviour**, because the failure is an absent call and
// absence is what a behavioural test cannot see: a fourth write site added tomorrow passes every
// test in this file.
func TestEveryWriteToTheArmsTableAnnouncesIt(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "session.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	writers := map[string]bool{}
	announcers := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// `se.arms[...] = ...` — an assignment INTO the table, not a read of it.
			if as, ok := n.(*ast.AssignStmt); ok {
				for _, lhs := range as.Lhs {
					ix, ok := lhs.(*ast.IndexExpr)
					if !ok {
						continue
					}
					if sel, ok := ix.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "arms" {
						writers[fn.Name.Name] = true
					}
				}
			}
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "armedChangedLocked" {
					announcers[fn.Name.Name] = true
				}
			}
			return true
		})
	}

	// STIMULUS: the scan found the writers at all. A rename makes every check below vacuous.
	if len(writers) < 3 {
		t.Fatalf("the scan found %d function(s) writing se.arms, want at least 3 (armIn, "+
			"displacePolicyArm, disarmWhen) — it is not reading session.go", len(writers))
	}

	var silent []string
	for name := range writers {
		if !announcers[name] {
			silent = append(silent, name)
		}
	}
	if len(silent) > 0 {
		t.Errorf("%v write(s) to the arms table without calling armedChangedLocked. Every open "+
			"window's close prompt then keeps a stale armed state — and it is wrong only for the "+
			"window that closes next, which is the one case nobody is watching (P01.S05, D5).",
			silent)
	}
}

// TestExitHasOneDoorAndIsIdempotent — P01.S06 T01, D6.
//
// D6's rule is that every exit path runs the same teardown in the same order, and the way that is
// kept true is that nothing tears down for itself: causes signal `RequestExit`, `run()` selects,
// and the teardown below it is the only one there has ever been. The grace (P01.S04) closed the
// channel directly until Quit needed to as well — which is exactly when a rule like this earns its
// keep, and exactly when it would otherwise have been quietly duplicated.
func TestExitHasOneDoorAndIsIdempotent(t *testing.T) {
	s := &Server{}
	select {
	case <-s.IdleExit():
		t.Fatal("a fresh server is already exiting")
	default:
	}

	s.RequestExit(exitCauseQuit)
	select {
	case <-s.IdleExit():
	case <-time.After(time.Second):
		t.Fatal("RequestExit did not signal the exit, so Quit would confirm and then do nothing")
	}

	// **Idempotent, because two causes genuinely race.** A user pressing Quit as the grace elapses
	// is ordinary rather than pathological, and `close` on a closed channel panics — which would
	// take the process down through `safe.Recover` on a path whose whole job is an orderly exit.
	s.RequestExit(exitCauseLastWindow)
	s.RequestExit(exitCauseQuit)
}

// TestTheQuitRouteSignalsTheSameExit — P01.S06 T02, the second acceptance clause.
func TestTheQuitRouteSignalsTheSameExit(t *testing.T) {
	ts, s := startServerWith(t)

	// STIMULUS: nothing is exiting before the request, or the assertion after it proves nothing.
	select {
	case <-s.IdleExit():
		t.Fatal("the server is exiting before Quit was called")
	default:
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/quit", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", ts.URL) // requirePublicLoopback's guard
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/quit: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("POST /api/quit returned %d, want 204", resp.StatusCode)
	}
	select {
	case <-s.IdleExit():
	case <-time.After(2 * time.Second):
		t.Error("Quit answered and the process was never asked to exit, so the user confirms and " +
			"Nib stays running — which is worse than no Quit at all")
	}
}

// TestTheArmedPushNamesTheCeremony — P01.S06 T03, the FIRST acceptance clause.
//
// "Names a live ceremony specifically, not generically" cannot be met from a bool, which is all
// P01.S05 pushed. The name is the ceremony's INTENT — the convener's own words for what this
// proceeding is — and never a fingerprint: the user is being asked whether to end something, and
// "a ceremony with a4f9c2…" is not a thing anyone can decide about.
func TestTheArmedPushNamesTheCeremony(t *testing.T) {
	s := &Server{}
	armed, what := s.sess.ArmedWhat()
	if armed || what != "" {
		t.Fatalf("an unarmed machine reports armed=%v what=%q", armed, what)
	}

	// A ceremony arm names its intent.
	cer := &ceremonyID{inv: ceremony.Invitation{ID: "abc", Intent: "We agree to co-sign the lease"}}
	if !s.sess.armIn(&arm{kind: armInteractive, addr: "127.0.0.1:0", cer: cer}) {
		t.Fatal("setup: could not arm")
	}
	armed, what = s.sess.ArmedWhat()
	if !armed {
		t.Fatal("armed a ceremony and ArmedWhat reports unarmed")
	}
	if what != "We agree to co-sign the lease" {
		t.Errorf("the armed ceremony is named %q, want its intent. A modal that says only "+
			"\"a ceremony is running\" asks the user to decide about a generality", what)
	}
	if strings.Contains(what, cer.peer) && cer.peer != "" {
		t.Error("the name carries a fingerprint. The panel's standing rule is that a person is " +
			"named, never fingerprinted, and a proceeding is no different")
	}

	// A MANUAL arm has no ceremony and therefore nothing to name — empty is correct rather than
	// missing, and the client says so in its own words instead of printing an empty string.
	s.sess.disarm()
	if !s.sess.armIn(&arm{kind: armInteractive, addr: "127.0.0.1:0"}) {
		t.Fatal("setup: could not arm the manual case")
	}
	armed, what = s.sess.ArmedWhat()
	if !armed || what != "" {
		t.Errorf("a manual co-signing arm reports armed=%v what=%q, want true and empty — it has "+
			"no ceremony and no intent, so there is nothing to name", armed, what)
	}
}
