package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// Creating a structure element and deleting one — ADR-124.

// deleteFixture is a three-page tree built for the delete: a Document holding
//
//	20 P     page 1, MCID 0                              "Alpha"
//	21 Sect  page 1: MCID 1 "Beta", element 22, an MCR with no /Pg (MCID 2 "Gamma"), an MCR naming page 2
//	         (MCID 0 "Zeta"), and an INLINE Span holding MCID 3 "Delta"
//	22 P     names no page — it reads 21's — MCID 4 "Epsilon"
//	23 Link  page 1, one OBJR with no /Pg, for the annotation at object 40 (/StructParent 2)
//	24 Sect  an MCR into the form XObject at object 60 (/StructParents 3), drawn on page 3: "Theta"
//	30 P     page 2, MCID 1 "Eta"
//
// documentPg gives the Document `/Pg` page 1, so what 21 holds stays on the new owner's page. nested writes
// the `/ParentTree` under `/Kids` with page 1's row as an INDIRECT array.
func deleteFixture(documentPg, nested bool) []byte {
	stream := func(dict, body string) string {
		return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
	}
	marked := func(words ...string) string {
		var b strings.Builder
		for i, w := range words {
			fmt.Fprintf(&b, "/P <</MCID %d>> BDC\nBT /F1 12 Tf 72 %d Td (%s) Tj ET\nEMC\n", i, 700-20*i, w)
		}
		return b.String()
	}
	page := func(contents int, extra string) string {
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /XObject << /Fm0 60 0 R >> >> /Contents %d 0 R %s >>", contents, extra)
	}
	docPg := ""
	if documentPg {
		docPg = "/Pg 3 0 R "
	}
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 10 0 R 12 0 R] /Count 3 >>",
		3:  page(4, "/StructParents 0 /Annots [40 0 R]"),
		4:  stream("", marked("Alpha", "Beta", "Gamma", "Delta", "Epsilon")),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 4 >>",
		8:  "<< /Type /StructElem /S /Document /P 7 0 R " + docPg + "/K [20 0 R 21 0 R 23 0 R 24 0 R 30 0 R] >>",
		9:  "<< /Nums [0 [20 0 R 21 0 R 21 0 R null 22 0 R] 1 [21 0 R 30 0 R] 2 23 0 R 3 [24 0 R]] >>",
		10: page(11, "/StructParents 1"),
		11: stream("", marked("Zeta", "Eta")),
		12: page(13, ""),
		13: stream("", "/Fm0 Do\n"),
		20: "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K [0] >>",
		21: "<< /Type /StructElem /S /Sect /P 8 0 R /Pg 3 0 R /K [1 22 0 R << /Type /MCR /MCID 2 >> << /Type /MCR /Pg 10 0 R /MCID 0 >> << /S /Span /P 21 0 R /K [3] >>] >>",
		22: "<< /Type /StructElem /S /P /P 21 0 R /K [4] >>",
		23: "<< /Type /StructElem /S /Link /P 8 0 R /Pg 3 0 R /K [<< /Type /OBJR /Obj 40 0 R >>] >>",
		24: "<< /Type /StructElem /S /Sect /P 8 0 R /K [<< /Type /MCR /Pg 12 0 R /Stm 60 0 R /MCID 0 >>] >>",
		30: "<< /Type /StructElem /S /P /P 8 0 R /Pg 10 0 R /K [1] >>",
		40: "<< /Type /Annot /Subtype /Link /Rect [72 690 200 712] /Border [0 0 0] /StructParent 2 /Contents (A link) >>",
		60: stream("/Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /StructParents 3",
			"/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 600 Td (Theta) Tj ET\nEMC\n"),
	}
	if nested {
		objs[9] = "<< /Kids [50 0 R] >>"
		objs[50] = "<< /Limits [0 3] /Nums [0 51 0 R 1 [21 0 R 30 0 R] 2 23 0 R 3 [24 0 R]] >>"
		objs[51] = "[20 0 R 21 0 R 21 0 R null 22 0 R]"
	}
	return assembleFixture(objs)
}

// textByElement is every addressable element's text, by object number.
func textByElement(v structureView) map[int]string {
	out := map[int]string{}
	for _, e := range v.elements {
		if e.id != 0 {
			out[e.id] = e.text
		}
	}
	return out
}

