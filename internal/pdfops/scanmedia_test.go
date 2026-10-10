package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// writtenFacts reports what is IN a file, wherever it hangs: how many annotations of each subtype, how many OBJRs
// naming an annotation of each subtype, and how many dictionaries of each `/Type`. An object nothing names is not
// written, so this is what a removal left behind rather than what a page still shows. (The validating reader,
// because the unvalidated one leaves objects held in object streams unread.)
func writtenFacts(t *testing.T, pdf []byte) (annots, objrs, types_ map[string]int) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	xt := ctx.XRefTable
	annots, objrs, types_ = map[string]int{}, map[string]int{}, map[string]int{}
	var visit func(d types.Dict, depth int)
	visit = func(d types.Dict, depth int) {
		if d == nil || depth > 6 {
			return
		}
		if ty := d.NameEntry("Type"); ty != nil {
			types_[*ty]++
			if *ty == "OBJR" {
				sub := "?"
				if obj, _ := xt.DereferenceDict(d["Obj"]); obj != nil {
					if s := obj.NameEntry("Subtype"); s != nil {
						sub = *s
					}
				}
				objrs[sub]++
			}
		}
		if _, isAnnot := d["Rect"]; isAnnot {
			if s := d.NameEntry("Subtype"); s != nil {
				annots[*s]++
			}
		}
		// Direct values only: an indirect one is its own row of the table.
		for _, v := range d {
			switch v := v.(type) {
			case types.Dict:
				visit(v, depth+1)
			case types.Array:
				for _, e := range v {
					if ed, ok := e.(types.Dict); ok {
						visit(ed, depth+1)
					}
				}
			}
		}
	}
	for _, e := range xt.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		switch o := e.Object.(type) {
		case types.Dict:
			visit(o, 0)
		case types.StreamDict:
			visit(o.Dict, 0)
		case types.Array:
			for _, el := range o {
				if ed, ok := el.(types.Dict); ok {
					visit(ed, 0)
				}
			}
		}
	}
	return annots, objrs, types_
}

// taggedWithNotes is a tagged conversion carrying two notes, each described in the structure tree by an `/Annot`
// element whose OBJR names it. media turns the FIRST note into a Screen annotation, tree entry and all.
func taggedWithNotes(t *testing.T, media bool) []byte {
	t.Helper()
	src := labelReady(t, "# Notes\n\nA paragraph to comment on.\n")
	out, err := AddNotes(src, []Note{{Page: 1, X: 100, Y: 700, Text: "first"}, {Page: 1, X: 200, Y: 600, Text: "second"}})
	if err != nil {
		t.Fatal(err)
	}
	if !media {
		return out
	}
	return mustMutate(t, out, func(ctx *model.Context) error {
		pd, _, _, err := ctx.XRefTable.PageDict(1, false)
		if err != nil {
			return err
		}
		for _, a := range derefArray(ctx.XRefTable, pd["Annots"]) {
			if ad := derefDict(ctx.XRefTable, a); ad != nil && nameVal(ad, "Subtype") == "Text" {
				ad["Subtype"] = types.Name("Screen")
				for _, k := range []string{"Popup", "Name", "Open", "IRT", "RT", "State", "StateModel"} {
					delete(ad, k)
				}
				return nil
			}
		}
		return nil
	})
}

