package uacheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P05.S01 — every verdict in the table below is veraPDF 1.30.2's, measured on that exact document
// before the rule was written (the slice's grill table in `PLAN-ua-coverage.md` records the run).

// annotFixture is a one-page tagged document with one `/P` element in the tree and one structure
// element (object 10) that an annotation's `/StructParent 1` names through the parent tree.
type annotFixture struct {
	// annot is object 30, the annotation. Empty means the page carries no `/Annots` at all.
	annot string
	// elem is the entries of the element the annotation's /StructParent names; empty drops it, and the
	// parent tree then has no row 1.
	elem string
	// annots overrides the page's whole /Annots array, for inline dictionaries and dangling references.
	annots string
	// page and pages are extra entries on the page and on the page tree node (/CropBox lives on either).
	page, pages string
	// cat is extra catalog entries, and extra adds or replaces objects.
	cat   string
	extra map[int]string
	// noTabs drops the page's `/Tabs /S`, which is otherwise always written — 7.18.3 t1 is the only clause
	// that reads it, and every other row wants it present so that clause does not fail for an unrelated reason.
	noTabs bool
}

const annotFixtureText = "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"

func (f annotFixture) build() []byte {
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 " + f.pages + ">>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(annotFixtureText), annotFixtureText),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
	}
	if f.elem != "" {
		objs[7] = "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R >>"
		objs[9] = "<< /Nums [0 [8 0 R] 1 10 0 R] >>"
		objs[10] = "<< /Type /StructElem " + f.elem + " /P 7 0 R /Pg 3 0 R /K [<< /Type /OBJR /Obj 30 0 R >>] >>"
	} else {
		objs[7] = "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>"
		objs[9] = "<< /Nums [0 [8 0 R]] >>"
	}
	annots := ""
	if f.annot != "" {
		objs[30] = f.annot
		annots = "/Annots [30 0 R] "
	}
	if f.annots != "" {
		annots = "/Annots " + f.annots + " "
	}
	tabs := "/Tabs /S "
	if f.noTabs {
		tabs = ""
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " + tabs + annots +
		f.page + " /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-US) " +
		f.cat + " >>"
	for k, v := range f.extra {
		objs[k] = v
	}
	return buildPDF(objs)
}

// note is a `/Text` annotation with overridable entries — the shape `pdfops.AddNotes` writes.
func note(entries ...string) string {
	d := map[string]string{"Type": "/Annot", "Subtype": "/Text", "Rect": "[0 0 10 10]", "F": "4",
		"Contents": "(a note)", "StructParent": "1"}
	for i := 0; i+1 < len(entries); i += 2 {
		if entries[i+1] == "" {
			delete(d, entries[i])
			continue
		}
		d[entries[i]] = entries[i+1]
	}
	// A stable order, so a fixture's bytes do not move between runs. A key not on the list would be
	// dropped silently and the fixture would still "pass", so an unknown one panics instead.
	order := []string{"Type", "Subtype", "Rect", "F", "Contents", "StructParent", "Alt", "FT", "T", "TU", "Parent", "AP", "DA"}
	for k := range d {
		if !contains(order, k) {
			panic("annotFixture: /" + k + " is not in note()'s key order, so it would be dropped silently")
		}
	}
	var b strings.Builder
	b.WriteString("<<")
	for _, k := range order {
		if v, ok := d[k]; ok {
			fmt.Fprintf(&b, " /%s %s", k, v)
		}
	}
	b.WriteString(" >>")
	return b.String()
}

const annotTag = "/S /Annot"

func TestTheAnnotationRulesAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fx     annotFixture
		t1, t2 Verdict
	}{
		// --- 7.18.1 t1: the tag is the enclosing element's STANDARD type ------------------------
		{"an Annot tag", annotFixture{annot: note(), elem: annotTag}, Pass, Pass},
		{"a P tag", annotFixture{annot: note(), elem: "/S /P"}, Fail, Pass},
		{"no /StructParent at all", annotFixture{annot: note("StructParent", ""), elem: annotTag}, Fail, Pass},
		{"a /StructParent naming no row", annotFixture{annot: note("StructParent", "7"), elem: annotTag}, Fail, Pass},
		{"a /StructParent whose row is an array", annotFixture{annot: note(), elem: annotTag,
			extra: map[int]string{9: "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>"}}, Fail, Pass},
		{"an indirect /StructParent", annotFixture{annot: note("StructParent", "40 0 R"), elem: annotTag,
			extra: map[int]string{40: "1"}}, Pass, Pass},
		{"a private type role-mapped to /Annot", annotFixture{annot: note(), elem: "/S /MyNote",
			extra: map[int]string{7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyNote /Annot >> >>"}},
			Pass, Pass},
		// The exclusions are by SUBTYPE and there are exactly three; a Popup and a subtype-less
		// annotation are both graded, which is why the list cannot be "annotations with a rule of
		// their own".
		// Both rows drop `/Contents` so that t2 GRADES them: with it present their t2 Pass comes from the
		// description and says nothing about whether the subtype is excluded there (only Widget is).
		{"a Link, excluded from t1 and graded by t2", annotFixture{annot: note("Subtype", "/Link", "StructParent", "", "Contents", "")}, Pass, Fail},
		{"a Popup, excluded from neither", annotFixture{annot: note("Subtype", "/Popup", "StructParent", "", "Contents", "")}, Fail, Fail},
		{"a visible widget with no /StructParent, excluded from t1", annotFixture{annot: note("Subtype", "/Widget",
			"StructParent", "", "FT", "/Btn", "T", "(b)")}, Pass, Pass},
		{"a PrinterMark, excluded from t1 and graded by t2", annotFixture{annot: note("Subtype", "/PrinterMark",
			"StructParent", "", "Contents", "", "AP", "<< /N 41 0 R >>"),
			extra: map[int]string{41: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"}}, Pass, Fail},
		// --- the shared exemption: hidden, or outside the crop box -----------------------------
		{"hidden, /F 2", annotFixture{annot: note("StructParent", "", "F", "2")}, Pass, Pass},
		{"hidden among other flags, /F 6", annotFixture{annot: note("StructParent", "", "F", "6")}, Pass, Pass},
		{"an indirect /F", annotFixture{annot: note("StructParent", "", "F", "40 0 R"),
			extra: map[int]string{40: "2"}}, Pass, Pass},
		{"wholly outside the crop box", annotFixture{annot: note("StructParent", ""),
			page: "/CropBox [100 100 500 500]"}, Pass, Pass},
		{"touching the crop box edge exactly", annotFixture{annot: note("StructParent", "", "Rect", "[90 90 100 100]"),
			page: "/CropBox [100 100 500 500]"}, Pass, Pass},
		{"overlapping the crop box edge by one unit", annotFixture{annot: note("StructParent", "", "Rect", "[90 90 101 101]"),
			page: "/CropBox [100 100 500 500]"}, Fail, Pass},
		{"a zero-area rectangle on the crop box corner", annotFixture{annot: note("StructParent", "", "Rect", "[100 100 100 100]"),
			page: "/CropBox [100 100 500 500]"}, Pass, Pass},
		{"real coordinates just inside", annotFixture{annot: note("StructParent", "", "Rect", "[99.5 99.5 100.5 100.5]"),
			page: "/CropBox [100 100 500 500]"}, Fail, Pass},
		// A reversed rectangle is used AS WRITTEN: sorting the corners first would call this one
		// inside, and veraPDF calls it outside. The control below it reads the other way.
		{"a reversed rectangle veraPDF calls outside", annotFixture{annot: note("StructParent", "", "Rect", "[600 600 50 50]"),
			page: "/CropBox [100 100 500 500]"}, Pass, Pass},
		{"a reversed rectangle veraPDF calls inside", annotFixture{annot: note("StructParent", "", "Rect", "[400 400 200 200]"),
			page: "/CropBox [100 100 500 500]"}, Fail, Pass},
		// null is NOT the exemption: the profile tests `isOutsideCropBox == true`.
		{"no /Rect at all", annotFixture{annot: note("StructParent", "", "Rect", "")}, Fail, Pass},
		{"a /Rect of three numbers", annotFixture{annot: note("StructParent", "", "Rect", "[0 0 10]")}, Fail, Pass},
		{"an indirect /Rect", annotFixture{annot: note("StructParent", "", "Rect", "40 0 R"),
			page: "/CropBox [100 100 500 500]", extra: map[int]string{40: "[0 0 10 10]"}}, Pass, Pass},
		// The crop box is inherited, clipped to the media box, and falls back to it entirely.
		{"an inherited crop box", annotFixture{annot: note("StructParent", ""),
			pages: "/CropBox [100 100 500 500] "}, Pass, Pass},
		{"a crop box larger than the media box", annotFixture{annot: note("StructParent", "", "Rect", "[700 800 750 850]"),
			page: "/CropBox [0 0 1000 1000]"}, Pass, Pass},
		{"no crop box, off the media box", annotFixture{annot: note("StructParent", "", "Rect", "[700 800 750 850]")}, Pass, Pass},
		// t2 carries the SAME exemption, and every row above reaches its `continue` through `/Contents`
		// instead — so without these two, deleting t2's whole guard leaves the table green (measured: it
		// did). An exempt annotation with nothing describing it is the shipped 7.18.4 t1 defect's shape.
		{"hidden, and nothing describes it", annotFixture{annot: note("StructParent", "", "F", "2", "Contents", ""),
			elem: annotTag}, Pass, Pass},
		{"off the crop box, and nothing describes it", annotFixture{annot: note("StructParent", "", "Contents", ""),
			elem: annotTag, page: "/CropBox [100 100 500 500]"}, Pass, Pass},
		{"a widget with nothing describing it", annotFixture{annot: note("Subtype", "/Widget", "Contents", "",
			"FT", "/Btn", "T", "(b)"), elem: "/S /Form"}, Pass, Pass},
		// F5: the UPPER-RIGHT half of the inclusive comparison. The rows above pin only the lower-left
		// pair, so making `<=` exclusive left the table green (measured). Both verdicts are veraPDF's.
		{"the crop box's upper-right corner is the annotation's lower-left", annotFixture{annot: note("StructParent", "",
			"Rect", "[500 500 600 600]"), page: "/CropBox [100 100 500 500]"}, Pass, Pass},
		{"overlapping the upper-right corner by one unit", annotFixture{annot: note("StructParent", "",
			"Rect", "[499 499 600 600]"), page: "/CropBox [100 100 500 500]"}, Fail, Pass},
		// F7: nib refuses rather than guesses when the role map cannot be followed. veraPDF FAILS this
		// document (measured); CannotCheck is nib's declared limit and the oracle never scores it.
		{"an element on a role-map loop", annotFixture{annot: note(), elem: "/S /MyNote",
			extra: map[int]string{7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyNote /Other /Other /MyNote >> >>"}},
			CannotCheck, Pass},
		// --- 7.18.1 t2: /Contents, else the ENCLOSING ELEMENT's /Alt ---------------------------
		{"no /Contents and no /Alt", annotFixture{annot: note("Contents", ""), elem: annotTag}, Pass, Fail},
		{"an empty /Contents", annotFixture{annot: note("Contents", "()"), elem: annotTag}, Pass, Fail},
		{"a hex-string /Contents", annotFixture{annot: note("Contents", "<FEFF00610062>"), elem: annotTag}, Pass, Pass},
		{"an indirect /Contents", annotFixture{annot: note("Contents", "40 0 R"), elem: annotTag,
			extra: map[int]string{40: "(a note)"}}, Pass, Pass},
		{"an /Alt on the element", annotFixture{annot: note("Contents", ""), elem: annotTag + " /Alt (a description)"}, Pass, Pass},
		{"an indirect /Alt on the element", annotFixture{annot: note("Contents", ""), elem: annotTag + " /Alt 40 0 R",
			extra: map[int]string{40: "(described)"}}, Pass, Pass},
		{"an empty /Alt on the element", annotFixture{annot: note("Contents", ""), elem: annotTag + " /Alt ()"}, Pass, Fail},
		// The /Alt veraPDF reads is the ELEMENT's; one on the annotation itself describes nothing.
		{"an /Alt on the annotation itself", annotFixture{annot: note("Contents", "", "Alt", "(on the annotation)"),
			elem: annotTag}, Pass, Fail},
		// --- the population --------------------------------------------------------------------
		{"an annotation written inline in /Annots", annotFixture{elem: annotTag,
			annots: "[<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (a) /StructParent 1 >>]"}, Pass, Pass},
		// pdfcpu REMOVES a dangling reference from the array before any rule sees it (measured, v0.13.0),
		// so this row measures nib's answer over a page that then has no annotations — not the door's
		// own "a non-dict entry is not a subject" branch, which nib's reader cannot reach.
		{"a dangling reference in /Annots", annotFixture{annots: "[99 0 R]"}, NotApplicable, NotApplicable},
		{"an empty /Annots", annotFixture{annots: "[]"}, NotApplicable, NotApplicable},
		{"no /Annots key", annotFixture{}, NotApplicable, NotApplicable},
		{"an indirect /Annots array", annotFixture{annots: "40 0 R",
			extra: map[int]string{40: "[41 0 R]", 41: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (a) >>"}},
			Fail, Pass},
		// Every annotation is its own check: one failing among passing ones fails the document.
		{"two annotations, the second failing both", annotFixture{elem: annotTag, annots: "[30 0 R 31 0 R]",
			extra: map[int]string{30: note(), 31: note("StructParent", "", "Contents", "")}}, Fail, Fail},
	} {
		pdf := tc.fx.build()
		if got := verdictOf(t, pdf, "7.18.1 t1"); got.Verdict != tc.t1 {
			t.Errorf("%s: 7.18.1 t1 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t1)
		}
		if got := verdictOf(t, pdf, "7.18.1 t2"); got.Verdict != tc.t2 {
			t.Errorf("%s: 7.18.1 t2 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t2)
		}
	}
}

// TestAPresentButUnusableRowIsDefiniteEvenWhenTheTreeIsShort — the distinction the shared door nearly
// lost, and which nothing else in the package pins.
//
// The rule this door absorbed asked `pt[sp]` for `found` BEFORE deciding, so a row that is present and
// is not a structure element was a **definite Fail**, while a row missing from a tree nib could not
// finish was a refusal. The door's first draft tested `elem == nil && why != ""` and answered
// CannotCheck for both — a shipped clause's verdict moved on a document with a deep tree AND a
// malformed row, with nothing declaring it. Measured: collapsing the two again leaves every other test
// in this package green.
//
// The fixture needs both halves at once: key 1's row is present and is an ARRAY, and a sibling branch
// nests past `maxWalkDepth` so the walk reports a reason. Definite beats refusal.
func TestAPresentButUnusableRowIsDefiniteEvenWhenTheTreeIsShort(t *testing.T) {
	deepBranch := map[int]string{}
	for i := 0; i <= maxWalkDepth+2; i++ {
		deepBranch[300+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [9 9] >>", 301+i)
	}
	deepBranch[300+maxWalkDepth+3] = "<< /Nums [9 11 0 R] /Limits [9 9] >>"
	deepBranch[11] = "<< /Type /StructElem /S /Annot /P 7 0 R >>"
	// The parent tree's root has two kids: a leaf whose row for key 1 is an array, and the deep chain.
	deepBranch[9] = "<< /Kids [290 0 R 300 0 R] >>" // the root of a number tree carries no /Limits
	deepBranch[290] = "<< /Nums [1 [10 0 R]] /Limits [1 1] >>"
	fx := annotFixture{annot: note(), elem: annotTag, extra: deepBranch}
	pdf := fx.build()
	got := verdictOf(t, pdf, "7.18.1 t1")
	if got.Verdict != Fail {
		t.Errorf("a present row that is not an element reports %v (%s), want Fail — the tree being short "+
			"elsewhere must not turn a definite answer into a refusal", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "names no element") {
		t.Errorf("the reason %q does not say the /StructParent names no element", got.Why)
	}
	// The stimulus: the SAME document with the deep branch removed answers Fail for the same reason, and
	// with the row made an element it passes — so the row above is the collapse and not a broken fixture.
	shallow := annotFixture{annot: note(), elem: annotTag,
		extra: map[int]string{9: "<< /Nums [1 [10 0 R]] >>"}}.build()
	if got := verdictOf(t, shallow, "7.18.1 t1"); got.Verdict != Fail {
		t.Errorf("control: with no deep branch, a present array row reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	if got := verdictOf(t, fx.build(), "7.18.1 t2"); got.Verdict != Pass {
		t.Errorf("control: t2 answers from the annotation's own /Contents and reports %v (%s), want Pass", got.Verdict, got.Why)
	}
}

// TestAHiddenOrOffPageWidgetIsNoLongerAFailure is P05.S01's regression over a SHIPPED clause.
//
// 7.18.4 t1 shipped in P03.S06 carrying neither half of its own exemption and refusing an inline
// widget for an `OBJR` the clause does not ask for. Measured on veraPDF 1.30.2, each row is a
// document it PASSES; each was a `Fail` in nib until the shared door landed. No corpus file holds
// one, which is why fourteen days of a 297-file corpus at 0 false fails never showed it.
func TestAHiddenOrOffPageWidgetIsNoLongerAFailure(t *testing.T) {
	widget := func(entries ...string) string {
		return note(append([]string{"Subtype", "/Widget", "FT", "/Btn", "T", "(b)", "TU", "(a button)"}, entries...)...)
	}
	for _, tc := range []struct {
		name string
		fx   annotFixture
		want Verdict
	}{
		{"a hidden widget outside any Form tag", annotFixture{annot: widget("StructParent", "", "F", "2")}, Pass},
		{"a widget wholly outside the crop box", annotFixture{annot: widget("StructParent", ""),
			page: "/CropBox [100 100 500 500]"}, Pass},
		{"a hidden widget under a P tag", annotFixture{annot: widget("F", "2"), elem: "/S /P"}, Pass},
		{"a widget written inline in /Annots, under a Form tag", annotFixture{elem: "/S /Form",
			annots: "[<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /F 4 /FT /Btn /T (b) /TU (a button) /StructParent 1 >>]"}, Pass},
		// The stimulus before the verdict: the same widget VISIBLE and outside a Form tag still fails,
		// so the rows above are the exemption working and not the rule going quiet.
		{"a visible widget outside any Form tag", annotFixture{annot: widget("StructParent", "")}, Fail},
		{"a visible widget under a P tag", annotFixture{annot: widget(), elem: "/S /P"}, Fail},
	} {
		if got := verdictOf(t, tc.fx.build(), "7.18.4 t1"); got.Verdict != tc.want {
			t.Errorf("%s: 7.18.4 t1 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.want)
		}
	}
}

// TestEveryAnnotationReaderRoutesThroughOneDoor is ADR-009's guard for the annotation population.
//
// It asserts the ROUTING and not the agreement: three readers agreeing today says nothing about a
// fourth added next slice, and this package had exactly that — `checkWidgetsInFormElements`,
// `scanAnnotsAndFields` and `walkAppearances` each dereferenced a page's `/Annots` for itself, and
// the first of them was missing the exemption the other two never needed.
func TestEveryAnnotationReaderRoutesThroughOneDoor(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sites := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		file, perr := parser.ParseFile(token.NewFileSet(), f, src, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		// **Every string literal, not only an index expression.** A reader written
		// `page.ArrayEntry("Annots")`, `page.Find("Annots")` or `page[annotsKey]` with a package-level
		// const would open a second door and pass an index-only scan — and those method forms are live
		// idiom in this package. The literal is the one thing every spelling must contain.
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING && lit.Value == `"Annots"` {
				sites[f]++
			}
			return true
		})
	}
	if got := sites["annots.go"]; got != 1 {
		t.Errorf("annots.go reads /Annots %d times, want exactly 1 — the door itself", got)
	}
	for f, n := range sites {
		if f != "annots.go" {
			t.Errorf("%s dereferences a page's /Annots %d time(s) of its own; the population is `annots()`'s "+
				"(ADR-009). Route it through the door, or name the exemption at the site and here", f, n)
		}
	}
}

// P05.S02 — the typed annotations. Every verdict below is veraPDF 1.30.2's, measured on that exact
// document before the rule was written (seventeen fixtures, the slice's grill table in the plan).

// typedFixture builds a one-page tagged document carrying one annotation of a given subtype, with an
// `/AP` because pdfcpu refuses a PrinterMark or a TrapNet without one (measured).
func typedFixture(subtype, elem string, entries ...string) annotFixture {
	const form = "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"
	a := note(append([]string{"Subtype", "/" + subtype, "AP", "<< /N 41 0 R >>"}, entries...)...)
	return annotFixture{annot: a, elem: elem, extra: map[int]string{41: form}}
}

func TestTheTypedAnnotationRulesAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fx         annotFixture
		clause     string
		want       Verdict
		alsoClause string
		alsoWant   Verdict
	}{
		// --- 7.18.5 t1: a Link is nested in a Link tag -----------------------------------------
		{"a link in a Link element", typedFixture("Link", "/S /Link"), "7.18.5 t1", Pass, "7.18.5 t2", Pass},
		{"a link in a P element", typedFixture("Link", "/S /P"), "7.18.5 t1", Fail, "7.18.5 t2", Pass},
		{"a link with no /StructParent", typedFixture("Link", "/S /Link", "StructParent", ""), "7.18.5 t1", Fail, "7.18.5 t2", Pass},
		{"a link in a type role-mapped to /Link", annotFixture{
			annot: note("Subtype", "/Link", "AP", "<< /N 41 0 R >>"), elem: "/S /MyLink",
			extra: map[int]string{41: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream",
				7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyLink /Link >> >>"}},
			"7.18.5 t1", Pass, "7.18.5 t2", Pass},
		{"a hidden link outside any Link tag", typedFixture("Link", "/S /P", "StructParent", "", "F", "2", "Contents", ""),
			"7.18.5 t1", Pass, "7.18.5 t2", Pass},
		{"a link off the crop box", annotFixture{
			annot: note("Subtype", "/Link", "AP", "<< /N 41 0 R >>", "StructParent", "", "Contents", ""),
			page:  "/CropBox [100 100 500 500]",
			extra: map[int]string{41: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"}},
			"7.18.5 t1", Pass, "7.18.5 t2", Pass},
		// --- 7.18.5 t2 reads the ANNOTATION's /Contents, and 7.18.1 t2 does not ----------------
		{"a link with no /Contents", typedFixture("Link", "/S /Link", "Contents", ""), "7.18.5 t2", Fail, "7.18.5 t1", Pass},
		{"a link with an empty /Contents", typedFixture("Link", "/S /Link", "Contents", "()"), "7.18.5 t2", Fail, "7.18.5 t1", Pass},
		// **The decisive pair**: the description is on the ELEMENT, so 7.18.1 t2 passes and 7.18.5 t2
		// fails — on one document. An implementation that shared a predicate between them cannot.
		{"a link described only by its element's /Alt", typedFixture("Link", "/S /Link /Alt (a description)", "Contents", ""),
			"7.18.5 t2", Fail, "7.18.1 t2", Pass},
		// --- 7.18.2 t1: the subtype is refused outright ----------------------------------------
		{"a visible TrapNet", typedFixture("TrapNet", "/S /P", "StructParent", ""), "7.18.2 t1", Fail, "7.18.1 t1", Fail},
		{"a hidden TrapNet", typedFixture("TrapNet", "/S /P", "StructParent", "", "F", "2"), "7.18.2 t1", Pass, "7.18.1 t1", Pass},
		{"a TrapNet off the crop box", annotFixture{
			annot: note("Subtype", "/TrapNet", "AP", "<< /N 41 0 R >>", "StructParent", ""),
			page:  "/CropBox [100 100 500 500]",
			extra: map[int]string{41: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"}},
			"7.18.2 t1", Pass, "7.18.1 t1", Pass},
		// --- 7.18.8 t1: a printer's mark is in NO element, and reads the RAW /S ----------------
		{"an untagged printer's mark", typedFixture("PrinterMark", "/S /P", "StructParent", ""), "7.18.8 t1", Pass, "7.18.1 t1", Pass},
		// The description says a PrinterMark "shall be considered Incidental Artifacts" — and veraPDF
		// FAILS one tagged /Artifact. The clause is "described by no element at all".
		{"a printer's mark tagged /Artifact", typedFixture("PrinterMark", "/S /Artifact"), "7.18.8 t1", Fail, "7.18.1 t1", Pass},
		{"a printer's mark in a P element", typedFixture("PrinterMark", "/S /P"), "7.18.8 t1", Fail, "7.18.1 t1", Pass},
		// A /StructParent whose row is not an element reads as null, so it PASSES — a definite answer,
		// not a refusal, and the row that proves the clause is `structParentType == null` and not
		// "carries no /StructParent".
		{"a printer's mark whose /StructParent row is an array", annotFixture{
			annot: note("Subtype", "/PrinterMark", "AP", "<< /N 41 0 R >>"), elem: "/S /P",
			extra: map[int]string{41: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream",
				9: "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>"}},
			"7.18.8 t1", Pass, "7.18.1 t1", Pass},
		{"a hidden printer's mark in a P element", typedFixture("PrinterMark", "/S /P", "F", "2"), "7.18.8 t1", Pass, "7.18.1 t1", Pass},
		// **A present but EMPTY `/S` is a type.** veraPDF fails this document and nib passed it until the
		// review: `d.name` answers "" for an absent key and for `/S /` alike, and the clause tests presence.
		{"a printer's mark in an element whose /S is an empty name", typedFixture("PrinterMark", "/S /"),
			"7.18.8 t1", Fail, "7.18.1 t1", Pass},
		// --- the branches a table of passing documents does not reach ---------------------------
		{"a link whose /StructParent names no row", typedFixture("Link", "/S /Link", "StructParent", "7"),
			"7.18.5 t1", Fail, "7.18.5 t2", Pass},
		{"a link in a type on a role-map loop", annotFixture{
			annot: note("Subtype", "/Link", "AP", "<< /N 41 0 R >>"), elem: "/S /MyLink",
			extra: map[int]string{41: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream",
				7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyLink /Other /Other /MyLink >> >>"}},
			"7.18.5 t1", CannotCheck, "7.18.5 t2", Pass},
		// --- the NotApplicable arm of each clause -----------------------------------------------
		// The oracle catches a NotApplicable silently turned into a Pass ("veraPDF no subject, nib pass"),
		// but it SKIPS when veraPDF is absent — which is the fresh-clone tier-1 case this repo supports.
		{"a document whose only annotation is a note: no link", typedFixture("Text", annotTag),
			"7.18.5 t1", NotApplicable, "7.18.5 t2", NotApplicable},
		{"a document whose only annotation is a note: no TrapNet and no mark", typedFixture("Text", annotTag),
			"7.18.2 t1", NotApplicable, "7.18.8 t1", NotApplicable},
	} {
		pdf := tc.fx.build()
		if got := verdictOf(t, pdf, tc.clause); got.Verdict != tc.want {
			t.Errorf("%s: %s = %v (%s), want %v", tc.name, tc.clause, got.Verdict, got.Why, tc.want)
		}
		if got := verdictOf(t, pdf, tc.alsoClause); got.Verdict != tc.alsoWant {
			t.Errorf("%s: %s = %v (%s), want %v", tc.name, tc.alsoClause, got.Verdict, got.Why, tc.alsoWant)
		}
	}
}

// TestAPrinterMarksTagIsReadRawAndNotThroughTheRoleMap — 7.18.8 t1 is the one clause in this family that
// does NOT go through `standardType`, and nothing else would notice.
//
// `GFPDAnnot.getstructParentType` takes the element's `/S` by name and stops there. So a printer's mark in
// an element whose private type is role-mapped to something harmless is still IN the tree and still fails,
// where a standard-type reading would resolve the map and could let it through.
func TestAPrinterMarksTagIsReadRawAndNotThroughTheRoleMap(t *testing.T) {
	const form = "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"
	// /MyMark maps to /Artifact; the raw /S is /MyMark, which is a name either way, so the mark is in the
	// tree and the clause fails. A reading that resolved the map would also fail here — so the row that
	// discriminates is the one below it, where the map dead-ends and `standardType` gives up.
	mapped := annotFixture{annot: note("Subtype", "/PrinterMark", "AP", "<< /N 41 0 R >>"), elem: "/S /MyMark",
		extra: map[int]string{41: form,
			7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyMark /Artifact >> >>"}}
	if got := verdictOf(t, mapped.build(), "7.18.8 t1"); got.Verdict != Fail {
		t.Errorf("a printer's mark in a type mapped to /Artifact reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	// A private type on a role-map LOOP: `standardType` cannot resolve it and answers CannotCheck for
	// every clause that asks. This clause never asks, so it stays a definite Fail — the raw /S is there.
	looped := annotFixture{annot: note("Subtype", "/PrinterMark", "AP", "<< /N 41 0 R >>"), elem: "/S /MyMark",
		extra: map[int]string{41: form,
			7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyMark /Other /Other /MyMark >> >>"}}
	if got := verdictOf(t, looped.build(), "7.18.8 t1"); got.Verdict != Fail {
		t.Errorf("a printer's mark on a role-map loop reports %v (%s), want Fail — this clause reads the raw /S "+
			"and never consults the role map, so a loop cannot make it refuse", got.Verdict, got.Why)
	}
	// **The control has to change the SUBTYPE, not the clause.** 7.18.1 t1 excludes a PrinterMark by subtype
	// and never reaches `standardType`, so asking it about this document proves nothing about the role map —
	// the row would stay green with `standardType` deleted from that clause. A `/Text` annotation in the SAME
	// looped element is the honest control: it does resolve a type, and it refuses.
	loopedNote := annotFixture{annot: note(), elem: "/S /MyMark",
		extra: map[int]string{7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << /MyMark /Other /Other /MyMark >> >>"}}
	if got := verdictOf(t, loopedNote.build(), "7.18.1 t1"); got.Verdict != CannotCheck {
		t.Errorf("control: a /Text annotation in the same looped element reports %v (%s) for 7.18.1 t1, want "+
			"CannotCheck — that clause DOES consult the role map, which is what makes 7.18.8 t1's definite "+
			"Fail over the same element a statement about reading the raw /S", got.Verdict, got.Why)
	}
}

// TestAnUnreadableAnnotsIsARefusalForEveryTypedClause — the door reports a short population once, and each
// typed clause must turn it into a refusal rather than into "the document has no X".
//
// Measured: deleting the `missed` branch from all four left the whole package green. The clause that wrote
// `TestAnAnnotsThatDoesNotResolveIsNotAPageWithoutWidgets` cannot cover these — its fixture's only annotation
// is a Widget, which every typed clause here excludes, so its control would be NotApplicable rather than Pass.
func TestAnUnreadableAnnotsIsARefusalForEveryTypedClause(t *testing.T) {
	const form = "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"
	// A link in a Link element (passes both link clauses), a hidden TrapNet (passes 7.18.2 t1) and an
	// untagged printer's mark (passes 7.18.8 t1) — so every clause has a subject and a definite Pass.
	// **The TrapNet is LAST, because pdfcpu refuses any other order** — "invalid page annotation list,
	// \"TrapNet\" has to be the last entry", measured here rather than read: the first spelling of this
	// fixture put it in the middle and the document would not open at all.
	fx := annotFixture{elem: "/S /Link", annots: "[30 0 R 32 0 R 31 0 R]",
		extra: map[int]string{
			30: note("Subtype", "/Link"),
			31: note("Subtype", "/TrapNet", "StructParent", "", "F", "2", "Contents", ""),
			32: note("Subtype", "/PrinterMark", "StructParent", "", "AP", "<< /N 41 0 R >>"),
			41: form,
		}}
	clauses := []string{"7.18.5 t1", "7.18.5 t2", "7.18.2 t1", "7.18.8 t1"}
	for _, clause := range clauses {
		check := registry[clause].Check
		if got := check(openMutated(t, fx.build(), func(*Document, types.Dict) {})); got.Verdict != Pass {
			t.Fatalf("control: unmutated, %s reports %v (%s), want Pass — the fixture does not carry its subject",
				clause, got.Verdict, got.Why)
		}
		d := openMutated(t, fx.build(), func(_ *Document, page types.Dict) {
			page["Annots"] = types.Name("NotAnArray")
		})
		got := check(d)
		if got.Verdict != CannotCheck {
			t.Errorf("with an /Annots that does not resolve to an array, %s reports %v (%s), want CannotCheck",
				clause, got.Verdict, got.Why)
			continue
		}
		if !strings.Contains(got.Why, "/Annots could not be read") {
			t.Errorf("%s: the reason %q does not say the /Annots could not be read", clause, got.Why)
		}
	}
}

// P05.S03 — a widget's alternative description, and the page's tab order. Thirteen fixtures, every verdict
// veraPDF 1.30.2's, measured before a rule was written.

// formFixture builds a one-page tagged document with one widget annotation in a Form element. pdfcpu requires
// `/DA` on a form field dictionary (measured: without it the document does not open at all), so every widget
// or field here carries one.
func formFixture(widget, elem, acro string, extra map[int]string, page, tabs string) annotFixture {
	fx := annotFixture{annot: widget, elem: elem, cat: acro, page: page, extra: extra}
	switch tabs {
	case "S":
	case "":
		fx.noTabs = true
	default:
		// The parameter only ever distinguishes present-as-/S from absent, so any other value would be
		// silently written as `/Tabs /S`. A row wanting `/Tabs /R` passes it through `page`, which is what the
		// one such row does — this refuses the misuse instead of honouring it wrongly.
		panic("formFixture: tabs must be \"S\" or \"\"; write any other /Tabs value through the page argument")
	}
	return fx
}

// widget is a `/Widget` annotation with `/DA`, overridable per row.
func widget(entries ...string) string {
	return note(append([]string{"Subtype", "/Widget", "FT", "/Tx", "DA", "(/Helv 0 Tf 0 g)",
		"Contents", "", "T", "(f)"}, entries...)...)
}

const acroSelf = "/AcroForm << /Fields [30 0 R] >>"
const acroParent = "/AcroForm << /Fields [31 0 R] >>"

func TestTheInteractiveSurfaceRulesAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	const da = "/DA (/Helv 0 Tf 0 g) "
	for _, tc := range []struct {
		name   string
		fx     annotFixture
		t3, tb Verdict
	}{
		// --- 7.18.1 t3: the field's /TU, else the enclosing element's /Alt --------------------
		{"a merged widget with its own /TU", formFixture(widget("TU", "(Your name)"), "/S /Form", acroSelf, nil, "", "S"), Pass, Pass},
		{"a merged widget described by its element", formFixture(widget(), "/S /Form /Alt (Your name)", acroSelf, nil, "", "S"), Pass, Pass},
		{"a merged widget with neither", formFixture(widget(), "/S /Form", acroSelf, nil, "", "S"), Fail, Pass},
		{"an empty /TU", formFixture(widget("TU", "()"), "/S /Form", acroSelf, nil, "", "S"), Fail, Pass},
		{"an empty /Alt on the element", formFixture(widget(), "/S /Form /Alt ()", acroSelf, nil, "", "S"), Fail, Pass},
		// A widget with no `/T` is not the field, so its PARENT's /TU is read — one level.
		{"a kid widget whose parent field carries /TU", formFixture(
			widget("T", "", "FT", "", "DA", "", "Parent", "31 0 R"), "/S /Form", acroParent,
			map[int]string{31: "<< /FT /Tx /T (f) /TU (Your name) " + da + "/Kids [30 0 R] >>"}, "", "S"), Pass, Pass},
		// **No climb**: a /TU on the GRANDPARENT field is not found.
		{"a kid widget whose grandparent carries /TU", formFixture(
			widget("T", "", "FT", "", "DA", "", "Parent", "31 0 R"), "/S /Form", "/AcroForm << /Fields [32 0 R] >>",
			map[int]string{31: "<< /T (sub) /Parent 32 0 R /Kids [30 0 R] >>",
				32: "<< /FT /Tx /T (top) /TU (Your name) " + da + "/Kids [31 0 R] >>"}, "", "S"), Fail, Pass},
		// **The sharpest row**: the widget's OWN /TU is ignored because it carries no /T, so it is not the
		// field. Reading the annotation's /TU unconditionally passes a document veraPDF fails.
		{"a kid widget carrying /TU itself", formFixture(
			widget("T", "", "FT", "", "DA", "", "Parent", "31 0 R", "TU", "(on the widget)"), "/S /Form", acroParent,
			map[int]string{31: "<< /FT /Tx /T (f) " + da + "/Kids [30 0 R] >>"}, "", "S"), Fail, Pass},
		{"a hidden widget with nothing describing it", formFixture(widget("F", "2"), "/S /Form", acroSelf, nil, "", "S"), Pass, Pass},
		{"a widget off the crop box", formFixture(widget(), "/S /Form", acroSelf, nil, "/CropBox [100 100 500 500]", "S"), Pass, Pass},
		// --- 7.18.3 t1: a page carrying annotations declares /Tabs /S -------------------------
		{"a page with an annotation and no /Tabs", formFixture(widget("TU", "(n)"), "/S /Form", acroSelf, nil, "", ""), Pass, Fail},
		{"a page declaring /Tabs /R", formFixture(widget("TU", "(n)"), "/S /Form", acroSelf, nil, "/Tabs /R", ""), Pass, Fail},
		// `/Tabs` may be indirect, and veraPDF dereferences it.
		{"an indirect /Tabs naming /S", formFixture(widget("TU", "(n)"), "/S /Form", acroSelf,
			map[int]string{41: "/S"}, "/Tabs 41 0 R", ""), Pass, Pass},
	} {
		pdf := tc.fx.build()
		if got := verdictOf(t, pdf, "7.18.1 t3"); got.Verdict != tc.t3 {
			t.Errorf("%s: 7.18.1 t3 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t3)
		}
		if got := verdictOf(t, pdf, "7.18.3 t1"); got.Verdict != tc.tb {
			t.Errorf("%s: 7.18.3 t1 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.tb)
		}
	}
}

// TestTabOrderCountsEveryAnnotationAndRefusesWhatItCannotRead — 7.18.3 t1 carries NO exemption, which is what
// separates it from every other clause over the door.
//
// A hidden annotation and one wholly off the crop box both still oblige the page to declare its tab order
// (measured, both). And a page whose `/Annots` nib cannot read is not a page without annotations.
func TestTabOrderCountsEveryAnnotationAndRefusesWhatItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		fx   annotFixture
		want Verdict
	}{
		{"a hidden annotation still counts", annotFixture{annot: note("F", "2"), elem: annotTag, noTabs: true}, Fail},
		{"an annotation off the crop box still counts", annotFixture{annot: note(), elem: annotTag,
			page: "/CropBox [100 100 500 500]", noTabs: true}, Fail},
		{"a page with no annotations at all", annotFixture{noTabs: true}, Pass},
		{"an empty /Annots", annotFixture{annots: "[]", noTabs: true}, Pass},
	} {
		if got := verdictOf(t, tc.fx.build(), "7.18.3 t1"); got.Verdict != tc.want {
			t.Errorf("%s: 7.18.3 t1 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.want)
		}
	}
	// A page whose /Annots does not resolve: the clause refuses rather than reporting that the page's tab
	// order does not matter.
	d := openMutated(t, annotFixture{annot: note(), elem: annotTag, noTabs: true}.build(),
		func(_ *Document, page types.Dict) { page["Annots"] = types.Name("NotAnArray") })
	got := registry["7.18.3 t1"].Check(d)
	if got.Verdict != CannotCheck {
		t.Errorf("with an /Annots that does not resolve, 7.18.3 t1 = %v (%s), want CannotCheck", got.Verdict, got.Why)
	}
}

// TestWhetherAWidgetIsTheFieldIsAPresenceTest — P05.S03's false pass, and the shape that caught it.
//
// veraPDF's `isField` is `knownKey(ASAtom.T)`: the KEY, whatever it holds. The first draft read `/T` as a
// string, and a `/T` naming a FREE object then flipped the branch — pdfcpu accepts such a document, veraPDF
// FAILS it (the key is present, so the widget is the field and its own absent `/TU` decides the clause), and
// nib PASSED it by reading the parent field's `/TU` instead.
//
// **Seven shapes, each measured against veraPDF 1.30.2.** Two of them nib cannot reach — pdfcpu refuses a
// non-string `/T` outright — and they are named here rather than dropped, because "unreachable" is a fact about
// this reader and not about the rule.
func TestWhetherAWidgetIsTheFieldIsAPresenceTest(t *testing.T) {
	const da = "/DA (/Helv 0 Tf 0 g) "
	// The parent field carries the /TU. So the verdict says which dictionary was read: Pass means the
	// PARENT's /TU was used (the widget is not the field), Fail means the widget's own absent /TU was.
	parent := map[int]string{31: "<< /FT /Tx /T (f) /TU (Your name) " + da + "/Kids [30 0 R] >>"}
	// **The widget is OBJECT 30 and not an inline dictionary**, so the field's `/Kids [30 0 R]` resolves and
	// pdfcpu validates it as a form field. The first draft of this test inlined it, `/Kids` dangled, and
	// pdfcpu never looked — which made two rows below claim a reachability the CLI contradicted.
	fx := func(tKey string, extra map[int]string) annotFixture {
		objs := map[int]string{}
		for k, v := range parent {
			objs[k] = v
		}
		for k, v := range extra {
			objs[k] = v
		}
		return annotFixture{elem: "/S /Form", cat: acroParent, extra: objs,
			annot: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /F 4 /StructParent 1 " +
				tKey + " /Parent 31 0 R >>"}
	}
	for _, tc := range []struct {
		name string
		fx   annotFixture
		want Verdict
	}{
		// The key is absent: the widget is not the field, so the parent's /TU describes it.
		{"no /T at all", fx("", nil), Pass},
		// The key is PRESENT in every row below, so the widget is the field and its own /TU is read — and it
		// has none, so every one fails. This is the whole finding.
		{"a /T naming a free object", fx("/T 99 0 R", nil), Fail},
		{"an empty /T", fx("/T ()", nil), Fail},
		{"an indirect /T that resolves", fx("/T 41 0 R", map[int]string{41: "(f)"}), Fail},
		{"a direct string /T", fx("/T (f)", nil), Fail},
	} {
		if got := verdictOf(t, tc.fx.build(), "7.18.1 t3"); got.Verdict != tc.want {
			t.Errorf("%s: 7.18.1 t3 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.want)
		}
	}
	// `/T null` is a sixth shape and it is the ABSENT case, not a present one: pdfcpu drops a null-valued key
	// before any rule sees it, so both readers find no `/T` and the parent's /TU applies. Measured on both.
	if got := verdictOf(t, fx("/T null", nil).build(), "7.18.1 t3"); got.Verdict != Pass {
		t.Errorf("a /T null reports %v (%s), want Pass — pdfcpu drops the key, so the widget is not the field",
			got.Verdict, got.Why)
	}
	// The seventh and eighth shapes — `/T` as a NAME and as a NUMBER — are where a presence test and a string
	// read differ most sharply, and nib cannot reach either: pdfcpu refuses the document. Asserted as a
	// reading limit so that a pdfcpu bump which starts accepting them is caught here rather than in the field.
	for _, bad := range []string{"/T /aname", "/T 42"} {
		if _, err := open(fx(bad, nil).build()); err == nil {
			t.Errorf("pdfcpu now accepts %q on a widget; the presence test above is what keeps nib in step with "+
				"veraPDF on it, and this row should become a verdict assertion", bad)
		}
	}
}
