package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The authored form gets a structure tree — `PLAN-accessibility.md` P06.S07.

func s07Fields() []FormField {
	return []FormField{
		{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "full_name", Label: "Your full name"},
		{Page: 1, Rect: [4]float64{100, 660, 112, 672}, Kind: "check", Name: "agree", Label: "I agree to the terms"},
	}
}

// widgetLinkage reports, per widget annotation, whether it points back into the tree and whether
// some element points at it by OBJR — the two halves of "nested within a Form tag", which must
// agree or a reader walking one direction sees a structure the other direction denies.
type widgetLinkage struct {
	structParent int  // the /StructParent value, -1 when absent
	single       bool // its ParentTree entry is a single reference, not an array
	objrFrom     int  // object number of the element whose OBJR names it, 0 when none
	elemType     string
}

func widgetLinkages(t *testing.T, pdf []byte) []widgetLinkage {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatalf("tree: %v", terr)
	}
	_, singles := parentTreeEntries(ctx, tree)

	// Which element's OBJR names which object.
	objrOwner := map[int]*structElem{}
	for i := range tree.elems {
		e := tree.elems[i]
		for _, k := range e.kids {
			if k.kind == kidOBJR && k.obj != 0 {
				objrOwner[k.obj] = e
			}
		}
	}

	var out []widgetLinkage
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			continue
		}
		annots, _ := ctx.DereferenceArray(d["Annots"])
		for _, a := range annots {
			ar, isRef := a.(types.IndirectRef)
			if !isRef {
				continue
			}
			ad, aerr := ctx.DereferenceDict(ar)
			if aerr != nil || ad == nil {
				continue
			}
			if sub, _ := ad["Subtype"].(types.Name); sub != "Widget" {
				continue
			}
			l := widgetLinkage{structParent: -1}
			if sp, ok := ad["StructParent"].(types.Integer); ok {
				l.structParent = sp.Value()
				_, l.single = singles[sp.Value()]
			}
			if e := objrOwner[ar.ObjectNumber.Value()]; e != nil {
				l.objrFrom = e.objNr
				l.elemType = e.kind
			}
			out = append(out, l)
		}
	}
	return out
}

// TestEveryWidgetIsNestedInAFormElementThatPointsBack — S07's first two acceptance clauses.
//
// veraPDF's demand is literal: *"A Widget annotation shall be nested within a Form tag"*, failing
// with *"nested within null tag (standard type = null) instead of Form"*. P06.S05 measured 7.18.4 t1
// as out of reach and was right only about KEYS — nesting is a tree.
//
// Both directions are asserted because a reader can walk either: the element names the annotation by
// OBJR, and the annotation names the element through its `/StructParent`. One without the other is a
// structure that disagrees with itself.
func TestEveryWidgetIsNestedInAFormElementThatPointsBack(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	out, tagged, err := AuthorTaggedForm(base, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("AuthorTaggedForm fell back to the untagged form, so nothing below is about a " +
			"described document. That fallback is deliberate and must not be the ordinary path")
	}
	if verr := Validate(out); verr != nil {
		t.Fatalf("the described form does not validate: %v", verr)
	}
	links := widgetLinkages(t, out)
	// Stimulus floor: no widgets means every assertion below passes over an empty set.
	if len(links) != 2 {
		t.Fatalf("setup: the document holds %d widget annotation(s), want 2 — the fixture changed "+
			"and the assertions below would be vacuous", len(links))
	}
	for i, l := range links {
		if l.objrFrom == 0 {
			t.Errorf("widget %d: no structure element points at it by OBJR, so a reader walking "+
				"the tree never reaches it", i)
			continue
		}
		if l.elemType != "Form" {
			t.Errorf("widget %d is nested in a %q element, want \"Form\" — veraPDF names the type: "+
				"\"nested within null tag (standard type = null) instead of Form\"", i, l.elemType)
		}
		if l.structParent < 0 {
			t.Errorf("widget %d carries no /StructParent, so a reader starting at the annotation "+
				"cannot find what describes it", i)
		}
	}
}

