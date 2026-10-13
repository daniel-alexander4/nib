package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"
)

// TestADocumentNibCannotCompactSaysSoOnEveryAnswer — /pending 766 part 7.
//
// A write-path read that skips pdfcpu's optimize pass returned nil exactly as one that ran it, so a document
// written from it was larger than it needed to be and nothing between the read and the window could say so. The
// fact now rides the document's own answer (`unoptimized`), read by the parse the flags already use: at the
// open, on the cached answer for the same bytes, and on the answer for the bytes a change produced — which are
// the ones that were not compacted.
func TestADocumentNibCannotCompactSaysSoOnEveryAnswer(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	doc := testpdf.PassSkipped()
	path := filepath.Join(t.TempDir(), "skipped.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	if dr := openByPath(t, ts.URL, c, csrf, path); !dr.Unoptimized {
		t.Fatalf("the open's answer does not say the document is not compacted: %+v", dr)
	}
	if dr := getDoc(t, ts, c); !dr.Unoptimized {
		t.Error("a second answer for the same bytes — the cached one — no longer says it")
	}
	status, body := rotateXFA(t, c, csrf, ts.URL, doc)
	if status != http.StatusOK {
		t.Fatalf("a change to the document: %d %v", status, body)
	}
	if body["unoptimized"] != true {
		t.Errorf("after a change the answer says unoptimized=%v, want true — these are the bytes the pass "+
			"was skipped for", body["unoptimized"])
	}

	// And an ordinary document says nothing: the key is absent, not false.
	plain, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	plainPath := filepath.Join(t.TempDir(), "plain.pdf")
	if err := os.WriteFile(plainPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	if dr := openByPath(t, ts.URL, c, csrf, plainPath); dr.Unoptimized {
		t.Error("an ordinary document's answer says Nib cannot compact it")
	}
	if status, body := rotateXFA(t, c, csrf, ts.URL, plain); status != http.StatusOK || body["unoptimized"] != nil {
		t.Errorf("a change to an ordinary document: %d, unoptimized=%v, want the key absent", status, body["unoptimized"])
	}
}
