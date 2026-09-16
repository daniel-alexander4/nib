package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// pathDoc499 registers a document read from a real file at mode.
func pathDoc499(t *testing.T, s *Server, data []byte, mode os.FileMode) *document {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(p, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil { // past the umask
		t.Fatal(err)
	}
	d, err := s.addDocCapped(newPathDoc(p, data))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestSaveRefusesToGrowADocumentPastTheByteCap — /pending 499, ADR-008.
//
// handleSave assigns doc.data from the posted bytes, which makes it a growth door, and it was the
// one writer of doc.data that never asked byteCapLocked. The refusal must also come BEFORE the
// write: after it, the user's file already holds bytes Nib then refuses to hold.
func TestSaveRefusesToGrowADocumentPastTheByteCap(t *testing.T) {
	s := New(nil, nil, t.TempDir(), "test")
	small, err := testpdf.Text("a")
	if err != nil {
		t.Fatal(err)
	}
	s.maxDocBytes = 4 * len(small)
	a := pathDoc499(t, s, small, 0o600)
	big := append(append([]byte{}, small...), bytes.Repeat([]byte("%pad\n"), 2*len(small))...)
	// STIMULUS: the posted bytes really cross the budget, or a 200 below would be correct.
	if len(big) <= s.docBudget() {
		t.Fatalf("setup: %d posted bytes do not exceed the %d budget", len(big), s.docBudget())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/save?overwrite=1", bytes.NewReader(big))
	req.Header.Set("X-Nib-Doc", a.id.String())
	rec := httptest.NewRecorder()
	s.handleSave(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("a save growing the open documents past the byte cap answered %d, want 409 (ADR-008): %s",
			rec.Code, rec.Body)
	}
	if onDisk, _ := os.ReadFile(a.path); !bytes.Equal(onDisk, small) {
		t.Error("the save was refused but the user's file had already been overwritten — the cap is " +
			"tested after the write, so the refusal protects memory and not the file")
	}
	if !bytes.Equal(s.docBytes(a), small) {
		t.Error("the refused save still replaced the document's bytes")
	}
}

// TestSaveAndSaveAsKeepTheReplacedFilesMode — /pending 499: a 0644 original came back 0600.
func TestSaveAndSaveAsKeepTheReplacedFilesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows carries only the read-only bit")
	}
	pdf, err := testpdf.Text("a")
	if err != nil {
		t.Fatal(err)
	}
	edited := append(append([]byte{}, pdf...), []byte("\n%edit\n")...)

	t.Run("save", func(t *testing.T) {
		s := New(nil, nil, t.TempDir(), "test")
		a := pathDoc499(t, s, pdf, 0o644)
		req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(edited))
		req.Header.Set("X-Nib-Doc", a.id.String())
		rec := httptest.NewRecorder()
		s.handleSave(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("save %d %s", rec.Code, rec.Body)
		}
		if onDisk, _ := os.ReadFile(a.path); bytes.Equal(onDisk, pdf) {
			t.Fatal("setup: the save did not reach the file, so its mode describes nothing")
		}
		if fi, _ := os.Stat(a.path); fi.Mode().Perm() != 0o644 {
			t.Errorf("the user's 0644 original is %v after Save — group and other lost read access, "+
				"with no message", fi.Mode().Perm())
		}
	})

	t.Run("save as overwrite", func(t *testing.T) {
		s := New(nil, nil, t.TempDir(), "test")
		dir := t.TempDir()
		target := filepath.Join(dir, "out.pdf")
		if err := os.WriteFile(target, pdf, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, 0o644); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("data", "out.pdf")
		fw.Write(edited)
		mw.WriteField("dir", dir)
		mw.WriteField("name", "out.pdf")
		mw.WriteField("overwrite", "1")
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/write", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		s.handleWriteFile(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("save as %d %s", rec.Code, rec.Body)
		}
		if onDisk, _ := os.ReadFile(target); bytes.Equal(onDisk, pdf) {
			t.Fatal("setup: the overwrite did not reach the file")
		}
		if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
			t.Errorf("Save As over a 0644 file left it %v", fi.Mode().Perm())
		}
	})
}