// TestAnAnnotationsParentTreeEntryIsASingleReferenceNotAnArray — S07's second acceptance clause, and
// the distinction P05.S03 modelled and nothing had written.
//
// `/ParentTree` maps a key to one of two shapes: a PAGE's key (from `/StructParents`, plural) gives
// an array indexed by MCID; an ANNOTATION's (from `/StructParent`, singular) gives one reference.
// Writing an array where a reader expects a reference resolves to the wrong kind of object.
func TestAnAnnotationsParentTreeEntryIsASingleReferenceNotAnArray(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	out, tagged, err := AuthorTaggedForm(base, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("AuthorTaggedForm fell back to the untagged form")
	}
	links := widgetLinkages(t, out)
	if len(links) == 0 {
		t.Fatal("setup: no widget annotations")
	}
	for i, l := range links {
		if l.structParent < 0 {
			continue // the test above reports this
		}
		if !l.single {
			t.Errorf("widget %d's /StructParent %d does not resolve to a single reference. An "+
				"array there is a page's shape, and a reader dereferencing it gets an array where "+
				"it expects the element that describes this annotation", i, l.structParent)
		}
	}
	// And the model agrees the whole tree is consistent — the same reader P05.S03 built.
	if _, defects := checkTree(t, out); len(defects) > 0 {
		t.Errorf("the described form's tree has %d defect(s): %v", len(defects), defects)
	}
}

// TestAKeyIsNeverHandedOutTwice — the key space the two ParentTree shapes SHARE.
//
// A page array at key 3 and an annotation reference at key 3 are the same entry, so the second write
// destroys the first. `freeParentTreeKey` scans both shapes; this drives the case where a page
// already owns keys, which is every document P05's emitter has touched.
func TestAKeyIsNeverHandedOutTwice(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	// A document whose page already owns a ParentTree key, from a committed proposal (the generic
	// emitter this came from was deleted at P08.S07).
	wrapped, err := commitProposal(base, proposeFor(t, base).elements)
	if err != nil {
		t.Fatal(err)
	}
	out, tagged, err := AuthorTaggedForm(wrapped, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("AuthorTaggedForm fell back on a document that already had a tree — the " +
			"add-to-an-existing-tree path is the one this test exists for")
	}
	// The floor: the page's own key must still be there, or the collision happened and this test
	// would pass on a document where the page lost its structure.
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	sp, hasPageKey := d["StructParents"].(types.Integer)
	if !hasPageKey {
		t.Fatal("the page lost its /StructParents while the form was being described")
	}
	links := widgetLinkages(t, out)
	for i, l := range links {
		if l.structParent == sp.Value() {
			t.Errorf("widget %d was given /StructParent %d, the key the PAGE already owns. One "+
				"entry cannot be both an array of MCIDs and a reference to an element, so "+
				"whichever was written second destroyed the other", i, l.structParent)
		}
	}
	if _, defects := checkTree(t, out); len(defects) > 0 {
		t.Errorf("tree defects after describing a form on an already-tagged page: %v", defects)
	}
}

