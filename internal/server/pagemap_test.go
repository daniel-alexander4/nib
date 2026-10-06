package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestThePageMapRouteAnswersForTheOpenDocument — ADR-088. The map is of the document the server holds, and a page it
// cannot map is a 422 naming its cause, so the caller can fall back and say so (ADR-072).
func TestThePageMapRouteAnswersForTheOpenDocument(t *testing.T) {
	pdf, err := testpdf.Text("first line", "second line")
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	rec := httptest.NewRecorder()
	s.handlePageMap(rec, httptest.NewRequest(http.MethodGet, "/api/pagemap?page=1", nil))
	var m pdfops.PageMap
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("the map = %d (%v)", rec.Code, err)
	}
	if m.Page != 1 || m.Width <= 0 || m.Height <= 0 || len(m.Text) == 0 || m.NoText {
		t.Errorf("the map of a page with text = page %d, %gx%g, %d runs, noText %v", m.Page, m.Width, m.Height, len(m.Text), m.NoText)
	}
	if m.Shapes == nil || m.Widgets == nil {
		t.Error("an empty list was sent as null; the client reads both as lists")
	}

	rec = httptest.NewRecorder()
	s.handlePageMap(rec, httptest.NewRequest(http.MethodGet, "/api/pagemap?page=99", nil))
	var refusal struct{ Cause string }
	if err := json.NewDecoder(rec.Body).Decode(&refusal); err != nil || rec.Code != http.StatusUnprocessableEntity || refusal.Cause != "page-unreadable" {
		t.Errorf("a page the document does not have = %d %q (%v), want 422 page-unreadable", rec.Code, refusal.Cause, err)
	}
	rec = httptest.NewRecorder()
	s.handlePageMap(rec, httptest.NewRequest(http.MethodGet, "/api/pagemap?page=x", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a page that is not a number = %d, want 400", rec.Code)
	}
}
