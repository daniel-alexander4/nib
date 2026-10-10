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

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Giving every inline element an object number — ADR-126.

// promoteFixture is a tree with inline elements of every shape, over three pages:
//
//	8 Document
//	  20 P        page 1, MCID 0                                           "Alpha"
//	  A  Sect     INLINE, /Pg page 1, no /P. Holds MCID 1 "Beta", B, element 22, an MCR naming page 2
//	              (MCID 0 "Eta"), an MCR into the form XObject at 60 drawn on page 3 (MCID 0 "Theta"), and an
//	              OBJR for the annotation at 40 (/StructParent 2)
//	    B  P      INLINE under an inline element, no page of its own: MCID 2 "Gamma"
//	    22 P      indirect, /P 8 0 R — its grandparent — MCID 3 "Delta"
//	  30 Table
//	    TR        INLINE
//	      TH      INLINE, MCID 4 "Epsilon"
//	      31 TD   indirect, /P 30 0 R, MCID 5 "Zeta"
//
// Page 1's row names 20, 22 and 31 and holds a null for every MCID an inline element owns; page 2's row and
// the form's row are one null each; the annotation's key has no entry. nested writes the `/ParentTree`
// under `/Kids` with page 1's row as an INDIRECT array.
func promoteFixture(nested bool) []byte {
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
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 10 0 R 12 0 R] /Count 3 >>",
		3:  page(4, "/StructParents 0 /Annots [40 0 R]"),
		4:  stream("", marked("Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta")),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 4 >>",
		8:  "<< /Type /StructElem /S /Document /P 7 0 R /K [20 0 R " + promoteFixtureSect + " 30 0 R] >>",
		9:  "<< /Nums [0 [20 0 R null null 22 0 R null 31 0 R] 1 [null] 3 [null]] >>",
		10: page(11, "/StructParents 1"),
		11: stream("", marked("Eta")),
		12: page(13, ""),
		13: stream("", "/Fm0 Do\n"),
		20: "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K [0] >>",
		22: "<< /Type /StructElem /S /P /P 8 0 R /K [3] >>",
		30: "<< /Type /StructElem /S /Table /P 8 0 R /K [<< /S /TR /K [<< /S /TH /Pg 3 0 R /K [4] >> 31 0 R] >>] >>",
		31: "<< /Type /StructElem /S /TD /P 30 0 R /Pg 3 0 R /K [5] >>",
		40: "<< /Type /Annot /Subtype /Link /Rect [72 690 200 712] /Border [0 0 0] /StructParent 2 /Contents (A link) >>",
		60: stream("/Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /StructParents 3",
			"/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 600 Td (Theta) Tj ET\nEMC\n"),
	}
	if nested {
		objs[9] = "<< /Kids [50 0 R] >>"
		objs[50] = "<< /Limits [0 3] /Nums [0 51 0 R 1 [null] 3 [null]] >>"
		objs[51] = "[20 0 R null null 22 0 R null 31 0 R]"
	}
	return assembleFixture(objs)
}

// promoteFixtureSect is the inline Sect of promoteFixture, as written inside the Document's `/K`.
const promoteFixtureSect = "<< /S /Sect /Pg 3 0 R /K [1 << /S /P /K [2] >> 22 0 R << /Type /MCR /Pg 10 0 R /MCID 0 >> " +
	"<< /Type /MCR /Pg 12 0 R /Stm 60 0 R /MCID 0 >> << /Type /OBJR /Obj 40 0 R >>] >>"

// reading is a view with the object numbers taken out: every element's place, type, text, page and parent,
// and whether it owns content — what a promotion must leave exactly as it was.
func reading(v structureView) []string {
	out := make([]string, len(v.elements))
	for i, e := range v.elements {
		out[i] = fmt.Sprintf("%d under %d: %s (%s) p%d marked=%v kids=%v %q alt=%q/%v scope=%q spans=%d,%d headers=%v rect=%v",
			i, e.parent, e.kind, e.standard, e.page, e.marked, e.kids, e.text, e.alt, e.hasAlt, e.scope, e.colSpan, e.rowSpan, e.headers, e.rect)
	}
	return out
}

// promoted is a promote applied, and the view of the result with its elements by reading position.
func promoted(t *testing.T, src []byte) ([]byte, structureView) {
	t.Helper()
	out := mustApply(t, src, structEdit{kind: editPromote})
	return out, viewOf(t, out)
}

