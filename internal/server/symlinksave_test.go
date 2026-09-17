package server

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSaveThroughASymlinkWritesTheDocumentAndKeepsTheLink — /pending 515.
//
// **A rename replaces a symlink**, and every write door in `internal/atomicfile` ends in one. So
// opening `~/contract.pdf` — a link into a synced folder, which is how a one-machine user keeps a
// working name for a document that lives somewhere structured — and saving it produced a regular
// file where the link was, left the real document holding the bytes from before the edit, and said
// nothing. Two documents, and the one the user goes back to is the stale one.
//
// **Driven at `/api/save` rather than at the door**, because the door's own test cannot see which
// path this route hands it. `internal/atomicfile`'s tests own the resolution; this owns the fact
// that the GUI's save reaches it at all — and the fix is a change to `ReplaceDurable`, so a save
// route that went back to `WriteDurable` would pass every test in that package.
//
// The third assertion is the one that says where the resolution belongs: the document still reports
// the path the user opened. Resolving at OPEN would satisfy the first two and quietly replace her
// link with its target everywhere Nib shows a path — the tab, Open Recent, the Save As folder.
func TestSaveThroughASymlinkWritesTheDocumentAndKeepsTheLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symlink needs a privilege this test cannot assume on Windows")
	}
	ts, target := startServer(t)
	c, csrf := authedClient(t, ts)

	link := filepath.Join(t.TempDir(), "contract.pdf")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this filesystem will not make a symlink: %v", err)
	}
	// STIMULUS: the document really is opened through the link. Without this the save below
	// writes a plain path and every assertion here passes for the wrong reason.
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("setup: %s is not a symlink (%v)", link, err)
	}
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	dr := openByPath(t, ts.URL, c, csrf, link)
	// A valid PDF still, and different from what is on disk — `rewriteFile`'s own reasoning, minus
	// the write, because here the edit travels through the route rather than around it.
	edited := append(append([]byte{}, original...), []byte("\n% edited in Nib\n")...)
	if bytes.Equal(edited, original) {
		t.Fatal("setup: the saved bytes equal the file's own, so \"the target was written\" below is unfalsifiable")
	}
	resp := writeDoc(t, c, csrf, ts.URL+"/api/save", dr.ID, edited)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("save through a symlink: status = %d, want 200", resp.StatusCode)
	}

	if fi, lerr := os.Lstat(link); lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("saving replaced the user's symlink with a regular file (%v, %v) — the link she "+
			"opens the document by is gone", fi.Mode(), lerr)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, edited) {
		t.Error("the file the link points at still holds the bytes from before the save — the " +
			"user's edits went into a new file standing where her link was, and the document she " +
			"opens tomorrow is the stale one")
	}

	got := docByID(t, ts, c, dr.ID)
	if got.Path != link {
		t.Errorf("the document now reports %q, want the link the user opened (%q). Resolving a "+
			"link at open would write the right file and still take her chosen name away from "+
			"every place Nib prints a path", got.Path, link)
	}
	if got.DiskChanged {
		t.Error("the document reports diskChanged immediately after its own save through a link — " +
			"the baseline does not describe the file that was actually written, so the banner is " +
			"armed forever")
	}
}
