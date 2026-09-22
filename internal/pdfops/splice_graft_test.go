package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `PLAN-ua-coverage.md` P02.S07b, ADR-048 — `splice` makes the ORIGINAL the host of one merge, at every
// insertion point, and puts the inserted elements where they are read.

// readingOrderPages is the page of every piece of content in the order a reader meets it: the structure
// tree walked depth-first from the root, one entry per MCID, marked-content reference or annotation
// reference. Walking the whole tree — not one level of it — is the point: an inserted element left at
// the root beside the host's /Document is read after the entire document, and a one-level reading
// cannot see that.
func readingOrderPages(t *testing.T, pdf []byte) []int {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	cat, _ := xt.Catalog()
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		t.Fatal("the output has no structure tree to read an order from")
	}
	pageNr := map[int]int{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, err := ctx.PageDictIndRef(p); err == nil && ir != nil {
			pageNr[ir.ObjectNumber.Value()] = p
		}
	}
	var out []int
	seen := map[int]bool{}
	var walk func(o types.Object, pg, depth int)
	walk = func(o types.Object, pg, depth int) {
		if depth > 64 {
			return
		}
		if ref, ok := o.(types.IndirectRef); ok {
			if seen[ref.ObjectNumber.Value()] {
				return
			}
			seen[ref.ObjectNumber.Value()] = true
		}
		if _, mcid := o.(types.Integer); mcid {
			out = append(out, pageNr[pg])
			return
		}
		if arr := derefArray(xt, o); arr != nil {
			for _, k := range arr {
				walk(k, pg, depth+1)
			}
			return
		}
		d := derefDict(xt, o)
		if d == nil {
			return
		}
		if r, ok := d["Pg"].(types.IndirectRef); ok {
			pg = r.ObjectNumber.Value()
		}
		if ty, _ := d["Type"].(types.Name); ty == "MCR" || ty == "OBJR" {
			out = append(out, pageNr[pg])
			return
		}
		walk(d["K"], pg, depth+1)
	}
	walk(root["K"], 0, 0)
	return out
}

