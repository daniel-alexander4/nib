package pdfops

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"reflect"
	"sort"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/draw"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Replacing a text layer Nib added — ADR-101. The four ways it can go wrong, each with a test here that fails when
// it does: content that is not Nib's layer taken out; structure left naming content that has gone; an old layer
// still under the new one; and (at the route, internal/server/ocr_test.go) a signed document touched.

// scannedPages is n pages that are each a picture — what an OCR'd document is.
func scannedPages(t *testing.T, n int) []byte {
	t.Helper()
	bg := image.NewRGBA(image.Rect(0, 0, 612, 792))
	for i := range bg.Pix {
		bg.Pix[i] = 0xf0
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, bg); err != nil {
		t.Fatal(err)
	}
	pages := make([]RasterPage, n)
	for i := range pages {
		pages[i] = RasterPage{Image: buf.Bytes(), W: 612, H: 792}
	}
	out, err := ImagesToPDF(pages)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// hiddenOn is the invisible runs the page map reads on one page, as the map gives them.
func hiddenOn(t *testing.T, pdf []byte, page int) []MapText {
	t.Helper()
	pm, err := MapPage(pdf, page)
	if err != nil {
		t.Fatalf("MapPage %d: %v", page, err)
	}
	var out []MapText
	for _, tx := range pm.Text {
		if tx.Hidden {
			out = append(out, tx)
		}
	}
	return out
}

func hiddenWords(runs []MapText) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Text
	}
	sort.Strings(out)
	return out
}

// noDanglingStructure fails if the tree and the pages disagree in either direction.
func noDanglingStructure(t *testing.T, pdf []byte, when string) {
	t.Helper()
	if _, defects := checkTree(t, pdf); len(defects) > 0 {
		t.Errorf("%s: the structure tree disagrees with itself: %v", when, defects)
	}
	if defects := completeness(t, pdf); len(defects) > 0 {
		t.Errorf("%s: the structure names content the pages do not draw, or the reverse: %v", when, defects)
	}
	// And the parent tree names no element that has left the tree: neither check above reads a slot whose
	// marked content the page no longer draws, and an element reachable only from there is still written.
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := readStructTree(ctx, livePageObjects(ctx))
	if err != nil {
		t.Fatal(err)
	}
	arrays, _ := parentTreeEntries(ctx, tree)
	for key, slots := range arrays {
		for mcid, obj := range slots {
			if obj != 0 && tree.byObj[obj] == nil {
				t.Errorf("%s: the parent tree's key %d still names object %d for /MCID %d, and no such element is in the tree", when, key, obj, mcid)
			}
		}
	}
}

// tampered is pdf with page 1's content, or the form it draws as /Fm0, rewritten by change.
func tampered(t *testing.T, pdf []byte, form bool, change func([]byte) []byte) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		pg := pdfread.Pages(ctx)[0]
		if !form {
			src, err := pdfread.PageContent(ctx, pg.Dict, 1)
			if err != nil {
				return err
			}
			return setPageContent(ctx, pg.Dict, change(src))
		}
		xobjs, err := ctx.DereferenceDict(pg.Attrs.Resources["XObject"])
		if err != nil {
			return err
		}
		ref := xobjs["Fm0"].(types.IndirectRef)
		sd, _, err := ctx.DereferenceStreamDict(ref)
		if err != nil {
			return err
		}
		if err := sd.Decode(); err != nil {
			return err
		}
		sd.Content = change(sd.Content)
		if err := sd.Encode(); err != nil {
			return err
		}
		entry, _ := ctx.FindTableEntryForIndRef(&ref)
		entry.Object = *sd
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// shortStamp stamps words as Nib did before ADR-092: at the box's height, to the whole point, unfitted. It is the
// layer a file OCR'd before v1.189.2 carries.
func shortStamp(t *testing.T, pdf []byte, words []Word) []byte {
	t.Helper()
	model.NewDefaultConfiguration()
	wms := map[int][]*model.Watermark{}
	for _, w := range words {
		desc := fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:bl, offset:%.2f %.2f, rotation:0",
			ocrFont, int(w.Rect[3]-w.Rect[1]), w.Rect[0], w.Rect[1])
		wm, err := api.TextWatermark(w.Text, desc, true, false, types.POINTS)
		if err != nil {
			t.Fatal(err)
		}
		wm.RenderMode = draw.RenderMode(3)
		wms[w.Page] = append(wms[w.Page], wm)
	}
	out, err := addWatermarks(pdf, wms)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestReplacingATaggedLayerLeavesOneRunPerWordAndNoStructurePointingNowhere — a described layer is replaced: the
