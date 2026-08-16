package server

import (
	"bytes"
	"net/http"
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
