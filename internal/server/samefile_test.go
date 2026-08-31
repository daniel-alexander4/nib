package server

import (
	"bytes"
	"os"
	"testing"
)

// /pending 339 — two tabs on one path, and until now nothing said so.
//
// `handleOpen` installs a second independent document for a path already open, with no
// `docForPath` check, while `handoff.go` states the opposite rule (D16) and enforces it.
// The exemption is CORRECT — an explicit Open is the user asking, a hand-off is the OS
// telling — and it is load-bearing for ADR-005, whose count cap is driven by opening one
// fixture path eight times. De-duplicating here breaks that criterion and five other
// tests. What was missing was the exemption being named, and the user being told.
//
// The entry's measured consequence — "both saves 200, the on-disk file is the
// PRE-redaction document" — was measured before /pending 333 landed and is no longer true
// of this tree: the second tab's save is refused. `TestASecondTabCannotSaveOverTheFirst`
// pins that, because 333's own rows drive an EXTERNAL rewrite and never this stimulus.

func TestOpeningAPathAlreadyOpenSaysSo(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	first := openByPath(t, ts.URL, c, csrf, path)
	// Asserted, and not merely as setup: reading `dup` AFTER the install would find the
	// document this very call added, so every open would report true. This is the arm
	// that catches that placement.
	if first.SameFileOpen {
		t.Fatal("the FIRST open of a path reports that the file is already open — dup is being read after the install, so the signal is true for every open and means nothing")
	}

	second := openByPath(t, ts.URL, c, csrf, path)
	if !second.SameFileOpen {
		t.Error("opening a path that is already open did not say so — the user gets two identically named tabs, two independent working copies, and no account of why")
	}
	if second.ID == first.ID {
		t.Errorf("both opens report id %s — the exemption is that this door ADDS a document; if it started de-duplicating, ADR-005's count cap could no longer be driven", first.ID)
	}
}

// The property /pending 333 established, driven through the stimulus that actually
// motivated /pending 339. 333's rows rewrite the file from OUTSIDE Nib; this is one tab
// legitimately saving and the other holding bytes from before it.
func TestASecondTabCannotSaveOverTheFirstsSave(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	a := openByPath(t, ts.URL, c, csrf, path)
	b := openByPath(t, ts.URL, c, csrf, path)

	aBytes := []byte("%PDF-1.7 tab A, redacted\n%%EOF\n")
	resp := writeDoc(t, c, csrf, ts.URL+"/api/save", a.ID, aBytes)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("tab A's save answered %d, want 200", resp.StatusCode)
	}
	if on, _ := os.ReadFile(path); !bytes.Equal(on, aBytes) {
		t.Fatal("setup: tab A's save did not reach the disk, so tab B has nothing to overwrite")
	}

	bBytes := []byte("%PDF-1.7 tab B, stale\n%%EOF\n")
	resp2 := writeDoc(t, c, csrf, ts.URL+"/api/save", b.ID, bBytes)
	resp2.Body.Close()
	if resp2.StatusCode == 200 {
		t.Error("tab B's save was accepted over tab A's — this is the redaction-restored case: B holds bytes from before A's save and puts them back")
	}
	on, _ := os.ReadFile(path)
	if !bytes.Equal(on, aBytes) {
		t.Error("the file no longer holds tab A's save — B's stale copy reached the disk, so whatever A removed is back")
	}
}
