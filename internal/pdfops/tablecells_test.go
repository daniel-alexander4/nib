package pdfops

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A table cell's spans and the header cells it names — ADR-119.

// cellFixture is a hand-built tree: a table of a header row (12, 13) and a data row (14, 15), a second
// table with one header cell (22), and a paragraph (30). extra objects replace or add to it.
func cellFixture(extra map[int]string) []byte {
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 20 0 R 30 0 R] >>",
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [10 0 R 11 0 R] >>",
		10: "<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R 13 0 R] >>",
		11: "<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R 15 0 R] >>",
		12: "<< /Type /StructElem /S /TH /P 10 0 R >>",
		13: "<< /Type /StructElem /S /TH /P 10 0 R >>",
		14: "<< /Type /StructElem /S /TD /P 11 0 R >>",
		15: "<< /Type /StructElem /S /TD /P 11 0 R >>",
		20: "<< /Type /StructElem /S /Table /P 7 0 R /K [21 0 R] >>",
		21: "<< /Type /StructElem /S /TR /P 20 0 R /K [22 0 R] >>",
		22: "<< /Type /StructElem /S /TH /P 21 0 R >>",
		30: "<< /Type /StructElem /S /P /P 7 0 R >>",
	}
	for n, body := range extra {
		objs[n] = body
	}
	return assembleFixture(objs)
}

// idsOf reads the `/ID` bytes of the given elements from a written document.
func idsOf(t *testing.T, pdf []byte, objs ...int) []string {
	t.Helper()
	var out []string
	for _, n := range objs {
		ctx, d := rawElement(t, pdf, n)
		id, _, _ := elementID(ctx, d)
		out = append(out, id)
	}
	return out
}

// idTreeNames is the document's `/IDTree` flattened in the order a reader walks it: key bytes → object number.
func idTreeNames(t *testing.T, pdf []byte) (keys []string, objs []int, indirect bool) {
	t.Helper()
	ctx, root := rawElement(t, pdf, 7)
	_, indirect = root["IDTree"].(types.IndirectRef)
	var walk func(o types.Object)
	walk = func(o types.Object) {
		d, err := ctx.DereferenceDict(o)
		if err != nil || d == nil {
			return
		}
		names, _ := ctx.DereferenceArray(d["Names"])
		for i := 0; i+1 < len(names); i += 2 {
			k, _, _ := byteString(ctx, names[i])
			keys = append(keys, k)
			if ir, ok := names[i+1].(types.IndirectRef); ok {
				objs = append(objs, ir.ObjectNumber.Value())
			} else {
				objs = append(objs, 0)
			}
		}
		kids, _ := ctx.DereferenceArray(d["Kids"])
		for _, k := range kids {
			walk(k)
		}
	}
	walk(root["IDTree"])
	return keys, objs, indirect
}

