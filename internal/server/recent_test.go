package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/ceremony"
	"nib/internal/testpdf"
)

// recentHas reports whether Open Recent holds path.
func recentHas(srv *Server, path string) bool {
	for _, r := range srv.vault.Recent() {
		if r == path {
			return true
		}
	}
	return false
}

// pdfAt writes a readable PDF at dir/name, creating dir.
func pdfAt(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestEveryRouteThatNamesAPathIsListedUnlessItIsACeremonysOwnCopy is /pending 490's table: one row per
// way a path reaches Nib, each with the verdict it must get. The ceremony rows are the point of the
// door — before it, `handleOpen` listed whatever path was typed, including one inside `~/nib/ceremonies`.
func TestEveryRouteThatNamesAPathIsListedUnlessItIsACeremonysOwnCopy(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	srv.SetHandoffSecret("secret")
	root, err := ceremony.StoreRoot()
	if err != nil {
		t.Fatal(err)
	}
	open := func(p string) { openByPath(t, ts.URL, c, csrf, p) }
	handoff := func(p string) {
		if code, _ := handoffTo(t, ts, c, "secret", p); code != http.StatusOK {
			t.Fatalf("hand-off of %s = %d", p, code)
		}
	}
	// save writes the file at path through Save As's route, with the bytes given.
	save := func(data []byte) func(string) {
		return func(p string) {
			resp := writeFileReq(t, ts, c, csrf, filepath.Dir(p), filepath.Base(p), data, false)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("save of %s = %d", p, resp.StatusCode)
			}
		}
	}
	pdfBytes, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name   string
		path   string
		route  func(string)
		listed bool
	}{
		{"Open… with an ordinary path", pdfAt(t, t.TempDir(), "a.pdf"), open, true},
		{"hand-off of an ordinary path", pdfAt(t, t.TempDir(), "b.pdf"), handoff, true},
		{"Open… typed inside the live ceremony folder", pdfAt(t, filepath.Join(root, "ceremonies", strings.Repeat("a", 32)), "c.pdf"), open, false},
		{"Open… typed inside ended/", pdfAt(t, filepath.Join(root, "ended", strings.Repeat("b", 32)), "d.pdf"), open, false},
		{"hand-off of a file inside the ceremony folder", pdfAt(t, filepath.Join(root, "ceremonies", strings.Repeat("c", 32)), "e.pdf"), handoff, false},
		{"Save As of a PDF (a dropped file, an upload, a URL or a combine gets its path here)", filepath.Join(t.TempDir(), "saved.pdf"), save(pdfBytes), true},
		{"Save As of an export that is not a PDF", filepath.Join(t.TempDir(), "export.md"), save([]byte("# not a pdf\n")), false},
		{"Save As of a PDF into the ceremony folder", filepath.Join(root, "ceremonies", strings.Repeat("d", 32), "f.pdf"), save(pdfBytes), false},
		{"a signed copy kept beside, not inside, the store", pdfAt(t, root, "kept.pdf"), open, true},
	}
	for _, r := range rows {
		r.route(r.path)
		if got := recentHas(srv, r.path); got != r.listed {
			t.Errorf("%s: listed = %v, want %v (recent: %v)", r.name, got, r.listed, srv.vault.Recent())
		}
	}
}

// TestOnlyTheRecentDoorAddsToRecent is ADR-009's guard for the door: a third route calling AddRecent
// itself would skip the ceremony refusal, and the table above would not know it existed.
func TestOnlyTheRecentDoorAddsToRecent(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "recent.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), ".AddRecent(") {
			t.Errorf("%s calls AddRecent; Open Recent is written through recordRecent only (recent.go, /pending 490)", f)
		}
	}
}