// TestReloadOfASignedCopyOverAnUnsignedFileAsksThenProceeds — /pending 499.
//
// The erasure door refuses with "Confirm that you want that, and Nib will do it", and /api/reload
// hard-coded the acknowledgement to false — so the refusal promised a way through that the route
// could never take.
func TestReloadOfASignedCopyOverAnUnsignedFileAsksThenProceeds(t *testing.T) {
	s := New(nil, nil, t.TempDir(), "test")
	signed := signedFixture(t)
	a := pathDoc499(t, s, signed, 0o600)
	unsigned, err := testpdf.Text("the lease", "v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.path, unsigned, 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/reload", nil)
	req.Header.Set("X-Nib-Doc", a.id.String())
	rec := httptest.NewRecorder()
	s.handleReload(rec, req)
	// STIMULUS: this is the erasure refusal, not some other 409.
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "no record it was ever signed") {
		t.Fatalf("setup: an unacknowledged reload answered %d %q, want the erasure refusal", rec.Code, rec.Body)
	}
	if !sign.HasSignatureBlob(s.docBytes(a)) {
		t.Fatal("setup: the refused reload replaced the signed copy anyway")
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("acceptSignatureLoss", "1")
	mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/reload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Nib-Doc", a.id.String())
	rec = httptest.NewRecorder()
	s.handleReload(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("a reload the user CONFIRMED was still refused %d: %s — the refusal says to confirm, "+
			"and the route reads nothing to confirm with", rec.Code, rec.Body)
	}
	if !bytes.Equal(s.docBytes(a), unsigned) {
		t.Error("the confirmed reload did not install the file's bytes")
	}
}