// TestACellsSpansAndHeadersAreWrittenAndReadBack — each edit lands in the cell's Table attribute object,
// the view reads it back, and "" (or no header at all) takes it away again.
func TestACellsSpansAndHeadersAreWrittenAndReadBack(t *testing.T) {
	out := mustApply(t, cellFixture(nil),
		structEdit{kind: editColSpan, elem: 12, value: "2"},
		structEdit{kind: editRowSpan, elem: 13, value: "3"},
		structEdit{kind: editHeaders, elem: 14, headers: []int{12, 13, 12}},
		structEdit{kind: editHeaders, elem: 15, headers: []int{13}})
	requireConsistent(t, "after the cell edits", out)
	v := viewOf(t, out)
	got := byID(v)
	if got[12].colSpan != 2 || got[12].rowSpan != 1 || got[13].rowSpan != 3 || got[13].colSpan != 1 {
		t.Errorf("spans read col %d row %d and col %d row %d, want 2 1 and 1 3", got[12].colSpan, got[12].rowSpan, got[13].colSpan, got[13].rowSpan)
	}
	headed := func(e viewElement) []int {
		var ids []int
		for _, j := range e.headers {
			ids = append(ids, v.elements[j].id)
		}
		return ids
	}
	if h := headed(got[14]); !reflect.DeepEqual(h, []int{12, 13}) {
		t.Errorf("cell 14 is headed by %v, want [12 13] — a header named twice is written once", h)
	}
	if h := headed(got[15]); !reflect.DeepEqual(h, []int{13}) {
		t.Errorf("cell 15 is headed by %v, want [13]", h)
	}
	// The identifiers are the header cells' own, the same in every cell that names them, and in the IDTree.
	if ids := idsOf(t, out, 12, 13, 22); ids[0] == "" || ids[1] == "" || ids[0] == ids[1] || ids[2] != "" {
		t.Errorf("the header cells carry /ID %q — want one each for 12 and 13, different, and none for the cell nothing named", ids)
	}
	keys, objs, indirect := idTreeNames(t, out)
	if !indirect || len(keys) != 2 || !reflect.DeepEqual(keys, idsOf(t, out, 12, 13)) || !reflect.DeepEqual(objs, []int{12, 13}) {
		t.Errorf("the /IDTree (indirect %v) reads %q → %v, want the two header cells' identifiers naming 12 and 13", indirect, keys, objs)
	}
	ctx, td := rawElement(t, out, 15)
	attr, _ := ctx.DereferenceDict(td["A"])
	hs, _ := ctx.DereferenceArray(attr["Headers"])
	if len(hs) != 1 {
		t.Fatalf("cell 15's /Headers reads %v", attr["Headers"])
	}
	if s, _, _ := byteString(ctx, hs[0]); s != idsOf(t, out, 13)[0] {
		t.Errorf("cell 15's /Headers names %q, and header 13's /ID is %q", s, idsOf(t, out, 13)[0])
	}

	cleared := mustApply(t, out,
		structEdit{kind: editColSpan, elem: 12, value: ""},
		structEdit{kind: editRowSpan, elem: 13, value: ""},
		structEdit{kind: editHeaders, elem: 14})
	cv := viewOf(t, cleared)
	back := byID(cv)
	if back[12].colSpan != 1 || back[13].rowSpan != 1 || len(back[14].headers) != 0 || len(back[15].headers) != 1 {
		t.Errorf("after removing: col %d row %d headers %v, and the untouched cell %v", back[12].colSpan, back[13].rowSpan, back[14].headers, back[15].headers)
	}
	tree, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range tree.Elements {
		ve := v.elements[i]
		if e.Headers == nil || e.ColSpan != ve.colSpan || e.RowSpan != ve.rowSpan || !reflect.DeepEqual(e.Headers, append([]int{}, ve.headers...)) {
			t.Errorf("element %d reads spans %d×%d headers %v, the view %d×%d %v", i, e.ColSpan, e.RowSpan, e.Headers, ve.colSpan, ve.rowSpan, ve.headers)
		}
	}
}

