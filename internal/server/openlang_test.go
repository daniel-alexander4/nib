package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// ADR-215: the Open dialog is the window's one way to convert a file, so the language field reaches BOTH of its
// routes, and a refusal by path says what a refusal by upload said.

// TestAURLOpenedFromTheDialogCarriesItsLanguage — the field sits beside "Path or URL", so a URL honours it too: a
// language declared, and the label only when chosen.
func TestAURLOpenedFromTheDialogCarriesItsLanguage(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("# Notes\n\nA paragraph.\n"))
	}))
	defer src.Close()
	for _, c := range []struct {
		name   string
		chosen bool
	}{{"pre-filled", false}, {"chosen", true}} {
		ts, _ := startServer(t)
		cl, csrf := authedClient(t, ts)
		resp := write(t, cl, csrf, http.MethodPost, ts.URL+"/api/open-url", "application/json",
			jsonBody(map[string]any{"url": src.URL + "/notes.md", "lang": "de", "langChosen": c.chosen}))
		out, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: open-url answered %d %s", c.name, resp.StatusCode, out)
		}
		if got := workingDocLang(t, cl, ts.URL); got != "de" {
			t.Errorf("%s: a URL opened with a language declares /Lang %q, want de", c.name, got)
		}
		pr, err := cl.Get(ts.URL + "/api/pdf")
		if err != nil {
			t.Fatal(err)
		}
		pdf, _ := io.ReadAll(pr.Body)
		pr.Body.Close()
		claims, err := testpdf.ClaimsUA(pdf)
		if err != nil {
			t.Fatal(err)
		}
		if claims != c.chosen {
			t.Errorf("%s: the URL's conversion claims PDF/UA = %v, want %v", c.name, claims, c.chosen)
		}
	}
}

// TestARefusalByPathIsTheRefusalByUpload — `isImportRefusal` kept its own copy of Import's errors and it drifted, so
// a CSV past the row cap opened by path answered "could not read file" (500). It is the cap's 413 at every door.
func TestARefusalByPathIsTheRefusalByUpload(t *testing.T) {
	ts, _ := startServer(t)
	cl, csrf := authedClient(t, ts)
	csv := strings.Repeat("a,b\n", 10_001)
	resp := importByPath(t, ts.URL, cl, csrf, "big.csv", []byte(csv), "", false)
	out, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("a CSV past the row cap opened by path answered %d %s, want 413 as an upload does", resp.StatusCode, out)
	}
}

// TestOnlyAFailedReadOfTheSourceIsTheDisks — the mark is put on at the read, so it is the only thing a path open
// calls a disk error; every error Import itself returns is a verdict about the file.
func TestOnlyAFailedReadOfTheSourceIsTheDisks(t *testing.T) {
	if isImportRefusal(sourceReadError{os.ErrClosed}) {
		t.Error("a failed read of the source was called a refusal")
	}
	for _, err := range []error{pdfops.ErrTextTooLong, pdfops.ErrCSVTooLong, pdfops.ErrNotADocument, errors.New("no document language given")} {
		if !isImportRefusal(err) {
			t.Errorf("%v is Import's verdict and was called a disk error", err)
		}
	}
	// A real read through the mark: a closed file fails its read, and the failure carries the mark.
	f, err := os.CreateTemp(t.TempDir(), "src")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := (sourceFile{f}).ReadAt(make([]byte, 1), 0); isImportRefusal(err) {
		t.Errorf("a read of a closed file (%v) was not marked as the disk's", err)
	}
	g, _ := os.CreateTemp(t.TempDir(), "empty")
	defer g.Close()
	if _, err := (sourceFile{g}).ReadAt(make([]byte, 1), 0); err != io.EOF {
		t.Errorf("the end of the file came back as %v, want io.EOF unwrapped", err)
	}
}

// TestARefusedLabelIsSaidOnBothOfTheDialogsRoutes — a language the person chose that did not earn the PDF/UA label
// rides on the open's own answer as `labelNote`, by path and by URL, and the document still opens. An empty Markdown
// file is one: there is nothing to tag (measured: "the document is not tagged…").
func TestARefusedLabelIsSaidOnBothOfTheDialogsRoutes(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer src.Close()
	ts, _ := startServer(t)
	cl, csrf := authedClient(t, ts)
	read := func(resp *http.Response) docResponse {
		t.Helper()
		defer resp.Body.Close()
		var d docResponse
		if resp.StatusCode != http.StatusOK {
			out, _ := io.ReadAll(resp.Body)
			t.Fatalf("answered %d %s", resp.StatusCode, out)
		}
		if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	byPath := read(importByPath(t, ts.URL, cl, csrf, "empty.md", nil, "en", true))
	byURL := read(write(t, cl, csrf, http.MethodPost, ts.URL+"/api/open-url", "application/json",
		jsonBody(map[string]any{"url": src.URL + "/empty.md", "lang": "en", "langChosen": true})))
	for name, d := range map[string]docResponse{"by path": byPath, "by URL": byURL} {
		if !strings.HasPrefix(d.LabelNote, "Not labelled PDF/UA:") {
			t.Errorf("%s: a chosen language refused the label and the answer's labelNote is %q", name, d.LabelNote)
		}
	}
	// Control: the pre-fill asks for no label, so nothing was refused and nothing is said.
	if d := read(importByPath(t, ts.URL, cl, csrf, "empty2.md", nil, "en", false)); d.LabelNote != "" {
		t.Errorf("the pre-filled language said %q; no label was asked for", d.LabelNote)
	}
}
