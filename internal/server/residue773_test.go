package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /pending 773 — /api/open-url reads the document's type from the URL's path.
//
// It took `path.Base` of the raw URL, query included, so `report.docx?dl=1` ended in `.docx?dl=1`,
// matched no type nib converts, and the user was told the URL "did not return a PDF" about a
// document nib has a route for.
func TestOpenURLReadsTheTypeFromThePathNotTheQuery(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("neither a PDF nor an image"))
	}))
	defer src.Close()
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	open := func(u string) (int, string) {
		t.Helper()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open-url", "application/json",
			jsonBody(map[string]string{"url": u}))
		defer resp.Body.Close()
		return resp.StatusCode, readBody(t, resp)
	}

	// The control: with no query the convertible refusal was always reached.
	if code, body := open(src.URL + "/report.docx"); code != http.StatusUnsupportedMediaType || !strings.Contains(body, ".docx through") {
		t.Fatalf("control: a .docx URL with no query answered %d %q, want the convertible refusal", code, body)
	}
	code, body := open(src.URL + "/report.docx?dl=1")
	if code != http.StatusUnsupportedMediaType || !strings.Contains(body, ".docx through") {
		t.Errorf("a .docx URL with a query answered %d %q, want the same convertible refusal — the "+
			"type was read from the query", code, body)
	}
	// And a query does not MAKE something convertible.
	if code, body := open(src.URL + "/report?name=x.docx"); !strings.Contains(body, "did not return a PDF") {
		t.Errorf("a URL whose QUERY ends in .docx answered %d %q, want the not-a-PDF refusal", code, body)
	}
}

// /pending 773 — the open-document cap is asked before a document is converted or fetched.
//
// Both routes did the whole of their work and were refused at the install. The observable is
// which refusal answers: each request below fails its own work (a language nib will not declare,
// a URL nothing listens on), so before the cap it answers with that failure and at the cap it
// must answer 409 without having got that far.
func TestTheOpenCapIsAskedBeforeADocumentIsConvertedOrFetched(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	office := func() (int, string) {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "notes.md")
		fw.Write([]byte("# Notes\n"))
		mw.WriteField("lang", "!!! not a language")
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/office", mw.FormDataContentType(), &buf)
		defer resp.Body.Close()
		return resp.StatusCode, readBody(t, resp)
	}
	openURL := func() (int, string) {
		t.Helper()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open-url", "application/json",
			jsonBody(map[string]string{"url": "http://127.0.0.1:1/nope.pdf"}))
		defer resp.Body.Close()
		return resp.StatusCode, readBody(t, resp)
	}

	// The controls: under the cap, each reaches its own failure.
	if code, body := office(); code != http.StatusBadRequest {
		t.Fatalf("control: a conversion naming a bad language answered %d %q under the cap, want 400", code, body)
	}
	if code, body := openURL(); code != http.StatusBadGateway {
		t.Fatalf("control: an unreachable URL answered %d %q under the cap, want 502", code, body)
	}

	for i := 1; i <= maxOpenDocs; i++ {
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", jsonBody(openRequest{Path: path}))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("setup: open %d of %d answered %d", i, maxOpenDocs, resp.StatusCode)
		}
	}

	if code, body := office(); code != http.StatusConflict || !strings.Contains(body, "too many documents open") {
		t.Errorf("at the cap /api/office answered %d %q, want 409 — it converted the document first", code, body)
	}
	if code, body := openURL(); code != http.StatusConflict || !strings.Contains(body, "too many documents open") {
		t.Errorf("at the cap /api/open-url answered %d %q, want 409 — it fetched the document first", code, body)
	}
}