// TestACellEditThatDoesNotDescribeTheTableIsRefused — each refusal by its own cause.
func TestACellEditThatDoesNotDescribeTheTableIsRefused(t *testing.T) {
	src := cellFixture(nil)
	for _, c := range []struct {
		name string
		edit structEdit
		want error
		says string
	}{
		{"a column span on a paragraph", structEdit{kind: editColSpan, elem: 30, value: "2"}, ErrTagsReview, "only a table cell"},
		{"a row span on a row", structEdit{kind: editRowSpan, elem: 10, value: "2"}, ErrTagsReview, "only a table cell"},
		{"a span of nothing", structEdit{kind: editColSpan, elem: 12, value: "0"}, ErrTagsReview, "not a number of columns"},
		{"a negative span", structEdit{kind: editRowSpan, elem: 12, value: "-2"}, ErrTagsReview, "not a number of rows"},
		{"a span that is not a whole number", structEdit{kind: editColSpan, elem: 12, value: "2.5"}, ErrTagsReview, "not a number of columns"},
		{"a span past what a PDF integer holds", structEdit{kind: editColSpan, elem: 12, value: "4294967297"}, ErrTagsReview, "not a number of columns"},
		{"headers on a paragraph", structEdit{kind: editHeaders, elem: 30, headers: []int{12}}, ErrTagsReview, "only a table cell"},
		{"a header that is a data cell", structEdit{kind: editHeaders, elem: 14, headers: []int{15}}, ErrTagsReview, "not another header cell"},
		{"a header of another table", structEdit{kind: editHeaders, elem: 14, headers: []int{22}}, ErrTagsReview, "not another header cell"},
		{"a header cell heading itself", structEdit{kind: editHeaders, elem: 12, headers: []int{12}}, ErrTagsReview, "not another header cell"},
		{"a header the tree does not have", structEdit{kind: editHeaders, elem: 14, headers: []int{99}}, ErrTagsStale, "99"},
		{"a header written inline", structEdit{kind: editHeaders, elem: 14, headers: []int{0}}, ErrTagsStale, ""},
	} {
		_, err := applyStructEdits(src, []structEdit{c.edit})
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v, want %v saying %q", c.name, err, c.want, c.says)
		}
	}
	// A cell in no table at all.
	loose := cellFixture(map[int]string{30: "<< /Type /StructElem /S /TD /P 7 0 R >>"})
	if _, err := applyStructEdits(loose, []structEdit{{kind: editHeaders, elem: 30, headers: []int{12}}}); !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), "in no table") {
		t.Errorf("a cell in no table: err = %v", err)
	}
}

// TestAHeaderCellKeepsTheIdentifierItHasAndANewOneCollidesWithNothing — a producer's `/ID` (a hex string
// here) is what `/Headers` names, byte for byte, and no entry is added for it; the identifier nib would
// have chosen for another header is already an element's, so it takes the next.
func TestAHeaderCellKeepsTheIdentifierItHasAndANewOneCollidesWithNothing(t *testing.T) {
	src := cellFixture(map[int]string{
		12: "<< /Type /StructElem /S /TH /P 10 0 R /ID <C3A9> >>",
		30: "<< /Type /StructElem /S /P /P 7 0 R /ID (nib-13) >>",
	})
	out := mustApply(t, src, structEdit{kind: editHeaders, elem: 14, headers: []int{12, 13}})
	if ids := idsOf(t, out, 12, 13, 30); ids[0] != "\xc3\xa9" || ids[1] != "nib-13-2" || ids[2] != "nib-13" {
		t.Errorf("identifiers read %q, want the producer's kept, nib-13-2 for the new one, and the paragraph's untouched", ids)
	}
	ctx, td := rawElement(t, out, 14)
	attr, _ := ctx.DereferenceDict(td["A"])
	hs, _ := ctx.DereferenceArray(attr["Headers"])
	var named []string
	for _, h := range hs {
		s, _, _ := byteString(ctx, h)
		named = append(named, s)
	}
	if !reflect.DeepEqual(named, []string{"\xc3\xa9", "nib-13-2"}) {
		t.Errorf("/Headers names %q", named)
	}
	if keys, objs, _ := idTreeNames(t, out); !reflect.DeepEqual(keys, []string{"nib-13-2"}) || !reflect.DeepEqual(objs, []int{13}) {
		t.Errorf("the /IDTree reads %q → %v, want only the identifier nib allocated", keys, objs)
	}
}