// contentStreams is every page's content, and every form XObject's a page names, decoded — what a tree
// edit that is not the artifact edit must leave byte for byte.
func contentStreams(t *testing.T, pdf []byte) map[string]string {
	t.Helper()
	ctx := parsed(t, pdf)
	out := map[string]string{}
	for _, pa := range pdfread.Pages(ctx) {
		if pa.Err != nil {
			t.Fatal(pa.Err)
		}
		b, err := pdfread.PageContent(ctx, pa.Dict, pa.Nr)
		if err != nil {
			t.Fatal(err)
		}
		out[fmt.Sprintf("page %d", pa.Nr)] = string(b)
		res, _ := ctx.DereferenceDict(pa.Dict["Resources"])
		xo, _ := ctx.DereferenceDict(res["XObject"])
		for name, o := range xo {
			sd, _, err := ctx.DereferenceStreamDict(o)
			if err != nil || sd == nil {
				continue
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			out[fmt.Sprintf("page %d /%s", pa.Nr, name)] = string(sd.Content)
		}
	}
	return out
}

// slotOwners is the written `/ParentTree` as a reader finds it: each page row's owners by MCID, and each
// single entry's owner.
func slotOwners(t *testing.T, pdf []byte) (map[int][]int, map[int]int) {
	t.Helper()
	ctx := parsed(t, pdf)
	tree, err := readStructTree(ctx, livePageObjects(ctx))
	if err != nil {
		t.Fatal(err)
	}
	return parentTreeEntries(ctx, tree)
}

// rawKids is the written `/K` of object objNr, as an array.
func rawKids(t *testing.T, pdf []byte, objNr int) (*model.Context, types.Array) {
	t.Helper()
	ctx, d := rawElement(t, pdf, objNr)
	k, _ := kidsArray(ctx, d)
	return ctx, k
}

// refTo reports whether o is a reference to object nr.
func refTo(o types.Object, nr int) bool {
	ir, ok := o.(types.IndirectRef)
	return ok && ir.ObjectNumber.Value() == nr
}

// TestACreatedElementIsPlacedAmongItsParentsElements — at an index, appended, and counted among ELEMENT
// kids only; it is written as a grouping element: its type, a `/P` naming its parent, an empty `/K`, no
// page.
func TestACreatedElementIsPlacedAmongItsParentsElements(t *testing.T) {
	src := editFixture() // 8 Sect › [inline P, 9 TH, 10 P › 11 Span]
	newUnder := func(v structureView, parent int, known ...int) (int, []int) {
		kids := kidIDs(v, byID(v)[parent])
		for _, k := range kids {
			old := k == 0
			for _, o := range known {
				old = old || k == o
			}
			if !old {
				return k, kids
			}
		}
		return 0, kids
	}

	out := mustApply(t, src, structEdit{kind: editCreate, value: "Div", parent: 8, index: 1})
	requireConsistent(t, "a create", out)
	v := viewOf(t, out)
	made, kids := newUnder(v, 8, 9, 10)
	if made == 0 || !equalInts(kids, []int{0, made, 9, 10}) {
		t.Fatalf("created at the second place, the section's kids read %v — want [inline new 9 10]", kids)
	}
	if got := byID(v)[made]; got.kind != "Div" || got.text != "" || got.marked || len(got.kids) != 0 {
		t.Errorf("the new element reads %+v — want an empty Div", got)
	}
	_, raw := rawElement(t, out, made)
	k, isArr := raw["K"].(types.Array)
	if ty := raw.NameEntry("Type"); ty == nil || *ty != "StructElem" || !refTo(raw["P"], 8) || !isArr || len(k) != 0 {
		t.Errorf("the new element is written %v — want /Type /StructElem, /P naming the section and an empty /K", raw)
	}
	if _, has := raw["Pg"]; has {
		t.Errorf("the new element names a page (%v): it owns no content", raw["Pg"])
	}

	for _, index := range []int{-1, 3, 99} {
		v := viewOf(t, mustApply(t, src, structEdit{kind: editCreate, value: "L", parent: 8, index: index}))
		if made, kids := newUnder(v, 8, 9, 10); made == 0 || !equalInts(kids, []int{0, 9, 10, made}) {
			t.Errorf("created at index %d, the section's kids read %v — want the new element last", index, kids)
		}
	}
	if v := viewOf(t, mustApply(t, src, structEdit{kind: editCreate, value: "L", parent: 8, index: 0})); kidIDs(v, byID(v)[8])[0] == 0 {
		t.Errorf("created at index 0, the section's kids read %v — want the new element first", kidIDs(v, byID(v)[8]))
	}

	// Among marked content: 8's /K is [0 9 1 10], and the second ELEMENT place is before 10.
	mixed := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Div /P 7 0 R /Pg 3 0 R /K [0 9 0 R 1 10 0 R] >>",
		9:  "<< /Type /StructElem /S /P /P 8 0 R >>",
		10: "<< /Type /StructElem /S /P /P 8 0 R >>",
	})
	out = mustApply(t, mixed, structEdit{kind: editCreate, value: "Span", parent: 8, index: 1})
	_, k = rawKids(t, out, 8)
	if len(k) != 5 || k[0] != types.Integer(0) || !refTo(k[1], 9) || k[2] != types.Integer(1) || !refTo(k[4], 10) {
		t.Errorf("created at the second element place among marked content, /K reads %v — want [0 9 1 new 10]", k)
	}
}

// TestACreatedElementGoesUnderTheRootWhenNoParentIsNamed — parent 0 and the root's own name both mean the
// structure tree root, and the element's `/P` is the root.
func TestACreatedElementGoesUnderTheRootWhenNoParentIsNamed(t *testing.T) {
	for _, parent := range []int{0, rootParent} {
		out := mustApply(t, editFixture(), structEdit{kind: editCreate, value: "Sect", parent: parent, index: -1})
		requireConsistent(t, "a create under the root", out)
		v := viewOf(t, out)
		var top []int
		for _, e := range v.elements {
			if e.parent == -1 {
				top = append(top, e.id)
			}
		}
		if len(top) != 2 || top[0] != 8 || top[1] == 0 {
			t.Fatalf("parent %d: the top of the tree reads %v — want the section and then the new element", parent, top)
		}
		if _, raw := rawElement(t, out, top[1]); !refTo(raw["P"], 7) {
			t.Errorf("parent %d: the new element's /P is %v, want the structure tree root", parent, raw["P"])
		}
		_, k := rawKids(t, out, 7)
		if len(k) != 2 || !refTo(k[0], 8) || !refTo(k[1], top[1]) {
			t.Errorf("parent %d: the root's /K reads %v", parent, k)
		}
	}
	first := viewOf(t, mustApply(t, editFixture(), structEdit{kind: editCreate, value: "Sect", index: 0}))
	if first.elements[0].id == 8 || first.elements[0].kind != "Sect" || first.elements[0].parent != -1 {
		t.Errorf("created first under the root, the tree opens with %+v", first.elements[0])
	}
}

// TestACreateThatDoesNotDescribeTheTreeIsRefused — each refusal with its own error, and nothing written.
func TestACreateThatDoesNotDescribeTheTreeIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		edit structEdit
		want error
		says string
	}{
		{"a type that is not standard", structEdit{kind: editCreate, value: "Bogus", parent: 8}, ErrTagsReview, "not a standard structure type"},
		{"no type", structEdit{kind: editCreate, parent: 8}, ErrTagsReview, "not a standard structure type"},
		{"an element named", structEdit{kind: editCreate, elem: 10, value: "Div", parent: 8}, ErrTagsReview, "names element 10"},
		{"a parent the tree does not have", structEdit{kind: editCreate, value: "Div", parent: 99}, ErrTagsStale, "element 99"},
		{"a parent that is no object", structEdit{kind: editCreate, value: "Div", parent: -7}, ErrTagsStale, ""},
	} {
		_, err := applyStructEdits(editFixture(), []structEdit{c.edit})
		if !errors.Is(err, c.want) || !strings.Contains(fmt.Sprint(err), c.says) {
			t.Errorf("%s: err = %v, want %v saying %q", c.name, err, c.want, c.says)
		}
	}
}

