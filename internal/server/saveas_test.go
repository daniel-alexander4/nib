package server

import (
	"bytes"
	"encoding/json"
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
	mw.WriteField("path", target)
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
		if d == "nested" {
			found = true
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

	if len(info.Files) != 2 || info.Files[0] != "B.PDF" || info.Files[1] != "a.pdf" {
		t.Errorf("files = %v, want [B.PDF a.pdf] (case-insensitive .pdf, dotfiles hidden)", info.Files)
	}
	foundSub := false
	for _, d := range info.Dirs {
		if d == "sub" {
			foundSub = true
		}
	}
	if !foundSub {
		t.Errorf("dirs = %v, want it to include sub", info.Dirs)
	}
}

func TestWriteFileRejectsRelativePath(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("path", "relative/out.pdf")
	fw, _ := mw.CreateFormFile("data", "out.pdf")
	fw.Write([]byte("x"))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("write(relative) status = %d, want 400", resp.StatusCode)
	}
}
