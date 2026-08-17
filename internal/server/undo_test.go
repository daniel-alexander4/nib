package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"nib/internal/testpdf"
)

// rotateDoc posts a rotate page-op with the given bytes as the working document
// (mirroring the client, which posts its baked bytes), returning the doc response.
func rotateDoc(t *testing.T, ts *httptest.Server, c *http.Client, csrf string, pdf []byte) docResponse {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("op", "rotate")
	mw.WriteField("deg", "90")
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d, want 200", resp.StatusCode)
	}
	var dr docResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	return dr
}

func postDoc(t *testing.T, ts *httptest.Server, c *http.Client, csrf, path string) docResponse {
	t.Helper()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+path, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d, want 200", path, resp.StatusCode)
	}
	var dr docResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	return dr
}

func fetchPDF(t *testing.T, ts *httptest.Server, c *http.Client) []byte {
	t.Helper()
	resp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b
}

// TestUndoRedoRoundTrip drives the full cycle over HTTP: a page op makes undo
// available, undo restores the exact pre-op bytes (and offers redo), redo
// re-applies them. Undo snapshots the operation's INPUT, so restoring it returns
// the document to precisely what the client posted.
func TestUndoRedoRoundTrip(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	original := fetchPDF(t, ts, c)

	dr := rotateDoc(t, ts, c, csrf, original)
	if !dr.CanUndo || dr.CanRedo {
		t.Fatalf("after op: canUndo=%v canRedo=%v, want true/false", dr.CanUndo, dr.CanRedo)
	}
	rotated := fetchPDF(t, ts, c)
	if bytes.Equal(rotated, original) {
		t.Fatal("rotate did not change the document")
	}

	dr = postDoc(t, ts, c, csrf, "/api/undo")
	if dr.CanUndo || !dr.CanRedo {
		t.Fatalf("after undo: canUndo=%v canRedo=%v, want false/true", dr.CanUndo, dr.CanRedo)
	}
	if got := fetchPDF(t, ts, c); !bytes.Equal(got, original) {
		t.Errorf("undo did not restore the original bytes (%d vs %d)", len(got), len(original))
	}

	dr = postDoc(t, ts, c, csrf, "/api/redo")
	if !dr.CanUndo || dr.CanRedo {
		t.Fatalf("after redo: canUndo=%v canRedo=%v, want true/false", dr.CanUndo, dr.CanRedo)
	}
	if got := fetchPDF(t, ts, c); !bytes.Equal(got, rotated) {
		t.Errorf("redo did not restore the rotated bytes (%d vs %d)", len(got), len(rotated))
	}
}

// TestUndoNoHistory proves undo/redo with nothing on the stack is a harmless
// no-op that returns the current state, not an error.
func TestUndoNoHistory(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	before := fetchPDF(t, ts, c)

	dr := postDoc(t, ts, c, csrf, "/api/undo")
	if dr.CanUndo || dr.CanRedo {
		t.Errorf("fresh doc should have no history, got canUndo=%v canRedo=%v", dr.CanUndo, dr.CanRedo)
	}
	if got := fetchPDF(t, ts, c); !bytes.Equal(got, before) {
		t.Error("undo with empty history changed the document")
	}
}

// TestOpenClearsHistory proves opening a new document wipes the undo history (you
// can't undo across a fresh open).
// TestOpenClearsHistory was DELETED here on 2026-08-17 (P06.S01) and replaced by
// TestOpenLeavesTheOtherDocumentsHistoryAlone in opencap_test.go.
//
// It opened the same file twice and asserted the second response reported no undo.
// That was a real claim while Open REPLACED the registry. Now that Open adds, the
// second open is a different document that never had a history, so the assertion
// cannot fail for the reason its name gives — it would stay green against an open
// that did nothing to history at all, and equally against one that wiped every
// other document's. Left as a note rather than silently removed, because a test
// that disappears in a diff looks like coverage lost rather than coverage re-aimed.

// TestCommitBarrierAndTrim covers the in-memory ring directly: commitMutation
// caps the undo depth (oldest evicted) and clears redo; commitBarrier wipes both
// stacks so a destructive op (redact/flatten) leaves nothing to resurrect.
func TestCommitBarrierAndTrim(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)

	// The rings live on the DOCUMENT now (P03.S04), so the assertions read the
	// document's own stacks. The depth cap is the single-document behaviour the
	// slice's premise pin promises is unchanged, so this test is also that clause's
	// evidence — it would go red if the budget's move to the undo+redo pair had
	// altered ordinary trimming.
	doc := s.activeDoc()
	for i := 0; i < maxUndoDepth+5; i++ {
		s.commitMutation(doc, pdf, pdf)
	}
	if len(doc.undo) != maxUndoDepth {
		t.Errorf("undo depth = %d, want %d (oldest evicted)", len(doc.undo), maxUndoDepth)
	}
	if len(doc.redo) != 0 {
		t.Errorf("commitMutation must clear redo, got %d", len(doc.redo))
	}

	s.commitBarrier(doc, pdf)
	if len(doc.undo) != 0 || len(doc.redo) != 0 {
		t.Errorf("barrier must clear history, got undo=%d redo=%d", len(doc.undo), len(doc.redo))
	}
	// A barrier is not an eviction. Reporting it as one would tell the user their
	// redaction cost them their undo history for memory reasons.
	if doc.historyEvicted {
		t.Error("commitBarrier marked the document as history-evicted")
	}
}
