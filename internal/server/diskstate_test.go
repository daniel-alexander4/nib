package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// /pending 333 — the file underneath an open document can change, and Nib could not tell.
//
// The reported symptom was "Nib didn't show the updated PDF and a hard reload didn't
// update it". The reload could not help: it re-fetches from the same in-memory copy the
// server has held since open. So there are two things to drive, and the second is the
// one that costs the user something:
//
//   - the REPORT — docResponse says the file moved, so the client can say so;
//   - the REFUSAL — /api/save will not write the stale copy over the newer file.
//
// A note on the fixtures. Every rewrite below changes the file's LENGTH as well as its
// content. That is deliberate and not incidental: mtime resolution is 1 ms on this host,
// so a same-size rewrite inside the same millisecond tick is invisible to the size+mtime
// trigger, and a test written that way would flake against correct code. Where the
// same-size case is the thing under test (TestDiskChangedSurvivesASameSizeRewrite) the
// mtime is moved explicitly with os.Chtimes rather than raced for.

// writeDoc posts bytes to a document-addressed route. The package's `write` helper does
// not pin, and every save below must name the document it is saving (ADR-004) or the
// route falls back to whichever document is active — which in a one-document test is
// the same answer for the wrong reason.
func writeDoc(t *testing.T, c *http.Client, csrf, url, docID string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Nib-Doc", docID)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// uploadPDF installs a document with NO path, the way the browser file-picker does.
func uploadPDF(t *testing.T, baseURL string, c *http.Client, csrf, name string, data []byte) docResponse {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, baseURL+"/api/upload", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d, want 200", resp.StatusCode)
	}
	var dr docResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatal(err)
	}
	return dr
}

// rewriteFile replaces path with different, longer bytes and returns them.
func rewriteFile(t *testing.T, path string) []byte {
	t.Helper()
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A valid PDF still, so the bytes could legitimately have come from another tool —
	// appending a comment keeps it parseable and changes both length and content.
	updated := append(append([]byte{}, orig...), []byte("\n% rewritten on disk by another program\n")...)
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	return updated
}

// TestDocReportsAFileChangedUnderneathIt is the report half.
func TestDocReportsAFileChangedUnderneathIt(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	if dr.DiskChanged {
		t.Fatal("a freshly opened document already reports diskChanged — the baseline is being recorded wrong, and every later assertion here would pass for the wrong reason")
	}

	rewriteFile(t, path)

	got := docByID(t, ts, c, dr.ID)
	if !got.DiskChanged {
		t.Error("the file was rewritten on disk and /api/doc still reports diskChanged=false — this is the defect /pending 333 was filed for: the user has no way to learn the document they are looking at is not the file")
	}
}

// TestSaveRefusesToOverwriteAFileThatChanged is the refusal half, and it is the one
// that separates "we noticed" from "we noticed after destroying it".
//
// The assertion is on the BYTES ON DISK, not on the status code alone. A refusal that
// answers 412 after the write has already landed would satisfy a status-only check
// while losing exactly as much of the user's data as no check at all.
func TestSaveRefusesToOverwriteAFileThatChanged(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	external := rewriteFile(t, path)

	stale, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	resp := writeDoc(t, c, csrf, ts.URL+"/api/save", dr.ID, stale)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("save over a changed file: status = %d, want 412", resp.StatusCode)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, external) {
		t.Error("the save reached the disk anyway: the file no longer holds what was written to it externally, so the refusal happened after the data was already gone")
	}
}

// TestSaveOverwritesWhenTheUserSaysSo — the refusal is a default, not a wall. The edits
// live in the browser, so a block with no way past it would strand the user's unsaved
// work behind the warning meant to protect it.
func TestSaveOverwritesWhenTheUserSaysSo(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	rewriteFile(t, path)

	mine, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	resp := writeDoc(t, c, csrf, ts.URL+"/api/save?overwrite=1", dr.ID, mine)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save with overwrite=1: status = %d, want 200", resp.StatusCode)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, mine) {
		t.Error("overwrite=1 answered 200 without writing the user's bytes")
	}
}

// TestSaveRerecordsItsOwnWrite is the row nobody writes, and without it the feature
// ships permanently armed.
//
// WriteDurable renames into place, so the inode AND the mtime change on every save Nib
// itself performs. If handleSave does not re-stamp the baseline, the document reports
// "changed on disk" from the moment the user first saves it — against Nib's own write,
// forever, and every other test here still passes.
func TestSaveRerecordsItsOwnWrite(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	mine, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	resp := writeDoc(t, c, csrf, ts.URL+"/api/save", dr.ID, mine)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save: status = %d, want 200", resp.StatusCode)
	}

	got := docByID(t, ts, c, dr.ID)
	if got.DiskChanged {
		t.Error("the document reports diskChanged immediately after ITS OWN save — the baseline is not re-recorded across the write, so the warning is armed forever and the user learns to ignore it")
	}
	// And a second save must still be allowed, which is the consequence that actually
	// reaches the user: an un-re-recorded baseline makes every save after the first
	// answer 412.
	resp2 := writeDoc(t, c, csrf, ts.URL+"/api/save", dr.ID, mine)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("the second consecutive save: status = %d, want 200", resp2.StatusCode)
	}
}