// TestAMoveCanNameTheRoot — `rootParent` takes an element to the top of the tree, which 0 ("the parent it
// has") never could; its `/P` is the root.
func TestAMoveCanNameTheRoot(t *testing.T) {
	out := mustApply(t, editFixture(), structEdit{kind: editMove, elem: 11, parent: rootParent, index: 0})
	requireConsistent(t, "a move to the root", out)
	_, k := rawKids(t, out, 7)
	if len(k) != 2 || !refTo(k[0], 11) || !refTo(k[1], 8) {
		t.Errorf("the root's /K reads %v, want [11 8]", k)
	}
	if _, raw := rawElement(t, out, 11); !refTo(raw["P"], 7) {
		t.Errorf("the moved element's /P is %v, want the structure tree root", raw["P"])
	}
	if _, k := rawKids(t, out, 10); len(k) != 0 {
		t.Errorf("the paragraph it left still lists %v", k)
	}
	// 0 still keeps the parent: a reorder, as the editor has always sent it.
	kept := viewOf(t, mustApply(t, editFixture(), structEdit{kind: editMove, elem: 10, parent: 0, index: 0}))
	if got := kidIDs(kept, byID(kept)[8]); !equalInts(got, []int{10, 0, 9}) {
		t.Errorf("a move with parent 0 reads %v under the section, want [10 inline 9]", got)
	}
}

// TestADeletedElementsKidsTakeItsPlace — the element kids are re-homed in place and in order, each `/P`
// names the new parent, and the deleted element is listed nowhere.
func TestADeletedElementsKidsTakeItsPlace(t *testing.T) {
	out := mustApply(t, deleteFixture(false, false), structEdit{kind: editDelete, elem: 21})
	requireConsistent(t, "a delete", out)
	v := viewOf(t, out)
	ids := byID(v)
	if _, still := ids[21]; still {
		t.Fatal("the deleted element is still in the tree")
	}
	if got := kidIDs(v, ids[8]); !equalInts(got, []int{20, 22, 0, 23, 24, 30}) {
		t.Errorf("the Document's element kids read %v — want [20 22 inline 23 24 30]", got)
	}
	ctx, k := rawKids(t, out, 8)
	if len(k) != 9 || !refTo(k[0], 20) || !refTo(k[2], 22) || !refTo(k[6], 23) || !refTo(k[8], 30) {
		t.Fatalf("the Document's /K reads %v — want 21's five entries where 21 was", k)
	}
	if _, raw := rawElement(t, out, 22); !refTo(raw["P"], 8) {
		t.Errorf("the re-homed paragraph's /P is %v, want the Document", raw["P"])
	}
	inline, _ := ctx.DereferenceDict(k[5])
	if _, isRef := k[5].(types.IndirectRef); isRef || inline == nil || *inline.NameEntry("S") != "Span" || !refTo(inline["P"], 8) {
		t.Errorf("the inline Span was written %v — want it inline still, its /P naming the Document", k[5])
	}
	if p := ids[22]; v.elements[p.parent].id != 8 {
		t.Errorf("the re-homed paragraph's parent reads %d", v.elements[p.parent].id)
	}
}

// TestADeleteAtTheTopHandsItsKidsToTheRoot — a top-level element that holds only elements: they become the
// top of the tree, each `/P` the root.
func TestADeleteAtTheTopHandsItsKidsToTheRoot(t *testing.T) {
	out := mustApply(t, deleteFixture(true, false), structEdit{kind: editDelete, elem: 8})
	requireConsistent(t, "a delete at the top", out)
	_, k := rawKids(t, out, 7)
	if len(k) != 5 || !refTo(k[0], 20) || !refTo(k[1], 21) || !refTo(k[4], 30) {
		t.Fatalf("the root's /K reads %v — want the Document's five kids", k)
	}
	for _, nr := range []int{20, 21, 23, 24, 30} {
		if _, raw := rawElement(t, out, nr); !refTo(raw["P"], 7) {
			t.Errorf("element %d's /P is %v, want the structure tree root", nr, raw["P"])
		}
	}
	// 24 named no page and read the Document's; under the root it would read none.
	if _, raw := rawElement(t, out, 24); !refTo(raw["Pg"], 3) {
		t.Errorf("element 24 took its page from the Document and now names %v", raw["Pg"])
	}
}

// TestAKidThatReadItsPageFromTheDeletedElementKeepsItsPage — element 22 names no page and owns MCID 4 on
// the page 21 names. Under the Document, which names none, it is given that page in writing; under a
// Document on the same page nothing is written.
func TestAKidThatReadItsPageFromTheDeletedElementKeepsItsPage(t *testing.T) {
	out := mustApply(t, deleteFixture(false, false), structEdit{kind: editDelete, elem: 21})
	v := viewOf(t, out)
	if got := byID(v)[22]; got.page != 1 || got.text != "Epsilon" {
		t.Errorf("the paragraph reads page %d and %q — want page 1 and its own text", got.page, got.text)
	}
	if _, raw := rawElement(t, out, 22); !refTo(raw["Pg"], 3) {
		t.Errorf("the paragraph's /Pg is %v, want the page the deleted element named", raw["Pg"])
	}
	ctx, k := rawKids(t, out, 8)
	if inline, _ := ctx.DereferenceDict(k[5]); inline == nil || !refTo(inline["Pg"], 3) {
		t.Errorf("the inline Span, which read the same page, is written %v", k[5])
	}
	for _, e := range v.elements {
		if e.id == 0 && (e.kind != "Span" || e.text != "Delta" || e.page != 1) {
			t.Errorf("the inline element reads %s %q on page %d", e.kind, e.text, e.page)
		}
	}

	same := mustApply(t, deleteFixture(true, false), structEdit{kind: editDelete, elem: 21})
	if _, raw := rawElement(t, same, 22); raw["Pg"] != nil {
		t.Errorf("under a parent on the same page the paragraph was given /Pg %v — nothing moved, so nothing is written", raw["Pg"])
	}
	if got := byID(viewOf(t, same))[22]; got.page != 1 || got.text != "Epsilon" {
		t.Errorf("under a parent on the same page the paragraph reads page %d and %q", got.page, got.text)
	}
}

