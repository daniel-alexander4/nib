package server

import (
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestACompletedHandshakeThatProducesNoSessionLeavesTheArmOpen — P05.S01.
//
// The accept loop tolerated a FAILED handshake and then spent the arm on the first
// COMPLETED one, whatever came of it. So a pinned peer whose connection dropped before the
// exchange — a closed laptop, a dropped VPN, a racer that kept a different candidate —
// took the user's armed session with it, and the user had to notice and re-arm.
//
// `TestAStrayConnectionDoesNotConsumeTheSession` is the sibling and it stops one step
// short: its stray is plain TCP with junk bytes, so it never reaches the statement this
// test is about. `internal/p2p/transport.go` says so in as many words about its own
// version of the same gap — "only proves the session SURVIVES one stray connection".
//
// This drives the real thing: a genuinely pinned peer, a completed mTLS handshake, and
// then a close with no protocol at all.
func TestACompletedHandshakeThatProducesNoSessionLeavesTheArmOpen(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// Bob is this server; Alice is the peer it will accept.
	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	bFPBytes, err := hex.DecodeString(me.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	aFPBytes, _ := sign.Fingerprint(aCert)
	aFP := hex.EncodeToString(aFPBytes)
	pinPeer(t, c, csrf, ts.URL, aFP)

	var armed sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0"})), &armed)
	if !armed.Armed || armed.Address == "" {
		t.Fatalf("setup: arm failed: %+v", armed)
	}

	// The connection this test exists for: a real pinned handshake, then nothing.
	conn, err := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
	if err != nil {
		t.Fatalf("setup: the pinned peer could not complete a handshake against the armed "+
			"listener (%s): %v — with no completed handshake there is nothing here to "+
			"consume the arm, and the assertion below would pass for the wrong reason",
			armed.Address, err)
	}
	// STIMULUS: the handshake really completed and really was mutually verified. A
	// `Dial` that returned a connection whose peer was unverified would make this a
	// second copy of the stray-connection test.
	if len(conn.Channel.PeerFP) == 0 {
		conn.Close()
		t.Fatal("setup: the dial returned a channel with no verified peer fingerprint, so " +
			"this is not the completed-handshake case")
	}
	conn.Close()

	// The session must still be armed, polled for the same reason the sibling polls:
	// the accept loop runs asynchronously, so reading once could observe the state
	// before the loop has even returned from Accept.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if !st.Armed {
			t.Fatal("a pinned peer that completed a handshake and then closed without " +
				"producing a session consumed the arm — the user's receive is gone and the " +
				"peer they are waiting for can no longer reach them, for a connection that " +
				"exchanged nothing")
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	// **And the arm is LIVE, not a stale flag.** This is the half that distinguishes
	// "still armed" from "still armed and still usable": a second pinned peer completes a
	// handshake against the same listener after the first one abandoned it. Without this,
	// a fix that left the flag set while the listener was closed would pass everything
	// above.
	second, err := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
	if err != nil {
		t.Fatalf("the listener reports armed but no longer accepts the pinned peer: %v — "+
			"the arm survived as a flag and not as a session", err)
	}
	second.Close()

	write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/disarm", "application/json", nil).Body.Close()
	var after sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &after)
	if after.Armed {
		t.Error("the session could not be disarmed afterwards")
	}
}

// TestTheArmWindowIsNotExtendedByConnectionsThatProduceNoSession — P05.S01's T02.
//
// The fix re-arms the accept timer after a connection produces nothing, and the whole
// question is *for how long*. Resetting to a full `sessionAcceptTimeout` would extend the
// arm window every time anybody dialled — a window an attacker holds open for free,
// against a listener whose port is broadcast to the link every 500 ms.
//
// The property is exactly "**the deadline is fixed at arm time and nothing moves it**",
// which is "`armedUntil` is assigned once, outside the loop". That is what is asserted —
// the routing, not the text of any one line (ADR-009). A behavioural version would have to
// spend `sessionAcceptTimeout`, which is five minutes.
func TestTheArmWindowIsNotExtendedByConnectionsThatProduceNoSession(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "session.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "runSession" {
			fn = fd
		}
	}
	if fn == nil {
		t.Fatal("setup: runSession not found in session.go — this guard walked nothing")
	}

	// Every assignment to armedUntil, and whether it is inside a loop.
	var total, inLoop int
	var loopRanges []struct{ lo, hi token.Pos }
	ast.Inspect(fn, func(n ast.Node) bool {
		switch l := n.(type) {
		case *ast.ForStmt:
			loopRanges = append(loopRanges, struct{ lo, hi token.Pos }{l.Pos(), l.End()})
		case *ast.RangeStmt:
			loopRanges = append(loopRanges, struct{ lo, hi token.Pos }{l.Pos(), l.End()})
		}
		return true
	})
	ast.Inspect(fn, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || id.Name != "armedUntil" {
				continue
			}
			total++
			for _, r := range loopRanges {
				if as.Pos() > r.lo && as.End() < r.hi {
					inLoop++
				}
			}
		}
		return true
	})

	// STIMULUS: the thing being policed exists. A renamed variable would leave every
	// count at zero and this guard would report a clean bill on code it never found.
	if total == 0 {
		t.Fatal("setup: runSession assigns no `armedUntil` — the absolute arm deadline is " +
			"gone, so there is no remainder to compute and nothing here to police")
	}
	if len(loopRanges) == 0 {
		t.Fatal("setup: runSession contains no loop — the accept loop this guard is about " +
			"does not exist, so `inLoop` could not have been non-zero")
	}
	if total != 1 {
		t.Errorf("`armedUntil` is assigned %d times; the arm deadline must be fixed once, at "+
			"arm time", total)
	}
	if inLoop != 0 {
		t.Errorf("`armedUntil` is assigned inside the accept loop (%d time(s)), so every "+
			"connection that produces no session pushes the arm window out — which is a "+
			"window anybody who can reach the listener holds open for free", inLoop)
	}

	// **And what the timer is reset TO.** The check above polices `armedUntil` and a
	// review pointed out it cannot fail for the defect its own message describes:
	// `timer.Reset(sessionAcceptTimeout)` leaves `armedUntil` assigned exactly once,
	// outside the loop, and pushes the window out by a full period on every connection.
	// The guard could fail for a renamed variable and not for the behaviour.
	var resets int
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Reset" || len(call.Args) != 1 {
			return true
		}
		resets++
		arg := types.ExprString(call.Args[0])
		if !strings.Contains(arg, "remaining") {
			t.Errorf("the accept timer is reset to %q; it must be reset to the REMAINDER of "+
				"the window fixed at arm time, or every connection that produces no session "+
				"extends the arm", arg)
		}
		return true
	})
	if resets == 0 {
		t.Fatal("setup: runSession resets no timer, so the accept loop does not re-arm its " +
			"window at all and this half of the guard policed nothing")
	}
}

