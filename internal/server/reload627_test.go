package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestReloadRefusesAFileThatBecameAnImage — /pending 627.
//
// A document opened from a PDF keeps its path. If the file at that path is then replaced by an
// image, the reload converted it, committed the PDF into the document that still held the path,
// and stamped the baseline from the image — so the next Save saw nothing changed on disk and
// wrote a PDF over the user's image. Both opens drop the path for exactly this; the reload cannot,
// so it refuses.
func TestReloadRefusesAFileThatBecameAnImage(t *testing.T) {
	s := New(nil, nil, t.TempDir(), "test")
	pdf, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	a := pathDoc499(t, s, pdf, 0o600)
	img := pageImage(t, 40, 40)
	if !pdfops.LooksLikeImage(img) {
		t.Fatal("setup: the fixture is not an image nib converts, so this would test the not-a-PDF refusal")
	}
	if err := os.WriteFile(a.path, img, 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/reload", nil)
	req.Header.Set("X-Nib-Doc", a.id.String())
	rec := httptest.NewRecorder()
	s.handleReload(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), "now an image") {
		t.Errorf("a reload of a path that now holds an image answered %d %q, want 415 naming the image",
			rec.Code, rec.Body)
	}
	if !bytes.Equal(s.docBytes(a), pdf) {
		t.Error("the reload installed a PDF made from the image into a document that keeps the image's " +
			"path — the next Save writes that PDF over the image")
	}
	onDisk, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, img) {
		t.Error("the refused reload changed the file")
	}
}

// TestARouteThatReadsOnlyFieldsCapsItsBody — /pending 627's second half, as amended.
//
// Three routes read a form field with a bare FormValue and nothing before it: a multipart body of
// any size was parsed, 32 MiB of it into memory and the rest onto disk, for one word. (The temp
// files were never the leak the entry first said — net/http removes them when the handler
// returns.) Each is sent a megabyte ahead of its field; with the cap the parse fails and the field
// reads as absent.
func TestARouteThatReadsOnlyFieldsCapsItsBody(t *testing.T) {
	s := New(nil, nil, t.TempDir(), "test")
	pdf, err := testpdf.Text("a")
	if err != nil {
		t.Fatal(err)
	}
	a := pathDoc499(t, s, pdf, 0o600)

	for _, tc := range []struct {
		name         string
		handle       http.HandlerFunc
		field, value string
	}{
		{"reload", s.handleReload, "acceptSignatureLoss", "1"},
		{"sanitize", s.handleSanitize, "method", "metadata"},
		{"decrypt", s.handleDecrypt, "password", "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			mw.WriteField("padding", strings.Repeat("x", 1<<20))
			mw.WriteField(tc.field, tc.value)
			mw.Close()
			size := buf.Len()
			if size <= maxFieldBody {
				t.Fatalf("setup: the body is %d bytes, inside the %d cap", size, maxFieldBody)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/"+tc.name, &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-Nib-Doc", a.id.String())
			rec := httptest.NewRecorder()
			tc.handle(rec, req)
			if req.MultipartForm != nil {
				t.Errorf("%s parsed a %d-byte multipart body whole (status %d) — nothing capped it",
					tc.name, size, rec.Code)
			}
		})
	}

	// The control: an ordinary small form is still read.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("method", "metadata")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/sanitize", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Nib-Doc", a.id.String())
	rec := httptest.NewRecorder()
	s.handleSanitize(rec, req)
	if rec.Code != http.StatusOK || req.MultipartForm == nil {
		t.Errorf("control: a small sanitize form answered %d %q and was not parsed", rec.Code, rec.Body)
	}
}
