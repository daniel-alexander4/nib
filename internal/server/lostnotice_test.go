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
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// notedFixture is an n-page document carrying one sticky note per page.
func notedFixture(t *testing.T, pages int) []byte {
	t.Helper()
	names := make([]string, pages)
	notes := make([]pdfops.Note, pages)
	for i := range names {
		names[i] = "page"
		notes[i] = pdfops.Note{Page: i + 1, X: 100, Y: 700, Text: "a comment"}
	}
	base, err := testpdf.Text(names...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pdfops.AddNotes(base, notes)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// writeTemp puts bytes on disk and returns the path, for a route that opens by path.
func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "noted.pdf")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAPageOperationSaysWhatItDestroyed — /pending 574.
//
// `grep -rn "notice\|warning" internal/server/pages.go` returned NOTHING: a page operation answered
// with the new document and had no channel for "and here is what it cost you". `/pending 524` settled
// on *"prefer the remap — the warning is the acceptable floor, not the fix"* and the floor was never
// built; `/pending 573`'s form widgets cannot be carried at all, so for them the floor is all there
// is.
//
// **Driven through the real route and read off the real response**, because the entry's question was
// about the CHANNEL, not about a counting function: it asked whether `/api/pages` grows a notices
// array or whether the client diffs before and after. Neither — the two commit doors already compare
// a document's before and after for the tagging claim, and their own comment rules on the design:
// *"Asking each operation to report its own fate would be ADR-009's rule inverted: 33 sites that have
// to remember, against two that cannot."*
func TestAPageOperationSaysWhatItDestroyed(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// A three-page document with a sticky note on each. An n-up composes them onto sheets and
	// carries `/Text` annotations — but a DELETE of the page a note sits on destroys it, which is
	// the plainest case a user can reach and the one no sentence covered.
	src := notedFixture(t, 3)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open",
		openRequest{Path: writeTemp(t, src)}); code != http.StatusOK {
		t.Fatalf("setup: open returned %d: %s", code, body)
	}

	// SETUP: the document really carries the annotations, or "two fewer afterwards" is true of a
	// document that never had any.
	if got := pdfops.Inspect(src); !got.Readable || got.Annotations != 3 {
		t.Fatalf("setup: the fixture reports %+v, want 3 readable annotations", got)
	}

	before := docMetaOf(t, c, ts.URL)
	if before.LostAnnots != 0 {
		t.Fatalf("setup: a freshly opened document already reports %d lost annotation(s)", before.LostAnnots)
	}

	after := pagesOp(t, ts, c, csrf, src, "delete", "2")
	if after.LostAnnots != 1 {
		t.Errorf("deleting a page carrying a comment reported %d lost annotation(s), want 1. The "+
			"route answers with the new document and nothing tells the user their comment is gone",
			after.LostAnnots)
	}

	// **It ACCUMULATES**, which is the difference between this and the sticky boolean beside it.
	// A second operation that destroys something costs the user both, and a per-operation figure
	// would report only the last one.
	second := pagesOp(t, ts, c, csrf, after.data, "delete", "1")
	if second.LostAnnots != 2 {
		t.Errorf("a second destroying operation reported %d lost annotation(s) in total, want 2 — "+
			"the count latched or reset instead of accumulating", second.LostAnnots)
	}

	// CONTROL: an operation that destroys nothing adds nothing. Without it every assertion above is
	// equally satisfied by a counter that increments on any commit at all.
	third := pagesOp(t, ts, c, csrf, second.data, "rotate", "1")
	if third.LostAnnots != 2 {
		t.Errorf("a rotate — which destroys no annotation — took the total to %d from 2. A count "+
			"that rises on an operation that lost nothing is a notice the user learns to ignore",
			third.LostAnnots)
	}
}

// TestAnUnreadableSideIsNotReportedAsALoss — the half a diffing door gets wrong by omission.
//
// `pdfops.Inspect` reports `Readable:false` for bytes it cannot parse, and every other field is then
// meaningless. Reading "0 annotations" off such a side would turn every unparseable input into a
// report that the user lost everything they had.
func TestAnUnreadableSideIsNotReportedAsALoss(t *testing.T) {
	doc := &document{}
	src := notedFixture(t, 2)

	// SETUP: the readable pair really does count a loss, or "the unreadable pair counts none" is
	// true of a function that never counts anything.
	noteTaggingFate(doc, src, notedFixture(t, 1))
	if doc.lostAnnots != 1 {
		t.Fatalf("setup: two notes to one reported %d lost, want 1", doc.lostAnnots)
	}

	doc.lostAnnots = 0
	noteTaggingFate(doc, src, []byte("this is not a PDF"))
	if doc.lostAnnots != 0 {
		t.Errorf("an unparseable RESULT was reported as losing %d annotation(s) — the door read "+
			"zero off bytes it could not open", doc.lostAnnots)
	}
	noteTaggingFate(doc, []byte("this is not a PDF"), src)
	if doc.lostAnnots != 0 {
		t.Errorf("an unparseable INPUT was reported as a loss of %d", doc.lostAnnots)
	}
}

// docMetaOf reads the open document's metadata off /api/status.
func docMetaOf(t *testing.T, c *http.Client, base string) docResponse {
	t.Helper()
	var out docResponse
	sessGet(t, c, base+"/api/status", &out)
	return out
}

type pagesResult struct {
	docResponse
	data []byte
}

// pagesOp posts one page operation and returns the response plus the resulting bytes, so a caller
// can chain operations the way a user does.
func pagesOp(t *testing.T, ts *httptest.Server, c *http.Client, csrf string, pdf []byte, op, pages string) pagesResult {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("op", op)
	mw.WriteField("pages", pages)
	if op == "rotate" {
		mw.WriteField("deg", "90")
	}
	fw, _ := mw.CreateFormFile("pdf", "in.pdf")
	fw.Write(pdf)
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s returned %d: %s", op, resp.StatusCode, body)
	}
	var out pagesResult
	if err := json.Unmarshal(body, &out.docResponse); err != nil {
		t.Fatal(err)
	}
	out.data = fetchPDF(t, ts, c)
	return out
}
