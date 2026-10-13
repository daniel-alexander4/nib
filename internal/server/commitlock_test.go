package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestTheCommitDoorsParseAndHashOutsideTheLock — /pending 581 and 628.
//
// Both commit doors read what an operation cost (two full parses) and the save hashed the whole
// document, all while holding `s.mu` — measured, every other route waited 93 ms behind a 50 MB
// commit and 1.16 s behind a 200 MB one, and 114 ms behind a 200 MB save. Each must do that work
// with the lock free, and still record what it found.
func TestTheCommitDoorsParseAndHashOutsideTheLock(t *testing.T) {
	noted := notedFixture(t, 3)
	plain, err := testpdf.Text("a page")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "d.pdf")
	if err := os.WriteFile(path, noted, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	doc := newPathDoc(path, noted)
	s.mu.Lock()
	s.registerLocked(doc)
	s.mu.Unlock()

	calls, held := 0, 0
	look := func() {
		calls++
		if s.mu.TryLock() {
			s.mu.Unlock()
		} else {
			held++
		}
	}
	savedInspect, savedStamp := inspectFacts, saveStamp
	t.Cleanup(func() { inspectFacts, saveStamp = savedInspect, savedStamp })
	inspectFacts = func(b []byte) pdfops.DocumentFacts { look(); return pdfops.Inspect(b) }
	saveStamp = func(p string, b []byte) *diskState { look(); return stampFor(p, b) }

	check := func(door string, wantCalls int) {
		t.Helper()
		if calls != wantCalls {
			t.Fatalf("setup: the %s made %d parse/hash calls, want %d — \"never under the lock\" is true of a door that never did the work", door, calls, wantCalls)
		}
		if held != 0 {
			t.Errorf("the %s parsed or hashed the document while holding s.mu (%d of %d calls) — every other route waits for it", door, held, calls)
		}
		calls, held = 0, 0
	}

	if err := s.commitMutation(doc, snapshotBase(s.docBytes(doc)), plain, false); err != nil {
		t.Fatal(err)
	}
	check("commitMutation", 2)
	if doc.lostAnnots != 3 {
		t.Errorf("commitMutation recorded %d lost annotations, want 3 — the cost was read and not recorded", doc.lostAnnots)
	}

	rec := httptest.NewRecorder()
	s.handleSave(rec, httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(notedFixture(t, 2))))
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	check("save", 1)
	if disk := s.diskOf(doc); disk == nil || diskChanged(path, disk) {
		t.Error("after a save the document reports its own write as a change on disk — the stamp built outside the lock was not recorded")
	}

	if err := s.commitBarrier(doc, plain, false); err != nil {
		t.Fatal(err)
	}
	check("commitBarrier", 2)
	if doc.lostAnnots != 5 {
		t.Errorf("after the barrier the document records %d lost annotations, want 5 (3 + 2)", doc.lostAnnots)
	}
}

// TestABarrierRecordsItsCostAgainstTheBytesItReplaced is the other half of moving the barrier's
// parses out of the lock: the document can move while they run. Here another commit lands during the
// first parse, so the cost in hand was read against bytes the barrier did not replace. It must be
// read again against what the document held at the commit — recording the stale one tells the user
// three comments were lost when six were.
func TestABarrierRecordsItsCostAgainstTheBytesItReplaced(t *testing.T) {
	three, six := notedFixture(t, 3), notedFixture(t, 6)
	plain, err := testpdf.Text("a page")
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, three)
	doc := s.activeDoc()

	saved := inspectFacts
	t.Cleanup(func() { inspectFacts = saved })
	moved := false
	inspectFacts = func(b []byte) pdfops.DocumentFacts {
		// TryLock, so a barrier that parses under the lock fails the assertion below rather than
		// deadlocking here.
		if !moved && s.mu.TryLock() {
			moved = true
			doc.install(six, doc.sig)
			s.mu.Unlock()
		}
		return pdfops.Inspect(b)
	}
	if err := s.commitBarrier(doc, plain, false); err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("the barrier never read its cost with the lock free, so no other commit could land during it")
	}
	if doc.lostAnnots != 6 {
		t.Errorf("the barrier replaced a document holding 6 comments and recorded %d lost — the cost "+
			"read against the bytes before the other commit was recorded beside the ones it replaced", doc.lostAnnots)
	}
}

// TestABarrierWhoseDocumentKeepsMovingStillCommits holds the loop's bound: when every capture is
// overtaken, the barrier reads its cost under the lock rather than spin, and what it records is still
// the cost against the bytes it replaced.
func TestABarrierWhoseDocumentKeepsMovingStillCommits(t *testing.T) {
	six := notedFixture(t, 6)
	plain, err := testpdf.Text("a page")
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, notedFixture(t, 3))
	doc := s.activeDoc()

	saved := inspectFacts
	t.Cleanup(func() { inspectFacts = saved })
	outside, under := 0, 0
	inspectFacts = func(b []byte) pdfops.DocumentFacts {
		if s.mu.TryLock() {
			outside++
			doc.install(append([]byte(nil), six...), doc.sig) // another commit, every time
			s.mu.Unlock()
		} else {
			under++
		}
		return pdfops.Inspect(b)
	}
	if err := s.commitBarrier(doc, plain, false); err != nil {
		t.Fatal(err)
	}
	if outside != 2*barrierFateAttempts || under != 2 {
		t.Errorf("the barrier parsed %d times outside the lock and %d under it, want %d and 2 — the "+
			"named fallback after %d overtaken captures", outside, under, 2*barrierFateAttempts, barrierFateAttempts)
	}
	if !sameSlice(s.docBytes(doc), plain) {
		t.Error("the barrier did not commit")
	}
	if doc.lostAnnots != 6 {
		t.Errorf("the barrier recorded %d lost comments, want the 6 the replaced bytes held", doc.lostAnnots)
	}
}