// TestAnIntegerMCIDStaysAnIntegerOnlyOnTheNewParentsPage — an integer means "on the owning element's
// page": kept where the new parent's page is the deleted element's, written as an MCR naming the page
// otherwise. The new parent's own `/Pg` is never changed.
func TestAnIntegerMCIDStaysAnIntegerOnlyOnTheNewParentsPage(t *testing.T) {
	out := mustApply(t, deleteFixture(false, false), structEdit{kind: editDelete, elem: 21})
	ctx, k := rawKids(t, out, 8)
	mcr, _ := ctx.DereferenceDict(k[1])
	if _, isInt := k[1].(types.Integer); isInt || mcr == nil || *mcr.NameEntry("Type") != "MCR" || !refTo(mcr["Pg"], 3) || *mcr.IntEntry("MCID") != 1 {
		t.Errorf("under a parent that names no page the integer was written %v — want an MCR naming page object 3 and MCID 1", k[1])
	}
	if _, doc := rawElement(t, out, 8); doc["Pg"] != nil {
		t.Errorf("the new parent was given /Pg %v", doc["Pg"])
	}

	same := mustApply(t, deleteFixture(true, false), structEdit{kind: editDelete, elem: 21})
	_, k = rawKids(t, same, 8)
	if k[1] != types.Integer(1) {
		t.Errorf("under a parent on the same page the integer was written %v — want the integer 1", k[1])
	}
	if _, doc := rawElement(t, same, 8); !refTo(doc["Pg"], 3) {
		t.Errorf("the new parent's /Pg reads %v, want it as it was", doc["Pg"])
	}
	requireConsistent(t, "a delete under a parent on the same page", same)
}

// TestAnIntegerMCIDIsNotLeftUnderAParentThatOnlyInheritsThePage — the new parent must NAME the page for an
// integer to stay one. Element 22 names no page and reads 21's here; ISO 32000-1 hands no page from element
// to element, so an integer left under it is content another reader cannot place.
func TestAnIntegerMCIDIsNotLeftUnderAParentThatOnlyInheritsThePage(t *testing.T) {
	out := mustApply(t, deleteFixture(false, false),
		structEdit{kind: editMove, elem: 20, parent: 22, index: -1}, structEdit{kind: editDelete, elem: 20})
	requireConsistent(t, "a delete under a parent that names no page", out)
	ctx, k := rawKids(t, out, 22)
	if len(k) != 2 {
		t.Fatalf("element 22's /K reads %v", k)
	}
	mcr, _ := ctx.DereferenceDict(k[1])
	if _, isInt := k[1].(types.Integer); isInt || mcr == nil || !refTo(mcr["Pg"], 3) || *mcr.IntEntry("MCID") != 0 {
		t.Errorf("under a parent that only inherits page object 3 the integer was written %v — want an MCR naming it", k[1])
	}
	if got := byID(viewOf(t, out))[22]; got.text != "EpsilonAlpha" {
		t.Errorf("element 22 reads %q", got.text)
	}
}

// TestMarkedContentAndObjectReferencesMoveWithTheirPage — an MCR that named its page is moved as it is; one
// that relied on the deleted element's, and an OBJR that did, are written with that page.
func TestMarkedContentAndObjectReferencesMoveWithTheirPage(t *testing.T) {
	out := mustApply(t, deleteFixture(false, false),
		structEdit{kind: editDelete, elem: 21}, structEdit{kind: editDelete, elem: 23}, structEdit{kind: editDelete, elem: 24})
	requireConsistent(t, "three deletes", out)
	ctx, k := rawKids(t, out, 8)
	if len(k) != 9 {
		t.Fatalf("the Document's /K reads %v", k)
	}
	dict := func(i int) types.Dict {
		d, _ := ctx.DereferenceDict(k[i])
		if d == nil {
			t.Fatalf("/K entry %d is %v", i, k[i])
		}
		return d
	}
	if d := dict(3); *d.NameEntry("Type") != "MCR" || !refTo(d["Pg"], 3) || *d.IntEntry("MCID") != 2 {
		t.Errorf("the MCR that named no page was written %v — want it naming page object 3", d)
	}
	if d := dict(4); *d.NameEntry("Type") != "MCR" || !refTo(d["Pg"], 10) || *d.IntEntry("MCID") != 0 {
		t.Errorf("the MCR that named page object 10 was written %v", d)
	}
	if d := dict(6); *d.NameEntry("Type") != "OBJR" || !refTo(d["Obj"], 40) || !refTo(d["Pg"], 3) {
		t.Errorf("the OBJR was written %v — want the annotation, and the page the deleted Link named", d)
	}
	if d := dict(7); *d.NameEntry("Type") != "MCR" || !refTo(d["Pg"], 12) || !refTo(d["Stm"], 60) {
		t.Errorf("the MCR into the form was written %v", d)
	}
	v := viewOf(t, out)
	if got := byID(v)[8]; got.text != "AlphaBetaEpsilonGammaZetaDeltaThetaEta" {
		t.Errorf("the Document reads %q", got.text)
	}
}

// TestEverySlotThatNamedTheDeletedElementNamesItsParent — the `/ParentTree`, asserted on the owners
// themselves: each page row's slot, the form XObject's row an MCR's `/Stm` reaches, and the annotation's
// single entry. Slots other elements own are left.
func TestEverySlotThatNamedTheDeletedElementNamesItsParent(t *testing.T) {
	for _, nested := range []bool{false, true} {
		src := deleteFixture(false, nested)
		before, _ := slotOwners(t, src)
		if !equalInts(before[0], []int{20, 21, 21, 0, 22}) {
			t.Fatalf("setup: page 1's row reads %v", before[0])
		}
		out := mustApply(t, src,
			structEdit{kind: editDelete, elem: 21}, structEdit{kind: editDelete, elem: 23}, structEdit{kind: editDelete, elem: 24})
		requireConsistent(t, "the deletes", out)
		rows, singles := slotOwners(t, out)
		if !equalInts(rows[0], []int{20, 8, 8, 0, 22}) {
			t.Errorf("nested=%v: page 1's row reads %v — want [20 8 8 0 22]", nested, rows[0])
		}
		if !equalInts(rows[1], []int{8, 30}) {
			t.Errorf("nested=%v: page 2's row reads %v — want [8 30]", nested, rows[1])
		}
		if !equalInts(rows[3], []int{8}) {
			t.Errorf("nested=%v: the form XObject's row reads %v — want [8]", nested, rows[3])
		}
		if singles[2] != 8 {
			t.Errorf("nested=%v: the annotation's entry names %d, want the Document", nested, singles[2])
		}
		if !nested {
			continue
		}
		// The row written as its own object is updated where it lives, and the tree stays nested.
		ctx, pt := rawElement(t, out, 9)
		if _, has := pt["Nums"]; has || pt["Kids"] == nil {
			t.Errorf("the nested /ParentTree was rewritten flat: %v", pt)
		}
		leaf, _ := ctx.DereferenceDict(*types.NewIndirectRef(50, 0))
		nums, _ := ctx.DereferenceArray(leaf["Nums"])
		if _, still := nums[1].(types.IndirectRef); !still {
			t.Errorf("the indirect row was replaced by %v rather than updated where it lives", nums[1])
		}
	}
}

