package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// /pending 340 — Save-As replaced whatever was already at that name, silently.
//
// `handleWriteFile` wrote to `target` with no existence check: typing the name of a file
// already in the folder replaced it, answered 200, and the client toasted "Saved to
// <path>". The Save dialog does not list the destination folder's existing PDFs either,
// so the collision was unseeable rather than merely unconfirmed — a signed original was
// one filename away from gone.
//
// It is also the door /pending 333's precondition does not reach: this route resolves no
// document, so a Save-As onto an OPEN document's own path walks past handleSave's 412
// and past ceremonyFreeze.

func writeFileReq(t *testing.T, ts *httptest.Server, c *http.Client, csrf, dir, name string, data []byte, overwrite bool) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("dir", dir)
	_ = mw.WriteField("name", name)
	if overwrite {
		_ = mw.WriteField("overwrite", "1")
	}
	fw, err := mw.CreateFormFile("data", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	return write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
}

func TestWriteRefusesToOverwriteAnExistingFile(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	precious := []byte("%PDF-1.7 THE SIGNED ORIGINAL")
	target := filepath.Join(dir, "precious.pdf")
	if err := os.WriteFile(target, precious, 0o600); err != nil {
		t.Fatal(err)
	}

	resp := writeFileReq(t, ts, c, csrf, dir, "precious.pdf", []byte("%PDF-1.7 whatever"), false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("Save-As onto an existing name answered %d, want 412", resp.StatusCode)
	}

	// **The assertion is on the BYTES, not the status.** A refusal that answers 412 after
	// the write has already landed satisfies a status-only check while losing exactly as
	// much of the user's file as no check at all — the same shape as /pending 333's
	// placement row.
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, precious) {
		t.Error("the write reached the disk anyway: the existing file no longer holds its own bytes, so the refusal happened after the data was already gone")
	}
}

func TestWriteOverwritesWhenTheUserSaysSo(t *testing.T) {
	// The refusal is a default, not a wall: replacing is often exactly what a re-export
	// means. Without this the user is stranded with no in-app way to write the file.
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	target := filepath.Join(dir, "precious.pdf")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	mine := []byte("%PDF-1.7 the replacement")
	resp := writeFileReq(t, ts, c, csrf, dir, "precious.pdf", mine, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overwrite=1 answered %d, want 200", resp.StatusCode)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(after, mine) {
		t.Error("overwrite=1 answered 200 without writing the user's bytes")
	}
}

func TestWriteStillCreatesANewFile(t *testing.T) {
	// The positive control, and it is what stops the guard above from being satisfied by a
	// route that refuses everything. An ordinary export to a fresh name must still work —
	// and this also proves the guard is `err == nil` rather than the inverted spelling.
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	resp := writeFileReq(t, ts, c, csrf, dir, "fresh.pdf", []byte("%PDF-1.7 new"), false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("an ordinary Save-As to a fresh name answered %d, want 200", resp.StatusCode)
	}
	var got map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["path"] != filepath.Join(dir, "fresh.pdf") {
		t.Errorf("path = %q, want the joined target", got["path"])
	}
}

// The crossing case, and the reason 340 is worth fixing beyond the general one: this is
// where /pending 339's data loss still actually lived after 333 closed the /api/save door.
func TestWriteWillNotSilentlyReplaceAnOpenDocumentsFile(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	if !dr.CanSave {
		t.Fatal("setup: the opened document reports no path, so it has no file to replace")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	resp := writeFileReq(t, ts, c, csrf, filepath.Dir(path), filepath.Base(path), []byte("%PDF-1.7 foreign"), false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("Save-As onto an OPEN document's own path answered %d, want 412 — this route resolves no document, so it walks past handleSave's precondition and past the ceremony freeze", resp.StatusCode)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, before) {
		t.Error("the open document's file was replaced through /api/write while its tab still showed the old bytes")
	}
}
