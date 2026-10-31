package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"nib/internal/pdfread"
)

// A document refused for how it is built is the document's fault: 422 and a cause, whichever status the handler
// reached for (/pending 919 F12).
func TestAStructureRefusalAnswers422WithItsCause(t *testing.T) {
	want := []error{pdfread.ErrNestedTooDeep, pdfread.ErrReferenceCycle, pdfread.ErrNodeNamedTwice, pdfread.ErrReferenceDepth, pdfread.ErrReferencePaths}
	var sentences []string
	for _, e := range want {
		sentences = append(sentences, e.Error())
	}
	if got := pdfread.StructureRefusals(); !reflect.DeepEqual(got, sentences) {
		t.Fatalf("the door recognises %q, want the five refusals %q", got, sentences)
	}
	for _, e := range sentences {
		rr := httptest.NewRecorder()
		httpError(rr, http.StatusBadRequest, "could not read: "+e+" (obj#7)")
		var body map[string]string
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		if rr.Code != http.StatusUnprocessableEntity || body["cause"] != "document-structure" {
			t.Errorf("%q: got %d %v", e, rr.Code, body)
		}
	}
	rr := httptest.NewRecorder()
	httpError(rr, http.StatusBadRequest, "could not read: some other error")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("an unrelated error changed status: %d", rr.Code)
	}
}
