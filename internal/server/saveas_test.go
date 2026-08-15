package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		"~":            home,
		"~/nib":        filepath.Join(home, "nib"),
		"~/a/b":        filepath.Join(home, "a/b"),
		"/abs/path":    "/abs/path",
		"~notuser/dir": "~notuser/dir", // only a bare ~ or ~/ expands
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteFileCreatesFolderAndListDir(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	// Write into a folder that doesn't exist yet — it should be created.
	dir := filepath.Join(t.TempDir(), "exports", "nested")
	target := filepath.Join(dir, "out.pdf")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("dir", dir)
	mw.WriteField("name", "out.pdf")
	fw, _ := mw.CreateFormFile("data", "out.pdf")
	fw.Write([]byte("%PDF-1.7 hello"))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("write status = %d, want 200", resp.StatusCode)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != "%PDF-1.7 hello" {
		t.Fatalf("written bytes = %q", got)
	}

	// listdir on the parent should now report the "nested" sub-folder.
	lr, err := c.Get(ts.URL + "/api/listdir?path=" + filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer lr.Body.Close()
	var info listDirResponse
	if err := json.NewDecoder(lr.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range info.Dirs {
		if d.Name == "nested" {
			found = true
			if d.Path != dir {
				t.Errorf("listdir built path = %q, want %q — the server owns the join", d.Path, dir)
			}
		}
	}
	if !found {
		t.Fatalf("listdir dirs = %v, want it to include %q", info.Dirs, "nested")
	}
}

func TestListDirReturnsPDFFiles(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)

	dir := t.TempDir()
	for _, name := range []string{"a.pdf", "B.PDF", "notes.txt", ".hidden.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	resp, err := c.Get(ts.URL + "/api/listdir?path=" + dir)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info listDirResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}

	if len(info.Files) != 2 || info.Files[0].Name != "B.PDF" || info.Files[1].Name != "a.pdf" {
		t.Errorf("files = %v, want [B.PDF a.pdf] (case-insensitive .pdf, dotfiles hidden)", info.Files)
	}
	if len(info.Files) == 2 && info.Files[1].Path != filepath.Join(dir, "a.pdf") {
		t.Errorf("file path = %q, want %q", info.Files[1].Path, filepath.Join(dir, "a.pdf"))
	}
	foundSub := false
	for _, d := range info.Dirs {
		if d.Name == "sub" {
			foundSub = true
		}
	}
	if !foundSub {
		t.Errorf("dirs = %v, want it to include sub", info.Dirs)
	}
	if info.Reason != "" {
		t.Errorf("reason = %q, want empty for a folder that listed fine", info.Reason)
	}
}

// A folder that can't be read must not pass for an empty one — that ambiguity is
// what made a denied share, a dead drive letter and a typo all look alike.
func TestListDirReportsWhyItIsEmpty(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)

	missing := filepath.Join(t.TempDir(), "no-such-folder")
	resp, err := c.Get(ts.URL + "/api/listdir?path=" + missing)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info listDirResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Reason != "missing" {
		t.Errorf("reason = %q, want %q", info.Reason, "missing")
	}

	// A file where a folder was asked for reads as "notdir", settled by a stat so
	// the answer is the same on every platform.
	file := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fr, err := c.Get(ts.URL + "/api/listdir?path=" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer fr.Body.Close()
	var finfo listDirResponse
	if err := json.NewDecoder(fr.Body).Decode(&finfo); err != nil {
		t.Fatal(err)
	}
	if finfo.Reason != "notdir" {
		t.Errorf("reason for a file = %q, want %q", finfo.Reason, "notdir")
	}
}

// Windows reports "you asked me to list a file" as ENOENT, where Linux reports
// ENOTDIR — so classifying on the errno first calls an existing file "missing"
// there, and a Linux-only test never notices because Linux answers correctly
// either way. Feed the Windows-shaped error explicitly to pin the ordering.
func TestListDirReasonPrefersStatOverErrno(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := listDirReason(file, fs.ErrNotExist); got != "notdir" {
		t.Errorf("listDirReason(file, ErrNotExist) = %q, want %q — this is the Windows shape", got, "notdir")
	}
	missing := filepath.Join(t.TempDir(), "nope")
	if got := listDirReason(missing, fs.ErrNotExist); got != "missing" {
		t.Errorf("listDirReason(absent, ErrNotExist) = %q, want %q", got, "missing")
	}
	if got := listDirReason(missing, fs.ErrPermission); got != "denied" {
		t.Errorf("listDirReason(absent, ErrPermission) = %q, want %q", got, "denied")
	}
	if got := listDirReason(missing, errors.New("disk on fire")); got != "unreadable" {
		t.Errorf("listDirReason(absent, other) = %q, want %q", got, "unreadable")
	}
}

// The save dialog's first open asks for ~/nib before it exists, by sending no
// path at all. That one case must stay quiet — the write step creates the folder.
func TestListDirDefaultFolderStaysQuiet(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)

	resp, err := c.Get(ts.URL + "/api/listdir")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info listDirResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Reason != "" {
		t.Errorf("reason = %q, want empty — the default folder not existing yet is expected", info.Reason)
	}
}

// A typed file name is free text, so "../" must not walk out of the folder the
// dialog says it is writing to. This escaped with a 200 before.
func TestWriteFileRejectsEscapingName(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	dir := filepath.Join(t.TempDir(), "chosen")
	escaped := filepath.Join(filepath.Dir(dir), "escaped.pdf")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("dir", dir)
	mw.WriteField("name", "../escaped.pdf")
	fw, _ := mw.CreateFormFile("data", "out.pdf")
	fw.Write([]byte("x"))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("write(../name) status = %d, want 400", resp.StatusCode)
	}
	if _, err := os.Stat(escaped); err == nil {
		t.Fatalf("%s was written — the name escaped the chosen folder", escaped)
	}
}

func TestWriteFileRejectsRelativePath(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("dir", "relative")
	mw.WriteField("name", "out.pdf")
	fw, _ := mw.CreateFormFile("data", "out.pdf")
	fw.Write([]byte("x"))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("write(relative) status = %d, want 400", resp.StatusCode)
	}
}