// TestATouchedButIdenticalFileIsReadOnceNotOnEveryResponse — /pending 499.
//
// Measured before building (diskCheck's comment): 517 ms per document response at 200 MiB once a
// file's mtime moved with its bytes unchanged, because the baseline never matched again.
func TestATouchedButIdenticalFileIsReadOnceNotOnEveryResponse(t *testing.T) {
	s := New(nil, nil, t.TempDir(), "test")
	pdf, err := testpdf.Text("a")
	if err != nil {
		t.Fatal(err)
	}
	a := pathDoc499(t, s, pdf, 0o600)
	later := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := os.Chtimes(a.path, later, later); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the touch really lands on the slow path (read and hash), or the refresh is untested.
	if changed, refreshed := diskCheck(a.path, s.diskOf(a)); changed || refreshed == nil {
		t.Fatalf("setup: a touched identical file gave changed=%v refreshed=%v, want false and a "+
			"refreshed baseline", changed, refreshed != nil)
	}

	if s.docResponse(a).DiskChanged {
		t.Fatal("a touched but identical file is reported as changed")
	}
	if !s.diskOf(a).info.ModTime().Equal(later) {
		t.Error("the document response re-hashed a touched file and kept the stale baseline, so every " +
			"later response reads and hashes the whole file again")
	}
	if _, refreshed := diskCheck(a.path, s.diskOf(a)); refreshed != nil {
		t.Error("after the refresh the next check still took the read-and-hash path")
	}

	// And a real same-size change after the refresh is still reported — the refresh kept the
	// recorded HASH, not the new bytes.
	changedBytes := bytes.Replace(pdf, []byte("%PDF-1."), []byte("%PDF-2."), 1)
	if len(changedBytes) != len(pdf) || bytes.Equal(changedBytes, pdf) {
		t.Fatal("setup: could not build a same-size different file")
	}
	if err := os.WriteFile(a.path, changedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	even := later.Add(time.Hour)
	_ = os.Chtimes(a.path, even, even)
	if !s.docResponse(a).DiskChanged {
		t.Error("a same-size rewrite after a refreshed baseline was not reported — the refresh " +
			"adopted the file's content instead of only its stat")
	}
}

// TestUndoAndRedoRefuseADocumentClosedMidCall — /pending 499.
//
// Both handlers resolved the document, released the lock and took it again to move its history.
// The door is tested directly, and the two handlers are checked for routing through it (ADR-009):
// the gap between the two lock holds cannot be driven from outside the handler.
func TestUndoAndRedoRefuseADocumentClosedMidCall(t *testing.T) {
	pdf, err := testpdf.Text("a")
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	doc := s.activeDoc()
	if doc == nil {
		t.Fatal("setup: no document open")
	}

	rec := httptest.NewRecorder()
	s.mu.Lock()
	if !s.stillHeldLocked(rec, doc) {
		t.Fatal("a registered document was refused")
	}
	s.mu.Unlock()

	s.removeDoc(doc)
	rec = httptest.NewRecorder()
	s.mu.Lock()
	if s.stillHeldLocked(rec, doc) {
		s.mu.Unlock()
		t.Fatal("a closed document passed the re-test, so undo moves the history of a document " +
			"nobody holds and answers 200")
	}
	if !s.mu.TryLock() {
		t.Fatal("the refusal did not release the server lock")
	}
	s.mu.Unlock()
	if rec.Code != http.StatusConflict {
		t.Errorf("refusal wrote %d, want 409 (ADR-004)", rec.Code)
	}

	src, err := os.ReadFile("undo.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	for _, h := range []string{"handleUndo", "handleRedo"} {
		i := strings.Index(code, "func (s *Server) "+h+"(")
		if i < 0 {
			t.Fatalf("setup: %s not found in undo.go", h)
		}
		if !strings.Contains(funcBodyFrom(code, i), "s.stillHeldLocked(") {
			t.Errorf("%s does not re-test registration under the lock that moves the history", h)
		}
	}
}

// TestBakeRefusesAStampWhoseLibraryImageIsGone — /pending 499: it used to be skipped silently.
func TestBakeRefusesAStampWhoseLibraryImageIsGone(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf, _ := testpdf.Form()

	post := func(stamps string) *http.Response {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(pdf)
		fields, _ := json.Marshal([]pdfops.Field{{Page: 1, Rect: [4]float64{72, 700, 300, 716}, Text: "Hello"}})
		mw.WriteField("fields", string(fields))
		if stamps != "" {
			mw.WriteField("stamps", stamps)
		}
		mw.Close()
		return write(t, c, csrf, http.MethodPost, ts.URL+"/api/bake", mw.FormDataContentType(), &buf)
	}

	// STIMULUS: the same bake without the stamp succeeds, so a refusal below is about the stamp.
	ok := post("")
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("setup: the bake without a stamp answered %d", ok.StatusCode)
	}

	resp := post(`[{"page":1,"rect":[72,600,200,650],"image":"0123456789abcdef0123456789abcdef"}]`)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("a stamp naming an image the library does not hold was baked as though it were " +
			"absent — the signed or saved file silently lacks the stamp the user placed")
	}
	if !strings.Contains(string(body), "no longer in your library") {
		t.Errorf("refused %d, but not with the sentence the user can act on: %s", resp.StatusCode, body)
	}
}

// TestGETsThatActRefuseACrossSiteRequest — /pending 499.
//
// requireUnlocked checks origin on non-GET methods only, and /api/update/check had no gate at all.
// Each of these ACTS: an outbound request to GitHub, a signing identity minted into the vault, an
// announcement on the local link.
func TestGETsThatActRefuseACrossSiteRequest(t *testing.T) {
	stubLatest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.0.1", "html_url": "https://example.test/rel"})
	})
	ts, _ := startServerWith(t)
	get := func(route, site string) int {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+route, nil)
		req.Header.Set("Sec-Fetch-Site", site)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	// STIMULUS, and the pill's own path: the UI's same-origin fetch still reaches the check.
	if code := get("/api/update/check", "same-origin"); code != http.StatusOK {
		t.Fatalf("the UI's same-origin update check answered %d — the version pill would stop working", code)
	}
	for _, route := range []string{"/api/update/check", "/api/identity", "/api/lan/test"} {
		if code := get(route, "cross-site"); code != http.StatusForbidden {
			t.Errorf("GET %s from a cross-site page answered %d, want 403 — the route acts, and any "+
				"page in the user's browser can reach it with an <img src>", route, code)
		}
	}
}
