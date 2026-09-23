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

// TestEveryAnnotationReaderRoutesThroughOneDoor is ADR-009's guard for the annotation population
// AND for the `/StructParent` hop, both of which `annots.go` owns.
//
// It asserts the ROUTING and not the agreement: three readers agreeing today says nothing about a
// fourth added next slice, and this package had exactly that — `checkWidgetsInFormElements`,
// `scanAnnotsAndFields` and `walkAppearances` each dereferenced a page's `/Annots` for itself, and
// the first of them was missing the exemption the other two never needed.
//
// **`/StructParent` was added at P05's phase close, and it is the case that proves the point.** The
// two resolvers of that key agreed on every document anyone had built and disagreed on one nobody
// had: a parent tree that is BOTH truncated and holds a present-but-unusable row. A guard over
// agreement would have been green throughout; this one counts sites. `"StructParents"` — the PAGE's
// key, a different question — is a different literal and is not matched.
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
			if ok && lit.Kind == token.STRING && (lit.Value == `"Annots"` || lit.Value == `"StructParent"`) {
				sites[f]++
			}
			return true
		})
	}
	// One for `annots()`'s /Annots, one for `elementForStructParent`'s /StructParent.
	if got := sites["annots.go"]; got != 2 {
		t.Errorf("annots.go names /Annots and /StructParent %d times between them, want exactly 2 — the two "+
			"doors themselves, one read each", got)
	}
	for f, n := range sites {
		if f != "annots.go" {
			t.Errorf("%s reads a page's /Annots or a holder's /StructParent %d time(s) of its own; those "+
				"populations are `annots()`'s and `elementForStructParent`'s (ADR-009). Route it through the "+
				"door, or name the exemption at the site and here", f, n)
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

// P05.S04 — media clips. Every verdict below is veraPDF 1.30.2's, measured on that document before the rule was
// written: eleven `/Alt` and `/CT` shapes, seven holder paths, and the corpus's own five files.

// clipFixture is a one-page tagged document whose Screen annotation's `/A` is a Rendition action carrying one
// media clip. `alt` and `ct` are the clip's entries verbatim, so a row can write a malformed shape.
func clipFixture(alt, ct string) []byte {
	clip := "<< /Type /MediaClip /S /MCD /D 45 0 R " + ct + alt + " >>"
	return buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-US) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S /Annots [30 0 R] " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(annotFixtureText), annotFixtureText),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		9:  "<< /Nums [0 [8 0 R]] >>",
		30: "<< /Type /Annot /Subtype /Screen /Rect [0 0 10 10] /F 4 /Contents (a clip) /A 44 0 R >>",
		44: "<< /Type /Action /S /Rendition /R << /Type /Rendition /S /MR /C " + clip + " >> >>",
		45: "<< /Type /Filespec /F (clip.mp3) /UF (clip.mp3) >>",
	})
}

func TestTheMediaClipRulesAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	const ct = "/CT (audio/mpeg) "
	for _, tc := range []struct {
		name   string
		pdf    []byte
		t1, t2 Verdict
	}{
		{"one language/description pair", clipFixture("/Alt [() (a clip)]", ct), Pass, Pass},
		{"two pairs", clipFixture("/Alt [() (a clip) (en) (a clip)]", ct), Pass, Pass},
		// **The shape, not the presence.** An odd length, and an empty DESCRIPTION, both fail.
		{"an odd number of entries", clipFixture("/Alt [() (a clip) (en)]", ct), Pass, Fail},
		{"an empty description", clipFixture("/Alt [() ()]", ct), Pass, Fail},
		// An empty LANGUAGE is the default entry ISO 32000-1 Table 274 describes, and it passes — which is why
		// the rule tests only the odd indices.
		{"empty languages with real descriptions", clipFixture("/Alt [() (a clip) () (another)]", ct), Pass, Pass},
		{"no /Alt at all", clipFixture("", ct), Pass, Fail},
		// An EMPTY array passes: even length, and every entry it has is a string. Measured, not reasoned — it
		// is the row that says `hasCorrectAlt` is a shape test and not "there is a description".
		{"an empty /Alt array", clipFixture("/Alt []", ct), Pass, Pass},
		{"no /CT", clipFixture("/Alt [() (a clip)]", ""), Fail, Pass},
	} {
		if got := verdictOf(t, tc.pdf, "7.18.6.2 t1"); got.Verdict != tc.t1 {
			t.Errorf("%s: 7.18.6.2 t1 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t1)
		}
		if got := verdictOf(t, tc.pdf, "7.18.6.2 t2"); got.Verdict != tc.t2 {
			t.Errorf("%s: 7.18.6.2 t2 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t2)
		}
	}
	// Three shapes veraPDF grades and nib's reader refuses, each with the refusal pdfcpu actually prints —
	// recorded so that a bump which starts accepting them turns this row red rather than reintroducing a
	// divergence silently. **Read, not assumed**: the three errors name three different entries.
	for _, tc := range []struct {
		name, want string
		pdf        []byte
	}{
		{"a non-string /Alt entry", "validateStringArrayEntry: invalid type at index 1", clipFixture("/Alt [() /aname]", ct)},
		{"an /Alt that is not an array", "entry=Alt invalid type", clipFixture("/Alt (a clip)", ct)},
		{"a name-typed /CT", "entry=CT invalid type", clipFixture("/Alt [() (a clip)]", "/CT /audio ")},
	} {
		_, err := open(tc.pdf)
		if err == nil {
			t.Errorf("%s: pdfcpu now accepts this document; veraPDF grades it, so the clause needs a verdict row "+
				"here rather than a reading limit", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: pdfcpu refuses it with %q, which does not name %q — the refusal this row records is a "+
				"different one from the refusal it now gets", tc.name, err.Error(), tc.want)
		}
	}
}

// TestAMediaClipIsReachedThroughSixHolders — the population, and the seventh holder that is NOT one.
//
// A clip sits at `<action>/R/C` for a Rendition action, and veraPDF's model reaches an action from an
// annotation's `/A` and `/AA`, an outline item's `/A`, the catalog's `/OpenAction` and `/AA`, and a page's
// `/AA`. **A form field's `/AA` is not a path**: `GFPDFormField` links one, and veraPDF evaluates NO check on a
// document whose only clip hangs off a non-widget parent field's `/AA` — measured, where nib first failed it.
func TestAMediaClipIsReachedThroughSixHolders(t *testing.T) {
	const act = "<< /Type /Action /S /Rendition /R << /Type /Rendition /S /MR /C " +
		"<< /Type /MediaClip /S /MCD /D 45 0 R /Alt [() (a clip)] >> >> >>"
	// The clip carries no /CT, so 7.18.6.2 t1 FAILS wherever the path is walked — "reached" is visible as a
	// failure, and a path silently dropped shows up as a Pass.
	doc := func(pageExtra, cat, annot string, extra map[int]string) []byte {
		objs := map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-US) " + cat + " >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(annotFixtureText), annotFixtureText),
			5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
			7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
			9:  "<< /Nums [0 [8 0 R]] >>",
			44: act,
			45: "<< /Type /Filespec /F (clip.mp3) /UF (clip.mp3) >>",
		}
		annots := ""
		if annot != "" {
			objs[30], annots = annot, "/Annots [30 0 R] "
		}
		objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S " + annots +
			pageExtra + " /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
		for k, v := range extra {
			objs[k] = v
		}
		return buildPDF(objs)
	}
	screen := func(action string) string {
		return "<< /Type /Annot /Subtype /Screen /Rect [0 0 10 10] /F 4 /Contents (a clip) " + action + " >>"
	}
	for _, tc := range []struct {
		name string
		pdf  []byte
	}{
		{"an annotation's /A", doc("", "", screen("/A 44 0 R"), nil)},
		{"an annotation's /AA", doc("", "", screen("/AA << /PV 44 0 R >>"), nil)},
		{"a page's /AA", doc("/AA << /O 44 0 R >>", "", "", nil)},
		// Two more trigger names, because the rows above used only one per holder and the walk now iterates a
		// FIXED LIST per holder rather than every `/AA` entry — `/C` for a page, `/Fo` for an annotation.
		{"a page's /AA under /C", doc("/AA << /C 44 0 R >>", "", "", nil)},
		{"an annotation's /AA under /Fo", doc("", "", screen("/AA << /Fo 44 0 R >>"), nil)},
		{"the catalog's /OpenAction", doc("", "/OpenAction 44 0 R", "", nil)},
		{"the catalog's /AA", doc("", "/AA << /WC 44 0 R >>", "", nil)},
		{"an outline item's /A", doc("", "/Outlines 20 0 R", "", map[int]string{
			20: "<< /Type /Outlines /First 21 0 R /Last 21 0 R /Count 1 >>",
			21: "<< /Title (One) /Parent 20 0 R /A 44 0 R >>"})},
	} {
		if got := verdictOf(t, tc.pdf, "7.18.6.2 t1"); got.Verdict != Fail {
			t.Errorf("a clip reached through %s reports %v (%s), want Fail — the path is not being walked",
				tc.name, got.Verdict, got.Why)
		}
	}
	// **Only a Rendition action carries a clip.** The same `/R` → `/C` under a `/GoTo` or a `/Movie` action is
	// graded by NEITHER reader — measured, both — and without these rows dropping the subtype check leaves the
	// package green, because every other fixture's action is a Rendition.
	for _, subtype := range []string{"/GoTo", "/Movie"} {
		pdf := doc("", "", screen("/A 44 0 R"), map[int]string{
			44: "<< /Type /Action /S " + subtype + " /R << /Type /Rendition /S /MR /C " +
				"<< /Type /MediaClip /S /MCD /D 45 0 R /Alt [() (a clip)] >> >> >>"})
		if got := verdictOf(t, pdf, "7.18.6.2 t1"); got.Verdict != NotApplicable {
			t.Errorf("a clip-shaped /R /C under a %s action reports %v (%s), want NotApplicable — only a Rendition "+
				"action carries a media clip, and the dictionary here is not one", subtype, got.Verdict, got.Why)
		}
	}
	// **A trigger name outside the holder's own list is refused by pdfcpu**, which enforces the same lists
	// veraPDF's `getActionNames()` hard-codes — so the over-broad walk this replaced (every `/AA` entry) was
	// unreachable through nib's reader. Recorded with the refusal, because the narrowing is what makes the
	// population veraPDF's rather than pdfcpu's.
	for _, tc := range []struct {
		name, want string
		pdf        []byte
	}{
		{"a page's /AA under an annotation trigger", "action PV not allowed for source page",
			doc("/AA << /PV 44 0 R >>", "", "", nil)},
		{"an annotation's /AA under a page trigger", "action O not allowed for source fieldOrAnnot",
			doc("", "", screen("/AA << /O 44 0 R >>"), nil)},
		{"the catalog's /AA under a page trigger", "action O not allowed for source root",
			doc("", "/AA << /O 44 0 R >>", "", nil)},
	} {
		_, err := open(tc.pdf)
		if err == nil {
			t.Errorf("%s: pdfcpu now accepts it, so the trigger list is the only thing keeping nib in step with "+
				"veraPDF here and this row should assert a NotApplicable verdict instead", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: pdfcpu refuses it with %q, which does not name %q", tc.name, err.Error(), tc.want)
		}
	}
	// The seventh holder, which veraPDF does not traverse: nib must NOT see this clip.
	field := doc("", "/AcroForm << /Fields [31 0 R] >>",
		"<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /F 4 /FT /Btn /T (b) /TU (b) /DA (/Helv 0 Tf 0 g) /Parent 31 0 R >>",
		map[int]string{31: "<< /FT /Btn /T (f) /TU (f) /DA (/Helv 0 Tf 0 g) /Kids [30 0 R] /AA << /U 44 0 R >> >>"})
	if got := verdictOf(t, field, "7.18.6.2 t1"); got.Verdict != NotApplicable {
		t.Errorf("a clip reached only through a non-widget parent field's /AA reports %v (%s), want NotApplicable "+
			"— veraPDF evaluates no check on that document and nib failed it before this was measured",
			got.Verdict, got.Why)
	}
}

