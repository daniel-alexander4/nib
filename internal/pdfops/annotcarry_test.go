package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// `/pending 562`'s readers — an n-up carries the user's sticky notes onto the sheets their pages
// landed on.
//
// **Every one of these was red before the change, and the first was red as `0 annotations`**: the
// measurement that opened the item is 3 notes in and 0 out, and the consequence that made it
// visible is `/pending 488`'s golden, where `ContentDigest` of `notes-then-nup` was byte-identical
// to that of `nup2` because there was nothing left of the notes to hash.

// carriedNote is one annotation read back off a sheet.
type carriedNote struct {
	sheet              int
	subtype            string
	text               string
	llx, lly, urx, ury float64
}

// notesOn reads every annotation off every page, with its rect and its comment.
func notesOn(t *testing.T, pdf []byte) []carriedNote {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	var out []carriedNote
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			t.Fatalf("page %d: %v", p, perr)
		}
		annots, aerr := ctx.DereferenceArray(d["Annots"])
		if aerr != nil {
			t.Fatalf("page %d /Annots: %v", p, aerr)
		}
		for _, a := range annots {
			ad, derr := ctx.DereferenceDict(a)
			if derr != nil || ad == nil {
				continue
			}
			n := carriedNote{sheet: p}
			if sub := ad.NameEntry("Subtype"); sub != nil {
				n.subtype = *sub
			}
			if s, ok := ad["Contents"].(types.StringLiteral); ok {
				// `types.StringLiteralToString` un-escapes AND decodes the UTF-16BE `AddNotes`
				// writes; reading `.Value()` would compare against interleaved NULs.
				if dec, derr := types.StringLiteralToString(s); derr == nil {
					n.text = dec
				}
			}
			llx, lly, urx, ury, ok := rectOf(ctx, ad["Rect"])
			if !ok {
				t.Fatalf("page %d: an annotation has no readable /Rect", p)
			}
			n.llx, n.lly, n.urx, n.ury = llx, lly, urx, ury
			out = append(out, n)
		}
	}
	return out
}

// notedThreePages is three A4 pages, one sticky note each, at the same point on every page.
//
// The point matters: `AddNotes` puts the icon's top-left at (72, 700) on a 595×842 page, so the
// note is near the TOP LEFT of its page and a transform that lost the rotation, the scale or the
// translation puts it somewhere a check can name.
func notedThreePages(t *testing.T) []byte {
	t.Helper()
	three, err := testpdf.Text("clause one", "clause two", "clause three")
	if err != nil {
		t.Fatalf("building the fixture: %v", err)
	}
	notes := make([]Note, 0, 3)
	for i := 1; i <= 3; i++ {
		notes = append(notes, Note{Page: i, X: 72, Y: 700, Text: fmt.Sprintf("note %d", i)})
	}
	noted, err := AddNotes(three, notes)
	if err != nil {
		t.Fatalf("adding notes: %v", err)
	}
	return noted
}

