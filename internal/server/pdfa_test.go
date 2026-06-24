package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// pdfaForm builds the multipart body the pdfa endpoint expects: the document as
// the "pdf" part (the shared export shape).
func pdfaForm(t *testing.T, pdf []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// TestPDFAHandler: an image-only PDF (no fonts) converts, and the returned PDF/A
// candidate carries the GTS_PDFA1 OutputIntent and the pdfaid identifier.
func TestPDFAHandler(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	src, err := pdfops.ImagesToPDF([]pdfops.RasterPage{{Image: tinyPNG(t), W: 200, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	body, ct := pdfaForm(t, src)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pdfa", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("pdfa status = %d: %s", resp.StatusCode, out)
	}
	out, _ := io.ReadAll(resp.Body)
	if err := pdfops.Validate(out); err != nil {
		t.Errorf("converted PDF invalid: %v", err)
	}
	if !bytes.Contains(out, []byte("GTS_PDFA1")) || !bytes.Contains(out, []byte("pdfaid:part")) {
		t.Error("output missing PDF/A markers")
	}
}

// TestPDFAHandlerRefusesNonEmbeddedFonts: a document with a non-embedded font is
// rejected with 400 and a reason naming the font problem.
func TestPDFAHandlerRefusesNonEmbeddedFonts(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	src, err := testpdf.Text("not embedded") // standard-14 Courier, not embedded
	if err != nil {
		t.Fatal(err)
	}
	body, ct := pdfaForm(t, src)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pdfa", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pdfa on a non-embedded-font PDF status = %d, want 400", resp.StatusCode)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(bytes.ToLower(out), []byte("embed")) {
		t.Errorf("400 body should explain the font problem, got: %s", out)
	}
}
