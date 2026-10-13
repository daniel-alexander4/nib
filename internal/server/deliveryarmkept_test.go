package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// TestAFailedTamperCheckOnADeliveryLegIsSaidAndTheArmGoesOnWaiting — /pending 766, the delivery arm.
//
// The delivery arm is spent by a delivered leg and by nothing else (/pending 500), so a leg whose
// commit-reveal check failed went back round a bare `continue`: the arm kept waiting, which is
// right, and told nobody, which is not — the convener sees a closed connection and this machine was
// the only one holding the evidence. The leg below is the real one, on a real pinned listener, with
// a dialler that reveals a value it did not commit to; what it returns is handed to the door the arm
// loop calls.
func TestAFailedTamperCheckOnADeliveryLegIsSaidAndTheArmGoesOnWaiting(t *testing.T) {
	aCert, aKey, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	bCert, bKey, err := sign.GenerateIdentity("Party")
	if err != nil {
		t.Fatal(err)
	}
	aFP, _ := sign.Fingerprint(aCert)
	bFP, _ := sign.Fingerprint(bCert)
	ln, err := p2p.Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	s := &Server{}
	cer := &ceremonyID{peer: hex.EncodeToString(aFP)}
	if !s.sess.armDeliveryForCeremony(cer, ln.Addr().String(), func() {}) {
		t.Fatal("setup: the delivery arm was refused on an empty session")
	}
	anchor := consentAnchor{cer: cer, kind: armDelivery}

	go func() {
		conn, derr := p2p.Dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if derr != nil {
			return
		}
		defer conn.Close()
		st := conn.Channel.Stream
		_ = st.SetDeadline(time.Now().Add(10 * time.Second))
		commit, reveal := make([]byte, 32), make([]byte, 32)
		_, _ = rand.Read(commit)
		_, _ = rand.Read(reveal)
		rawFrame(t, st, commit)
		_ = readRawFrame(t, st)
		rawFrame(t, st, reveal) // NOT what was committed to
		_, _ = io.Copy(io.Discard, st)
	}()
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("setup: accept: %v", err)
	}
	_, legErr := s.deliverOneLeg(conn.Channel, cer, bFP, nil, false)
	conn.Close()
	// STIMULUS: the leg failed, and failed the tamper check — not a dropped channel that happens to
	// look like one from here.
	if !p2p.IsCommitmentFailure(legErr) {
		t.Fatalf("setup: the delivery leg ended with %v, want the commit-reveal check failing — "+
			"nothing below would be about a tamper check", legErr)
	}
	if n := s.sess.status().Notice; n != nil {
		t.Fatalf("setup: the leg itself already left a notice (%+v); the door below would not be what said it", n)
	}

	kept := 0
	start := time.Now()
	if !s.deliveryArmKept(context.Background(), anchor, legErr, &kept, time.Now().Add(time.Hour)) {
		t.Fatal("a failed tamper check ENDED the delivery arm's loop — the finished document is " +
			"still owed to this machine and nothing would be listening for it (/pending 500)")
	}
	if el := time.Since(start); el < reraceBase || kept != 1 {
		t.Errorf("the arm came back round after %v with %d failure(s) counted, want at least %v and 1 "+
			"— a peer that fails the check sets the pace of the loop", el, kept, reraceBase)
	}
	n := s.sess.status().Notice
	if n == nil || n.What != "delivery-tamper-check" {
		t.Fatalf("a delivery leg failed its tamper check and the status notice is %+v, want "+
			"delivery-tamper-check — the one machine that saw it told nobody", n)
	}
	if !strings.Contains(n.Summary, "still waiting") || n.Detail == "" {
		t.Errorf("the notice does not say the arm is still waiting, or has no detail: %+v", n)
	}
	if strings.Contains(n.Summary+n.Detail, "stopped waiting") {
		t.Errorf("the notice says this computer stopped waiting, and it has not: %+v", n)
	}
	if !s.sess.slotTaken(armDelivery) {
		t.Error("the delivery slot is free after the notice; the arm must still be standing")
	}
}