// TestANewIdentifierEntersTheIDTreeTheDocumentHas — flat and nested: in byte order, in the leaf whose
// limits hold it, with every `/Limits` on the way down widened; an entry already under the key is re-pointed.
func TestANewIdentifierEntersTheIDTreeTheDocumentHas(t *testing.T) {
	flat := cellFixture(map[int]string{
		7:  "<< /Type /StructTreeRoot /K [8 0 R 20 0 R 30 0 R] /IDTree 40 0 R >>",
		22: "<< /Type /StructElem /S /TH /P 21 0 R /ID (zz) >>",
		30: "<< /Type /StructElem /S /P /P 7 0 R /ID (aa) >>",
		40: "<< /Names [(aa) 30 0 R (nib-12) 30 0 R (zz) 22 0 R] >>",
	})
	out := mustApply(t, flat, structEdit{kind: editHeaders, elem: 14, headers: []int{13, 12}})
	if keys, objs, _ := idTreeNames(t, out); !reflect.DeepEqual(keys, []string{"aa", "nib-12", "nib-13", "zz"}) || !reflect.DeepEqual(objs, []int{30, 12, 13, 22}) {
		t.Errorf("the flat /IDTree reads %q → %v, want aa nib-12 nib-13 zz → 30 12 13 22 (the stale nib-12 entry re-pointed)", keys, objs)
	}

	nested := cellFixture(map[int]string{
		7:  "<< /Type /StructTreeRoot /K [8 0 R 20 0 R 30 0 R] /IDTree 40 0 R >>",
		22: "<< /Type /StructElem /S /TH /P 21 0 R /ID (pp) >>",
		30: "<< /Type /StructElem /S /P /P 7 0 R /ID (bb) >>",
		20: "<< /Type /StructElem /S /Table /P 7 0 R /K [21 0 R] /ID (zz) >>",
		40: "<< /Kids [41 0 R 42 0 R 44 0 R] >>",
		41: "<< /Limits [(bb) (bb)] /Names [(bb) 30 0 R] >>",
		42: "<< /Limits [(pp) (pp)] /Names 43 0 R >>",
		43: "[(pp) 22 0 R]",
		44: "<< /Limits [(zz) (zz)] /Names [(zz) 20 0 R] >>",
	})
	// nib-12 and nib-13 sort between bb and pp: both go to the first kid whose upper bound is not below them.
	out = mustApply(t, nested, structEdit{kind: editHeaders, elem: 14, headers: []int{12, 13}})
	if keys, objs, _ := idTreeNames(t, out); !reflect.DeepEqual(keys, []string{"bb", "nib-12", "nib-13", "pp", "zz"}) || !reflect.DeepEqual(objs, []int{30, 12, 13, 22, 20}) {
		t.Errorf("the nested /IDTree reads %q → %v", keys, objs)
	}
	if err := Validate(out); err != nil {
		t.Errorf("the document does not validate with the entries added to its nested tree: %v", err)
	}
}

