package pdfops

import (
	"sort"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// keyedFixture is a two-page conversion carrying three sticky notes — one not ASCII, one with an
// `/NM` — and two form widgets, so every part of an annotation's key is filled on some annotation.
func keyedFixture(t *testing.T) []byte {
	t.Helper()
	src := labelReady(t, "# One\n\nA paragraph to comment on, long enough to swap a word in.\n\n"+
		"<div style=\"page-break-after: always\"></div>\n\n# Two\n\nMore to read here.\n")
	src, err := AddNotes(src, []Note{
		{Page: 1, X: 100, Y: 700, Text: "first"},
		{Page: 1, X: 200, Y: 600, Text: "Zoë — señor"},
		{Page: 1, X: 300, Y: 500, Text: "named"},
	})
	if err != nil {
		t.Fatal(err)
	}
	src, err = AuthorForm(src, []FormField{
		{Page: 1, Rect: [4]float64{100, 100, 200, 120}, Kind: "text", Name: "fullName"},
		{Page: 1, Rect: [4]float64{100, 140, 120, 160}, Kind: "checkbox", Name: "agree"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mustMutate(t, src, func(ctx *model.Context) error {
		pd, _, _, err := ctx.XRefTable.PageDict(1, false)
		if err != nil {
			return err
		}
		for _, a := range derefArray(ctx.XRefTable, pd["Annots"]) {
			d, _ := ctx.DereferenceDict(a)
			if s, _ := ctx.DereferenceStringOrHexLiteral(d["Contents"], model.V10, nil); s == "named" {
				d["NM"] = types.StringLiteral("note-0001")
			}
		}
		return nil
	})
}

// keysLost is the commit door's count: the keys `before` holds that `after` does not, as a multiset.
func keysLost(before, after []string) int {
	left := map[string]int{}
	for _, k := range after {
		left[k]++
	}
	lost := 0
	for _, k := range before {
		if left[k] > 0 {
			left[k]--
		} else {
			lost++
		}
	}
	return lost
}

// TestAnAnnotationsKeyIsWhatItIsNotWhereItIs — the key itself (/pending 875).
func TestAnAnnotationsKeyIsWhatItIsNotWhereItIs(t *testing.T) {
	facts := Inspect(keyedFixture(t))
	if !facts.Readable || facts.Annotations != 5 || len(facts.AnnotationKeys) != 5 {
		t.Fatalf("the fixture reads as %d annotation(s) under %d key(s), want 5 and 5", facts.Annotations, len(facts.AnnotationKeys))
	}
	distinct := map[string]bool{}
	for _, k := range facts.AnnotationKeys {
		distinct[k] = true
	}
	if len(distinct) != 5 {
		t.Errorf("5 annotations that differ in their text, their /NM or their field name share keys: %q — "+
			"dropping one and writing another would read as nothing lost", facts.AnnotationKeys)
	}
}

// TestNoOperationRenamesAnAnnotationItKeeps — zero false losses, over every driven operation.
//
// The commit door reports an annotation as lost when its key is gone from the result. That is only honest
// if no operation changes a key of an annotation it KEEPS — a moved, re-encoded or renumbered annotation
// that read as "gone, and another arrived" would tell the user a comment was destroyed while it sits on the
// page. So for every operation in the census, what is lost by key must be exactly what is lost by count:
// more means a kept annotation came back under another key. An operation that really does drop one
// annotation and write another has to be named here with what it drops.
//
// Measured when written: 41 operations driven, 0 over. With the `/Rect` in the key `NUp` reads 4 lost
// where 2 are — the two notes it carries onto the sheet.
func TestNoOperationRenamesAnAnnotationItKeeps(t *testing.T) {
	src := keyedFixture(t)
	before := Inspect(src)
	if len(before.AnnotationKeys) != 5 {
		t.Fatalf("setup: the fixture holds %d keyed annotation(s), want 5", len(before.AnnotationKeys))
	}
	names := make([]string, 0, len(tagFates))
	for n := range tagFates {
		names = append(names, n)
	}
	sort.Strings(names)
	driven, dropped := 0, 0
	for _, name := range names {
		drive := tagFates[name].drive
		if drive == nil {
			continue
		}
		out, err := drive(src)
		if err != nil || out == nil {
			continue // the census's own tests own whether a drive runs on its fixture
		}
		after := Inspect(out)
		if !after.Readable || len(after.AnnotationKeys) != after.Annotations {
			t.Errorf("%s: the result reads as %d annotation(s) under %d key(s)", name, after.Annotations, len(after.AnnotationKeys))
			continue
		}
		driven++
		byCount := before.Annotations - after.Annotations
		if byCount < 0 {
			byCount = 0
		}
		byKey := keysLost(before.AnnotationKeys, after.AnnotationKeys)
		if byCount > 0 {
			dropped++
		}
		if byKey != byCount {
			t.Errorf("%s: %d annotation(s) lost by key and %d by count (%d before, %d after) — an "+
				"annotation it kept came back under another key, and the loss notice would report a "+
				"comment that is still on the page.\n  before %q\n  after  %q",
				name, byKey, byCount, before.Annotations, after.Annotations, before.AnnotationKeys, after.AnnotationKeys)
		}
	}
	if driven < 35 {
		t.Errorf("%d operation(s) were driven over the annotated fixture; the census drives at least 35", driven)
	}
	if dropped == 0 {
		t.Error("no driven operation dropped an annotation, so nothing here showed a carried one keeps its key beside dropped ones (the n-up does)")
	}
}

// TestTooManyAnnotationsToKeyAreCounted — past the bound there are no keys, and the count still stands.
func TestTooManyAnnotationsToKeyAreCounted(t *testing.T) {
	const n = maxAnnotKeys + 5
	big := mustMutate(t, labelReady(t, "# One\n\nText.\n"), func(ctx *model.Context) error {
		pd, _, _, err := ctx.XRefTable.PageDict(1, false)
		if err != nil {
			return err
		}
		annots := make(types.Array, 0, n)
		link := types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name("Link"),
			"Rect": types.Array{types.Integer(0), types.Integer(0), types.Integer(10), types.Integer(10)}}
		for i := 0; i < n; i++ {
			annots = append(annots, link)
		}
		pd["Annots"] = annots
		return nil
	})
	facts := Inspect(big)
	if !facts.Readable || facts.Annotations != n || facts.AnnotationKeys != nil {
		t.Errorf("%d annotations read as %d under %d key(s) (readable %v); want the count and no keys",
			n, facts.Annotations, len(facts.AnnotationKeys), facts.Readable)
	}
}