// TestAMediaClipIsReachedThroughEveryPathVeraPDFWalks — the four the review found, each a measured divergence.
//
// Two false PASSES, one false FAIL, and one silent truncation, all in the population rather than in either
// rule. The clip in every row below carries no `/CT`, so "reached" shows up as a Fail and a path silently
// dropped shows up as a Pass.
func TestAMediaClipIsReachedThroughEveryPathVeraPDFWalks(t *testing.T) {
	const clip = "<< /Type /MediaClip /S /MCD /D 45 0 R /Alt [() (a clip)] >>"
	const rend = "<< /Type /Action /S /Rendition /R << /Type /Rendition /S /MR /C " + clip + " >> >>"
	base := func(extra map[int]string, annot, cat, pageExtra string) []byte {
		objs := map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-US) " + cat + " >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(annotFixtureText), annotFixtureText),
			5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
			7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
			9:  "<< /Nums [0 [8 0 R]] >>",
			30: annot,
			45: "<< /Type /Filespec /F (clip.mp3) /UF (clip.mp3) >>",
		}
		objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S /Annots [30 0 R] " +
			pageExtra + " /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
		for k, v := range extra {
			objs[k] = v
		}
		return buildPDF(objs)
	}
	const screen = "<< /Type /Annot /Subtype /Screen /Rect [0 0 10 10] /F 4 /Contents (a clip) /A 44 0 R >>"
	const widget = "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /F 4 /FT /Btn /T (b) /TU (b) " +
		"/DA (/Helv 0 Tf 0 g) /Parent 31 0 R >>"
	for _, tc := range []struct {
		name string
		pdf  []byte
	}{
		// **A form field's `/AA` IS a path** — under ITS OWN triggers `{K, F, V, C}`. The measurement that
		// dropped this path used `/U`, which is in the annotation list and not the field's, so veraPDF
		// evaluated nothing and the null result was misread as "fields are not traversed".
		{"a form field's /AA under /K", base(map[int]string{
			31: "<< /FT /Btn /T (f) /TU (f) /DA (/Helv 0 Tf 0 g) /Kids [30 0 R] /AA << /K 46 0 R >> >>",
			46: rend}, widget, "/AcroForm << /Fields [31 0 R] >>", "")},
		// **An action's `/Next` chain is walked**, as a dictionary and as an array — `PDAction.getNext` reads
		// both, and `GFPDAction` links it.
		{"a /Next chain, as a dictionary", base(map[int]string{
			44: "<< /Type /Action /S /GoTo /D [3 0 R /Fit] /Next 46 0 R >>", 46: rend}, screen, "", "")},
		{"a /Next chain, as an array", base(map[int]string{
			44: "<< /Type /Action /S /GoTo /D [3 0 R /Fit] /Next [46 0 R] >>", 46: rend}, screen, "", "")},
		// The outline's two recursion edges, which no fixture walked: a SIBLING and a CHILD.
		{"an outline item's sibling", base(map[int]string{
			20: "<< /Type /Outlines /First 21 0 R /Last 22 0 R /Count 2 >>",
			21: "<< /Title (One) /Parent 20 0 R /Next 22 0 R >>",
			22: "<< /Title (Two) /Parent 20 0 R /Prev 21 0 R /A 46 0 R >>",
			46: rend}, screen[:len(screen)-len("/A 44 0 R >>")]+">>", "/Outlines 20 0 R", "")},
		{"an outline item's child", base(map[int]string{
			20: "<< /Type /Outlines /First 21 0 R /Last 21 0 R /Count 2 >>",
			21: "<< /Title (One) /Parent 20 0 R /First 22 0 R /Last 22 0 R /Count 1 >>",
			22: "<< /Title (Child) /Parent 21 0 R /A 46 0 R >>",
			46: rend}, screen[:len(screen)-len("/A 44 0 R >>")]+">>", "/Outlines 20 0 R", "")},
	} {
		if got := verdictOf(t, tc.pdf, "7.18.6.2 t1"); got.Verdict != Fail {
			t.Errorf("a clip reached through %s reports %v (%s), want Fail — the path is not being walked",
				tc.name, got.Verdict, got.Why)
		}
	}
	// **A false FAIL: the catalog has no `/A`.** `GFPDDocument` links `/OpenAction`, its destination form and
	// `/AA`, and nothing else; reading an `/A` there graded a clip veraPDF never sees.
	noA := base(map[int]string{46: rend}, screen[:len(screen)-len("/A 44 0 R >>")]+">>", "/A 46 0 R", "")
	if got := verdictOf(t, noA, "7.18.6.2 t1"); got.Verdict != NotApplicable {
		t.Errorf("a Rendition action under a catalog /A reports %v (%s), want NotApplicable — the catalog has no "+
			"/A key in veraPDF's model", got.Verdict, got.Why)
	}
	// **A truncated outline is a REFUSAL, not a Pass.** veraPDF's walk is unbounded; nib's stops, and an
	// ordinary table of contents is long enough to reach the bound.
	long := map[int]string{46: rend}
	first := 100
	for i := 0; i <= maxWalkDepth+4; i++ {
		item := fmt.Sprintf("<< /Title (Item) /Parent 20 0 R /Next %d 0 R >>", first+i+1)
		if i == maxWalkDepth+4 {
			item = "<< /Title (Last) /Parent 20 0 R /A 46 0 R >>"
		}
		long[first+i] = item
	}
	long[20] = fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>", first, first+maxWalkDepth+4, maxWalkDepth+5)
	deepOutline := base(long, screen[:len(screen)-len("/A 44 0 R >>")]+">>", "/Outlines 20 0 R", "")
	got := verdictOf(t, deepOutline, "7.18.6.2 t1")
	if got.Verdict != CannotCheck {
		t.Errorf("an outline chain past the walk bound reports %v (%s), want CannotCheck — a clip nib never "+
			"reached is not a document without one", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "more than") {
		t.Errorf("the reason %q does not say the outline chain ran past the bound", got.Why)
	}
}

