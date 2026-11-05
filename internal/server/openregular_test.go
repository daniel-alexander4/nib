package server

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"nib/internal/pdfops"
)

// TestAFileNibMayNotReadIsA403NotA500 — /pending 940. A path whose mode forbids the read used to pass the
// stat and fail the later open as "could not read file" (500); it is permission, and says so.
func TestAFileNibMayNotReadIsA403NotA500(t *testing.T) {
	p := filepath.Join(t.TempDir(), "locked.pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(p); err == nil {
		f.Close()
		t.Skip("SKIP (not a pass): this user reads a mode-0 file (root), so the refusal cannot be provoked")
	}
	_, ref := readInstallablePDF(context.Background(), p, pdfops.ImportOptions{Door: "path"})
	if ref == nil || ref.status != http.StatusForbidden || ref.kind != refuseUnreadable {
		t.Fatalf("a mode-0 file: %+v, want 403 refuseUnreadable", ref)
	}
	// CONTROL: a missing path is still a 404, so the 403 is about permission and not about every open error.
	_, ref = readInstallablePDF(context.Background(), filepath.Join(filepath.Dir(p), "absent.pdf"), pdfops.ImportOptions{Door: "path"})
	if ref == nil || ref.status != http.StatusNotFound {
		t.Fatalf("a missing path: %+v, want 404", ref)
	}
}

// TestOpeningAFIFOCannotBlock — /pending 940. The refusal is asked of the OPENED descriptor, so the open itself
// must not wait for a writer: a FIFO named at the moment between a stat and an open used to hang here.
func TestOpeningAFIFOCannotBlock(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "swapped-in.pdf")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("SKIP (not a pass): this filesystem makes no FIFO: %v", err)
	}
	done := make(chan *pathRefusal, 1)
	go func() {
		f, _, ref := openRegular(fifo)
		if f != nil {
			f.Close()
		}
		done <- ref
	}()
	select {
	case ref := <-done:
		if ref == nil || ref.kind != refuseUnopenable {
			t.Fatalf("a FIFO opened: %+v, want refuseUnopenable", ref)
		}
	case <-time.After(3 * time.Second):
		if w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
		t.Fatal("openRegular blocked opening a FIFO with no writer")
	}
}

// An open that failed for the machine's reasons is not "file not found" (/pending 946 I9): no descriptors left and an I/O
// error are a 500 that names the open; the path-shaped errors stay 404, and permission stays 403.
func TestAFailedOpenIsAnsweredByWhatFailed(t *testing.T) {
	for name, c := range map[string]struct {
		err    error
		status int
		kind   refusalKind
	}{
		"missing":             {syscall.ENOENT, 404, refuseUnopenable},
		"parent is a file":    {syscall.ENOTDIR, 404, refuseUnopenable},
		"loop of links":       {syscall.ELOOP, 404, refuseUnopenable},
		"name too long":       {syscall.ENAMETOOLONG, 404, refuseUnopenable},
		"permission":          {syscall.EACCES, 403, refuseUnreadable},
		"no descriptors left": {syscall.EMFILE, 500, refuseUnreadable},
		"system table full":   {syscall.ENFILE, 500, refuseUnreadable},
		"i/o error":           {syscall.EIO, 500, refuseUnreadable},
	} {
		ref := openRefusal(&fs.PathError{Op: "open", Path: "p", Err: c.err})
		if ref.status != c.status || ref.kind != c.kind {
			t.Errorf("%s: %d kind %v, want %d kind %v", name, ref.status, ref.kind, c.status, c.kind)
		}
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	if _, _, ref := openRegular(a); ref == nil || ref.status != 404 {
		t.Errorf("a loop of links through openRegular: %+v, want 404", ref)
	}
}
