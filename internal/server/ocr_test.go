package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// hiddenRunsOn counts the invisible text runs the page map reads on each page of the open document.
func hiddenRunsOn(t *testing.T, s *Server, pages int) []int {
	t.Helper()
	out := make([]int, pages)
	for p := 1; p <= pages; p++ {
		m, err := pdfops.MapPage(s.docBytes(s.activeDoc()), p)
		if err != nil {
			t.Fatalf("page %d: %v", p, err)
		}
		for _, tx := range m.Text {
			if tx.Hidden {
				out[p-1]++
			}
		}
	}
	return out
}

// TestASecondOCRDoesNotAddASecondTextLayer — /pending 851 part 4. Running OCR on a document that had already been
// OCR'd stamped every word again: one word, two invisible runs, found twice by search and copied twice. A page that
// already has a text layer is now left as it is, the route says which pages those were, and a page that has none
// still gets its layer in the same request.
func TestASecondOCRDoesNotAddASecondTextLayer(t *testing.T) {
	pdf, err := testpdf.Text("scan one", "scan two")
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	ocr := func(words ...pdfops.Word) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"lang": "eng", "words": words})
		rec := httptest.NewRecorder()
		s.handleOCR(rec, httptest.NewRequest(http.MethodPost, "/api/ocr", bytes.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("the OCR route = %d: %s", rec.Code, rec.Body.String())
		}
		return rec
	}
	one := pdfops.Word{Page: 1, Text: "Invoice", Rect: [4]float64{100, 400, 180, 412}, Block: 1, Para: 2, Line: 3}
	two := pdfops.Word{Page: 2, Text: "Total", Rect: [4]float64{100, 400, 150, 412}, Block: 1, Para: 2, Line: 3}

	if rec := ocr(one); rec.Header().Get("X-Nib-OCR") != "" {
		t.Errorf("a first OCR said pages were left alone: %s", rec.Header().Get("X-Nib-OCR"))
	}
	if got := hiddenRunsOn(t, s, 2); got[0] != 1 || got[1] != 0 {
		t.Fatalf("after the first OCR the pages hold %v invisible runs, want [1 0] invisible", got)
	}
	afterFirst := s.docBytes(s.activeDoc())

	// The same page again, and a page that has not been read yet, in one request.
	rec := ocr(one, two)
	if got := hiddenRunsOn(t, s, 2); got[0] != 1 || got[1] != 1 {
		t.Errorf("after the second OCR the pages hold %v invisible runs, want [1 1]: page 1 keeps its one layer and page 2 gets its own", got)
	}
	var facts struct {
		Skipped []int `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(rec.Header().Get("X-Nib-OCR")), &facts); err != nil || len(facts.Skipped) != 1 || facts.Skipped[0] != 1 {
		t.Errorf("X-Nib-OCR = %q (%v), want page 1 named as left alone", rec.Header().Get("X-Nib-OCR"), err)
	}

	// And the window is told the same thing before it reads a page, by the same rule.
	pr := httptest.NewRecorder()
	s.handleOCRPages(pr, httptest.NewRequest(http.MethodGet, "/api/ocr/pages", nil))
	var told struct {
		Layered []int `json:"layered"`
	}
	if err := json.NewDecoder(pr.Body).Decode(&told); err != nil || pr.Code != http.StatusOK || len(told.Layered) != 2 || told.Layered[0] != 1 || told.Layered[1] != 2 {
		t.Errorf("GET /api/ocr/pages = %d %v (%v), want pages 1 and 2", pr.Code, told.Layered, err)
	}

	// Nothing left to add: the document is not rewritten at all, so there is nothing to undo.
	before := s.docBytes(s.activeDoc())
	ocr(one, two)
	if !bytes.Equal(before, s.docBytes(s.activeDoc())) {
		t.Error("an OCR with nothing to add rewrote the document")
	}
	if bytes.Equal(afterFirst, before) {
		t.Error("the second OCR did not add page 2's layer: the test's last step proves nothing")
	}
}
