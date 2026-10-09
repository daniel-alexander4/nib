package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// ocrRequest posts an OCR body to the route and returns what it answered.
func ocrRequest(s *Server, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	s.handleOCR(rec, httptest.NewRequest(http.MethodPost, "/api/ocr", bytes.NewReader(raw)))
	return rec
}

// hiddenWordsOn is the invisible words the page map reads on one page of the open document.
func hiddenWordsOn(t *testing.T, s *Server, page int) []string {
	t.Helper()
	m, err := pdfops.MapPage(s.docBytes(s.activeDoc()), page)
	if err != nil {
		t.Fatalf("page %d: %v", page, err)
	}
	out := []string{}
	for _, tx := range m.Text {
		if tx.Hidden {
			out = append(out, tx.Text)
		}
	}
	sort.Strings(out)
	return out
}

// TestTheWindowIsToldWhichPagesNothingHasRead — ADR-106. A command that needs a page's text asks here which pages to
// read for it: the ones that set no text and are not blank, and none once a page has been read.
func TestTheWindowIsToldWhichPagesNothingHasRead(t *testing.T) {
	const helv = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
	typed, err := testpdf.Text("typed")
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := pdfops.Combine([][]byte{typed, testpdf.WithContent("72 72 400 300 re f", helv), testpdf.WithContent("", helv)})
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	unread := func() []int {
		pr := httptest.NewRecorder()
		s.handleOCRPages(pr, httptest.NewRequest(http.MethodGet, "/api/ocr/pages", nil))
		var told struct{ Unread []int }
		if err := json.NewDecoder(pr.Body).Decode(&told); err != nil || pr.Code != http.StatusOK || told.Unread == nil {
			t.Fatalf("GET /api/ocr/pages = %d, unread %v (%v)", pr.Code, told.Unread, err)
		}
		return told.Unread
	}
	if got := unread(); !reflect.DeepEqual(got, []int{2}) {
		t.Errorf("unread = %v, want [2]: page 1 has text and page 3 is blank", got)
	}
	word := pdfops.Word{Page: 2, Text: "Invoice", Rect: [4]float64{100, 400, 180, 412}, Block: 1, Para: 1, Line: 1}
	if rec := ocrRequest(s, map[string]any{"lang": "eng", "words": []pdfops.Word{word}}); rec.Code != http.StatusOK {
		t.Fatalf("the OCR = %d: %s", rec.Code, rec.Body.String())
	}
	if got := unread(); len(got) != 0 {
		t.Errorf("unread = %v after page 2 was read, want none", got)
	}
}

