package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// openByPath opens a path-based document on an authenticated client.
func openByPath(t *testing.T, baseURL string, c *http.Client, csrf, path string) docResponse {
	t.Helper()
	body, _ := json.Marshal(openRequest{Path: path})
	resp := write(t, c, csrf, http.MethodPost, baseURL+"/api/open", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open status = %d, want 200", resp.StatusCode)
	}
	var dr docResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatal(err)
	}
	return dr
}

func TestOpenAndFetchPDF(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	dr := openByPath(t, ts.URL, c, csrf, path)

	if !dr.CanSave {
		t.Error("path-opened document should be saveable")
	}
	if dr.Path != path {
		t.Errorf("path = %q, want %q", dr.Path, path)
	}
	if dr.Signature.State != "unsigned" {
		t.Errorf("signature = %q, want unsigned", dr.Signature.State)
	}

	resp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(got, []byte("%PDF")) {
		t.Errorf("served bytes are not a PDF: %.10q", got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("content-type = %q, want application/pdf", ct)
	}
}

func TestSaveOverwritesInPlace(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	newBytes, _ := testpdf.Form()
	newBytes = append(newBytes, "\n% edited\n"...)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/save", "application/pdf", bytes.NewReader(newBytes))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d, want 200", resp.StatusCode)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, newBytes) {
		t.Error("file on disk was not overwritten with the saved bytes")
	}
}

func TestSaveWithoutPathRejected(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	data, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "form.pdf")
	fw.Write(data)
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/upload", mw.FormDataContentType(), &buf)
	var dr docResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	resp.Body.Close()
	if dr.CanSave {
		t.Error("uploaded document should not be saveable in place")
	}

	saveResp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/save", "application/pdf", bytes.NewReader(data))
	saveResp.Body.Close()
	if saveResp.StatusCode != http.StatusConflict {
		t.Errorf("save status = %d, want 409", saveResp.StatusCode)
	}
}

func TestOpenMissingFile(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	body, _ := json.Marshal(openRequest{Path: filepath.Join(t.TempDir(), "nope.pdf")})
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("open missing status = %d, want 404", resp.StatusCode)
	}
}

func TestLoopbackGuardRejectsForeignHost(t *testing.T) {
	ts, _ := startServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.Host = "evil.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign-host status = %d, want 403", resp.StatusCode)
	}
}

