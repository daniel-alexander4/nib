package server

import (
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// acceptAt drives sessionAccepter.Accept to completion with the user saying yes, and returns
// what the p2p layer would see. HOME is already redirected by the caller.
func acceptAt(t *testing.T, s *Server, doc []byte) (bool, error) {
	t.Helper()
	ln := &stubListener{}
	if !s.sess.arm(ln, nil) {
		t.Fatal("setup: the session refused to arm, so setPending below cannot succeed")
	}
	t.Cleanup(s.sess.disarm)

	var saw reached
	sa := sessionAccepter{s: s, label: "Bob", saw: &saw, anchor: consentAnchor{ln: ln}}

	// The user clicks accept as soon as the request is parked. Concurrent because Accept blocks.
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if s.sess.pendingPDF() != nil {
				s.sess.respond(sessionDecision{accept: true})
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	return sa.Accept([]byte("peer-fingerprint-bytes-0123456789"), doc)
}

// TestAFailedReceivedWriteIsNotAnAcknowledgement — C10's core, driven at the door.
//
// Until P08.S05a the durable write ran AFTER `ReceiveDocument` had already sent `ackOK`, from
// `serveOneSession`, returning nothing. So a party whose disk failed was recorded as delivered,
// never retried and never told — and `saveReceived`'s own comment said the sender "will not send
// it again". The write is now the last thing `Accept` does, so the receipt attests to it.
//
// The failure is injected by making `~/nib` a FILE, which makes `MkdirAll` refuse deterministically
// on every platform and as any user — a chmod-based fixture is a no-op for root and on Windows.
func TestAFailedReceivedWriteIsNotAnAcknowledgement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	doc := []byte("%PDF-1.4\nnot going anywhere")

	// SETUP: the same call SUCCEEDS when the directory is writable. Without this the assertion
	// below passes against an Accept that refuses everything, which is the vacuous green.
	okServer := &Server{}
	accepted, err := acceptAt(t, okServer, doc)
	if !accepted || err != nil {
		t.Fatalf("setup: a writable ~/nib produced (%v, %v); want (true, nil) — the failure "+
			"arm below cannot be distinguished from a door that never works", accepted, err)
	}
	if okServer.sess.status().Received == nil {
		t.Fatal("setup: a successful save recorded no `received`, so the negative arm below " +
			"cannot show that the failure is what suppressed it")
	}
	// And it really is on disk, or "persisted before the ack" names a write nobody made.
	var found string
	_ = filepath.Walk(filepath.Join(home, "nib"), func(p string, fi os.FileInfo, e error) error {
		if e == nil && fi != nil && !fi.IsDir() {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatal("setup: the accepted document is not on disk anywhere under ~/nib")
	}

	// Now break it: ~/nib becomes a regular file, so MkdirAll cannot create the subdirectory.
	if err := os.RemoveAll(filepath.Join(home, "nib")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "nib"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	badServer := &Server{}
	accepted, err = acceptAt(t, badServer, doc)
	if accepted {
		t.Error("Accept reported the document accepted while its durable write had failed. " +
			"ReceiveDocument writes ackOK on that basis, so the sender is told the document " +
			"arrived and will not send it again — which is the exact loss C10 names.")
	}
	if !errors.Is(err, p2p.ErrNotStored) {
		t.Errorf("Accept returned %v; want p2p.ErrNotStored. Any other error either reaches the "+
			"wire as ackDeclined — a false statement about the user, who said yes — or maps to "+
			"no frame at all, which the sender reads as EOF.", err)
	}
	if errors.Is(err, p2p.ErrDeclined) {
		t.Error("a failed write was reported as ErrDeclined; the sender would tell its user " +
			"that this person refused the document")
	}
	// The user on THIS machine is told too — the arm is a background goroutine with no response
	// to write into, which is what P08.S08's sticky notice exists for.
	st := badServer.sess.status()
	if st.Notice == nil || st.Notice.What != "received-not-saved" {
		t.Errorf("the failed write left notice %+v; want a sticky 'received-not-saved'", st.Notice)
	}
	if st.Received != nil {
		t.Errorf("a failed write recorded `received` = %+v; the panel would tell the user the "+
			"document was saved and name a path that does not exist", st.Received)
	}
}

// TestTheReceivedWriteHasOneDoor is the ADR-009 half: the rule above is only worth what the
// number of places that can bypass it says.
//
// It asserts ROUTING rather than comparing the text of each site, which is ADR-009's own wording:
// the defect this prevents is a NEW call site added without the ordering, and a test that checked
// the two known sites for agreement would say nothing about a third. `serveOneSession` is named
// explicitly because it is where the call used to be.
func TestTheReceivedWriteHasOneDoor(t *testing.T) {
	// **The WHOLE package, not session.go.** The first cut parsed one file, so a new caller in
	// `lan.go` or `saveas.go` would have passed clean under a message asserting "the write has one
	// door" — a guard narrower than its own claim, which is the shape it exists to prevent.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["server"]
	if !ok {
		t.Fatal("setup: package `server` did not parse, so this guard walked nothing")
	}

	callers := map[string]int{}
	// A method VALUE (`f := s.saveReceived`) is a second way to reach it that no call-site count
	// can see, so it is counted separately and forbidden outright.
	methodValues := 0
	for name, f := range pkg.Files {
		var fn string
		ast.Inspect(f, func(n ast.Node) bool {
			if d, ok := n.(*ast.FuncDecl); ok {
				fn = d.Name.Name
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "saveReceived" {
				// Counted here, then discounted below if it is the callee of a CallExpr.
				methodValues++
				_ = name
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "saveReceived" {
				return true
			}
			callers[fn]++
			methodValues--
			return true
		})
	}
	if methodValues != 0 {
		t.Errorf("saveReceived is referenced %d time(s) other than as a direct call — a method "+
			"value escapes every call-site count, and its ordering against the acknowledgement "+
			"is unasserted", methodValues)
	}

	// STIMULUS: the thing being policed exists. A rename would leave the map empty and this
	// guard would report a clean bill on code it never found.
	if len(callers) == 0 {
		t.Fatal("setup: nothing in package server calls saveReceived — either it is gone or it has " +
			"been renamed, and this guard walked over the rule rather than checking it")
	}

	if n := callers["Accept"]; n != 1 {
		t.Errorf("sessionAccepter.Accept calls saveReceived %d time(s), want exactly 1. The "+
			"durable write must happen there and only there, because that is the last thing "+
			"before ReceiveDocument writes ackOK — a write anywhere else is a write the "+
			"sender's receipt does not attest to.", n)
	}
	if n := callers["serveOneSession"]; n != 0 {
		t.Errorf("serveOneSession calls saveReceived %d time(s). That is where it used to be, "+
			"and it runs AFTER the acknowledgement has gone out — which is the defect C10 "+
			"names: a party whose disk fails is recorded as delivered and never told.", n)
	}
	for name, n := range callers {
		if name != "Accept" {
			t.Errorf("%s calls saveReceived %d time(s); the write has one door and this is a "+
				"second one, so its ordering against the acknowledgement is unasserted", name, n)
		}
	}
}

// notStoringAccepter is the receiving gate for a peer whose disk fails: the human says yes and
// the write does not land. It is the far side of `sessionAccepter.Accept`'s new error return.
type notStoringAccepter struct{}

func (notStoringAccepter) Accept([]byte, []byte) (bool, error) { return false, p2p.ErrNotStored }

// TestSendReportsNotStoredAsItsOwnOutcome — the HTTP half, which nothing covered.
//
// The wire gained `ackNotStored` so a disk failure would stop being reported as the receiving user
// DECLINING. Falling through to `httpError` at the route would report it as *"could not send"* —
// the sentence a dead peer produces — which is the same false statement pointing the other way:
// the transport worked, the peer is fine, a human said yes, and the action this calls for (ask
// them to arm again and resend) is not the action an unreachable peer calls for.
//
// Modelled on TestSessionSend, with the receiving gate swapped for one whose write fails.
func TestSendReportsNotStoredAsItsOwnOutcome(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	aFPBytes, _ := hex.DecodeString(me.Fingerprint)

	bCert, bKey, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	bFPBytes, _ := sign.Fingerprint(bCert)
	bFP := hex.EncodeToString(bFPBytes)
	pinPeer(t, c, csrf, ts.URL, bFP)

	ln, err := p2p.Listen("127.0.0.1:0", bCert, bKey, aFPBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	recvErr := make(chan error, 1)
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			recvErr <- e
			return
		}
		defer conn.Close()
		_, e = p2p.ReceiveDocument(conn.Channel, notStoringAccepter{}, bFPBytes, okVerifier{})
		recvErr <- e
	}()

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	body, ct := sendForm(t, base, bFP, ln.Addr().String())
	go confirmVerificationVia(t, c, csrf, ts.URL)

	var res sendResult
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/send", ct, body), &res)

	if res.Sent {
		t.Error("the route reported the transfer sent while the receiver could not store it")
	}
	if !res.NotStored {
		t.Errorf("the route reported %+v; want notStored. Without its own field this outcome "+
			"reaches the browser as a 502 toasted 'could not send: …' — the sentence a DEAD PEER "+
			"produces — so the user is told the transport failed when a person accepted and a "+
			"disk did not.", res)
	}
	if res.Declined {
		t.Error("a failed write was reported as declined; that is the very collapse ackNotStored " +
			"was added to undo")
	}
	if e := <-recvErr; !errors.Is(e, p2p.ErrNotStored) {
		t.Errorf("the receiving side saw %v; want p2p.ErrNotStored", e)
	}
}
