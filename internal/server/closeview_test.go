package server

import (
	"net/http"
	"testing"

	"nib/internal/testpdf"
)

// P06.S02 — closing ONE document.
//
// The property that matters is not "the closed document is gone" but "only the closed
// document is gone", and those fail apart: the shape this replaces (setDoc(nil)) also
// makes the first assertion true.

// TestCloseViewDropsOnlyTheAddressedDocument is the slice's central claim.
func TestCloseViewDropsOnlyTheAddressedDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	first := openByPath(t, ts.URL, c, csrf, path)
	second := openByPath(t, ts.URL, c, csrf, path)
	third := openByPath(t, ts.URL, c, csrf, path)
	// The stimulus. Without it, "two documents survive a close" is being asserted
	// against a registry that may never have held three.
	for i, d := range []docResponse{first, second, third} {
		if d.ID == "" {
			t.Fatalf("setup: open %d returned no id", i+1)
		}
	}

	resp := writeTo(t, c, csrf, ts.URL+"/api/close-view", second.ID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close-view status = %d, want 200", resp.StatusCode)
	}

	// The closed one is gone…
	if got := docByID(t, ts, c, second.ID); got.ID != "" {
		t.Errorf("the closed document is still addressable as %s", got.ID)
	}
	// …and the other two are NOT. This is the half that setDoc(nil) also passes on the
	// first assertion and fails here.
	for _, d := range []docResponse{first, third} {
		if got := docByID(t, ts, c, d.ID); got.ID != d.ID {
			t.Errorf("closing one document also dropped %s (got id %q) — close-view emptied the registry", d.ID, got.ID)
		}
	}
}

// TestCloseViewActivatesTheNeighbourAndSaysSo covers the wire half: the client learns
// which document is active from the response rather than computing it, so the response
// has to actually carry it.
func TestCloseViewActivatesTheNeighbourAndSaysSo(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	first := openByPath(t, ts.URL, c, csrf, path)
	second := openByPath(t, ts.URL, c, csrf, path)
	third := openByPath(t, ts.URL, c, csrf, path)
	// The third is active after three opens; assert it, or "the neighbour became
	// active" is being read against an unknown starting point.
	if active := docByID(t, ts, c, ""); active.ID != third.ID {
		t.Fatalf("setup: active is %s, want the last-opened %s", active.ID, third.ID)
	}

	// Close the middle one, which is NOT active. Nothing should move.
	resp := writeTo(t, c, csrf, ts.URL+"/api/close-view", second.ID)
	body := decodeDoc(t, resp)
	if body.ID != third.ID {
		t.Errorf("closing an inactive document reported %s as active, want the untouched %s", body.ID, third.ID)
	}
	if active := docByID(t, ts, c, ""); active.ID != third.ID {
		t.Errorf("closing an inactive document moved the active id to %s", active.ID)
	}

	// Now close the ACTIVE one. The registry is [first, third]; third is last, so the
	// neighbour is the previous — first.
	resp = writeTo(t, c, csrf, ts.URL+"/api/close-view", third.ID)
	body = decodeDoc(t, resp)
	if body.ID != first.ID {
		t.Errorf("closing the active document reported %s as active, want the neighbour %s", body.ID, first.ID)
	}
	if active := docByID(t, ts, c, ""); active.ID != first.ID {
		t.Errorf("the server's active id is %s, not the %s it reported — the client would follow a lie", active.ID, first.ID)
	}

	// And closing the last one leaves the zero response, not a dangling active id.
	resp = writeTo(t, c, csrf, ts.URL+"/api/close-view", first.ID)
	body = decodeDoc(t, resp)
	if body.ID != "" {
		t.Errorf("closing the last document reported %s as active", body.ID)
	}
}

// TestCloseViewIsRefusedForADocumentTheServerDoesNotHold — the 409 the client turns
// into "remove the tab anyway".
func TestCloseViewIsRefusedForADocumentTheServerDoesNotHold(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	only := openByPath(t, ts.URL, c, csrf, path)

	resp := writeTo(t, c, csrf, ts.URL+"/api/close-view", only.ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: the first close-view was refused (%d), so the second proves nothing", resp.StatusCode)
	}

	resp = writeTo(t, c, csrf, ts.URL+"/api/close-view", only.ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("closing an already-closed document = %d, want 409 — a stale id must be refused, not silently applied to whatever is active", resp.StatusCode)
	}
}

// TestCloseViewReleasesTheClosedDocumentsRings — the property setDoc's comment argues
// for and that a registry-only removal would leave to the garbage collector.
func TestCloseViewReleasesTheClosedDocumentsRings(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	keep := &document{data: pdf}
	s.mu.Lock()
	s.registerLocked(keep)
	s.mu.Unlock()
	doomed := &document{data: pdf}
	s.mu.Lock()
	s.registerLocked(doomed)
	s.mu.Unlock()

	s.commitMutation(doomed, pdf, pdf)
	s.commitMutation(keep, pdf, pdf)
	// The stimulus: both rings hold something, so an empty ring afterwards is a release
	// and not a ring that was never filled.
	if len(doomed.undo) == 0 || len(keep.undo) == 0 {
		t.Fatalf("setup: rings are empty (doomed %d, keep %d)", len(doomed.undo), len(keep.undo))
	}

	next := s.removeDoc(doomed)

	if len(doomed.undo) != 0 || len(doomed.redo) != 0 {
		t.Errorf("the closed document kept its history: undo=%d redo=%d — an in-flight handler still holding this pointer keeps whole PDFs alive", len(doomed.undo), len(doomed.redo))
	}
	if len(keep.undo) == 0 {
		t.Error("closing one document cleared ANOTHER document's history")
	}
	if next != keep {
		t.Errorf("the surviving document is not active after the close")
	}
}
