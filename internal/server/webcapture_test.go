package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// /pending 889 part 2 at the routes: a document whose Web Capture record Nib could not read whole says so at the
// open, every change is refused naming it and saying it can go, the sanitize route answers that refusal instead of
// "try the next method", and the record's removal lets the change through.
func TestAWebCaptureRecordIsAnnouncedRefusedByNameAndRemovable(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	w := testpdf.WholeWebCapture
	w.Refused = true
	doc := testpdf.WebCaptured(w)
	path := filepath.Join(t.TempDir(), "captured.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	dr := openByPath(t, ts.URL, c, csrf, path)
	if len(dr.UnreadLists) != 1 || dr.UnreadLists[0] != "URLS" || dr.UnreadWhat != pdfread.WebCaptureWords {
		t.Fatalf("the open answered unreadLists %v, unreadWhat %q — want [URLS] and %q", dr.UnreadLists, dr.UnreadWhat, pdfread.WebCaptureWords)
	}

	rotate := func() (int, map[string]string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(fetchPDF(t, ts, c)) // the route takes the document's bytes, as the window sends them
		mw.WriteField("op", "rotate")
		mw.WriteField("deg", "90")
		mw.WriteField("pages", "1")
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
		defer resp.Body.Close()
		var got map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&got)
		return resp.StatusCode, got
	}
	if code, got := rotate(); code != http.StatusUnprocessableEntity || got["cause"] != "name-tree" || got["removable"] != "webcapture" ||
		!strings.Contains(got["error"], pdfread.WebCaptureWords) {
		t.Errorf("a rotation: %d %v — want 422, cause name-tree, removable webcapture, the record named", code, got)
	}

	// Another removal is refused for the same reason — said, not reported as a method that failed.
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=strip", "", nil)
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || got["cause"] != "name-tree" {
		t.Errorf("a strip: %d %v — want 422 with cause name-tree, not {ok:false}", resp.StatusCode, got)
	}

	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=webcapture", "", nil)
	var out sanitizeResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !out.Ok {
		t.Fatalf("the removal: %d ok=%v", resp.StatusCode, out.Ok)
	}
	if len(out.UnreadLists) != 0 || out.UnreadWhat != "" {
		t.Errorf("after the removal the answer still says unreadLists %v, unreadWhat %q", out.UnreadLists, out.UnreadWhat)
	}
	for _, f := range out.Residual.Findings {
		if f.Kind == "webCapture" {
			t.Errorf("the residual still lists the record: %+v", f)
		}
	}
	if code, got := rotate(); code != http.StatusOK {
		t.Errorf("a rotation after the removal: %d %v — want 200", code, got)
	}
}