// TestAPromotionNumbersEveryInlineElementAndTheTreeReadsAsItDid — the reading is identical element for
// element, every id is above zero, no content stream changes, and the written tree is consistent and valid.
func TestAPromotionNumbersEveryInlineElementAndTheTreeReadsAsItDid(t *testing.T) {
	for _, nested := range []bool{false, true} {
		src := promoteFixture(nested)
		before := viewOf(t, src)
		if before.unaddressable != 4 || len(before.elements) != 9 {
			t.Fatalf("setup (nested %v): %d element(s), %d inline — want 9 and 4", nested, len(before.elements), before.unaddressable)
		}
		requireConsistent(t, "the fixture", src)
		out, after := promoted(t, src)
		if !reflect.DeepEqual(reading(after), reading(before)) {
			t.Errorf("nested %v: the tree reads\n%s\nand read\n%s", nested, strings.Join(reading(after), "\n"), strings.Join(reading(before), "\n"))
		}
		if after.unaddressable != 0 {
			t.Errorf("nested %v: %d element(s) still read as inline", nested, after.unaddressable)
		}
		seen := map[int]bool{}
		for i, e := range after.elements {
			if e.id <= 0 || seen[e.id] {
				t.Errorf("nested %v: element %d reads id %d — every element needs a number of its own", nested, i, e.id)
			}
			seen[e.id] = true
			if b := before.elements[i]; b.id != 0 && b.id != e.id {
				t.Errorf("nested %v: element %d was object %d and is now %d — an element that had a number keeps it", nested, i, b.id, e.id)
			}
		}
		if !reflect.DeepEqual(contentStreams(t, out), contentStreams(t, src)) {
			t.Errorf("nested %v: a content stream changed", nested)
		}
		requireConsistent(t, "a promotion", out)
		if err := Validate(out); err != nil {
			t.Errorf("nested %v: the promoted document does not validate: %v", nested, err)
		}
	}
}

// TestAPromotedElementTakesItsPlaceInItsParentsKByReference — order unchanged, and the dictionary written
// is the one that was inline: same keys, plus the `/P` it could not have.
func TestAPromotedElementTakesItsPlaceInItsParentsKByReference(t *testing.T) {
	out, after := promoted(t, promoteFixture(false))
	sect := after.elements[2].id
	ctx, kids := rawKids(t, out, 8)
	if len(kids) != 3 || !refTo(kids[0], 20) || !refTo(kids[1], sect) || !refTo(kids[2], 30) {
		t.Fatalf("the Document's /K reads %v — want 20, the promoted Sect (%d), 30, in that order", kids, sect)
	}
	d, err := ctx.DereferenceDict(kids[1])
	if err != nil || d == nil {
		t.Fatal("the promoted Sect is not a dictionary")
	}
	if s := d.NameEntry("S"); s == nil || *s != "Sect" || !refTo(d["Pg"], 3) {
		t.Errorf("the promoted Sect is written %v — its /S and /Pg are the inline dictionary's", d)
	}
	sk, _ := kidsArray(ctx, d)
	if len(sk) != 6 {
		t.Fatalf("the promoted Sect's /K has %d entries, want the 6 it had", len(sk))
	}
	if n, ok := sk[0].(types.Integer); !ok || n.Value() != 1 {
		t.Errorf("its first kid is %v, want the integer MCID 1", sk[0])
	}
	if !refTo(sk[1], after.elements[3].id) || !refTo(sk[2], 22) {
		t.Errorf("its second and third kids are %v and %v — want the promoted P (%d) and element 22", sk[1], sk[2], after.elements[3].id)
	}
	for i, ty := range map[int]string{3: "MCR", 4: "MCR", 5: "OBJR"} {
		kd, _ := sk[i].(types.Dict)
		if n := kd.NameEntry("Type"); n == nil || *n != ty {
			t.Errorf("kid %d is %v, want the %s it was, still written in place", i, sk[i], ty)
		}
	}
}

