package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nib/internal/pdfops"
)

// TestEveryStampProducerReportsUnrepresentableTextAsTheUsersToFix drives the ONE fact the
// four producers must agree on: text pdfcpu cannot bake is a 400 carrying the sentence that
// names the fix.
//
// It is written over the producers rather than over the handlers because the defect was a
// hand-mirrored mapping — reached by two of four — and a per-handler test would have been
// written for the two that already had it. See wroteStampTextError.
func TestEveryStampProducerReportsUnrepresentableTextAsTheUsersToFix(t *testing.T) {
	base := threePagePDF(t)
	// `%%` is the case pdfops names: pdfcpu advances one character and bakes something the
	// user did not type. Every producer that renders user text must refuse it.
	const bad = "100%% done"

	producers := map[string]func() ([]byte, error){
		"StampWatermark": func() ([]byte, error) {
			return pdfops.StampWatermark(base, bad, pdfops.WatermarkStyle{})
		},
		"StampPageNumbers": func() ([]byte, error) {
			return pdfops.StampPageNumbers(base, pdfops.PageNumberStyle{Prefix: bad})
		},
	}
	for name, produce := range producers {
		t.Run(name, func(t *testing.T) {
			_, err := produce()
			if err == nil {
				t.Fatalf("%s baked %q without complaint — the stimulus this test grades never happened,"+
					" so the mapping below is graded against a nil error", name, bad)
			}
			rr := httptest.NewRecorder()
			if !wroteStampTextError(rr, err) {
				t.Fatalf("%s's error was not routed to the user: %v\n"+
					"the sentence names the one character they can change, and the door discarded it", name, err)
			}
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("%s: status %d, want 400 — a 5xx tells the user their document is broken", name, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "cannot be stamped as written") {
				t.Fatalf("%s: body %q does not carry the explanation", name, rr.Body.String())
			}
		})
	}
}

// TestAPathlessDocumentIsNamedByWhereItCameFrom covers the two producers that were still
// rendering "Untitled" after a reload: a URL-opened document and a co-signed arrival.
func TestAPathlessDocumentIsNamedByWhereItCameFrom(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://example.org/deed-of-sale.pdf", "deed-of-sale.pdf"},
		{"https://example.org/a/b/contract.pdf?v=2", "contract.pdf"},
		{"https://example.org/", "downloaded.pdf"},
		{"https://example.org", "downloaded.pdf"},
		{"://not a url", "downloaded.pdf"},
	} {
		if got := urlDocName(tc.in); got != tc.want {
			t.Errorf("urlDocName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// `nil` ceremony is the manual/two-party path, whose name P06.S09 deliberately left alone: an
	// ordinary co-sign IS finished when it arrives. The ceremony cases are
	// TestAnInProgressCopyIsNotNamedAsTheFinishedDocument.
	if got := arrivalDocName("Marta", nil, nil); !strings.Contains(got, "Marta") || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("arrivalDocName(%q) = %q — an arrival's one identifying fact is who sent it", "Marta", got)
	}
	if got := arrivalDocName("", nil, nil); got == "" || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("arrivalDocName(\"\") = %q — an unlabelled peer must still not produce an empty name", got)
	}
	// Neither may return the empty string, because the empty string is what renders "Untitled".
	if urlDocName("https://example.org/") == "" || arrivalDocName("x", nil, nil) == "" {
		t.Fatal("a name producer returned \"\" — the exact value that renders as Untitled")
	}
}
