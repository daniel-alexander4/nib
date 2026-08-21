package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

func sessGet(t *testing.T, c *http.Client, url string, v any) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func sessDecode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

// TestSessionArmReceiveSign drives a full receive session end to end: this server
// (Bob) arms for a pinned peer (Alice), Alice dials in and sends a document she
// signed, the user accepts via /api/session/respond, and the result — doubly
// signed — becomes Bob's open document.
func TestSessionArmReceiveSign(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// Bob's own fingerprint (the receiver's vault identity).
	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	bFP := me.Fingerprint
	bFPBytes, err := hex.DecodeString(bFP)
	if err != nil {
		t.Fatal(err)
	}

	// Alice: a remote initiator Bob has pinned.
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	aFPBytes, _ := sign.Fingerprint(aCert)
	aFP := hex.EncodeToString(aFPBytes)
	pinPeer(t, c, csrf, ts.URL, aFP)

	// Arm to receive from Alice on a loopback "routable" bind.
	var armed sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0"})), &armed)
	if !armed.Armed || armed.Address == "" {
		t.Fatalf("arm failed: %+v", armed)
	}

	// Alice prepares + signs a doc accepting Bob, dials in, and waits for Bob to sign.
	result := make(chan []byte, 1)
	errc := make(chan error, 1)
	initiatorVerifier := &recordingVerifier{}
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
		att := p2p.Attestation{Signer: "Alice", AcceptedPeer: bFP, AcceptedPeerLabel: "Bob", Intent: "I agree", When: time.Now()}
		aSigned, e := p2p.Contribute(prepared, aCert, aKey, att, nil, place)
		if e != nil {
			errc <- e
			return
		}
		conn, e := p2p.Dial(context.Background(), armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		final, e := p2p.Initiate(conn.Channel, aSigned, aFPBytes, initiatorVerifier)
		if e != nil {
			errc <- e
			return
		}
		result <- final
	}()

	// Bob's UI: the SPOKEN CHECK comes first, before any document byte (L2). The status
	// poller surfaces the four words; the user confirms them on a separate request, which
	// is why the blocking session goroutine can wait on it.
	vdeadline := time.Now().Add(10 * time.Second)
	var serverWords string
	for {
		if time.Now().After(vdeadline) {
			t.Fatal("no verification appeared — the session reached the document exchange " +
				"without asking anyone to confirm who they were talking to (L2)")
		}
		select {
		case e := <-errc:
			t.Fatalf("initiator: %v", e)
		default:
		}
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Verify != nil {
			serverWords = st.Verify.Words
			break
		}
		if st.Pending != nil {
			t.Fatal("a document arrived for consent before the verification was confirmed — " +
				"document bytes crossed the wire unverified")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n := len(strings.Fields(serverWords)); n != 4 {
		t.Fatalf("the verification words are %q, which is not four words", serverWords)
	}
	vr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/verify", "application/json",
		jsonBody(map[string]any{"confirmed": true}))
	if vr.StatusCode != http.StatusOK {
		t.Fatalf("verify status = %d", vr.StatusCode)
	}
	vr.Body.Close()

	// Bob's UI: poll status until the request is pending, then accept it.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no pending request appeared")
		}
		select {
		case e := <-errc:
			t.Fatalf("initiator: %v", e)
		default:
		}
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Pending != nil {
			if st.Pending.Fingerprint != aFP {
				t.Errorf("pending peer = %s, want %s", st.Pending.Fingerprint, aFP)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	rr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/respond", "application/json",
		jsonBody(map[string]any{"accept": true, "intent": "I accept"}))
	if rr.StatusCode != http.StatusOK {
		t.Fatalf("respond status = %d", rr.StatusCode)
	}
	rr.Body.Close()

	// Both sides derived the same words, driven through the real server rather than
	// through two halves of one function. This is the acceptance clause that the spoken
	// check is worth speaking: if the two screens disagreed, honest users would read a
	// mismatch as an attack.
	if got := initiatorVerifier.seen(); got != serverWords {
		t.Errorf("the initiator was shown %q and the receiver %q — the two ends of one "+
			"session derived different words", got, serverWords)
	}

	// The initiator receives a doubly-signed result.
	select {
	case e := <-errc:
		t.Fatalf("initiator: %v", e)
	case final := <-result:
		if n := len(p2p.ReadAttestations(final)); n != 2 {
			t.Fatalf("initiator result has %d signers, want 2", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("initiator did not finish")
	}

	// Bob's open document is the doubly-signed result.
	pr := write(t, c, csrf, http.MethodGet, ts.URL+"/api/pdf", "", nil)
	pdf, _ := io.ReadAll(pr.Body)
	pr.Body.Close()
	if n := len(p2p.ReadAttestations(pdf)); n != 2 {
		t.Errorf("open document has %d signers, want 2", n)
	}

	// **A completed session SPENDS the arm** — D22's one-session-per-arm, and the
	// counter-assertion to P05.S01.
	//
	// S01 made the accept loop keep accepting when a connection produces no session, so
	// what now ends the arm is the *outcome*, not the connection. Nothing asserted that
	// end. A `serveOneSession` that always reported "not served" would leave this
	// listener armed forever after a finished ceremony and the whole suite would stay
	// green — the rule needs both arms or it is half a rule.
	//
	// Polled, because the accept goroutine disarms after `serveOneSession` returns and
	// the initiator's result can arrive first.
	spent := time.Now().Add(5 * time.Second)
	for {
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if !st.Armed {
			break
		}
		if time.Now().After(spent) {
			t.Fatal("the listener is still armed after a completed co-signing session — " +
				"one arm has served more than one session, which is the containment the " +
				"session TRIPWIRE names")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Disarm leaves nothing listening.
	var off sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/disarm", "application/json", nil), &off)
	if off.Armed {
		t.Error("still armed after disarm")
	}
}

// TestSessionDeclineLeavesOpenDoc proves a received request never replaces the open
// document: while it's parked, the received doc is served only from
// /api/session/pending-pdf and the open doc is untouched, and a decline leaves the
// open doc exactly as it was (the open-doc-clobber fix).
func TestSessionDeclineLeavesOpenDoc(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	// Bob has a document open (the test PDF, unsigned — zero attestations).
	openByPath(t, ts.URL, c, csrf, path)
	if n := attCount(t, c, ts.URL+"/api/pdf"); n != 0 {
		t.Fatalf("open document has %d signers before session, want 0", n)
	}

	// Bob's own fingerprint, and a pinned remote initiator (Alice).
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
	if !armed.Armed {
		t.Fatalf("arm failed: %+v", armed)
	}

	// Alice signs a doc accepting Bob and dials in; the round-trip will be declined.
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
		att := p2p.Attestation{Signer: "Alice", AcceptedPeer: me.Fingerprint, AcceptedPeerLabel: "Bob", Intent: "I agree", When: time.Now()}
		aSigned, e := p2p.Contribute(prepared, aCert, aKey, att, nil, place)
		if e != nil {
			errc <- e
			return
		}
		conn, e := p2p.Dial(context.Background(), armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		if _, e := p2p.Initiate(conn.Channel, aSigned, aFPBytes, okVerifier{}); e == nil {
			errc <- nil // a declined round-trip must surface an error to the initiator
			return
		}
		errc <- nil
	}()

	// Wait for the request to park.
	// waitPending walks the spoken check on the way — since P01.S05 no consent request can
	// exist until it has been confirmed, so a loop that polled only for Pending would time
	// out saying nothing about why.
	waitPending(t, c, ts.URL, aFP)

	// While parked: the received doc is served from pending-pdf (Alice's one
	// signature), and the open doc is still Bob's untouched, unsigned PDF.
	if n := attCount(t, c, ts.URL+"/api/session/pending-pdf"); n != 1 {
		t.Errorf("pending-pdf has %d signers, want 1", n)
	}
	if n := attCount(t, c, ts.URL+"/api/pdf"); n != 0 {
		t.Errorf("open document changed while a request was pending: %d signers, want 0", n)
	}

	// Bob declines.
	rr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/respond", "application/json",
		jsonBody(map[string]any{"accept": false}))
	if rr.StatusCode != http.StatusOK {
		t.Fatalf("respond status = %d", rr.StatusCode)
	}
	rr.Body.Close()
	if e := <-errc; e != nil {
		t.Fatalf("initiator: %v", e)
	}

	// After decline: the open doc is unchanged, and nothing is pending.
	if n := attCount(t, c, ts.URL+"/api/pdf"); n != 0 {
		t.Errorf("open document changed after decline: %d signers, want 0", n)
	}
	pr := write(t, c, csrf, http.MethodGet, ts.URL+"/api/session/pending-pdf", "", nil)
	pr.Body.Close()
	if pr.StatusCode != http.StatusNotFound {
		t.Errorf("pending-pdf after decline = %d, want 404", pr.StatusCode)
	}

	// **A DECLINE spends the arm too** — the third arm of P05.S01's rule, and the one
	// that would otherwise be an asymmetric pair. S01 keeps the listener armed when a
	// connection produces no session; a decline is not "no session", it is the user's
	// decision, and leaving the arm open after one lets the peer re-dial and ask the
	// same person again. `serveOneSession` distinguishes them with `errors.Is` against
	// `p2p.ErrCoSignDeclined`, and nothing asserted the distinction in either direction.
	spent := time.Now().Add(5 * time.Second)
	for {
		var ds sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &ds)
		if !ds.Armed {
			break
		}
		if time.Now().After(spent) {
			t.Fatal("the listener is still armed after the user declined — the peer can " +
				"re-dial and ask again, and the decline was treated as a failed connection")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestSessionQuoteForPendingPeer checks the responder's appearance quote: with a
// request parked, /api/session/quote returns this user's own visible-block lines
// accepting the connected peer and carrying the intent the user typed, with a
// non-degenerate rect — and with nothing pending it reports a conflict. Unlike
// /api/cosign/quote it must not depend on an open document (there is none here).
func TestSessionQuoteForPendingPeer(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// Nothing pending (and no document open) -> conflict, not a crash.
	nq := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/quote", "application/json",
		jsonBody(map[string]any{"intent": "x"}))
	if nq.StatusCode != http.StatusConflict {
		t.Errorf("quote with nothing pending = %d, want 409", nq.StatusCode)
	}
	nq.Body.Close()

	// Park a request from a pinned peer (Alice dials Bob).
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
		t.Fatalf("arm failed: %+v", armed)
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
		att := p2p.Attestation{Signer: "Alice", AcceptedPeer: me.Fingerprint, AcceptedPeerLabel: "Bob", Intent: "I agree", When: time.Now()}
		aSigned, e := p2p.Contribute(prepared, aCert, aKey, att, nil, place)
		if e != nil {
			errc <- e
			return
		}
		conn, e := p2p.Dial(context.Background(), armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		_, _ = p2p.Initiate(conn.Channel, aSigned, aFPBytes, okVerifier{}) // declined below; an error is expected
		errc <- nil
	}()

	// waitPending walks the spoken check on the way — since P01.S05 no consent request can
	// exist until it has been confirmed, so a loop that polled only for Pending would time
	// out saying nothing about why.
	waitPending(t, c, ts.URL, aFP)

	var q cosignQuote
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/quote", "application/json",
		jsonBody(map[string]any{"intent": "I consent here"})), &q)
	joined := strings.Join(q.Lines, "\n")
	if !strings.Contains(joined, "Nib co-signing attestation") {
		t.Errorf("quote lines missing header: %q", q.Lines)
	}
	if !strings.Contains(joined, "I consent here") {
		t.Errorf("quote lines missing the typed intent: %q", q.Lines)
	}
	if q.Rect[2]-q.Rect[0] <= 0 || q.Rect[3]-q.Rect[1] <= 0 {
		t.Errorf("quote rect has no area: %v", q.Rect)
	}

	// Clean up: decline, and let the initiator goroutine finish.
	write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/respond", "application/json",
		jsonBody(map[string]any{"accept": false})).Body.Close()
	<-errc
}

// autoAccept is the peer's consent gate for a one-way transfer in tests: always accept.
type autoAccept struct{}

func (autoAccept) Accept([]byte, []byte) (bool, error) { return true, nil }

// sendForm builds the multipart body /api/session/send expects.
func sendForm(t *testing.T, pdf []byte, fp, address string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pf, _ := mw.CreateFormFile("pdf", "doc.pdf")
	pf.Write(pdf)
	mw.WriteField("fingerprint", fp)
	mw.WriteField("address", address)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// TestSessionReceiveTransfer drives the receive side of a one-way transfer: this
// server (Bob) arms in receive mode for a pinned peer (Alice), Alice dials in and
// sends a flagged document, the user accepts, and the document is saved under
// ~/nib/to-sign/ (routed by its embedded flags) with its bytes intact.
func TestSessionReceiveTransfer(t *testing.T) {
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
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0", Mode: sessionModeReceive})), &armed)
	if !armed.Armed {
		t.Fatalf("arm failed: %+v", armed)
	}

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	flagged, err := pdfops.SetFlags(base, []byte(`[{"page":1,"frac":[0.1,0.1,0.3,0.15],"type":"sign"}]`))
	if err != nil {
		t.Fatal(err)
	}

	errc := make(chan error, 1)
	go func() {
		conn, e := p2p.Dial(context.Background(), armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		errc <- p2p.SendDocument(conn.Channel, flagged, aFPBytes, okVerifier{})
	}()

	waitPending(t, c, ts.URL, aFP)
	rr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/respond", "application/json",
		jsonBody(map[string]any{"accept": true}))
	if rr.StatusCode != http.StatusOK {
		t.Fatalf("respond = %d", rr.StatusCode)
	}
	rr.Body.Close()
	if e := <-errc; e != nil {
		t.Fatalf("sender: %v", e)
	}

	// The save completes just after the sender's ack; poll status for the result.
	var done sessionStatus
	deadline := time.Now().Add(5 * time.Second)
	for done.Received == nil {
		if time.Now().After(deadline) {
			t.Fatal("no received info after accept")
		}
		time.Sleep(50 * time.Millisecond)
		sessGet(t, c, ts.URL+"/api/session/status", &done)
	}
	if got := filepath.Base(filepath.Dir(done.Received.Path)); got != "to-sign" {
		t.Errorf("saved under %q dir, want to-sign", got)
	}
	saved, err := os.ReadFile(done.Received.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, flagged) {
		t.Error("saved bytes differ from what was sent")
	}
}

// TestSessionReceiveTransferDecline proves a declined transfer saves nothing and tells
// the sender it was declined.
func TestSessionReceiveTransferDecline(t *testing.T) {
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
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0", Mode: sessionModeReceive})), &armed)
	if !armed.Armed {
		t.Fatalf("arm failed: %+v", armed)
	}

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() {
		conn, e := p2p.Dial(context.Background(), armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		errc <- p2p.SendDocument(conn.Channel, base, aFPBytes, okVerifier{})
	}()

	waitPending(t, c, ts.URL, aFP)
	write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/respond", "application/json",
		jsonBody(map[string]any{"accept": false})).Body.Close()

	if e := <-errc; !errors.Is(e, p2p.ErrDeclined) {
		t.Fatalf("sender got %v, want ErrDeclined", e)
	}
	var st sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &st)
	if st.Received != nil {
		t.Errorf("a declined transfer reported a saved file: %+v", st.Received)
	}

	// **A DECLINE spends the arm too** — the third arm of P05.S01's rule, and the one
	// that would otherwise be an asymmetric pair. S01 keeps the listener armed when a
	// connection produces no session; a decline is not "no session", it is the user's
	// decision, and leaving the arm open after one lets the peer re-dial and ask the
	// same person again. `serveOneSession` distinguishes them with `errors.Is` against
	// `p2p.ErrDeclined`, and nothing asserted the distinction in either direction.
	spent := time.Now().Add(5 * time.Second)
	for {
		var ds sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &ds)
		if !ds.Armed {
			break
		}
		if time.Now().After(spent) {
			t.Fatal("the listener is still armed after the user declined — the peer can " +
				"re-dial and ask again, and the decline was treated as a failed connection")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestSessionSend drives the dialing side of a one-way transfer: this server (Alice)
// posts a document to /api/session/send and a pinned peer (Bob) receives the exact
// bytes over a receive listener.
func TestSessionSend(t *testing.T) {
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

	gotc := make(chan []byte, 1)
	errc := make(chan error, 1)
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		doc, e := p2p.ReceiveDocument(conn.Channel, autoAccept{}, bFPBytes, okVerifier{})
		if e != nil {
			errc <- e
			return
		}
		gotc <- doc
	}()

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	body, ct := sendForm(t, base, bFP, ln.Addr().String())

	// The SENDING side confirms the spoken check too, and its handler blocks while it
	// waits — so the confirmation has to arrive on a different request, concurrently. That
	// is not a test artefact: it is exactly the shape the UI needs, because the user who
	// clicked Send has a request in flight and can only answer on another one.
	go confirmVerificationVia(t, c, csrf, ts.URL)

	var res sendResult
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/send", ct, body), &res)
	if !res.Sent {
		t.Fatalf("send not confirmed: %+v", res)
	}

	select {
	case e := <-errc:
		t.Fatalf("receiver: %v", e)
	case got := <-gotc:
		if !bytes.Equal(got, base) {
			t.Error("received bytes differ from what was sent")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("receiver did not finish")
	}
}

// waitPending walks the session's gates until the consent request appears.
//
// **It confirms the spoken check on the way**, because since P01.S05 no document byte
// crosses before that gate — so a helper that polled only for `Pending` would wait ten
// seconds and report "no pending request appeared", which is true and says nothing about
// why. Four tests failed exactly that way when the gate landed. The verification is a
// precondition of a consent request existing at all, so it belongs in the same wait.
func waitPending(t *testing.T, c *http.Client, baseURL, fp string) {
	t.Helper()
	csrf := csrfFor(t, c, baseURL)
	deadline := time.Now().Add(10 * time.Second)
	confirmed := false
	for {
		if time.Now().After(deadline) {
			if !confirmed {
				t.Fatal("no verification and no pending request appeared — the session never " +
					"reached the spoken check")
			}
			t.Fatal("the verification was confirmed but no pending request followed")
		}
		var st sessionStatus
		sessGet(t, c, baseURL+"/api/session/status", &st)
		if st.Verify != nil && !confirmed {
			r := write(t, c, csrf, http.MethodPost, baseURL+"/api/session/verify", "application/json",
				jsonBody(map[string]any{"confirmed": true}))
			r.Body.Close()
			confirmed = true
			continue
		}
		if st.Pending != nil {
			if !confirmed {
				t.Fatal("a document arrived for consent without the spoken check ever being " +
					"shown — document bytes crossed the wire unverified (L2)")
			}
			if st.Pending.Fingerprint != fp {
				t.Errorf("pending peer = %s, want %s", st.Pending.Fingerprint, fp)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// attCount fetches a PDF endpoint and returns how many attestations it carries.
func attCount(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return len(p2p.ReadAttestations(b))
}

// autoConfirm is the peer's consent gate in tests: it always accepts.
type autoConfirm struct{ intent string }

func (a autoConfirm) Confirm(p2p.SignerAttestation, []byte) (bool, string, []byte, error) {
	return true, a.intent, nil, nil
}

// initiateForm builds the multipart body /api/session/initiate expects.
func initiateForm(t *testing.T, pdf, appearance []byte, params map[string]string, address string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pf, _ := mw.CreateFormFile("pdf", "doc.pdf")
	pf.Write(pdf)
	af, _ := mw.CreateFormFile("appearance", "appearance.png")
	af.Write(appearance)
	pj, _ := json.Marshal(params)
	mw.WriteField("params", string(pj))
	mw.WriteField("address", address)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// TestSessionInitiate drives the dialing side end to end: this server (Alice) signs
// the open document accepting a pinned peer (Bob) and dials Bob's receive listener;
// Bob co-signs and returns the result, which becomes Alice's doubly-signed open doc.
func TestSessionInitiate(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// Alice = the server (initiator); her fingerprint identifies her to the peer.
	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	aFPBytes, err := hex.DecodeString(me.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}

	// Bob = a pinned peer running a receive listener that accepts Alice.
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
		_, e = p2p.Receive(conn.Channel, bCert, bKey, "Alice", autoConfirm{intent: "I accept"}, okVerifier{})
		recvErr <- e
	}()

	// Alice initiates against Bob's listener.
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	body, ct := initiateForm(t, base, pageImage(t, 120, 40),
		map[string]string{"fingerprint": bFP, "intent": "I agree"}, ln.Addr().String())

	// Same shape as the send test: the initiating handler blocks on the local user's
	// spoken check, so the confirmation arrives on another request.
	go confirmVerificationVia(t, c, csrf, ts.URL)

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/initiate", ct, body)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("initiate status = %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	if e := <-recvErr; e != nil {
		t.Fatalf("receiver: %v", e)
	}

	// The open document is now the doubly-signed result.
	if n := attCount(t, c, ts.URL+"/api/pdf"); n != 2 {
		t.Errorf("open document has %d signers, want 2", n)
	}
}

// TestSessionArmRejectsUnpinnedPeer confirms arming for a peer who isn't pinned is
// refused — you can only open the listener for someone you've already pinned.
func TestSessionArmRejectsUnpinnedPeer(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	stranger := make([]byte, 32) // a fingerprint that was never pinned
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: hex.EncodeToString(stranger), Bind: "127.0.0.1:0"}))
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("armed for an unpinned peer")
	}
}

// A finished session's teardown does not close the one that replaced it.
//
// A session's accept goroutine can live for minutes — p2p.Receive spans the user's
// consent, the signing and a 128 MiB write — and arm() refuses only while a listener is
// present. So the user can Cancel and arm a NEW session while the old goroutine is still
// winding down, and its `defer disarm()` used to close whatever was armed by then. The
// user then sits waiting on a receive that a predecessor tore down, and is told "no peer
// connected" for a session that never had its chance.
//
// Driven at the session struct rather than over TLS: the race is entirely about which
// listener a teardown names, and a networked reproduction would add a peer, a handshake
// and a timing window to test a comparison.
func TestDisarmDoesNotCloseALaterSession(t *testing.T) {
	var se session

	lnA := &stubListener{}
	defer lnA.Close()
	if !se.arm(lnA, nil) {
		t.Fatal("setup: the first session did not arm")
	}

	// The user cancels, then arms again — the sequence that makes the old goroutine's
	// teardown dangerous.
	se.disarm()
	lnB := &stubListener{}
	defer lnB.Close()
	if !se.arm(lnB, nil) {
		t.Fatal("setup: the second session did not arm, so there is nothing for the old teardown to damage")
	}

	// Now the FIRST session's goroutine finishes and runs its deferred teardown.
	se.disarmIf(lnA)

	se.mu.Lock()
	armed := se.ln
	se.mu.Unlock()
	if armed != lnB {
		t.Errorf("after the previous session's teardown fired, the armed listener is %v, want the second session's. The user armed a new session and a predecessor closed it — they wait for a peer that can no longer reach them.", armed)
	}

	// And the unconditional form still works, because Cancel and shutdown mean exactly it.
	se.disarm()
	se.mu.Lock()
	armed = se.ln
	se.mu.Unlock()
	if armed != nil {
		t.Error("an explicit disarm left the session armed")
	}
}

// The same, for the consent request — where the consequence is worse: an unconditional
// clear discards a LATER session's pending consent, abandoning a peer's document while the
// user is looking at it.
func TestClearPendingDoesNotDropALaterSessionsConsent(t *testing.T) {
	var se session
	ln := &stubListener{}
	defer ln.Close()
	if !se.arm(ln, nil) {
		t.Fatal("setup: could not arm")
	}

	old := &pendingReq{resp: make(chan sessionDecision, 1)}
	if !se.setPending(old) {
		t.Fatal("setup: could not set the first pending request")
	}
	current := &pendingReq{resp: make(chan sessionDecision, 1)}
	if !se.setPending(current) {
		t.Fatal("setup: could not set the second pending request")
	}

	// The earlier confirmer's defer fires.
	se.clearPendingIf(old)

	se.mu.Lock()
	got := se.pending
	se.mu.Unlock()
	if got != current {
		t.Error("a finished confirmer's teardown discarded the consent request that replaced it — the peer's document is dropped from under the user while they are reviewing it")
	}
}

// A connection that never completes the TLS handshake does not consume the armed session.
//
// tls.Listener.Accept returns on the TCP accept and does no handshake, so ANY connection
// used to take the one-shot session: a port scan, a stray dial, a browser probing the
// address the user just pasted. Refusal was free for whoever connected and expensive for
// the user, who had to notice their session had died and arm it again — while the peer they
// were waiting for could no longer reach them.
//
// Driven with a bare TCP dial, which is what a scanner does: connect, send nothing that
// parses as TLS, close. The session must still be armed afterwards.
func TestAStrayConnectionDoesNotConsumeTheSession(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	aCert, _, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	aFPBytes, _ := sign.Fingerprint(aCert)
	pinPeer(t, c, csrf, ts.URL, hex.EncodeToString(aFPBytes))

	var armed sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: hex.EncodeToString(aFPBytes), Bind: "127.0.0.1:0"})), &armed)
	if !armed.Armed || armed.Address == "" {
		t.Fatalf("setup: arm failed: %+v", armed)
	}

	// The stray connection: plain TCP, junk bytes, gone.
	stray, err := net.DialTimeout("tcp", armed.Address, 5*time.Second)
	if err != nil {
		t.Fatalf("setup: could not reach the armed listener at %s: %v", armed.Address, err)
	}
	_, _ = stray.Write([]byte("hello?\n"))
	stray.Close()

	// The session must still be armed. Polled rather than read once: the accept loop
	// handles the stray connection asynchronously, so an immediate read could observe the
	// state before the handshake has even been attempted and pass without meaning it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if !st.Armed {
			t.Fatal("a bare TCP connection that sent no TLS consumed the armed session — a port scan disarms the user's receive, and the peer they are waiting for can no longer reach them")
		}
		if time.Now().After(deadline) {
			break // stayed armed throughout, which is the property
		}
		time.Sleep(150 * time.Millisecond)
	}

	// And it can still be disarmed normally, so "armed" above is a live session rather
	// than a stuck flag.
	write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/disarm", "application/json", nil).Body.Close()
	var after sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &after)
	if after.Armed {
		t.Error("the session could not be disarmed afterwards")
	}
}

