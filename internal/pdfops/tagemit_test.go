package pdfops

import (
	"bytes"
	"nib/mdpdf"
	"regexp"
	"testing"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tree-writing invariants — `PLAN-accessibility.md` P05.S04's acceptance, driven through the commit
// writer since P08.S07 deleted the generic emitter they were first written against. The invariants are
// the writer's, not the emitter's: every door that brackets content and builds a tree owes them.

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

// committedP4 is an untagged Markdown render with its proposal committed.
func committedP4(t *testing.T) (src, out []byte) {
	t.Helper()
	src, err := untaggedMarkdown([]byte(p4Markdown))
	if err != nil {
		t.Fatal(err)
	}
	out, err = commitProposal(src, proposeFor(t, src).elements)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return src, out
}

// insertedMarkedContent is exactly what the commit writer inserts: an element's opener, an artifact's
// opener, and a closer.
var insertedMarkedContent = regexp.MustCompile(`/[A-Za-z0-9]+ <</MCID \d+>> BDC\n|/Artifact BMC\n|\nEMC`)

// TestBracketingDoesNotDisturbTheBracketedBytes — S01's round trip is what makes this possible: the
// writer inserts at offsets and copies everything else, so removing exactly what it inserted must
// give back the page's own bytes, every one of them.
func TestBracketingDoesNotDisturbTheBracketedBytes(t *testing.T) {
	src, out := committedP4(t)
	before := pageContentOf(t, src, 1)
	if len(before) < 500 {
		t.Fatalf("the page carries %d bytes of content — too little for this to mean anything", len(before))
	}
	after := pageContentOf(t, out, 1)
	if n := len(insertedMarkedContent.FindAllIndex(after, -1)); n == 0 {
		t.Fatal("setup: the committed page carries no inserted marked content, so nothing below is measured")
	}
	recovered := insertedMarkedContent.ReplaceAll(after, nil)
	if !bytes.Equal(recovered, before) {
		t.Errorf("the committed page, less what the commit inserted, is not the original bytes\n  %d bytes in, %d recovered\n  first difference at %d",
			len(before), len(recovered), firstDiff(before, recovered))
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

// TestTheMCIDsInTheStreamAreTheOnesTheTreeClaims — read back from the document rather than remembered
// from the call. The MCID goes into a `BDC` property list and the element goes into the ParentTree;
// nothing but a check makes them the same numbers.
func TestTheMCIDsInTheStreamAreTheOnesTheTreeClaims(t *testing.T) {
	_, out := committedP4(t)
	content := pageContentOf(t, out, 1)

	inStream := map[int]bool{}
	toks := contentstream.Tokenize(content)
	for i, tk := range toks {
		if tk.Kind != contentstream.Operator || string(tk.Bytes(content)) != "BDC" {
			continue
		}
		// Walk back to the `/MCID <n>` in the property dictionary. **The value is the next
		// non-whitespace token**: `Tokenize` emits whitespace as tokens of its own.
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
					inStream[n] = true
				}
				break
			}
		}
	}
	if len(inStream) == 0 {
		t.Fatal("no /MCID found in the committed page's BDC property lists")
	}
	tree, defects := checkTree(t, out)
	if len(defects) > 0 {
		t.Fatalf("the committed document is not self-consistent: %v", defects)
	}
	claimed := map[int]bool{}
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if k.kind == kidMCID {
				claimed[k.mcid] = true
			}
		}
	}
	if len(claimed) != len(inStream) {
		t.Errorf("the content stream carries %d MCID(s); the tree claims %d", len(inStream), len(claimed))
	}
	for m := range inStream {
		if !claimed[m] {
			t.Errorf("the content stream says /MCID %d and no element claims it", m)
		}
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

// TestAlreadyMarkedIsDecidedByTOKENS, not by a byte search.
//
// Since P04 every glyph nib draws is a two-byte index into an embedded font, so arbitrary byte pairs
// inside literal strings are the normal case. A page whose text happens to contain the bytes `BDC`
// would be reported as already marked and silently skipped.
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

// TestEveryCommittedElementNamesTheParentThatHoldsIt — the defect measurement found at P05.S04, and the
// one that looks like something else.
//
// An element with no `/P` still parses, still validates, and its content still reports as `{mcid:0}` —
// marked. veraPDF nonetheless fails ua1 **7.1 t3**, because an element outside the tree is not something
// an MCID can resolve into. The commit writer nests (`L` › `LI` › `LBody`), so the parent is not always
// the root: each element must name exactly the element, or the root, whose `/K` holds it.
func TestEveryCommittedElementNamesTheParentThatHoldsIt(t *testing.T) {
	_, out := committedP4(t)
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
			t.Errorf("an element of type /%s has no /P — it is outside the tree, and every content item its "+
				"MCID covers will fail ua1 7.1 t3 while reporting as marked", e.kind)
			continue
		}
		ind, isInd := p.(types.IndirectRef)
		if !isInd {
			t.Errorf("an element of type /%s names /P %v, which is not a reference", e.kind, p)
			continue
		}
		want := rootRef.ObjectNumber.Value()
		if e.parent != nil {
			want = e.parent.objNr
		}
		if ind.ObjectNumber.Value() != want {
			t.Errorf("an element of type /%s names /P %d, and the object whose /K holds it is %d", e.kind, ind.ObjectNumber.Value(), want)
		}
	}
}