// TestATextLayerIsReplacedOnlyWhenAskedAndOnlyWhereItIsNibsOwn — ADR-101. A second OCR still leaves a layered page
// alone (ADR-094); the same words sent with `replace` take Nib's own layer off that page and stamp them in its
// place — once each — in one undo step; and a page whose invisible text Nib did not stamp is left as it is and
// named with the cause, whatever was asked.
func TestATextLayerIsReplacedOnlyWhenAskedAndOnlyWhereItIsNibsOwn(t *testing.T) {
	// Page 1 will carry Nib's layer; page 2 carries another program's: invisible text set straight on the page.
	const helv = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
	own, err := testpdf.Text("scan one")
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := pdfops.Combine([][]byte{own, testpdf.WithContent("BT /F1 12 Tf 3 Tr 72 700 Td (theirs) Tj ET", helv)})
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	old := pdfops.Word{Page: 1, Text: "Invoice", Rect: [4]float64{100, 400, 180, 412}, Block: 1, Para: 2, Line: 3}
	if rec := ocrRequest(s, map[string]any{"lang": "eng", "words": []pdfops.Word{old}}); rec.Code != http.StatusOK {
		t.Fatalf("the first OCR = %d: %s", rec.Code, rec.Body.String())
	}
	layered := s.docBytes(s.activeDoc())
	undoDepth := len(s.activeDoc().undo)

	// The window is told which layered pages are Nib's own, by the rule the route will apply.
	pr := httptest.NewRecorder()
	s.handleOCRPages(pr, httptest.NewRequest(http.MethodGet, "/api/ocr/pages", nil))
	var told struct{ Layered, Own []int }
	if err := json.NewDecoder(pr.Body).Decode(&told); err != nil || !reflect.DeepEqual(told.Layered, []int{1, 2}) || !reflect.DeepEqual(told.Own, []int{1}) {
		t.Errorf("GET /api/ocr/pages = %+v (%v), want layered [1 2] and own [1]", told, err)
	}

	fresh := []pdfops.Word{
		{Page: 1, Text: "Rechnung", Rect: [4]float64{100, 400, 190, 412}, Block: 1, Para: 2, Line: 3},
		{Page: 2, Text: "Seite", Rect: [4]float64{100, 400, 150, 412}, Block: 1, Para: 2, Line: 3},
	}
	// Not asked: nothing changes (ADR-094 stands).
	if rec := ocrRequest(s, map[string]any{"lang": "deu", "words": fresh}); rec.Code != http.StatusOK || !bytes.Equal(layered, s.docBytes(s.activeDoc())) {
		t.Fatalf("an OCR that did not ask to replace changed a layered document (%d)", rec.Code)
	}

	rec := ocrRequest(s, map[string]any{"lang": "deu", "words": fresh, "replace": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("the replace = %d: %s", rec.Code, rec.Body.String())
	}
	if got := hiddenWordsOn(t, s, 1); !reflect.DeepEqual(got, []string{"Rechnung"}) {
		t.Errorf("page 1 holds the invisible words %v, want only the new one — the old layer is still there", got)
	}
	if got := hiddenWordsOn(t, s, 2); !reflect.DeepEqual(got, []string{"theirs"}) {
		t.Errorf("page 2 holds the invisible words %v, want another program's layer exactly as it was", got)
	}
	var facts ocrFacts
	if err := json.Unmarshal([]byte(rec.Header().Get("X-Nib-OCR")), &facts); err != nil ||
		!reflect.DeepEqual(facts.Replaced, []int{1}) || !reflect.DeepEqual(facts.Skipped, []int{2}) ||
		facts.Causes["2"] != string(pdfops.LayerOther) {
		t.Errorf("X-Nib-OCR = %q (%v), want page 1 replaced and page 2 skipped as %q", rec.Header().Get("X-Nib-OCR"), err, pdfops.LayerOther)
	}
	if got := len(s.activeDoc().undo); got != undoDepth+1 {
		t.Errorf("the replace made %d undo steps, want one", got-undoDepth)
	}
	ur := httptest.NewRecorder()
	s.handleUndo(ur, httptest.NewRequest(http.MethodPost, "/api/undo", nil))
	if ur.Code != http.StatusOK || !bytes.Equal(layered, s.docBytes(s.activeDoc())) {
		t.Errorf("undo (%d) did not bring back the document with its old layer", ur.Code)
	}
}

// TestASignedDocumentsTextLayerIsNeverReplaced — taking a layer out rewrites pages the signature covers. Refused at
// the server door with the cause, the bytes untouched, and the window is told no page can be read again.
func TestASignedDocumentsTextLayerIsNeverReplaced(t *testing.T) {
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	word := pdfops.Word{Page: 1, Text: "Invoice", Rect: [4]float64{100, 400, 180, 412}, Block: 1, Para: 2, Line: 3}
	layered, _, err := pdfops.TagOCRLayer(base, []pdfops.Word{word}, "eng")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := sign.GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.Sign(layered, certPEM, keyPEM, sign.Options{Name: "Nib Test", When: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	// Unsigned, the same page IS Nib's own — so what is refused below is the signature, not the layer.
	u := openTestServer(t, layered)
	ur := httptest.NewRecorder()
	u.handleOCRPages(ur, httptest.NewRequest(http.MethodGet, "/api/ocr/pages", nil))
	var unsigned struct{ Own []int }
	if err := json.NewDecoder(ur.Body).Decode(&unsigned); err != nil || !reflect.DeepEqual(unsigned.Own, []int{1}) {
		t.Fatalf("before signing, own = %v (%v), want [1]", unsigned.Own, err)
	}

	s := openTestServer(t, signed)
	rec := ocrRequest(s, map[string]any{"lang": "eng", "words": []pdfops.Word{word}, "replace": true})
	var refusal struct{ Cause string }
	if err := json.NewDecoder(rec.Body).Decode(&refusal); err != nil || rec.Code != http.StatusUnprocessableEntity || refusal.Cause != "signed" {
		t.Errorf("a replace on a signed document = %d %+v (%v), want 422 with cause signed", rec.Code, refusal, err)
	}
	if !bytes.Equal(signed, s.docBytes(s.activeDoc())) {
		t.Errorf("the refused replace changed the signed document's bytes")
	}
	pr := httptest.NewRecorder()
	s.handleOCRPages(pr, httptest.NewRequest(http.MethodGet, "/api/ocr/pages", nil))
	var told struct{ Layered, Own []int }
	if err := json.NewDecoder(pr.Body).Decode(&told); err != nil || !reflect.DeepEqual(told.Layered, []int{1}) || len(told.Own) != 0 {
		t.Errorf("GET /api/ocr/pages on a signed document = %+v (%v), want page 1 layered and none to read again", told, err)
	}
}

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
