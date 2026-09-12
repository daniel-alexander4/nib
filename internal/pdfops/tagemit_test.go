package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The wrapping emitter — `PLAN-accessibility.md` P05.S04.

// pageContentOf returns one page's decoded content stream.
func pageContentOf(t *testing.T, pdf []byte, pageNr int) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	d, _, _, derr := ctx.PageDict(pageNr, false)
	if derr != nil || d == nil {
		t.Fatalf("page %d: %v", pageNr, derr)
	}
	c, cerr := ctx.PageContent(d, pageNr)
	if cerr != nil {
		t.Fatalf("content of page %d: %v", pageNr, cerr)
	}
	return c
}

// TestWrappingDoesNotDisturbTheWrappedBytes — the slice's first acceptance clause, and the reason
// S01's round trip had to be byte-identical.
//
// **Asserted as a byte comparison of the span BETWEEN the inserted operators**, not as a length or
// a substring check. The emitter inserts at two offsets and copies everything else; if any byte of
// the document's own drawing changed, the walker re-serialised something and every later claim
// about "nib did not alter your content" is false.
func TestWrappingDoesNotDisturbTheWrappedBytes(t *testing.T) {
	src, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatal(err)
	}
	before := pageContentOf(t, src, 1)
	if len(before) < 500 {
		t.Fatalf("the page carries %d bytes of content — too little for this to mean anything", len(before))
	}

	out, wrapped, err := tagAuthoredContent(src)
	if err != nil {
		t.Fatalf("tagAuthoredContent: %v", err)
	}
	if wrapped != 1 {
		t.Fatalf("wrapped %d page(s), want 1", wrapped)
	}
	after := pageContentOf(t, out, 1)

	// The opener ends at the first `BDC`; the closer begins at the last `EMC`.
	toks := contentstream.Tokenize(after)
	var firstBDC, lastEMC *contentstream.Token
	for i := range toks {
		if toks[i].Kind != contentstream.Operator {
			continue
		}
		switch string(toks[i].Bytes(after)) {
		case "BDC":
			if firstBDC == nil {
				firstBDC = &toks[i]
			}
		case "EMC":
			lastEMC = &toks[i]
		}
	}
	if firstBDC == nil || lastEMC == nil {
		t.Fatalf("the wrapped page has no BDC/EMC pair (BDC=%v EMC=%v)", firstBDC, lastEMC)
	}
	inner := after[firstBDC.End:lastEMC.Start]
	// One leading and one trailing separator are the emitter's own.
	if !bytes.Equal(bytes.TrimSpace(inner), bytes.TrimSpace(before)) {
		t.Errorf("the wrapped content is not the original bytes\n  %d bytes in, %d between the "+
			"operators\n  first difference at %d", len(before), len(inner), firstDiff(before, inner))
	}
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// TestTheMCIDsTheEmitterWritesAreTheOnesTheParentTreePointsAt — the second acceptance clause, read
// back from the document rather than remembered from the call.
//
// The two directions are stored in different places: the MCID goes into the page's content stream
// inside a `BDC` property list, and the element goes into the ParentTree array at that index.
// Nothing but a check makes them the same number.
func TestTheMCIDsTheEmitterWritesAreTheOnesTheParentTreePointsAt(t *testing.T) {
	out, _, err := tagAuthoredContent(authoredPDF(t))
	if err != nil {
		t.Fatal(err)
	}
	content := pageContentOf(t, out, 1)

	// The MCID written into the content stream.
	mcidInStream := -1
	toks := contentstream.Tokenize(content)
	for i, tk := range toks {
		if tk.Kind != contentstream.Operator || string(tk.Bytes(content)) != "BDC" {
			continue
		}
		// Walk back to the `/MCID <n>` inside the property dictionary. **The value is the next
		// non-whitespace token**, not the next token: `Tokenize` emits whitespace as tokens of its
		// own (that totality is what makes the round trip byte-identical), so `toks[j+1]` is the
		// space after the name.
		for j := i - 1; j >= 0 && j > i-12; j-- {
			if toks[j].Kind != contentstream.Operand || string(toks[j].Bytes(content)) != "/MCID" {
				continue
			}
			for k := j + 1; k < len(toks); k++ {
				if toks[k].Kind == contentstream.Whitespace {
					continue
				}
				var n int
				if _, serr := fmtSscan(string(toks[k].Bytes(content)), &n); serr == nil {
					mcidInStream = n
				}
				break
			}
		}
	}
	if mcidInStream < 0 {
		t.Fatal("no /MCID found in the wrapped page's BDC property list")
	}

	tree, defects := checkTree(t, out)
	if len(defects) > 0 {
		t.Fatalf("the emitted document is not self-consistent: %v", defects)
	}
	// The element the model holds must claim exactly that MCID.
	var claimed []int
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if k.kind == kidMCID {
				claimed = append(claimed, k.mcid)
			}
		}
	}
	if len(claimed) != 1 || claimed[0] != mcidInStream {
		t.Errorf("the content stream says /MCID %d; the tree claims %v", mcidInStream, claimed)
	}
}

// fmtSscan is a tiny wrapper so the test above reads without an fmt import shadowing the package's.
func fmtSscan(s string, n *int) (int, error) {
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNoStructTree
		}
		v = v*10 + int(c-'0')
	}
	*n = v
	return 1, nil
}