// TestDiskChangedSurvivesASameSizeRewrite drives the case the cheap trigger cannot see
// on its own. A rewrite of identical length only announces itself through mtime, so
// this is where the size shortcut must NOT be allowed to answer "unchanged".
func TestDiskChangedSurvivesASameSizeRewrite(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same length, different content: flip a byte in the trailing comment region.
	same := append([]byte{}, orig...)
	same[len(same)-1] ^= 0xff
	if err := os.WriteFile(path, same, 0o600); err != nil {
		t.Fatal(err)
	}
	// Moved explicitly rather than raced for — see the note at the top of this file.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	if got := docByID(t, ts, c, dr.ID); !got.DiskChanged {
		t.Error("a same-size rewrite is reported as unchanged — the size shortcut is answering a question only a content comparison can")
	}
}

// TestUntouchedFileIsNeverReportedChanged is the false-positive guard, and it earns its
// place: a detector that answers "changed" too eagerly produces a banner over every
// document, which is the same outcome as no banner at all.
//
// The bare mtime touch is the specific case. "This file has changed on disk" is a FALSE
// STATEMENT about a file whose bytes are identical, and mtime alone cannot tell them
// apart — which is why the content hash exists.
func TestUntouchedFileIsNeverReportedChanged(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)

	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	if got := docByID(t, ts, c, dr.ID); got.DiskChanged {
		t.Error("a file whose mtime moved but whose bytes are identical is reported as changed — the banner would tell the user something untrue about her document")
	}
	// And the save must not be refused for it either.
	mine, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	resp := writeDoc(t, c, csrf, ts.URL+"/api/save", dr.ID, mine)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("save after a bare touch: status = %d, want 200 — a mtime change with identical bytes is not a reason to refuse", resp.StatusCode)
	}
}

// TestPathlessDocumentNeverReportsChanged — an upload has no file to disagree with, and
// asking about one would be asking about "".
func TestPathlessDocumentNeverReportsChanged(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dr := uploadPDF(t, ts.URL, c, csrf, "up.pdf", data)
	if dr.CanSave {
		t.Fatal("an uploaded document reports canSave — the fixture is not the path-less case this test needs")
	}
	if dr.DiskChanged {
		t.Error("a path-less document reports diskChanged — it has no file, so there is nothing the report could be about")
	}
}

// TestBothInstallDoorsRecordABaseline is the ADR-009 guard: the rule has one door and
// this asserts the ROUTING, not two copies of the stamping agreeing with each other.
//
// A baseline stamped at one install door and not the other is worse than none — the
// unstamped door produces documents that can never report a change, silently. Counting
// the callers is what catches a third door added without one.
func TestBothInstallDoorsRecordABaseline(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	ho, err := os.ReadFile("handoff.go")
	if err != nil {
		t.Fatal(err)
	}
	all := string(src) + string(ho)
	// Two CALL sites and no more: handleOpen and openHandedOff. The definition lives in
	// diskstate.go and is deliberately not counted here. A third path-install door is
	// not forbidden — it just has to come through the door and update this number
	// deliberately, which is the point of asserting a count rather than a minimum.
	if n := bytes.Count([]byte(all), []byte("newPathDoc(")); n != 2 {
		t.Errorf("newPathDoc has %d call sites across server.go and handoff.go, want 2 (handleOpen and openHandedOff) — if a third door was added, route it through newPathDoc and update this count; a document built from a path without going through the door cannot ever report a disk change", n)
	}
	// The literal both doors used to build, which must not come back at either of them.
	if bytes.Contains([]byte(all), []byte("&document{path: path")) {
		t.Error("a document is being built from a path outside newPathDoc — that door records no baseline, so its documents are silently exempt from the whole check")
	}
}

// --- the remedy half: reloading the file into the open document ------------------