// old words are gone from the page AND from the tree, the new ones are there once each, fitted, and described.
func TestReplacingATaggedLayerLeavesOneRunPerWordAndNoStructurePointingNowhere(t *testing.T) {
	first, tagged, err := TagOCRLayer(scannedPages(t, 1), ocrWords(), "eng")
	if err != nil || !tagged {
		t.Fatalf("the first layer: tagged %v, %v", tagged, err)
	}
	if kinds, kerr := TextLayers(first); kerr != nil || kinds[1] != LayerOwn {
		t.Fatalf("Nib's own tagged layer reads as %q (%v), want %q", kinds[1], kerr, LayerOwn)
	}
	again := []Word{
		{Page: 1, Rect: [4]float64{72, 700, 150, 712}, Text: "Rechnung", Block: 1, Para: 2, Line: 3},
		{Page: 1, Rect: [4]float64{155, 700, 200, 712}, Text: "2025", Block: 1, Para: 2, Line: 3},
		{Page: 1, Rect: [4]float64{72, 680, 130, 692}, Text: "GmbH", Block: 1, Para: 4, Line: 5},
	}
	out, tagged, replaced, left, err := ReplaceOCRLayer(first, again, "deu")
	if err != nil || out == nil {
		t.Fatalf("replace: %v (out nil: %v)", err, out == nil)
	}
	if !tagged || !reflect.DeepEqual(replaced, []int{1}) || len(left) != 0 {
		t.Errorf("replace said tagged %v, replaced %v, left %v — want tagged, [1], none", tagged, replaced, left)
	}
	got := hiddenOn(t, out, 1)
	if want := []string{"2025", "GmbH", "Rechnung"}; !reflect.DeepEqual(hiddenWords(got), want) {
		t.Errorf("after the replace the page's invisible words are %v, want exactly %v — an old word left behind is found twice", hiddenWords(got), want)
	}
	for _, r := range got {
		if r.Short || len(r.Ink) != 2 {
			t.Errorf("the new word %q is not fitted to its box (short %v, ink %v)", r.Text, r.Short, r.Ink)
		}
	}
	noDanglingStructure(t, out, "after the replace")
	// The old elements are gone, not emptied and left: two paragraphs before, two after.
	tree, _ := checkTree(t, out)
	paras := 0
	for _, e := range tree.elems {
		if e.kind == "P" {
			paras++
			if len(e.kids) == 0 {
				t.Errorf("element %d is a paragraph that owns nothing", e.objNr)
			}
		}
	}
	if paras != 2 {
		t.Errorf("the tree holds %d paragraphs, want the new layer's 2", paras)
	}
	if strings.Contains(string(pageStream(t, out, 1)), "Watermark") {
		t.Errorf("a word of the replaced layer is still marked as an artifact to skip")
	}
}