// TestAPageThatIsAlreadyMarkedIsNotWrappedAgain — the third acceptance clause.
//
// Wrapping twice produces nested marked content whose inner MCIDs belong to a producer's tree and
// whose outer one belongs to nib's, and no reader can be expected to describe a page twice.
func TestAPageThatIsAlreadyMarkedIsNotWrappedAgain(t *testing.T) {
	// The corpus fixture already carries `/P <</MCID 0>> BDC … EMC`.
	before := pageContentOf(t, taggedFixture(), 1)
	out, wrapped, err := tagAuthoredContent(taggedFixture())
	if err != nil {
		t.Fatalf("tagAuthoredContent: %v", err)
	}
	if wrapped != 0 {
		t.Errorf("wrapped %d page(s) of a document that is already marked", wrapped)
	}
	after := pageContentOf(t, out, 1)
	if !bytes.Equal(before, after) {
		t.Errorf("an already-marked page's content changed\n  before %q\n  after  %q", before, after)
	}
	// And running it twice over nib's own output is a no-op too.
	once, n1, err := tagAuthoredContent(authoredPDF(t))
	if err != nil || n1 != 1 {
		t.Fatalf("first pass: wrapped=%d err=%v", n1, err)
	}
	_, n2, err := tagAuthoredContent(once)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("a second pass wrapped %d page(s) — the emitter is not idempotent and a document "+
			"tagged twice describes its content twice", n2)
	}
}

// TestAlreadyMarkedIsDecidedByTOKENS, not by a byte search.
//
// Since P04 every glyph nib draws is a two-byte index into an embedded font, so arbitrary byte
// pairs inside literal strings are the normal case. A page whose text happens to contain the bytes
// `BDC` would be reported as already tagged and silently skipped — the document would come out
// untagged, and nothing would say why.
func TestAlreadyMarkedIsDecidedByTOKENS(t *testing.T) {
	// `BDC` and `EMC` appear only inside a string.
	withStrings := []byte("BT /F1 12 Tf (BDC) Tj (EMC) Tj ET")
	if alreadyMarked(withStrings) {
		t.Error("a page whose TEXT contains \"BDC\" was reported as already marked — the check is " +
			"reading the document's content rather than its operators, and every page that draws " +
			"those letters would be skipped")
	}
	// A real operator is found.
	withOperator := []byte("/P <</MCID 0>> BDC BT (x) Tj ET EMC")
	if !alreadyMarked(withOperator) {
		t.Error("real marked content was not detected")
	}
	// BMC counts too — marked content without a property list is still marked content.
	if !alreadyMarked([]byte("/Artifact BMC BT (x) Tj ET EMC")) {
		t.Error("BMC was not detected as marked content")
	}
}

// TestTheEmittedElementNamesItsParent — the defect measurement found, and it is the one that looks
// like something else.
//
// An element with no `/P` still parses, still validates, and the page's content items still report
// as `{mcid:0}` — marked. veraPDF nonetheless fails ua1 **7.1 t3**, *content shall be marked as
// Artifact or tagged as real content*, because an element outside the tree is not something an MCID
// can resolve into. **The failure names the CONTENT, so it reads as a wrapping problem and is not
// one.** Every element of a real LibreOffice tree carries `/P` — 36 of 36, measured at S02.
func TestTheEmittedElementNamesItsParent(t *testing.T) {
	out, _, err := tagAuthoredContent(authoredPDF(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	cat, _ := ctx.XRefTable.Catalog()
	rootRef, ok := cat["StructTreeRoot"].(types.IndirectRef)
	if !ok {
		t.Fatal("no /StructTreeRoot indirect reference")
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatal(terr)
	}
	if len(tree.elems) == 0 {
		t.Fatal("no elements")
	}
	for _, e := range tree.elems {
		p, has := e.dict["P"]
		if !has {
			t.Errorf("an element of type /%s has no /P — it is outside the tree, and every content "+
				"item its MCID covers will fail ua1 7.1 t3 while reporting as marked", e.kind)
			continue
		}
		ind, isInd := p.(types.IndirectRef)
		if !isInd || ind.ObjectNumber.Value() != rootRef.ObjectNumber.Value() {
			t.Errorf("an element of type /%s names /P %v, which is not the /StructTreeRoot (%v)",
				e.kind, p, rootRef)
		}
	}
}

// TestTheEmitterClaimsGroupingAndNotRole.
//
// The emitter knows where a page's content is and nothing about what it says. `/P` would claim
// every page is one paragraph, which is false of any page with a heading; `/Div` is ISO 32000-1's
// generic block-level grouping element and says only *this is real content, grouped*.
//
// Asserted because it is a decision, not an implementation detail: P06 replaces the TYPE with real
// structure from `mdpdf`'s AST, and a silent change of this constant to `/P` in the meantime would
// be a claim nobody made deliberately.
func TestTheEmitterClaimsGroupingAndNotRole(t *testing.T) {
	if authoredStructType != "Div" {
		t.Errorf("the emitter tags content as /%s. It knows nothing about what the content SAYS, "+
			"so the only honest type is a generic grouping element", authoredStructType)
	}
	out, _, err := tagAuthoredContent(authoredPDF(t))
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := checkTree(t, out)
	for _, e := range tree.elems {
		if e.kind == "P" || strings.HasPrefix(e.kind, "H") {
			t.Errorf("the emitter produced an element of type /%s, which claims a role it cannot "+
				"know", e.kind)
		}
	}
}
