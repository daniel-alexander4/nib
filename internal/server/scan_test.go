package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The scan endpoint reports active/hidden content for the open document and the
// sanitize endpoint removes it. This locks the wiring (route, auth, JSON shape,
// the ok/reload path) end to end; the per-method removal behaviour itself is
// covered by the pdfops tests. The form fixture carries no active content, so a
// strip is a clean no-op that must still validate and report ok.
func TestScanAndSanitizeWiring(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp, err := c.Get(ts.URL + "/api/scan")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scan status = %d, want 200", resp.StatusCode)
	}
	var rep struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		t.Fatalf("scan response not JSON: %v", err)
	}
	resp.Body.Close()

	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=strip", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sanitize status = %d, want 200", resp.StatusCode)
	}
	var out sanitizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("sanitize response not JSON: %v", err)
	}
	if !out.Ok {
		t.Error("strip of a clean document should report ok")
	}
}

// The metadata-strip method is wired through the same rail. The form fixture has
// no identifying metadata, so this is a clean no-op that must still report ok
// (the actual /Info //Metadata //ID removal is covered by the pdfops tests).
func TestSanitizeMetadataMethod(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=metadata", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metadata sanitize status = %d, want 200", resp.StatusCode)
	}
	var out sanitizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("sanitize response not JSON: %v", err)
	}
	if !out.Ok {
		t.Error("metadata strip should report ok")
	}
}

// An unknown removal method is a client error, not a silent no-op.
func TestSanitizeUnknownMethod(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=bogus", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown method status = %d, want 400", resp.StatusCode)
	}
}

// A residual that could not be scanned is not an empty one (`/pending 600`). The removal succeeds here — it
// takes out no attachments, which the file has none of — but Scan refuses the result's action graph as too
// large to walk honestly, and the handler used to ignore that error and answer ok with ZERO findings: the
// page's "Cleaned — nothing hidden remains" over a document nobody had looked at. It is not-ok now, and the
// open document is left as it was.
func TestSanitizeIsNotOkOverAResidualItCouldNotScan(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// Forty link annotations each naming the head of one 1000-wide /Next array: past Scan's action budget,
	// cheap for pdfcpu's validator (pdfops' TestManyAnnotationsNamingOneWideActionGraphAreRefused).
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
	}
	var annots, next []string
	for i := 0; i < 40; i++ {
		annots = append(annots, "<< /Type /Annot /Subtype /Link /Rect [0 0 1 1] /A 10 0 R >>")
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Annots [" + strings.Join(annots, " ") + "] >>"
	for i := 0; i < 1000; i++ {
		next = append(next, fmt.Sprintf("%d 0 R", 11+i))
		objs[11+i] = "<< /S /GoTo /D [3 0 R /Fit] >>"
	}
	objs[10] = "<< /S /GoTo /D [3 0 R /Fit] /Next [" + strings.Join(next, " ") + "] >>"
	doc := testpdf.Assemble(objs)
	if _, err := pdfops.Scan(doc); err == nil {
		t.Fatal("setup: Scan reads this document, so the handler's error branch is not reached")
	}
	path := filepath.Join(t.TempDir(), "wide.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, path)
	before := fetchPDF(t, ts, c)

	// Every removal now re-scans its own result (pdfops' verifyRemoved) and refuses here first; the handler's
	// own branch is the backstop for a removal that does not. Through either, the answer is not-ok.
	for _, method := range []string{"metadata", "safe"} {
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method="+method, "", nil)
		var out sanitizeResponse
		err := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil {
			t.Fatalf("%s: sanitize status = %d (%v), want 200", method, resp.StatusCode, err)
		}
		if out.Ok {
			t.Errorf("%s: sanitize reported ok with %d residual findings over a result Scan refused to read",
				method, len(out.Residual.Findings))
		}
	}
	if after := fetchPDF(t, ts, c); !bytes.Equal(before, after) {
		t.Error("a sanitize whose result could not be checked replaced the open document")
	}
}