// TestTakingALayerOutLeavesTheDocumentAsItWasBeforeIt — the layer and everything it brought, and nothing else: the
// page's own print, a visible watermark stamped before the OCR, and the document's claim about tagging.
func TestTakingALayerOutLeavesTheDocumentAsItWasBeforeIt(t *testing.T) {
	text, err := testpdf.Text("the page's own print")
	if err != nil {
		t.Fatal(err)
	}
	marked, err := StampWatermark(text, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		base   []byte
		tagged bool
	}{
		"print under a watermark, layer artifacted": {marked, false},
		"a scan, layer described":                   {scannedPages(t, 1), true},
	} {
		t.Run(name, func(t *testing.T) {
			layered, tagged, err := TagOCRLayer(tc.base, ocrWords(), "eng")
			if err != nil || tagged != tc.tagged {
				t.Fatalf("the layer: tagged %v (want %v), %v", tagged, tc.tagged, err)
			}
			before, _ := MapPage(tc.base, 1)
			if n := len(hiddenOn(t, layered, 1)); n != len(ocrWords()) {
				t.Fatalf("the fixture's layer holds %d invisible words, want %d", n, len(ocrWords()))
			}
			out, removed, left, err := removeOwnTextLayers(layered, map[int]bool{1: true})
			if err != nil || !reflect.DeepEqual(removed, []int{1}) || len(left) != 0 {
				t.Fatalf("removed %v, left %v, %v — want [1], none", removed, left, err)
			}
			// The content itself: every stamp wrapped the page in a `q … Q` of its own, and those go with the words.
			// Where nothing described the layer the page's operators are the ones it had, in order.
			was, now := strings.Fields(string(pageStream(t, tc.base, 1))), strings.Fields(string(pageStream(t, out, 1)))
			saves := func(f []string) (n int) {
				for _, s := range f {
					if s == "q" {
						n++
					}
				}
				return n
			}
			if saves(was) != saves(now) {
				t.Errorf("the page saved its graphics state %d time(s) before the layer and %d after it was taken out", saves(was), saves(now))
			}
			if !tc.tagged && !reflect.DeepEqual(was, now) {
				t.Errorf("the page's content is not what it was before the layer:\n before %q\n after  %q", was, now)
			}
			after, _ := MapPage(out, 1)
			if !reflect.DeepEqual(before.Text, after.Text) || !reflect.DeepEqual(before.Shapes, after.Shapes) {
				t.Errorf("the page reads differently once its layer is out:\n before %+v\n after  %+v", before.Text, after.Text)
			}
			if a, b := len(watermarkArtifactSpans(pageStream(t, tc.base, 1))), len(watermarkArtifactSpans(pageStream(t, out, 1))); a != b {
				t.Errorf("the page drew %d watermark(s) before the layer and %d after it was taken out", a, b)
			}
			ctx, rerr := pdfread.Validated(out, model.NewDefaultConfiguration())
			if rerr != nil {
				t.Fatal(rerr)
			}
			base, _ := pdfread.Validated(tc.base, model.NewDefaultConfiguration())
			names := func(c *model.Context, key string) []string {
				pg := pdfread.Pages(c)[0]
				d, _ := c.DereferenceDict(pg.Attrs.Resources[key])
				var out []string
				for k := range d {
					out = append(out, k)
				}
				sort.Strings(out)
				return out
			}
			for _, key := range []string{"XObject", "ExtGState"} {
				if a, b := names(base, key), names(ctx, key); !reflect.DeepEqual(a, b) {
					t.Errorf("the page's /%s names were %v before the layer and are %v after it was taken out", key, a, b)
				}
			}
			cat, _ := ctx.XRefTable.Catalog()
			if _, has := cat["StructTreeRoot"]; has {
				t.Errorf("a structure tree is left over a document whose only described content was the layer")
			}
			if _, has := cat["MarkInfo"]; has {
				t.Errorf("the document still claims tagging with the layer and its tree gone")
			}
			if err := Validate(out); err != nil {
				t.Errorf("the document no longer validates: %v", err)
			}
		})
	}
}