// TestANUpCarriesEveryNoteOntoTheSheetItsPageLandedOn is the item's own measurement, inverted.
//
// **The expected rects are hand-derived from the sheet's own placement, not copied off a run.**
// `api.NUp` draws source page 1 under `0 0.70665 -0.70665 0 595 421.27138 cm` and page 2 under the
// same matrix with `f = 0.27138`; page 3 opens sheet 2 under page 1's. The scale is arithmetic —
// a 595×842 page best-fitting into a 595×421 half-sheet turns 90° and scales by 595/842 = 0.70665 —
// and the matrix maps `(x, y)` to `(595 − 0.70665·y, f + 0.70665·x)`. So the note at
// `[72 680 92 700]` lands at `[100.34 472.15 114.48 486.29]` on the top tile, and 421 lower on the
// bottom one.
func TestANUpCarriesEveryNoteOntoTheSheetItsPageLandedOn(t *testing.T) {
	const s = 595.0 / 842.0 // the best-fit scale, from the page and half-sheet dimensions alone
	up, err := NUp(notedThreePages(t), 2, false)
	if err != nil {
		t.Fatalf("n-up: %v", err)
	}
	got := notesOn(t, up)
	if len(got) != 3 {
		t.Fatalf("three notes went into the n-up and %d came out; `api.NUp` drops every annotation "+
			"with the page dictionaries it removes, so a count below three is the loss /pending 562 "+
			"opened on: %+v", len(got), got)
	}
	want := []struct {
		sheet              int
		text               string
		llx, lly, urx, ury float64
	}{
		{1, "note 1", 595 - s*700, 421.27138 + s*72, 595 - s*680, 421.27138 + s*92},
		{1, "note 2", 595 - s*700, 0.27138 + s*72, 595 - s*680, 0.27138 + s*92},
		{2, "note 3", 595 - s*700, 421.27138 + s*72, 595 - s*680, 421.27138 + s*92},
	}
	byText := map[string]carriedNote{}
	for _, n := range got {
		if n.subtype != "Text" {
			t.Errorf("an annotation of subtype /%s reached a sheet; this door carries /Text only", n.subtype)
		}
		byText[n.text] = n
	}
	for _, w := range want {
		n, ok := byText[w.text]
		if !ok {
			t.Errorf("%q is not on any sheet; the notes that are: %v", w.text, byText)
			continue
		}
		if n.sheet != w.sheet {
			t.Errorf("%q landed on sheet %d and its page landed on sheet %d — a note must follow "+
				"its own page", w.text, n.sheet, w.sheet)
		}
		const tol = 0.05
		if math.Abs(n.llx-w.llx) > tol || math.Abs(n.lly-w.lly) > tol ||
			math.Abs(n.urx-w.urx) > tol || math.Abs(n.ury-w.ury) > tol {
			t.Errorf("%q is at [%.2f %.2f %.2f %.2f] and the matrix that placed its page puts it at "+
				"[%.2f %.2f %.2f %.2f] — the rect must go through the same transform the content did",
				w.text, n.llx, n.lly, n.urx, n.ury, w.llx, w.lly, w.urx, w.ury)
		}
	}
}

// TestANUpsNoteIsScaledAndNotMerelyMoved pins the half a tile check cannot see.
//
// A carry that appended the source rect unchanged puts note 1 at `[72 680 92 700]`, which is still
// in the top tile and still on the right sheet — so "the note is on the sheet its page landed on"
// passes over it. The icon box is 20 points square in page space (`noteIconSize`) and the placement
// scales by 595/842, so a carried note is **14.13 points square**, and an uncarried transform is
// 20. That is the one number that separates them.
func TestANUpsNoteIsScaledAndNotMerelyMoved(t *testing.T) {
	up, err := NUp(notedThreePages(t), 2, false)
	if err != nil {
		t.Fatalf("n-up: %v", err)
	}
	want := noteIconSize * 595.0 / 842.0
	for _, n := range notesOn(t, up) {
		w, h := n.urx-n.llx, n.ury-n.lly
		if math.Abs(w-want) > 0.05 || math.Abs(h-want) > 0.05 {
			t.Errorf("%q is %.2f×%.2f points on the sheet and the placement scales by 595/842, so a "+
				"transformed %g-point icon box is %.2f×%.2f — an unscaled rect means the annotation "+
				"was copied without the matrix", n.text, w, h, noteIconSize, want, want)
		}
	}
}