// TestASlotAnotherElementOwnsIsNotTakenOver — a row that already disagrees with the tree is not repaired
// by a delete: the slot the deleted element claimed and never owned keeps naming who it named.
func TestASlotAnotherElementOwnsIsNotTakenOver(t *testing.T) {
	src := bytes.Replace(deleteFixture(false, false), []byte("[20 0 R 21 0 R 21 0 R null 22 0 R]"), []byte("[20 0 R 21 0 R 30 0 R null 22 0 R]"), 1)
	if _, defects := checkTree(t, src); len(defects) != 1 || !strings.HasPrefix(defects[0].key, "mcid-owner ") {
		t.Fatalf("setup: the fixture's defects read %v, want one wrong owner", defects)
	}
	_, err := applyStructEdits(src, []structEdit{{kind: editDelete, elem: 21}})
	if err == nil || !strings.Contains(err.Error(), "contradicting itself") {
		t.Errorf("a delete that hands on content the tree already disagreed about: err = %v — want the batch refused, since the claim would now be the parent's", err)
	}
	ctx := parsed(t, src)
	tree, terr := readStructTree(ctx, livePageObjects(ctx))
	if terr != nil {
		t.Fatal(terr)
	}
	if err := passSlot(ctx, tree, 0, 2, 21, *types.NewIndirectRef(8, 0)); err != nil {
		t.Fatal(err)
	}
	if rows, _ := parentTreeEntries(ctx, tree); !equalInts(rows[0], []int{20, 21, 30, 0, 22}) {
		t.Errorf("a slot naming element 30 was handed on from element 21: the row reads %v", rows[0])
	}
	// The same for an annotation's single entry: it names the Link, so it is not the section's to hand on.
	passSingle(ctx, tree, 2, 21, *types.NewIndirectRef(8, 0))
	if _, singles := parentTreeEntries(ctx, tree); singles[2] != 23 {
		t.Errorf("an entry naming element 23 was handed on from element 21: it names %d", singles[2])
	}
	// And a page's row is not an annotation's entry, whatever its first slot names.
	passSingle(ctx, tree, 1, 21, *types.NewIndirectRef(8, 0))
	if rows, _ := parentTreeEntries(ctx, tree); !equalInts(rows[1], []int{21, 30}) {
		t.Errorf("a page's row was written as a single entry: it reads %v", rows[1])
	}
}

// idFixture is a table whose two header cells carry identifiers that a data cell's `/Headers` names. nested
// writes the `/IDTree` as two leaves under `/Kids`.
func idFixture(nested bool) []byte {
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] /IDTree 30 0 R >>",
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [10 0 R 11 0 R] >>",
		10: "<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R 13 0 R] >>",
		11: "<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R 15 0 R] >>",
		12: "<< /Type /StructElem /S /TH /P 10 0 R /ID (h1) /K [16 0 R] >>",
		13: "<< /Type /StructElem /S /TH /P 10 0 R /ID (h2) >>",
		14: "<< /Type /StructElem /S /TD /P 11 0 R /A << /O /Table /Headers [(h1) (h2)] >> >>",
		15: "<< /Type /StructElem /S /TD /P 11 0 R /A [<< /O /Layout /Placement /Block >> << /O /Table /ColSpan 2 /Headers [(h1)] >>] >>",
		16: "<< /Type /StructElem /S /P /P 12 0 R >>",
		30: "<< /Names [(h1) 12 0 R (h2) 13 0 R] >>",
	}
	if nested {
		objs[30] = "<< /Kids [31 0 R 32 0 R] >>"
		objs[31] = "<< /Limits [(h1) (h1)] /Names [(h1) 12 0 R] >>"
		objs[32] = "<< /Limits [(h2) (h2)] /Names 33 0 R >>"
		objs[33] = "[(h2) 13 0 R]"
	}
	return assembleFixture(objs)
}

// idTreeOwners is the written `/IDTree` flattened: identifier → the object it names. nil for no tree.
func idTreeOwners(t *testing.T, pdf []byte) map[string]int {
	t.Helper()
	ctx, root := rawElement(t, pdf, 7)
	if root["IDTree"] == nil {
		return nil
	}
	out := map[string]int{}
	var walk func(o types.Object)
	walk = func(o types.Object) {
		d, _ := ctx.DereferenceDict(o)
		names, _ := ctx.DereferenceArray(d["Names"])
		for i := 0; i+1 < len(names); i += 2 {
			s, _, _ := byteString(ctx, names[i])
			out[s] = names[i+1].(types.IndirectRef).ObjectNumber.Value()
		}
		kids, _ := ctx.DereferenceArray(d["Kids"])
		for _, k := range kids {
			walk(k)
		}
	}
	walk(root["IDTree"])
	return out
}

