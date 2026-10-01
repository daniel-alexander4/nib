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
	"strconv"
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
	if start < 0 {
		t.Fatal("web/app.js has no REFLOW_CAUSES block")
	}
	end := strings.Index(src[start:], "\n};")
	if end < 0 {
		t.Fatal("web/app.js's REFLOW_CAUSES block does not close")
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

// flowLine is paragraph k of page p's line in flowFixture.
func flowLine(p, k int) string {
	return "Page " + strconv.Itoa(p) + " paragraph " + strconv.Itoa(k) + " words run on"
}

// flowFixture is two pages of two-line Helvetica paragraphs, 38 points apart from y 700: sixteen on page 1, which fill
// it, and three on page 2.
func flowFixture() []byte {
	page := func(p, n int) string {
		var b strings.Builder
		for k := 0; k < n; k++ {
			b.WriteString("BT /F1 12 Tf 14 TL 72 " + strconv.Itoa(700-38*k) + " Td (" + flowLine(p, k) + ") Tj T* (" + flowLine(p, k) + ") Tj ET\n")
		}
		return b.String()
	}
	c1, c2 := page(1, 16), page(2, 3)
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 5 0 R >>",
		4: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>",
		5: "<< /Length " + strconv.Itoa(len(c1)) + " >>\nstream\n" + c1 + "\nendstream",
		6: "<< /Length " + strconv.Itoa(len(c2)) + " >>\nstream\n" + c2 + "\nendstream",
		7: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	})
}

// TestAFlowAcrossPagesCommitsAndUndoesAsOne — the P07 phase-close review: a growth that flows page 1's last paragraph
// onto page 2 commits through the server door as ONE mutation, and one Undo puts BOTH pages back — the document's bytes
// exactly as they were.
func TestAFlowAcrossPagesCommitsAndUndoesAsOne(t *testing.T) {
	pdf := flowFixture()
	s := openTestServer(t, pdf)
	orig := firstParagraph(t, s)
	l := flowLine(1, 0)
	rec := postReflow(t, s, orig, orig+" "+l+" "+l+" "+l)
	var out reflowResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); rec.Code != http.StatusOK || err != nil || !out.Ok {
		t.Fatalf("reflow: %d %+v (%v)", rec.Code, out, err)
	}
	p2, err := pdfops.Paragraphs(s.docBytes(s.activeDoc()), 2)
	if err != nil || len(p2) == 0 || p2[0].Text != flowLine(1, 15)+" "+flowLine(1, 15) {
		t.Fatalf("page 2 does not open with page 1's last paragraph after the flow: %+v (%v)", p2, err)
	}
	s.handleUndo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/undo", nil))
	if got := s.docBytes(s.activeDoc()); !bytes.Equal(got, pdf) {
		t.Error("one Undo did not restore both pages of the flow")
	}
}

// TestARefusalNamesTheParagraphBelow — the P07 phase-close review: when a paragraph below the edited one cannot move with
// it, the server's answer carries that paragraph's text in `below`, which the dialog names; nothing had sent one end to
// end. The paragraph below here — one pitch under it, inside the region — is drawn inside a form XObject, which a move
// refuses.
func TestARefusalNamesTheParagraphBelow(t *testing.T) {
	form := "BT /F1 12 Tf 14 TL 72 648 Td (Words run across the column here and wrap) Tj T* (Formed words here) Tj ET"
	content := "BT /F1 12 Tf 14 TL 72 700 Td (Words run across the column here and wrap) Tj T* (Words run across the column here and wrap) Tj T* (Words run across the column here and wrap) Tj ET q /Fm1 Do Q"
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /XObject << /Fm1 6 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		6: "<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Length " + strconv.Itoa(len(form)) + " >>\nstream\n" + form + "\nendstream",
	})
	s := openTestServer(t, pdf)
	orig := firstParagraph(t, s)
	rec := postReflow(t, s, orig, orig+" Words run across the column here and wrap")
	var out reflowResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Ok || out.Cause != "text-in-form" || out.Below != "Words run across the column here and wrap Formed words here" {
		t.Errorf("answered %d %+v (%v), want a text-in-form refusal naming the formed paragraph below", rec.Code, out, err)
	}
	if got := s.docBytes(s.activeDoc()); !bytes.Equal(got, pdf) {
		t.Error("the refused reflow changed the document")
	}
}