// TestADeclinedSpokenCheckSpendsTheArm — the man-in-the-middle case, and the one the
// first draft of P05.S01 got wrong.
//
// `p2p.ErrVerificationDeclined` means the user looked at the four words and said they did
// not match. `internal/p2p/verify.go` states what must never happen to it: *"it must never
// be reported as a network error — 'could not connect' invites a retry, which is the worst
// possible advice when someone is sitting between you."*
//
// S01's first draft enumerated the outcomes that spend the arm and this was not among
// them, so the listener re-armed and **performed that retry itself** — silently, with the
// status still reading armed, as many times as the attacker cared to dial. The rule is
// engagement now: a connection that put the spoken check on screen has spent the arm
// however it ended.
func TestADeclinedSpokenCheckSpendsTheArm(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	bFPBytes, _ := hex.DecodeString(me.Fingerprint)
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	aFPBytes, _ := sign.Fingerprint(aCert)
	aFP := hex.EncodeToString(aFPBytes)
	pinPeer(t, c, csrf, ts.URL, aFP)

	var armed sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0"})), &armed)
	if !armed.Armed {
		t.Fatalf("setup: arm failed: %+v", armed)
	}

	errc := make(chan error, 1)
	go func() {
		base, e := testpdf.Form()
		if e != nil {
			errc <- e
			return
		}
		prepared, e := p2p.PrepareDocument(base)
		if e != nil {
			errc <- e
			return
		}
		place, e := p2p.NextPlacement(prepared)
		if e != nil {
			errc <- e
			return
		}
		att := p2p.Attestation{Signer: "Alice", AcceptedPeer: me.Fingerprint, AcceptedPeerLabel: "Bob",
			Intent: "I agree", When: time.Now()}
		aSigned, e := p2p.Contribute(prepared, aCert, aKey, att, nil, place)
		if e != nil {
			errc <- e
			return
		}
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		_, e = p2p.Initiate(conn.Channel, aSigned, aFPBytes, okVerifier{})
		errc <- e // an error is expected: this side is declined
	}()

	// Wait for the spoken check to reach the screen, then say the words do NOT match.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("setup: the spoken check never appeared, so nothing was declined and " +
				"the assertion below would be about a connection that reached no user")
		}
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Verify != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	vr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/verify", "application/json",
		jsonBody(map[string]any{"confirmed": false}))
	if vr.StatusCode != http.StatusOK {
		t.Fatalf("setup: verify(false) status = %d", vr.StatusCode)
	}
	vr.Body.Close()

	// THE ASSERTION. A refused spoken check is the strongest outcome in the protocol.
	spent := time.Now().Add(5 * time.Second)
	for {
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if !st.Armed {
			break
		}
		if time.Now().After(spent) {
			t.Fatal("the listener is STILL ARMED after the user said the verification words " +
				"did not match — the man-in-the-middle signal was filed as a failed " +
				"connection, so the listener retries automatically and the attacker gets " +
				"another attempt with no user action and no warning")
		}
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case <-errc:
	case <-time.After(10 * time.Second):
		t.Fatal("the initiator never returned")
	}
}

