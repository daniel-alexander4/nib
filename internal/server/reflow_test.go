package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	form := url.Values{"page": {"1"}, "paragraph": {"0"}, "original": {original}, "text": {text}}
	req := httptest.NewRequest(http.MethodPost, "/api/reflow", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