// TestANotedTaggedDocumentKeepsItsTagTreeThroughAnNUp is the cost the item did not know about.
//
// `describeNotes` nests a note of a tagged document in an `/Annot` element whose `/ParentTree` key
// is claimed by the annotation's own `/StructParent`. Dropping the annotation leaves that key
// claimed by nobody, which is `structureCarriedCompletely`'s `unowned-key`, so `completeOrHonest`
// abandoned the entire carry and `honest` deleted `/StructTreeRoot`. **Measured before the change,
// on this very fixture: `unowned-key key=2` and `unowned-key key=3`, fate `dropped`, no tree.**
// The same document without notes n-ups `carried`, which is what makes this a note's fault.
func TestANotedTaggedDocumentKeepsItsTagTreeThroughAnNUp(t *testing.T) {
	must := mustFn(t)
	src := collidingMCIDFixture()
	if f := inspectTags(must(NUp(src, 2, false))); !f.claimsHonestly() {
		t.Fatalf("the fixture does not carry its tags through an n-up even without notes (%v), so "+
			"this test cannot tell a note's cost from a pre-existing loss", f)
	}
	noted := must(AddNotes(src, []Note{
		{Page: 1, X: 72, Y: 700, Text: "note one"},
		{Page: 2, X: 72, Y: 700, Text: "note two"},
	}))
	up := must(NUp(noted, 2, false))
	if f := inspectTags(up); !f.claimsHonestly() {
		t.Errorf("a tagged document with two sticky notes n-ups to %v; without the notes the same "+
			"document carries, so the note is what costs the tree", f)
	}
	if n := len(notesOn(t, up)); n != 2 {
		t.Errorf("two notes went in and %d came out", n)
	}
}

// TestACarriedNotesAnnotElementNamesTheAnnotationOnTheSheet is the defect the tree check cannot see.
//
// An `OBJR` is reachable from `/StructTreeRoot` and pdfcpu writes by reachability, so an OBJR left
// naming the SOURCE annotation keeps that annotation in the file — on no page, claiming the same
// `/ParentTree` key as the copy. `parentTreeOwners` walks the annotations of pages, so it cannot see
// the second claimant and `structureCarriedCompletely` passes either way. Measured with the carry in
// and the repoint out: `OBJR -> obj 15 onPage=0` and `OBJR -> obj 16 onPage=0`, beside two fresh
// copies on sheet 1 that the tree described nothing about.
func TestACarriedNotesAnnotElementNamesTheAnnotationOnTheSheet(t *testing.T) {
	must := mustFn(t)
	noted := must(AddNotes(collidingMCIDFixture(), []Note{
		{Page: 1, X: 72, Y: 700, Text: "note one"},
		{Page: 2, X: 72, Y: 700, Text: "note two"},
	}))
	up := must(NUp(noted, 2, false))

	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(up), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	onPage := map[int]int{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, _ := ctx.PageDict(p, false)
		arr, _ := ctx.DereferenceArray(d["Annots"])
		for _, a := range arr {
			if ir, ok := a.(types.IndirectRef); ok {
				onPage[ir.ObjectNumber.Value()] = p
			}
		}
	}
	targets := objrTargets(t, ctx)
	if len(targets) != 2 {
		t.Fatalf("the carried tree holds %d OBJR(s) and the two notes were described by two; a "+
			"missing one is an /Annot element describing nothing", len(targets))
	}
	for _, nr := range targets {
		if onPage[nr] == 0 {
			t.Errorf("an /Annot element's OBJR names object %d, which is on no page — the element "+
				"describes the source annotation the composition orphaned, not the copy on the sheet",
				nr)
		}
	}
}

// objrTargets returns the object number every OBJR in the structure tree names.
func objrTargets(t *testing.T, ctx *model.Context) []int {
	t.Helper()
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
	if rerr != nil || root == nil {
		t.Fatalf("the n-up has no structure tree, so there is no OBJR to read")
	}
	var out []int
	seen := map[int]bool{}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 64 {
			return
		}
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x, depth+1)
			}
			return
		}
		if ir, isRef := o.(types.IndirectRef); isRef {
			if seen[ir.ObjectNumber.Value()] {
				return
			}
			seen[ir.ObjectNumber.Value()] = true
		}
		d, e := ctx.DereferenceDict(o)
		if e != nil || d == nil {
			return
		}
		if ty := d.NameEntry("Type"); ty != nil && *ty == "OBJR" {
			if ir, isRef := d["Obj"].(types.IndirectRef); isRef {
				out = append(out, ir.ObjectNumber.Value())
			}
		}
		if k, ok := d["K"]; ok {
			walk(k, depth+1)
		}
	}
	walk(root["K"], 0)
	return out
}