// TestOneStructParentDoorAnswersTheSameForAFieldAsForAnAnnotation — P05's phase-close regression.
//
// **The hop had TWO implementations and they disagreed.** `elementForStructParent` (the door above,
// reached by `annotElement`) keeps a `found` check, so a row that is PRESENT and is not a dictionary
// is a definite answer even when the tree is short elsewhere. `scanAnnotsAndFields` resolved the same
// key with `d.dict(pt[sp]) != nil` and nothing else, so on a document holding BOTH shapes it recorded
// the truncation on the subject and `7.2 t24/t25/t29` answered `CannotCheck` — over the very row
// `7.18.1 t1` and `7.18.4 t1` called a definite `Fail`. `annots.go` already carried a comment calling
// that collapse found-and-fixed; it was fixed in the door P05.S01 wrote and not in the door P04.S03
// wrote beside it, which is exactly what ADR-009 exists to refuse.
//
// The fixture is `TestAPresentButUnusableRowIsDefiniteEvenWhenTheTreeIsShort`'s, with a form field
// added on the same key and the catalog's `/Lang` removed — with it, `checkAssociatedTextLanguage`
// answers from the catalog and never reaches the subject at all.
func TestOneStructParentDoorAnswersTheSameForAFieldAsForAnAnnotation(t *testing.T) {
	// Row 1 is present and is an ARRAY (not an element), and a sibling branch nests past the bound so
	// the walk reports a reason. Both halves at once are what separate the two answers.
	shortTreeWithAPresentRow := func() map[int]string {
		m := map[int]string{}
		for i := 0; i <= maxWalkDepth+2; i++ {
			m[300+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [9 9] >>", 301+i)
		}
		m[300+maxWalkDepth+3] = "<< /Nums [9 11 0 R] /Limits [9 9] >>"
		m[11] = "<< /Type /StructElem /S /Annot /P 7 0 R >>"
		m[9] = "<< /Kids [290 0 R 300 0 R] >>"
		m[290] = "<< /Nums [1 [10 0 R]] /Limits [1 1] >>"
		// A catalog with NO /Lang, so the clause must read the subject, plus the AcroForm.
		m[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R " +
			"/AcroForm << /Fields [31 0 R] >> >>"
		m[31] = "<< /FT /Tx /T (f) /TU (a field) /DA (/Helv 0 Tf 0 g) /StructParent 1 >>"
		return m
	}
	fx := annotFixture{annot: note(), elem: annotTag, extra: shortTreeWithAPresentRow()}
	if got := verdictOf(t, fx.build(), "7.2 t25"); got.Verdict != Fail {
		t.Errorf("a field naming a present row that is not an element reports %v (%s), want Fail — the "+
			"annotation door calls that row definite, and one key may not have two answers", got.Verdict, got.Why)
	}
	// The annotation half of the SAME document, so the test compares the two doors rather than
	// asserting one of them in isolation.
	if got := verdictOf(t, fx.build(), "7.18.1 t1"); got.Verdict != Fail {
		t.Errorf("the annotation half of the same document reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	// **The stimulus, before the response.** With the deep branch gone the tree is fully read, so the
	// row is definite by both readings and Fail proves nothing about the collapse; with row 1 made a
	// real element carrying a /Lang, the field passes. Together they show the document above exercises
	// the disagreement rather than failing for an unrelated reason.
	shallow := shortTreeWithAPresentRow()
	for i := 0; i <= maxWalkDepth+3; i++ {
		delete(shallow, 300+i)
	}
	shallow[9] = "<< /Kids [290 0 R] >>"
	if got := verdictOf(t, (annotFixture{annot: note(), elem: annotTag, extra: shallow}).build(), "7.2 t25"); got.Verdict != Fail {
		t.Errorf("control: with the tree fully read, the same present array row reports %v (%s), want Fail",
			got.Verdict, got.Why)
	}
	resolvable := shortTreeWithAPresentRow()
	resolvable[290] = "<< /Nums [1 12 0 R] /Limits [1 1] >>"
	resolvable[12] = "<< /Type /StructElem /S /Form /P 7 0 R /Lang (en-GB) >>"
	if got := verdictOf(t, (annotFixture{annot: note(), elem: annotTag, extra: resolvable}).build(), "7.2 t25"); got.Verdict != Pass {
		t.Errorf("control: a row that IS an element declaring /Lang reports %v (%s), want Pass", got.Verdict, got.Why)
	}
}

// TestAnUnreadablePopulationIsARefusalAndNotAnEmptyOne — P05's phase-close regression, two walks.
//
// **A walk that returns quietly makes "nib could not finish reading" indistinguishable from "there is
// nothing there",** and the two produce opposite verdicts: an empty population is `NotApplicable`, an
// unread one must be `CannotCheck`. Both walks below returned quietly.
//
//   - The AcroForm walk discarded `DereferenceArray`'s error for `/Fields` and every `/Kids`, so
//     `7.2 t25` answered "the document has no form fields". Since P05.S04 `mediaclips` builds the clip
//     population from the same walk, so a dropped field path would also have left `7.18.6.2 t1/t2`
//     answering Pass over a clip nobody looked at — the silent truncation S04 fixed for the OUTLINE
//     walk, twelve lines from this one.
//   - `parentTree` skipped a present-but-unreadable `/Nums` or `/Kids` without setting `ptErr`, so the
//     keys below were simply missing with no reason — and `elementForStructParent` then takes its
//     DEFINITE branch and reports "the /StructParent names no element", a Fail over a tree nib never
//     finished reading. `elementForMCID` separates the two for its own array read; this walk did not.
//
// **Neither shape can be reached through a file**, measured against pdfcpu v0.13.0: a `/Fields` or a
// `/Nums` that is present and is not an array is refused by its validator first
// (`dereferenceArray: wrong type types.Dict`), before any rule runs. That is a fact about the
// DEPENDENCY, not about nib — the same footing `openMutated` already states — so the state is made in
// memory, where nothing stands between the reader and the branch. If a pdfcpu bump ever admits such a
// file, these are the readers that decide the verdict.
func TestAnUnreadablePopulationIsARefusalAndNotAnEmptyOne(t *testing.T) {
	notAnArray := types.Dict{"Type": types.Name("Font")}
	t25 := associatedTextKeys[1]

	// The AcroForm half. The field population is unreadable; the annotation population is untouched,
	// so t24 must still answer in full — a truncated field walk says nothing about annotations.
	d := openMutated(t, (annotFixture{annot: note(), elem: annotTag}).build(), func(d *Document, _ types.Dict) {
		d.Catalog["AcroForm"] = types.Dict{"Fields": notAnArray}
	})
	if got := checkAssociatedTextLanguage(d, t25); got.Verdict != CannotCheck {
		t.Errorf("a /Fields nib cannot read reports %v (%s), want CannotCheck — an unreadable population "+
			"is not an empty one", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "short of members") {
		t.Errorf("the reason %q does not say the field population is short", got.Why)
	}
	// The stimulus: with a REAL empty /Fields the same clause is NotApplicable, so the row above is the
	// refusal firing and not the clause failing for an unrelated reason.
	empty := openMutated(t, (annotFixture{annot: note(), elem: annotTag}).build(), func(d *Document, _ types.Dict) {
		d.Catalog["AcroForm"] = types.Dict{"Fields": types.Array{}}
	})
	if got := checkAssociatedTextLanguage(empty, t25); got.Verdict != NotApplicable {
		t.Errorf("control: an empty /Fields reports %v (%s), want NotApplicable", got.Verdict, got.Why)
	}

	// The parent-tree half. `/Nums` is present and unreadable, so every row below it is unread.
	tree := openMutated(t, (annotFixture{annot: note(), elem: annotTag}).build(), func(d *Document, _ types.Dict) {
		root := d.dict(d.Catalog["StructTreeRoot"])
		if root == nil {
			t.Fatal("the fixture has no StructTreeRoot")
		}
		root["ParentTree"] = types.Dict{"Nums": notAnArray}
	})
	if got := checkAnnotationsAreNestedInAnnotTags(tree); got.Verdict != CannotCheck {
		t.Errorf("an annotation whose parent tree nib could not read reports %v (%s), want CannotCheck — "+
			"a tree that was never read may not yield a definite \"names no element\"", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "never read") {
		t.Errorf("the reason %q does not say the keys below were never read", got.Why)
	}
	// The stimulus: a tree that IS readable and genuinely has no row 1 stays a definite Fail.
	noRow := openMutated(t, (annotFixture{annot: note(), elem: annotTag}).build(), func(d *Document, _ types.Dict) {
		root := d.dict(d.Catalog["StructTreeRoot"])
		root["ParentTree"] = types.Dict{"Nums": types.Array{types.Integer(0), types.Array{}}}
	})
	if got := checkAnnotationsAreNestedInAnnotTags(noRow); got.Verdict != Fail {
		t.Errorf("control: a readable tree with no row 1 reports %v (%s), want Fail", got.Verdict, got.Why)
	}
}