// TestAnIDTreeEntryWidensEveryLimitOnItsWayDown — read from the context the writer left, before any
// validation: pdfcpu's relaxed validator rewrites a leaf's `/Limits` to its first and last key on every read
// (`validateNameTreeDictLimitsEntry`), so a document written and read back shows limits nib never wrote —
// measured, a writer that widened nothing passed that reading.
func TestAnIDTreeEntryWidensEveryLimitOnItsWayDown(t *testing.T) {
	src := cellFixture(map[int]string{
		7:  "<< /Type /StructTreeRoot /K [8 0 R 20 0 R 30 0 R] /IDTree 40 0 R >>",
		20: "<< /Type /StructElem /S /Table /P 7 0 R /K [21 0 R] /ID (zz) >>",
		22: "<< /Type /StructElem /S /TH /P 21 0 R /ID (pp) >>",
		30: "<< /Type /StructElem /S /P /P 7 0 R /ID (bb) >>",
		40: "<< /Kids [41 0 R 45 0 R] >>",
		41: "<< /Limits [(bb) (bb)] /Names [(bb) 30 0 R] >>",
		45: "<< /Limits [(pp) (zz)] /Kids [42 0 R 44 0 R] >>",
		42: "<< /Limits [(pp) (pp)] /Names [(pp) 22 0 R] >>",
		44: "<< /Limits [(zz) (zz)] /Names [(zz) 20 0 R] >>",
	})
	limits := func(ctx *model.Context, obj int) []string {
		d, _ := ctx.DereferenceDict(*types.NewIndirectRef(obj, 0))
		lim, _ := ctx.DereferenceArray(d["Limits"])
		var out []string
		for _, l := range lim {
			s, _, _ := byteString(ctx, l)
			out = append(out, s)
		}
		return out
	}
	for _, c := range []struct {
		id   string
		want map[int][]string
	}{
		// Between bb and pp: the first kid whose upper bound is not below it, and its leaf — both lower bounds fall.
		{"mm", map[int][]string{41: {"bb", "bb"}, 45: {"mm", "zz"}, 42: {"mm", "pp"}, 44: {"zz", "zz"}}},
		// Inside a leaf's bounds already: nothing moves.
		{"pp", map[int][]string{41: {"bb", "bb"}, 45: {"pp", "zz"}, 42: {"pp", "pp"}, 44: {"zz", "zz"}}},
		// Past every key: the last kid at each level, and both upper bounds rise.
		{"zzz", map[int][]string{41: {"bb", "bb"}, 45: {"pp", "zzz"}, 42: {"pp", "pp"}, 44: {"zz", "zzz"}}},
	} {
		if _, err := writeMutatedTree(t, src, func(ctx *model.Context, tree *structTree) error {
			if err := setIDTreeEntry(ctx, tree, c.id, types.StringLiteral(c.id), *types.NewIndirectRef(12, 0)); err != nil {
				return err
			}
			if root, _ := ctx.DereferenceDict(*types.NewIndirectRef(40, 0)); root["Limits"] != nil {
				t.Errorf("entering %q: the tree's root gained /Limits %v, and a name tree's root carries none", c.id, root["Limits"])
			}
			for obj, want := range c.want {
				if got := limits(ctx, obj); !reflect.DeepEqual(got, want) {
					t.Errorf("entering %q: node %d's /Limits read %q, want %q", c.id, obj, got, want)
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("entering %q: %v", c.id, err)
		}
	}
}

// TestACellEditNeverWritesThroughASharedAttributeObject — two cells share one indirect Table attribute
// object; a span on one leaves the other's, and its scope, as they were.
func TestACellEditNeverWritesThroughASharedAttributeObject(t *testing.T) {
	src := cellFixture(map[int]string{
		12: "<< /Type /StructElem /S /TH /P 10 0 R /A 40 0 R >>",
		13: "<< /Type /StructElem /S /TH /P 10 0 R /A 40 0 R >>",
		40: "<< /O /Table /Scope /Column /ColSpan 1 >>",
	})
	out := mustApply(t, src, structEdit{kind: editColSpan, elem: 12, value: "2"})
	got := byID(viewOf(t, out))
	if got[12].colSpan != 2 || got[13].colSpan != 1 || got[12].scope != "Column" || got[13].scope != "Column" {
		t.Errorf("spans %d and %d, scopes %q and %q — want 2 and 1, both Column", got[12].colSpan, got[13].colSpan, got[12].scope, got[13].scope)
	}
}

// TestASpanIsReadAsTheCheckerReadsIt — the first Table attribute object whose value is an INTEGER, else 1:
// a real is no span, and an attribute object another owner wrote is not the table's.
func TestASpanIsReadAsTheCheckerReadsIt(t *testing.T) {
	src := cellFixture(map[int]string{
		12: "<< /Type /StructElem /S /TH /P 10 0 R /A [<< /O /Layout /ColSpan 9 >> << /O /Table /ColSpan 2.0 >> << /O /Table /ColSpan 3 /RowSpan 40 0 R >>] >>",
		40: "4",
	})
	got := byID(viewOf(t, src))
	if got[12].colSpan != 3 || got[12].rowSpan != 4 || got[13].colSpan != 1 {
		t.Errorf("spans read %d×%d (and %d for a cell declaring none), want 3×4 and 1", got[12].colSpan, got[12].rowSpan, got[13].colSpan)
	}
}
