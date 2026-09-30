package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// taggedCarryShape is one variation of the two-page tagged fixture.
type taggedCarryShape struct {
	content1, content2 string
	res1               string // added to page 1's resources
	nested             bool   // the /ParentTree nests its row under /Kids
	page2Untagged      bool   // page 2 has no /StructParents, no row and no element
	row0               string // page 1's row, when not the default [21 0 R 22 0 R]
	elem21K            string // element 21's /K, when not 0
	extra              map[int]string
}

const (
	tagCarried = "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC\n"
	tagStays   = "/P <</MCID 1>> BDC BT /F1 12 Tf 14 TL 72 640 Td (Stays here) Tj T* (and here) Tj ET EMC\n"
	tagPage2   = "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Page two) Tj ET EMC\n"
)

// taggedCarryDoc is two tagged pages: page 1 draws "Carried" (MCID 0, element 21) and "Stays here" (MCID 1, element 22),
// page 2 draws "Page two" (MCID 0, element 23). "Carried" is one line and "Stays here" two, 60 points below, so they read as
// two paragraphs — unless the shape says otherwise.
func taggedCarryDoc(sh taggedCarryShape) []byte {
	if sh.content1 == "" {
		sh.content1 = tagCarried + tagStays
	}
	if sh.content2 == "" {
		sh.content2 = tagPage2
	}
	if sh.row0 == "" {
		sh.row0 = "[21 0 R 22 0 R]"
	}
	if sh.elem21K == "" {
		sh.elem21K = "0"
	}
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 20 0 R /MarkInfo << /Marked true >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> " + sh.res1 + " >> /Contents 5 0 R /StructParents 0 >>",
		5:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(sh.content1), sh.content1),
		6:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(sh.content2), sh.content2),
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		8:  "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>",
		21: "<< /Type /StructElem /S /P /P 20 0 R /Pg 3 0 R /K " + sh.elem21K + " >>",
		22: "<< /Type /StructElem /S /P /P 20 0 R /Pg 3 0 R /K 1 >>",
	}
	nums := "0 " + sh.row0
	if sh.page2Untagged {
		objs[4] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 8 0 R >> >> /Contents 6 0 R >>"
		objs[20] = "<< /Type /StructTreeRoot /K [21 0 R 22 0 R] /ParentTree 24 0 R /ParentTreeNextKey 1 >>"
	} else {
		objs[4] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 8 0 R >> >> /Contents 6 0 R /StructParents 1 >>"
		objs[20] = "<< /Type /StructTreeRoot /K [21 0 R 22 0 R 23 0 R] /ParentTree 24 0 R /ParentTreeNextKey 2 >>"
		objs[23] = "<< /Type /StructElem /S /P /P 20 0 R /Pg 4 0 R /K 0 >>"
		nums += " 1 [23 0 R]"
	}
	if sh.nested {
		objs[24] = "<< /Kids [25 0 R] >>"
		lim := "[0 1]"
		if sh.page2Untagged {
			lim = "[0 0]" // key 1 is new: the carry must raise the leaf's /Limits to reach it
		}
		objs[25] = "<< /Limits " + lim + " /Nums [" + nums + "] >>"
	} else {
		objs[24] = "<< /Nums [" + nums + "] >>"
	}
	for k, v := range sh.extra {
		objs[k] = v
	}
	return assembleFixture(objs)
}

// structureOf reads pdf's structure: each element's text and page by object number, the consistency defects, and the
// written tree for its rows.
func structureOf(t *testing.T, pdf []byte) (map[int]StructureElement, []structDefect, *model.Context, *structTree) {
	t.Helper()
	st, err := ReadStructure(pdf)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int]StructureElement{}
	for _, e := range st.Elements {
		byID[e.ID] = e
	}
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	live := map[int]bool{}
	for _, pa := range pdfread.Pages(ctx) {
		if pa.Ref != nil {
			live[pa.Ref.ObjectNumber.Value()] = true
		}
	}
	tree, err := readStructTree(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	return byID, checkStructConsistency(ctx, tree), ctx, tree
}

// slotNames says whether slot i of key's row names object obj.
func slotNames(ctx *model.Context, tree *structTree, key, i, obj int) bool {
	row, _, found := rowFor(ctx, tree, key)
	if !found || i >= len(row) {
		return false
	}
	r, ok := row[i].(types.IndirectRef)
	return ok && r.ObjectNumber.Value() == obj
}

