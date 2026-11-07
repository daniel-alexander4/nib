package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// The save dialogs start in the open document's own folder (ADR-214). Three halves: the server names
// the folder (`docResponse.Dir`), `listdir` vets it (`start=1`), and the window asks for it through one
// door (test/jsdom/startfolder.test.mjs).

// getListDir asks /api/listdir the way the dialog does and returns the answer.
func getListDir(t *testing.T, base string, c *http.Client, path, start string) listDirResponse {
	t.Helper()
	resp, err := c.Get(base + "/api/listdir?path=" + url.QueryEscape(path) + "&start=" + start)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info listDirResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	return info
}

// TestADocumentNamesTheFolderItCameFrom — a PDF opened by path, a photo converted at the same door, and
// an upload. The photo is the case a client-side dirname would have lost: it installs pathless (so a Save
// cannot destroy the picture) and its folder lives only in `srcPath`.
func TestADocumentNamesTheFolderItCameFrom(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)

	if got := openByPath(t, ts.URL, c, csrf, pdfPath); got.Dir != filepath.Dir(pdfPath) {
		t.Errorf("a PDF opened from %s names its folder %q, want %q", pdfPath, got.Dir, filepath.Dir(pdfPath))
	}

	dir := t.TempDir()
	photo := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(photo, testPNG(t, 24, 16), 0o600); err != nil {
		t.Fatal(err)
	}
	got := openByPath(t, ts.URL, c, csrf, photo)
	if got.Path != "" || got.CanSave {
		t.Fatalf("a converted photo reports path %q canSave %v, want neither", got.Path, got.CanSave)
	}
	if got.Dir != dir {
		t.Errorf("a converted photo opened from %s names its folder %q, want %q", dir, got.Dir, dir)
	}

	// /api/docs is what a reload reads: the folder must survive it.
	docs := fetchDocs(t, ts, c)
	found := false
	for _, d := range docs.Docs {
		if d.ID == got.ID {
			found = true
			if d.Dir != dir {
				t.Errorf("/api/docs lists the photo with folder %q, want %q", d.Dir, dir)
			}
		}
	}
	if !found {
		t.Fatal("the converted photo is not in /api/docs")
	}

	up := uploadForBody(t, c, csrf, ts.URL, "photo.png", testPNG(t, 24, 16))
	var upl docResponse
	if err := json.Unmarshal([]byte(up), &upl); err != nil {
		t.Fatal(err)
	}
	if upl.Dir != "" {
		t.Errorf("an upload names folder %q; the browser never reveals one, so it must be empty", upl.Dir)
	}
}

// TestAConvertedHandOffKeepsItsFolder — the second constructor that has a source path (handoff.go).
func TestAConvertedHandOffKeepsItsFolder(t *testing.T) {
	ts, srv := startServerWith(t)
	_ = ts
	dir := t.TempDir()
	photo := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(photo, testPNG(t, 24, 16), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.openHandedOff(context.Background(), photo); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	doc := srv.docs[len(srv.docs)-1]
	srv.mu.Unlock()
	if got := srv.docResponse(doc).Dir; got != dir {
		t.Errorf("a double-clicked photo names folder %q, want %q", got, dir)
	}
}

// TestAFolderIsOfferedOnlyWhenItIsAbsolute — a relative path would list against the server's working
// directory and `destDir` refuses it at the write, so it is no folder at all.
func TestAFolderIsOfferedOnlyWhenItIsAbsolute(t *testing.T) {
	for _, c := range []struct {
		d    document
		want string
	}{
		{document{path: "/a/b/c.pdf"}, "/a/b"},
		{document{srcPath: "/a/b/c.png"}, "/a/b"},
		{document{path: "/a/b/c.pdf", srcPath: "/x/y.png"}, "/a/b"},
		{document{path: "c.pdf"}, ""},
		{document{srcPath: "rel/c.png"}, ""},
		{document{}, ""},
	} {
		if got := c.d.folder(); got != c.want {
			t.Errorf("folder() of path %q srcPath %q = %q, want %q", c.d.path, c.d.srcPath, got, c.want)
		}
	}
}

// TestAStartFolderIsVettedAndFallsBackQuietly — every way a suggestion can be unusable answers ~/nib with
// no reason row, while the same path asked for WITHOUT `start` still says why it failed (a typed path).
func TestAStartFolderIsVettedAndFallsBackQuietly(t *testing.T) {
	ts, _ := startServer(t) // sets HOME to a temp dir, so ~/nib is hermetic
	c, _ := authedClient(t, ts)
	home, _ := os.UserHomeDir()
	def := filepath.Join(home, "nib")

	good := t.TempDir()
	file := filepath.Join(good, "a.pdf")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(def, "ceremonies", "abc")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	ended := filepath.Join(def, "ended", "abc")
	if err := os.MkdirAll(ended, 0o700); err != nil {
		t.Fatal(err)
	}

	if got := getListDir(t, ts.URL, c, good, "1"); got.Path != good || got.Reason != "" {
		t.Errorf("a usable start folder answered %q reason %q, want %q", got.Path, got.Reason, good)
	}
	for name, p := range map[string]string{
		"missing":           filepath.Join(good, "gone"),
		"a file":            file,
		"relative (exists)": ".",
		"the ceremony set":  store,
		"an ended set":      ended,
	} {
		if got := getListDir(t, ts.URL, c, p, "1"); got.Path != def || got.Reason != "" {
			t.Errorf("start folder %s (%s) answered %q reason %q, want the default %q with no reason", name, p, got.Path, got.Reason, def)
		}
	}
	// Typed navigation is unchanged: the same missing folder, not a start, says why.
	if got := getListDir(t, ts.URL, c, filepath.Join(good, "gone"), "0"); got.Reason != "missing" {
		t.Errorf("a typed missing folder reports %q, want missing", got.Reason)
	}
	// And typed navigation INTO the store is still allowed; only the first folder is vetted.
	if got := getListDir(t, ts.URL, c, store, "0"); got.Path != store {
		t.Errorf("typing the ceremony folder answered %q, want %q", got.Path, store)
	}
}

// TestListDirNamesTheDefaultFolder — the destination dialogs offer ~/nib, by its whole path, wherever else they are
// (ADR-215): every listing names the default folder, and says whether it IS that folder, cleaned on the server.
func TestListDirNamesTheDefaultFolder(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)
	home, _ := os.UserHomeDir()
	def := filepath.Join(home, "nib")
	if err := os.MkdirAll(def, 0o700); err != nil {
		t.Fatal(err)
	}
	away := t.TempDir()
	for _, tc := range []struct {
		name, path string
		at         bool
	}{
		{"an empty path lists the default", "", true},
		{"the default asked by name", def, true},
		{"the default with a trailing slash", def + "/", true},
		{"somewhere else", away, false},
	} {
		got := getListDir(t, ts.URL, c, tc.path, "0")
		if got.DefaultDir != def {
			t.Errorf("%s: defaultDir %q, want %q", tc.name, got.DefaultDir, def)
		}
		if got.AtDefault != tc.at {
			t.Errorf("%s (%q): atDefault %v, want %v", tc.name, got.Path, got.AtDefault, tc.at)
		}
	}
}