// TestAPromotedElementNamesItsParentAndItsKidsNameIt — `/P` at every level: the root's Document for the
// Sect, the promoted Sect for the promoted P and for element 22 (whose `/P` named its grandparent), the Table
// for the promoted TR, and the promoted TR for the promoted TH and for element 31.
func TestAPromotedElementNamesItsParentAndItsKidsNameIt(t *testing.T) {
	out, v := promoted(t, promoteFixture(false))
	sect, para, tr, th := v.elements[2].id, v.elements[3].id, v.elements[6].id, v.elements[7].id
	if v.elements[2].kind != "Sect" || v.elements[3].kind != "P" || v.elements[6].kind != "TR" || v.elements[7].kind != "TH" {
		t.Fatalf("setup: the promoted elements read %v", reading(v))
	}
	for _, c := range []struct {
		what        string
		elem, wantP int
	}{
		{"the promoted Sect", sect, 8},
		{"the promoted P inside it", para, sect},
		{"element 22, which named its grandparent", 22, sect},
		{"the promoted TR", tr, 30},
		{"the promoted TH", th, tr},
		{"element 31, which named the Table", 31, tr},
		{"element 20, which no inline element holds", 20, 8},
		{"element 30, which no inline element holds", 30, 8},
	} {
		if _, d := rawElement(t, out, c.elem); !refTo(d["P"], c.wantP) {
			t.Errorf("%s (object %d) is written with /P %v, want %d 0 R", c.what, c.elem, d["P"], c.wantP)
		}
	}
}

// TestATopLevelInlineElementNamesTheRoot — an inline element directly under the root, in a `/K` written as
// one dictionary rather than an array, and an inline element in a `/K` that is an indirect array.
func TestATopLevelInlineElementNamesTheRoot(t *testing.T) {
	src := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K << /S /Document /K 9 0 R >> >>",
		9:  "[10 0 R << /S /Sect >> 11 0 R]",
		10: "<< /Type /StructElem /S /P >>",
		11: "<< /Type /StructElem /S /P /P 7 0 R >>",
	})
	before := viewOf(t, src)
	out, after := promoted(t, src)
	if !reflect.DeepEqual(reading(after), reading(before)) || after.unaddressable != 0 {
		t.Fatalf("the tree reads %v and read %v", reading(after), reading(before))
	}
	doc, sect := after.elements[0].id, after.elements[2].id
	if _, d := rawElement(t, out, doc); !refTo(d["P"], 7) || !refTo(d["K"], 9) {
		t.Errorf("the promoted Document is written with /P %v and /K %v — want the root (7 0 R) and the array object it had (9 0 R)", d["P"], d["K"])
	}
	ctx, root := rawElement(t, out, 7)
	if rk, _ := kidsArray(ctx, root); len(rk) != 1 || !refTo(rk[0], doc) {
		t.Errorf("the root's /K reads %v, want the one reference to the promoted Document", root["K"])
	}
	arr, _ := ctx.DereferenceArray(*types.NewIndirectRef(9, 0))
	if len(arr) != 3 || !refTo(arr[0], 10) || !refTo(arr[1], sect) || !refTo(arr[2], 11) {
		t.Errorf("the indirect /K array reads %v — want 10, the promoted Sect (%d), 11, written where it lives", arr, sect)
	}
	for _, kid := range []int{10, 11, sect} {
		if _, d := rawElement(t, out, kid); !refTo(d["P"], doc) {
			t.Errorf("object %d is written with /P %v, want the promoted Document (%d)", kid, d["P"], doc)
		}
	}
}

// TestEveryEmptySlotAPromotedElementOwnsNamesIt — integer MCIDs, an MCR's page row, an MCR's stream row and
// an annotation's entry; a flat tree and a nested one with an indirect row. The slots other elements own are
// as they were.
func TestEveryEmptySlotAPromotedElementOwnsNamesIt(t *testing.T) {
	for _, nested := range []bool{false, true} {
		out, v := promoted(t, promoteFixture(nested))
		sect, para, th := v.elements[2].id, v.elements[3].id, v.elements[7].id
		rows, singles := slotOwners(t, out)
		if want := []int{20, sect, para, 22, th, 31}; !equalInts(rows[0], want) {
			t.Errorf("nested %v: page 1's row reads %v, want %v", nested, rows[0], want)
		}
		if !equalInts(rows[1], []int{sect}) {
			t.Errorf("nested %v: page 2's row reads %v, want the promoted Sect (%d) for the MCR that names page 2", nested, rows[1], sect)
		}
		if !equalInts(rows[3], []int{sect}) {
			t.Errorf("nested %v: the form's row reads %v, want the promoted Sect (%d) for the MCR with a /Stm", nested, rows[3], sect)
		}
		if nested {
			// Key 2 falls inside the nested tree's range, where no entry can be placed without re-sorting a leaf.
			if _, has := singles[2]; has {
				t.Errorf("nested: the annotation's key 2 has an entry (%d) the number tree could not take", singles[2])
			}
			ctx := parsed(t, out)
			if row, err := ctx.DereferenceArray(*types.NewIndirectRef(51, 0)); err != nil || len(row) != 6 || !refTo(row[1], sect) {
				t.Errorf("nested: the indirect row object reads %v — the slots are written where the row lives", row)
			}
			continue
		}
		if singles[2] != sect {
			t.Errorf("the annotation's entry (key 2) names %d, want the promoted Sect (%d)", singles[2], sect)
		}
	}
}

