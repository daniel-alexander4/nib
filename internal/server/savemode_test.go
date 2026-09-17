package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/atomicfile"
)

// TestANewFileTheUserNamedIsCreatedAtTheSameModeWhicheverDoorWroteIt — /pending 572.
//
// Three doors call `atomicfile.ReplaceDurable` and its `perm` is reached for only when the path does
// not exist yet — which makes it invisible on every ordinary save and decisive on a Save As. Each
// named a literal alone: `internal/cli`'s `writeNamed` had always given a new file 0644 and both GUI
// doors passed 0600, so the same document landed with different permissions depending on the surface
// that wrote it. That is `/pending 570`'s split-part finding at a third door.
//
// **Asserted through the real route and then read off the FILESYSTEM**, because the mode is in no
// response: a user discovers it when something that could read their document stops being able to.
// `/pending 499` records that symptom in its own words — *"a document shared with a group or served
// by a local web server stopped being readable the moment Nib saved it, with no message."*
//
// **The second half is the guard on the first.** Widening a file the user had tightened would be the
// mirror of the defect being fixed, so the overwrite case is asserted in the same test rather than
// trusted to `ReplaceDurable`'s contract — this change edits the argument that decides it.
func TestANewFileTheUserNamedIsCreatedAtTheSameModeWhicheverDoorWroteIt(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	saveAs := func(name string) string {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("dir", dir)
		mw.WriteField("name", name)
		mw.WriteField("overwrite", "1") // the second case writes over a file it planted
		fw, _ := mw.CreateFormFile("data", name)
		fw.Write(data)
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
		out, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("write %s status %d: %s", name, resp.StatusCode, out)
		}
		return filepath.Join(dir, name)
	}

	// SETUP: the target really does not exist, or `ReplaceDurable` keeps the mode it finds and this
	// asserts the fixture's permissions rather than the door's decision.
	if _, serr := os.Stat(filepath.Join(dir, "saved.pdf")); !os.IsNotExist(serr) {
		t.Fatalf("setup: saved.pdf already exists (%v), so perm is never reached", serr)
	}
	fi, err := os.Stat(saveAs("saved.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != atomicfile.UserFileMode {
		t.Errorf("Save As created the user's document %v, want %v — `internal/cli`'s writeNamed has "+
			"always created a user-named file %v, so this door disagreeing with it means the same "+
			"document is readable or not depending on which surface wrote it",
			got, atomicfile.UserFileMode, atomicfile.UserFileMode)
	}

	tight := filepath.Join(dir, "tight.pdf")
	if err := os.WriteFile(tight, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, err = os.Stat(saveAs("tight.pdf")); err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("saving over a 0600 file left it %v — an overwrite keeps the replaced file's mode "+
			"(/pending 499), and widening one the user tightened is the mirror of the defect this "+
			"change fixes", got)
	}
}