// TestADeletedElementsIdentifierGoesWithIt — its `/IDTree` entry is removed and its name is taken out of
// every `/Headers`; the other header cell's identifier, entry and name stay. When the last identifier
// goes, the `/IDTree` goes and a `/Headers` naming nothing is removed.
func TestADeletedElementsIdentifierGoesWithIt(t *testing.T) {
	for _, nested := range []bool{false, true} {
		src := idFixture(nested)
		if got := byID(viewOf(t, src)); len(got[14].headers) != 2 || len(got[15].headers) != 1 {
			t.Fatalf("setup: the cells read %d and %d headers", len(got[14].headers), len(got[15].headers))
		}
		out := mustApply(t, src, structEdit{kind: editDelete, elem: 12})
		if err := Validate(out); err != nil {
			t.Errorf("nested=%v: the written document does not validate: %v", nested, err)
		}
		if got := idTreeOwners(t, out); !reflect.DeepEqual(got, map[string]int{"h2": 13}) {
			t.Errorf("nested=%v: the /IDTree reads %v — want only h2, naming element 13", nested, got)
		}
		v := viewOf(t, out)
		ids := byID(v)
		if h := ids[14].headers; len(h) != 1 || v.elements[h[0]].id != 13 {
			t.Errorf("nested=%v: the first cell's headers read %v — want only element 13", nested, h)
		}
		ctx, raw := rawElement(t, out, 14)
		attr, _ := ctx.DereferenceDict(raw["A"])
		if names, _ := ctx.DereferenceArray(attr["Headers"]); len(names) != 1 {
			t.Errorf("nested=%v: the first cell's /Headers is written %v — want (h2) alone", nested, attr["Headers"])
		}
		if got := ids[15]; len(got.headers) != 0 || got.colSpan != 2 {
			t.Errorf("nested=%v: the second cell reads headers %v and a span of %d — want no header and its span of 2 kept", nested, got.headers, got.colSpan)
		}
		ctx, raw = rawElement(t, out, 15)
		if arr, _ := ctx.DereferenceArray(raw["A"]); len(arr) != 2 {
			t.Errorf("nested=%v: the second cell's /A reads %v — want its Layout object and its Table one", nested, raw["A"])
		} else if table, _ := ctx.DereferenceDict(arr[1]); table["Headers"] != nil {
			t.Errorf("nested=%v: a /Headers naming nothing was left: %v", nested, table)
		}
		// The deleted header's own kid took its place in the row.
		if got := kidIDs(v, ids[10]); !equalInts(got, []int{16, 13}) {
			t.Errorf("nested=%v: the row's kids read %v, want [16 13]", nested, got)
		}

		both := mustApply(t, out, structEdit{kind: editDelete, elem: 13})
		if err := Validate(both); err != nil {
			t.Errorf("nested=%v: with both identifiers gone the document does not validate: %v", nested, err)
		}
		if got := idTreeOwners(t, both); got != nil {
			t.Errorf("nested=%v: with no identifier left the /IDTree still reads %v", nested, got)
		}
		ctx, raw = rawElement(t, both, 14)
		if attr, _ := ctx.DereferenceDict(raw["A"]); attr["Headers"] != nil {
			t.Errorf("nested=%v: the first cell still carries /Headers %v", nested, attr["Headers"])
		}
	}
}

// TestAnIdentifierAnotherElementAlsoCarriesKeepsItsNameInHeaders — two elements with one identifier: the
// `/IDTree` entry names the other, and is kept, and so is the name in `/Headers`.
func TestAnIdentifierAnotherElementAlsoCarriesKeepsItsNameInHeaders(t *testing.T) {
	src := bytes.Replace(idFixture(false), []byte("/ID (h2)"), []byte("/ID (h1)"), 1)
	src = bytes.Replace(src, []byte("[(h1) 12 0 R (h2) 13 0 R]"), []byte("[(h1) 13 0 R]"), 1)
	out := mustApply(t, src, structEdit{kind: editDelete, elem: 12})
	if got := idTreeOwners(t, out); !reflect.DeepEqual(got, map[string]int{"h1": 13}) {
		t.Errorf("the /IDTree reads %v — the entry names the other element and is not this one's to remove", got)
	}
	v := viewOf(t, out)
	if h := byID(v)[15].headers; len(h) != 1 || v.elements[h[0]].id != 13 {
		t.Errorf("the cell's headers read %v — want the identifier still named, now reaching element 13", h)
	}
}

// TestADeleteThatWouldBreakTheTreeIsRefused — each refusal is ErrTagsReview with its own sentence, naming
// the element and what to do instead.
func TestADeleteThatWouldBreakTheTreeIsRefused(t *testing.T) {
	emptyOnly := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4: "<< /Length 1 >>\nstream\n \nendstream",
		7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8: "<< /Type /StructElem /S /Div /P 7 0 R /K [] >>",
	})
	underInline := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Sect /P 7 0 R /K [<< /S /P /K [9 0 R 10 0 R] >>] >>",
		9:  "<< /Type /StructElem /S /Span /P 8 0 R >>",
		10: "<< /Type /StructElem /S /Span /P 8 0 R >>",
	})
	for _, c := range []struct {
		name string
		src  []byte
		elem int
		want error
		says []string
	}{
		{"a top-level element that holds content", taggedFixture(), 8, ErrTagsReview, []string{"element 8", "top of the structure tree", "change its type", "decoration"}},
		{"a top-level element holding an element and content", bytes.Replace(deleteFixture(false, false), []byte("/K [20 0 R 21 0 R 23 0 R 24 0 R 30 0 R]"), []byte("/Pg 3 0 R /K [20 0 R 21 0 R 23 0 R 24 0 R 30 0 R << /Type /MCR /MCID 3 >>]"), 1), 8, ErrTagsReview, []string{"element 8", "top of the structure tree"}},
		{"the last element of the tree", emptyOnly, 8, ErrTagsReview, []string{"element 8", "last element", "remove all tags"}},
		{"an element inside one written inline", underInline, 9, ErrTagsReview, []string{"element 9", "written inline", "change element 9's type"}},
		{"an element written inline", editFixture(), 0, ErrTagsReview, []string{"written inline has no object number"}},
		{"an element the tree does not have", editFixture(), 99, ErrTagsStale, []string{"element 99"}},
	} {
		_, err := applyStructEdits(c.src, []structEdit{{kind: editDelete, elem: c.elem}})
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
			continue
		}
		for _, s := range c.says {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("%s: the refusal reads %q and does not say %q", c.name, err, s)
			}
		}
	}
	// The same element is deleted once it is no longer what was refused: an empty Div beside another element.
	out := mustApply(t, emptyOnly, structEdit{kind: editCreate, value: "P"}, structEdit{kind: editDelete, elem: 8})
	if v := viewOf(t, out); len(v.elements) != 1 || v.elements[0].kind != "P" {
		t.Errorf("with another element beside it the empty Div was not deleted: %+v", v.elements)
	}
	// And a top-level element that holds only elements goes, even as the root's only kid.
	if v := viewOf(t, mustApply(t, editFixture(), structEdit{kind: editDelete, elem: 8})); len(v.elements) != 4 || v.elements[0].parent != -1 || v.elements[0].id != 0 {
		t.Errorf("the only top-level element, holding three elements, was not replaced by them: %+v", v.elements)
	}
}