// TestASlotThatNamesAnotherElementIsLeftAndDoesNotRefuseThePromotion — a defect the tree already had, which
// the checker could not ask of an element with no number: the slot keeps its owner, the annotation's entry
// keeps its, and the promotion is applied.
func TestASlotThatNamesAnotherElementIsLeftAndDoesNotRefuseThePromotion(t *testing.T) {
	src := bytes.Replace(promoteFixture(false), []byte("/Nums [0 [20 0 R null null 22 0 R null 31 0 R] 1 [null] 3 [null]]"),
		[]byte("/Nums [0 [20 0 R 20 0 R null 22 0 R null 31 0 R] 1 [null] 2 22 0 R 3 [31 0 R]]"), 1)
	out, v := promoted(t, src)
	sect, para, th := v.elements[2].id, v.elements[3].id, v.elements[7].id
	rows, singles := slotOwners(t, out)
	if want := []int{20, 20, para, 22, th, 31}; !equalInts(rows[0], want) {
		t.Errorf("page 1's row reads %v, want %v — slot 1 named element 20 and still does", rows[0], want)
	}
	if !equalInts(rows[3], []int{31}) || singles[2] != 22 {
		t.Errorf("the form's row reads %v and the annotation's entry %d — each named another element and still does", rows[3], singles[2])
	}
	if !equalInts(rows[1], []int{sect}) {
		t.Errorf("page 2's row reads %v, want the promoted Sect (%d)", rows[1], sect)
	}
	// What the checker now says is the defect that was there: slot 1 names 20 while the Sect claims it.
	_, defects := checkTree(t, out)
	if len(defects) != 1 || !strings.HasPrefix(defects[0].key, "mcid-owner key=0 mcid=1 ") {
		t.Errorf("the promoted tree's defects read %v — want the one conflict the tree already had, now nameable", defects)
	}
}

// TestARowIsGrownOnlySoFar — an id past the end of its row is given a slot, with nulls between; one past
// `promoteMaxSlot` is not, and it does not refuse the promotion: the checker said the id was out of range
// before, about object 0.
func TestARowIsGrownOnlySoFar(t *testing.T) {
	src := bytes.Replace(promoteFixture(false), []byte("<< /S /P /K [2] >>"), []byte("<< /S /P /K [8 2000000000] >>"), 1)
	out, v := promoted(t, src)
	sect, para, th := v.elements[2].id, v.elements[3].id, v.elements[7].id
	rows, _ := slotOwners(t, out)
	if want := []int{20, sect, 0, 22, th, 31, 0, 0, para}; !equalInts(rows[0], want) {
		t.Errorf("page 1's row reads %v, want %v — grown to MCID 8 with nulls between, and no further", rows[0], want)
	}
	if promoteMaxSlot != 1<<16 {
		t.Errorf("promoteMaxSlot is %d — ADR-126 declares 65536", promoteMaxSlot)
	}
	// At the bound and one past it, on a row of six.
	for mcid, want := range map[int]int{promoteMaxSlot: promoteMaxSlot + 1, promoteMaxSlot + 1: 6} {
		src := bytes.Replace(promoteFixture(false), []byte("<< /S /P /K [2] >>"), []byte(fmt.Sprintf("<< /S /P /K [%d] >>", mcid)), 1)
		out, _ := promoted(t, src)
		if rows, _ := slotOwners(t, out); len(rows[0]) != want {
			t.Errorf("an inline element claiming MCID %d leaves page 1's row %d slot(s) long, want %d", mcid, len(rows[0]), want)
		}
	}
}