// TestATaggedParagraphCarriesItsStructure — `PLAN-text-reflow.md` P07.S07: a paragraph whose marked content belongs to
// page 1's structure, carried to page 2, reads back there with its text AND its element — the structure reader finds the
// element's text on page 2 and places the element there — while page 1's row forgets it, page 2's row names it at the MCID
// page 2 now draws it under, and the tree is consistent. Over a flat and a nested /ParentTree (7 of the 14 multi-page
// real-producer documents nest one), and onto a page that had no row.
//
// **The stimulus is asserted**: before the carry the element's text is on page 1, under MCID 0.
func TestATaggedParagraphCarriesItsStructure(t *testing.T) {
	for _, c := range []struct {
		name    string
		sh      taggedCarryShape
		dstKey  int
		dstMCID int
	}{
		{"flat", taggedCarryShape{}, 1, 1},
		{"nested", taggedCarryShape{nested: true}, 1, 1},
		{"onto a page with no row", taggedCarryShape{page2Untagged: true, content2: "BT /F1 12 Tf 72 700 Td (Page two) Tj ET\n"}, 1, 0},
		{"nested, onto a page with no row", taggedCarryShape{nested: true, page2Untagged: true, content2: "BT /F1 12 Tf 72 700 Td (Page two) Tj ET\n"}, 1, 0},
		{"page 2's stream draws an MCID its row does not have", taggedCarryShape{content2: tagPage2 +
			"/Span <</MCID 4>> BDC BT /F1 12 Tf 72 650 Td (Unlisted) Tj ET EMC\n"}, 1, 5},
		{"page 2's stream draws the MCID just past its row", taggedCarryShape{content2: tagPage2 +
			"/Span <</MCID 1>> BDC BT /F1 12 Tf 72 650 Td (Unlisted) Tj ET EMC\n"}, 1, 2},
		{"nested, page 2 declares a key above every row and has none", taggedCarryShape{nested: true, page2Untagged: true,
			content2: "BT /F1 12 Tf 72 700 Td (Page two) Tj ET\n", extra: map[int]string{
				4: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 8 0 R >> >> /Contents 6 0 R /StructParents 5 >>"}}, 5, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := taggedCarryDoc(c.sh)
			before, _, _, _ := structureOf(t, pdf)
			if e := before[21]; e.Text != "Carried" || e.Page != 1 {
				t.Fatalf("setup: element 21 reads %q on page %d", e.Text, e.Page)
			}
			out, cause := carried(t, pdf, 0, 300)
			if cause != "" {
				t.Fatalf("refused: %s", cause)
			}
			got, defects, ctx, tree := structureOf(t, out)
			if e := got[21]; e.Text != "Carried" || e.Page != 2 {
				t.Errorf("the carried element reads %q on page %d, want \"Carried\" on page 2", e.Text, e.Page)
			}
			if e := got[22]; e.Text != "Stays hereand here" || e.Page != 1 {
				t.Errorf("the element that stays reads %q on page %d", e.Text, e.Page)
			}
			if !c.sh.page2Untagged {
				if e := got[23]; e.Text != "Page two" || e.Page != 2 {
					t.Errorf("page 2's own element reads %q on page %d", e.Text, e.Page)
				}
			}
			if len(defects) != 0 {
				t.Errorf("the tree is inconsistent after the carry: %+v", defects)
			}
			if c.sh.nested {
				leaf := rightmostLeaf(ctx, tree)
				if lim := derefArray(ctx.XRefTable, leaf["Limits"]); len(lim) != 2 || fmt.Sprint(lim[1]) != fmt.Sprint(c.dstKey) {
					t.Errorf("the nested tree's rightmost leaf has /Limits %v, which a reader following /Limits reads as excluding key %d", lim, c.dstKey)
				}
			}
			if slotNames(ctx, tree, 0, 0, 21) {
				t.Error("page 1's row still names the carried element at MCID 0")
			}
			if !slotNames(ctx, tree, 0, 1, 22) {
				t.Error("page 1's row lost the element that stays")
			}
			if !slotNames(ctx, tree, c.dstKey, c.dstMCID, 21) {
				t.Errorf("page 2's row (key %d) does not name the carried element at MCID %d", c.dstKey, c.dstMCID)
			}
			r, ok := findRun(runsOnPage(t, out, 2), "Carried")
			if !ok || r.mcid != c.dstMCID {
				t.Errorf("page 2 draws \"Carried\" under MCID %d (found %v), want %d", r.mcid, ok, c.dstMCID)
			}
			for _, r := range runsOnPage(t, out, 1) {
				if r.mcid == 0 {
					t.Errorf("page 1 still draws %q under MCID 0", r.text)
				}
			}
			assertSequences(t, out, 1, map[int]int{0: 0, 1: 1})
			assertSequences(t, out, 2, map[int]int{c.dstMCID: 1})
		})
	}
}

// TestANamedPropertyListIsCarriedInline — P07.S07: a sequence whose property list is named in page 1's /Properties is
// opened on page 2 with that list written inline — its /Lang kept — under the new MCID.
func TestANamedPropertyListIsCarriedInline(t *testing.T) {
	pdf := taggedCarryDoc(taggedCarryShape{
		content1: "/P /MC0 BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC\n" + tagStays,
		res1:     "/Properties << /MC0 << /MCID 0 /Lang (fr-FR) >> >>",
	})
	out, cause := carried(t, pdf, 0, 300)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	c2, err := pdfread.PageContent(ctx, pageAt(ctx, nil, 2).Dict, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(c2, []byte("/Lang(fr-FR)")) || !bytes.Contains(c2, []byte("/MCID 1")) {
		t.Errorf("page 2 does not open the carried sequence with its language and MCID 1:\n%s", c2)
	}
	if got, defects, _, _ := structureOf(t, out); got[21].Text != "Carried" || got[21].Page != 2 || len(defects) != 0 {
		t.Errorf("element 21 reads %q on page %d, defects %+v", got[21].Text, got[21].Page, defects)
	}
}

// TestATaggedCarryRefusesWhatCannotGoWhole — P07.S07: what a carry cannot take to the other page whole with its structure
// refuses `tagged-across-pages` by name, and the untouched fixture carries (so each refusal is the variation's).
func TestATaggedCarryRefusesWhatCannotGoWhole(t *testing.T) {
	if _, cause := carried(t, taggedCarryDoc(taggedCarryShape{}), 0, 300); cause != "" {
		t.Fatalf("setup: the plain fixture refuses %s", cause)
	}
	for _, c := range []struct {
		name string
		sh   taggedCarryShape
	}{
		{"a sequence opened inside it", taggedCarryShape{
			content1: "/P <</MCID 0>> BDC /Span <</Lang (en)>> BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC EMC\n" + tagStays}},
		{"the sequence draws a rule too", taggedCarryShape{
			content1: "/P <</MCID 0>> BDC 72 690 100 1 re f BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC\n" + tagStays}},
		{"nested in another MCID's sequence", taggedCarryShape{
			content1: "/Div <</MCID 2>> BDC /P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC EMC\n" + tagStays,
			row0:     "[21 0 R 22 0 R 22 0 R]"}},
		{"no element claims the MCID", taggedCarryShape{row0: "[null 22 0 R]"}},
		{"the element does not name it", taggedCarryShape{elem21K: "5"}},
		{"a property list with an indirect value", taggedCarryShape{
			content1: "/P /MC0 BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC\n" + tagStays,
			res1:     "/Properties << /MC0 << /MCID 0 /Foo 30 0 R >> >>",
			extra:    map[int]string{30: "<< /X 1 >>"}}},
		{"the sequence draws a form", taggedCarryShape{
			content1: "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET /Fm0 Do EMC\n" + tagStays,
			res1:     "/XObject << /Fm0 30 0 R >>",
			extra:    map[int]string{30: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 3 >>\nstream\n0 g\nendstream"}}},
		{"the sequence draws an inline image", taggedCarryShape{
			content1: "/P <</MCID 0>> BDC BI /W 1 /H 1 /CS /G /BPC 8 ID a EI BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC\n" + tagStays}},
		{"the target's key has no row inside a nested tree's range", taggedCarryShape{nested: true, extra: map[int]string{
			23: "<< /Type /StructElem /S /P /P 20 0 R /K [] >>",
			25: "<< /Limits [0 3] /Nums [0 [21 0 R 22 0 R] 3 []] >>"}}},
		{"the target's key names a single element", taggedCarryShape{extra: map[int]string{
			24: "<< /Nums [0 [21 0 R 22 0 R] 1 23 0 R] >>"}}},
		{"the page's row stops before the MCID", taggedCarryShape{
			content1: "/P <</MCID 5>> BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET EMC\n" + tagStays, elem21K: "5"}},
		{"two sequences carry the MCID", taggedCarryShape{
			content1: tagCarried + "/P <</MCID 0>> BDC BT /F1 12 Tf 300 500 Td (Also zero) Tj ET EMC\n" + tagStays}},
		{"the element's integer kid is on another page", taggedCarryShape{extra: map[int]string{
			21: "<< /Type /StructElem /S /P /P 20 0 R /Pg 4 0 R /K 0 >>"}}},
		{"the sequence is left open", taggedCarryShape{
			content1: tagStays + "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Carried) Tj ET\n"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := taggedCarryDoc(c.sh)
			l, _ := layoutOf(t, pdf)
			pi := -1
			for i, p := range l.paragraphs {
				if p.text() == "Carried" {
					pi = i
				}
			}
			if pi < 0 {
				t.Fatal("setup: no paragraph reads \"Carried\"")
			}
			if _, cause := carried(t, pdf, pi, 300); cause != causeTaggedAcross {
				t.Errorf("cause %q, want %q", cause, causeTaggedAcross)
			}
		})
	}
}

// TestACarriedAnnotationTakesItsStructureReference — P07.S07 (found by the slice's deepdive, on S06's path): an
// annotation that crosses to page 2 re-points the OBJR the structure tree describes it by — and the element holding only
// it follows it — so the tree does not place a link on a page it left.
func TestACarriedAnnotationTakesItsStructureReference(t *testing.T) {
	pdf := taggedCarryDoc(taggedCarryShape{
		row0: "[21 0 R 22 0 R]",
		extra: map[int]string{
			26: "<< /Type /StructElem /S /Link /P 20 0 R /Pg 3 0 R /K [<< /Type /OBJR /Obj 30 0 R >>] >>",
			30: "<< /Type /Annot /Subtype /Link /Rect [72 695 140 712] /P 3 0 R /StructParent 2 /Border [0 0 0] /A << /S /URI /URI (https://example.org/) >> >>",
			24: "<< /Nums [0 [21 0 R 22 0 R] 1 [23 0 R] 2 26 0 R] >>",
		},
	})
	pdf = bytes.Replace(pdf, []byte("/Contents 5 0 R /StructParents 0"), []byte("/Contents 5 0 R /StructParents 0 /Annots [30 0 R]"), 1)
	pdf = bytes.Replace(pdf, []byte("/K [21 0 R 22 0 R 23 0 R]"), []byte("/K [21 0 R 22 0 R 23 0 R 26 0 R]"), 1)
	pdf = repairXref(t, pdf)
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 := pageAt(ctx, nil, 1), pageAt(ctx, nil, 2)
	objr := func() types.Dict {
		el := derefDict(ctx.XRefTable, types.IndirectRef{ObjectNumber: 26})
		return derefDict(ctx.XRefTable, derefArray(ctx.XRefTable, el["K"])[0])
	}
	if _, has := objr()["Pg"]; has {
		t.Fatal("setup: the OBJR names a page of its own")
	}
	if err := carryAnchors(ctx, p1, p2, anchorsIn(ctx, p1, [4]float64{0, 690, 612, 720}), -300); err != nil {
		t.Fatal(err)
	}
	if !sameObject(objr()["Pg"], *p2.Ref) {
		t.Errorf("the carried link's OBJR names page %v, want page 2", objr()["Pg"])
	}
	if el := derefDict(ctx.XRefTable, types.IndirectRef{ObjectNumber: 26}); !sameObject(el["Pg"], *p2.Ref) {
		t.Errorf("the link's element, holding nothing else, still names page %v", el["Pg"])
	}
}

// repairXref rewrites a fixture's xref after an in-place byte edit, by re-reading its objects.
func repairXref(t *testing.T, pdf []byte) []byte {
	t.Helper()
	objs := map[int]string{}
	s := string(pdf)
	for {
		i := strings.Index(s, " 0 obj\n")
		if i < 0 {
			break
		}
		j := strings.LastIndexAny(s[:i], "\n") + 1
		var n int
		fmt.Sscanf(s[j:i], "%d", &n)
		end := strings.Index(s[i:], "\nendobj\n")
		objs[n] = s[i+len(" 0 obj\n") : i+end]
		s = s[i+end:]
	}
	return assembleFixture(objs)
}

// taggedCascadeDoc is cascadeDoc's pages tagged: paragraph k of page p its own /P element owning MCID k of the page, under
// a flat or nested /ParentTree. Element (p, k) is object 100 + 40p + k, pages counted from 0.
func taggedCascadeDoc(counts []int, nested bool, pad ...string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 3 0 R /MarkInfo << /Marked true >> >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var kids, elems, nums []string
	for i, n := range counts {
		pageObj, contentObj := 10+2*i, 11+2*i
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
		var b strings.Builder
		var row []string
		for k := 0; k < n; k++ {
			fmt.Fprintf(&b, "/P <</MCID %d>> BDC BT /F1 12 Tf 14 TL 72 %d Td (%s) Tj T* (%s) Tj ET EMC\n", k, 700-38*k, cascadeLine(i+1, k), cascadeLine(i+1, k))
			e := 100 + 40*i + k
			objs[e] = fmt.Sprintf("<< /Type /StructElem /S /P /P 3 0 R /Pg %d 0 R /K %d >>", pageObj, k)
			elems = append(elems, fmt.Sprintf("%d 0 R", e))
			row = append(row, fmt.Sprintf("%d 0 R", e))
		}
		nums = append(nums, fmt.Sprintf("%d [%s]", i, strings.Join(row, " ")))
		if i < len(pad) {
			b.WriteString(pad[i])
		}
		c := b.String()
		objs[pageObj] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents %d 0 R /StructParents %d >>", contentObj, i)
		objs[contentObj] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c)
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(counts))
	objs[3] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ParentTree 4 0 R /ParentTreeNextKey %d >>", strings.Join(elems, " "), len(counts))
	if nested {
		// One leaf per page, as Acrobat and Word write them.
		var leaves []string
		for i := range counts {
			objs[90+i] = fmt.Sprintf("<< /Limits [%d %d] /Nums [%s] >>", i, i, nums[i])
			leaves = append(leaves, fmt.Sprintf("%d 0 R", 90+i))
		}
		objs[4] = fmt.Sprintf("<< /Kids [%s] >>", strings.Join(leaves, " "))
	} else {
		objs[4] = fmt.Sprintf("<< /Nums [%s] >>", strings.Join(nums, " "))
	}
	return assembleFixture(objs)
}

// TestATaggedGrowthFlowsWithItsStructure — P07.S07 through the reflow door: a growth on a tagged document flows page 1's
// last paragraph onto page 2 and page 2's onto page 3, and each carried element reads back on its new page, the rows moved,
// the tree consistent — over a flat and a nested /ParentTree. Before S07 this refused `tagged-across-pages`.
func TestATaggedGrowthFlowsWithItsStructure(t *testing.T) {
	for _, c := range []struct {
		nested bool
		pad    string // drawn on page 2 after its paragraphs: an MCID its row does not have
		at     int    // the MCID page 1's carried paragraph takes on page 2
	}{{false, "", 16}, {true, "", 16}, {false, "/Span <</MCID 30>> BDC EMC\n", 31}} {
		nested := c.nested
		t.Run(fmt.Sprintf("nested=%v pad=%q", nested, c.pad), func(t *testing.T) {
			pdf := taggedCascadeDoc([]int{16, 16, 5}, nested, "", c.pad)
			orig, edit := threeLinesMore()
			out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
			if err != nil || out == nil {
				t.Fatalf("refused %+v (%v)", refusal, err)
			}
			got, defects, ctx, tree := structureOf(t, out)
			want := func(obj, page int, text string) {
				t.Helper()
				if e := got[obj]; e.Page != page || e.Text != text+text {
					t.Errorf("element %d reads %q on page %d, want %q on page %d", obj, e.Text, e.Page, text+text, page)
				}
			}
			want(100+15, 2, cascadeLine(1, 15))    // page 1's last paragraph, now on page 2
			want(100+40+15, 3, cascadeLine(2, 15)) // page 2's last, now on page 3
			want(100+40, 2, cascadeLine(2, 0))     // page 2's first, still there
			want(100+14, 1, cascadeLine(1, 14))
			if len(defects) != 0 {
				t.Errorf("the tree is inconsistent after the flow: %+v", defects)
			}
			if slotNames(ctx, tree, 0, 15, 115) || slotNames(ctx, tree, 1, 15, 155) {
				t.Error("a source row still names an element that left its page")
			}
			assertSequences(t, out, 1, map[int]int{15: 0})
			if !slotNames(ctx, tree, 1, c.at, 115) || !slotNames(ctx, tree, 2, 5, 155) {
				t.Error("a target row does not name the element that arrived, at the first MCID free there")
			}
		})
	}
}

// TestASplitSequenceIsCarriedAsASecondKid — P07.S07: one sequence drawing both the carried paragraph and one that stays is
// split — the source keeps the sequence, its slot and its kid for what stays, and the carried text becomes a second kid
// of the SAME element on page 2, after the first — so the element reads its text in order across the pages. A clipping
// path inside the sequence paints nothing and does not stop it.
func TestASplitSequenceIsCarriedAsASecondKid(t *testing.T) {
	pdf := taggedCarryDoc(taggedCarryShape{
		content1: "/P <</MCID 0>> BDC q 0 0 612 792 re W n BT /F1 12 Tf 14 TL 72 640 Td (Stays here) Tj T* (and here) Tj ET " +
			"BT /F1 12 Tf 72 400 Td (Carried) Tj ET Q EMC\n",
		row0: "[21 0 R]",
	})
	pdf = bytes.Replace(pdf, []byte("/K [21 0 R 22 0 R 23 0 R]"), []byte("/K [21 0 R 23 0 R]"), 1)
	pdf = repairXref(t, pdf)
	l, _ := layoutOf(t, pdf)
	pi := -1
	for i, p := range l.paragraphs {
		if p.text() == "Carried" {
			pi = i
		}
	}
	if pi < 0 || len(l.paragraphs) != 2 {
		t.Fatalf("setup: paragraphs %d, \"Carried\" at %d", len(l.paragraphs), pi)
	}
	out, cause := carried(t, pdf, pi, 0)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	got, defects, ctx, tree := structureOf(t, out)
	if e := got[21]; e.Text != "Stays hereand hereCarried" || e.Page != 1 {
		t.Errorf("the split element reads %q on page %d, want its page 1 text then its page 2 text, on page 1", e.Text, e.Page)
	}
	if len(defects) != 0 {
		t.Errorf("the tree is inconsistent after the carry: %+v", defects)
	}
	if !slotNames(ctx, tree, 0, 0, 21) || !slotNames(ctx, tree, 1, 1, 21) {
		t.Error("the element is not named by both its page 1 slot and its new page 2 slot")
	}
	if r, ok := findRun(runsOnPage(t, out, 1), "Stays here"); !ok || r.mcid != 0 {
		t.Errorf("page 1's remaining text is drawn under MCID %d (found %v), want 0", r.mcid, ok)
	}
}

// rightmostLeaf is the last leaf of the /ParentTree, following the last kid down.
func rightmostLeaf(ctx *model.Context, tree *structTree) types.Dict {
	d := derefDict(ctx.XRefTable, tree.root["ParentTree"])
	for {
		kids := derefArray(ctx.XRefTable, d["Kids"])
		if len(kids) == 0 {
			return d
		}
		d = derefDict(ctx.XRefTable, kids[len(kids)-1])
	}
}

// TestAPageRowJoinsAFlatTreeInKeyOrder — P07.S07's review: a target page declaring a key with no row, below a higher key
// the tree holds, gains its row BEFORE that key — a number tree's keys ascend (ISO 32000-1 §7.9.7), and appending put it
// after.
func TestAPageRowJoinsAFlatTreeInKeyOrder(t *testing.T) {
	pdf := taggedCarryDoc(taggedCarryShape{extra: map[int]string{
		23: "<< /Type /StructElem /S /P /P 20 0 R /K [] >>",
		24: "<< /Nums [0 [21 0 R 22 0 R] 3 []] >>"}})
	out, cause := carried(t, pdf, 0, 300)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	_, _, ctx, tree := structureOf(t, out)
	nums := derefArray(ctx.XRefTable, derefDict(ctx.XRefTable, tree.root["ParentTree"])["Nums"])
	var keys []string
	for i := 0; i+1 < len(nums); i += 2 {
		keys = append(keys, fmt.Sprint(nums[i]))
	}
	if strings.Join(keys, " ") != "0 1 3" {
		t.Errorf("the /ParentTree's keys are %v, want 0 1 3", keys)
	}
}

// TestCloseOpenClosesInnermostFirst — P07.S07: what a target page leaves open is closed innermost first, a marked-content
// sequence included, so the carried text is not read as inside a sequence the page forgot to end.
func TestCloseOpenClosesInnermostFirst(t *testing.T) {
	for _, c := range []struct{ content, want string }{
		{"q BT /P <</MCID 0>> BDC (x) Tj", "EMC\nET\nQ\n"},
		{"/P BMC q BT (x) Tj", "ET\nQ\nEMC\n"},
		{"/P BMC (x) Tj EMC q BT ET Q", ""},
	} {
		if got := closeOpen([]byte(c.content)); got != c.want {
			t.Errorf("closeOpen(%q) = %q, want %q", c.content, got, c.want)
		}
	}
}

// assertSequences asserts that page p of pdf opens exactly want[m] sequences carrying MCID m, and that its content leaves
// no marked-content sequence, text object or state open — a carry's brackets balance, and a source keeps no empty
// sequence naming a slot it cleared.
func assertSequences(t *testing.T, pdf []byte, p int, want map[int]int) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, p)
	l, err := readPageGlyphLayout(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]int{}
	for _, sq := range l.sequences {
		if !sq.inForm {
			got[sq.mcid]++
		}
	}
	for m, n := range want {
		if got[m] != n {
			t.Errorf("page %d opens %d sequence(s) with MCID %d, want %d", p, got[m], m, n)
		}
	}
	c, err := pdfread.PageContent(ctx, pg.Dict, p)
	if err != nil {
		t.Fatal(err)
	}
	if open := closeOpen(c); open != "" {
		t.Errorf("page %d leaves %q open", p, open)
	}
}

// TestAnElementKeepsItsPageWhileAnythingReadsIt — P07.S07's `followsItsContent`: the element's own /Pg follows its content
// only when nothing is left for it to describe. It stays when the element keeps an OBJR with no page of its own, when it
// keeps an MCR naming the source page, and it is not added where the element had none.
func TestAnElementKeepsItsPageWhileAnythingReadsIt(t *testing.T) {
	for _, c := range []struct {
		name, elem string
		wantPg     string // the element's /Pg afterwards, "" for none
	}{
		{"an OBJR inheriting its page", "<< /Type /StructElem /S /P /P 20 0 R /Pg 3 0 R /K [0 << /Type /OBJR /Obj 30 0 R >>] >>", "3"},
		{"an MCR on the source page", "<< /Type /StructElem /S /P /P 20 0 R /Pg 3 0 R /K [0 << /Type /MCR /Pg 3 0 R /MCID 9 >>] >>", "3"},
		{"no /Pg of its own", "<< /Type /StructElem /S /P /P 20 0 R /K [<< /Type /MCR /Pg 3 0 R /MCID 0 >>] >>", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := taggedCarryDoc(taggedCarryShape{extra: map[int]string{21: c.elem, 30: "<< /Type /Annot /Subtype /Text /Rect [500 10 510 20] >>"}})
			out, cause := carried(t, pdf, 0, 300)
			if cause != "" {
				t.Fatalf("refused: %s", cause)
			}
			ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			pg, has := derefDict(ctx.XRefTable, types.IndirectRef{ObjectNumber: 21})["Pg"]
			got := ""
			if has {
				for n := 1; n <= 2; n++ {
					if p := pageAt(ctx, nil, n); p.Ref != nil && sameObject(pg, *p.Ref) {
						got = fmt.Sprint(n + 2)
					}
				}
			}
			if got != c.wantPg {
				t.Errorf("element 21's /Pg is object %q, want %q", got, c.wantPg)
			}
		})
	}
}

// TestInterleavedSequencesAreEachOpenedOnce — P07.S07's `groupedByMCID`: a paragraph whose lines alternate between two
// sequences (drawn out of order, placed by Td) is carried as ONE sequence per MCID, each element reading its own lines.
func TestInterleavedSequencesAreEachOpenedOnce(t *testing.T) {
	pdf := taggedCarryDoc(taggedCarryShape{
		content1: "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Line one) Tj 0 -28 Td (Line three) Tj ET EMC\n" +
			"/Span <</MCID 2>> BDC BT /F1 12 Tf 72 686 Td (Line two) Tj ET EMC\n" + tagStays,
		row0:  "[21 0 R 22 0 R 26 0 R]",
		extra: map[int]string{26: "<< /Type /StructElem /S /Span /P 20 0 R /Pg 3 0 R /K 2 >>"},
	})
	pdf = bytes.Replace(pdf, []byte("/K [21 0 R 22 0 R 23 0 R]"), []byte("/K [21 0 R 22 0 R 23 0 R 26 0 R]"), 1)
	pdf = repairXref(t, pdf)
	if l, _ := layoutOf(t, pdf); len(l.paragraphs) == 0 || l.paragraphs[0].text() != "Line one Line two Line three" {
		t.Fatalf("setup: paragraph 0 does not interleave the two sequences")
	}
	out, cause := carried(t, pdf, 0, 300)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	assertSequences(t, out, 2, map[int]int{1: 1, 2: 1})
	got, defects, _, _ := structureOf(t, out)
	if got[21].Text != "Line oneLine three" || got[26].Text != "Line two" || len(defects) != 0 {
		t.Errorf("elements read %q and %q, defects %+v", got[21].Text, got[26].Text, defects)
	}
}

// TestAddingAnMCIDKeepsASingleKid — P07.S07's deepdive: `addMCIDTo` on an element whose /K is a single integer keeps that
// kid and adds the new one; reading /K through `DereferenceArray` failed on the integer and overwrote it.
func TestAddingAnMCIDKeepsASingleKid(t *testing.T) {
	ctx, err := pdfread.Validated(taggedCarryDoc(taggedCarryShape{}), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := ctx.XRefTable.Catalog()
	tree := &structTree{root: derefDict(ctx.XRefTable, cat["StructTreeRoot"])}
	m, err := addMCIDTo(ctx, tree, 1, types.IndirectRef{ObjectNumber: 21})
	if err != nil {
		t.Fatal(err)
	}
	k := derefDict(ctx.XRefTable, types.IndirectRef{ObjectNumber: 21})["K"]
	if fmt.Sprint(k) != fmt.Sprintf("[0 %d]", m) {
		t.Errorf("element 21's /K is %v after adding MCID %d, want [0 %d]", k, m, m)
	}
}

// ptDoc is a two-page tagged document: page 1 (object 3) and page 2 (object 4) with the page-dictionary extras given, a
// root naming elements 21 (on page 1) and 22 (on page 2), and objects from 24 on — its /ParentTree is object 24 when
// objs gives one.
func ptDoc(t *testing.T, page1, page2 string, objs map[int]string) (*model.Context, *structTree) {
	t.Helper()
	all := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 20 0 R /MarkInfo << /Marked true >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + page1 + " >>",
		4:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + page2 + " >>",
		20: "<< /Type /StructTreeRoot /K [21 0 R 22 0 R] /ParentTreeNextKey 2 >>",
		21: "<< /Type /StructElem /S /P /P 20 0 R /Pg 3 0 R /K 0 >>",
		22: "<< /Type /StructElem /S /P /P 20 0 R /Pg 4 0 R /K 0 >>",
	}
	if _, has := objs[24]; has {
		all[20] = "<< /Type /StructTreeRoot /K [21 0 R 22 0 R] /ParentTree 24 0 R /ParentTreeNextKey 2 >>"
	}
	for k, v := range objs {
		all[k] = v
	}
	ctx, err := pdfread.Validated(assembleFixture(all), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	return ctx, &structTree{root: derefDict(ctx.XRefTable, cat["StructTreeRoot"])}
}

// TestAnIndirectStructParentsIsAKey — P07 phase-close review: a page whose `/StructParents` is an indirect reference to its
// key was read as having NO key by five sites — `pageRow` gave it a new one, orphaning its row, and `addMCIDTo` refused.
// One reader (`structParentsOf`) resolves it everywhere.
func TestAnIndirectStructParentsIsAKey(t *testing.T) {
	ctx, tree := ptDoc(t, "/StructParents 30 0 R", "/StructParents 1", map[int]string{
		24: "<< /Nums [0 [21 0 R] 1 [22 0 R]] >>", 30: "0",
	})
	pd := pageAt(ctx, nil, 1).Dict
	key, slots, err := pageRow(ctx, tree, pd, 1)
	if err != nil || key != 0 || slots != 1 {
		t.Errorf("pageRow: key %d, %d slots (%v), want key 0 with its one slot", key, slots, err)
	}
	if _, still := pd["StructParents"].(types.IndirectRef); !still {
		t.Errorf("the page's key was rewritten: %v", pd["StructParents"])
	}
	mcid, err := addMCIDTo(ctx, tree, 1, *types.NewIndirectRef(21, 0))
	if err != nil || mcid != 1 {
		t.Errorf("addMCIDTo: MCID %d (%v), want 1 on the page's own row", mcid, err)
	}
	if row, _, _ := rowFor(ctx, tree, 0); len(row) != 2 {
		t.Errorf("row 0 has %d slots, want 2", len(row))
	}
}

// TestANewKeyRaisesEveryLimitOnItsWay — P07 phase-close review: a key written above every key goes at the end of the
// rightmost leaf, and EVERY `/Limits` on the way down spans it — the root's too, when it carries one. (A `/Limits` of more
// than two entries, which `widenLimits` also takes, does not survive pdfcpu's validating read to be asked about here.)
func TestANewKeyRaisesEveryLimitOnItsWay(t *testing.T) {
	ctx, tree := ptDoc(t, "/StructParents 0", "/StructParents 1", map[int]string{
		24: "<< /Limits [0 1] /Kids [25 0 R 26 0 R] >>",
		25: "<< /Limits [0 0] /Nums [0 [21 0 R]] >>",
		26: "<< /Limits [1 1] /Nums [1 [22 0 R]] >>",
	})
	leaf, err := parentTreeDict(ctx, tree, 5)
	if err != nil {
		t.Fatal(err)
	}
	lim := func(n int) string {
		d := derefDict(ctx.XRefTable, *types.NewIndirectRef(n, 0))
		return fmt.Sprint(derefArray(ctx.XRefTable, d["Limits"]))
	}
	if got := lim(24); got != "[0 5]" {
		t.Errorf("the root's /Limits %s, want [0 5]", got)
	}
	if got := lim(26); got != "[1 5]" {
		t.Errorf("the rightmost leaf's /Limits %s, want [1 5]", got)
	}
	if !sameDict(leaf, derefDict(ctx.XRefTable, *types.NewIndirectRef(26, 0))) {
		t.Error("the key was not placed in the rightmost leaf")
	}
}

// sameDict says whether a and b are one dictionary (a map's identity).
func sameDict(a, b types.Dict) bool {
	return a != nil && b != nil && fmt.Sprintf("%p", a) == fmt.Sprintf("%p", b)
}

// TestATargetIsRefusedWhereTheWriterWouldBe — P07 phase-close review: `targetRefusal` restated one of `parentTreeDict`'s
// refusals and missed the rest, so a carry onto a page whose nested tree had an empty `/Kids` was accepted and failed
// half-written. It now asks the writer's own non-mutating predicate — for a page with a key and for one without (which
// gets one above every key) — and writes nothing: a document with no `/ParentTree` does not gain one by being asked.
func TestATargetIsRefusedWhereTheWriterWouldBe(t *testing.T) {
	emptyKids := map[int]string{24: "<< /Kids [] >>"}
	for _, c := range []struct {
		name, page2 string
		objs        map[int]string
		want        string
	}{
		{"an empty /Kids, the page keyed", "/StructParents 1", emptyKids, causeTaggedAcross},
		{"an empty /Kids, the page given a key", "", emptyKids, causeTaggedAcross},
		{"a flat tree", "/StructParents 1", map[int]string{24: "<< /Nums [0 [21 0 R] 1 [22 0 R]] >>"}, ""},
		{"no /ParentTree at all", "", nil, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, tree := ptDoc(t, "", c.page2, c.objs)
			_, hadTree := tree.root["ParentTree"]
			if got := targetRefusal(ctx, pageAt(ctx, nil, 2)); got != c.want {
				t.Errorf("targetRefusal %q, want %q", got, c.want)
			}
			if _, hasTree := tree.root["ParentTree"]; hasTree != hadTree {
				t.Error("asking created a /ParentTree")
			}
		})
	}
}