// TestADeleteKeepsEveryWordAndEveryContentStream — the test that says "content kept": every element that
// remains reads the text it read, the deleted element's text reads under its parent in the place it had,
// and no page's content, and no form XObject's, changes by a byte. A create changes none either.
func TestADeleteKeepsEveryWordAndEveryContentStream(t *testing.T) {
	for _, documentPg := range []bool{false, true} {
		src := deleteFixture(documentPg, false)
		before := viewOf(t, src)
		wantText, streams := textByElement(before), contentStreams(t, src)
		if wantText[8] != "AlphaBetaEpsilonGammaZetaDeltaThetaEta" || wantText[21] != "BetaEpsilonGammaZetaDelta" || len(streams) != 4 { // three pages and the one form a page draws
			t.Fatalf("setup: the Document reads %q, the section %q, over %d streams", wantText[8], wantText[21], len(streams))
		}
		if byID(before)[8].marked {
			t.Fatal("setup: the Document already owns content directly")
		}
		for _, target := range []int{21, 23, 24, 20, 22} {
			out := mustApply(t, src, structEdit{kind: editDelete, elem: target})
			requireConsistent(t, fmt.Sprintf("deleting %d", target), out)
			after := viewOf(t, out)
			got := textByElement(after)
			if _, still := got[target]; still || len(got) != len(wantText)-1 {
				t.Errorf("documentPg=%v, deleting %d: %d elements remain, the deleted one among them: %v", documentPg, target, len(got), still)
			}
			for id, text := range wantText {
				if id != target && got[id] != text {
					t.Errorf("documentPg=%v, deleting %d: element %d read %q and now reads %q", documentPg, target, id, text, got[id])
				}
			}
			for _, e := range after.elements {
				if e.id == 0 && e.text != "Delta" {
					t.Errorf("documentPg=%v, deleting %d: the inline Span reads %q", documentPg, target, e.text)
				}
			}
			if target == 21 && !byID(after)[8].marked {
				t.Errorf("documentPg=%v: the Document does not own the deleted section's content directly", documentPg)
			}
			if !reflect.DeepEqual(contentStreams(t, out), streams) {
				t.Errorf("documentPg=%v, deleting %d: a content stream changed", documentPg, target)
			}
		}
		made := mustApply(t, src, structEdit{kind: editCreate, value: "Sect", parent: 8, index: 1})
		if !reflect.DeepEqual(contentStreams(t, made), streams) {
			t.Errorf("documentPg=%v: a create changed a content stream", documentPg)
		}
		if got := textByElement(viewOf(t, made)); len(got) != len(wantText)+1 {
			t.Errorf("documentPg=%v: after a create %d elements read, want one more than %d", documentPg, len(got), len(wantText))
		} else {
			for id, text := range wantText {
				if got[id] != text {
					t.Errorf("documentPg=%v: after a create element %d reads %q, not %q", documentPg, id, got[id], text)
				}
			}
		}
	}
}

// TestADeletedElementIsNotWritten — nothing lists it, so nothing writes it: the object number it had is no
// structure element in the output.
func TestADeletedElementIsNotWritten(t *testing.T) {
	out := mustApply(t, deleteFixture(false, false), structEdit{kind: editDelete, elem: 21})
	ctx := parsed(t, out)
	if d, _ := ctx.DereferenceDict(*types.NewIndirectRef(21, 0)); d != nil && d.NameEntry("S") != nil {
		t.Errorf("object 21 is still written as %v", d)
	}
}

// TestADeletedElementAStraySlotStillNamesClaimsNothing — a `/ParentTree` slot the deleted element never
// claimed still names it, so its object is still written. It is written holding nothing: its old kids have
// one owner, the parent, and not two.
func TestADeletedElementAStraySlotStillNamesClaimsNothing(t *testing.T) {
	src := bytes.Replace(deleteFixture(false, false), []byte("1 [21 0 R 30 0 R]"), []byte("1 [21 0 R 30 0 R 21 0 R]"), 1)
	if _, defects := checkTree(t, src); len(defects) != 0 {
		t.Fatalf("setup: the fixture's defects read %v", defects)
	}
	out := mustApply(t, src, structEdit{kind: editDelete, elem: 21})
	requireConsistent(t, "the delete", out)
	rows, _ := slotOwners(t, out)
	if !equalInts(rows[1], []int{8, 30, 21}) {
		t.Fatalf("page 2's row reads %v — want the claimed slot handed on and the stray one left", rows[1])
	}
	ctx := parsed(t, out)
	left, _ := ctx.DereferenceDict(*types.NewIndirectRef(21, 0))
	if left == nil {
		t.Fatal("setup: the stray slot no longer keeps object 21 in the document, so this proves nothing")
	}
	if k, has := left["K"]; has {
		t.Errorf("the deleted element is still written with /K %v — its old kids have two owners", k)
	}
	if got := textByElement(viewOf(t, out)); got[8] != "AlphaBetaEpsilonGammaZetaDeltaThetaEta" {
		t.Errorf("the Document reads %q", got[8])
	}
}

// TestAKidAnotherParentListsFirstKeepsItsParent — a tree that lists one element under two parents: the
// reader gives it to the first. Deleting the second moves the listing and leaves the element's `/P` and page
// as they are.
func TestAKidAnotherParentListsFirstKeepsItsParent(t *testing.T) {
	src := bytes.Replace(deleteFixture(false, false), []byte("/K [<< /Type /MCR /Pg 12 0 R /Stm 60 0 R /MCID 0 >>] >>"),
		[]byte("/Pg 10 0 R /K [<< /Type /MCR /Pg 12 0 R /Stm 60 0 R /MCID 0 >> 22 0 R] >>"), 1)
	v := viewOf(t, src)
	if p := byID(v)[22]; v.elements[p.parent].id != 21 {
		t.Fatalf("setup: element 22 reads under %d", v.elements[p.parent].id)
	}
	out := mustApply(t, src, structEdit{kind: editDelete, elem: 24})
	_, raw := rawElement(t, out, 22)
	if !refTo(raw["P"], 21) || raw["Pg"] != nil {
		t.Errorf("element 22 is written with /P %v and /Pg %v — it was the first parent's, and neither was this delete's to change", raw["P"], raw["Pg"])
	}
	if got := byID(viewOf(t, out))[22]; got.text != "Epsilon" || got.page != 1 {
		t.Errorf("element 22 reads %q on page %d", got.text, got.page)
	}
}

