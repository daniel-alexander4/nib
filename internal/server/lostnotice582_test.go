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

// twoNotes is one page carrying two sticky notes with the given texts.
func twoNotes(t *testing.T, first, second string) []byte {
	t.Helper()
	base, err := testpdf.Text("page")
	if err != nil {
		t.Fatal(err)
	}
	out, err := pdfops.AddNotes(base, []pdfops.Note{{Page: 1, X: 100, Y: 700, Text: first}, {Page: 1, X: 200, Y: 600, Text: second}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAnAnnotationWrittenDoesNotMaskOneDropped — /pending 875 (1).
//
// Inside one document the door still compared TOTALS: an operation that dropped one annotation and wrote
// another left two before and two after, and the notice said nothing about the comment that was destroyed.
// Annotations are compared by what they are now, as form fields have been since /pending 582.
func TestAnAnnotationWrittenDoesNotMaskOneDropped(t *testing.T) {
	before := twoNotes(t, "kept", "dropped by the operation")
	after := twoNotes(t, "kept", "written by the operation")
	if b, a := pdfops.Inspect(before), pdfops.Inspect(after); b.Annotations != 2 || a.Annotations != 2 {
		t.Fatalf("setup: %d annotation(s) before and %d after, want 2 and 2 — equal totals are the case", b.Annotations, a.Annotations)
	}
	if got := fateOf(before, after, 0, 0).lostAnnots; got != 1 {
		t.Errorf("one annotation dropped and one written reported %d lost, want 1 — the totals hid it", got)
	}
	// CONTROLS: the same annotations are no loss, whatever order the result holds them in.
	if got := fateOf(before, twoNotes(t, "dropped by the operation", "kept"), 0, 0).lostAnnots; got != 0 {
		t.Errorf("the same two annotations in another order reported %d lost, want 0", got)
	}
	if got := fateOf(before, before, 0, 0).lostAnnots; got != 0 {
		t.Errorf("an operation that changed nothing reported %d lost, want 0", got)
	}
}

// TestAnnotationsThatCouldNotBeKeyedAreComparedByCount — a side with no keys (more annotations than
// `pdfops` keys) is never read as "holds none of them": the door falls back to the totals.
func TestAnnotationsThatCouldNotBeKeyedAreComparedByCount(t *testing.T) {
	keyed := pdfops.DocumentFacts{Readable: true, Annotations: 3, AnnotationKeys: []string{"a", "b", "c"}}
	unkeyed := pdfops.DocumentFacts{Readable: true, Annotations: 3}
	if got := lostAnnotations(keyed, unkeyed, 0); got != 0 {
		t.Errorf("3 annotations before and 3 unkeyed after reported %d lost, want 0", got)
	}
	one := pdfops.DocumentFacts{Readable: true, Annotations: 1, AnnotationKeys: []string{"a"}}
	if got := lostAnnotations(unkeyed, one, 0); got != 2 {
		t.Errorf("3 unkeyed before and 1 after reported %d lost, want 2", got)
	}
}

// TestTwoFormsSharingAFieldNameLoseNothing — /pending 875 (2), measured.
//
// What pdfcpu does when the appended form shares a field name with the document's was unmeasured; the
// by-name comparison is only right if the document's own names survive. They do: every incoming form is
// filed under a new parent named by its ordinal, collision or none, so nothing merges and nothing of the
// document's is renamed. This is that measurement, held: if pdfcpu starts merging same-named fields, the
// names below change and the door's arithmetic has to be looked at again.
func TestTwoFormsSharingAFieldNameLoseNothing(t *testing.T) {
	form := func(names ...string) []byte {
		base, err := testpdf.Text("page")
		if err != nil {
			t.Fatal(err)
		}
		var fields []pdfops.FormField
		for i, n := range names {
			fields = append(fields, pdfops.FormField{Page: 1, Kind: "text", Name: n,
				Rect: [4]float64{100, 100 + 30*float64(i), 200, 120 + 30*float64(i)}})
		}
		out, err := pdfops.AuthorForm(base, fields)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	doc, other := form("fullName", "agree"), form("fullName", "email")
	out, err := pdfops.Append(doc, other)
	if err != nil {
		t.Fatal(err)
	}
	got := pdfops.Inspect(out).FieldNames
	want := []string{"fullName", "agree", "2.fullName", "2.email"}
	if len(got) != len(want) {
		t.Fatalf("two forms sharing a field name appended to %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("two forms sharing a field name appended to %q, want %q", got, want)
		}
	}
	annots, fields := broughtIn(other)
	if f := fateOf(doc, out, annots, fields); f.lostFields != 0 || f.lostAnnots != 0 {
		t.Errorf("appending a form that shares a field name reported %d field(s) and %d annotation(s) "+
			"lost, want none: all four fields and all four widgets are in the result", f.lostFields, f.lostAnnots)
	}
}