// TestTheDescribedFormClearsTheClauseS05CouldNot — S07's third acceptance clause, on the ua1 oracle.
func TestTheDescribedFormClearsTheClauseS05CouldNot(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P06's exit criterion for the authored " +
			"form is UNCHECKED in this run.")
	}
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	fields := s07Fields()
	plain, err := AuthorForm(base, fields)
	if err != nil {
		t.Fatal(err)
	}
	described, tagged, err := AuthorTaggedForm(base, fields)
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("AuthorTaggedForm fell back, so this differential is not about a described form")
	}
	// A language on both, so 7.2 t25/t34 do not confound the comparison — S05 measured that the
	// catalog /Lang is what clears them, and this is not the clause under test.
	plain, err = SetLang(plain, "en")
	if err != nil {
		t.Fatal(err)
	}
	described, err = SetLang(described, "en")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"plain.pdf": plain, "described.pdf": described} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["plain.pdf"], cl["described.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	// The stimulus: the undescribed form must actually fail the clause, or clearing it is vacuous.
	if !before["7.18.4 t1"] {
		t.Fatalf("setup: the undescribed form does not fail 7.18.4 t1, so this test is not "+
			"measuring what S07 exists for. It fails %v", sortedClauses(before))
	}
	if after["7.18.4 t1"] {
		t.Errorf("the described form still fails 7.18.4 t1 — a widget is not nested in a Form "+
			"element. It fails %v", sortedClauses(after))
	}
	// And nothing was traded for it.
	var added []string
	for c := range after {
		if !before[c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		t.Errorf("describing the form ADDED ua1 clause(s) it did not fail: %s",
			strings.Join(added, ", "))
	}
	t.Logf("undescribed form fails %v\ndescribed form fails %v",
		sortedClauses(before), sortedClauses(after))
}

// TestTheFormsTreeSaysItCameFromNibsOwnFieldList — S07's fourth acceptance clause.
//
// The widget-to-field correspondence is nib's own authored input: the caller placed these fields and
// named them, so which element describes which widget is KNOWN. That is D4's exact tier, the same
// footing as `mdpdf`'s AST and unlike an OCR engine's reading of a picture.
func TestTheFormsTreeSaysItCameFromNibsOwnFieldList(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	out, tagged, err := AuthorTaggedForm(base, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("AuthorTaggedForm fell back to the untagged form")
	}
	src, ok := StructureSource(out)
	if !ok {
		t.Fatal("the described form records no structure source, so every tree nib writes looks " +
			"equally trustworthy")
	}
	if src != sourceExact {
		t.Errorf("the form's tree records source %q, want %q", src, sourceExact)
	}
}

// TestTheFormSURVIVESATreeItCannotBeGiven — S07's fifth acceptance clause, the both-halves law.
//
// `/MarkInfo` and the tree go together or neither goes: a catalog claiming `Marked true` over a
// document with nothing in its tree is `orphaned()`, which ADR-031 law 1 forbids and which
// `tagMarkdown`, `TagOCRLayer` and `commitProposal` all hold as their own post-condition.
//
// What must not happen is losing the form. A form whose widgets are undescribed is what nib shipped
// for years; a document with no fields is worse than both.
func TestTheFormSURVIVESATreeItCannotBeGiven(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	out, tagged, err := AuthorTaggedForm(base, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	// The ordinary path first, as the control: without it the two assertions below hold trivially on
	// a build where nothing is ever described.
	if !tagged {
		t.Fatal("control: the ordinary path did not describe the form, so the post-conditions " +
			"below are not being checked against a tagged document")
	}
	if s := inspectTags(out); s.orphaned() {
		t.Errorf("the described form is orphaned (%+v): it claims tagging its content does not "+
			"support, which is the state the post-condition exists to refuse", s)
	}
	// Both halves present, checked separately — a document with one is worse than one with neither.
	s := inspectTags(out)
	if !s.marked {
		t.Error("the catalog does not say /MarkInfo /Marked true, so a reader is never told to " +
			"look for the tree that is there")
	}
	if !s.tree {
		t.Error("there is no /StructTreeRoot, so /MarkInfo is a claim with nothing behind it")
	}
	// And the fields are real either way.
	js, jerr := ExportFormJSON(out)
	if jerr != nil {
		t.Fatalf("ExportFormJSON: %v", jerr)
	}
	for _, name := range []string{"full_name", "agree"} {
		if !strings.Contains(string(js), name) {
			t.Errorf("field %q is gone from the described form — describing a form must never "+
				"cost the user the form: %s", name, js)
		}
	}
}
