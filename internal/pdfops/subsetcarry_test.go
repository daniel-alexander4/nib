package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P02.S04b's readers — a subset carries the structure of the pages it keeps.
//
// # Why these fixtures exist, and what the repo had instead
//
// The tag-fate census drives `taggedFixture()`, which has ONE page — so `Collect(b, ["1"])` is the
// identity selection, `RemovePages(b, ["1"])` removes the document's only page and is not driven at
// all, and a `carried` verdict there is true without a single page having been dropped. The census
// document (`censusMarkdown`) has eight pages but **no MCR, no OBJR, no RoleMap and no annotation of
// any kind**, measured. So every shape below needed a fixture of its own, and each one is the
// stimulus for a defect that was measured rather than imagined.

// streamObj renders a content stream object body.
func streamObj(content string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
}

// markedPage renders one page's content with a single marked-content sequence carrying MCID 0.
func markedPage(marker string) string {
	return fmt.Sprintf("/P <</MCID 0>> BDC\nBT /F1 18 Tf 72 700 Td (%s) Tj ET\nEMC\n", marker)
}

// subsetFixture is four tagged pages holding every shape the census document lacks: a RoleMap, an
// OBJR-referenced Link annotation on a page the tests KEEP and on a page they DROP, an element whose
// own `/Pg` is page 1 and whose only kid is an MCR naming page 2, a named destination into page 3,
// `/ViewerPreferences`, and a `/Metadata` packet with producer-style provenance in it.
//
// The `/ParentTree` holds BOTH entry shapes: arrays for the four pages (keys 0–3) and single
// references for the two annotations (keys 4–5). Every page's text is a unique marker, so a residue
// reader can tell which pages reached the output.
func subsetFixture() []byte { return assembleFixture(subsetFixtureObjects()) }

