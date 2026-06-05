package server

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// Redacting a page must REPLACE it with a flat image so the underlying content
// is genuinely removed, not merely covered. The form fixture carries a
// "fullName" field on page 1; after redacting that page through the HTTP handler
// the field must be gone from the document the server now serves.
func TestRedactRemovesContent(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if before, _ := pdfops.ExportFormJSON(form); !bytes.Contains(before, []byte("fullName")) {
		t.Fatal("fixture should contain the fullName field before redaction")
	}

	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path) // a current document must exist

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(form)
	iw, _ := mw.CreateFormFile("page", "page-1.png")
	png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 1224, 1584)))
	mw.WriteField("pageNum", "1")
	mw.WriteField("pageW", "612")
	mw.WriteField("pageH", "792")
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/redact", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redact status = %d, want 200", resp.StatusCode)
	}

	pdfResp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := pdfops.PageCount(got); n != 1 {
		t.Errorf("redacted document page count = %d, want 1", n)
	}
	if after, _ := pdfops.ExportFormJSON(got); bytes.Contains(after, []byte("fullName")) {
		t.Error("redacted page still exposes the form field — content not truly removed")
	}
}