// recordingVerifier confirms the spoken check and remembers what it was shown, so a test
// can compare the two ends of one session. Recording rather than merely accepting: an
// auto-accepting verifier proves the gate was passed and says nothing about whether both
// sides were shown the same thing.
type recordingVerifier struct {
	mu    sync.Mutex
	words string
	calls int
}

func (r *recordingVerifier) ConfirmVerification(words string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.words = words
	r.calls++
	return true, nil
}

func (r *recordingVerifier) seen() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.words
}

func (r *recordingVerifier) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// confirmVerificationVia polls the status route for the spoken check and confirms it.
//
// **Self-contained on purpose: it is called from a goroutine.** The sending side's handler
// blocks while it waits for this, so the confirmation must arrive on another request — and
// `t.Fatal` from a non-test goroutine does not stop the test, it just fails to report. So
// this uses the http client directly rather than the Fatal-ing helpers, and reports with
// Errorf. `go vet` catches the mistake, which is how this shape was arrived at.
func confirmVerificationVia(t *testing.T, c *http.Client, csrf, base string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := c.Get(base + "/api/session/status")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var st sessionStatus
		_ = json.NewDecoder(resp.Body).Decode(&st)
		resp.Body.Close()
		if st.Verify == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		req, err := http.NewRequest(http.MethodPost, base+"/api/session/verify",
			strings.NewReader(`{"confirmed":true}`))
		if err != nil {
			t.Errorf("build verify request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		vr, err := c.Do(req)
		if err != nil {
			t.Errorf("verify request: %v", err)
			return
		}
		vr.Body.Close()
		if vr.StatusCode != http.StatusOK {
			t.Errorf("verify status = %d", vr.StatusCode)
		}
		return
	}
	t.Errorf("no verification appeared within 20s — either the gate did not fire, or the " +
		"session reached the document exchange without it (L2)")
}

// okVerifier confirms the spoken check without looking, for the far side of a test whose
// subject is something else. Named so a reader sees the gate was satisfied deliberately.
type okVerifier struct{}

func (okVerifier) ConfirmVerification(string) (bool, error) { return true, nil }

// peerFPForTest is the SPKI fingerprint of a test identity's cert.
func peerFPForTest(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	fp, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

// csrfFor reads the CSRF token off /api/status, so a helper that needs to POST does not
// have to be handed one through every call site it already has.
func csrfFor(t *testing.T, c *http.Client, baseURL string) string {
	t.Helper()
	var st struct {
		CSRF string `json:"csrf"`
	}
	sessGet(t, c, baseURL+"/api/status", &st)
	if st.CSRF == "" {
		t.Fatal("no CSRF token on /api/status")
	}
	return st.CSRF
}

// peerChannel establishes a p2p.Channel over a completed mTLS connection, exactly as the
// server's own call sites do. Built through p2p.TLSChannel rather than assembled here so
// a test cannot keep passing while the constructor stops reading the peer's fingerprint
// off the verified chain.
func peerChannel(t *testing.T, conn *tls.Conn) p2p.Channel {
	t.Helper()
	ch, err := p2p.TLSChannel(conn)
	if err != nil {
		// Errorf, not Fatalf: several call sites are inside accept goroutines, where
		// FailNow is not legal. The zero Channel then fails p2p's own check.
		t.Errorf("establish channel: %v", err)
		return p2p.Channel{}
	}
	return ch
}

// stubListener is two distinct p2p.Listener values and nothing else. The test it serves
// is about disarmIf comparing IDENTITY — that a teardown fired by an old goroutine does
// not close the session that REPLACED it — so it never accepts, and a real bound socket
// would only add a port and a cleanup to a test that is entirely about pointer equality.
type stubListener struct{ closed bool }

func (l *stubListener) Accept() (*p2p.Conn, error) { return nil, net.ErrClosed }
func (l *stubListener) Transport() string          { return "tcp" }
func (l *stubListener) Close() error               { l.closed = true; return nil }

// Addr must be non-nil: session.arm records ln.Addr().String() as the address it
// reports in status, so a nil here is a panic at arm rather than a failed assertion.
func (l *stubListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
}

// TestAnUnknownTransportIsRefusedNotSilentlyDowngraded.
//
// Introduced by P02.S05 and caught at P02's phase close: the selector was
// `transport == "quic"`, so every other value — "QUIC", "Quic", a typo, "udp" —
// silently chose TCP. The two failures that produces are the hardest kind to
// diagnose. Either the sides disagree, and a spelling mistake is reported to the
// user as "could not connect to peer"; or they agree on the fallback, and the user
// believes they are on a transport they are not.
//
// The empty string stays TCP, because every caller that predates D14 sends nothing.
func TestAnUnknownTransportIsRefusedNotSilentlyDowngraded(t *testing.T) {
	// Stimulus: the values that ARE accepted are accepted. Without this, "every
	// input is refused" passes the loop below and breaks the product.
	for _, ok := range []string{"", "tcp", "quic"} {
		if err := checkTransport(ok); err != nil {
			t.Fatalf("transport %q was refused: %v — the accepted set is wrong and the "+
				"refusals below prove nothing", ok, err)
		}
	}
	for _, bad := range []string{"QUIC", "Quic", "quic ", "tcp/ip", "udp", "quick", "TCP"} {
		err := checkTransport(bad)
		if err == nil {
			t.Errorf("transport %q was accepted and silently means TCP", bad)
			continue
		}
		if !errors.Is(err, errUnknownTransport) {
			t.Errorf("transport %q failed with %v, not as an unknown transport — the route "+
				"would report it as a peer problem rather than the caller's", bad, err)
		}
		// The message must name what was sent, or a typo is invisible in a log.
		if !strings.Contains(err.Error(), bad) {
			t.Errorf("the error for %q does not quote it: %v", bad, err)
		}
	}
}

// TestAnUnknownSessionModeIsRefusedNotSilentlyDowngraded.
//
// `req.Mode` was used raw — `if mode == sessionModeReceive`, anything else co-signs. That
// is byte-for-byte the defect `checkTransport` was written to refuse, with the argument
// spelled out over sixteen lines and a sibling test of nearly this name. "Receive",
// "recieve" and "transfer" all silently armed a CO-SIGNING listener when the user asked for
// a one-way transfer — which is the difference between showing someone a document and
// putting your signing key on it.
func TestAnUnknownSessionModeIsRefusedNotSilentlyDowngraded(t *testing.T) {
	// The controls FIRST: both real modes, and the empty string older clients send, must
	// still be accepted. A validator that refuses everything is an outage.
	for _, ok := range []string{sessionModeReceive, sessionModeCoSign, ""} {
		if err := checkSessionMode(ok); err != nil {
			t.Fatalf("checkSessionMode(%q) refused a mode this build ships: %v", ok, err)
		}
	}
	for _, bad := range []string{"Receive", "recieve", "transfer", "RECEIVE", "co-sign", "send"} {
		err := checkSessionMode(bad)
		if !errors.Is(err, errUnknownSessionMode) {
			t.Errorf("checkSessionMode(%q) = %v — this armed a co-signing listener for a "+
				"user who asked for a transfer", bad, err)
		}
		// The message must quote what was sent, or a client typo is undiagnosable.
		if err != nil && !strings.Contains(err.Error(), bad) {
			t.Errorf("the refusal for %q does not quote it: %v", bad, err)
		}
	}
}

// TestASecondSpokenCheckCannotDisplaceTheOneOnScreen.
//
// `setVerify` assigned unconditionally while `clearVerifyIf` beside it carries an identity
// guard — two halves of one invariant disagreeing. Two gates can genuinely be in flight (an
// armed receive session while the user posts /api/session/send), and the second overwrote
// the first: the displaced goroutine sat on a channel nobody would write to until its
// five-minute timeout, and `respondVerify` routed the user's answer to whichever was
// current.
//
// The user cannot tell them apart — `verifyView` deliberately carries no fingerprint and no
// peer label, which is right for one gate and is exactly what makes two unanswerable.
func TestASecondSpokenCheckCannotDisplaceTheOneOnScreen(t *testing.T) {
	_, s := startServerWith(t)

	first := &pendingVerify{words: "one two three four", resp: make(chan bool, 1)}
	if !s.sess.setVerify(first) {
		t.Fatal("setup: the first gate was refused with nothing pending")
	}
	// STIMULUS: the first gate is really the one parked. Without this the refusal below
	// could be a setVerify that refuses everything.
	if pending := s.sess.currentVerify(); pending != first {
		t.Fatalf("setup: the parked gate is not the first one (%+v)", pending)
	}

	second := &pendingVerify{words: "five six seven eight", resp: make(chan bool, 1)}
	if s.sess.setVerify(second) {
		t.Error("a second session's spoken check displaced the one already on screen — the " +
			"user's answer then belongs to words they never saw, and the first session " +
			"hangs on a channel nobody will write to")
	}
	if pending := s.sess.currentVerify(); pending != first {
		t.Errorf("the parked gate changed after a second setVerify; the incumbent must survive")
	}

	// And the seat frees up properly: after the incumbent clears, the next one is admitted.
	// A refusal that never lifts would make the gate a one-shot for the process.
	s.sess.clearVerifyIf(first)
	if !s.sess.setVerify(second) {
		t.Error("the verification seat did not free after the incumbent cleared")
	}
	s.sess.clearVerifyIf(second)
}

// TestTheSessionBudgetsCoverBothPeerGates — the coupling asserted from the side that can see both.
//
// `internal/p2p` sizes its remote-decision budget from `p2p.PeerGateWindow`, and this package
// enforces the actual gate with `sessionConsentTimeout`. They are two statements of one number
// in two packages, and p2p cannot import this one (the dependency runs the other way), so the
// assertion has to live here — which is also why the drift went unnoticed.
func TestTheSessionBudgetsCoverBothPeerGates(t *testing.T) {
	if p2p.PeerGateWindow != sessionConsentTimeout {
		t.Errorf("p2p.PeerGateWindow = %v and sessionConsentTimeout = %v — p2p sizes the "+
			"dialing side's wait from its own copy, so a change here silently makes that "+
			"budget too small and the dialer times out while the peer is still deciding",
			p2p.PeerGateWindow, sessionConsentTimeout)
	}
	// And the dialing side must outwait two of them plus the transfer.
	if got := p2p.MaxRemoteDecisionWait(); got <= 2*sessionConsentTimeout {
		t.Errorf("the dialing side allows %v for the peer's decisions, and this server lets "+
			"one user hold a gate for %v — twice that is already %v", got,
			sessionConsentTimeout, 2*sessionConsentTimeout)
	}
}
