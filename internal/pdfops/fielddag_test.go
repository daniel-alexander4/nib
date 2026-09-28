package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 689` — the field-tree walks a page selection runs are bounded by the number of distinct
// fields, never by the number of PATHS to them.
//
// The shape is the review's: a chain of `levels` fields, each of whose /Kids names the next one k
// times. There are `levels` distinct fields and k^(levels-1) paths to the last, so any walk without a
// visited set pays the second figure — measured before the fix, 1,596 bytes at k=4 cost
// `RemovePages` 1.15 s and k=8 did not finish in 300 s.

// fieldDAG is that document: two pages, the field chain in /AcroForm /Fields, the last field a
// terminal field of type ft carrying a widget on page 1.
func fieldDAG(k, levels int, ft string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
		4: "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>",
		5: "<< /Length 14 >>\nstream\n0 0 10 10 re f\nendstream",
	}
	last := 6 + levels - 1
	objs[3] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [%d 0 R] >>", last)
	for l := 0; l < levels; l++ {
		n := 6 + l
		if n == last {
			objs[n] = fmt.Sprintf("<< /T (f%d) /FT /%s /Subtype /Widget /Rect [0 0 10 10] /P 3 0 R >>", l, ft)
			continue
		}
		objs[n] = fmt.Sprintf("<< /T (f%d) /Kids [%s] >>", l, strings.Repeat(fmt.Sprintf("%d 0 R ", n+1), k))
	}
	return assembleFixture(objs)
}

func readDAG(t *testing.T, pdf []byte) (*model.Context, types.Dict) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), subsetConf())
	if err != nil {
		t.Fatalf("setup: the fixture does not read: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if derefDict(ctx.XRefTable, root["AcroForm"]) == nil {
		t.Fatal("setup: the fixture's AcroForm did not survive the read, so no walk below runs")
	}
	return ctx, root
}

// TestKeepFieldsDecidesEachFieldOnce — the rebuild asks its predicate once per distinct terminal
// field. Twelve levels at k=2 is 2,048 paths to the one terminal field; unfixed, the predicate ran
// 2,048 times.
func TestKeepFieldsDecidesEachFieldOnce(t *testing.T) {
	const k, levels = 2, 12
	ctx, root := readDAG(t, fieldDAG(k, levels, "Tx"))
	xt := ctx.XRefTable
	fields := derefArray(xt, derefDict(xt, root["AcroForm"])["Fields"])
	if len(fields) != 1 {
		t.Fatalf("setup: %d top-level fields, want 1", len(fields))
	}
	asked := 0
	kept := keepFields(xt, fields, func(int, types.Dict) bool { asked++; return true }, 0)
	if asked != 1 {
		t.Errorf("keepFields asked its predicate %d times for ONE terminal field reached by %d paths — "+
			"the rebuild pays per path, not per field (/pending 689)", asked, 1<<(levels-1))
	}
	if len(kept) != 1 {
		t.Errorf("keepFields kept %d top-level fields, want 1 — deciding each field once must not change "+
			"what is kept", len(kept))
	}
	// The rebuilt /Kids still names the child k times: the memo changes the cost, not the tree.
	top := derefDict(xt, kept[0])
	if n := len(derefArray(xt, top["Kids"])); n != k {
		t.Errorf("the top field's rebuilt /Kids holds %d entries, want %d", n, k)
	}
}

// TestKeepFieldsDropsACycleWithoutLosingTheField — a /Kids naming an ancestor is a back edge, not a
// second field: it is dropped, and the field it points at is still decided on its own merits.
func TestKeepFieldsDropsACycleWithoutLosingTheField(t *testing.T) {
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [7 0 R] >>",
		5: "<< /Length 14 >>\nstream\n0 0 10 10 re f\nendstream",
		6: "<< /T (p) /Kids [7 0 R 6 0 R] >>",
		7: "<< /T (c) /FT /Tx /Subtype /Widget /Rect [0 0 10 10] /P 3 0 R /Parent 6 0 R >>",
	})
	// pdfcpu's validating read refuses a circular field tree, so the shape is reachable only through
	// a read that does not validate — the one this test uses to reach keepFields at all.
	ctx, err := api.ReadContext(bytes.NewReader(src), subsetConf())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	kept := keepFields(xt, derefArray(xt, derefDict(xt, root["AcroForm"])["Fields"]),
		func(int, types.Dict) bool { return true }, 0)
	if len(kept) != 1 {
		t.Fatalf("the cyclic parent was dropped (%d kept), want it kept for its real child", len(kept))
	}
	kids := derefArray(xt, derefDict(xt, kept[0])["Kids"])
	if len(kids) != 1 || kids[0].(types.IndirectRef).ObjectNumber.Value() != 7 {
		t.Errorf("the rebuilt /Kids is %v, want [7 0 R] — the back edge to 6 must go and the child stay", kids)
	}
}

// TestTheFieldWalkVisitsEachFieldOnce — `eachFormField`, the one walk (ADR-009), calls fn once per
// distinct field however many paths reach it, and hands over the reference a marking caller needs.
func TestTheFieldWalkVisitsEachFieldOnce(t *testing.T) {
	const k, levels = 4, 12
	ctx, root := readDAG(t, fieldDAG(k, levels, "Tx"))
	xt := ctx.XRefTable
	visits, refs := 0, 0
	eachFormField(xt, derefDict(xt, root["AcroForm"]), func(o types.Object, _ types.Dict) {
		visits++
		if _, ok := o.(types.IndirectRef); ok {
			refs++
		}
	})
	if visits != levels {
		t.Errorf("the field walk visited %d times for %d distinct fields", visits, levels)
	}
	if refs != visits {
		t.Errorf("%d of %d visits carried their reference — dropSignature marks by object number", refs, visits)
	}
}

// TestDropSignatureWalksTheFieldTreeThroughTheOneDoor — its own recursion had no visited set, so the
// guard is that it has none: the field tree is reached through `eachFormField`.
func TestDropSignatureWalksTheFieldTreeThroughTheOneDoor(t *testing.T) {
	calls := callsIn(t, "pageselect.go")
	if calls["dropSignature"] == nil {
		t.Fatal("dropSignature is not declared in pageselect.go, so this guard reads nothing")
	}
	if !calls["dropSignature"]["eachFormField"] {
		t.Error("dropSignature does not walk /AcroForm /Fields through eachFormField — a private walk " +
			"of the field tree is the k^depth shape /pending 689 removed")
	}
	if calls["dropSignature"]["scan"] {
		t.Error("dropSignature calls a `scan` of its own again — the field tree has one walk (ADR-009)")
	}
}

// TestASignatureAtTheEndOfAFieldDAGIsStillErased — the merged walk kept dropSignature's reach: a
// signature at the bottom of the chain is still found and erased by a page operation.
func TestASignatureAtTheEndOfAFieldDAGIsStillErased(t *testing.T) {
	out, err := RemovePages(fieldDAG(4, 12, "Sig"), []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), subsetConf())
	if rerr != nil {
		t.Fatal(rerr)
	}
	root, _ := ctx.XRefTable.Catalog()
	found := false
	if form := derefDict(ctx.XRefTable, root["AcroForm"]); form != nil {
		eachFormField(ctx.XRefTable, form, func(_ types.Object, d types.Dict) {
			if nameVal(d, "FT") == "Sig" {
				found = true
			}
		})
	}
	if found {
		t.Error("a signature field at the end of a 12-level field DAG survived a page operation")
	}
}