// TestAnAbandonedConnectionIsFollowedByAWorkingSession — the clause P05.S01 exists for.
//
// Everything else in this file proves the listener *survives* an abandoned connection.
// None of it proves the next one can be **served**, and that is the whole point: P05.S02
// races several candidate addresses that all reach this one listener, keeps one, and drops
// the rest. If the loop survived the dropped connection but could not then complete a
// ceremony on the next, the slice would have moved the failure rather than fixed it.
//
// It is also the honest stimulus the other test lacks. `p2p.Dial` returns when the
// CLIENT's handshake completes, so a test that dials and closes cannot tell "the server
// accepted a pinned connection and moved past it" from "the server never accepted it at
// all" — and would pass with the accept path deleted. A completed session afterwards can
// only happen if the server really did process the abandoned one and carry on.
func TestAnAbandonedConnectionIsFollowedByAWorkingSession(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	bFPBytes, _ := hex.DecodeString(me.Fingerprint)
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	aFPBytes, _ := sign.Fingerprint(aCert)
	aFP := hex.EncodeToString(aFPBytes)
	pinPeer(t, c, csrf, ts.URL, aFP)

	var armed sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0"})), &armed)
	if !armed.Armed {
		t.Fatalf("setup: arm failed: %+v", armed)
	}

	// THE ABANDONED CONNECTION — what a racer leaves behind.
	abandoned, err := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
	if err != nil {
		t.Fatalf("setup: the abandoned dial did not complete a handshake: %v", err)
	}
	abandoned.Close()

	// Now a real ceremony on a second connection.
	result := make(chan []byte, 1)
	errc := make(chan error, 1)
	go func() {
		base, e := testpdf.Form()
		if e != nil {
			errc <- e
			return
		}
		prepared, e := p2p.PrepareDocument(base)
		if e != nil {
			errc <- e
			return
		}
		place, e := p2p.NextPlacement(prepared)
		if e != nil {
			errc <- e
			return
		}
		att := p2p.Attestation{Signer: "Alice", AcceptedPeer: me.Fingerprint, AcceptedPeerLabel: "Bob",
			Intent: "I agree", When: time.Now()}
		aSigned, e := p2p.Contribute(prepared, aCert, aKey, att, nil, place)
		if e != nil {
			errc <- e
			return
		}
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		final, e := p2p.Initiate(conn.Channel, aSigned, aFPBytes, okVerifier{})
		if e != nil {
			errc <- e
			return
		}
		result <- final
	}()

	// Confirm the spoken check, then accept.
	for _, step := range []struct {
		field string
		route string
		body  map[string]any
	}{
		{"Verify", "/api/session/verify", map[string]any{"confirmed": true}},
		{"Pending", "/api/session/respond", map[string]any{"accept": true, "intent": "I accept"}},
	} {
		deadline := time.Now().Add(15 * time.Second)
		for {
			if time.Now().After(deadline) {
				t.Fatalf("the %s gate never appeared on the connection AFTER the abandoned "+
					"one — the accept loop survived the abandonment but could not serve the "+
					"next peer, which is the case this slice exists for", step.field)
			}
			select {
			case e := <-errc:
				t.Fatalf("initiator: %v", e)
			default:
			}
			var st sessionStatus
			sessGet(t, c, ts.URL+"/api/session/status", &st)
			if (step.field == "Verify" && st.Verify != nil) || (step.field == "Pending" && st.Pending != nil) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		r := write(t, c, csrf, http.MethodPost, ts.URL+step.route, "application/json", jsonBody(step.body))
		if r.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", step.route, r.StatusCode)
		}
		r.Body.Close()
	}

	select {
	case e := <-errc:
		t.Fatalf("initiator: %v", e)
	case final := <-result:
		if n := len(p2p.ReadAttestations(final)); n != 2 {
			t.Fatalf("the session after the abandoned connection produced %d signers, want 2", n)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the session after the abandoned connection never finished")
	}

	// And THAT session spent the arm.
	var st sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &st)
	if st.Armed {
		t.Error("still armed after a completed session that followed an abandoned connection")
	}
}
