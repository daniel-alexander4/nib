package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// reflowFixture is a three-line Helvetica paragraph and a signature line below it, both drawn by one text object.
func reflowFixture(t *testing.T) []byte {
	t.Helper()
	content := "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog and runs) Tj T* (away from here.) Tj T* T* T* (Signature line) Tj ET"
	return testpdf.WithContent(content, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
}

func postReflow(t *testing.T, s *Server, original, text string) *httptest.ResponseRecorder {
	t.Helper()
	// Multipart, as the page's FormData sends it: the handler reads through `parseMultipart`, the door that bounds the
	// body and removes what the parser spills to disk.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, kv := range [][2]string{{"page", "1"}, {"paragraph", "0"}, {"original", original}, {"text", text}} {
		if err := mw.WriteField(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/reflow", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleReflow(rec, req)
	return rec
}

func firstParagraph(t *testing.T, s *Server) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleParagraphs(rec, httptest.NewRequest(http.MethodGet, "/api/paragraphs?page=1", nil))
	var out paragraphsResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || len(out.Paragraphs) == 0 {
		t.Fatalf("paragraphs: %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	return out.Paragraphs[0].Text
}

// TestReflowCommitsAndUndoes — P06.S05's route: an edit commits into the open document through commitMutation, the page
// reads back as the edit, and one Undo restores the paragraph as it was. A stale paragraph is 409 and changes nothing.
func TestReflowCommitsAndUndoes(t *testing.T) {
	s := openTestServer(t, reflowFixture(t))
	orig := firstParagraph(t, s)
	edit := "The quick brown fox jumps over the sleepy old dog and runs away from here."

	if rec := postReflow(t, s, "a paragraph read before the page changed", edit); rec.Code != http.StatusConflict {
		t.Errorf("a stale paragraph answered %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if got := firstParagraph(t, s); got != orig {
		t.Fatalf("a refused reflow changed the document: %q", got)
	}
	rec := postReflow(t, s, orig, edit)
	var out reflowResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); rec.Code != http.StatusOK || err != nil || !out.Ok {
		t.Fatalf("reflow: %d %+v (%v)", rec.Code, out, err)
	}
	if got := firstParagraph(t, s); got != edit {
		t.Errorf("after the reflow the paragraph reads %q", got)
	}
	s.handleUndo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/undo", nil))
	if got := firstParagraph(t, s); got != orig {
		t.Errorf("after Undo the paragraph reads %q, want %q", got, orig)
	}
}

// TestReflowRefusesASignedDocumentAtTheServerDoor — D11: whatever the UI allowed, the server refuses, names the cause,
// and leaves the document's bytes exactly as they were.
func TestReflowRefusesASignedDocumentAtTheServerDoor(t *testing.T) {
	certPEM, keyPEM, err := sign.GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.Sign(reflowFixture(t), certPEM, keyPEM, sign.Options{Name: "Nib Test", When: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, signed)
	orig := firstParagraph(t, s)
	rec := postReflow(t, s, orig, "The quick brown fox jumps over the sleepy old dog and runs away from here.")
	var out reflowResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Ok || out.Cause != pdfops.ReflowCauseSigned {
		t.Errorf("a signed document answered %d %+v (%v), want ok=false cause %q", rec.Code, out, err, pdfops.ReflowCauseSigned)
	}
	if got := s.docBytes(s.activeDoc()); string(got) != string(signed) {
		t.Errorf("the refused reflow changed the signed document's bytes")
	}
	rec = httptest.NewRecorder()
	s.handleParagraphs(rec, httptest.NewRequest(http.MethodGet, "/api/paragraphs?page=1", nil))
	var list paragraphsResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil || len(list.Paragraphs) == 0 {
		t.Fatalf("paragraphs: %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	for _, p := range list.Paragraphs {
		if p.Refusal != pdfops.ReflowCauseSigned {
			t.Errorf("a signed document's paragraph %d is listed as reflowable (%q) — the editor would let the user type first", p.Index, p.Refusal)
		}
	}
}

// TestAReflowRequestIsBoundedAtTheDoor — the P06 phase-close review: the handler read its form with a bare FormValue, so
// the multipart body was unbounded and what the parser spilled to disk was never removed. It reads through
// `parseMultipart` now, so a body past the reflow bound is refused before anything is parsed.
func TestAReflowRequestIsBoundedAtTheDoor(t *testing.T) {
	s := openTestServer(t, reflowFixture(t))
	original := firstParagraph(t, s)
	if rec := postReflow(t, s, original, "The quick brown fox leaps over the lazy dog and runs away from here."); rec.Code != http.StatusOK {
		t.Fatalf("setup: a reflow of ordinary size answered %d, so the refusal below proves nothing", rec.Code)
	}
	if rec := postReflow(t, s, original, strings.Repeat("word ", maxReflowFormBytes/5+1)); rec.Code != http.StatusBadRequest {
		t.Errorf("a %d-byte reflow body answered %d, want 400", maxReflowFormBytes, rec.Code)
	}
}

// TestEveryReflowCauseIsSaidToTheUser — the P06 phase-close review: ten causes had no sentence in `web/app.js`, so the
// user was shown "It cannot be re-set exactly (mixed-content)". Every cause the Go side can return (`pdfops.ReflowCauses`,
// the server's own included) must be a key of REFLOW_CAUSES, and every key there must be a cause — a sentence for a cause
// nothing returns is a promise nothing keeps.
func TestEveryReflowCauseIsSaidToTheUser(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "const REFLOW_CAUSES = {")
	end := strings.Index(src[start:], "\n};")
	if start < 0 || end < 0 {
		t.Fatal("web/app.js has no REFLOW_CAUSES block")
	}
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*'?([a-z][a-z-]*)'?:`).FindAllStringSubmatch(src[start:start+end], -1) {
		keys[m[1]] = true
	}
	if len(pdfops.ReflowCauses) < 20 || len(keys) == 0 {
		t.Fatalf("setup: %d causes and %d sentences read — the census would pass over nothing", len(pdfops.ReflowCauses), len(keys))
	}
	causes := map[string]bool{}
	for _, c := range pdfops.ReflowCauses {
		causes[c] = true
		if !keys[c] {
			t.Errorf("the reflow cause %q has no sentence in REFLOW_CAUSES — the user is shown the code", c)
		}
	}
	for k := range keys {
		if !causes[k] {
			t.Errorf("REFLOW_CAUSES has a sentence for %q, which no reflow returns", k)
		}
	}
}
