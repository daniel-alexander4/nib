package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The READ half of operation pinning (/pending 806, 625). The routes below commit nothing, so the
// MUTATING suite never drove them, and the client sent them with whatever tab was current. Each is
// driven here with two documents open and the OTHER one active: the handler must answer about the
// document the header names, and refuse (409, ADR-004) one it no longer holds.

// docReadsRequest posts a multipart form addressed to `doc` ("" sends no header).
func docReadsRequest(t *testing.T, c *http.Client, csrf, url, doc string, fields map[string]string, pdf []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if pdf != nil {
		fw, err := mw.CreateFormFile("pdf", "doc.pdf")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(pdf)
	}
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	if doc != "" {
		req.Header.Set("X-Nib-Doc", doc)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// An attachment id is positional, so two documents carrying one file each list the SAME id. The
// extract must come from the document the header names, never from the active one.
func TestAnAttachmentIsExtractedFromTheAddressedDocument(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	base, err := testpdf.Text("one")
	if err != nil {
		t.Fatal(err)
	}
	withFile := func(payload string) []byte {
		out, err := pdfops.AddAttachment(base, "note.txt", []byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	docA := srv.setDoc(&document{path: filepath.Join(t.TempDir(), "a.pdf"), data: withFile("A's note\n")})
	docB := srv.addDoc(&document{path: filepath.Join(t.TempDir(), "b.pdf"), data: withFile("B's note\n")})
	if srv.activeDoc() != docB {
		t.Fatal("setup: B is not active, so there is no switch to survive")
	}

	// The id as the page would hold it: listed from A, addressed to A.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/attachments", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Nib-Doc", docA.id.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var list attachmentsResponse
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Attachments) != 1 {
		t.Fatalf("A lists %d attachments, want 1", len(list.Attachments))
	}
	id := list.Attachments[0].ID

	resp = docReadsRequest(t, c, csrf, ts.URL+"/api/attachments/extract", docA.id.String(), map[string]string{"id": id}, nil)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pinned extract = %d: %s", resp.StatusCode, got)
	}
	if string(got) != "A's note\n" {
		t.Errorf("extract addressed to A returned %q — the ACTIVE document's file under A's id", got)
	}

	// A closed document is refused, never redirected at whatever is active.
	srv.removeDoc(docA)
	resp = docReadsRequest(t, c, csrf, ts.URL+"/api/attachments/extract", docA.id.String(), map[string]string{"id": id}, nil)
	got, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("extract for a closed document = %d (%q), want 409 — it answered from another document", resp.StatusCode, got)
	}
}

// The split's only guard against a part overwriting its source is the ADDRESSED document's path.
// A sits at the name the split would write; B is active. Addressed to A, the split is refused and
// A's file survives.
func TestTheSplitGuardsTheAddressedDocumentsPath(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	pdf, err := testpdf.Text("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	pathA := filepath.Join(dir, "foo1-2.pdf") // what `prefix foo, ranges 1-2` produces
	if err := os.WriteFile(pathA, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	docA := srv.setDoc(&document{path: pathA, data: pdf})
	srv.addDoc(&document{path: filepath.Join(t.TempDir(), "b.pdf"), data: pdf})

	fields := map[string]string{"dir": dir, "mode": "ranges", "ranges": "1-2", "prefix": "foo"}
	resp := docReadsRequest(t, c, csrf, ts.URL+"/api/split-pages", docA.id.String(), fields, pdf)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	got, rerr := os.ReadFile(pathA)
	if rerr != nil || !bytes.Equal(got, pdf) {
		t.Fatalf("the split addressed to A replaced A's own file (status %d: %s) — the guard checked "+
			"the ACTIVE document's path", resp.StatusCode, body)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d (%s), want 400: a part aimed at the addressed document's file is refused", resp.StatusCode, body)
	}

	srv.removeDoc(docA)
	resp = docReadsRequest(t, c, csrf, ts.URL+"/api/split-pages", docA.id.String(), fields, pdf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("split for a closed document = %d, want 409", resp.StatusCode)
	}
}
