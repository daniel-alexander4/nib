package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nib/internal/testpdf"
)

// splitPagesForm posts a page-range split of pdf into dir and returns the response.
func splitPagesForm(t *testing.T, c *http.Client, csrf, url string, pdf []byte, dir, ranges, prefix string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	f, err := mw.CreateFormFile("pdf", "doc.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(pdf); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"dir": dir, "mode": "ranges", "ranges": ranges, "prefix": prefix} {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	mw.Close()
	return write(t, c, csrf, http.MethodPost, url+"/api/split-pages", mw.FormDataContentType(), &buf)
}

// TestTheGUISplitNeverWritesOverTheDocumentItIsSplitting — /pending 569's other door.
//
// # Why this door needed checking rather than assuming
//
// `writeSplitParts` was reported to have "the same shape" as the CLI's writer BY READING, and the
// shape is the same — containment only — while the mechanism is not: this handler never opens a
// file. Its bytes arrive in the multipart body, so there is no descriptor to clobber and nothing
// here that could tell the difference. What it CAN do is replace the file on disk that the open
// document was loaded from, which is the same loss to the user and reaches it by a different
// route: `document.path`, not an input argument.
//
// So the door resolves the addressed document and hands its path to the same check the CLI uses.
// The GUI already warns *"Files with the same name will be replaced"* before a split — and the one
// file the user cannot possibly mean by that is the document currently on their screen.
//
// # What survives, and why the assertion is still the file
//
// Unlike the CLI, the bytes are NOT gone: the document stays open and `doc.data` still holds it,
// so a user who notices can Save As. The on-disk original is destroyed either way, and that is
// what is asserted.
func TestTheGUISplitNeverWritesOverTheDocumentItIsSplitting(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	pdf, err := testpdf.Text("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	// The document lives at the name a `--prefix foo --ranges 1-2` split produces.
	path := filepath.Join(dir, "foo1-2.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: path}); code != http.StatusOK {
		t.Fatalf("open = %d: %s", code, body)
	}

	resp := splitPagesForm(t, c, csrf, ts.URL, pdf, dir, "1-2", "foo")
	resp.Body.Close()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the document the split was reading is gone entirely: %v", err)
	}
	if !bytes.Equal(got, pdf) {
		t.Errorf("the GUI split replaced the open document's own file: %d bytes became %d, "+
			"status %d. A part is a subset, so the file on disk is now two pages of the document "+
			"that used to be there", len(pdf), len(got), resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("status = %d, want a refusal: a split aimed at the open document's own file must "+
			"be refused before the first write", resp.StatusCode)
	}
}

// TestTheGUISplitStillWritesPartsBesideTheDocument — the refusal is NARROW.
//
// The check refuses the ONE part that is the document itself, and the temptation is to refuse any
// name already on disk. That would break re-running a split into the same folder, which is the
// normal thing to do after changing a prefix, and it would contradict the confirm the client
// already shows. This asserts the ordinary case still works from the same folder as the document.
func TestTheGUISplitStillWritesPartsBesideTheDocument(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	pdf, err := testpdf.Text("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: path}); code != http.StatusOK {
		t.Fatalf("open = %d: %s", code, body)
	}

	resp := splitPagesForm(t, c, csrf, ts.URL, pdf, dir, "1-2,3", "part")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("an ordinary split into the document's own folder = %d, want 200 — the "+
			"self-overwrite refusal has widened into 'any existing name'", resp.StatusCode)
	}
	for _, n := range []string{"part1-2.pdf", "part3.pdf"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s was not written: %v", n, err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pdf) {
		t.Error("the document itself changed during an ordinary split")
	}
}

// TestASplitPartIsReadableByTheAccountTheUserGaveItTo — /pending 570.
//
// # The disagreement
//
// A new split part was 0600 from this door (`atomicfile.Write(full, p.Data, 0o600)`) and 0644 from
// the CLI (`writeNamed` → `atomicfile.ReplaceDurable(path, data, 0o644)`). Same operation, same
// re-derivable output, and **nobody chose it** — it fell out of which helper each door happened to
// call, exactly as the durability difference did.
//
// # Why 0644 and not 0600
//
// A split part is a DOCUMENT the user asked to be written into a folder they named, not one of
// nib's own files. `ReplaceDurable` records the identical lesson one door over (/pending 499):
// *"saving a user's 0644 original left it 0600 — a document shared with a group or served by a
// local web server stopped being readable the moment Nib saved it, with no message"*. The repo has
// already made this call once, for the save door, and made it against 0600.
//
// The cost of the losing option is named in `pdfops.SplitPartMode` rather than hidden here. The
// source document is opened at 0600 on purpose, so this also pins the THIRD option — carrying the
// source's mode onto each part — as refused: that one is closed to this door for a document with no
// path at all, so it would split the doors again along a line the user cannot see.
func TestASplitPartIsReadableByTheAccountTheUserGaveItTo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): POSIX permission bits")
	}
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	pdf, err := testpdf.Text("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	// Opened from a DIFFERENT folder, so the mode is the only thing under test here.
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: path}); code != http.StatusOK {
		t.Fatalf("open = %d: %s", code, body)
	}

	resp := splitPagesForm(t, c, csrf, ts.URL, pdf, dir, "1-2,3", "part")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("split status = %d, want 200", resp.StatusCode)
	}
	for _, n := range []string{"part1-2.pdf", "part3.pdf"} {
		info, err := os.Stat(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("a part written by the GUI is %#o, want 0644 — the CLI writes the same part "+
				"0644, so where a split's output lands depends on which surface produced it (%s)",
				got, n)
		}
	}
}