// TestCreateAndDeleteOnAProducersTree — LibreOffice's own tree: a wrapper created and filled by moves, then
// deleted again, reads as the tree read at the start; a list deleted hands its items to the Document; and
// every element keeps its text and every content stream its bytes throughout.
func TestCreateAndDeleteOnAProducersTree(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("NOTE: LibreOffice is not installed, so there is no producer's tree to create in and delete from; the hand-built cases still run")
	}
	src := truthCorpus(t)[0].pdf // Document › H1 P H2 P L P
	v := viewOf(t, src)
	top := kidIDs(v, v.elements[0])
	doc := v.elements[0].id
	if v.elements[0].kind != "Document" || len(top) != 6 {
		t.Fatalf("setup: the root reads %s with %d kids", v.elements[0].kind, len(top))
	}
	wantText, streams := textByElement(v), contentStreams(t, src)
	shape := func(v structureView) string {
		var b strings.Builder
		for _, e := range v.elements {
			fmt.Fprintf(&b, "%d<%d:%s:%q ", e.id, e.parent, e.kind, e.text)
		}
		return b.String()
	}
	same := func(what string, out []byte, gone int) structureView {
		t.Helper()
		requireConsistent(t, what, out)
		if err := Validate(out); err != nil {
			t.Errorf("%s: the document does not validate: %v", what, err)
		}
		after := viewOf(t, out)
		got := textByElement(after)
		for id, text := range wantText {
			if id != gone && got[id] != text {
				t.Errorf("%s: element %d read %q and now reads %q", what, id, text, got[id])
			}
		}
		if !reflect.DeepEqual(contentStreams(t, out), streams) {
			t.Errorf("%s: a content stream changed", what)
		}
		return after
	}

	// A wrapper after the first heading, the paragraph moved into it.
	created := mustApply(t, src, structEdit{kind: editCreate, value: "Sect", parent: doc, index: 1})
	cv := same("a create", created, -1)
	wrapper := kidIDs(cv, cv.elements[0])[1]
	if _, known := wantText[wrapper]; known || wrapper == 0 {
		t.Fatalf("the Document's second kid reads %d, which is not a new element", wrapper)
	}
	filled := mustApply(t, created, structEdit{kind: editMove, elem: top[1], parent: wrapper, index: -1})
	fv := same("a move into the new element", filled, -1)
	if got := byID(fv)[wrapper]; got.text != wantText[top[1]] || !equalInts(kidIDs(fv, got), []int{top[1]}) {
		t.Errorf("the new element reads %q with kids %v — want the paragraph it was given", got.text, kidIDs(fv, got))
	}
	back := mustApply(t, filled, structEdit{kind: editDelete, elem: wrapper})
	if bv := same("deleting the new element", back, -1); shape(bv) != shape(v) {
		t.Errorf("created, filled and deleted, the tree reads\n%s\nand read\n%s", shape(bv), shape(v))
	}

	// The list: its items take its place under the Document.
	list := top[4]
	items := kidIDs(v, byID(v)[list])
	lv := same("deleting the list", mustApply(t, src, structEdit{kind: editDelete, elem: list}), list)
	want := append(append(append([]int{}, top[:4]...), items...), top[5])
	if got := kidIDs(lv, lv.elements[0]); !equalInts(got, want) {
		t.Errorf("with the list deleted the Document's kids read %v, want %v", got, want)
	}

	// A paragraph that owns content: the Document owns it afterwards, in the paragraph's place.
	pv := same("deleting a paragraph", mustApply(t, src, structEdit{kind: editDelete, elem: top[1]}), top[1])
	if d := pv.elements[0]; !d.marked || d.text != v.elements[0].text {
		t.Errorf("with a paragraph deleted the Document reads %q (owning content directly: %v), and read %q", d.text, d.marked, v.elements[0].text)
	}
}

// TestCreateAndDeleteAddNoUA1Failure — veraPDF's verdict on the producer's document, before and after: a
// wrapper created and filled, and a wrapper deleted, fail exactly the clauses the document failed.
func TestCreateAndDeleteAddNoUA1Failure(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so ADR-124's ua1 differential is UNCHECKED in this run.")
	}
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is absent, so there is no producer's document to compare.")
	}
	src := truthCorpus(t)[0].pdf
	v := viewOf(t, src)
	top := kidIDs(v, v.elements[0])
	doc := v.elements[0].id

	created := mustApply(t, src, structEdit{kind: editCreate, value: "Sect", parent: doc, index: 1})
	cv := viewOf(t, created)
	wrapper := kidIDs(cv, cv.elements[0])[1]
	filled := mustApply(t, created,
		structEdit{kind: editMove, elem: top[1], parent: wrapper, index: -1},
		structEdit{kind: editMove, elem: top[2], parent: wrapper, index: -1})
	docs := map[string][]byte{
		"original.pdf": src,
		// An element created and left empty.
		"created.pdf": created,
		// A wrapper created and two elements moved into it.
		"filled.pdf": filled,
		// That wrapper deleted again: its kids back under the Document.
		"unwrapped.pdf": mustApply(t, filled, structEdit{kind: editDelete, elem: wrapper}),
		// The Document itself deleted: a top-level wrapper, its kids handed to the root.
		"no-document.pdf": mustApply(t, src, structEdit{kind: editDelete, elem: doc}),
		// A paragraph deleted: its marked content owned by the Document.
		"no-paragraph.pdf": mustApply(t, src, structEdit{kind: editDelete, elem: top[1]}),
	}
	dir := t.TempDir()
	var files []string
	for n, b := range docs {
		f := filepath.Join(dir, n)
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	orig := cl["original.pdf"]
	if orig == nil {
		t.Fatal("veraPDF could not validate the producer's document, so there is no differential")
	}
	for name := range docs {
		got := cl[name]
		if got == nil {
			t.Errorf("veraPDF could not validate %s", name)
			continue
		}
		if !reflect.DeepEqual(got, orig) {
			t.Errorf("%s fails %v and the producer's document fails %v", name, sortedClauses(got), sortedClauses(orig))
		}
	}
	t.Logf("the producer's document fails %v, and so does every edited one", sortedClauses(orig))
}