// TestAnNUpLeavesAWidgetBehindAndTheBoundaryIsDeclared pins the refusal, which is the half of this
// door that is easiest to widen by accident.
//
// A `/Widget` is not carried for two reasons stated in `annotcarry.go`: its appearance stream would
// be drawn upright inside a rect the placement rotates, and a signature widget reaching a `/V` blob
// would return to a document whose byte ranges the composition destroyed, past four gates that key
// on an edited document having no signature. The observable is that the composition still reads
// `unsigned` and the sheet holds no widget.
func TestAnNUpLeavesAWidgetBehindAndTheBoundaryIsDeclared(t *testing.T) {
	must := mustFn(t)
	form := must(testpdf.Form())
	filled := must(FillFormJSON(form, []byte(
		`{"forms":[{"textfield":[{"name":"fullName","value":"Jane Doe"}],"checkbox":[{"name":"agree","value":true}]}]}`)))
	if n := len(notesOn(t, filled)); n != 2 {
		t.Fatalf("the filled form carries %d annotations and the fixture has two widgets; the "+
			"refusal below cannot be observed without them", n)
	}
	up := must(NUp(filled, 2, false))
	for _, a := range notesOn(t, up) {
		t.Errorf("an annotation of subtype /%s reached the sheet; this door carries /Text only, "+
			"and a widget in particular is refused", a.subtype)
	}
}

// linkFixture is two pages whose first carries a `/Link` annotation and nothing else.
//
// **Its point is that the link is COPYABLE.** Every key is direct — no `/AP`, no indirect action —
// so `copyAnnot` would rebuild it happily and the only thing standing between it and the sheet is
// the subtype test. The widget fixture cannot say that: a filled form's widgets carry indirect
// appearance streams, so `copyAnnot` refuses them whatever the subtype rule says, and the two
// reasons are indistinguishable there. Measured — widening the subtype test to accept everything
// left `TestAnNUpLeavesAWidgetBehindAndTheBoundaryIsDeclared` GREEN, which is the coverage hole this
// fixture closes.
func linkFixture() []byte {
	content := "BT /F1 24 Tf 72 700 Td (page one) Tj ET\n"
	other := "BT /F1 24 Tf 72 700 Td (page two) Tj ET\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 8 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> " +
			"/Contents 4 0 R /Annots [6 0 R] >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Link /Rect [72 680 200 700] /Border [0 0 0] " +
			"/A << /Type /Action /S /URI /URI (https://example.invalid/) >> >>",
		8: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> " +
			"/Contents 9 0 R >>",
		9: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(other), other),
	})
}

// TestAnNUpLeavesACopyableNonNoteAnnotationBehind is the subtype rule itself.
//
// The door carries `/Text` and nothing else, and this is the only test that can say the SUBTYPE is
// why: the link is fully copyable, so accepting every subtype puts it on the sheet.
func TestAnNUpLeavesACopyableNonNoteAnnotationBehind(t *testing.T) {
	must := mustFn(t)
	src := linkFixture()
	in := notesOn(t, src)
	if len(in) != 1 || in[0].subtype != "Link" {
		t.Fatalf("the fixture should carry exactly one /Link and carries %+v; without it the "+
			"subtype rule has nothing to refuse", in)
	}
	for _, a := range notesOn(t, must(NUp(src, 2, false))) {
		t.Errorf("an annotation of subtype /%s reached the sheet; this door carries /Text alone, "+
			"because only a sticky note is drawn from its /Rect with no appearance to rotate", a.subtype)
	}
}

