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

// removedScreen, removedMovie and removedField put an annotation of the kind an action needs on removed
// page 1; uriNext is an action that names no page, behind the one under test.
const (
	removedScreen = "<< /Type /Annot /Subtype /Screen /Rect [0 0 10 10] /Contents (onremovedpage) /P 3 0 R >>"
	removedMovie  = "<< /Type /Annot /Subtype /Movie /Rect [0 0 10 10] /Contents (onremovedpage) /P 3 0 R /Movie << /F (m.mov) >> >>"
	removedField  = "<< /Type /Annot /Subtype /Widget /FT /Tx /T (gone) /TU (onremovedpage) /Rect [0 0 10 10] /P 3 0 R >>"
	uriNext       = "/Next << /S /URI /URI (https://example.org) >>"
)

// link is annotation 11 on kept page 2, carrying action.
func link(action string) string {
	return "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /P 4 0 R /A << " + action + " >> >>"
}

// brokenActions is `/pending 835`: every action type that names an annotation, a field or a bead, with
// what it names on removed page 1. `want` is the `/S` chain the kept link must be left with — "" for no
// `/A` at all — and `check` reads what a surviving action is left holding.
var brokenActions = []struct {
	name  string
	extra map[int]string
	want  string
	check func(t *testing.T, xt *model.XRefTable, act types.Dict)
}{
	{"a /Hide of one annotation, with a /Next", map[int]string{
		11: link("/S /Hide /T 10 0 R " + uriNext),
	}, "URI", nil},
	{"a /Hide of a list that is all on the removed page", map[int]string{
		11: link("/S /Hide /T [10 0 R]"),
	}, "", nil},
	{"a /Hide of a list held as its own object", map[int]string{
		11: link("/S /Hide /T 12 0 R"),
		12: "[10 0 R]",
	}, "", nil},
	{"a /Hide of a list with one target still on a page", map[int]string{
		11: link("/S /Hide /T [10 0 R 11 0 R]"),
	}, "Hide", func(t *testing.T, xt *model.XRefTable, act types.Dict) {
		if got := derefArray(xt, act["T"]); len(got) != 1 {
			t.Errorf("the /Hide's /T is %v, want the one target that is still on a page", got)
		}
	}},
	{"a /Rendition with an /OP", map[int]string{
		10: removedScreen,
		11: link("/S /Rendition /OP 1 /AN 10 0 R " + uriNext),
	}, "URI", nil},
	{"a script-only /Rendition", map[int]string{
		10: removedScreen,
		11: link("/S /Rendition /JS (1) /AN 10 0 R"),
	}, "Rendition", func(t *testing.T, _ *model.XRefTable, act types.Dict) {
		if _, has := act["AN"]; has {
			t.Errorf("the rendition action still names its screen annotation: %v", act)
		}
	}},
	{"a /GoTo3DView", map[int]string{
		11: link("/S /GoTo3DView /TA 10 0 R /V 0"),
	}, "", nil},
	{"a /RichMediaExecute", map[int]string{
		11: link("/S /RichMediaExecute /TA 10 0 R /CMD << /C (go) >> " + uriNext),
	}, "URI", nil},
	{"a /Movie", map[int]string{
		10: removedMovie,
		11: link("/S /Movie /Annotation 10 0 R"),
	}, "", nil},
	{"a /ResetForm of the one field that went", map[int]string{
		10: removedField,
		11: link("/S /ResetForm /Fields [10 0 R] " + uriNext),
	}, "URI", nil},
	{"a /SubmitForm of the one field that went", map[int]string{
		10: removedField,
		11: link("/S /SubmitForm /F << /FS /URL /F (https://example.org/f) >> /Fields [10 0 R]"),
	}, "", nil},
	{"a /Thread whose only bead went", map[int]string{
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [10 0 R] /B [20 0 R] >>",
		11: link("/S /Thread /D 19 0 R " + uriNext),
		19: "<< /Type /Thread /F 20 0 R >>",
		20: "<< /Type /Bead /T 19 0 R /N 20 0 R /V 20 0 R /P 3 0 R /R [0 0 10 10] >>",
	}, "URI", nil},
	{"a /Thread whose first bead went", map[int]string{
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [10 0 R] /B [20 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R /Annots [11 0 R] /B [21 0 R 22 0 R] >>",
		11: link("/S /Thread /D 19 0 R /B 20 0 R"),
		19: "<< /Type /Thread /F 20 0 R >>",
		20: "<< /Type /Bead /T 19 0 R /N 21 0 R /V 22 0 R /P 3 0 R /R [0 0 10 10] >>",
		21: "<< /Type /Bead /T 19 0 R /N 22 0 R /V 20 0 R /P 4 0 R /R [0 20 10 30] >>",
		22: "<< /Type /Bead /T 19 0 R /N 20 0 R /V 21 0 R /P 4 0 R /R [0 40 10 50] >>",
	}, "Thread", func(t *testing.T, xt *model.XRefTable, act types.Dict) {
		if _, has := act["B"]; has {
			t.Errorf("the thread action still names the bead that went: %v", act)
		}
		ringIsTheKeptBeads(t, xt, derefDict(xt, act["D"]))
	}},
	// No action names this thread: only the kept page's own /B reaches it.
	{"an article whose first bead went, named by no action", map[int]string{
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [10 0 R] /B [20 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R /Annots [11 0 R] /B [21 0 R 22 0 R] >>",
		11: link("/S /URI /URI (https://example.org)"),
		19: "<< /Type /Thread /F 20 0 R >>",
		20: "<< /Type /Bead /T 19 0 R /N 21 0 R /V 22 0 R /P 3 0 R /R [0 0 10 10] >>",
		21: "<< /Type /Bead /T 19 0 R /N 22 0 R /V 20 0 R /P 4 0 R /R [0 20 10 30] >>",
		22: "<< /Type /Bead /T 19 0 R /N 20 0 R /V 21 0 R /P 4 0 R /R [0 40 10 50] >>",
	}, "URI", func(t *testing.T, xt *model.XRefTable, _ types.Dict) {
		root, _ := xt.Catalog()
		leaves, _, _ := collectLeaves(xt, root)
		beads := derefArray(xt, leaves[0].dic["B"])
		if len(beads) != 2 {
			t.Fatalf("the kept page's /B is %v, want its two beads", beads)
		}
		ringIsTheKeptBeads(t, xt, derefDict(xt, derefDict(xt, beads[0])["T"]))
	}},
}

// ringIsTheKeptBeads walks thread's beads from its first: every bead names a page the output holds and
// its neighbours both ways, the first carries /T, and the ring closes having read the two beads of the
// kept page in their order.
func ringIsTheKeptBeads(t *testing.T, xt *model.XRefTable, thread types.Dict) {
	t.Helper()
	first, ok := thread["F"].(types.IndirectRef)
	if !ok {
		t.Fatalf("the thread has no first bead: %v", thread)
	}
	pages := map[int]bool{}
	root, _ := xt.Catalog()
	leaves, _, _ := collectLeaves(xt, root)
	for _, l := range leaves {
		pages[l.ref.ObjectNumber.Value()] = true
	}
	rects := ""
	cur, prev := first, types.IndirectRef{}
	for i := 0; i < 5; i++ {
		b := derefDict(xt, cur)
		if p, isRef := b["P"].(types.IndirectRef); !isRef || !pages[p.ObjectNumber.Value()] {
			t.Fatalf("bead %v names no page of the output as its /P: %v", cur, b)
		}
		if v, isRef := b["V"].(types.IndirectRef); i > 0 && (!isRef || v != prev) {
			t.Errorf("bead %v has /V %v, want the bead before it %v", cur, b["V"], prev)
		}
		if i == 0 && b["T"] == nil {
			t.Errorf("the thread's first bead has no /T: %v", b)
		}
		rects += fmt.Sprint(derefArray(xt, b["R"])[1]) + " "
		next, isRef := b["N"].(types.IndirectRef)
		if !isRef {
			t.Fatalf("bead %v has no /N: %v", cur, b)
		}
		prev, cur = cur, next
		if cur == first {
			break
		}
	}
	if rects != "20 40 " {
		t.Errorf("the ring from the thread's first bead reads beads at %q, want the two on the kept page in their order", rects)
	}
}

// TestAnActionThatLosesItsTargetIsRemovedWholeOrLeftValid — `/pending 835`. The net cut the reference
// and left the action: a `/Hide` with no `/T`, a bead with no `/P`. Each case is read back two ways —
// pdfcpu's own validator over the output, which is what names a required entry as missing, and the
// chain the kept link is left with.
func TestAnActionThatLosesItsTargetIsRemovedWholeOrLeftValid(t *testing.T) {
	doors := map[string]func([]byte) ([]byte, error){
		"RemovePages": func(src []byte) ([]byte, error) { return RemovePages(src, []string{"1"}) },
		"Collect":     func(src []byte) ([]byte, error) { return Collect(src, []string{"2"}) },
	}
	for _, c := range brokenActions {
		for door, run := range doors {
			t.Run(door+"/"+c.name, func(t *testing.T) {
				// The pages draw a rectangle and name no font: roadDoc's font dictionary is one pdfcpu's
				// strict mode refuses for itself, and this test's question is the action.
				objs := map[int]string{
					2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
					5: stream("0 0 10 10 re f"),
					6: stream("0 0 10 10 re f"),
				}
				for k, v := range c.extra {
					objs[k] = v
				}
				src := roadDoc("11 0 R", objs)
				if !parsedStringsContain(t, src, "onremovedpage") {
					t.Fatal("setup: the source holds no annotation on the removed page")
				}
				out, err := run(src)
				if err != nil {
					t.Fatal(err)
				}
				if parsedStringsContain(t, out, "onremovedpage") {
					t.Error("the removed page's annotation is in the output")
				}
				if d := danglingRefs(t, out); len(d) > 0 {
					t.Errorf("the output names object(s) %v it does not hold", d)
				}
				ctx := readCtx(t, out)
				pages := keptAnnots(t, out)
				if len(pages) != 1 || len(pages[0]) != 1 {
					t.Fatalf("want one page with the link on it, got %v", pages)
				}
				// STRICT, because pdfcpu's relaxed mode — its default — does not read an annotation's action
				// at all: measured, a `/Hide` with no `/T` validates relaxed and is refused strict.
				strict := model.NewDefaultConfiguration()
				strict.ValidationMode = model.ValidationStrict
				if verr := api.Validate(bytes.NewReader(out), strict); verr != nil {
					t.Errorf("pdfcpu's validator refuses the output: %v", verr)
				}
				act := derefDict(ctx.XRefTable, pages[0][0]["A"])
				if got := strings.Join(actionChain(act), ","); got != c.want {
					t.Fatalf("the link's action chain is %q, want %q: %v", got, c.want, pages[0][0])
				}
				if c.check != nil {
					c.check(t, ctx.XRefTable, act)
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
