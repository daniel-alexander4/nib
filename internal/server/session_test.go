package server

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"nib/internal/p2p"
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
