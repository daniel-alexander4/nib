package pdfops

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// An OCR'd scan is tagged from tesseract's own hierarchy — `PLAN-accessibility.md` P06.S06.

// scannedPage is what an OCR'd document actually is: one page that is a picture. A text fixture
// would let the base content answer clauses the scan's image is what really owes.
func scannedPage(t *testing.T) []byte {
	t.Helper()
	bg := image.NewRGBA(image.Rect(0, 0, 612, 792))
	for i := range bg.Pix {
		bg.Pix[i] = 0xf0
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, bg); err != nil {
		t.Fatal(err)
	}
	out, err := ImagesToPDF([]RasterPage{{Image: buf.Bytes(), W: 612, H: 792}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ocrWords is a two-paragraph page in one block, the shape a letterhead produces.
func ocrWords() []Word {
	return []Word{
		{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice", Block: 1, Para: 2, Line: 3},
		{Page: 1, Rect: [4]float64{145, 700, 190, 712}, Text: "2024", Block: 1, Para: 2, Line: 3},
		{Page: 1, Rect: [4]float64{72, 680, 200, 692}, Text: "Acme", Block: 1, Para: 4, Line: 5},
		{Page: 1, Rect: [4]float64{205, 680, 240, 692}, Text: "Ltd", Block: 1, Para: 4, Line: 5},
	}
}

func pageStream(t *testing.T, pdf []byte, page int) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	d, _, _, derr := ctx.PageDict(page, false)
	if derr != nil || d == nil {
		t.Fatalf("page %d: %v", page, derr)
	}
	c, cerr := ctx.PageContent(d, page)
	if cerr != nil {
		t.Fatalf("content: %v", cerr)
	}
	return c
}

// TestNoWordOfTheTextLayerIsMarkedAsSomethingToSKIP — S06's second acceptance clause, and the defect
// the slice exists for.
//
// An artifact is content a conforming reader is told to skip. `StampTextLayer` wraps every OCR'd
// word `/Artifact <</Subtype /Watermark /Type /Pagination>> BDC`, so the searchable layer — the one
// thing that makes a scanned page readable at all — was explicitly declared not to be read. This
// asserts on the page stream, not on a clause, because the clause it satisfied was satisfied BY
// disclaiming the content.
func TestNoWordOfTheTextLayerIsMarkedAsSomethingToSKIP(t *testing.T) {
	base := scannedPage(t)
	words := ocrWords()

	// The stimulus, asserted first: the untagged stamp really does artifact every word, or this
	// test is measuring a defect that is not in the fixture.
	stamped, err := StampTextLayer(base, words, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(watermarkArtifactSpans(pageStream(t, stamped, 1))); n != len(words) {
		t.Fatalf("setup: the plain stamp marks %d of %d words as watermark artifacts — the defect "+
			"this slice is about is not present in the fixture", n, len(words))
	}

	out, tagged, err := TagOCRLayer(base, words, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("TagOCRLayer fell back to the plain stamp, so nothing below is about a tagged " +
			"document. That fallback is deliberate and must not be the ordinary path")
	}
	c := pageStream(t, out, 1)
	if n := len(watermarkArtifactSpans(c)); n != 0 {
		t.Errorf("%d word(s) of the text layer are still marked /Artifact, so a screen reader is "+
			"told to skip them:\n%.300s", n, c)
	}
	if !bytes.Contains(c, []byte("/MCID")) {
		t.Errorf("the page carries no marked-content id, so nothing in the tree points at any "+
			"word:\n%.300s", c)
	}
}

// TestAParagraphIsONEElementNotOneElementPerWord — the hierarchy is used, not merely received.
//
// Four words in two paragraphs of one block. The tree that describes them is one `Sect` holding two
// `P`s, each `P` owning its own words' MCIDs. The failure this guards is a page of single-word
// paragraphs, which reads aloud as a list of disconnected words and looks entirely correct in any
// structural check that only counts.
func TestAParagraphIsONEElementNotOneElementPerWord(t *testing.T) {
	out, tagged, err := TagOCRLayer(scannedPage(t), ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("TagOCRLayer fell back to the plain stamp")
	}
	s := inspectTags(out)
	// 1 Sect + 2 P. Four words share two elements, and that is the whole assertion.
	if s.elements != 3 {
		t.Errorf("the tree holds %d element(s) for 4 words in 2 paragraphs of 1 block; want 3 "+
			"(one Sect, two P). One element per WORD is a page that reads aloud as disconnected "+
			"words: %+v", s.elements, s)
	}
	if s.anchored != 2 {
		t.Errorf("%d element(s) point at content, want 2 — the two paragraphs. A Sect anchors "+
			"nothing itself, which is what makes it a grouping element: %+v", s.anchored, s)
	}
	if s.undescribed != 0 {
		t.Errorf("%d element(s) describe nothing: %+v", s.undescribed, s)
	}
}

// TestWordsWithNoHierarchyAreNotAllOneSentence — the wire's compatibility case.
//
// An older client sends no indices, so every word arrives with block/para/line zero. Reading zero as
// a group id would put every word on the page into one paragraph of one section — a whole scan
// announced as a single sentence. Each unattributed word gets its own element instead, which is the
// honest reading: nothing is known about what it belongs with.
func TestWordsWithNoHierarchyAreNotAllOneSentence(t *testing.T) {
	words := []Word{
		{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice"},
		{Page: 1, Rect: [4]float64{145, 700, 190, 712}, Text: "2024"},
		{Page: 1, Rect: [4]float64{72, 680, 200, 692}, Text: "Acme"},
	}
	out, tagged, err := TagOCRLayer(scannedPage(t), words, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("TagOCRLayer fell back to the plain stamp")
	}
	s := inspectTags(out)
	if s.elements != 3 || s.anchored != 3 {
		t.Errorf("3 words with no hierarchy produced %d element(s), %d anchored; want 3 and 3. "+
			"One element means zero was read as a group id and the whole page is one sentence; "+
			"a Sect appearing means a grouping element was invented over content nothing grouped: "+
			"%+v", s.elements, s.anchored, s)
	}
}

// TestTaggingAnOCRdScanCostsItNoUA1Clause — S06's third acceptance clause, on the ua1 oracle.
//
// The tagged scan must fail a strict SUBSET of what the untagged one failed. What remains is named
// per clause rather than summarised, because "except what its images owe" is a sentence and a
// clause list is a measurement.
func TestTaggingAnOCRdScanCostsItNoUA1Clause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P06.S06's third acceptance clause is " +
			"UNCHECKED in this run.")
	}
	base := scannedPage(t)
	out, tagged, err := TagOCRLayer(base, ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("TagOCRLayer fell back to the plain stamp, so this differential is not about a " +
			"tagged document")
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"scan.pdf": base, "tagged.pdf": out} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["scan.pdf"], cl["tagged.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	// Stimulus floor: a scan that fails nothing makes "a subset" vacuous.
	if len(before) == 0 {
		t.Fatal("setup: the untagged scan fails NO ua1 clause, so every comparison below is empty")
	}
	var added []string
	for c := range after {
		if !before[c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		t.Errorf("describing the text layer ADDED ua1 clause(s) the scan did not fail: %s.\n\t"+
			"A document must not be worse for having been described. 7.2 t34 is the one that "+
			"appears if the recognised language stops reaching the catalog — there was no tagged "+
			"content to ask about before the tree existed",
			strings.Join(added, ", "))
	}
	// And it must actually BUY something, or a no-op would pass the clause above.
	var cleared []string
	for c := range before {
		if !after[c] {
			cleared = append(cleared, c)
		}
	}
	sort.Strings(cleared)
	if len(cleared) == 0 {
		t.Errorf("tagging cleared NO clause. The scan fails %v either way, so nothing was "+
			"described", sortedClauses(before))
	}
	t.Logf("untagged scan fails %v\ntagged scan fails %v\ncleared: %v",
		sortedClauses(before), sortedClauses(after), cleared)
}

// TestTheTreeSaysItCameFromOCRAndNotFromAnAST — S06's fourth acceptance clause, and D4's tiers.
//
// An OCR engine's opinion about a picture is not a document's own structure: two columns can be read
// as one block, a table as paragraphs. `sourceExact` is for a tree read from an authoring format's
// AST, which is what `tagMarkdown` does. Recording the difference is what lets a later reader decide
// how far to trust the tree — the question S03 built the key to answer.
func TestTheTreeSaysItCameFromOCRAndNotFromAnAST(t *testing.T) {
	out, tagged, err := TagOCRLayer(scannedPage(t), ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("TagOCRLayer fell back to the plain stamp")
	}
	src, ok := StructureSource(out)
	if !ok {
		t.Fatal("the tagged scan records no structure source at all, so every tree nib writes " +
			"looks equally trustworthy")
	}
	if src != sourceApproximate {
		t.Errorf("the tree records source %q, want %q — an OCR engine's reading of a picture is "+
			"not a document's own structure", src, sourceApproximate)
	}
}

// TestTheTextLayerSURVIVESAStructureItCannotBeGiven — the fallback, asserted rather than assumed.
//
// A word count that disagrees with the marker count means the correspondence cannot be established
// by position, and describing content by a position you cannot trust attaches every element from
// the divergence onward to the wrong word. `StampTextLayer` SKIPS a word it cannot represent rather
// than failing the layer, so this is a real state and not a hypothetical.
//
// What must not happen is losing the text. A scan whose text is searchable but artifacted is what
// nib shipped for years; a scan with no text layer at all is worse than both.
func TestTheTextLayerSURVIVESAStructureItCannotBeGiven(t *testing.T) {
	base := scannedPage(t)
	words := ocrWords()
	// A word the stamp cannot represent: pdfops/ocr.go skips a blank one, so the marker count comes
	// back one short of the word count and the correspondence check refuses.
	words = append(words, Word{Page: 1, Rect: [4]float64{300, 600, 320, 612}, Text: "   ", Block: 1, Para: 2, Line: 3})

	out, tagged, err := TagOCRLayer(base, words, "eng")
	if err != nil {
		t.Fatalf("TagOCRLayer failed outright: %v — a structure it cannot build must cost the "+
			"document its tree, never its text", err)
	}
	if tagged {
		t.Fatal("setup: the structure WAS built, so this test is not exercising the fallback — " +
			"the unrepresentable word no longer breaks the correspondence")
	}
	// The text layer is there, and it is the ordinary artifacted stamp.
	c := pageStream(t, out, 1)
	if n := len(watermarkArtifactSpans(c)); n == 0 {
		t.Errorf("the fallback returned a page with no stamped words at all — the user has lost " +
			"the searchable text to a failure in describing it")
	}
	if s := inspectTags(out); s.claims() {
		t.Errorf("the fallback document CLAIMS tagging (%+v). ADR-031 law 1: nothing claims "+
			"tagging it has not", s)
	}
}
