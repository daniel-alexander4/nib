package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// finalizeWith posts pdf to /api/finalize with params (a JSON object, or "" for none) and returns the response.
func finalizeWith(t *testing.T, c *http.Client, csrf, base string, pdf []byte, params string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	if params != "" {
		mw.WriteField("params", params)
	}
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, base+"/api/finalize", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

// keptFiles lists what is in ~/nib/signed: the kept copies, and anything else (a staging temp file included).
func keptFiles(t *testing.T) (kept, other []string) {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(defaultOutputDir(), keptSubdir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if keptGrammar.MatchString(e.Name()) {
			kept = append(kept, e.Name())
		} else {
			other = append(other, e.Name())
		}
	}
	return kept, other
}

// TestFinalizeKeepsExactlyTheCopyAskedFor is P04.S01's acceptance at the route (D13, D14): ticked writes exactly one
// kept copy, byte-identical to what was returned, 0600, named in X-Nib-Kept; unticked writes nothing, and so does a
// request that does not send the field at all (Complete & sign's shape). The ticked row is the control that makes the
// other two mean something — they run in the same HOME.
func TestFinalizeKeepsExactlyTheCopyAskedFor(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r","keep":true,"name":"Lease 2026 (final).pdf"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ticked: status %d: %s", resp.StatusCode, body)
	}
	kept, other := keptFiles(t)
	if len(kept) != 1 || len(other) != 0 {
		t.Fatalf("ticked: ~/nib/signed holds kept %v and other %v; want exactly one kept copy", kept, other)
	}
	if got := resp.Header.Get("X-Nib-Kept"); got != kept[0] {
		t.Errorf("X-Nib-Kept %q does not name the kept copy %q", got, kept[0])
	}
	if !strings.HasPrefix(kept[0], "kept_lease-2026-final_") {
		t.Errorf("kept name %q does not carry the document's slug", kept[0])
	}
	path := filepath.Join(defaultOutputDir(), keptSubdir, kept[0])
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, body) {
		t.Fatalf("the kept copy (%d bytes) is not the signed output returned (%d bytes), byte for byte", len(onDisk), len(body))
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("kept copy mode %v, want 0600 — signed PII readable by other local users", fi.Mode().Perm())
	}
	// The join the dispute surface and the checklist will make: the kept copy ends where its signature's coverage does.
	recs, err := sign.Revisions(onDisk)
	if err != nil || len(recs) == 0 {
		t.Fatalf("the kept copy's signature records: %v (%d)", err, len(recs))
	}
	if end := recs[len(recs)-1].CoverageEnd; end != int64(len(onDisk)) {
		t.Errorf("the kept copy's last coverage end is %d, its length %d — the size join P04.S02 builds on fails", end, len(onDisk))
	}

	for _, tc := range []struct{ name, params string }{
		{"unticked", `{"reason":"r","keep":false,"name":"x.pdf"}`},
		{"no keep field (Complete & sign's shape)", `{"reason":"r"}`},
		{"no params at all", ""},
	} {
		resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, tc.params)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d: %s", tc.name, resp.StatusCode, body)
		}
		if got := resp.Header.Get("X-Nib-Kept"); got != "" {
			t.Errorf("%s: X-Nib-Kept %q on a request that kept nothing", tc.name, got)
		}
		if k, _ := keptFiles(t); len(k) != 1 {
			t.Errorf("%s: ~/nib/signed now holds %d kept copies, want still the one from the ticked run", tc.name, len(k))
		}
	}
}

