package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestAGainDoesNotMaskALoss — /pending 582.
//
// The door compared a document's annotation TOTALS before and after. An append or an insert brings
// the other document's annotations with it, so an operation that dropped three of the original's
// and carried three in left the totals equal and the notice silent — exactly when something was
// destroyed. The door is told what was brought in and counts against that.
func TestAGainDoesNotMaskALoss(t *testing.T) {
	three := notedFixture(t, 3)
	// SETUP: the fixture holds what the arithmetic below assumes.
	if got := pdfops.Inspect(three); !got.Readable || got.Annotations != 3 {
		t.Fatalf("setup: the fixture reports %+v, want 3 readable annotations", got)
	}

	// Three before, three brought in, three afterwards: three are gone, whichever three they were.
	doc := &document{}
	noteTaggingFate(doc, three, three, 3)
	if doc.lostAnnots != 3 {
		t.Errorf("an operation that brought in 3 annotations and left the document holding 3 of 6 "+
			"reported %d lost, want 3 — the gain hid the loss", doc.lostAnnots)
	}

	// CONTROL: brought in and all carried is no loss, or the figure above is just the brought count.
	doc = &document{}
	noteTaggingFate(doc, three, notedFixture(t, 6), 3)
	if doc.lostAnnots != 0 {
		t.Errorf("3 annotations plus 3 brought in, 6 afterwards, reported %d lost, want 0", doc.lostAnnots)
	}
}

// TestAnAppendThatCarriesEverythingReportsNoLoss — the route's half of /pending 582.
//
// `/api/pages` counts what the appended document carries and hands it to the door. Append carries
// every note today, so the honest answer is "nothing lost" — and this holds the count the route
// takes to what the operation really carries: an over-count would report a loss on every append.
func TestAnAppendThatCarriesEverythingReportsNoLoss(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	src, other := notedFixture(t, 3), notedFixture(t, 2)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open",
		openRequest{Path: writeTemp(t, src)}); code != http.StatusOK {
		t.Fatalf("setup: open returned %d: %s", code, body)
	}
	if got, _ := broughtIn(other); got != 2 {
		t.Fatalf("setup: the appended document is counted as carrying %d annotations, want 2", got)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("op", "append")
	fw, _ := mw.CreateFormFile("pdf", "in.pdf")
	fw.Write(src)
	fw, _ = mw.CreateFormFile("append", "other.pdf")
	fw.Write(other)
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("append returned %d: %s", resp.StatusCode, body)
	}
	var meta docResponse
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the append really carried the notes, or "no loss" is true of a result nobody read.
	if got := pdfops.Inspect(fetchPDF(t, ts, c)); got.Annotations != 5 {
		t.Fatalf("setup: the appended result holds %d annotations, want 5", got.Annotations)
	}
	if meta.LostAnnots != 0 {
		t.Errorf("an append that carried all 5 annotations reported %d lost", meta.LostAnnots)
	}
}

// TestFormFieldsAreComparedByNameNotByCount — /pending 582, ADR-147.
//
// The door compared the LENGTH of the form's top-level field array before and after. That figure cannot
// see a field dropped beside one added, and it is not the number of fields at all: a form appended to
// itself goes from 2 entries to 3 because pdfcpu puts the incoming two under one new parent, while all four
// fields stand. So nothing merged, nothing is reported as merged, and the comparison is by name.
func TestFormFieldsAreComparedByNameNotByCount(t *testing.T) {
	cases := []struct {
		name          string
		before, after []string
		brought, want int
	}{
		{"nothing changed", []string{"a", "b"}, []string{"a", "b"}, 0, 0},
		{"one dropped", []string{"a", "b"}, []string{"a"}, 0, 1},
		{"one dropped and another added — the totals are equal", []string{"a", "b"}, []string{"a", "c"}, 0, 1},
		{"two brought in and both arrived under new names", []string{"a", "b"}, []string{"a", "b", "2.a", "2.b"}, 2, 0},
		{"two brought in, one arrived", []string{"a", "b"}, []string{"a", "b", "2.a"}, 2, 1},
		{"two brought in while one of the document's own went", []string{"a", "b"}, []string{"a", "2.a", "2.b"}, 2, 1},
		{"two fields sharing a name, one gone", []string{"a", "a"}, []string{"a"}, 0, 1},
		{"fields added and none brought is not a loss", []string{"a"}, []string{"a", "b"}, 0, 0},
	}
	for _, c := range cases {
		if got := lostFieldsByName(c.before, c.after, c.brought); got != c.want {
			t.Errorf("%s: %d lost, want %d", c.name, got, c.want)
		}
	}

	// The door, on real bytes: a form appended to itself loses nothing, and the route is told what came in.
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	doubled, err := pdfops.Append(form, form)
	if err != nil {
		t.Fatal(err)
	}
	before, after := pdfops.Inspect(form), pdfops.Inspect(doubled)
	if len(before.FieldNames) != 2 || len(after.FieldNames) != 4 {
		t.Fatalf("setup: the form reads %q and its self-append %q — want 2 fields, then 4", before.FieldNames, after.FieldNames)
	}
	_, broughtFields := broughtIn(form)
	if f := fateOf(form, doubled, 0, broughtFields); f.lostFields != 0 {
		t.Errorf("a form appended to itself reports %d fields lost — all four are there", f.lostFields)
	}
	// And the same append where the document's own fields did not survive is a loss the totals hid:
	// 2 before, 2 brought, and a result holding only the 2 that came in.
	if f := fateOf(doubled, form, 0, 0); f.lostFields != 2 {
		t.Errorf("a result missing two of the document's four fields reports %d lost, want 2", f.lostFields)
	}
}