// TestAKeyWithNoRowIsGivenNone — a page whose `/StructParents` names a key the `/ParentTree` does not have:
// the promotion writes no row for it, and is applied.
func TestAKeyWithNoRowIsGivenNone(t *testing.T) {
	src := bytes.Replace(promoteFixture(false), []byte(" 1 [null] 3 [null]]"), []byte(" 3 [null]]"), 1)
	_, was := checkTree(t, src)
	out, v := promoted(t, src)
	rows, _ := slotOwners(t, out)
	if _, has := rows[1]; has {
		t.Errorf("page 2 was given a row (%v) — a key that named no row names none after", rows[1])
	}
	if !equalInts(rows[3], []int{v.elements[2].id}) {
		t.Errorf("the form's row reads %v, want the promoted Sect", rows[3])
	}
	if _, now := checkTree(t, out); len(now) != len(was) || len(was) != 2 {
		t.Errorf("the tree had %d defect(s) and has %d, want the 2 a missing row is: %v", len(was), len(now), now)
	}
}

// TestAPageWithNoKeyClaimsNoRow — a page with no `/StructParents` has no key, which is not the key -1: a
// malformed tree that holds a row under -1 keeps it as it was.
func TestAPageWithNoKeyClaimsNoRow(t *testing.T) {
	// The third page has no /StructParents, and the Sect's MCR into the form names it.
	src := bytes.Replace(promoteFixture(false), []byte("/Nums [0 ["), []byte("/Nums [-1 [null] 0 ["), 1)
	if rows, _ := slotOwners(t, src); !equalInts(rows[-1], []int{0}) {
		t.Fatalf("setup: the row under -1 reads %v", rows[-1])
	}
	out, v := promoted(t, src)
	rows, _ := slotOwners(t, out)
	if !equalInts(rows[-1], []int{0}) {
		t.Errorf("the row under -1 reads %v — a page with no key was read as the page with key -1", rows[-1])
	}
	if !equalInts(rows[3], []int{v.elements[2].id}) {
		t.Errorf("the form's row reads %v, want the promoted Sect", rows[3])
	}
}

// TestAPromotionIsRefusedWhereThereIsNothingToPromote — and where the edit names something: it takes the
// whole document.
func TestAPromotionIsRefusedWhereThereIsNothingToPromote(t *testing.T) {
	out, _ := promoted(t, promoteFixture(false))
	_, err := applyStructEdits(out, []structEdit{{kind: editPromote}})
	if !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), "no tag in this document is written inline") {
		t.Errorf("a second promotion: err = %v, want ErrTagsReview saying no tag is written inline", err)
	}
	if _, err := applyStructEdits(roleFixture(true, false), []structEdit{{kind: editPromote}}); !errors.Is(err, ErrTagsReview) {
		t.Errorf("a promotion of a tree with no inline element: err = %v, want ErrTagsReview", err)
	}
	src := promoteFixture(false)
	for name, ed := range map[string]structEdit{
		"an element":   {kind: editPromote, elem: 20},
		"a value":      {kind: editPromote, value: "P"},
		"a parent":     {kind: editPromote, parent: 8},
		"header cells": {kind: editPromote, headers: []int{31}},
		"a role":       {kind: editPromote, role: "Custom"},
	} {
		if _, err := applyStructEdits(src, []structEdit{ed}); !errors.Is(err, ErrTagsReview) {
			t.Errorf("a promotion naming %s: err = %v, want ErrTagsReview", name, err)
		}
	}
	if _, err := EditStructure(src, []StructureEdit{{Kind: "promote", Index: -1}}); err != nil {
		t.Errorf("the promote edit as a request sends it (an absent index is -1): %v", err)
	}
}