// TestTheDeliveryArmPacesAFailureAndSaysOnlyATamperCheck — the door's other rows.
func TestTheDeliveryArmPacesAFailureAndSaysOnlyATamperCheck(t *testing.T) {
	armed := func() (*Server, consentAnchor) {
		s := &Server{}
		cer := &ceremonyID{}
		if !s.sess.armDeliveryForCeremony(cer, "127.0.0.1:9", func() {}) {
			t.Fatal("setup: the delivery arm was refused on an empty session")
		}
		return s, consentAnchor{cer: cer, kind: armDelivery}
	}
	far := func() time.Time { return time.Now().Add(time.Hour) }

	// A hop served cleanly on the way waits for nothing and counts nothing.
	s, anchor := armed()
	kept := 0
	start := time.Now()
	if !s.deliveryArmKept(context.Background(), anchor, nil, &kept, far()) || kept != 0 {
		t.Errorf("a connection that went well: kept=%d, want the arm to go on with nothing counted", kept)
	}
	if el := time.Since(start); el > reraceBase/2 {
		t.Errorf("a connection that went well was paced (%v); only a failure is", el)
	}

	// A lost channel, a refused role and an unanswered gate are paced and NOT said.
	for _, err := range []error{io.EOF, p2p.ErrRoleRefused, p2p.ErrConsentTimedOut} {
		start = time.Now()
		before := kept
		if !s.deliveryArmKept(context.Background(), anchor, err, &kept, far()) {
			t.Errorf("%v ended the delivery arm's loop", err)
		}
		if kept != before+1 || time.Since(start) < reraceBase {
			t.Errorf("%v: failures counted %d → %d after %v; a failure is counted once and waited for",
				err, before, kept, time.Since(start))
		}
		if n := s.sess.status().Notice; n != nil {
			t.Errorf("%v left a notice (%+v); only a failed tamper check is said by this door", err, n)
		}
	}

	// The window closed: the loop ends rather than sleeping past its own end.
	if s.deliveryArmKept(context.Background(), anchor, io.EOF, &kept, time.Now().Add(-time.Second)) {
		t.Error("a failure after the arm's window closed sent the loop round again")
	}
	// Disarmed while pacing: the wait is abandoned and the loop ends.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	high := 30 // deep in the backoff, so the wait is the two-second ceiling
	start = time.Now()
	if s.deliveryArmKept(ctx, anchor, io.EOF, &high, far()) {
		t.Error("a cancelled delivery arm went round its loop again")
	}
	if el := time.Since(start); el > reraceCap/2 {
		t.Errorf("a cancelled delivery arm took %v to stop pacing", el)
	}
}

// TestEveryWayRoundTheDeliveryArmLoopGoesThroughOneDoor — /pending 766, ADR-009 over the third arm.
//
// The delivery arm's loop had five ways back to `Accept` after a connection that did not deliver,
// every one a bare `continue` or a fall off the end. Each is now `deliveryArmKept`, which paces a
// failure and says a tamper check; this holds the routing, so a sixth way round cannot be added
// without it.
func TestEveryWayRoundTheDeliveryArmLoopGoesThroughOneDoor(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "delivery.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "armForDelivery" {
			fn = fd
		}
	}
	if fn == nil {
		t.Fatal("setup: armForDelivery not found in delivery.go — this guard walked nothing")
	}
	// isDoor: `if !s.deliveryArmKept(…) { return }`.
	isDoor := func(st ast.Stmt) bool {
		is, ok := st.(*ast.IfStmt)
		if !ok {
			return false
		}
		un, ok := is.Cond.(*ast.UnaryExpr)
		if !ok || un.Op != token.NOT {
			return false
		}
		call, ok := un.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "deliveryArmKept" || len(is.Body.List) != 1 {
			return false
		}
		_, ok = is.Body.List[0].(*ast.ReturnStmt)
		return ok
	}
	var loop *ast.ForStmt
	ast.Inspect(fn, func(n ast.Node) bool {
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond != nil || fs.Init != nil {
			return true
		}
		// The accept loop is the bare `for {` whose first statement accepts.
		if as, ok := fs.Body.List[0].(*ast.AssignStmt); ok && len(as.Rhs) == 1 {
			if call, ok := as.Rhs[0].(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Accept" {
					loop = fs
				}
			}
		}
		return true
	})
	if loop == nil {
		t.Fatal("setup: armForDelivery has no accept loop — nothing here could have failed")
	}
	continues := 0
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		blk, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, st := range blk.List {
			br, ok := st.(*ast.BranchStmt)
			if !ok || br.Tok != token.CONTINUE {
				continue
			}
			continues++
			if i == 0 || !isDoor(blk.List[i-1]) {
				t.Errorf("the delivery arm `continue`s at %s without asking deliveryArmKept first: a "+
					"peer that fails there is taken again as fast as it fails, and a failed tamper "+
					"check there is told to nobody", fset.Position(br.Pos()))
			}
		}
		return true
	})
	// STIMULUS: the unreadable role, the refused role, the unanswered accept and the served hop.
	if continues < 4 {
		t.Fatalf("found %d `continue` in the delivery arm's loop, want its four at least — the walk "+
			"is not reading the loop", continues)
	}
	if last := loop.Body.List[len(loop.Body.List)-1]; !isDoor(last) {
		t.Errorf("the delivery arm's loop does not END in deliveryArmKept (last statement at %s): a "+
			"leg that failed falls off the end and straight back into Accept",
			fset.Position(last.Pos()))
	}
}