// The loopback guard's two predicates: an unspoofable peer-IP check plus the
// Host allowlist that blocks DNS rebinding. httptest always connects over
// loopback, so the non-loopback peer path is only reachable as a unit test.
func TestLoopbackPredicates(t *testing.T) {
	for _, tc := range []struct {
		remote string
		want   bool
	}{
		{"127.0.0.1:5051", true},
		{"[::1]:5051", true},
		{"192.168.1.10:5051", false},
		{"8.8.8.8:443", false},
		{"garbage", false},
	} {
		if got := peerIsLoopback(tc.remote); got != tc.want {
			t.Errorf("peerIsLoopback(%q) = %v, want %v", tc.remote, got, tc.want)
		}
	}
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"127.0.0.1:5051", true},
		{"localhost:5051", true},
		{"[::1]:5051", true},
		{"localhost", true},
		{"evil.example.com:5051", false},
		{"192.168.1.10:5051", false},
	} {
		if got := hostIsLoopback(tc.host); got != tc.want {
			t.Errorf("hostIsLoopback(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// The defect this closes: any readable file became the open document, and because
// a path-opened document reports canSave, the next Save wrote PDF bytes over it.
// Asserting the refusal alone would miss the half that actually destroyed data,
// so this also proves the file on disk is untouched and no document was adopted.
func TestOpenRejectsNonPDFAndLeavesItIntact(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	notes := filepath.Join(t.TempDir(), "notes.txt")
	original := []byte("not a pdf at all\n")
	if err := os.WriteFile(notes, original, 0o600); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(openRequest{Path: notes})
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("open(non-PDF) status = %d, want 415", resp.StatusCode)
	}

	// Nothing was adopted, so /api/pdf must still report "no document open"
	// rather than handing the text back as a PDF.
	pr, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Body.Close()
	if pr.StatusCode != http.StatusNotFound {
		t.Errorf("pdf status after a refused open = %d, want 404", pr.StatusCode)
	}

	if got, err := os.ReadFile(notes); err != nil || !bytes.Equal(got, original) {
		t.Errorf("the file changed on disk: %q (err %v)", got, err)
	}
}

// A PDF whose header sits a little way in still opens — the guard must not be so
// strict that it locks users out of documents Nib can actually render.
func TestOpenAcceptsOffsetHeader(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	real, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	offset := filepath.Join(t.TempDir(), "offset.pdf")
	if err := os.WriteFile(offset, append([]byte("\n\n\n"), real...), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(openRequest{Path: offset})
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open(offset header) status = %d, want 200", resp.StatusCode)
	}
}

// The flags cache answers from memory when the bytes have not changed, and re-parses
// when they have.
//
// FlagsJSON is a full PDF parse (api.Properties), and docResponse is built by every
// document route's reply — /api/docs builds one per open document, so a boot restore with
// eight tabs parsed eight whole PDFs to answer a question none of them had changed. The
// cache is keyed on slice IDENTITY, which is only sound because doc.data is always
// replaced wholesale; this test is what would notice if a future writer started mutating
// it in place, because the stale answer shows up here as flags that did not change when
// the document did.
func TestDocResponseFlagsCacheInvalidatesWithTheBytes(t *testing.T) {
	_, srv := startServerWith(t)
	plain, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	withFlags, err := pdfops.SetFlags(plain, []byte(`[{"page":1,"kind":"marker"}]`))
	if err != nil {
		t.Fatal(err)
	}
	id := addDocument(srv, withFlags)

	srv.mu.Lock()
	var doc *document
	for _, d := range srv.docs {
		if d.id == id {
			doc = d
		}
	}
	srv.mu.Unlock()
	if doc == nil {
		t.Fatal("setup: the document was not registered")
	}

	first := srv.docResponse(doc)
	if len(first.Flags) == 0 {
		t.Fatal("setup: the crafted document reports no flags, so nothing below distinguishes a cache hit from a miss")
	}
	if got := srv.docResponse(doc); string(got.Flags) != string(first.Flags) {
		t.Errorf("a cached response disagrees with the parse it cached: %s vs %s", got.Flags, first.Flags)
	}
	srv.mu.Lock()
	populated := doc.flagsFor != nil
	srv.mu.Unlock()
	if !populated {
		t.Fatal("the cache was never populated, so this test is measuring the uncached path twice")
	}

	// Replace the bytes with a document carrying NO flags. A cache keyed on anything
	// weaker than the bytes themselves would keep reporting the old flags here.
	srv.mu.Lock()
	doc.data = plain
	srv.mu.Unlock()
	if got := srv.docResponse(doc); len(got.Flags) != 0 {
		t.Errorf("after the document's bytes were replaced with a flagless PDF, docResponse still reports flags %s — the cache did not invalidate", got.Flags)
	}
}

// Every response carries the security headers, and the CSP is the strict one.
//
// Asserted on a STATIC asset rather than an API route, because the asset handler is the
// one that serves the document-rendering page and was the finding's subject. The policy
// is spelled out rather than merely checked for presence: a CSP that had quietly gained
// 'unsafe-inline' or 'unsafe-eval' would still be "a CSP", and those two are exactly
// what the app was verified not to need.
func TestSecurityHeaders(t *testing.T) {
	ts, _ := startServerWith(t)
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	csp := res.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("no Content-Security-Policy on the static UI")
	}
	for _, want := range []string{
		"default-src 'self'", "object-src 'none'", "frame-ancestors 'none'",
		"base-uri 'none'", "style-src 'self'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("the CSP is missing %q; got %q", want, csp)
		}
	}
	for _, forbidden := range []string{"'unsafe-inline'", "'unsafe-eval'"} {
		if strings.Contains(csp, forbidden) {
			t.Errorf("the CSP contains %s — the app was verified not to need it, so this is a regression rather than a requirement: %q", forbidden, csp)
		}
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := res.Header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", got)
	}
}