// TestAfterAPromotionTheElementsAreEdited — the refusals an inline element caused are gone: an element inside
// one is deleted (its content passing to the promoted parent), the promoted element is retyped and moved
// into, and an edit in the SAME batch as the promotion sees the numbered tree.
func TestAfterAPromotionTheElementsAreEdited(t *testing.T) {
	src := promoteFixture(false)
	if _, err := applyStructEdits(src, []structEdit{{kind: editDelete, elem: 22}}); !errors.Is(err, ErrTagsReview) ||
		!strings.Contains(err.Error(), "make inline tags editable") {
		t.Fatalf("deleting an element inside an inline one: err = %v, want ErrTagsReview pointing at the promotion", err)
	}
	out, v := promoted(t, src)
	sect, para := v.elements[2].id, v.elements[3].id

	gone := mustApply(t, out, structEdit{kind: editDelete, elem: 22}, structEdit{kind: editDelete, elem: para})
	requireConsistent(t, "promote then delete", gone)
	gv := viewOf(t, gone)
	if got := byID(gv)[sect]; got.text != "BetaGammaDeltaEtaTheta" || len(got.kids) != 0 {
		t.Errorf("with both paragraphs deleted the promoted Sect reads %q with %d element kid(s), want every word and none", got.text, len(got.kids))
	}
	rows, _ := slotOwners(t, gone)
	if want := []int{20, sect, sect, sect}; !equalInts(rows[0][:4], want) {
		t.Errorf("page 1's row begins %v, want %v — what the deleted elements owned, the promoted Sect owns", rows[0][:4], want)
	}

	edited := mustApply(t, out, structEdit{kind: editRetype, elem: sect, value: "Art"}, structEdit{kind: editMove, elem: 20, parent: sect, index: 0})
	requireConsistent(t, "promote then retype and move", edited)
	if got := byID(viewOf(t, edited))[sect]; got.kind != "Art" || !equalInts(kidIDs(viewOf(t, edited), got), []int{20, para, 22}) {
		t.Errorf("the promoted element reads %s with kids %v, want Art holding 20, %d, 22", got.kind, kidIDs(viewOf(t, edited), got), para)
	}

	// One batch: the delete is resolved against the tree the promotion left.
	batch := mustApply(t, src, structEdit{kind: editPromote}, structEdit{kind: editDelete, elem: 22})
	if _, still := byID(viewOf(t, batch))[22]; still {
		t.Error("a delete in the same batch as the promotion did not happen")
	}
}

// TestAPromotedHeaderCellCanBeNamedAsAHeader — ADR-119's declared gap (a `TH` written inline has no id to
// send) closes: before, the data cell has no header cell an edit can name; after, it names the promoted one.
func TestAPromotedHeaderCellCanBeNamedAsAHeader(t *testing.T) {
	src := promoteFixture(false)
	if _, err := applyStructEdits(src, []structEdit{{kind: editHeaders, elem: 31, headers: []int{0}}}); !errors.Is(err, ErrTagsStale) {
		t.Fatalf("naming an inline header cell: err = %v, want ErrTagsStale — 0 is no element", err)
	}
	out, v := promoted(t, src)
	th := v.elements[7].id
	headed := mustApply(t, out, structEdit{kind: editHeaders, elem: 31, headers: []int{th}}, structEdit{kind: editScope, elem: th, value: "Column"})
	requireConsistent(t, "promote then headers", headed)
	hv := viewOf(t, headed)
	cell := byID(hv)[31]
	if len(cell.headers) != 1 || hv.elements[cell.headers[0]].id != th {
		t.Errorf("the data cell's headers read %v, want the promoted header cell (%d)", cell.headers, th)
	}
	if got := byID(hv)[th]; got.scope != "Column" || got.text != "Epsilon" {
		t.Errorf("the promoted header cell reads scope %q and text %q", got.scope, got.text)
	}
}

