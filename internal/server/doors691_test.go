//go:build unix

package server

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestAPathThatIsNotARegularFileIsRefusedWithoutBeingRead — /pending 691 R6-5.
//
// A FIFO stats as size 0, so it cleared the size cap, and `os.ReadFile` on one with no writer never
// returns — under `setupMu`, from the hand-off drain. The read runs on a goroutine so that a build
// without the refusal fails here by name instead of hanging the package; the writer opened
// afterwards is only what lets that goroutine end.
func TestAPathThatIsNotARegularFileIsRefusedWithoutBeingRead(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "looks-like.pdf")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("SKIP (not a pass): this filesystem makes no FIFO: %v", err)
	}
	type answer struct {
		data []byte
		ref  *pathRefusal
	}
	done := make(chan answer, 1)
	go func() {
		data, _, ref := readInstallablePDF(fifo)
		done <- answer{data, ref}
	}()
	select {
	case a := <-done:
		if a.ref == nil {
			t.Fatalf("a FIFO was read as a document (%d bytes)", len(a.data))
		}
		if a.ref.kind != refuseUnopenable {
			t.Errorf("a FIFO was refused as kind %v (%q), want refuseUnopenable", a.ref.kind, a.ref.msg)
		}
	case <-time.After(3 * time.Second):
		// Release the blocked read: a writer that opens and closes gives it end-of-file.
		if w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
		t.Fatal("readInstallablePDF blocked reading a FIFO — it must refuse anything that is not a " +
			"regular file before it reads, because this runs under setupMu")
	}

	// CONTROL: a link to a regular file is still a file that opens (the stat follows links).
	real := filepath.Join(t.TempDir(), "real.pdf")
	if err := os.WriteFile(real, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.pdf")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("SKIP (not a pass): no symlinks here: %v", err)
	}
	if _, _, ref := readInstallablePDF(link); ref != nil && ref.kind == refuseUnopenable {
		t.Errorf("a link to a regular file was refused as unopenable: %q", ref.msg)
	}
}

// TestClosingADocumentLeavesNoPointerToOneInTheVacatedSlot — /pending 691 R6-7.
//
// Removing by `append(docs[:i], docs[i+1:]...)` left the backing array's old last slot pointing at
// the document that had just moved down, so closing that one as well kept its bytes reachable.
func TestClosingADocumentLeavesNoPointerToOneInTheVacatedSlot(t *testing.T) {
	s := &Server{epoch: "test-epoch"}
	a, b := &document{data: []byte("a")}, &document{data: []byte("b")}
	s.mu.Lock()
	s.registerLocked(a)
	s.registerLocked(b)
	s.mu.Unlock()

	s.removeDoc(a)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.docs) != 1 || s.docs[0] != b {
		t.Fatalf("setup: after closing the first of two, the registry is not the second alone (%d held)", len(s.docs))
	}
	if cap(s.docs) < 2 {
		t.Fatalf("setup: the backing array has no vacated slot to look at (cap %d)", cap(s.docs))
	}
	if stale := s.docs[:2][1]; stale != nil {
		t.Errorf("the vacated slot still points at a document (%p) — its bytes stay reachable after it is closed", stale)
	}
}
