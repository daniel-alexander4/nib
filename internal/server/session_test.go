package server

import (
	"bytes"
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
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		final, e := p2p.Initiate(conn, aSigned, aFPBytes)
		if e != nil {
			errc <- e
			return
		}
		result <- final
	}()

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
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		if _, e := p2p.Initiate(conn, aSigned, aFPBytes); e == nil {
			errc <- nil // a declined round-trip must surface an error to the initiator
			return
		}
		errc <- nil
	}()

	// Wait for the request to park.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no pending request appeared")
		}
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Pending != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

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
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		_, _ = p2p.Initiate(conn, aSigned, aFPBytes) // declined below; an error is expected
		errc <- nil
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no pending request appeared")
		}
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Pending != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

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
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		errc <- p2p.SendDocument(conn, flagged)
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
		conn, e := p2p.Dial(armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		errc <- p2p.SendDocument(conn, base)
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
		doc, _, e := p2p.ReceiveDocument(conn.(*tls.Conn), autoAccept{})
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

// waitPending polls session status until a request from fp is parked, or fails.
func waitPending(t *testing.T, c *http.Client, baseURL, fp string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no pending request appeared")
		}
		var st sessionStatus
		sessGet(t, c, baseURL+"/api/session/status", &st)
		if st.Pending != nil {
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
		_, e = p2p.Receive(conn.(*tls.Conn), bCert, bKey, "Alice", autoConfirm{intent: "I accept"})
		recvErr <- e
	}()

	// Alice initiates against Bob's listener.
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	body, ct := initiateForm(t, base, pageImage(t, 120, 40),
		map[string]string{"fingerprint": bFP, "intent": "I agree"}, ln.Addr().String())
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

	lnA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lnA.Close()
	if !se.arm(lnA) {
		t.Fatal("setup: the first session did not arm")
	}

	// The user cancels, then arms again — the sequence that makes the old goroutine's
	// teardown dangerous.
	se.disarm()
	lnB, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lnB.Close()
	if !se.arm(lnB) {
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if !se.arm(ln) {
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