// TestADefectAboutAnInlineElementDoesNotReadAsNewOnceItHasANumber — the checker keys a defect by the
// element's object number, 0 for an inline one, so an id with no page and a `/Stm` that is no reference are
// the same defects under a new name after the promotion; a null slot under a claimed id is not one of them.
func TestADefectAboutAnInlineElementDoesNotReadAsNewOnceItHasANumber(t *testing.T) {
	src := bytes.Replace(promoteFixture(false), []byte("<< /S /TH /Pg 3 0 R /K [4] >>"),
		[]byte("<< /S /TH /K [4 << /Type /MCR /Pg 3 0 R /Stm 5 /MCID 4 >>] >>"), 1)
	_, was := checkTree(t, src)
	if len(was) != 2 {
		t.Fatalf("setup: the fixture's defects read %v, want an id with no page and a malformed /Stm", was)
	}
	out, v := promoted(t, src)
	_, now := checkTree(t, out)
	if len(now) != 2 {
		t.Fatalf("the promoted tree's defects read %v, want the two it had", now)
	}
	for _, d := range now {
		if kind, obj := defectSubject(d.key); obj != v.elements[7].id || (kind != "mcid-no-page" && kind != "stm-not-a-reference") {
			t.Errorf("defect %q is not one of the two the tree had, about the promoted TH (%d)", d.key, v.elements[7].id)
		}
	}
	already := map[string]bool{}
	defectsAPromotionUncovers(already, map[int]bool{41: true}, []structDefect{
		{key: "mcid-owner key=0 mcid=1 obj=41"}, {key: "mcid-unowned key=0 mcid=2 obj=41"},
		{key: "mcid-range obj=41 key=0 mcid=9"}, {key: "mcid-owner key=0 mcid=1 obj=20"}, {key: "structparents-missing page=2 key=1"},
	})
	if want := map[string]bool{"mcid-owner key=0 mcid=1 obj=41": true, "mcid-range obj=41 key=0 mcid=9": true}; !reflect.DeepEqual(already, want) {
		t.Errorf("a promotion of object 41 excuses %v, want %v — never a null slot it should have filled, never another element's defect", already, want)
	}
}

// inlineGroupingFixture is a tree whose inline elements own no content: a Document and a Sect written
// inline, grouping two paragraphs that are objects and own the page's two marked-content ids.
func inlineGroupingFixture() []byte {
	body := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (Alpha) Tj ET\nEMC\n/P <</MCID 1>> BDC\nBT /F1 12 Tf 72 680 Td (Beta) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K << /S /Document /K [10 0 R << /S /Sect /K [11 0 R] >>] >> /ParentTree 9 0 R >>",
		9:  "<< /Nums [0 [10 0 R 11 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		11: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [1] >>",
	})
}

// TestAPromotionAddsNoUA1Failure — veraPDF's verdict before and after.
//
// Where the inline elements own no content, the promoted document fails exactly the clauses the document
// failed. Where one owns content it fails FEWER, and never one more: veraPDF finds a marked-content id's
// owner through the `/ParentTree`, no slot can name an inline element, so it read that content as untagged
// (7.1 t3) and its tags as owning nothing — and the promotion is what fills the slots. Measured on veraPDF
// 1.30.2: the clauses that go are 7.1 t3 and 7.2 t4, t8 and t9.
func TestAPromotionAddsNoUA1Failure(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so ADR-126's ua1 differential is UNCHECKED in this run.")
	}
	docs := map[string][]byte{}
	for name, src := range map[string][]byte{"flat": promoteFixture(false), "nested": promoteFixture(true), "grouping": inlineGroupingFixture()} {
		docs[name+"-original.pdf"] = src
		docs[name+"-promoted.pdf"], _ = promoted(t, src)
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
	for _, name := range []string{"flat", "nested", "grouping"} {
		orig, got := cl[name+"-original.pdf"], cl[name+"-promoted.pdf"]
		if orig == nil || got == nil {
			t.Errorf("veraPDF could not validate the %s fixture or its promotion", name)
			continue
		}
		for c := range got {
			if !orig[c] {
				t.Errorf("%s: promoted, the document fails %s, which it did not fail before (%v)", name, c, sortedClauses(orig))
			}
		}
		if name == "grouping" {
			if !reflect.DeepEqual(got, orig) {
				t.Errorf("grouping: promoted, the document fails %v and failed %v — no inline element owned content, so nothing may change", sortedClauses(got), sortedClauses(orig))
			}
			continue
		}
		// The inline elements owned content no slot could give them.
		for _, c := range []string{"7.1 t3", "7.2 t4", "7.2 t8", "7.2 t9"} {
			if !orig[c] || got[c] {
				t.Errorf("%s: clause %s failed before: %v, fails after: %v — want it to fail only while the content had no owner a slot names", name, c, orig[c], got[c])
			}
		}
		if len(orig)-len(got) != 4 {
			t.Errorf("%s: %d clause(s) went, want exactly the four about content with no owner: before %v, after %v", name, len(orig)-len(got), sortedClauses(orig), sortedClauses(got))
		}
	}
}
