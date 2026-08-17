package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"
)

// P04.S01's headline acceptance, and the only place in this plan where the failure is
// a FILE ON DISK rather than a response body.
//
// The scenario, which is ordinary rather than exotic: the user saves a large document,
// the bake takes a few seconds, and while it runs the open document changes — by
// opening another file, or by a co-signature arriving. Every read of the live document
// after that point describes a different document than the bytes came from. `/api/save`
// writes the posted bytes to the **addressed** document's path, so without a captured
// id the request is addressed to whatever is now current and document A's contents land
// in document B's file. Past the signature guard, with a success reply.
//
// This test asserts the two halves separately, because they fail independently: the
// addressed file is written, AND the other file is byte-identical to what it was.
func TestSaveGoesToTheCapturedDocumentNotTheCurrentOne(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.pdf")
	pathB := filepath.Join(dir, "b.pdf")
	originalB := append([]byte("B-ORIGINAL-"), pdf...)
	if err := os.WriteFile(pathA, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, originalB, 0o600); err != nil {
		t.Fatal(err)
	}

	// A is opened first and its id captured — this is what the client does before its
	// first await.
	docA := srv.setDoc(&document{path: pathA, data: pdf})
	capturedA := docA.id

	// ...then the document changes underneath, exactly as an arrival or a new open does.
	docB := srv.addDoc(&document{path: pathB, data: originalB})
	if srv.activeDoc() != docB {
		t.Fatal("setup: B is not the active document, so there is no switch to survive")
	}
	if capturedA == docB.id {
		t.Fatal("setup: the two documents share an id")
	}

	// The save carries A's bytes and A's CAPTURED id, while B is active.
	saved := append([]byte("A-EDITED-"), pdf...)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/save", bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Nib-Doc", capturedA.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pinned save = %d, want 200", resp.StatusCode)
	}

	// The stimulus, asserted before the response: A must actually have been written, or
	// "B is untouched" is satisfied by a save that did nothing at all.
	gotA, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotA, saved) {
		t.Fatal("the captured document's file was not written — the assertion below would pass against a no-op")
	}

	// And the clause itself: B's file is byte-identical to what it was.
	gotB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotB, originalB) {
		t.Errorf("the save landed in the OTHER document's file (%d bytes -> %d) — this is document A's contents overwriting document B, which is the corruption operation pinning exists to prevent",
			len(originalB), len(gotB))
	}
}

// The other half of the captured-id contract, and the reason ADR-001's never-reuse law
// is what makes this phase safe rather than merely tidier: when the captured document is
// GONE, the operation is refused. It is not redirected at whatever is current, and — the
// part the law buys — it cannot be redirected at whatever inherited the number, because
// nothing ever inherits a number.
func TestSaveToAClosedDocumentIsRefusedNotRedirected(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.pdf")
	pathB := filepath.Join(dir, "b.pdf")
	originalB := append([]byte("B-ORIGINAL-"), pdf...)
	if err := os.WriteFile(pathA, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, originalB, 0o600); err != nil {
		t.Fatal(err)
	}

	docA := srv.setDoc(&document{path: pathA, data: pdf})
	capturedA := docA.id

	// setDoc REPLACES, so A is now closed and its id names nothing. This is the
	// ordinary case: the user opened another file while the save was baking.
	srv.setDoc(&document{path: pathB, data: originalB})

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/save", bytes.NewReader([]byte("A-EDITED")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Nib-Doc", capturedA.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("save to a closed document = %d, want 409 — anything else means the bytes went somewhere", resp.StatusCode)
	}
	gotB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotB, originalB) {
		t.Error("the refused save still wrote to the now-current document's file")
	}
}

// TestSaveRefusesADocumentClosedWhileTheBodyWasRead covers the window the test
// above cannot reach. There, the id already named nothing when the request
// arrived, so resolveDoc refused it. Here the document is live at resolveDoc and
// closed by the time the bytes have been read — which is the ordinary case for a
// large save, because the body read is the slow part.
//
// commitMutation's contract names this caller: "a caller that tested the document
// first would leave a window for a close to land in between, which is the very
// defect". handleSave does not route through commitMutation, so it inherited the
// defect and not the guard, and answered 200 with an EMPTY docResponse — a success
// reply for discarded work — after writing the file.
//
// The handler is invoked DIRECTLY rather than over a client, and that is the point
// of the test rather than a shortcut. Two earlier shapes both passed with the
// guards removed: sequencing the close on a pipe write raced the dispatch and
// closed the document before resolveDoc (a 409 for an entirely different reason),
// and hanging the hook on the request body failed because a client-side body is
// read by the CLIENT as it transmits, so it fired before the server saw anything.
// Only a server-side body puts the close strictly between resolveDoc and the
// write. Both false greens said "refused" and neither had been near the defect.
func TestSaveRefusesADocumentClosedWhileTheBodyWasRead(t *testing.T) {
	_, srv := startServerWith(t)

	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "doc.pdf")
	original := append([]byte("ON-DISK-ORIGINAL-"), pdf...)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	doc := srv.setDoc(&document{path: path, data: pdf})

	body := &closeOnFirstRead{data: append([]byte("EDITED-"), pdf...), closeIt: func() { srv.setDoc(nil) }}
	req := httptest.NewRequest(http.MethodPost, "/api/save", body)
	req.Header.Set("X-Nib-Doc", doc.id.String())
	rec := httptest.NewRecorder()

	srv.handleSave(rec, req)

	if !body.read {
		t.Fatal("the handler never read the body, so the window this test exists for was never entered")
	}
	if rec.Code == http.StatusOK {
		t.Errorf("save into a document closed mid-read answered 200 — a success reply for discarded work")
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	// And it must not have touched the disk: the document is gone, so this is an
	// operation against something the server no longer holds.
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, original) {
		t.Errorf("the file was written for a closed document: now %d bytes, want the original %d", len(onDisk), len(original))
	}
}

// closeOnFirstRead delivers a request body and runs closeIt the first time the
// handler reads from it. handleSave calls resolveDoc before io.ReadAll, so the
// first Read is strictly after the document was resolved and strictly before the
// file is written — the window, entered deterministically.
type closeOnFirstRead struct {
	data    []byte
	off     int
	read    bool
	closeIt func()
}

func (c *closeOnFirstRead) Read(p []byte) (int, error) {
	if !c.read {
		c.read = true
		c.closeIt()
	}
	if c.off >= len(c.data) {
		return 0, io.EOF
	}
	n := copy(p, c.data[c.off:])
	c.off += n
	return n, nil
}