// TestACommitWritesBothHalvesOrNeither — ADR-031 law 1 at the one door that writes inferred structure:
// `/MarkInfo` with no tree is `orphaned()`, and a tree with no `/MarkInfo` is a document that carries
// structure and does not say so.
func TestACommitWritesBothHalvesOrNeither(t *testing.T) {
	_, out := committedP4(t)
	s := inspectTags(out)
	if !s.marked {
		t.Error("the committed document has no /MarkInfo /Marked true — it carries structure and does not " +
			"say it is tagged, so a reader has no reason to look")
	}
	if !s.tree || s.elements == 0 {
		t.Errorf("the committed document has no usable tree (tree=%v elements=%d)", s.tree, s.elements)
	}
	if s.orphaned() {
		t.Errorf("the committed document claims tagging its content does not support: %+v", s)
	}
	if _, defects := checkTree(t, out); len(defects) > 0 {
		t.Errorf("the committed document is not self-consistent: %v", defects)
	}
}

// TestSplittingTheTwoHalvesIsOrphaned is the stimulus floor for the test above: it proves `orphaned()`
// can actually SEE the failure the door exists to prevent, on this shape of document.
//
// Without it, `!s.orphaned()` above is satisfied by a predicate that never returns true.
func TestSplittingTheTwoHalvesIsOrphaned(t *testing.T) {
	// /MarkInfo with no tree — the half P03 struck from its own floor to avoid producing.
	markedOnly, err := writeMutated(authoredPDF(t), func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		cat["MarkInfo"] = types.Dict{"Marked": types.Boolean(true)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(markedOnly); !s.orphaned() {
		t.Errorf("a document with /MarkInfo /Marked true and NO tree is not reported as orphaned "+
			"(%+v) — the post-condition in claimTagging cannot see the state it exists to refuse", s)
	}
}

// untaggedMarkdown renders Markdown the way `ConvertDocToPDF` did before `/pending 481` wired tagging
// in: real `mdpdf` output with nib's faces, and no structure.
//
// The commit writer refuses a page that already carries marked content, and `ConvertDocToPDF` tags now —
// so the fixture these tests need is the untagged render, named here rather than borrowed from a door
// whose output changed.
func untaggedMarkdown(md []byte) ([]byte, error) {
	faces := authoringFaces()
	out, err := mdpdf.ConvertWithFaces(md, faces, markdownFallbackFonts())
	if err != nil {
		return nil, err
	}
	return embeddedFontsAreHonest(out), nil
}