// subsetFixtureObjects is that fixture's objects, so a test needing one shape changed replaces an
// entry rather than copying the whole document.
func subsetFixtureObjects() map[int]string {
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>` +
		`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmpMM="http://ns.adobe.com/xap/mm/">` +
		`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">ZZTITLEMARKER</rdf:li></rdf:Alt></dc:title>` +
		`<dc:creator><rdf:Seq><rdf:li>ZZCREATORMARKER</rdf:li></rdf:Seq></dc:creator>` +
		`<dc:description>ZZDESCRIPTIONMARKER</dc:description>` +
		`<xmpMM:DocumentID>ZZDOCIDMARKER</xmpMM:DocumentID>` +
		`<xmpMM:History>ZZHISTORYMARKER</xmpMM:History>` +
		`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
	return map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R " +
			"/Lang (en-GB) /Names 40 0 R /ViewerPreferences 41 0 R /Metadata 43 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 5 0 R 7 0 R 9 0 R] /Count 4 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 4 0 R /StructParents 0 /Annots [20 0 R] >>",
		4:  streamObj(markedPage("ZZPAGEONE")),
		5:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 6 0 R /StructParents 1 >>",
		6:  streamObj(markedPage("ZZPAGETWO")),
		7:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 8 0 R /StructParents 2 /Annots [22 0 R] >>",
		8:  streamObj(markedPage("ZZPAGETHREE")),
		9:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 10 0 R /StructParents 3 >>",
		10: streamObj(markedPage("ZZPAGEFOUR")),
		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		20: "<< /Type /Annot /Subtype /Link /Rect [72 690 300 720] /Border [0 0 0] /P 3 0 R /StructParent 4 /A << /S /GoTo /D [7 0 R /Fit] >> >>",
		22: "<< /Type /Annot /Subtype /Link /Rect [72 690 300 720] /Border [0 0 0] /P 7 0 R /StructParent 5 /A << /S /GoTo /D [3 0 R /Fit] >> >>",
		30: "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R /RoleMap 38 0 R /ParentTreeNextKey 6 >>",
		31: "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R] >>",
		32: "<< /Type /StructElem /S /Para /P 31 0 R /Pg 3 0 R /K [0] >>",
		33: "<< /Type /StructElem /S /P /P 31 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 5 0 R /MCID 0 >>] >>",
		34: "<< /Type /StructElem /S /Link /P 31 0 R /Pg 3 0 R /K [<< /Type /OBJR /Obj 20 0 R /Pg 3 0 R >>] >>",
		35: "<< /Type /StructElem /S /P /P 31 0 R /Pg 7 0 R /K [0] >>",
		36: "<< /Type /StructElem /S /Link /P 31 0 R /Pg 7 0 R /K [<< /Type /OBJR /Obj 22 0 R /Pg 7 0 R >>] >>",
		37: "<< /Type /StructElem /S /P /P 31 0 R /Pg 9 0 R /K [0] >>",
		38: "<< /Para /P >>",
		39: "<< /Nums [0 [32 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R] 4 34 0 R 5 36 0 R] >>",
		40: "<< /Dests 42 0 R >>",
		41: "<< /DisplayDocTitle true /PrintPageRange [3 7] /NumCopies 3 >>",
		42: "<< /Names [(toPageThree) [7 0 R /Fit]] >>",
		43: fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp),
	}
}

// livePages is the set of page objects the page tree lists, which is what `orphanPageObjects` reads
// its subject against.
func livePages(ctx *model.Context) map[int]bool {
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	return live
}

// carryOf reads a document and reports everything this slice grades: its tag fate, the completeness
// defects of its tree, and any page object the page tree no longer lists.
func carryOf(t *testing.T, pdf []byte) (verdict string, defects []structDefect, orphans []int, elems int) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the output could not be read back: %v", err)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	orphans = orphanPageObjects(ctx, live)
	verdict = fate(pdf)
	tree, terr := readStructTree(ctx, live)
	switch {
	case errors.Is(terr, errNoStructTree):
		// No tree at all: zero defects and zero elements are the truth, not a silent read failure.
	case terr != nil:
		// **A tree the model cannot READ is not a clean one**, and reporting `defects=nil` for it
		// would be two outcomes where three are needed — the instrument answering "clean" about a
		// document it could not look at.
		t.Fatalf("the output carries a structure tree that does not parse: %v.\nEvery assertion "+
			"about defects or element counts below would have read as clean", terr)
	default:
		defects = structureCarriedCompletely(ctx, tree)
		elems = tree.elements()
	}
	return verdict, defects, orphans, elems
}

// TestASubsetCarriesTheStructureOfThePagesItKeeps is the slice's central reader, and it is driven on
// a document a page can actually be taken OUT of — which is what the tag-fate census cannot do.
func TestASubsetCarriesTheStructureOfThePagesItKeeps(t *testing.T) {
	src := subsetFixture()
	if v, d, _, n := carryOf(t, src); v != "carried" || len(d) > 0 || n != 7 {
		t.Fatalf("setup: the fixture is %s with %d defect(s) and %d element(s), want a clean "+
			"7-element carried document (a /Document over six kids) — every assertion below would "+
			"be measured against a document that was already broken", v, len(d), n)
	}
	// Keep pages 1 and 2 in reverse order: a real prune (two pages go) AND a reorder.
	out, err := Collect(src, []string{"2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	v, defects, orphans, elems := carryOf(t, out)
	if v != "carried" {
		t.Errorf("a subset of a tagged document measured %q, want carried", v)
	}
	if len(defects) > 0 {
		t.Errorf("the carried tree is incomplete: %v", defects)
	}
	if len(orphans) > 0 {
		t.Errorf("page object(s) %v are still in the file and not in the page tree — pdfcpu writes "+
			"by reachability, so their /Contents shipped", orphans)
	}
	// Elements 35, 36 and 37 describe pages 3 and 4, which are gone; the /Document and 32, 33, 34 stay.
	if elems != 4 {
		t.Errorf("the carried tree has %d element(s), want 4 — the /Document and the three "+
			"describing the pages that survived; the other three should have left with their pages", elems)
	}
	for _, m := range []string{"ZZPAGETHREE", "ZZPAGEFOUR"} {
		if raw, streams := fileCarries(out, m); raw || streams > 0 {
			t.Errorf("a dropped page's text %q is still in the output (raw=%v streams=%d)", m, raw, streams)
		}
	}
	for _, m := range []string{"ZZPAGEONE", "ZZPAGETWO"} {
		if raw, streams := fileCarries(out, m); !raw && streams == 0 {
			t.Errorf("a KEPT page's text %q is absent from the output, so the assertions above are "+
				"passing on a document with nothing in it", m)
		}
	}
	// The fixture's named destination points into page 3, which this selection drops: `pruneNames`
	// is on the path and the dest must not survive as a dangling name.
	if raw, streams := fileCarries(out, "toPageThree"); raw || streams > 0 {
		t.Errorf("the named destination into a dropped page survived (raw=%v streams=%d)", raw, streams)
	}
	// **The REORDER half needs its own assertion.** `["2","1"]` and `["1","2"]` produce identical
	// counts and identical defect sets, so without this the "AND a reorder" in the selection above
	// is a stimulus nothing grades.
	if got := pageText(t, out, 1); !strings.Contains(got, "ZZPAGETWO") {
		t.Errorf("the output's first page reads %q, want the source's SECOND page — the selection "+
			"asked for [2 1] and the carry has to follow the order, not just the set", got)
	}
	if got := pageText(t, out, 2); !strings.Contains(got, "ZZPAGEONE") {
		t.Errorf("the output's second page reads %q, want the source's first", got)
	}
}

// pageText returns one page's decoded content stream, for asserting which source page landed where.
func pageText(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	d, _, _, perr := ctx.PageDict(page, false)
	if perr != nil || d == nil {
		t.Fatalf("page %d does not resolve: %v", page, perr)
	}
	b, cerr := ctx.PageContent(d, page)
	if cerr != nil {
		t.Fatalf("page %d has no content: %v", page, cerr)
	}
	return string(b)
}

// TestASubsetKeepsAnElementWhoseOwnPageIsGone — the predicate that nearly destroyed the tree.
//
// LibreOffice writes `/Pg` on 523 of 523 elements, including the root `/Document` (`/Pg` = page 1)
// and a `/Table` whose `/Pg` is page 3 while its rows are on 3–5. Under "an element whose own `/Pg`
// has gone is dead" a subset that drops page 1 leaves **0** elements of 523, and does so on 5 of the
// 7 real multi-page tagged documents available. Death is decided by KIDS, not by `/Pg`.
func TestASubsetKeepsAnElementWhoseOwnPageIsGone(t *testing.T) {
	// A container whose own /Pg is page 1 and whose kids describe pages 2 and 3.
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R 7 0 R] /Count 3 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: streamObj(markedPage("ZZFIRST")),
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 6 0 R /StructParents 1 >>",
		6: streamObj(markedPage("ZZSECOND")),
		7: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 8 0 R /StructParents 2 >>",
		8: streamObj(markedPage("ZZTHIRD")),
		// The root element's /Pg is page 1, exactly as LibreOffice writes it.
		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		30: "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R >>",
		31: "<< /Type /StructElem /S /Document /P 30 0 R /Pg 3 0 R /K [32 0 R 33 0 R 34 0 R] >>",
		32: "<< /Type /StructElem /S /P /P 31 0 R /Pg 3 0 R /K [0] >>",
		33: "<< /Type /StructElem /S /P /P 31 0 R /Pg 5 0 R /K [0] >>",
		34: "<< /Type /StructElem /S /P /P 31 0 R /Pg 7 0 R /K [0] >>",
		39: "<< /Nums [0 [32 0 R] 1 [33 0 R] 2 [34 0 R]] >>",
	})
	if v, d, _, n := carryOf(t, src); v != "carried" || len(d) > 0 || n != 4 {
		t.Fatalf("setup: the fixture is %s with %d defect(s) and %d element(s), want a clean 4", v, len(d), n)
	}
	out, err := RemovePages(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	v, defects, orphans, elems := carryOf(t, out)
	if v != "carried" || len(defects) > 0 || len(orphans) > 0 {
		t.Fatalf("dropping the page the container element names left %s, defects %v, orphan pages %v",
			v, defects, orphans)
	}
	// The container survives with its `/Pg` gone; the two elements on kept pages survive with it.
	if elems != 3 {
		t.Errorf("the carry left %d element(s), want 3 (the container plus the two kept pages'). "+
			"A container's own /Pg is where its MCIDs WOULD be, not a statement that its whole "+
			"subtree is on that page — treating it as a death sentence empties the tree", elems)
	}
	for _, m := range []string{"ZZSECOND", "ZZTHIRD"} {
		if raw, streams := fileCarries(out, m); !raw && streams == 0 {
			t.Errorf("a kept page's text %q is gone from the output", m)
		}
	}
}

// TestACarriedOutputIsNeverOrphaned states the implication the carry relies on so it costs one parse
// rather than two: a complete carry over an anchored tree cannot be `orphaned()`.
func TestACarriedOutputIsNeverOrphaned(t *testing.T) {
	// **A population floor, because every case `continue`s when the carry is refused.** Measured:
	// with `carryStructure` returning `(false, nil)` immediately, all three cases skipped and this
	// test PASSED while eleven of the file's others went red — and production cites it by name as
	// the reason the gate pays for one parse rather than two.
	asked := 0
	for _, c := range []struct {
		name  string
		src   []byte
		order []string
	}{
		{"a prune and a reorder", subsetFixture(), []string{"2", "1"}},
		{"every page kept", subsetFixture(), []string{"1-"}},
		{"a repeat", subsetFixture(), []string{"1", "1", "2"}},
	} {
		out, err := Collect(c.src, c.order)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		s := inspectTags(out)
		if !s.tree {
			continue // the carry was refused; the claim went with it, which is the honest loss
		}
		asked++
		if s.orphaned() {
			t.Errorf("%s: a carried output is orphaned (%+v). Completeness condition 3 forces every "+
				"/ParentTree key to have a live owner, and the carry refuses an unanchored tree — so "+
				"this state should be unreachable, and the gate pays for one parse on the strength "+
				"of that", c.name, s)
		}
	}
	if asked == 0 {
		t.Fatal("not one case produced a carried tree, so orphaned() was never asked and this test " +
			"asserted nothing. It is cited in production as the reason the output gate reads the " +
			"document once instead of twice")
	}
}

// TestACarryThatAnchorsNothingIsRefused — the refusal neither codified predicate can make.
//
// `structureCarriedCompletely` is vacuously clean on a tree with no elements and no owned keys, and
// `orphaned()` answers false because the kept page's stale `/StructParents` still counts as an
// anchor. Together they ship `/MarkInfo /Marked true` over content nothing describes.
func TestACarryThatAnchorsNothingIsRefused(t *testing.T) {
	// Page 2 is tagged; page 1 carries a /StructParents whose row does not exist. Keeping page 1
	// alone empties the tree while leaving that dangling number behind.
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 4 0 R /StructParents 7 >>",
		4: streamObj(markedPage("ZZUNDESCRIBED")),
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 6 0 R /StructParents 1 >>",
		6: streamObj(markedPage("ZZDESCRIBED")),

		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		30: "<< /Type /StructTreeRoot /K [33 0 R] /ParentTree 39 0 R >>",
		33: "<< /Type /StructElem /S /P /P 30 0 R /Pg 5 0 R /K [0] >>",
		39: "<< /Nums [1 [33 0 R]] >>",
	})
	if !inspectTags(src).claims() {
		t.Fatal("setup: the fixture claims no tagging, so a refusal below proves nothing")
	}
	// **The control, on the SAME document.** Keeping page 2 — the one an element describes — must
	// carry, or the refusal below is a build that refuses everything rather than one that refuses an
	// unanchored prune. Measured without this: with `carryStructure` returning `(false, nil)` this
	// test passed.
	if kept, cerr := Collect(src, []string{"2"}); cerr != nil {
		t.Fatal(cerr)
	} else if !inspectTags(kept).tree {
		t.Fatal("setup: keeping the DESCRIBED page did not carry either, so the refusal below is " +
			"not about the prune leaving nothing anchored")
	}
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	s := inspectTags(out)
	if s.tree || s.marked {
		t.Errorf("a subset keeping only the page nothing describes still claims tagging "+
			"(tree=%v marked=%v elements=%d anchored=%d). The completeness predicate is vacuously "+
			"clean on an empty tree and orphaned() reads the stale /StructParents as an anchor, so "+
			"the refusal has to be the carry's own", s.tree, s.marked, s.elements, s.anchored)
	}
	if raw, streams := fileCarries(out, "ZZUNDESCRIBED"); !raw && streams == 0 {
		t.Error("the kept page's own content is missing from the output, so this test is about the wrong thing")
	}
}

// TestADuplicatedPageGetsItsOwnParentTreeKey — one key names one row, so two pages cannot share one.
func TestADuplicatedPageGetsItsOwnParentTreeKey(t *testing.T) {
	src := subsetFixture()
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n, perr := PageCount(out); perr != nil || n != 5 {
		t.Fatalf("the duplicate produced %d page(s) (err %v), want 5", n, perr)
	}
	v, defects, orphans, _ := carryOf(t, out)
	if v != "carried" {
		t.Errorf("DuplicatePage measured %q, want carried", v)
	}
	if len(defects) > 0 {
		t.Errorf("the duplicate's tree is incomplete: %v.\nA shared /ParentTree key means the "+
			"elements describe one of the two pages and the other's content is reached through "+
			"references naming its twin (condition 5)", defects)
	}
	if len(orphans) > 0 {
		t.Errorf("orphan page object(s) %v", orphans)
	}
	// Both copies of page 1 are described, so neither is `undescribed`.
	if s := inspectTags(out); s.undescribed != 0 {
		t.Errorf("%d page(s) of the duplicate are described by nothing; the copy's subtree was not "+
			"carried onto it", s.undescribed)
	}
}

// TestAnOBJROnADroppedPageGoesAndOneOnAKeptPageStays — the acceptance clause's own fixture.
func TestAnOBJROnADroppedPageGoesAndOneOnAKeptPageStays(t *testing.T) {
	src := subsetFixture()
	out, err := Collect(src, []string{"1", "2"}) // page 3 and its Link annotation go
	if err != nil {
		t.Fatal(err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatalf("the carried tree does not parse: %v", terr)
	}
	objrs, links := 0, 0
	for _, e := range tree.elems {
		if e.kind == "Link" {
			links++
		}
		for _, k := range e.kids {
			if k.kind == kidOBJR {
				objrs++
			}
		}
	}
	if links != 1 || objrs != 1 {
		t.Errorf("the carried tree has %d /Link element(s) and %d OBJR kid(s), want 1 and 1: the "+
			"annotation on the kept page keeps its element, and the one on the dropped page takes "+
			"its element with it", links, objrs)
	}
	if defects := structureCarriedCompletely(ctx, tree); len(defects) > 0 {
		t.Errorf("incomplete: %v", defects)
	}
	if raw, streams := fileCarries(out, "ZZPAGETHREE"); raw || streams > 0 {
		t.Error("the dropped page's text reached the output, re-anchored through something")
	}
}

// TestASubsetRefusesToCarryANestedParentTree — the shape the write half will not rebalance.
//
// Measured: 0 of 294 tagged files in veraPDF's corpus nest one, and neither LibreOffice conversion
// does, so this is the only place the refusal can be exercised at all.
func TestASubsetRefusesToCarryANestedParentTree(t *testing.T) {
	// **The control is a `Collect` OF the flat fixture, not the fixture's own bytes.** Reading the
	// hand-assembled fixture says `carried` whatever the carry does — measured, with
	// `carryStructure` returning `(false, nil)` this test passed — so the control has to be the same
	// operation on the same objects with only the nesting changed.
	flat, ferr := Collect(subsetFixture(), []string{"1", "2"})
	if ferr != nil {
		t.Fatal(ferr)
	}
	if !inspectTags(flat).tree {
		t.Fatal("setup: a subset of the FLAT fixture does not carry either, so a refusal below is " +
			"not about the nesting")
	}
	src := assembleFixture(nestedParentTreeObjects())
	out, err := Collect(src, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); s.tree {
		t.Errorf("a nested /ParentTree was carried (%+v). The write half refuses to rebalance one "+
			"(`parentTreeDict`), so a reader following /Kids would never find the entries this "+
			"prune rewrote", s)
	}
}

// nestedParentTreeObjects is subsetFixture with its `/ParentTree` expressed as a nested number
// tree — one `/Kids` level over the same entries.
func nestedParentTreeObjects() map[int]string {
	objs := map[int]string{}
	for k, v := range subsetFixtureObjects() {
		objs[k] = v
	}
	objs[39] = "<< /Kids [44 0 R] >>"
	objs[44] = "<< /Limits [0 5] /Nums [0 [32 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R] 4 34 0 R 5 36 0 R] >>"
	return objs
}

// TestACarriedElementDoesNotShipAnEmbeddedFileThroughItsAF — `/AF` on an element names a `/Filespec`
// whose `/EF` stream is an attachment payload, and `pruneNames` deletes the catalog's
// `/EmbeddedFiles` tree on every subset for exactly the reason its own header records.
//
// Worse than an ordinary leak: `Attachments()` reads the name tree, so nib would report no
// attachment and offer no way to remove one.
func TestACarriedElementDoesNotShipAnEmbeddedFileThroughItsAF(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[32] = "<< /Type /StructElem /S /Para /P 31 0 R /Pg 3 0 R /K [0] /AF [50 0 R] >>"
	objs[50] = "<< /Type /Filespec /F (ZZFILENAMEMARKER.csv) /EF << /F 51 0 R >> >>"
	objs[51] = fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\nZZPAYLOADMARKER\nendstream", len("ZZPAYLOADMARKER\n"))
	src := assembleFixture(objs)
	if raw, streams := fileCarries(src, "ZZPAYLOADMARKER"); !raw && streams == 0 {
		t.Fatal("setup: the payload is not in the source, so its absence below proves nothing")
	}
	out, err := Collect(src, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); !s.tree {
		t.Fatalf("the carry was refused (%+v). This reads as a setup failure and is more likely a "+
			"REGRESSION: `subsetCarrying` turns any completeness defect into a refusal, so a carry "+
			"broken anywhere lands here rather than on the assertion below", s)
	}
	for _, m := range []string{"ZZPAYLOADMARKER", "ZZFILENAMEMARKER"} {
		if raw, streams := fileCarries(out, m); raw || streams > 0 {
			t.Errorf("%q reached the output through a carried element's /AF (raw=%v streams=%d). "+
				"pruneNames deletes /EmbeddedFiles from every subset because an extract shipped the "+
				"source's attachments with the payload readable in the bytes; /AF re-opens that door "+
				"below the name tree, where Attachments() cannot even see it", m, raw, streams)
		}
	}
	if names, aerr := Attachments(out); aerr == nil && len(names) > 0 {
		t.Errorf("the output reports %d attachment(s)", len(names))
	}
}

// TestACarriedTreeDropsAnIDTreeThatWouldReanchorARemovedElement.
//
// An `/IDTree` entry naming an element the prune removed keeps that element alive, and the element's
// `/Pg` keeps a DROPPED page dictionary and its `/Contents` alive — through a key no walk of `/K` can
// see, because an unreachable element is one `readStructTree` never visits and so one completeness
// condition 2 cannot fire on.
func TestACarriedTreeDropsAnIDTreeThatWouldReanchorARemovedElement(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[30] = "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R /RoleMap 38 0 R " +
		"/ParentTreeNextKey 6 /IDTree 52 0 R >>"
	objs[35] = "<< /Type /StructElem /S /P /P 31 0 R /Pg 7 0 R /K [0] /ID (ZZIDMARKER) >>"
	objs[52] = "<< /Names [(ZZIDMARKER) 35 0 R] >>"
	src := assembleFixture(objs)
	out, err := Collect(src, []string{"1", "2"}) // page 3, which element 35 describes, goes
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); !s.tree {
		t.Fatalf("the carry was refused (%+v). This reads as a setup failure and is more likely a "+
			"REGRESSION: `subsetCarrying` turns any completeness defect into a refusal", s)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	if orphans := orphanPageObjects(ctx, livePages(ctx)); len(orphans) > 0 {
		t.Errorf("page object(s) %v are still in the file, re-anchored by an /IDTree entry naming a "+
			"removed element", orphans)
	}
	if raw, streams := fileCarries(out, "ZZPAGETHREE"); raw || streams > 0 {
		t.Errorf("the dropped page's text is in the output (raw=%v streams=%d)", raw, streams)
	}
	root, _ := ctx.XRefTable.Catalog()
	if st := derefDict(ctx.XRefTable, root["StructTreeRoot"]); st != nil {
		if _, has := st["IDTree"]; has {
			t.Error("the carried root kept its /IDTree. Nothing in this repo reads or writes one, " +
				"and an /IDTree naming elements a subset removed is wrong whatever else is true of it")
		}
	}
}

// TestASubsetErasesASignatureItsStructureTreeStillNamed.
//
// `dropSignature` strips a signature widget from `/Annots` and its field from `/Fields` but removes no
// objects, and pdfcpu writes by reachability — so a `/Form` element's OBJR still naming the widget
// re-anchors it, and with it the `/V` signature dictionary: the signer's name, the date, and the
// PKCS#7 blob carrying their certificate. PDF/UA 7.18.1 requires that element, and nib's own
// `AuthorTaggedForm` writes exactly this shape.
func TestASubsetErasesASignatureItsStructureTreeStillNamed(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R " +
		"/Lang (en-GB) /Names 40 0 R /ViewerPreferences 41 0 R /Metadata 43 0 R " +
		"/AcroForm << /Fields [60 0 R] /SigFlags 3 >> >>"
	// The widget is merged with its field and sits on page 1, which this test KEEPS.
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> " +
		"/Contents 4 0 R /StructParents 0 /Annots [20 0 R 60 0 R] >>"
	objs[60] = "<< /Type /Annot /Subtype /Widget /FT /Sig /T (ZZFIELDMARKER) /Rect [0 0 10 10] " +
		"/P 3 0 R /StructParent 6 /V 61 0 R >>"
	objs[61] = "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached " +
		"/Name (ZZSIGNERMARKER) /M (D:20260101000000Z) /ByteRange [0 100 200 300] /Contents <5A5A5349474E> >>"
	// A /Form element whose OBJR names the widget — the re-anchoring route.
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R 62 0 R] >>"
	objs[62] = "<< /Type /StructElem /S /Form /P 31 0 R /Pg 3 0 R /K [<< /Type /OBJR /Obj 60 0 R /Pg 3 0 R >>] >>"
	objs[39] = "<< /Nums [0 [32 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R] 4 34 0 R 5 36 0 R 6 62 0 R] >>"
	src := assembleFixture(objs)
	for _, m := range []string{"ZZSIGNERMARKER", "ZZFIELDMARKER", "adbe.pkcs7.detached"} {
		if raw, streams := fileCarries(src, m); !raw && streams == 0 {
			t.Fatalf("setup: %q is not in the source, so its absence below proves nothing", m)
		}
	}
	out, err := Collect(src, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); !s.tree {
		t.Fatalf("the carry was refused (%+v). This reads as a setup failure and is more likely a "+
			"REGRESSION: `subsetCarrying` turns any completeness defect into a refusal", s)
	}
	for _, m := range []string{"ZZSIGNERMARKER", "ZZFIELDMARKER", "adbe.pkcs7.detached"} {
		if raw, streams := fileCarries(out, m); raw || streams > 0 {
			t.Errorf("%q survived a page operation (raw=%v streams=%d). dropSignature removes no "+
				"OBJECTS — it unlinks — so an OBJR naming the widget is enough to put the signature "+
				"blob and the signer's certificate back in a document that says it was never signed",
				m, raw, streams)
		}
	}
}

// TestACarriedSubsetRebuildsItsMetadataRatherThanInheritingIt.
//
// `selectPages` empties the document `/Info` and clears the trailer's permanent `/ID[0]` so an
// extract does not travel with the source's identity. XMP carries the same facts and more, so the
// title is taken and a fresh packet written — the allowlist principle applied inside the packet.
func TestACarriedSubsetRebuildsItsMetadataRatherThanInheritingIt(t *testing.T) {
	src := subsetFixture()
	provenance := []string{"ZZCREATORMARKER", "ZZDESCRIPTIONMARKER", "ZZDOCIDMARKER", "ZZHISTORYMARKER"}
	for _, m := range append([]string{"ZZTITLEMARKER"}, provenance...) {
		if raw, streams := fileCarries(src, m); !raw && streams == 0 {
			t.Fatalf("setup: %q is not in the source packet", m)
		}
	}
	out, err := Collect(src, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); !s.tree {
		t.Fatalf("the carry was refused (%+v). This reads as a setup failure and is more likely a "+
			"REGRESSION: `subsetCarrying` turns any completeness defect into a refusal", s)
	}
	if raw, streams := fileCarries(out, "ZZTITLEMARKER"); !raw && streams == 0 {
		t.Error("the carried subset lost the document's dc:title, which PDF/UA 7.1 t8 and t9 require " +
			"and which is not page-indexed")
	}
	for _, m := range provenance {
		if raw, streams := fileCarries(out, m); raw || streams > 0 {
			t.Errorf("%q travelled into the subset's XMP (raw=%v streams=%d). /Info is emptied and "+
				"the trailer's /ID[0] cleared six lines away for exactly this reason; carrying the "+
				"packet whole restores through one door what another strips", m, raw, streams)
		}
	}
}

// TestACarriedSubsetPRESERVESDisplayDocTitleAndNothingElse — `/ViewerPreferences` key by key, in both
// directions, over the three things a source can say.
//
// **The first cut of this could not fail for the reason it named.** The fixture already said
// `/DisplayDocTitle true`, so "carried" and "asserted unconditionally" were indistinguishable — and
// measured, the code was asserting: a source saying `false` and a source saying nothing both came out
// `true`. A subset that writes `true` over an author's explicit `false` is authoring a preference,
// which is the one thing this whole slice is built not to do.
func TestACarriedSubsetPRESERVESDisplayDocTitleAndNothingElse(t *testing.T) {
	for _, c := range []struct {
		name string
		vp   string
		want string // "true", "false" or "absent"
	}{
		{"the source says true", "<< /DisplayDocTitle true /PrintPageRange [3 7] /NumCopies 3 >>", "true"},
		{"the source says false", "<< /DisplayDocTitle false /PrintPageRange [3 7] >>", "false"},
		{"the source says nothing", "<< /PrintPageRange [3 7] /NumCopies 3 >>", "absent"},
	} {
		objs := subsetFixtureObjects()
		objs[41] = c.vp
		out, err := Collect(assembleFixture(objs), []string{"1", "2"})
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !inspectTags(out).tree {
			t.Errorf("%s: the carry was refused, so /ViewerPreferences is not being graded", c.name)
			continue
		}
		ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
		if rerr != nil {
			t.Errorf("%s: %v", c.name, rerr)
			continue
		}
		root, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			t.Errorf("%s: %v", c.name, cerr)
			continue
		}
		vp := derefDict(ctx.XRefTable, root["ViewerPreferences"])
		got := "absent"
		if vp != nil {
			if b := readBool(ctx.XRefTable, vp["DisplayDocTitle"]); b {
				got = "true"
			} else if _, has := vp["DisplayDocTitle"]; has {
				got = "false"
			}
		}
		if got != c.want {
			t.Errorf("%s: the subset says /DisplayDocTitle %s, want %s. A subset states what the "+
				"document stated — writing `true` over an author's `false`, or inventing the key "+
				"where the source asked for nothing, is authoring a preference rather than carrying "+
				"one", c.name, got, c.want)
		}
		// Whatever it said about the title, the page-indexed keys never travel.
		for _, k := range []string{"PrintPageRange", "NumCopies"} {
			if vp != nil {
				if _, has := vp[k]; has {
					t.Errorf("%s: /%s was carried into a two-page extract of a four-page document. "+
						"/PrintPageRange names page INDICES, which is why /PageLabels is dropped "+
						"explicitly; a dictionary carried whole is a denylist of the keys somebody "+
						"thought of", c.name, k)
				}
			}
		}
	}
}

// TestACarriedSubsetKeepsItsThreeTitlePlacesInAgreement — `SetTitle`'s one-door rule, kept by the
// second door rather than broken by it.
//
// That door's header: *"a document carrying two of them is a worse state than one carrying none — a
// viewer told to display a title it cannot find shows an empty chrome bar."* The first cut wrote the
// XMP packet and the preference and skipped `/Info`'s `/Title`, producing exactly that state.
// `/Info` is still emptied first, so only the title comes back.
func TestACarriedSubsetKeepsItsThreeTitlePlacesInAgreement(t *testing.T) {
	out, err := Collect(subsetFixture(), []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if !inspectTags(out).tree {
		t.Fatal("the carry was refused, so there is no title floor to grade")
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	root, _ := ctx.XRefTable.Catalog()
	if got := documentTitle(ctx, root); got != "ZZTITLEMARKER" {
		t.Errorf("the carried XMP says dc:title %q, want the source's", got)
	}
	if ctx.Info == nil {
		t.Fatal("the output has no /Info dictionary at all, so /Title cannot agree with the packet")
	}
	info := derefDict(ctx.XRefTable, *ctx.Info)
	// `setInfoTitle` writes UTF-16 (`types.EncodeUTF16String`), so the raw value carries a BOM and
	// NUL-interleaved bytes; the marker is what has to be in there.
	title := fmt.Sprint(info["Title"])
	if !strings.Contains(strings.ReplaceAll(title, "\x00", ""), "ZZTITLEMARKER") {
		t.Errorf("/Info /Title is %q and the XMP packet says ZZTITLEMARKER. That is the two-of-three "+
			"state SetTitle's one-door rule exists to prevent: a viewer that prefers /Info over XMP "+
			"shows the filename while the document asks it to display a title", title)
	}
	// And only the TITLE came back. `/Producer`, `/CreationDate` and `/ModDate` are pdfcpu's own,
	// written at write time — measured, identical on the non-carrying door and naming pdfcpu rather
	// than the source's producer — so the thing to assert is that the source's own keys stayed gone.
	if _, has := info["NibFlags"]; has {
		t.Error("/Info kept nib's own NibFlags. It is emptied so a derived artifact does not travel " +
			"with the source's state; the title floor re-adds the title and nothing else")
	}
	if p := fmt.Sprint(info["Producer"]); strings.Contains(p, "ZZ") {
		t.Errorf("/Info /Producer is %q — the SOURCE's, which a derived artifact must not carry", p)
	}
}

// TestACarriedSubsetRenumbersItsParentTreeKeys — the source's keys ARE the source's page indices.
//
// Measured on nib's own six-page document, keeping pages 4 and 5: the output's two pages carried
// `/StructParents` 3 and 4 over `/Nums` keys `[3 4]` with `/ParentTreeNextKey` still 6. A two-page
// document declaring six spent keys says "I was pages 4 and 5 of something with six".
func TestACarriedSubsetRenumbersItsParentTreeKeys(t *testing.T) {
	out, err := Collect(subsetFixture(), []string{"3", "4"}) // the LAST two pages
	if err != nil {
		t.Fatal(err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatalf("the carried tree does not parse: %v", terr)
	}
	arrays, singles := parentTreeEntries(ctx, tree)
	var keys []int
	for k := range arrays {
		keys = append(keys, k)
	}
	for k := range singles {
		keys = append(keys, k)
	}
	for _, k := range keys {
		if k >= len(keys) {
			t.Errorf("the output's /ParentTree keys are %v — key %d is past the %d entries it has, "+
				"so the keys are still the SOURCE's page indices and the document declares how many "+
				"pages it was cut from", keys, k, len(keys))
		}
	}
	nk, ok := pdfNumber(ctx.XRefTable, tree.root["ParentTreeNextKey"])
	if !ok {
		t.Fatal("the carried root has no /ParentTreeNextKey; the source had one and a stale-low " +
			"value hands the next tool a key nib has already used")
	}
	if int(nk) != len(keys) {
		t.Errorf("/ParentTreeNextKey is %d over %d entries, want %d — it declares the source's key "+
			"count, which is the source's page count", int(nk), len(keys), len(keys))
	}
}

// TestTheNonCarryingDoorIsWhereEveryCOMPOSITIONRoutes extends P02.S03's routing guard to the second
// reason for being on that door, and closes the S04a inventory's G6 gap: the original guard read
// `RedactPages`' callees only, so it said nothing about what the non-carrying door itself calls, nor
// about the composition sites.
func TestTheNonCarryingDoorIsWhereEveryCOMPOSITIONRoutes(t *testing.T) {
	calls := callsIn(t, "pdfops.go")
	if calls["collectWithoutStructure"] == nil {
		t.Fatal("collectWithoutStructure is gone, so there is no door for anything to route through")
	}
	// **The door itself must not route through the carrying primitive** — G6. It shares the selection
	// RULE with `Collect` (`collectPick`) and deliberately not the door.
	if calls["collectWithoutStructure"]["subsetCarrying"] || calls["collectWithoutStructure"]["Collect"] {
		t.Error("the non-carrying door calls the carrying one. Every caller that routes through it " +
			"to refuse the carry would inherit it, and the routing guard above cannot see that: it " +
			"reads RedactPages' callees, not this function's")
	}
	// **The list is ENUMERATED from the code and then dispositioned**, not hand-written. A
	// hand-written list of four is complete exactly until someone adds a fifth site, which is
	// ADR-009's own objection to checking copies for agreement.
	//
	// Three dispositions, and every function calling either door must have one:
	//   - COMPOSITION: it merges, cuts or normalises its subset's result, so it must not carry.
	//   - SUBSET: nothing is composed after it, so it carries (the two file splitters, and the
	//     server and CLI doors themselves).
	//   - BEHIND ANOTHER GATE: `Booklet` is `InsertBlank`×pad → `Collect` → `NUp`, and `NUp`'s own
	//     `completeOrHonest` stands behind it, so a carry that the composition breaks is dropped
	//     there rather than shipped. Measured: `carried` on the census document, `dropped` on a
	//     fixture whose annotations `api.NUp` drops.
	composition := map[string]bool{
		"splice": true, "normalizePage": true, "SplitRegions": true, "SplitPage": true,
	}
	subsetOnly := map[string]bool{
		"SplitByBookmarks": true, "SplitBySpans": true, "DuplicatePage": true,
	}
	behindAnotherGate := map[string]bool{"Booklet": true}
	var undispositioned []string
	for fn, calls := range calls {
		if !calls["Collect"] && !calls["collectWithoutStructure"] && !calls["subsetCarrying"] &&
			!calls["subset"] {
			continue
		}
		switch {
		case fn == "Collect" || fn == "RemovePages" || fn == "collectWithoutStructure" ||
			fn == "subsetCarrying" || fn == "subset":
			// the doors themselves
		case composition[fn] || subsetOnly[fn] || behindAnotherGate[fn] || fn == "RedactPages":
			// dispositioned below, or redaction, which the sibling guard owns
		default:
			undispositioned = append(undispositioned, fn)
		}
	}
	sort.Strings(undispositioned)
	if len(undispositioned) > 0 {
		t.Errorf("%v call a page-subset door and are dispositioned nowhere in this guard. Every "+
			"caller is one of three things — a COMPOSITION (must not carry), a plain SUBSET (carries), "+
			"or behind another gate — and a caller with no disposition is a carry nobody decided",
			undispositioned)
	}
	for _, fn := range []string{"splice", "normalizePage", "SplitRegions", "SplitPage"} {
		if calls[fn] == nil {
			t.Errorf("%s is not declared in pdfops.go, so its route is unchecked", fn)
			continue
		}
		if calls[fn]["Collect"] {
			t.Errorf("%s calls Collect, which carries the source structure tree since P02.S04b. "+
				"api.MergeRaw keeps only the FIRST document's catalog and /ParentTree, and CutPage "+
				"leaves /StructParents on tiles whose tree it destroyed — so a carry reaching here "+
				"makes a page assert another page's words. P02.S05/S06/S07 own that question and are "+
				"blocked", fn)
		}
		if !calls[fn]["collectWithoutStructure"] {
			t.Errorf("%s routes through neither door; a subset it performs is ungraded", fn)
		}
	}
}

// TestTheCarryIsSkippedEntirelyForAnUntaggedDocument — the precondition, not a fast path.
//
// `carryIsComplete` answers FALSE for an untagged document (`readStructTree` returns
// `errNoStructTree`), so a gate that ran unconditionally would fall back to a second full
// read-change-write on every untagged document forever.
func TestTheCarryIsSkippedEntirelyForAnUntaggedDocument(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if inspectTags(src).claims() {
		t.Fatal("setup: the fixture claims tagging, so it cannot exercise the untagged path")
	}
	if carryIsComplete(src) {
		t.Fatal("setup: carryIsComplete says an untagged document is complete, so the fallback " +
			"below could never have fired and this test asserts nothing")
	}
	out, err := Collect(src, []string{"3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if n, perr := PageCount(out); perr != nil || n != 2 {
		t.Fatalf("the subset produced %d page(s) (err %v), want 2", n, perr)
	}
	if inspectTags(out).claims() {
		t.Error("a subset of an untagged document claims tagging")
	}
	// **The observable half, because the promised property is a COST and nothing reads a cost.**
	// The earlier version's doc promised that an unconditional gate would pay a second full write on
	// every untagged document forever — measured, replacing the gate with one that always runs left
	// this test green, because both paths emit the same document. What IS observable is that the
	// carrying door produced exactly what the non-carrying door produces: same catalog, same page
	// count, same bytes but for the `/ID` and dates pdfcpu mints per write.
	plain, perr := collectWithoutStructure(src, []string{"3", "1"})
	if perr != nil {
		t.Fatal(perr)
	}
	if got, want := catalogKeys(t, out), catalogKeys(t, plain); got != want {
		t.Errorf("the carrying door left catalog %s and the non-carrying door %s. For a document "+
			"with no tree there is nothing to carry, so the two must be the same document — and if "+
			"they are not, the carry ran on a document that has no structure to carry", got, want)
	}
	if len(out) < len(plain)-64 || len(out) > len(plain)+64 {
		t.Errorf("the carrying door wrote %d bytes and the non-carrying door %d; for an untagged "+
			"document they should differ only by the /ID and the dates pdfcpu mints per write",
			len(out), len(plain))
	}
}

// catalogKeys renders a document's catalog key set, sorted, for comparing two doors' output.
func catalogKeys(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		t.Fatalf("catalog: %v", cerr)
	}
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprint(keys)
}

// TestTheCarryKeepsAlternateTextAndDropsATitleItNoLongerCovers — the declared limit.
//
// `/Alt` and `/ActualText` are accessibility payload and are KEPT: dropping them loses the reader
// who needs them, and a truncated `/ActualText` would be a false statement rather than a lossy one.
// `/T` is a human title for a span the element no longer wholly covers, and it goes.
func TestTheCarryKeepsAlternateTextAndDropsATitleItNoLongerCovers(t *testing.T) {
	objs := subsetFixtureObjects()
	// A /Sect over pages 1 and 2, described as covering both, with a title.
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [70 0 R 34 0 R 35 0 R 36 0 R 37 0 R] >>"
	objs[70] = "<< /Type /StructElem /S /Sect /P 31 0 R /K [32 0 R 33 0 R] " +
		"/Alt (ZZALTMARKER) /ActualText (ZZACTUALMARKER) /T (ZZTITLEDMARKER) >>"
	objs[32] = "<< /Type /StructElem /S /Para /P 70 0 R /Pg 3 0 R /K [0] >>"
	objs[33] = "<< /Type /StructElem /S /P /P 70 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 5 0 R /MCID 0 >>] >>"
	src := assembleFixture(objs)
	for _, m := range []string{"ZZALTMARKER", "ZZACTUALMARKER", "ZZTITLEDMARKER"} {
		if raw, streams := fileCarries(src, m); !raw && streams == 0 {
			t.Fatalf("setup: %q is not in the source", m)
		}
	}
	// Keep page 1 only: the /Sect loses its MCR kid on page 2, so the prune CHANGED it.
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); !s.tree {
		t.Fatalf("the carry was refused (%+v). This reads as a setup failure and is more likely a "+
			"REGRESSION: `subsetCarrying` turns any completeness defect into a refusal", s)
	}
	for _, m := range []string{"ZZALTMARKER", "ZZACTUALMARKER"} {
		if raw, streams := fileCarries(out, m); !raw && streams == 0 {
			t.Errorf("%q was dropped. /Alt and /ActualText are what a screen reader speaks, and a "+
				"subset losing them is a loss to the reader who needs them most", m)
		}
	}
	if raw, streams := fileCarries(out, "ZZTITLEDMARKER"); raw || streams > 0 {
		t.Errorf("/T survived on an element the prune changed (raw=%v streams=%d). It names a span "+
			"the element no longer wholly covers, and nothing a reader depends on reads it", raw, streams)
	}
}

// TestTheCensusRowsThatFlippedSayWhichReaderGradesThem is a guard on this file's own premise: the
// four census rows now declaring `carried` are measured on a ONE-page fixture, so the prune they
// describe is graded here and nowhere else.
func TestTheCensusRowsThatFlippedSayWhichReaderGradesThem(t *testing.T) {
	n, err := PageCount(taggedFixture())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Skipf("the corpus fixture now has %d pages; if it can have a page taken out of it, the "+
			"census is no longer blind to the prune and this note can go", n)
	}
	for _, name := range []string{"Collect", "RemovePages", "DuplicatePage", "Booklet"} {
		f, ok := tagFates[name]
		if !ok {
			t.Fatalf("%s has no census row", name)
		}
		if f.verdict != "carried" {
			t.Errorf("%s declares %q; P02.S04b's acceptance says carried", name, f.verdict)
		}
	}
	if strings.Contains(fmt.Sprint(knownUA1Deltas["Collect"].clauses), "7.1") {
		t.Error("`knownUA1Deltas` still records a structure loss for Collect")
	}
}

// TestASubsetPRESERVESItsInputsFateRatherThanSettingIt — the invariant the census's single declared
// verdict cannot express, and the finding that produced it.
//
// **`Collect` declares `carried`, and that is true of the census fixture and not of every input.**
// Before this slice `dropped` was unconditional: the tree went whatever the document was. Now the
// output's fate follows the INPUT's, which is the honest property and is what the census row should
// be read as. Measured on `Append(taggedFixture, untagged)` — the shape `api.MergeRaw` produces and
// the one every ceremony document takes (ADR-031's recorded `partial` decision):
//
//	merged source                  partial
//	reorder both pages             partial   (undescribed=1, complete)
//	keep both pages                partial   (undescribed=1, complete)
//	keep the described page only    carried
//	keep the undescribed page only  dropped   <- the anchors-nothing refusal
//
// **A subset does not MAKE a partial document, and that distinction is the whole disposition.** The
// appended page was already undescribed under `/MarkInfo /Marked true` before any subset ran; what
// the old behaviour did was launder that by destroying the whole live tree, which ADR-031 records as
// the cure being worse than the disease at the only scale that matters. Preserving it is continuity
// with a decision already taken. And where the subset keeps ONLY pages nothing describes, it does
// better than preserve: the carry refuses and the claim goes.
func TestASubsetPRESERVESItsInputsFateRatherThanSettingIt(t *testing.T) {
	merged, err := Append(taggedFixture(), untaggedFixture())
	if err != nil {
		t.Fatal(err)
	}
	if fate(merged) != "partial" {
		t.Fatalf("setup: the merged document is %q, want partial — `api.MergeRaw` taking the first "+
			"catalog whole is what makes this fixture, and without it every row below is about a "+
			"fully-described document and asserts nothing new", fate(merged))
	}
	for _, c := range []struct {
		name  string
		order []string
		want  string
	}{
		{"a reorder of a partial document stays partial", []string{"2", "1"}, "partial"},
		{"keeping every page of one stays partial", []string{"1-"}, "partial"},
		{"keeping only the described page is carried", []string{"1"}, "carried"},
		{"keeping only the undescribed page DROPS the claim", []string{"2"}, "dropped"},
	} {
		out, oerr := Collect(merged, c.order)
		if oerr != nil {
			t.Errorf("%s: %v", c.name, oerr)
			continue
		}
		if got := fate(out); got != c.want {
			s := inspectTags(out)
			t.Errorf("%s: measured %q, want %q (%+v).\n\tA subset carries the structure of the pages "+
				"it keeps, so its fate follows its input's — it neither invents a claim nor launders "+
				"one by destroying a live tree. The last row is the exception and is the stronger "+
				"direction: a carry that anchors nothing is refused.", c.name, got, c.want, s)
		}
		// Whatever the fate, the tree the output DOES carry must be complete: a partial output is a
		// live tree that does not reach every page, never a broken one.
		if inspectTags(out).tree {
			if !carryIsComplete(out) {
				t.Errorf("%s: the output carries a tree that is not complete", c.name)
			}
		}
	}
}

// TestTheOutputGateFallsBackToTheHonestLoss — the branch the whole gate design rests on, and it had
// no reader until the slice's own code review asked for one.
//
// `subsetCarrying` writes, re-reads, and on an incomplete carry re-runs the selection with the carry
// off. The shape that reaches it is the one `subsetCarrying`'s header cites: a page whose content
// draws an MCID-bearing form XObject, DUPLICATED — so the form is drawn twice, its marked content
// has two semantic parents, and one `/StructParents` key cannot name them both (completeness
// condition 4, veraPDF's 7.20 t2). An in-context check cannot see it, because it is pdfcpu's
// optimizing READ of the written bytes that reveals the shared draw.
func TestTheOutputGateFallsBackToTheHonestLoss(t *testing.T) {
	// One page drawing a form XObject that carries the marked content, so duplicating the page
	// draws the same form twice.
	form := "/P <</MCID 0>> BDC\nBT /F1 18 Tf 72 700 Td (ZZINFORM) Tj ET\nEMC\n"
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		// **The FORM carries `/StructParents`, not the page**: the marked content is in the form's
		// stream, so the form is what owns the key. Giving the page one too makes two owners of key
		// 0 — completeness condition 5 — and the fixture would arrive broken, so the fallback below
		// could be firing for that rather than for the duplicated draw.
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /XObject << /Fm1 12 0 R >> /Font << /F1 11 0 R >> >> /Contents 4 0 R >>",
		4:  streamObj("q 1 0 0 1 0 0 cm /Fm1 Do Q\n"),
		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		12: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /StructParents 0 "+
			"/Resources << /Font << /F1 11 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
		30: "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R >>",
		31: "<< /Type /StructElem /S /P /P 30 0 R /Pg 3 0 R /K [0] >>",
		39: "<< /Nums [0 [31 0 R]] >>",
	})
	// **Stimulus before response.** The source must be a complete carry, or the fallback below could
	// be firing for a defect the fixture arrived with rather than for the duplicated draw.
	if v, d, _, _ := carryOf(t, src); v != "carried" || len(d) > 0 {
		t.Fatalf("setup: the fixture is %s with %d defect(s); it has to be a clean carried document "+
			"for the duplicate to be the thing that breaks it", v, len(d))
	}
	if !carryIsComplete(src) {
		t.Fatal("setup: the fixture's own carry is not complete")
	}
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n, perr := PageCount(out); perr != nil || n != 2 {
		t.Fatalf("the duplicate produced %d page(s) (err %v), want 2", n, perr)
	}
	// The gate refused the carry, so the claim went with it: an honest loss, not a partial tree.
	if s := inspectTags(out); s.tree || s.marked {
		t.Errorf("the duplicate shipped a tagging claim (tree=%v marked=%v) over a form XObject drawn "+
			"twice. One /StructParents key cannot name two semantic parents — that is completeness "+
			"condition 4 — so the gate should have re-run the selection with the carry off and "+
			"produced the honest loss", s.tree, s.marked)
	}
	// And it is still a working document: the fallback is a subset, not a failure.
	if raw, streams := fileCarries(out, "ZZINFORM"); !raw && streams == 0 {
		t.Error("the fallback lost the page's content, so it produced something worse than an " +
			"honest loss")
	}
}

// TestTheClaimantWalksAgreeOnWhoOwnsAKey — the one exemption this slice took from ADR-009, asserted.
//
// `parentTreeOwners` (the read side, shared with the completeness predicate and the key allocator)
// and `eachParentTreeClaim` (the write side, which needs a setter per claim) walk the same three
// places: a page's `/StructParents`, an annotation's `/StructParent`, and a form XObject's, in both
// spellings. The plan's T02 said the carry would SHARE the read-side walk; it does not, because
// renumbering needs to write back and `parentTreeOwners` returns strings. So the exemption is
// declared — and this is what stops the two drifting, which a declaration alone does not.
func TestTheClaimantWalksAgreeOnWhoOwnsAKey(t *testing.T) {
	for _, name := range []string{"the rich fixture", "a duplicate", "a subset"} {
		var pdf []byte
		var err error
		switch name {
		case "the rich fixture":
			pdf = subsetFixture()
		case "a duplicate":
			pdf, err = DuplicatePage(subsetFixture(), 1)
		case "a subset":
			pdf, err = Collect(subsetFixture(), []string{"2", "1"})
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
		if rerr != nil {
			t.Fatalf("%s: %v", name, rerr)
		}
		readSide := map[int]int{}
		for k, who := range parentTreeOwners(ctx) {
			readSide[k] = len(who)
		}
		writeSide := map[int]int{}
		eachParentTreeClaim(ctx, func(key int, _ func(int)) { writeSide[key]++ })
		if len(readSide) == 0 {
			t.Errorf("%s: neither walk found a claimant, so the comparison below is vacuous", name)
			continue
		}
		if fmt.Sprint(readSide) != fmt.Sprint(writeSide) {
			t.Errorf("%s: the read-side walk answers %v and the write-side walk %v.\n\tADR-009: one "+
				"rule, one answer. A write side that misses a claim deletes a key the read side "+
				"still counts, which silently drops the carry to the honest loss; the other "+
				"direction ships a row nothing can reach", name, readSide, writeSide)
		}
	}
}

// TestACarriedTreeKeepsItsRoleMap — a property nothing guarded, found by mutation.
//
// **Measured: adding `delete(tree.root, "RoleMap")` beside the `/IDTree` drop left the ENTIRE
// `internal/pdfops` suite green**, veraPDF differential included. `subsetFixture`'s own doc names a
// RoleMap as one of the shapes the census lacks, and the census document genuinely has none — so
// obj 38 was the only RoleMap in the repo and nothing read it. A lost RoleMap makes a carried subset
// of a real LibreOffice or Word document carry element types no reader can interpret: `/Para` means
// nothing without the map that says it is a `/P`.
func TestACarriedTreeKeepsItsRoleMap(t *testing.T) {
	src := subsetFixture()
	before := roleMapOf(t, src)
	if len(before) == 0 {
		t.Fatal("setup: the fixture carries no RoleMap, so this test grades nothing")
	}
	for _, c := range []struct {
		name string
		op   func([]byte) ([]byte, error)
	}{
		{"a prune", func(b []byte) ([]byte, error) { return Collect(b, []string{"1", "2"}) }},
		{"a reorder", func(b []byte) ([]byte, error) { return Collect(b, []string{"2", "1"}) }},
		{"a delete", func(b []byte) ([]byte, error) { return RemovePages(b, []string{"4"}) }},
		{"a duplicate", func(b []byte) ([]byte, error) { return DuplicatePage(b, 1) }},
	} {
		out, err := c.op(src)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !inspectTags(out).tree {
			t.Errorf("%s: the carry was refused, so the RoleMap below is not being graded", c.name)
			continue
		}
		if got := roleMapOf(t, out); fmt.Sprint(got) != fmt.Sprint(before) {
			t.Errorf("%s: the carried RoleMap is %v, want the source's %v.\n\tAn element typed "+
				"/Para means nothing without the map that says it is a /P, and the carry authors no "+
				"element types of its own — so the map has to come through whole", c.name, got, before)
		}
	}
}

// roleMapOf reads a document's `/StructTreeRoot /RoleMap`, by parsing.
func roleMapOf(t *testing.T, pdf []byte) map[string]string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tree, terr := readStructTree(ctx, livePages(ctx))
	if terr != nil {
		return nil
	}
	return tree.roleMap
}

// TestTheOrphanPageConditionDECIDESTheGate — the conjunct's own stimulus.
//
// `carryIsComplete` gained "no `/Type /Page` object outside the page tree", and every assertion that
// touches it asserts ZERO — so the detector had never been seen to decide anything. Measured: with
// the conjunct removed, the whole `internal/pdfops` suite still passed. It is not inert (it returns
// a non-empty list on the right input); it simply had no input.
//
// The stimulus is a kept page's annotation whose `/P` names a page the subset DROPPED. Nothing
// prunes an annotation's `/P` — it is a malformed source rather than a shape nib produces — and
// pdfcpu writes by reachability, so the dropped page's dictionary and its `/Contents` reach the
// output while the TREE stays clean, which is precisely the gap the five tree conditions cannot see.
func TestTheOrphanPageConditionDECIDESTheGate(t *testing.T) {
	objs := subsetFixtureObjects()
	// The Link on kept page 1, repointed at dropped page 3 — and its /Dest removed, so
	// `unlinkDestinations` is not what catches this.
	objs[20] = "<< /Type /Annot /Subtype /Link /Rect [72 690 300 720] /Border [0 0 0] " +
		"/P 7 0 R /StructParent 4 >>"
	src := assembleFixture(objs)
	if v, d, _, _ := carryOf(t, src); v != "carried" || len(d) > 0 {
		t.Fatalf("setup: the fixture is %s with %d defect(s)", v, len(d))
	}
	out, err := Collect(src, []string{"1", "2"}) // page 3 goes, and the annotation still names it
	if err != nil {
		t.Fatal(err)
	}
	// The gate refused, so the claim went with it — the honest loss.
	if s := inspectTags(out); s.tree || s.marked {
		t.Errorf("the output claims tagging (tree=%v marked=%v) over a document that still holds a "+
			"page the subset removed", s.tree, s.marked)
	}
	// And the conjunct is what decided it: the TREE was clean.
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	if orphans := orphanPageObjects(ctx, livePages(ctx)); len(orphans) == 0 {
		t.Error("no orphan page object in the output, so the condition under test had nothing to " +
			"fire on and this test would pass with the conjunct deleted")
	}
}

// TestPDFCPURefusesATypelessPageNode — why `orphanPageObjects` tests `/Type /Page` and nothing more.
//
// `collectLeaves` counts a node with no `/Type` and no `/Kids` as a leaf page, with a comment about
// why the two walks must agree — so a `/Type`-only orphan reader looks like a blind spot. It is not,
// because pdfcpu refuses such a node during the read and neither walk ever sees one. This asserts
// the REASON rather than the conclusion: if the reader relaxes, the blind spot becomes live and this
// goes red first.
func TestPDFCPURefusesATypelessPageNode(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[9] = "<< /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> " +
		"/Contents 10 0 R /StructParents 3 >>" // page 4, with no /Type
	_, err := api.ReadValidateAndOptimize(bytes.NewReader(assembleFixture(objs)),
		model.NewDefaultConfiguration())
	if err == nil {
		t.Error("pdfcpu now accepts a page-tree node with no /Type. `orphanPageObjects` tests " +
			"`/Type /Page` alone on the strength of that refusal, so such a page would now be a " +
			"page the walk counts and the orphan reader cannot see — give it collectLeaves' rule")
	}
}

// TestARepeatDoesNotRESURRECTARemovedElement — the ORDER fix, and the leak it closed.
//
// `carryOntoClone` copies the elements a `/ParentTree` row names. A row still naming an element the
// prune took out hands the clone path one to deep-copy — and a removed element kept the `/AF` and
// `/T` the prune strips only from survivors. Measured before `clearRemovedFromRows` was moved ahead
// of the clones: `Collect(src, ["1","1"])` shipped the source's embedded payload and its filename
// with `fate=carried`, **0** completeness defects and **0** orphan pages.
//
// **The single-selection arm cannot fail this**, which is why the `/AF` test alone did not catch it:
// it took a repeat to reach the clone path at all.
func TestARepeatDoesNotRESURRECTARemovedElement(t *testing.T) {
	objs := subsetFixtureObjects()
	// An element on page 1 whose only kid is an OBJR at an annotation the prune will remove — the
	// Link on page 3 — so the element itself is removed while page 1's row still names it.
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R 80 0 R] >>"
	objs[80] = "<< /Type /StructElem /S /Form /P 31 0 R /Pg 3 0 R /AF [81 0 R] /T (ZZDOOMEDTITLE) " +
		"/K [<< /Type /OBJR /Obj 22 0 R /Pg 7 0 R >>] >>"
	objs[81] = "<< /Type /Filespec /F (ZZDOOMEDFILE.csv) /EF << /F 82 0 R >> >>"
	objs[82] = fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\nZZDOOMEDPAYLOAD\nendstream",
		len("ZZDOOMEDPAYLOAD\n"))
	// Page 1's row names the doomed element, so a clone of page 1 would copy it.
	objs[39] = "<< /Nums [0 [32 0 R 80 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R] 4 34 0 R 5 36 0 R] >>"
	src := assembleFixture(objs)
	for _, m := range []string{"ZZDOOMEDPAYLOAD", "ZZDOOMEDFILE", "ZZDOOMEDTITLE"} {
		if raw, streams := fileCarries(src, m); !raw && streams == 0 {
			t.Fatalf("setup: %q is not in the source", m)
		}
	}
	// Keep page 1 TWICE: page 3 goes (so element 80 is removed) and page 1 is cloned.
	out, err := Collect(src, []string{"1", "1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	// **The discriminating assertion is the FATE, and finding that out is what the red probe was
	// for.** With `clearRemovedFromRows` disabled the clone path copies the removed element, the
	// output gate then refuses the whole carry, and the document comes out `dropped` — so the
	// residue never ships and a residue-only test passes for the wrong reason. Measured both ways:
	// `carried` with the cleaning, `dropped` without it. What the ordering buys is a carry that
	// SUCCEEDS where it would otherwise fall back to the honest loss.
	if v := fate(out); v != "carried" {
		t.Errorf("a repeat of a page whose /ParentTree row names a REMOVED element measured %q. The "+
			"clone path copies what the row names, so a row not cleaned first hands it an element "+
			"the prune took out — the gate then refuses the carry and the whole tree is lost", v)
	}
	// And the backstop, which says what the loss would have been if it had got past the gate: the
	// removed element carried an `/AF` at an embedded file and a `/T`, neither of which the prune
	// strips from an element it is not keeping.
	for _, m := range []string{"ZZDOOMEDPAYLOAD", "ZZDOOMEDFILE", "ZZDOOMEDTITLE"} {
		if raw, streams := fileCarries(out, m); raw || streams > 0 {
			t.Errorf("%q reached the output (raw=%v streams=%d) — the resurrected element's payload",
				m, raw, streams)
		}
	}
}

// TestAnElementUNDERTWOPARENTSIsJudgedPerParent — the memo key, and the order-dependence it fixed.
//
// An element with no `/Pg` of its own owns MCIDs on whichever page reached it, so its fate differs by
// parent. Memoizing on the object alone cached whichever judgment came first. Measured before the fix
// on this fixture: keeping page 1 came out `carried` and keeping page 2 came out **`dropped`** — the
// whole tree lost, because the shared element was judged under the dead page-1 context and the memo
// then killed both parents.
func TestAnElementUNDERTWOPARENTSIsJudgedPerParent(t *testing.T) {
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: streamObj(markedPage("ZZONEPAGE")),
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 6 0 R /StructParents 1 >>",
		6: streamObj(markedPage("ZZTWOPAGE")),

		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		30: "<< /Type /StructTreeRoot /K [31 0 R 33 0 R] /ParentTree 39 0 R >>",
		// Two parents, one on each page, both naming the SAME kid — which has no /Pg of its own.
		31: "<< /Type /StructElem /S /Sect /P 30 0 R /Pg 3 0 R /K [35 0 R] >>",
		33: "<< /Type /StructElem /S /Sect /P 30 0 R /Pg 5 0 R /K [35 0 R] >>",
		35: "<< /Type /StructElem /S /P /P 31 0 R /K [0] >>",
		39: "<< /Nums [0 [35 0 R] 1 [35 0 R]] >>",
	})
	// Both selections must carry. Whichever page the walk reaches first, the OTHER page's parent has
	// to be judged in its own context rather than inheriting the first's verdict.
	for _, c := range []struct {
		name  string
		order []string
	}{
		{"keeping the page the walk reaches first", []string{"1"}},
		{"keeping the page it reaches second", []string{"2"}},
	} {
		out, err := Collect(src, c.order)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if s := inspectTags(out); !s.tree {
			t.Errorf("%s: the whole tree was lost. An element reachable from two parents owns MCIDs "+
				"on whichever page reached it, so a fate memoized on the object alone applies one "+
				"parent's verdict to the other — and the loss is order-dependent, which is worse "+
				"than either answer", c.name)
		}
	}
}

// TestAGROUPINGFormElementWithNoPageSurvivesIfItsAnnotationDoes — the OBJR rule's own shape.
//
// `/Pg` is optional on both an element and an OBJR, and a grouping `/Form` element carrying neither
// is the shape PDF/UA asks for — `anchored()`'s kid-walk was added at P06.S07 for exactly it. A rule
// requiring a live page unconditionally inherits page 0, which is never live, so the element loses
// its only kid and goes even with its annotation on a kept page.
func TestAGROUPINGFormElementWithNoPageSurvivesIfItsAnnotationDoes(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 85 0 R] >>"
	// No /Pg on the element and none on the OBJR: the annotation is what anchors it.
	objs[85] = "<< /Type /StructElem /S /Form /P 31 0 R /K [<< /Type /OBJR /Obj 20 0 R >>] >>"
	objs[39] = "<< /Nums [0 [32 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R] 4 85 0 R 5 36 0 R] >>"
	src := assembleFixture(objs)
	out, err := Collect(src, []string{"1"}) // annotation 20 is on page 1, which is KEPT
	if err != nil {
		t.Fatal(err)
	}
	if !inspectTags(out).tree {
		t.Fatal("the carry was refused, so the element below is not being graded")
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	tree, terr := readStructTree(ctx, livePages(ctx))
	if terr != nil {
		t.Fatalf("the carried tree does not parse: %v", terr)
	}
	forms := 0
	for _, e := range tree.elems {
		if e.kind == "Form" {
			forms++
		}
	}
	if forms != 1 {
		t.Errorf("the carried tree has %d /Form element(s), want 1. A grouping element with no /Pg "+
			"whose OBJR also has none inherits page 0 — never live — so a rule that demands a live "+
			"page destroys the only correct way to describe an annotation", forms)
	}
}

// TestADuplicatedChildElementNamesTheCOPYSParent — `/P`, which no reader in this package checks.
//
// `Dict.Clone` copies `/P` with everything else, so a copied child pointed at the ORIGINAL's parent
// while living in the copy's `/K` — a tree that disagrees with itself in the two directions a reader
// walks it. `internal/uacheck`'s `declaresLangFor` climbs `/P`, and nothing here does, so it went
// unseen with `structureCarriedCompletely` reporting zero defects.
func TestADuplicatedChildElementNamesTheCOPYSParent(t *testing.T) {
	objs := subsetFixtureObjects()
	// **Page 1 gets TWO marked sequences and the row names the parent AND the child**, which is what
	// it takes to make the clone path RECURSE: the `/Sect` owns MCID 0 (so slot 0 may name it) and
	// its child owns MCID 1. A row naming only a leaf gives a copy whose parent was not copied, and
	// then keeping the original's `/P` is correct — so that shape cannot see this defect.
	objs[4] = streamObj("/P <</MCID 0>> BDC\nBT /F1 18 Tf 72 700 Td (ZZPAGEONE) Tj ET\nEMC\n" +
		"/P <</MCID 1>> BDC\nBT /F1 18 Tf 72 660 Td (ZZPAGEONEB) Tj ET\nEMC\n")
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [90 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R] >>"
	objs[90] = "<< /Type /StructElem /S /Sect /P 31 0 R /Pg 3 0 R /K [0 32 0 R] >>"
	objs[32] = "<< /Type /StructElem /S /Para /P 90 0 R /Pg 3 0 R /K [1] >>"
	objs[39] = "<< /Nums [0 [90 0 R 32 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R] 4 34 0 R 5 36 0 R] >>"
	src := assembleFixture(objs)
	if v, d, _, _ := carryOf(t, src); v != "carried" || len(d) > 0 {
		t.Fatalf("setup: the fixture is %s with %d defect(s): %v", v, len(d), d)
	}
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !inspectTags(out).tree {
		t.Fatal("the carry was refused, so no copy exists to grade")
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	tree, terr := readStructTree(ctx, livePages(ctx))
	if terr != nil {
		t.Fatalf("the carried tree does not parse: %v", terr)
	}
	// Every element that has a parent in the tree must say so: its `/P` names that parent.
	checked := 0
	for _, e := range tree.elems {
		if e.parent == nil || e.parent.objNr == 0 || e.objNr == 0 {
			continue
		}
		ir, isRef := e.dict["P"].(types.IndirectRef)
		if !isRef {
			continue
		}
		checked++
		if ir.ObjectNumber.Value() != e.parent.objNr {
			t.Errorf("element %d (/%s) says /P names %d, and its tree parent is %d. `/P` is required "+
				"and names the immediate parent; a copy that keeps the original's makes the tree "+
				"disagree with itself in the two directions a reader can walk it",
				e.objNr, e.kind, ir.ObjectNumber.Value(), e.parent.objNr)
		}
	}
	if checked < 2 {
		t.Errorf("only %d element(s) had both a tree parent and a /P, so this asserted almost "+
			"nothing — the fixture has to produce a copied element with a copied parent", checked)
	}
}

// TestAClaimOnAMissingRowIsDELETEDFromTheClaimant — both directions or neither.
//
// `checkStructConsistency` tests that a PAGE's `/StructParents` resolves and never that an
// annotation's `/StructParent` does, so a claimant left naming a row the output does not have is
// invisible to every codified predicate. The renumbering deletes such a claim rather than carrying
// a number into a table that has no such row — which is the same rule the rest of this package keeps
// in the other direction.
func TestAClaimOnAMissingRowIsDELETEDFromTheClaimant(t *testing.T) {
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		// Page 1 declares key 7, which the /ParentTree does not have — a dangling number, the shape
		// `allocParentTreeKey`'s own header describes a tool leaving behind.
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 4 0 R /StructParents 7 >>",
		4: streamObj(markedPage("ZZUNDESCRIBED")),
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 6 0 R /StructParents 1 >>",
		6: streamObj(markedPage("ZZDESCRIBED")),

		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		30: "<< /Type /StructTreeRoot /K [33 0 R] /ParentTree 39 0 R >>",
		33: "<< /Type /StructElem /S /P /P 30 0 R /Pg 5 0 R /K [0] >>",
		39: "<< /Nums [1 [33 0 R]] >>",
	})
	// Keep BOTH, so the carry succeeds on page 2's strength and page 1's dangling claim is what is
	// left to dispose of.
	out, err := Collect(src, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if !inspectTags(out).tree {
		t.Fatal("the carry was refused, so there is no claim to have disposed of")
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	d, _, _, perr := ctx.PageDict(1, false)
	if perr != nil || d == nil {
		t.Fatalf("page 1 does not resolve: %v", perr)
	}
	if v, ok := d["StructParents"]; ok {
		t.Errorf("page 1 still declares /StructParents %v over a /ParentTree that has no such row. "+
			"Nothing in this package checks that an annotation's or a page's claim RESOLVES after a "+
			"carry, so a claim left naming a missing row is a dangling number no predicate can see", v)
	}
}

// TestTheClaimantWalksAgreeAboutAFormXObjectDrawnOnTwoPages — the arm the agreement test lacked.
//
// **Measured: with a fresh visited set per page, sharing a form across two pages made the walk offer
// it twice — the second time with the key the first offer had just written.** The sibling test's
// three documents all derive from a fixture with no form XObject at all, so the shape the two walks
// differ most on was ungraded. The form here carries NO marked content, so completeness condition 4
// (an MCID-bearing form drawn twice) does not fire and the carry reaches the renumbering.
func TestTheClaimantWalksAgreeAboutAFormXObjectDrawnOnTwoPages(t *testing.T) {
	plain := "q 1 0 0 RG 10 10 m 100 100 l S Q\n" // a line: drawn content, no marked content
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /XObject << /Fm1 12 0 R >> /Font << /F1 11 0 R >> >> /Contents 4 0 R >>",
		4: streamObj(markedPage("ZZFIRST") + "q /Fm1 Do Q\n"),
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 1 " +
			"/Resources << /XObject << /Fm1 12 0 R >> /Font << /F1 11 0 R >> >> /Contents 6 0 R >>",
		6: streamObj(markedPage("ZZSECOND") + "q /Fm1 Do Q\n"),

		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		12: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /StructParent 4 "+
			"/Length %d >>\nstream\n%s\nendstream", len(plain), plain),
		30: "<< /Type /StructTreeRoot /K [31 0 R 33 0 R 35 0 R] /ParentTree 39 0 R >>",
		31: "<< /Type /StructElem /S /P /P 30 0 R /Pg 3 0 R /K [0] >>",
		33: "<< /Type /StructElem /S /P /P 30 0 R /Pg 5 0 R /K [0] >>",
		35: "<< /Type /StructElem /S /Figure /P 30 0 R /Pg 3 0 R /Alt (a line) >>",
		39: "<< /Nums [0 [31 0 R] 1 [33 0 R] 4 35 0 R] >>",
	})
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(src), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatalf("setup: %v", rerr)
	}
	readSide := map[int]int{}
	for k, who := range parentTreeOwners(ctx) {
		readSide[k] = len(who)
	}
	writeSide := map[int]int{}
	eachParentTreeClaim(ctx, func(key int, _ func(int)) { writeSide[key]++ })
	if len(readSide) == 0 {
		t.Fatal("setup: neither walk found a claimant")
	}
	if readSide[4] != 1 {
		t.Errorf("the read-side walk reports %d owner(s) of the form's key 4, want 1. One object "+
			"claiming one key is one owner, however many pages draw it — reporting two is "+
			"completeness condition 5's shape and would make this document's carry unable ever to "+
			"pass the gate", readSide[4])
	}
	if writeSide[4] != 1 {
		t.Errorf("the write-side walk offers the form's key %d time(s), want 1. A second offer "+
			"re-reads the key the first offer's setter has already rewritten, so the claimant ends "+
			"up naming a key nothing allocated to it", writeSide[4])
	}
	if fmt.Sprint(readSide) != fmt.Sprint(writeSide) {
		t.Errorf("the read-side walk answers %v and the write-side walk %v", readSide, writeSide)
	}
}
