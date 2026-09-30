package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 709` — the P08 phase-close review's page-selection findings (R2-3..R2-13): what a kept
// page's annotations, form and resources still reach after a subset, and the splitters' one cap.

// keptAnnots returns the annotation dictionaries of every page of pdf, in page order.
func keptAnnots(t *testing.T, pdf []byte) [][]types.Dict {
	t.Helper()
	ctx := readCtx(t, pdf)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	leaves, _, err := collectLeaves(ctx.XRefTable, root)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]types.Dict
	for _, l := range leaves {
		var page []types.Dict
		for _, a := range derefArray(ctx.XRefTable, l.dic["Annots"]) {
			if ad := derefDict(ctx.XRefTable, a); ad != nil {
				page = append(page, ad)
			}
		}
		out = append(out, page)
	}
	return out
}

// namedDestDoc is three pages. The /Dests name tree maps "one" to page 1 and "two" to page 2; page 2
// carries four links — to "two" in each of the three named shapes, and to "one" as a string.
func namedDestDoc() []byte {
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Names << /Dests 20 0 R >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 /MediaBox [0 0 612 792] >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R /Annots [10 0 R 11 0 R 12 0 R 13 0 R] >>",
		5:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R >>",
		6:  stream("0 0 10 10 re f"),
		10: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /T (str) /Dest (two) >>",
		11: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /T (name) /Dest /two >>",
		12: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /T (goto) /A << /S /GoTo /D (two) >> >>",
		13: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /T (gone) /Dest (one) >>",
		20: "<< /Names [(one) [3 0 R /Fit] (two) [4 0 R /Fit]] >>",
	})
}

// TestALinkToANamedDestinationOnAKeptPageKeepsItsTarget — R2-4. The link unlinker asked a question
// that cannot resolve a name, so every Word or hyperref internal link lost its target in every subset,
// kept page or not. The control is the link whose name DID go.
func TestALinkToANamedDestinationOnAKeptPageKeepsItsTarget(t *testing.T) {
	out, err := RemovePages(namedDestDoc(), []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	pages := keptAnnots(t, out)
	if len(pages) != 2 || len(pages[0]) != 4 {
		t.Fatalf("setup: want 2 pages with 4 links on the first, got %d pages (%v)", len(pages), pages)
	}
	for _, ad := range pages[0] {
		title, _ := litString(ad["T"])
		_, dest := ad["Dest"]
		_, act := ad["A"]
		if title == "gone" {
			if dest || act {
				t.Error("a link to a named destination on the REMOVED page kept its target")
			}
			continue
		}
		if !dest && !act {
			t.Errorf("the %q link names a destination on a KEPT page and lost its target", title)
		}
	}
}

// replyDoc is two pages. Page 1 draws secretText and carries annotation 10; page 2 carries annotation
// 11, which names 10 under key (an /IRT reply, or a /Popup), and — the control — annotation 12, a
// reply to 13 on its own page.
func replyDoc(key string) []byte {
	parent := "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (parent) /P 3 0 R >>"
	if key == "Popup" {
		parent = "<< /Type /Annot /Subtype /Popup /Rect [0 0 10 10] /P 3 0 R >>"
	}
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> >>",
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [10 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 6 0 R /Annots [11 0 R 12 0 R 13 0 R] >>",
		5:  stream(fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", secretText)),
		6:  stream(fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", publicText)),
		10: parent,
		11: fmt.Sprintf("<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (reply) /P 4 0 R /%s 10 0 R >>", key),
		12: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (local) /P 4 0 R /IRT 13 0 R >>",
		13: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (localparent) /P 4 0 R >>",
	})
}

// TestAReplyToACommentOnARemovedPageDoesNotShipThatPage — R2-3. The reply's /IRT reached the removed
// page's annotation, whose /P is the removed page, and pdfcpu writes by reachability.
func TestAReplyToACommentOnARemovedPageDoesNotShipThatPage(t *testing.T) {
	for _, key := range []string{"IRT", "Popup"} {
		t.Run(key, func(t *testing.T) {
			src := replyDoc(key)
			if !bytes.Contains(everyDecodedByte(t, src), []byte(secretText)) {
				t.Fatal("setup: the source does not carry page 1's text, so its absence proves nothing")
			}
			out, err := RemovePages(src, []string{"1"})
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(everyDecodedByte(t, out), []byte(secretText)) {
				t.Errorf("removing page 1 shipped page 1's text: a kept annotation's /%s reached the "+
					"removed page's annotation, and its /P the removed page", key)
			}
			pages := keptAnnots(t, out)
			if len(pages) != 1 {
				t.Fatalf("want one page, got %d", len(pages))
			}
			local := false
			for _, ad := range pages[0] {
				if c, _ := litString(ad["Contents"]); c == "local" {
					_, local = ad["IRT"]
				}
			}
			if !local {
				t.Error("a reply to a comment on the SAME kept page lost its /IRT — the thread is not cut " +
					"where nothing was removed")
			}
		})
	}
}

// TestAFormWhoseOnlyNestedWidgetWentIsReportedChanged — R2-5. A parent field keeps one of its two
// widgets, so the top-level count is unchanged while its /Kids is rewritten; the prune used to report
// "unchanged", and its caller skipped the write that carried the rewrite.
func TestAFormWhoseOnlyNestedWidgetWentIsReportedChanged(t *testing.T) {
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [7 0 R] >>",
		5: stream("0 0 10 10 re f"),
		6: "<< /T (f) /FT /Tx /Kids [7 0 R 8 0 R] >>",
		7: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /Parent 6 0 R /P 3 0 R >>",
		8: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /Parent 6 0 R >>",
	})
	ctx := readCtx(t, src)
	changed, err := pruneOrphanedAcroForm(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("a field lost one of its two widgets and the prune reported no change, so the write " +
			"that carries the rewritten /Kids is skipped and the orphaned widget stays in the bytes")
	}
	root, _ := ctx.XRefTable.Catalog()
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	if form == nil {
		t.Fatal("the form went, though one widget is still on the page")
	}
	f := derefDict(ctx.XRefTable, derefArray(ctx.XRefTable, form["Fields"])[0])
	if n := len(derefArray(ctx.XRefTable, f["Kids"])); n != 1 {
		t.Errorf("the field has %d kid(s), want the 1 whose widget is on a page", n)
	}
}

// TestASplitByBookmarksRefusesPastThePartCap — R2-6. The cap bound one splitter; each part of the
// other is a full Collect, so a document of thousands of top-level bookmarks was thousands of parses.
func TestASplitByBookmarksRefusesPastThePartCap(t *testing.T) {
	n := maxSplitParts + 1
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Outlines 3 0 R >>",
		4: stream("0 0 10 10 re f"),
	}
	var kids []string
	for i := 0; i < n; i++ {
		pg, it := 10+i, 10+n+i
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
		objs[pg] = "<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>"
		item := fmt.Sprintf("<< /Title (b%d) /Parent 3 0 R /Dest [%d 0 R /Fit]", i, pg)
		if i > 0 {
			item += fmt.Sprintf(" /Prev %d 0 R", it-1)
		}
		if i < n-1 {
			item += fmt.Sprintf(" /Next %d 0 R", it+1)
		}
		objs[it] = item + " >>"
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", strings.Join(kids, " "), n)
	objs[3] = fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>", 10+n, 10+2*n-1, n)
	src := assembleFixture(objs)

	start := time.Now()
	parts, err := SplitByBookmarks(src, "")
	took := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "too many output files") {
		t.Fatalf("a split into %d bookmark parts returned %d part(s) and err=%v in %v, want the part-cap "+
			"refusal SplitBySpans gives", n, len(parts), err, took)
	}
}

// fieldDAGSigAtTheTop is `fieldDAG` with the signature's /FT on a FIELD, inherited by the widget, and
// none on the widget itself — so only the field-tree walk can find it. It sits on the widget's parent
// rather than the top because pdfcpu validates a non-terminal /Sig field against its first kid's
// /Rect. `fieldDAG`'s own signature
// sits on a merged field/widget in page 3's /Annots, which the page-annotation route finds as well.
func fieldDAGSigAtTheTop(k, levels int) []byte {
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
		switch {
		case n == last:
			objs[n] = fmt.Sprintf("<< /T (f%d) /Subtype /Widget /Rect [0 0 10 10] /P 3 0 R >>", l)
		case n == last-1:
			objs[n] = fmt.Sprintf("<< /T (f%d) /FT /Sig /Kids [%s] >>", l, strings.Repeat(fmt.Sprintf("%d 0 R ", n+1), k))
		default:
			objs[n] = fmt.Sprintf("<< /T (f%d) /Kids [%s] >>", l, strings.Repeat(fmt.Sprintf("%d 0 R ", n+1), k))
		}
	}
	return assembleFixture(objs)
}

// TestASignatureInheritedDownAFieldDAGIsErasedByTheFieldWalk — R2-7. The field-tree route alone can
// find this signature, so a regression in it goes red here, which it could not in
// TestASignatureAtTheEndOfAFieldDAGIsStillErased.
func TestASignatureInheritedDownAFieldDAGIsErasedByTheFieldWalk(t *testing.T) {
	out, err := RemovePages(fieldDAGSigAtTheTop(4, 12), []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), subsetConf())
	if rerr != nil {
		t.Fatal(rerr)
	}
	root, _ := ctx.XRefTable.Catalog()
	if form := derefDict(ctx.XRefTable, root["AcroForm"]); form != nil {
		eachFormField(ctx.XRefTable, form, func(_ types.Object, d types.Dict) {
			if nameVal(d, "FT") == "Sig" {
				t.Error("a signature field whose /FT is inherited from a field of a 12-level field DAG " +
					"survived a page operation")
			}
		})
	}
	if pages := keptAnnots(t, out); len(pages) != 1 || len(pages[0]) != 0 {
		t.Errorf("the signature's widget is still on the kept page: %v", pages)
	}
}

// inheritedSigDoc is two pages; page 1 carries a signature widget whose /FT /Sig is on its
// GRANDPARENT field, and the catalog has no /AcroForm at all — so the page-annotation route is the
// only one that can find it.
func inheritedSigDoc() []byte {
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [9 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>",
		5:  stream("0 0 10 10 re f"),
		7:  "<< /T (top) /FT /Sig /Kids [8 0 R] >>",
		8:  "<< /T (mid) /Parent 7 0 R /Kids [9 0 R] >>",
		9:  "<< /Type /Annot /Subtype /Widget /Rect [10 10 110 60] /Parent 8 0 R /P 3 0 R /AP << /N 10 0 R >> >>",
		10: "<< /Type /XObject /Subtype /Form /BBox [0 0 100 50] /Length 14 >>\nstream\n0 0 10 10 re f\nendstream",
	})
}

// TestASignatureWidgetTwoLevelsUnderItsTypeIsStillOne — R2-13, at both sites that ask of a single
// dictionary: `SignatureWidgets` and `dropSignature`'s page route. /FT is inheritable from any
// ancestor; both read the widget and at most its direct parent.
func TestASignatureWidgetTwoLevelsUnderItsTypeIsStillOne(t *testing.T) {
	src := inheritedSigDoc()
	ws, err := SignatureWidgets(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0].Page != 1 || !ws[0].HasAP {
		t.Errorf("SignatureWidgets = %+v, want the one widget on page 1 with its appearance", ws)
	}
	out, err := RemovePages(src, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if pages := keptAnnots(t, out); len(pages) != 1 || len(pages[0]) != 0 {
		t.Errorf("a signature widget whose /FT is on its grandparent survived a page operation: %v", pages)
	}
}

// TestThePruneKeepsWhatType3SoftMaskAndTilingPatternRoadsDraw — R2-8. Three roads into the resource
// closure had no test: a Type 3 font's /CharProcs, an ExtGState's /SMask /G group and a tiling
// pattern. Each draws an image only through that road, with no /Resources of its own, so it reads the
// page's; deleting the road blanks the image. The control is an image nothing names.
func TestThePruneKeepsWhatType3SoftMaskAndTilingPatternRoadsDraw(t *testing.T) {
	img := "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n\x00\nendstream"
	glyph := "0 0 0 0 1 1 d1 /ImA Do"
	group := "/ImB Do"
	tile := "/ImC Do"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources 11 0 R >>",
		11: "<< /XObject << /ImA 20 0 R /ImB 21 0 R /ImC 22 0 R /ImD 23 0 R >> /Font << /T3 6 0 R >> " +
			"/ExtGState << /GSm << /SMask << /S /Luminosity /G 8 0 R >> >> >> /Pattern << /P1 9 0 R >> >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>",
		4: "<< /Type /Page /Parent 2 0 R /Contents 10 0 R >>",
		5: stream("BT /T3 12 Tf (a) Tj ET /GSm gs /Pattern cs /P1 scn 0 0 10 10 re f"),
		6: "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 1 1] /FontMatrix [1 0 0 1 0 0] /CharProcs << /a 7 0 R >> " +
			"/Encoding << /Differences [97 /a] >> /FirstChar 97 /LastChar 97 /Widths [1] >>",
		7:  stream(glyph),
		8:  fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Group << /S /Transparency >> /Length %d >>\nstream\n%s\nendstream", len(group), group),
		9:  fmt.Sprintf("<< /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources 12 0 R /Length %d >>\nstream\n%s\nendstream", len(tile), tile),
		10: stream("0 0 1 1 re f"),
		12: "<< /XObject << /ImC 22 0 R /ImD 23 0 R >> >>",
		20: img, 21: img, 22: img, 23: img,
	}
	out, err := collectWithoutStructure(assembleFixture(objs), []string{"1"}, refuseUnreadable)
	if err != nil {
		t.Fatal(err)
	}
	got := keptResources(t, out)
	for im, road := range map[string]string{"ImA": "a Type 3 glyph's CharProc", "ImB": "an ExtGState's soft-mask group"} {
		if !has(got["XObject"], im) {
			t.Errorf("/%s is drawn by %s on the kept page and was pruned (kept: %v)", im, road, got["XObject"])
		}
	}
	if has(got["XObject"], "ImD") {
		t.Error("/ImD is named by nothing the kept page draws and survived the prune")
	}
	// The tiling pattern has /Resources of its own (pdfcpu refuses one without), so it is an OWNER: its
	// own dictionary is pruned to what its cell draws. (It is a dictionary of its own rather than the
	// shared one because the shared one names the pattern itself, and that overflows pdfcpu's
	// validator's stack.)
	ctx := readCtx(t, out)
	root, _ := ctx.XRefTable.Catalog()
	leaves, _, err := collectLeaves(ctx.XRefTable, root)
	if err != nil || len(leaves) != 1 {
		t.Fatalf("want one page, got %d (%v)", len(leaves), err)
	}
	pres := derefDict(ctx.XRefTable, leaves[0].dic["Resources"])
	pat, _, err := ctx.XRefTable.DereferenceStreamDict(derefDict(ctx.XRefTable, pres["Pattern"])["P1"])
	if err != nil || pat == nil {
		t.Fatalf("the kept page's tiling pattern is gone (%v)", err)
	}
	cell := derefDict(ctx.XRefTable, derefDict(ctx.XRefTable, pat.Dict["Resources"])["XObject"])
	if cell["ImC"] == nil {
		t.Errorf("/ImC is drawn by the tiling pattern's cell and was pruned from its resources (kept: %v)", cell)
	}
	if cell["ImD"] != nil {
		t.Error("the tiling pattern's own resources still name /ImD, which its cell never draws — the " +
			"pattern road was not walked, so its shared dictionary went out whole")
	}
}

// litString reads a direct string literal, which is how every fixture above writes one.
func litString(o types.Object) (string, bool) {
	if v, ok := o.(types.StringLiteral); ok {
		s, err := types.StringLiteralToString(v)
		return s, err == nil
	}
	return "", false
}
