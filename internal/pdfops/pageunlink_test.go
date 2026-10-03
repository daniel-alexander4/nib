package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 726` and `/pending 765` — `unlinkDroppedPages`, the one door between what a page selection
// keeps and the pages it drops.

// roadDoc is two pages. Page 1 draws secretText and carries annotation 10, a comment; page 2 draws
// publicText and carries page2Annots. extra adds or replaces objects, so each case is the one road it
// names from page 2 back to page 1.
func roadDoc(page2Annots string, extra map[int]string) []byte {
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [10 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R /Annots [" + page2Annots + "] >>",
		5:  stream(fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", secretText)),
		6:  stream(fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", publicText)),
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		10: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (onremovedpage) /P 3 0 R >>",
	}
	for k, v := range extra {
		objs[k] = v
	}
	return assembleFixture(objs)
}

// roads is every road from a kept page's annotation to removed page 1 that a review or this door's
// own enumeration has named. `check` asserts what the repair must KEEP, where there is a repair; the
// leak and the dangling reference are asserted of every case.
var roads = []struct {
	name, annots string
	extra        map[int]string
	check        func(t *testing.T, kept []types.Dict)
}{
	// /pending 726: a popup listed on the kept page whose own /P is the removed page.
	{"a kept popup's own /P", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Popup /Rect [0 0 10 10] /P 3 0 R >>",
	}, keptOnItsPage},
	{"a kept markup's /Popup whose /P is the removed page", "11 0 R 12 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /P 4 0 R /Popup 12 0 R >>",
		12: "<< /Type /Annot /Subtype /Popup /Rect [0 0 10 10] /P 3 0 R /Parent 11 0 R >>",
	}, func(t *testing.T, kept []types.Dict) {
		keptOnItsPage(t, kept)
		if _, ok := kept[0]["Popup"]; !ok {
			t.Error("the markup lost its /Popup, though the popup is on the same kept page")
		}
	}},
	{"a kept annotation's own /P", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /P 3 0 R >>",
	}, keptOnItsPage},
	// /pending 765 (1): a GoTo behind a non-GoTo first action, and a GoTo in a /Next array.
	{"a /Next chain behind a URI action", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /P 4 0 R /A << /S /URI /URI (https://example.org) " +
			"/Next << /S /GoTo /D [3 0 R /Fit] >> >> >>",
	}, func(t *testing.T, kept []types.Dict) {
		if got := strings.Join(actionChain(kept[0]["A"]), ","); got != "URI" {
			t.Errorf("the chain is %q, want the URI action alone — the GoTo to the removed page spliced "+
				"out, not left behind as a GoTo with nowhere to go: %v", got, kept[0])
		}
	}},
	{"a GoTo in a /Next array behind a live GoTo", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /P 4 0 R /A << /S /GoTo /D [4 0 R /Fit] " +
			"/Next [<< /S /GoTo /D [3 0 R /Fit] >>] >> >>",
	}, func(t *testing.T, kept []types.Dict) {
		a, _ := kept[0]["A"].(types.Dict)
		if got := strings.Join(actionChain(a), ","); got != "GoTo" || a["D"] == nil {
			t.Errorf("the chain is %q, want the GoTo to the KEPT page alone: %v", got, kept[0])
		}
	}},
	{"an /AA trigger", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Widget /FT /Btn /T (b) /Rect [0 0 10 10] /P 4 0 R " +
			"/AA << /U << /S /GoTo /D [3 0 R /Fit] >> /D << /S /URI /URI (https://example.org) >> >> >>",
	}, func(t *testing.T, kept []types.Dict) {
		aa, _ := kept[0]["AA"].(types.Dict)
		if aa["D"] == nil {
			t.Errorf("the /AA trigger that names no page went with the one that did: %v", kept[0])
		}
		if _, has := aa["U"]; has {
			t.Errorf("the /AA trigger whose GoTo reached the removed page is still there: %v", kept[0])
		}
	}},
	// The net: roads no semantic rule names.
	{"a field listed nowhere whose other widget is on the removed page", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /P 4 0 R /Parent 12 0 R >>",
		12: "<< /T (f) /FT /Tx /Kids [11 0 R 13 0 R] >>",
		13: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /P 3 0 R /Parent 12 0 R >>",
	}, nil},
	{"a /Hide action's target on the removed page", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /P 4 0 R /A << /S /Hide /T 10 0 R >> >>",
	}, nil},
	{"a /Hide action's target listed on no page", "11 0 R", map[int]string{
		11: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /P 4 0 R /A << /S /Hide /T 12 0 R >> >>",
		12: "<< /Type /Annot /Subtype /Square /Rect [0 0 10 10] /Contents (onremovedpage) /P 3 0 R >>",
	}, nil},
}

// keptOnItsPage asserts every kept annotation still exists and names a page that is in the output.
func keptOnItsPage(t *testing.T, kept []types.Dict) {
	t.Helper()
	for _, ad := range kept {
		if _, ok := ad["P"]; !ok {
			t.Errorf("a kept annotation lost its /P instead of being re-pointed at its page: %v", ad)
		}
	}
}

// TestNoRoadFromAKeptAnnotationShipsARemovedPage — every road, through every door that selects pages.
// The removed page's text must be ABSENT from the parsed and decoded output (never a byte count of a
// compressed file), its comment's text too, and nothing kept may name an object the file does not
// hold — the shape pdfcpu leaves where it does not happen to follow a key.
func TestNoRoadFromAKeptAnnotationShipsARemovedPage(t *testing.T) {
	doors := map[string]func([]byte) ([]byte, error){
		"RemovePages": func(src []byte) ([]byte, error) { return RemovePages(src, []string{"1"}) },
		"Collect":     func(src []byte) ([]byte, error) { return Collect(src, []string{"2"}) },
	}
	for _, r := range roads {
		for door, run := range doors {
			t.Run(door+"/"+r.name, func(t *testing.T) {
				src := roadDoc(r.annots, r.extra)
				if !bytes.Contains(everyDecodedByte(t, src), []byte(secretText)) {
					t.Fatal("setup: the source does not carry page 1's text, so its absence proves nothing")
				}
				out, err := run(src)
				if err != nil {
					t.Fatal(err)
				}
				all := everyDecodedByte(t, out)
				if bytes.Contains(all, []byte(secretText)) {
					t.Errorf("the removed page's text is in the output, reached through %s", r.name)
				}
				// A comment's text is a string in a dictionary, and a dictionary may sit in a compressed
				// object stream the raw bytes cannot show — so it is searched for in the PARSED objects.
				if parsedStringsContain(t, out, "onremovedpage") {
					t.Errorf("the removed page's comment text is in the output, reached through %s", r.name)
				}
				if d := danglingRefs(t, out); len(d) > 0 {
					t.Errorf("the output names object(s) %v it does not hold, reached through %s", d, r.name)
				}
				pages := keptAnnots(t, out)
				if len(pages) != 1 || len(pages[0]) == 0 {
					t.Fatalf("want one page with its annotations, got %v", pages)
				}
				if r.check != nil {
					r.check(t, pages[0])
				}
			})
		}
	}
}

// eachParsedObject calls fn with every direct object inside every object pdf holds, parsed — so an
// object in a compressed object stream is seen as well as one written plainly.
func eachParsedObject(t *testing.T, pdf []byte, fn func(xt *model.XRefTable, o types.Object)) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the output does not read: %v", err)
	}
	xt := ctx.XRefTable
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 100 {
			return
		}
		fn(xt, o)
		switch v := o.(type) {
		case types.Dict:
			for _, x := range v {
				walk(x, depth+1)
			}
		case types.StreamDict:
			walk(v.Dict, depth+1)
		case types.Array:
			for _, x := range v {
				walk(x, depth+1)
			}
		}
	}
	for nr, e := range xt.Table {
		if e == nil || e.Free || nr == 0 {
			continue
		}
		if o, derr := xt.Dereference(*types.NewIndirectRef(nr, 0)); derr == nil {
			walk(o, 0)
		}
	}
}

// danglingRefs returns every object number something in pdf names that pdf does not hold.
func danglingRefs(t *testing.T, pdf []byte) []int {
	t.Helper()
	missing := map[int]bool{}
	eachParsedObject(t, pdf, func(xt *model.XRefTable, o types.Object) {
		if r, ok := o.(types.IndirectRef); ok {
			if e, has := xt.Table[r.ObjectNumber.Value()]; !has || e == nil || e.Free {
				missing[r.ObjectNumber.Value()] = true
			}
		}
	})
	var out []int
	for nr := range missing {
		out = append(out, nr)
	}
	return out
}

// parsedStringsContain reports whether any string object in pdf, parsed, contains marker.
func parsedStringsContain(t *testing.T, pdf []byte, marker string) bool {
	t.Helper()
	found := false
	eachParsedObject(t, pdf, func(_ *model.XRefTable, o types.Object) {
		if v, ok := o.(types.StringLiteral); ok {
			if s, err := types.StringLiteralToString(v); err == nil && strings.Contains(s, marker) {
				found = true
			}
		}
	})
	return found
}

// actionChain lists the /S of every action on an annotation's /A chain, in order, /Next included.
func actionChain(a types.Object) []string {
	var out []string
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 10 {
			return
		}
		switch v := o.(type) {
		case types.Dict:
			out = append(out, nameVal(v, "S"))
			walk(v["Next"], depth+1)
		case types.Array:
			for _, x := range v {
				walk(x, depth+1)
			}
		}
	}
	walk(a, 0)
	return out
}

// coDoc is two pages and one text field with a widget on each, written as a parent field with /Kids —
// the shape every field with more than one widget has — plus a second field whose only widget is on
// page 2. /CO names both fields.
func coDoc() []byte {
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 9 0 R] /CO [6 0 R 9 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [7 0 R] >>",
		4: "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [8 0 R 9 0 R] >>",
		5: stream("0 0 10 10 re f"),
		6: "<< /T (total) /FT /Tx /Kids [7 0 R 8 0 R] /AA << /C << /S /JavaScript /JS (1) >> >> >>",
		7: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /Parent 6 0 R /P 3 0 R >>",
		8: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /Parent 6 0 R /P 4 0 R >>",
		9: "<< /T (other) /FT /Tx /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /P 4 0 R /AA << /C << /S /JavaScript /JS (2) >> >> >>",
	})
}

// calcOrder reads the output's /AcroForm /CO as the /T of each field it names.
func calcOrder(t *testing.T, ctx *model.Context) []string {
	t.Helper()
	root, _ := ctx.XRefTable.Catalog()
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	if form == nil {
		return nil
	}
	var out []string
	for _, o := range derefArray(ctx.XRefTable, form["CO"]) {
		s, _ := litString(derefDict(ctx.XRefTable, o)["T"])
		out = append(out, s)
	}
	return out
}

// TestACalculationOrderKeepsAFieldSeparateFromItsWidget — `/pending 765` (2), at both doors that rebuild
// `/Fields`. /CO names fields and both doors tested it against widget numbers, so a field written as a
// parent with /Kids lost its calculation-order entry while it survived. The control is the field whose
// only widget went.
func TestACalculationOrderKeepsAFieldSeparateFromItsWidget(t *testing.T) {
	out, err := RemovePages(coDoc(), []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := calcOrder(t, readCtx(t, out)); strings.Join(got, ",") != "total" {
		t.Errorf("page selection: /CO is %v, want [total] — the field survives through its page-1 widget, "+
			"and the field whose only widget went is gone", got)
	}

	// The composition door's prune, over a document whose page 2 has lost its annotations.
	ctx := readCtx(t, coDoc())
	root, _ := ctx.XRefTable.Catalog()
	leaves, _, lerr := collectLeaves(ctx.XRefTable, root)
	if lerr != nil || len(leaves) != 2 {
		t.Fatalf("setup: %d pages (%v)", len(leaves), lerr)
	}
	delete(leaves[1].dic, "Annots")
	if _, err := pruneOrphanedAcroForm(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calcOrder(t, ctx); strings.Join(got, ",") != "total" {
		t.Errorf("composition prune: /CO is %v, want [total]", got)
	}
}

// TestAMailMergeRefusesPastThePartCap — `/pending 765` (3). Each CSV record is a full fill and parse,
// and the CSV is user input; the splitters' cap is the same question and is asked before the form is.
func TestAMailMergeRefusesPastThePartCap(t *testing.T) {
	var csv strings.Builder
	csv.WriteString("name\n")
	for i := 0; i <= maxSplitParts; i++ {
		fmt.Fprintf(&csv, "r%d\n", i)
	}
	parts, err := FillFormCSV([]byte("not a pdf"), []byte(csv.String()), "")
	if err == nil || !strings.Contains(err.Error(), "too many output files") {
		t.Fatalf("a %d-record merge returned %d part(s) and err=%v, want the part-cap refusal",
			maxSplitParts+1, len(parts), err)
	}
}