// reloadDoc asks the server to re-read the file under a document. Pinned, because the
// route is the one the client fires WITHOUT the user asking and docFor falls back to the
// active document when no header arrives (ADR-004).
func reloadDoc(t *testing.T, ts *httptest.Server, c *http.Client, csrf, docID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/reload", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Nib-Doc", docID)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// pdfOf fetches the bytes the server is serving for a document, which is the only
// statement that matters here: docResponse could report anything it liked while /api/pdf
// went on serving the copy read at open, and that is precisely the defect being fixed.
func pdfOf(t *testing.T, ts *httptest.Server, c *http.Client, docID string) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/pdf?doc="+docID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/pdf status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// TestReloadInstallsTheFileIntoTheSameDocument is the remedy, and the id is half of it.
//
// The old Reload went through /api/open, so it produced a NEW document and the client had
// to build a second view and close the first. Asserting the id is unchanged is what stops
// that regressing: a reload that returns a new id would still show the right bytes, and
// would silently take back the in-place repaint the whole feature rests on.
func TestReloadInstallsTheFileIntoTheSameDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	updated := rewriteFile(t, path)
	if !docByID(t, ts, c, dr.ID).DiskChanged {
		t.Fatal("the rewrite did not register as a disk change — every assertion below would pass for the wrong reason")
	}

	resp := reloadDoc(t, ts, c, csrf, dr.ID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reload status = %d, want 200", resp.StatusCode)
	}
	var after docResponse
	if err := json.NewDecoder(resp.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.ID != dr.ID {
		t.Errorf("reload returned document id %q, want %q — a reload that renames the document forces the client to build a second view and close the first, which is the behaviour this route replaced", after.ID, dr.ID)
	}
	if !bytes.Equal(pdfOf(t, ts, c, dr.ID), updated) {
		t.Error("after a reload /api/pdf still serves the bytes read at open — the user pressed Reload and is still looking at the stale copy")
	}
	if after.DiskChanged {
		t.Error("the document reports diskChanged immediately after reloading FROM that file — the baseline is not re-stamped across the reload, so the banner is armed forever and the user learns to ignore it")
	}
}

// TestReloadIsUndoable is the safety net for an action the user did not ask for.
//
// The reload fires by itself on return-to-foreground. If it turns out to have been
// unwanted — the other program was mid-write, the user preferred what she had — Undo has
// to be able to put it back, and that is a property of routing the reload through
// commitMutation rather than assigning doc.data.
func TestReloadIsUndoable(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	original := pdfOf(t, ts, c, dr.ID)
	rewriteFile(t, path)

	resp := reloadDoc(t, ts, c, csrf, dr.ID)
	defer resp.Body.Close()
	var after docResponse
	if err := json.NewDecoder(resp.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if !after.CanUndo {
		t.Fatal("a reload leaves canUndo false — the bytes the user was looking at are gone with no way back, and an automatic reload owes her one")
	}

	undo := writeDoc(t, c, csrf, ts.URL+"/api/undo", dr.ID, nil)
	defer undo.Body.Close()
	if undo.StatusCode != http.StatusOK {
		t.Fatalf("undo status = %d, want 200", undo.StatusCode)
	}
	if !bytes.Equal(pdfOf(t, ts, c, dr.ID), original) {
		t.Error("undoing a reload did not restore the bytes the document held before it — the undo entry recorded something other than the pre-reload state")
	}
}

// TestReloadRefusesADocumentWithNoFile — an upload has no path, so there is no file to
// re-read. The refusal is a 400 rather than a 409 deliberately: the document is still very
// much open, and 409 is the client's signal to drop the tab.
func TestReloadRefusesADocumentWithNoFile(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	blank, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dr := uploadPDF(t, ts.URL, c, csrf, "nopath.pdf", blank)
	resp := reloadDoc(t, ts, c, csrf, dr.ID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("reload of a path-less document: status = %d, want 400", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusConflict {
		t.Error("a path-less document answered 409, which the client reads as \"that document is gone\" and acts on by dropping the tab — the document is open and has simply never had a file")
	}
}

// TestReloadRefusesWhatIsNoLongerAPDF, and leaves the document it refused intact.
//
// The file under an open document can be replaced by anything, including a half-written
// file caught mid-copy. The status matters less than the second assertion: a door that
// refuses AFTER replacing doc.data has destroyed the user's document to tell her it could
// not read the new one.
func TestReloadRefusesWhatIsNoLongerAPDF(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	dr := openByPath(t, ts.URL, c, csrf, path)
	original := pdfOf(t, ts, c, dr.ID)
	if err := os.WriteFile(path, []byte("this is not a PDF any more"), 0o600); err != nil {
		t.Fatal(err)
	}

	resp := reloadDoc(t, ts, c, csrf, dr.ID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("reload of a file that is no longer a PDF: status = %d, want 415", resp.StatusCode)
	}
	if !bytes.Equal(pdfOf(t, ts, c, dr.ID), original) {
		t.Error("a refused reload changed the document anyway — the user's open copy was destroyed by a door that then said it could not read the replacement")
	}
}

// TestReloadIsRefusedOnAConvenedDocument — D29's freeze, reached without this route
// writing a predicate of its own.
//
// This is the whole argument for committing through commitMutation. Nothing in
// handleReload mentions ceremonies; the rule is enforced because the reload goes through
// the door that enforces it, which is what ADR-009 asks for and what a hand-rolled
// `doc.data = data` would have quietly skipped.
func TestReloadIsRefusedOnAConvenedDocument(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	pdf, _, _ := convenedDocument(t)
	path := t.TempDir() + "/convened.pdf"
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	dr := openByPath(t, ts.URL, c, csrf, path)
	rewriteFile(t, path)

	resp := reloadDoc(t, ts, c, csrf, dr.ID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("reload of a convened document: status = %d, want 409 — the ceremony freeze must refuse it, and a reload that replaced the bytes would drop the embedded record every other party has signed against", resp.StatusCode)
	}
}