// TestARemovedMediaAnnotationLeavesTheStructureTree — `/pending 824`, first half. removeMediaAnnots unlinks a media
// annotation from its page, and "an object nothing names is not written" was the whole of how it left the file. In
// a tagged document something else names it: the OBJR of the structure element that describes it. So the element
// went on describing an annotation no page shows, and the annotation — the thing the removal exists to take out —
// was still written, by both tiers.
func TestARemovedMediaAnnotationLeavesTheStructureTree(t *testing.T) {
	pdf := taggedWithNotes(t, true)
	annots, objrs, _ := writtenFacts(t, pdf)
	if annots["Screen"] != 1 || objrs["Screen"] != 1 || objrs["Text"] != 1 {
		t.Fatalf("setup: want one Screen annotation with an OBJR and one note with an OBJR; annotations %v, OBJRs %v", annots, objrs)
	}
	for name, remove := range map[string]func([]byte) ([]byte, error){"RemoveFilesAndMedia": RemoveFilesAndMedia, "StripActive": StripActive} {
		out, err := remove(pdf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := Validate(out); err != nil {
			t.Fatalf("%s: the result does not validate: %v", name, err)
		}
		annots, objrs, _ := writtenFacts(t, out)
		if annots["Screen"] != 0 {
			t.Errorf("%s: the removed Screen annotation is still written to the file (%d object) — the tree's OBJR kept it", name, annots["Screen"])
		}
		if len(objrs) != 1 || objrs["Text"] != 1 {
			t.Errorf("%s: OBJRs by what they name = %v, want only the note's — an OBJR naming the removed annotation, or nothing, is left", name, objrs)
		}
		// The annotation's own row of the parent tree goes with it: a row no object claims describes nothing.
		ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		claimed := parentTreeOwners(ctx)
		arrays, singles := parentTreeEntries(ctx, mustStructTree(t, ctx))
		for key := range singles {
			if len(claimed[key]) == 0 {
				t.Errorf("%s: /ParentTree row %d is claimed by no object", name, key)
			}
		}
		if len(singles) != 1 || len(arrays) == 0 {
			t.Errorf("%s: the parent tree has %d annotation row(s) and %d page row(s), want the note's one and the page's", name, len(singles), len(arrays))
		}
	}
}

func mustStructTree(t *testing.T, ctx *model.Context) *structTree {
	t.Helper()
	tree, err := readStructTree(ctx, nil)
	if err != nil || tree == nil {
		t.Fatalf("the structure tree does not read: %v", err)
	}
	return tree
}

// A tagged document with nothing to remove keeps every annotation, every OBJR and every element.
func TestRemovingMediaLeavesATaggedDocumentWithNoneAsItWas(t *testing.T) {
	pdf := taggedWithNotes(t, false)
	annots, objrs, kinds := writtenFacts(t, pdf)
	if objrs["Text"] != 2 {
		t.Fatalf("setup: %v OBJRs, want the two notes'", objrs)
	}
	out, err := RemoveFilesAndMedia(pdf)
	if err != nil {
		t.Fatal(err)
	}
	gotAnnots, gotObjrs, gotKinds := writtenFacts(t, out)
	for _, k := range []string{"StructElem", "OBJR", "StructTreeRoot", "Annot"} {
		if kinds[k] != gotKinds[k] {
			t.Errorf("%d /%s object(s) before, %d after", kinds[k], k, gotKinds[k])
		}
	}
	if annots["Text"] != gotAnnots["Text"] || gotObjrs["Text"] != 2 {
		t.Errorf("the notes changed: annotations %v → %v, OBJRs %v → %v", annots, gotAnnots, objrs, gotObjrs)
	}
}

// mediaElsewherePDF holds media no page's /Annots array is the only road to: a rendition in the catalog's
// /Names /Renditions tree, its clip an embedded file stream. A Screen annotation and — with link — a Link whose
// action plays the rendition and carries JavaScript reach the same rendition from a page; with neither annotation,
// the tree is the only thing in the file that names it.
func mediaElsewherePDF(screen, link bool) []byte {
	annots := ""
	if screen {
		annots += "12 0 R"
	}
	if link {
		annots += " 13 0 R"
	}
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Names << /Renditions << /Names [(clip) 10 0 R] >> >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << >> /Annots [" + annots + "] >>",
		4:  "<< /Length 0 >>\nstream\n\nendstream",
		10: "<< /Type /Rendition /S /MR /N (clip) /C << /Type /MediaClip /S /MCD /CT (video/mp4) /D << /Type /Filespec /F (a.mp4) /EF << /F 11 0 R >> >> >> >>",
		11: "<< /Type /EmbeddedFile /Length 5 >>\nstream\nHELLO\nendstream",
		12: "<< /Type /Annot /Subtype /Screen /Rect [0 0 50 50] /P 3 0 R /A << /Type /Action /S /Rendition /OP 0 /R 10 0 R /AN 12 0 R >> >>",
		13: "<< /Type /Annot /Subtype /Link /Rect [60 0 90 50] /A << /Type /Action /S /Rendition /OP 0 /R 10 0 R /AN 12 0 R /JS (app.alert\\(1\\)) >> >>",
	})
}

// TestMediaInTheNamesTreeIsSeenAndRemoved — `/pending 824`, second half. Scan reached media only through a page's
// /Annots, so a rendition held in the catalog's /Names /Renditions tree — clip, embedded stream and all — was not
// reported, survived both removals, and each removal's own re-scan passed it.
func TestMediaInTheNamesTreeIsSeenAndRemoved(t *testing.T) {
	const want = "media/medium: Media clips the document can play (renditions)"
	reported := func(pdf []byte) bool {
		for _, f := range must(t, pdf).Findings {
			if f.Kind+"/"+f.Severity+": "+f.Detail == want {
				return true
			}
		}
		return false
	}
	// The tree alone, with no annotation and no action beside it: the scan's only road to it is the tree.
	alone := mediaElsewherePDF(false, false)
	if _, _, kinds := writtenFacts(t, alone); kinds["Rendition"] != 1 || kinds["EmbeddedFile"] != 1 {
		t.Fatalf("setup: the fixture's rendition and its clip did not read: %v", kinds)
	}
	if !reported(alone) {
		t.Errorf("the scan did not report %q; findings: %+v", want, must(t, alone).Findings)
	}
	for name, c := range map[string]struct {
		remove func([]byte) ([]byte, error)
		pdf    []byte
		links  int
	}{
		"RemoveFilesAndMedia, the tree alone":            {RemoveFilesAndMedia, alone, 0},
		"RemoveFilesAndMedia, with a Screen annotation":  {RemoveFilesAndMedia, mediaElsewherePDF(true, false), 0},
		"StripActive, the tree alone":                    {StripActive, alone, 0},
		"StripActive, with a Screen and a scripted Link": {StripActive, mediaElsewherePDF(true, true), 1},
	} {
		out, err := c.remove(c.pdf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := Validate(out); err != nil {
			t.Fatalf("%s: the result does not validate: %v", name, err)
		}
		annots, _, kinds := writtenFacts(t, out)
		if kinds["Rendition"]+kinds["MediaClip"]+kinds["EmbeddedFile"] != 0 || annots["Screen"] != 0 {
			t.Errorf("%s: media is still in the file: typed dictionaries %v, annotations %v", name, kinds, annots)
		}
		if annots["Link"] != c.links {
			t.Errorf("%s: %d Link annotation(s) left, want %d — the annotation itself is not media", name, annots["Link"], c.links)
		}
		if strings.Contains(string(out), "HELLO") {
			t.Errorf("%s: the clip's bytes are still in the file", name)
		}
	}
}