// TestANUpCarriesANoteThroughAPagesOwnCropOffset is the form `/Matrix` term, which the plain
// fixture cannot reach because its boxes start at the origin.
//
// `createNUpFormForPDF` writes the form's `/Matrix` as a translation by `−cropBox.LL`, so a page
// whose box does NOT start at the origin has a non-identity form matrix between its own coordinates
// and the sheet's. **`SplitPage` produces exactly that** — measured, its second tile carries
// `/CropBox [297.5 0 595 842]` — so the term is reachable through nib's own operations rather than
// only through a foreign producer.
//
// Hand-derived: the sheet draws that tile under `0.5 0 0 0.5 74.375 0 cm`, and the form matrix
// translates by −297.5 first, so a page point `(x, y)` reaches `(0.5·x − 0.5·297.5 + 74.375, 0.5·y)`.
// **Dropping the form matrix moves the note 148.75 points right, clean out of a 297.5-wide image.**
func TestANUpCarriesANoteThroughAPagesOwnCropOffset(t *testing.T) {
	must := mustFn(t)
	const (
		cropLLx = 297.5  // the second tile's /CropBox lower-left x
		scale   = 0.5    // the sheet's own cm for that tile
		tx      = 74.375 // ... and its translation
		noteX   = 400.0  // inside the tile's own coordinate space, which starts at 297.5
		noteY   = 700.0
	)
	two := must(testpdf.Text("clause one", "clause two"))
	split := must(SplitPage(two, 1, 2, 1, false))
	noted := must(AddNotes(split, []Note{{Page: 2, X: noteX, Y: noteY, Text: "split note"}}))
	up := must(NUp(noted, 2, false))

	got := notesOn(t, up)
	if len(got) != 1 {
		t.Fatalf("one note went in and %d came out: %+v", len(got), got)
	}
	n := got[0]
	wantLLx := scale*noteX - scale*cropLLx + tx
	wantLLy := scale * (noteY - noteIconSize)
	const tol = 0.05
	if math.Abs(n.llx-wantLLx) > tol || math.Abs(n.lly-wantLLy) > tol {
		t.Errorf("the note is at (%.3f, %.3f) and its tile's crop offset puts it at (%.3f, %.3f); "+
			"the form /Matrix translates by -%.1f before the sheet's own cm, so ignoring it moves "+
			"the note %.2f points", n.llx, n.lly, wantLLx, wantLLy, cropLLx, scale*cropLLx)
	}
}

// TestANUpCarriesANoteThroughAPagesOwnRotation is the rotation-prefix term.
//
// A source page with `/Rotate` has `ContentBytesForPageRotation`'s `cm` prepended INSIDE its form,
// so form space is not page space and an annotation rect — which is always in UNROTATED page space
// — has to go through that matrix too. This code reads the prefix back off the form rather than
// recomputing it, and this is what says the reading is right.
//
// Hand-derived, and each step is a document fact: pdfcpu swaps the crop box for a ±90° rotation, so
// the form covers 842×595 and its prefix rotates by −90 and translates by `(0, 595)` — mapping
// `(x, y)` to `(y, 595 − x)`. The sheet then draws it under `0.5 0 0 0.5 210.5 297.5 cm`. So the
// icon box `[72 680 92 700]` lands at `[550.5 549 560.5 559]` — the top RIGHT of the page's image,
// which is where a 90°-turned page puts what was its top left. **Ignoring the prefix puts it at
// `(246.5, 637.5)`, off a 595-point-high sheet entirely.**
func TestANUpCarriesANoteThroughAPagesOwnRotation(t *testing.T) {
	must := mustFn(t)
	const (
		scale = 0.5
		tx    = 210.5
		ty    = 297.5
		swapH = 595.0 // the swapped crop box's height, which the rotation translates by
	)
	three := must(testpdf.Text("clause one", "clause two", "clause three"))
	rotated := must(Rotate(three, nil, 90))
	noted := must(AddNotes(rotated, []Note{{Page: 1, X: 72, Y: 700, Text: "turned note"}}))
	up := must(NUp(noted, 2, false))

	got := notesOn(t, up)
	if len(got) != 1 {
		t.Fatalf("one note went in and %d came out: %+v", len(got), got)
	}
	n := got[0]
	// The icon box in unrotated page space, then through (x, y) -> (y, swapH - x), then the sheet.
	turn := func(x, y float64) (float64, float64) { return scale*y + tx, scale*(swapH-x) + ty }
	ax, ay := turn(72, 680)
	bx, by := turn(92, 700)
	wantLLx, wantURx := math.Min(ax, bx), math.Max(ax, bx)
	wantLLy, wantURy := math.Min(ay, by), math.Max(ay, by)
	const tol = 0.05
	if math.Abs(n.llx-wantLLx) > tol || math.Abs(n.lly-wantLLy) > tol ||
		math.Abs(n.urx-wantURx) > tol || math.Abs(n.ury-wantURy) > tol {
		t.Errorf("the note is at [%.2f %.2f %.2f %.2f] and its page's own /Rotate 90 puts it at "+
			"[%.2f %.2f %.2f %.2f]; the rotation is prepended inside the form, so a transform that "+
			"reads only the sheet's cm leaves the rect in unrotated page space",
			n.llx, n.lly, n.urx, n.ury, wantLLx, wantLLy, wantURx, wantURy)
	}
}

