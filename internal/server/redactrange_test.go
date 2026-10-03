package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestRedactRouteRefusesAPageTheDocumentDoesNotHave walks /pending 819 to the observable the user
// sees: a raster keyed n+1, 0 or -1 against the one-page form used to come back 200 — the client's
// "Redacted" toast — with the field's page untouched. It must now be ADR-072's 422 naming the cause
// and the page, the open document's bytes must be exactly what they were, and a page posted twice is
// a malformed request (400), because the map kept only the later image.
func TestRedactRouteRefusesAPageTheDocumentDoesNotHave(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	served := func() []byte {
		t.Helper()
		resp, err := c.Get(ts.URL + "/api/pdf")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	post := func(nums ...string) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(form)
		for _, n := range nums {
			iw, _ := mw.CreateFormFile("page", "page-"+n+".png")
			png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 100, 100)))
			mw.WriteField("pageNum", n)
			mw.WriteField("pageW", "612")
			mw.WriteField("pageH", "792")
		}
		mw.Close()
		return write(t, c, csrf, http.MethodPost, ts.URL+"/api/redact", mw.FormDataContentType(), &buf)
	}

	before := served()
	for _, tc := range []struct {
		nums []string
		page int
	}{{[]string{"2"}, 2}, {[]string{"0"}, 0}, {[]string{"-1"}, -1}, {[]string{"1", "2"}, 2}} {
		resp := post(tc.nums...)
		var got pageRefusal
		decErr := json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("pageNum %v: status = %d, want 422", tc.nums, resp.StatusCode)
		}
		if decErr != nil || got.Cause != "page-not-in-document" || got.Page != tc.page || got.Pages != 1 {
			t.Errorf("pageNum %v: body = %+v (%v), want cause page-not-in-document, page %d of 1", tc.nums, got, decErr, tc.page)
		}
		if after := served(); !bytes.Equal(after, before) {
			t.Errorf("pageNum %v: the refused redaction changed the open document", tc.nums)
		}
	}

	resp := post("1", "1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("page posted twice: status = %d, want 400", resp.StatusCode)
	}
	if after := served(); !bytes.Equal(after, before) {
		t.Error("page posted twice: the refused redaction changed the open document")
	}

	// The in-range page still lands, so the refusals above are about the key and nothing else.
	resp = post("1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("in-range redaction: status = %d, want 200", resp.StatusCode)
	}
	if j, _ := pdfops.ExportFormJSON(served()); bytes.Contains(j, []byte("fullName")) {
		t.Error("in-range redaction kept the page's field")
	}
}