// assertInsertedAt fails unless the elements read on the inserted pages [first, last] sit in one run
// with every element read before them starting before the insertion and every one after starting after
// it. It is deliberately NOT "the tree reads in page order": a producer's own tree need not (this
// package's four-page fixture reads its page-1 link after an element reaching page 2), and the graft
// promises only where the inserted content goes.
func assertInsertedAt(t *testing.T, pdf []byte, first, last int) {
	t.Helper()
	order := readingOrderPages(t, pdf)
	lo, hi := -1, -1
	for i, p := range order {
		if p >= first && p <= last {
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		t.Fatalf("setup: no element is read on the inserted pages %d–%d: %v", first, last, order)
	}
	for i, p := range order {
		switch {
		case p == 0: // content on no page this document has
		case i < lo && p >= first:
			t.Errorf("element %d (page %d) is read before the inserted content, which starts on page %d: %v", i, p, first, order)
		case i > hi && p <= last:
			t.Errorf("element %d (page %d) is read after the inserted content, which ends on page %d: %v", i, p, last, order)
		case i >= lo && i <= hi && (p < first || p > last):
			t.Errorf("element %d (page %d) sits inside the inserted run: %v", i, p, order)
		}
	}
}

// TestInsertingATaggedDocumentGraftsItWhereItIsRead — the inserted document's element lands between the
// host's page-2 and page-3 elements, and every element keeps a key of its own.
func TestInsertingATaggedDocumentGraftsItWhereItIsRead(t *testing.T) {
	host, mid := subsetFixture(), taggedFixture()
	_, _, _, hostElems := carryOf(t, host)
	_, _, _, midElems := carryOf(t, mid)
	out, err := InsertPDF(host, mid, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := pageText(t, out, 3); !bytes.Contains([]byte(got), []byte("Tagged heading")) {
		t.Fatalf("setup: page 3 reads %q; the inserted document is not where it was put", got)
	}
	verdict, defects, orphans, elems := carryOf(t, out)
	if verdict != "carried" || len(defects) != 0 || len(orphans) != 0 {
		t.Errorf("fate %q, %d defects %v, orphans %v — want a complete carried tree", verdict, len(defects), defects, orphans)
	}
	if elems != hostElems+midElems {
		t.Errorf("%d elements, want %d + %d", elems, hostElems, midElems)
	}
	assertInsertedAt(t, out, 3, 3)

	// Every element at the /Document's level names the /Document as its parent — the grafted one too.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := ctx.XRefTable.Catalog()
	docRef, _ := rootKids(ctx.XRefTable, derefDict(ctx.XRefTable, cat["StructTreeRoot"]))[0].(types.IndirectRef)
	for i, k := range rootKids(ctx.XRefTable, derefDict(ctx.XRefTable, docRef)) {
		if p, _ := derefDict(ctx.XRefTable, k)["P"].(types.IndirectRef); p.ObjectNumber != docRef.ObjectNumber {
			t.Errorf("the /Document's kid %d names parent %v, not the /Document %v", i, p, docRef)
		}
	}

	// Two placements the plain fixture cannot tell apart from wrong ones. A LINK element read right
	// after the insertion point is located by its annotation reference's page; an element nothing
	// locates is passed over rather than read as "after".
	for _, v := range []struct {
		name string
		kids string
		objs map[int]string
	}{
		{"a link first after the insertion point", "[32 0 R 33 0 R 34 0 R 36 0 R 35 0 R 37 0 R]", nil},
		{"an unlocated element before it", "[32 0 R 50 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R]",
			map[int]string{50: "<< /Type /StructElem /S /Div /P 31 0 R >>"}},
		{"a document wrapped in a /Part", "[51 0 R]", map[int]string{
			51: "<< /Type /StructElem /S /Part /P 31 0 R /K [32 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R] >>"}},
	} {
		objs := subsetFixtureObjects()
		objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K " + v.kids + " >>"
		for n, o := range v.objs {
			objs[n] = o
		}
		got, err := InsertPDF(assembleFixture(objs), taggedFixture(), 2, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(v.name, func(t *testing.T) { assertInsertedAt(t, got, 3, 3) })
	}
}

// TestInsertingBeforePageOneKeepsTheOriginalAsTheHost — the case that made "the first input" the wrong
// host: an untagged page inserted at page 1 was the first SEGMENT, so its catalog won and the original's
// whole tree went.
func TestInsertingBeforePageOneKeepsTheOriginalAsTheHost(t *testing.T) {
	host := subsetFixture()
	_, _, _, hostElems := carryOf(t, host)

	out, err := InsertPDF(host, untaggedFixture(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ClaimsTagging(out) {
		t.Fatal("inserting an untagged page before page 1 dropped the original's tagging")
	}
	verdict, defects, _, elems := carryOf(t, out)
	if verdict != "partial" || len(defects) != 0 || elems != hostElems {
		t.Errorf("fate %q, %d defects, %d of %d elements — want partial, complete, every original element", verdict, len(defects), elems, hostElems)
	}
	if lang := readLangOf(t, out); lang != "en-GB" {
		t.Errorf("the result's /Lang is %q, want the original's en-GB", lang)
	}

	// And the mirror: a tagged page inserted into an UNTAGGED document is not grafted onto nothing.
	plain, err := InsertPDF(untaggedFixture(), taggedFixture(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if ClaimsTagging(plain) {
		t.Error("a tagged page inserted into an untagged document made the result claim tagging")
	}
	for p, k := range pageClaims(t, plain) {
		if k >= 0 {
			t.Errorf("page %d keeps /StructParents %d with no tree to index", p+1, k)
		}
	}

	tagged, err := InsertPDF(host, taggedFixture(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	assertInsertedAt(t, tagged, 1, 1)
}

// TestSplittingAPageKeepsTheRestOfTheDocumentsTree — P02.S06's other half. The tiles carry no subtree,
// but splitting one page no longer destroys every OTHER page's tags.
func TestSplittingAPageKeepsTheRestOfTheDocumentsTree(t *testing.T) {
	host := subsetFixture()
	out, err := SplitPage(host, 3, 2, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	claims := pageClaims(t, out)
	if len(claims) != 5 {
		t.Fatalf("setup: the split produced %d pages, want 5 (two tiles for page 3)", len(claims))
	}
	if !ClaimsTagging(out) {
		t.Fatal("splitting one page dropped the whole document's tagging")
	}
	verdict, defects, orphans, elems := carryOf(t, out)
	if verdict != "partial" || len(defects) != 0 || len(orphans) != 0 || elems == 0 {
		t.Errorf("fate %q, %d defects %v, orphans %v, %d elements — want partial and complete", verdict, len(defects), defects, orphans, elems)
	}
	for _, p := range []int{0, 1, 4} {
		if claims[p] < 0 {
			t.Errorf("page %d (an original page) lost its /StructParents", p+1)
		}
	}
	for _, p := range []int{2, 3} {
		if claims[p] >= 0 {
			t.Errorf("tile page %d keeps /StructParents %d; a tile carries no subtree (P02.S06)", p+1, claims[p])
		}
	}
}

func readLangOf(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := ctx.XRefTable.Catalog()
	return readLang(ctx.XRefTable, cat["Lang"])
}

// TestAnIncompleteInsertFallsBackToTheHonestLoss — `splice`'s output gate. An inserted document two of
// whose pages claim the same `/ParentTree` key (completeness condition 5) grafts without complaint and
// fails the gate; the result must not ship that tree, and the loss must be the INSERT's tags only. The
// fallback once went straight to the non-carrying shape, so a defect in the inserted document wiped the
// original's whole tree.
func TestAnIncompleteInsertFallsBackToTheHonestLoss(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[5] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 6 0 R /StructParents 0 >>"
	mid := assembleFixture(objs)
	host := taggedFixture()
	_, _, _, hostElems := carryOf(t, host)
	if hostElems == 0 {
		t.Fatal("setup: the host has no elements, so losing its tree would be invisible")
	}
	// The stimulus, asserted: the grafting attempt really is refused by the gate. Without this the test
	// passes on an insert that grafts cleanly and never reaches the fallback — which it once did.
	grafted, _, carried, err := spliceOnce(host, 1, 2, 1, mid, true)
	if err != nil {
		t.Fatal(err)
	}
	if !carried || carryIsComplete(grafted) {
		t.Fatal("setup: the grafting attempt passes the gate, so the fallback is never exercised")
	}

	out, err := InsertPDF(host, mid, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	_, defects, _, elems := carryOf(t, out)
	if len(defects) != 0 {
		t.Errorf("the insert shipped an incomplete tree: %v", defects)
	}
	if elems != hostElems {
		t.Errorf("%d elements after the fallback, want the host's own %d kept", elems, hostElems)
	}
	claims := pageClaims(t, out)
	if claims[0] < 0 {
		t.Error("the host's own page lost its claim; the fallback strips the INSERT, not the host")
	}
	for p := 1; p < len(claims); p++ {
		if claims[p] >= 0 {
			t.Errorf("inserted page %d keeps /StructParents %d; its tree was refused, so its claims must go", p+1, claims[p])
		}
	}
}

// TestInsertingBeforeTheLastPage — the host's LAST page is a page an element can start on, so inserting
// in front of it must place the inserted content ahead of that element.
func TestInsertingBeforeTheLastPage(t *testing.T) {
	out, err := InsertPDF(subsetFixture(), taggedFixture(), 4, true)
	if err != nil {
		t.Fatal(err)
	}
	assertInsertedAt(t, out, 4, 4)
}

// TestFirstPageIsTheLowestPageAndTheNearestPgWins pins firstPage's two rules on elements built in
// memory, where the fixture documents cannot reach them: an element's kids need not be in page order,
// so it starts at its LOWEST page, not its first kid's; and a `/Pg` is inherited from the NEAREST
// element that states one, so a kid's own `/Pg` beats its parent's.
func TestFirstPageIsTheLowestPageAndTheNearestPgWins(t *testing.T) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(subsetFixture()), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pageNr := map[int]int{}
	refs := map[int]types.IndirectRef{}
	for p := 1; p <= ctx.PageCount; p++ {
		ir, err := ctx.PageDictIndRef(p)
		if err != nil || ir == nil {
			t.Fatal(err)
		}
		pageNr[ir.ObjectNumber.Value()] = p
		refs[p] = *ir
	}
	xt := ctx.XRefTable
	unordered := types.Dict{"S": types.Name("P"), "Pg": refs[3], "K": types.Array{
		types.Integer(0), // on page 3, the element's own /Pg
		types.Dict{"Type": types.Name("MCR"), "Pg": refs[1], "MCID": types.Integer(0)},
	}}
	if got := firstPage(xt, unordered, 0, pageNr, 0, map[int]bool{}); got != 1 {
		t.Errorf("an element whose second kid is on page 1 starts on page %d, want 1", got)
	}
	nested := types.Dict{"S": types.Name("Sect"), "Pg": refs[1], "K": types.Array{
		types.Dict{"S": types.Name("P"), "Pg": refs[4], "K": types.Array{types.Integer(0)}},
	}}
	if got := firstPage(xt, nested, 0, pageNr, 0, map[int]bool{}); got != 4 {
		t.Errorf("a kid stating /Pg page 4 under a parent stating page 1 is located on page %d, want 4", got)
	}
}

// TestAnInsertIsNeverPlacedInsideAnElementsContent — the placement descends through single-element
// wrappers and must STOP at an element whose kids are content: the one-page fixture's root holds a sole
// /P whose kid is an MCID, and descending into it would make the inserted document a child of a
// paragraph.
func TestAnInsertIsNeverPlacedInsideAnElementsContent(t *testing.T) {
	out, err := InsertPDF(taggedFixture(), taggedFixture(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	xt, root := structRoot(t, out)
	if kids := rootKids(xt, root); len(kids) != 2 {
		t.Errorf("the root holds %d element(s), want the host's /P and the inserted one side by side", len(kids))
	}
	if v, d, _, _ := carryOf(t, out); v != "carried" || len(d) != 0 {
		t.Errorf("fate %q with %d defects", v, len(d))
	}
	assertInsertedAt(t, out, 2, 2)
}