// TestAFailedKeepRefusesTheSigning is D14 at the route: with ~/nib/signed planted as a regular FILE (a failure that
// holds for root too, unlike a chmod), a ticked Finalize answers non-2xx with the copy-not-kept cause, returns no PDF
// bytes, and leaves nothing new in ~/nib. **What it reaches is the folder's creation failing** — `saveKept` stops at
// `MkdirAll`, before any staging file could exist. A failure mid-WRITE relies on `atomicfile.WriteDurable`'s own
// deferred removal of its temp file (`atomicfile.go:316`), which no test here drives (declared).
func TestAFailedKeepRefusesTheSigning(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(defaultOutputDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(defaultOutputDir(), keptSubdir)
	if err := os.WriteFile(blocker, []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Stimulus: the same request UNTICKED signs — the failure below is the keep, not the signing.
	if resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: an unticked finalize failed too (%d: %s), so the refusal below would not be the keep's", resp.StatusCode, body)
	}
	resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r","keep":true,"name":"x.pdf"}`)
	if resp.StatusCode/100 == 2 {
		t.Fatalf("a ticked finalize whose copy could not be written answered %d — the user holds a signature they believe is kept", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		t.Errorf("422: the modal reads that as a wrong certificate passphrase")
	}
	var ref keptRefusal
	if err := json.Unmarshal(body, &ref); err != nil || ref.Cause != "copy-not-kept" {
		t.Errorf("body %q: want a keptRefusal with cause copy-not-kept (%v)", body, err)
	}
	if ref.Full {
		t.Errorf("a file where the folder should be was reported as a full disk — the user is sent to free space they do not lack")
	}
	if bytes.Contains(body, []byte("%PDF")) {
		t.Errorf("the refusal carried the signed document's bytes")
	}
	if fi, err := os.Stat(blocker); err != nil || fi.IsDir() {
		t.Fatalf("the planted file changed under the test: %v", err)
	}
	if ents, _ := os.ReadDir(defaultOutputDir()); len(ents) != 1 {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Errorf("~/nib holds %v after the failed keep, want only the planted file", names)
	}
}

// TestTheKeptGrammarIsTheOnlyDoor — keptPathFor accepts only names keptName makes, rebuilds the path inside
// ~/nib/signed, and refuses every name another writer in that folder makes, and every escape.
func TestTheKeptGrammarIsTheOnlyDoor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 10, 2, 14, 15, 2, 0, time.UTC)
	if got := keptName("Contract v1.2", nil, now); !strings.HasPrefix(got, "kept_contract-v12_") {
		t.Errorf("keptName(\"Contract v1.2\") = %q: only a .pdf extension may be dropped", got)
	}
	for _, doc := range []string{"Lease.pdf", "", "___", strings.Repeat("ab_", 40) + ".pdf", "../../etc/passwd", "über straße"} {
		name := keptName(doc, []byte("x"), now)
		path, err := keptPathFor(name)
		if err != nil {
			t.Errorf("keptPathFor refused keptName(%q) = %q: %v", doc, name, err)
			continue
		}
		if filepath.Dir(path) != filepath.Join(defaultOutputDir(), keptSubdir) || filepath.Base(path) != name {
			t.Errorf("keptName(%q) = %q resolved to %q, outside ~/nib/signed or renamed", doc, name, path)
		}
	}
	peer := receivedName("Alice_Smith", []byte{1, 2, 3, 4, 5}, []byte("doc"))
	for _, name := range []string{
		peer, // a signed arrival from a peer
		"lease-0123456789abcdef0123456789abcdef.pdf", // a delivered ceremony copy
		"kept_x_20261002-141502-0011aabb.pdf/../../a", "../kept_x_20261002-141502-0011aabb.pdf",
		"/abs/kept_x_20261002-141502-0011aabb.pdf", "kept__20261002-141502-0011aabb.pdf",
		"kept_x_20261002-141502-0011AABB.pdf", "kept_x_20261002-141502-0011aabb.PDF", "kept_x_20261002-141502-0011aabb.pdf ",
		"kept_a_b_20261002-141502-0011aabb.pdf",
	} {
		if _, err := keptPathFor(name); err == nil {
			t.Errorf("keptPathFor accepted %q", name)
		}
	}
}

// TestAnExternallySignedKeptCopyEndsAtItsCoverage — the size join P04.S02 builds on, for the imported-certificate
// signer too: Finalize with signAs "external" keeps a copy whose last signature covers it to its last byte.
func TestAnExternallySignedKeptCopyEndsAtItsCoverage(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	if resp := importP12(t, c, csrf, ts.URL, testP12(t, "pw"), "pw"); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: importing the external signer = %d", resp.StatusCode)
	}
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r","keep":true,"name":"x.pdf","signAs":"external","passphrase":"pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	kept, _ := keptFiles(t)
	if len(kept) != 1 {
		t.Fatalf("kept %v, want one", kept)
	}
	onDisk, _ := os.ReadFile(filepath.Join(defaultOutputDir(), keptSubdir, kept[0]))
	recs, err := sign.Revisions(onDisk)
	if err != nil || len(recs) == 0 {
		t.Fatalf("records: %v (%d)", err, len(recs))
	}
	if end := recs[len(recs)-1].CoverageEnd; end != int64(len(onDisk)) || !bytes.Equal(onDisk, body) {
		t.Errorf("external: last coverage end %d, length %d, identical to the response %v", end, len(onDisk), bytes.Equal(onDisk, body))
	}
}