// composeFor runs the composition `NUp` performs, up to the point it hands `carryNoteAnnots` the
// sheets — so a test can read the carry's own report, which `NUp` discards.
func composeFor(t *testing.T, pdf []byte, n int) []byte {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	nup, err := api.PDFNUpConfig(n, "border:off, margin:0", conf)
	if err != nil {
		t.Fatalf("n-up config: %v", err)
	}
	var out bytes.Buffer
	if err := api.NUp(bytes.NewReader(pdf), &out, nil, nil, nup, conf); err != nil {
		t.Fatalf("composing: %v", err)
	}
	raw, err := withoutUAClaim(out.Bytes())
	if err != nil {
		t.Fatalf("dropping the UA claim: %v", err)
	}
	return raw
}

// TestTheNoteCarryCountsWhatItLeftBehind gives the residue figure a reader.
//
// ADR-045's declared gap is that an annotation which is not a `/Text` is still dropped and still
// without a sentence — and the door's answer to that is to COUNT it, so the residue is a number
// somebody can ask for rather than a silence. A count nothing reads is the same silence one level
// in, so this is what makes the claim true today.
func TestTheNoteCarryCountsWhatItLeftBehind(t *testing.T) {
	must := mustFn(t)

	noted := notedThreePages(t)
	if _, rep := carryNoteAnnots(noted, composeFor(t, noted, 2)); rep.carried != 3 || rep.left != 0 {
		t.Errorf("three sticky notes and nothing else reported carried=%d left=%d, want 3 and 0",
			rep.carried, rep.left)
	}

	form := must(testpdf.Form())
	filled := must(FillFormJSON(form, []byte(
		`{"forms":[{"textfield":[{"name":"fullName","value":"Jane Doe"}],"checkbox":[{"name":"agree","value":true}]}]}`)))
	if _, rep := carryNoteAnnots(filled, composeFor(t, filled, 2)); rep.carried != 0 || rep.left != 2 {
		t.Errorf("a filled form's two widgets reported carried=%d left=%d, want 0 and 2 — the "+
			"widgets are the residue ADR-045 declares, and a count that does not see them is the "+
			"silence it exists to replace", rep.carried, rep.left)
	}
}

// mustFn binds a fixture builder to one test, so a failure names the test rather than a helper.
func mustFn(t *testing.T) func([]byte, error) []byte {
	t.Helper()
	return func(b []byte, err error) []byte {
		t.Helper()
		if err != nil {
			t.Fatalf("building the fixture: %v", err)
		}
		return b
	}
}
