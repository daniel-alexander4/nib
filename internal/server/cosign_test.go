package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

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
	if w, h := q.Rect[2]-q.Rect[0], q.Rect[3]-q.Rect[1]; w != 280 || h != 84 {
		t.Errorf("quote rect size = %gx%g, want 280x84", w, h)
	}
	if q.When == "" {
		t.Error("quote did not return a signing time")
	}
}