// TestOnlyNibsOwnLayerIsEverTakenOut — the dangerous direction. Invisible text Nib did not stamp is another
// program's layer or a user's own content, and a page that holds any is left whole, byte for byte.
func TestOnlyNibsOwnLayerIsEverTakenOut(t *testing.T) {
	const helv = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
	other := testpdf.WithContent("BT /F1 12 Tf 3 Tr 72 700 Td (other) Tj ET", helv)
	word := []Word{{Page: 1, Rect: [4]float64{72, 600, 140, 612}, Text: "Invoice"}}

	// A text watermark set invisibly in a face Nib does not stamp a layer in: the stamp's shape, somebody's content.
	model.NewDefaultConfiguration()
	wm, err := api.TextWatermark("Hidden", "fontname:Helvetica, points:12, scalefactor:1 abs, position:bl, offset:72 500, rotation:0", true, false, types.POINTS)
	if err != nil {
		t.Fatal(err)
	}
	wm.RenderMode = draw.RenderMode(3)
	plain, err := testpdf.Text("print")
	if err != nil {
		t.Fatal(err)
	}
	foreignStamp, err := addWatermarks(plain, map[int][]*model.Watermark{1: {wm}})
	if err != nil {
		t.Fatal(err)
	}
	// Another program's words with Nib's stamped over them — what a file OCR'd twice before ADR-094 can hold.
	mixed, err := StampTextLayer(other, word, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(hiddenOn(t, mixed, 1)); n != 2 {
		t.Fatalf("the mixed fixture holds %d invisible runs, want 2", n)
	}
	// Nib's own stamp, departed from in one respect each: a word is Nib's only when all of it is the stamp's shape.
	stamped, err := StampTextLayer(plain, word, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if kinds, kerr := TextLayers(stamped); kerr != nil || kinds[1] != LayerOwn {
		t.Fatalf("the untampered stamp reads as %q (%v), want %q — the three rows built on it would show nothing", kinds[1], kerr, LayerOwn)
	}
	paints := tampered(t, stamped, true, func(b []byte) []byte { return append(b, " 0 0 5 5 re f"...) })
	twoStrings := tampered(t, stamped, true, func(b []byte) []byte {
		return bytes.Replace(b, []byte("Tj"), []byte("Tj 0 -14 Td (\x00\x25) Tj"), 1)
	})
	otherMarker := tampered(t, stamped, false, func(b []byte) []byte {
		return bytes.Replace(b, []byte("/Subtype /Watermark"), []byte("/Subtype /Header"), 1)
	})
	for name, pdf := range map[string][]byte{
		"another program's layer":                        other,
		"an invisible stamp in a face that is not Nib's": foreignStamp,
		"another program's words under Nib's":            mixed,
		"a word's form that also paints":                 paints,
		"a form that sets two strings":                   twoStrings,
		"an artifact that is not a stamp's marker":       otherMarker,
	} {
		t.Run(name, func(t *testing.T) {
			if kinds, err := TextLayers(pdf); err != nil || kinds[1] != LayerOther {
				t.Errorf("the page reads as %q (%v), want %q", kinds[1], err, LayerOther)
			}
			was := hiddenWords(hiddenOn(t, pdf, 1))
			out, removed, left, err := removeOwnTextLayers(pdf, map[int]bool{1: true})
			if err != nil || len(removed) != 0 || left[1] != LayerOther {
				t.Errorf("removed %v, left %v, %v — want nothing removed and page 1 left as %q", removed, left, err, LayerOther)
			}
			if !bytes.Equal(out, pdf) {
				t.Errorf("a page that is not Nib's own layer was rewritten")
			}
			res, _, replaced, left, err := ReplaceOCRLayer(pdf, word, "eng")
			if err != nil || res != nil || len(replaced) != 0 || left[1] != LayerOther {
				t.Errorf("a replace changed the document (%v), replaced %v, left %v, %v", res != nil, replaced, left, err)
			}
			if now := hiddenWords(hiddenOn(t, pdf, 1)); !reflect.DeepEqual(was, now) {
				t.Errorf("the page's invisible text was %v and is %v", was, now)
			}
		})
	}
}

// TestAShortStampIsBroughtUpToDateByReplacingIt — the file this exists for: OCR'd before ADR-092, every word about
// 0.72 of its scanned width. Replaced, each word spans its box and none of the short ones is left under it.
func TestAShortStampIsBroughtUpToDateByReplacingIt(t *testing.T) {
	words := ocrWords()
	old := shortStamp(t, scannedPages(t, 1), words)
	for _, r := range hiddenOn(t, old, 1) {
		if !r.Short {
			t.Fatalf("the fixture's word %q is not a short stamp, so this test would show nothing", r.Text)
		}
	}
	if kinds, err := TextLayers(old); err != nil || kinds[1] != LayerOwn {
		t.Fatalf("a layer Nib stamped before ADR-092 reads as %q (%v), want %q", kinds[1], err, LayerOwn)
	}
	out, _, replaced, _, err := ReplaceOCRLayer(old, words, "eng")
	if err != nil || !reflect.DeepEqual(replaced, []int{1}) {
		t.Fatalf("replace: replaced %v, %v", replaced, err)
	}
	got := hiddenOn(t, out, 1)
	if len(got) != len(words) {
		t.Fatalf("the page holds %d invisible words after the replace, want %d", len(got), len(words))
	}
	pm, _ := MapPage(out, 1)
	for _, r := range got {
		if r.Short {
			t.Errorf("%q is still a short stamp", r.Text)
		}
		for _, w := range words {
			if w.Text == r.Text {
				if x1 := r.Rect[2] * pm.Width; x1 < w.Rect[2]-0.5 || x1 > w.Rect[2]+0.5 {
					t.Errorf("%q ends at %.1f, its scanned box at %.1f", r.Text, x1, w.Rect[2])
				}
			}
		}
	}
}

// TestALayerIsReplacedInAnotherLanguageAndARightToLeftWordIsSetAsADR097Sets — the other two reasons to read a page
// again: the wrong language was chosen, or the layer predates ADR-097.
func TestALayerIsReplacedInAnotherLanguageAndARightToLeftWordIsSetAsADR097Sets(t *testing.T) {
	first, _, err := TagOCRLayer(scannedPages(t, 1), ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	heb := []Word{{Page: 1, Rect: [4]float64{72, 700, 120, 714}, Text: "שלום", Block: 1, Para: 2, Line: 3}}
	out, _, replaced, _, err := ReplaceOCRLayer(first, heb, "heb")
	if err != nil || !reflect.DeepEqual(replaced, []int{1}) {
		t.Fatalf("replace: replaced %v, %v", replaced, err)
	}
	got := hiddenOn(t, out, 1)
	if len(got) != 1 || got[0].Text != "שלום" || !got[0].Reversed {
		t.Errorf("the page's invisible text is %+v, want the one Hebrew word, set in reverse and read the right way", got)
	}
	noDanglingStructure(t, out, "after the replace")
}

// TestAPageNotAskedForKeepsItsLayerAndItsStructure — the replace is per page. Words for page 1 only: page 2's layer,
// its elements and its row of the parent tree are what they were.
func TestAPageNotAskedForKeepsItsLayerAndItsStructure(t *testing.T) {
	words := append(ocrWords(), Word{Page: 2, Rect: [4]float64{72, 700, 140, 712}, Text: "Second", Block: 1, Para: 2, Line: 3})
	first, tagged, err := TagOCRLayer(scannedPages(t, 2), words, "eng")
	if err != nil || !tagged {
		t.Fatalf("the first layer: tagged %v, %v", tagged, err)
	}
	again := []Word{{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Again", Block: 1, Para: 2, Line: 3}}
	out, _, replaced, left, err := ReplaceOCRLayer(first, again, "eng")
	if err != nil || !reflect.DeepEqual(replaced, []int{1}) || len(left) != 0 {
		t.Fatalf("replace: replaced %v, left %v, %v", replaced, left, err)
	}
	if got := hiddenWords(hiddenOn(t, out, 1)); !reflect.DeepEqual(got, []string{"Again"}) {
		t.Errorf("page 1 holds %v, want the one new word", got)
	}
	if got := hiddenWords(hiddenOn(t, out, 2)); !reflect.DeepEqual(got, []string{"Second"}) {
		t.Errorf("page 2 holds %v, want its own word untouched", got)
	}
	noDanglingStructure(t, out, "after replacing one page of two")
	if !strings.Contains(string(pageStream(t, out, 2)), "/MCID") {
		t.Errorf("page 2's word is no longer described")
	}
}

// TestALayerWhoseStructureCannotBeTakenOutWithItIsLeftAlone — a word two elements both claim. Taking the word out
// from under either would leave the tree saying something nobody decided, so the page is refused, with the cause,
// and nothing is written.
func TestALayerWhoseStructureCannotBeTakenOutWithItIsLeftAlone(t *testing.T) {
	first, tagged, err := TagOCRLayer(scannedPages(t, 1), ocrWords(), "eng")
	if err != nil || !tagged {
		t.Fatalf("the first layer: tagged %v, %v", tagged, err)
	}
	twice, err := writeMutated(first, func(ctx *model.Context) error {
		tree, terr := readStructTree(ctx, livePageObjects(ctx))
		if terr != nil {
			return terr
		}
		_, pageRef, perr := tree.page(ctx, 1)
		if perr != nil {
			return perr
		}
		rootRef, rerr := structTreeRootRef(ctx)
		if rerr != nil {
			return rerr
		}
		ref, nerr := ctx.IndRefForNewObject(types.Dict{"Type": types.Name("StructElem"), "S": types.Name("P"),
			"P": *rootRef, "Pg": *pageRef, "K": types.Array{types.Integer(0)}})
		if nerr != nil {
			return nerr
		}
		return appendToRootKids(ctx, tree, *ref)
	})
	if err != nil {
		t.Fatal(err)
	}
	if kinds, kerr := TextLayers(twice); kerr != nil || kinds[1] != LayerStructure {
		t.Errorf("the page reads as %q (%v), want %q", kinds[1], kerr, LayerStructure)
	}
	out, _, replaced, left, err := ReplaceOCRLayer(twice, ocrWords(), "eng")
	if err != nil || out != nil || len(replaced) != 0 || left[1] != LayerStructure {
		t.Errorf("replace changed the document (%v), replaced %v, left %v, %v — want it refused as %q", out != nil, replaced, left, err, LayerStructure)
	}
}

// TestAWatermarkStampedAfterTheLayerStaysWhenTheLayerGoes — the stamp's own `q … Q` wrappers come off with the
// words only where they are the outermost thing on the page. A watermark added since wraps them in turn; it is the
// user's, it is visible, and it and its wrapper are still there, balanced, when the layer has gone.
func TestAWatermarkStampedAfterTheLayerStaysWhenTheLayerGoes(t *testing.T) {
	text, err := testpdf.Text("print")
	if err != nil {
		t.Fatal(err)
	}
	layered, err := StampTextLayer(text, ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	marked, err := StampWatermark(layered, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	out, removed, _, err := removeOwnTextLayers(marked, map[int]bool{1: true})
	if err != nil || !reflect.DeepEqual(removed, []int{1}) {
		t.Fatalf("removed %v, %v — want [1]", removed, err)
	}
	if n := len(hiddenOn(t, out, 1)); n != 0 {
		t.Errorf("%d invisible word(s) are still on the page", n)
	}
	stream := pageStream(t, out, 1)
	if n := len(watermarkArtifactSpans(stream)); n != 1 {
		t.Errorf("the page draws %d watermark(s) with the layer gone, want the one stamped after it", n)
	}
	opens, closes, ends := 0, 0, 0
	for _, f := range strings.Fields(string(stream)) {
		switch f {
		case "q":
			opens++
		case "Q":
			closes++
		case "EMC":
			ends++
		}
	}
	if opens != closes || ends != 1 {
		t.Errorf("the page's content is unbalanced with the layer gone: %d q, %d Q, %d EMC (want one, the watermark's)", opens, closes, ends)
	}
	if err := Validate(out); err != nil {
		t.Errorf("the document no longer validates: %v", err)
	}
}
