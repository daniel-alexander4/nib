package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
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

func pinPeer(t *testing.T, c *http.Client, csrf, base, fp string) {
	t.Helper()
	resp := write(t, c, csrf, http.MethodPost, base+"/api/peers/pin", "application/json",
		jsonBody(pinPeerRequest{Fingerprint: fp, Label: "Alice"}))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin peer status = %d", resp.StatusCode)
	}
}

func cosignSignForm(t *testing.T, fp, intent string) (*bytes.Buffer, string) {
	t.Helper()
	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	pj, _ := json.Marshal(cosignParams{Fingerprint: fp, Intent: intent})
	mw.WriteField("params", string(pj))
	aw, _ := mw.CreateFormFile("appearance", "att.png")
	aw.Write(tinyPNG(t))
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// Co-signing a pinned peer produces a valid approval signature whose /Reason
// cross-binds to that peer's fingerprint.
func TestCosignSignProducesCoSignedDoc(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	fp := strings.Repeat("ab", 32)
	pinPeer(t, c, csrf, ts.URL, fp)

	body, ct := cosignSignForm(t, fp, "I agree to sign this document.")
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/sign", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("cosign sign status = %d: %s", resp.StatusCode, b)
	}
	signed, _ := io.ReadAll(resp.Body)

	if st := sign.Verify(signed); st.State != sign.Valid || len(st.Signers) != 1 {
		t.Fatalf("co-signed verify = %q signers=%d, want valid/1", st.State, len(st.Signers))
	}
	atts := p2p.ReadAttestations(signed)
	if len(atts) != 1 || atts[0].AcceptedPeer != fp {
		t.Fatalf("attestation = %+v, want accepted peer %s", atts, fp)
	}
}

// Co-signing accepting a peer that isn't pinned must be refused.
func TestCosignSignRefusesUnpinned(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	body, ct := cosignSignForm(t, strings.Repeat("cd", 32), "x")
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/sign", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unpinned cosign status = %d, want 400", resp.StatusCode)
	}
}

// Quote returns the canonical attestation lines and a placement whose constant
// size (280×84) is what the client sizes the rasterized block to.
func TestCosignQuoteReturnsLinesAndRect(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)

	fp := strings.Repeat("ab", 32)
	pinPeer(t, c, csrf, ts.URL, fp)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json",
		jsonBody(openRequest{Path: pdfPath}))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open status = %d", resp.StatusCode)
	}

	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/quote", "application/json",
		jsonBody(cosignParams{Fingerprint: fp, Intent: "ok"}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("quote status = %d: %s", resp.StatusCode, b)
	}
	var q cosignQuote
	json.NewDecoder(resp.Body).Decode(&q)
	if len(q.Lines) != 5 {
		t.Errorf("quote lines = %d, want 5", len(q.Lines))
	}
	// Sized from p2p's door rather than from the literals it happens to produce: a
	// test that names 280x84 asserts the copy agrees with itself and stays green while
	// both sites drift together.
	if want := p2p.NominalBlockRect(); q.Rect[2]-q.Rect[0] != want[2]-want[0] ||
		q.Rect[3]-q.Rect[1] != want[3]-want[1] {
		t.Errorf("quote rect size = %gx%g, want %gx%g (p2p.NominalBlockRect)",
			q.Rect[2]-q.Rect[0], q.Rect[3]-q.Rect[1], want[2]-want[0], want[3]-want[1])
	}
	if q.When == "" {
		t.Error("quote did not return a signing time")
	}
}

// The attestations endpoint surfaces, per signer, the cross-binding (Matched) and
// whether the viewer has pinned that signer locally.
func TestAttestationsEndpoint(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// A mutual A<->B co-signed document, built out-of-band.
	base, _ := testpdf.Form()
	cA, kA, _ := sign.GenerateIdentity("Alice")
	cB, kB, _ := sign.GenerateIdentity("Bob")
	fA, _ := sign.Fingerprint(cA)
	fB, _ := sign.Fingerprint(cB)
	now := time.Now()
	prep, _ := p2p.PrepareDocument(base)
	pa, _ := p2p.NextPlacement(prep)
	dA, _ := p2p.Contribute(prep, cA, kA, p2p.Attestation{Signer: "Alice", AcceptedPeer: hex.EncodeToString(fB), AcceptedPeerLabel: "Bob", When: now}, tinyPNG(t), pa)
	pb, _ := p2p.NextPlacement(dA)
	dB, _ := p2p.Contribute(dA, cB, kB, p2p.Attestation{Signer: "Bob", AcceptedPeer: hex.EncodeToString(fA), AcceptedPeerLabel: "Alice", When: now}, tinyPNG(t), pb)

	// Pin only Alice in the viewer's vault.
	pinPeer(t, c, csrf, ts.URL, hex.EncodeToString(fA))

	path := filepath.Join(t.TempDir(), "co.pdf")
	if err := os.WriteFile(path, dB, 0o600); err != nil {
		t.Fatal(err)
	}
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", jsonBody(openRequest{Path: path}))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open status = %d", resp.StatusCode)
	}

	r, err := c.Get(ts.URL + "/api/attestations")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var got attestationsResponse
	json.NewDecoder(r.Body).Decode(&got)
	if len(got.Attestations) != 2 {
		t.Fatalf("attestations = %d, want 2", len(got.Attestations))
	}
	a, b := got.Attestations[0], got.Attestations[1]
	if !a.Matched || !b.Matched {
		t.Errorf("both should cross-bind: %+v / %+v", a, b)
	}
	if a.Signer != "Alice" || !a.Pinned {
		t.Errorf("Alice should be pinned: %+v", a)
	}
	if b.Signer != "Bob" || b.Pinned {
		t.Errorf("Bob should not be pinned: %+v", b)
	}
}
