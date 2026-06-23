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

// encryptForm builds the multipart body the encrypt endpoint expects: the PDF as
// the "pdf" part plus an optional password field (omitted when password == "").
func encryptForm(t *testing.T, pdf []byte, password string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	if password != "" {
		mw.WriteField("password", password)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// TestEncryptHandler: POSTing a PDF + password returns protected bytes that need
// the password (RemovePassword with it round-trips). It operates on the uploaded
// bytes, so it needs no open document.
func TestEncryptHandler(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	pdf, _ := testpdf.Text("protect me")
	body, ct := encryptForm(t, pdf, "secret123")
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/encrypt", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("encrypt status = %d: %s", resp.StatusCode, out)
	}
	enc, _ := io.ReadAll(resp.Body)
	if err := pdfops.Validate(enc); err == nil {
		t.Fatal("returned PDF validated without a password — encryption did not take")
	}
	if _, err := pdfops.RemovePassword(enc, "secret123"); err != nil {
		t.Fatalf("protected copy does not open with its password: %v", err)
	}
}

// TestEncryptHandlerNoPassword: a missing password is rejected (400), never an
// unprotected file.
func TestEncryptHandlerNoPassword(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	pdf, _ := testpdf.Text("protect me")
	body, ct := encryptForm(t, pdf, "")
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/encrypt", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("encrypt with no password status = %d, want 400", resp.StatusCode)
	}
}
