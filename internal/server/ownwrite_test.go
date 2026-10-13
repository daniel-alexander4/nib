package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestASaveRefusedLateAtTheCapDoesNotMakeTheNextSaveA412 — /pending 784 part 1.
//
// A save writes the file and THEN makes the authoritative cap test under the lock. Refused there —
// another document grew while the file was being written — the bytes are on disk and the document
// keeps the ones it had, with its baseline deliberately left alone so the banner says the file
// differs from what is open. But the next save was then refused 412 "this file has changed on disk
// since Nib opened it" about the file Nib itself had just written. Nib remembers its own write, and
// the 412 test accepts it; a change by anyone ELSE after that is still refused.
func TestASaveRefusedLateAtTheCapDoesNotMakeTheNextSaveA412(t *testing.T) {
	opened, err := testpdf.Text("as opened")
	if err != nil {
		t.Fatal(err)
	}
	edited, err := testpdf.Text("as edited and saved")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "d.pdf")
	if err := os.WriteFile(path, opened, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	doc := newPathDoc(path, opened)
	other := &document{data: []byte("%PDF-1.4 another open document")}
	s.mu.Lock()
	s.registerLocked(other)
	s.registerLocked(doc)
	s.mu.Unlock()
	s.maxDocBytes = len(edited) + len(other.data) + 64

	save := func(body []byte) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handleSave(rec, httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body)))
		return rec
	}

	// The other document grows after this save's first cap test and before its last: the stamp is
	// built between the two, so the hook is the window.
	saved := saveStamp
	t.Cleanup(func() { saveStamp = saved })
	small := other.data
	saveStamp = func(p string, b []byte) *diskState {
		s.mu.Lock()
		other.install(make([]byte, len(small)+4096), other.sig)
		s.mu.Unlock()
		return stampFor(p, b)
	}
	rec := save(edited)
	saveStamp = saved
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "the file was saved") {
		t.Fatalf("setup: the save was not refused late at the cap: %d %s", rec.Code, rec.Body)
	}
	if onDisk, _ := os.ReadFile(path); !bytes.Equal(onDisk, edited) {
		t.Fatal("setup: the refused save's bytes are not on disk")
	}
	if !bytes.Equal(s.docBytes(doc), opened) {
		t.Fatal("the document took bytes the cap refused")
	}
	// The banner stays true: the file is not what is open.
	if !s.docResponse(doc).DiskChanged {
		t.Error("after a save the document could not take, it reports the file as matching what is open")
	}

	// Room again. The next save is of the same file Nib wrote — not a change by anyone else.
	s.mu.Lock()
	other.install(small, other.sig)
	s.mu.Unlock()
	if rec = save(edited); rec.Code != http.StatusOK {
		t.Fatalf("the save after a late cap refusal answered %d %s — refused about Nib's own write", rec.Code, rec.Body)
	}
	if s.docResponse(doc).DiskChanged {
		t.Error("after the save landed the document still reports the file as changed")
	}
	s.mu.Lock()
	leftover := doc.wrote
	s.mu.Unlock()
	if leftover != nil {
		t.Error("the save landed and the remembered own write was kept — a later change by another hand matching nothing would be measured against a stale stamp")
	}
}

// TestAfterALateCapRefusalAChangeByAnotherHandIsStillRefused is the control for the test above: what
// the 412 test accepts is the file Nib wrote, not any file.
func TestAfterALateCapRefusalAChangeByAnotherHandIsStillRefused(t *testing.T) {
	opened, _ := testpdf.Text("as opened")
	edited, _ := testpdf.Text("as edited and saved")
	theirs, _ := testpdf.Text("written by another program")
	path := filepath.Join(t.TempDir(), "d.pdf")
	if err := os.WriteFile(path, opened, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	doc := newPathDoc(path, opened)
	s.mu.Lock()
	s.registerLocked(doc)
	// What a late cap refusal leaves: Nib's write on disk, remembered, the baseline untouched.
	s.mu.Unlock()
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	doc.wrote = stampFor(path, edited)
	s.mu.Unlock()
	if s.changedByAnother(doc) {
		t.Fatal("setup: Nib's own write reads as another's change")
	}

	if err := os.WriteFile(path, theirs, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleSave(rec, httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(edited)))
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("a save over a file another program rewrote answered %d %s, want 412", rec.Code, rec.Body)
	}
	if onDisk, _ := os.ReadFile(path); !bytes.Equal(onDisk, theirs) {
		t.Error("the refused save replaced the other program's file")
	}
}
