package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P02.S08's readers — MCIDs inside a Form XObject are reached through MCR dictionaries.
//
// Every one of these was red before the slice and each was probed separately, because the defect
// they cover is invisible to every instrument that existed: veraPDF and `nib ua` score the broken
// and the repaired n-up **identically** (measured: `5 t1`, `7.2 t33`, `7.2 t34` for both, all of
// them the fixture's missing `/Lang`). A slice graded only on those would have shipped on a green
// that could not have gone red.

// collidingMCIDFixture is two pages whose marked content is DIFFERENT and whose MCIDs are the SAME.
//
// **The collision is the stimulus and it is not incidental.** An n-up draws both pages' content as
// two form XObjects on one sheet; both carry an MCID 0, because MCIDs are numbered per content
// stream and every producer starts at zero. A reader keying text by `(page, mcid)` then hands both
// texts to both elements. `repeatedPagesFixture` cannot show this — its two pages are byte-identical
// by design, so the merged reading and the correct reading are the same string.
func collidingMCIDFixture() []byte {
	one := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (ALPHA) Tj ET\nEMC\n"
	two := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (BRAVO) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(one), one),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 13 0 R /K [0] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 14 0 R /StructParents 1 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(two), two),
	})
}

// textsOf returns every element's read-back text, in tree order.
func textsOf(t *testing.T, pdf []byte) []string {
	t.Helper()
	v := viewOf(t, pdf)
	out := make([]string, 0, len(v.elements))
	for _, e := range v.elements {
		out = append(out, e.text)
	}
	return out
}

// TestACarriedNUpReadsItsOwnText is the slice's point, and the only reader that grades it.
//
// It compares the carried document's text against the SOURCE document's, element for element,
// rather than against a literal — a literal would pass a carry that lost both texts and invented
// two others, and it would have to be rewritten every time the fixture's words changed.
func TestACarriedNUpReadsItsOwnText(t *testing.T) {
	src := collidingMCIDFixture()

	// ── Stimulus, asserted before the response is graded.
	//
	// Both pages must really use MCID 0, or there is no collision and this test grades nothing. It
	// is checked through the model rather than by reading the fixture's own string, because the
	// fixture is what would drift.
	before := textsOf(t, src)
	if len(before) != 2 {
		t.Fatalf("the fixture has %d element(s), want 2 — the collision needs two", len(before))
	}
	if before[0] == before[1] {
		t.Fatalf("both elements read %q, so a merged reading and a correct one are the same string "+
			"and this test cannot fail", before[0])
	}
	stree, terr := readTree(t, src)
	if terr != nil {
		t.Fatalf("readTree: %v", terr)
	}
	for _, e := range stree.elems {
		for _, k := range e.kids {
			if k.kind == kidMCID && k.mcid != 0 {
				t.Fatalf("an element's kid is /MCID %d, not 0 — the two pages no longer collide", k.mcid)
			}
		}
	}

	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	// **`fate` alone would pass on the source returned unchanged** — the fixture is already
	// `carried`, so that check grades the tree and not the composition. Assert the n-up HAPPENED.
	nctx, nerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if nerr != nil {
		t.Fatalf("read: %v", nerr)
	}
	if nctx.PageCount != 1 {
		t.Fatalf("NUp(2) over two pages left %d page(s); nothing was composed, so the collision this "+
			"test grades never arose", nctx.PageCount)
	}
	if got := fate(out); got != "carried" {
		t.Fatalf("the n-up reports %q, so there is no carried tree to read", got)
	}
	after := textsOf(t, out)
	if len(after) != len(before) {
		t.Fatalf("the carry left %d element(s) where the source had %d", len(after), len(before))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("element %d reads %q after the carry and %q before it — an MCID was resolved in "+
				"a stream it does not live in", i, after[i], before[i])
		}
	}

	// ── The GEOMETRY half, which the text half cannot see.
	//
	// The same cross-stream merge unions the two source pages' boxes into one rectangle spanning the
	// sheet — measured before the fix at `[104.0, 44.8, 360.9, 453.6]` for an element whose source box
	// was `[72, 694, 112.5, 716]`. That rectangle is what the Tags panel highlights, so it is a
	// user-visible half of the same defect and it goes green on the text fix alone.
	vsrc, vout := viewOf(t, src), viewOf(t, out)
	for i := range vout.elements {
		a, b := vsrc.elements[i], vout.elements[i]
		if !a.hasRect || !b.hasRect {
			t.Errorf("element %d: hasRect is %v in the source and %v after the carry", i, a.hasRect, b.hasRect)
			continue
		}
		// An n-up scales and translates, so the box moves — but its PROPORTIONS are the page's own,
		// and a box unioned across two source pages is wider or taller than the page allows. Compare
		// the fraction of the sheet each box covers against the fraction of its page the source
		// covered; a merge inflates it by the number of tiles.
		srcW, srcH := a.rect[2]-a.rect[0], a.rect[3]-a.rect[1]
		outW, outH := b.rect[2]-b.rect[0], b.rect[3]-b.rect[1]
		if srcW <= 0 || srcH <= 0 {
			t.Fatalf("element %d has a degenerate source box %v", i, a.rect)
		}
		if r := outW / srcW; r > 1.0 {
			t.Errorf("element %d's box is %.2f× WIDER after an n-up that only ever shrinks: %v from %v "+
				"— the box was unioned across content the element does not own", i, r, b.rect, a.rect)
		}
		if r := outH / srcH; r > 1.0 {
			t.Errorf("element %d's box is %.2f× TALLER after an n-up that only ever shrinks: %v from %v",
				i, r, b.rect, a.rect)
		}
	}
}

// eachStructDict walks the whole structure tree in RAW form — every `/K` entry, direct or indirect,
// at every depth — and hands each dictionary to fn with its `/Type` (empty when it has none, which
// Table 323 permits for a structure element).
//
// It exists because the model deliberately does not keep every key, and `/Stm` on the wrong dictionary
// is exactly the kind of key a model drops: the first version of the test below scanned only INDIRECT
// objects, and the MCRs this carry writes are DIRECT dicts inside `/K`, so it reached nothing and
// asserted nothing. The `-v` run printed its own escape hatch and passed.
func eachStructDict(t *testing.T, ctx *model.Context, o types.Object, seen map[int]bool, fn func(ty string, d types.Dict)) {
	t.Helper()
	if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
		for _, x := range arr {
			eachStructDict(t, ctx, x, seen, fn)
		}
		return
	}
	if ir, isInd := o.(types.IndirectRef); isInd {
		nr := ir.ObjectNumber.Value()
		if seen[nr] {
			return
		}
		seen[nr] = true
	}
	d, e := ctx.DereferenceDict(o)
	if e != nil || d == nil {
		return
	}
	ty := ""
	if n := d.NameEntry("Type"); n != nil {
		ty = *n
	}
	fn(ty, d)
	if k, ok := d["K"]; ok {
		eachStructDict(t, ctx, k, seen, fn)
	}
}

// TestOnlyAnMCRCarriesAStmKey — `/Stm` is Table 324's and nobody else's.
//
// ISO 32000-1 Table 323 enumerates a structure element dictionary's keys and `/Stm` is not among
// them; Table 325 does the same for an object reference. A key outside its own table is a key a
// conforming reader ignores, which is worse than absent: it reads as an anchor and anchors nothing.
//
// **Both halves are asserted, and the coverage count is asserted first.** A scan that reaches nothing
// reports "no illegal key" in exactly the words a correct document does — which is what the first
// version of this test did for its whole life, and it is the reason `eachStructDict` exists.
func TestOnlyAnMCRCarriesAStmKey(t *testing.T) {
	out, err := NUp(collidingMCIDFixture(), 2, false)
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatalf("read: %v", rerr)
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		t.Fatalf("catalog: %v", cerr)
	}
	root, derr := ctx.DereferenceDict(cat["StructTreeRoot"])
	if derr != nil || root == nil {
		t.Fatalf("the carried document has no /StructTreeRoot to scan: %v", derr)
	}

	elems, mcrs, withStm := 0, 0, 0
	eachStructDict(t, ctx, root["K"], map[int]bool{}, func(ty string, d types.Dict) {
		_, has := d["Stm"]
		switch ty {
		case "MCR":
			mcrs++
			if has {
				withStm++
			}
		case "OBJR":
			if has {
				t.Errorf("an /OBJR carries /Stm; Table 325 is /Type, /Pg and /Obj, and nothing else")
			}
		default: // StructElem, or an untyped dict, which Table 323 says is one
			elems++
			if has {
				t.Errorf("element %v carries /Stm, a key Table 323 does not define — a conforming "+
					"reader ignores it and resolves the MCID against /Pg's own stream, which is the "+
					"failure it was written to prevent", d["S"])
			}
		}
	})

	// Stimulus before response: the scan must have reached both kinds, or its silence means nothing.
	if elems == 0 {
		t.Fatal("the scan reached no structure element, so finding no illegal /Stm says nothing")
	}
	if mcrs == 0 {
		t.Fatal("the scan reached no MCR, so the carry wrote none and this document is not the " +
			"shape this test grades")
	}
	if withStm != mcrs {
		t.Errorf("%d of %d MCRs name a stream; a marked-content reference with no /Stm claims its "+
			"content is in the page's own stream, which after an n-up holds only the Do", withStm, mcrs)
	}
}

// TestTheCarryWritesMCRKidsNamingTheForm — the positive half.
//
// An element whose content moved into a form must say so, and the only dictionary allowed to say it
// is a marked-content reference. Asserted against the set of forms drawn on a sheet, not merely
// against "some /Stm is present" — **which is weaker than naming the right one**: swapping the two
// elements' `/Stm` between the two forms passes here. `TestACarriedNUpReadsItsOwnText` is what
// catches that, by reading the text back, and the two are meant to be read together.
func TestTheCarryWritesMCRKidsNamingTheForm(t *testing.T) {
	out, err := NUp(collidingMCIDFixture(), 2, false)
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatalf("read: %v", rerr)
	}
	forms := map[int]bool{}
	seen := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			continue
		}
		res, e := ctx.DereferenceDict(d["Resources"])
		if e != nil || res == nil {
			continue
		}
		eachFormXObject(ctx, res, seen, 0, func(nr int, sd *types.StreamDict) { forms[nr] = true })
	}
	if len(forms) < 2 {
		t.Fatalf("the n-up left %d form XObject(s); the carry needs one per source page", len(forms))
	}

	tree, terr := readTree(t, out)
	if terr != nil {
		t.Fatalf("readTree: %v", terr)
	}
	mcrs, ints := 0, 0
	for _, e := range tree.elems {
		for _, k := range e.kids {
			switch k.kind {
			case kidMCR:
				mcrs++
				if k.stm == 0 {
					t.Errorf("an MCR kid names no stream, so its MCID still resolves against the page")
					continue
				}
				if !forms[k.stm] {
					t.Errorf("an MCR kid's /Stm names object %d, which is not a form drawn on any sheet", k.stm)
				}
			case kidMCID:
				ints++
			}
		}
	}
	if mcrs == 0 {
		t.Error("the carry wrote no MCR kids, so nothing records which stream the content moved to")
	}
	if ints != 0 {
		t.Errorf("%d kid(s) are still bare integers, which claim the content is in the page's own "+
			"stream — after an n-up the page's own stream holds only the Do that draws the form", ints)
	}
}

// pageAndFormFixture draws text in the PAGE's own stream and text inside a form the page draws.
//
// `sharedFormFixture` cannot serve here: its page stream is two `Do` operators and nothing else, so a
// test written over it exercises only the in-form arm and its page-stream arm is unreachable — which
// is what the first version of the test below was, silently, while its own doc comment claimed to
// assert both directions.
func pageAndFormFixture() []byte {
	form := "/P <</MCID 1>> BDC\nBT /F1 12 Tf 0 0 Td (in the form) Tj ET\nEMC\n"
	// **The form is drawn FIRST and the page's own sequence opens after it**, and that order is the
	// stimulus rather than a detail. `show` takes a run's stream from the sequence it is inside, so a
	// `drawForm` that set the walker's stream and never restored it would be invisible while every
	// page sequence opened *before* the `Do`. Probed: with the form last, deleting the restore in
	// `drawForm` left this test green. With it first, the page's run is stamped with the form.
	page := "q 1 0 0 1 72 400 cm /Fm0 Do Q\n" +
		"/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (on the page) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /Font << /F1 5 0 R >> /XObject << /Fm0 11 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 3 0 R /Stm 11 0 R /MCID 1 >>] >>",
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] /StructParents 1 "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
	})
}

// TestARunKnowsWhichStreamItWasReadFrom — the reader's half, at its narrowest.
//
// The zero value is load-bearing and is asserted in BOTH directions: a run from the page's own stream
// is 0 by construction, and a stamp that was never written is also 0 — so a test that only checked
// "form runs are non-zero" would pass a walker that stamped everything, and one that only checked the
// page arm would pass a walker that stamped nothing. **Both arms are required to have executed**,
// because the first version of this test ran on a fixture whose page drew no text at all and its
// page-stream arm was therefore dead while the comment claimed otherwise.
func TestARunKnowsWhichStreamItWasReadFrom(t *testing.T) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pageAndFormFixture()), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	pr, perr := readPageRuns(ctx, 1)
	if perr != nil {
		t.Fatalf("readPageRuns: %v", perr)
	}
	inForm, onPage := 0, 0
	for _, r := range pr.runs {
		if r.inForm {
			inForm++
			if r.stm == 0 {
				t.Errorf("a run read from inside a form XObject carries stm=0, so it is "+
					"indistinguishable from one read from the page's own stream (text %q)", r.text)
			}
		} else {
			onPage++
			if r.stm != 0 {
				t.Errorf("a run drawn in the page's own stream carries stm=%d; the page's stream is "+
					"named by /Pg and never by /Stm (text %q)", r.stm, r.text)
			}
		}
	}
	if inForm == 0 {
		t.Fatal("the fixture produced no run inside a form XObject, so the in-form arm was never exercised")
	}
	if onPage == 0 {
		t.Fatal("the fixture produced no run in the page's own stream, so the page arm was never " +
			"exercised and half this test's claim is unasserted")
	}
}

// TestASequenceOpenedOnThePageKeepsThePagesStream — where the sequence is, not where the glyphs are.
//
// A `BDC` in the page stream around a `Do` tags what the form draws (`runWalker.mcStack` spans the
// boundary deliberately). Table 324 defines `/Stm` as *"the content stream containing the
// marked-content **sequence**"*, so those runs belong to the PAGE's stream — stamping them with the
// form they were drawn in would file the text under a stream no `/Stm` can name, and the element's
// text would come back empty. Found in review before it shipped.
func TestASequenceOpenedOnThePageKeepsThePagesStream(t *testing.T) {
	form := "BT /F1 12 Tf 0 0 Td (drawn inside the form) Tj ET\n"
	page := "/P <</MCID 0>> BDC\nq 1 0 0 1 72 400 cm /Fm0 Do Q\nEMC\n"
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /Font << /F1 5 0 R >> /XObject << /Fm0 11 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
	})
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	pr, perr := readPageRuns(ctx, 1)
	if perr != nil {
		t.Fatalf("readPageRuns: %v", perr)
	}
	seen := 0
	for _, r := range pr.runs {
		if r.mcid != 0 {
			continue
		}
		seen++
		if !r.inForm {
			t.Errorf("setup: the run %q was not drawn inside the form, so this fixture does not "+
				"separate where-opened from where-drawn", r.text)
		}
		if r.stm != 0 {
			t.Errorf("a run whose sequence was opened in the PAGE stream carries stm=%d — it was "+
				"stamped with the stream it was drawn in, and no /Stm will ever name that", r.stm)
		}
	}
	if seen == 0 {
		t.Fatal("no run carried /MCID 0, so nothing was exercised")
	}
	// The element reads its text through the page index, as it always has.
	v := viewOf(t, pdf)
	if len(v.elements) != 1 || v.elements[0].text == "" {
		t.Errorf("the element reads %q; a sequence opened on the page must still resolve its text",
			v.elements[0].text)
	}
}

// TestAnMCRKidCarriesItsStm — the model keeps the key instead of collapsing it away.
//
// Before this slice `readKid` reduced an MCR to `(pgObj, mcid)` and dropped `/Stm` on the floor, so
// the one fact that says which stream the MCID lives in never reached any consumer.
func TestAnMCRKidCarriesItsStm(t *testing.T) {
	tree, terr := readTree(t, sharedFormFixture())
	if terr != nil {
		t.Fatalf("readTree: %v", terr)
	}
	found := false
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if k.kind != kidMCR {
				continue
			}
			found = true
			if k.stm == 0 {
				t.Error("an MCR with /Stm reaches the model with stm=0 — the key was read and discarded")
			}
		}
	}
	if !found {
		t.Fatal("the fixture produced no MCR kid, so nothing was exercised")
	}
}

// TestArtifactingAFormResidentElementIsRefusedAtTheTree — the branch this slice brought to life.
//
// `structartifact.go:79` refuses an element whose kid carries `/Stm`, because the rewrite it would
// perform edits the PAGE's content stream and the content is not there. **The branch was never dead —
// `structartifact_test.go:310-313` has driven a hand-built MCR through it all along — but it was dead
// for every document nib itself produced**: it dereferences the kid (`k.raw`), and a bare integer is
// not a dictionary, so it could not fire on n-up output whose `/Stm` sat on the element instead. Such
// an element was caught later and differently, by the page scan at `structartifact.go:125` or `:139`,
// which reaches the same answer only after tokenizing every page.
//
// Writing the kids as MCRs makes the tree-level refusal the one that fires — and since BOTH sites
// return the same sentinel, the message is what this test has to read.
func TestArtifactingAFormResidentElementIsRefusedAtTheTree(t *testing.T) {
	out, err := NUp(collidingMCIDFixture(), 2, false)
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	v := viewOf(t, out)
	if len(v.elements) < 2 {
		t.Fatalf("setup: the carried document has %d element(s); artifacting the LAST one is refused "+
			"for a different reason entirely (ADR-031), so this needs at least two", len(v.elements))
	}
	target := v.elements[0].id

	// Stimulus: the element really does own marked content that lives in a form. Without this the
	// refusal below could be any of artifactElement's other three, and the test would grade nothing.
	tree, terr := readTree(t, out)
	if terr != nil {
		t.Fatalf("readTree: %v", terr)
	}
	inForm := false
	for _, e := range tree.elems {
		if e.objNr != target {
			continue
		}
		for _, k := range e.kids {
			if k.kind == kidMCR && k.stm > 0 {
				inForm = true
			}
		}
	}
	if !inForm {
		t.Fatalf("setup: element %d owns no marked content in a form, so the /Stm refusal cannot fire", target)
	}

	_, aerr := applyStructEdits(out, []structEdit{{kind: editArtifact, elem: target}})
	if aerr == nil {
		t.Fatal("artifacting an element whose content lives in a form XObject was accepted; the " +
			"rewrite edits the page's content stream, where that content is not")
	}
	if !errors.Is(aerr, errCommitInForm) {
		t.Fatalf("err = %v, want errCommitInForm", aerr)
	}
	// **The sentinel alone cannot grade this, and that is the whole point of the test.**
	// `structartifact.go` returns `errCommitInForm` from two places: the tree-level `/Stm` refusal,
	// which formats `(element N, /MCID M)`, and the page-scan refusal at the bottom, which formats
	// `(page N, /MCID M)`. Before this slice the kids were bare integers, the tree-level check could
	// not dereference them, and the SAME sentinel arrived from the page scan — so asserting only
	// `errors.Is` would have been green before the slice and green after it, for different reasons.
	if got := aerr.Error(); !strings.Contains(got, fmt.Sprintf("element %d", target)) {
		t.Errorf("the refusal reads %q; it should name the ELEMENT, which is the tree-level /Stm "+
			"refusal, rather than a page, which is the content scan reaching the same verdict the "+
			"expensive way", got)
	}
}

// kShapeFixture is two tagged pages whose elements spell `/K` differently.
//
// `/K` may be one object or an array of them (Table 323), and a producer picks whichever suits: a
// single-kid element is routinely written `/K 0` or `/K << /Type /MCR … >>` rather than `/K [0]`.
// Every other fixture in this package uses the array form, which is how the carry shipped a rewrite
// that turned the single-dictionary form into a dropped tag tree — `DereferenceArray` returns a
// wrong-type ERROR for a dict, the rewrite reported failure, and `NUp` fell through to `honest`.
// Green suite, whole tree gone, on the shape ISO 32000-1's own Example 2 writes.
func kShapeFixture(k1, k2 string) []byte {
	one := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (KSHAPEONE) Tj ET\nEMC\n"
	two := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (KSHAPETWO) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(one), one),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K " + k1 + " >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 13 0 R /K " + k2 + " >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 14 0 R /StructParents 1 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(two), two),
	})
}

// TestEveryKShapeSurvivesTheCarry — the carry reads `/K`'s shape, not one producer's habit.
//
// The bare-integer and single-dictionary spellings had no fixture anywhere in this package until a
// red-proof probe showed the arms that handle them were unreachable from any test. The
// single-dictionary case is not hypothetical: `structartifact_test.go:310-313` already writes
// `/K << /Type /MCR /Pg 3 0 R /MCID 0 /Stm 5 0 R >>`, so the repo held the shape and drove it nowhere
// near `NUp`.
func TestEveryKShapeSurvivesTheCarry(t *testing.T) {
	for _, c := range []struct {
		name   string
		k1, k2 string
		// wantMCR says the shape carries a rewritable integer; a dictionary kid is already a
		// reference and owns no integer to rewrite.
		wantMCR bool
	}{
		{"array of one integer", "[0]", "[0]", true},
		{"a bare integer", "0", "0", true},
		{"a single MCR dictionary", "<< /Type /MCR /Pg 3 0 R /MCID 0 >>", "<< /Type /MCR /Pg 13 0 R /MCID 0 >>", true},
		{"mixed — one bare, one array", "0", "[0]", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := kShapeFixture(c.k1, c.k2)
			if got := fate(src); got != "carried" {
				t.Fatalf("setup: the fixture itself reports %q, so a drop after NUp would prove nothing", got)
			}
			out, err := NUp(src, 2, false)
			if err != nil {
				t.Fatalf("NUp: %v", err)
			}
			if got := fate(out); got != "carried" {
				t.Fatalf("NUp over a document whose /K is %s reports %q — the whole structure tree was "+
					"dropped because the carry could not read that spelling of /K", c.name, got)
			}
			if d := completeness(t, out); len(d) > 0 {
				t.Errorf("the carry of a %s /K is not complete: %v", c.name, d)
			}
			tree, terr := readTree(t, out)
			if terr != nil {
				t.Fatalf("readTree: %v", terr)
			}
			mcrs, ints := 0, 0
			for _, e := range tree.elems {
				for _, k := range e.kids {
					switch k.kind {
					case kidMCR:
						mcrs++
						if k.stm == 0 {
							t.Errorf("an MCR kid names no stream after the carry")
						}
					case kidMCID:
						ints++
					}
				}
			}
			if ints != 0 {
				t.Errorf("%d kid(s) are still bare integers, claiming content in a sheet stream that "+
					"holds only the Do", ints)
			}
			if c.wantMCR && mcrs == 0 {
				t.Errorf("the carry produced no MCR kid from a %s /K", c.name)
			}
			// The text half, which is what a bare structural check cannot see.
			before, after := textsOf(t, src), textsOf(t, out)
			for i := range before {
				if i < len(after) && after[i] != before[i] {
					t.Errorf("element %d reads %q after the carry and %q before it", i, after[i], before[i])
				}
			}
		})
	}
}

// TestAnIntegerKidWithNoPlacementABANDONSTheCarry — the refusal the inheritance fix brought with it.
//
// `/Pg` is optional and inheritable (Table 323), and a bare integer kid claims its content is in the
// stream of *"the page that is specified in the Pg entry of the structure element dictionary"*
// (§14.7.4.2) — resolved up the ancestry when the element itself has none. An n-up moves that content
// into a form, so every such integer has to become an MCR naming the form. **An element whose whole
// ancestry names no page this n-up dismantled cannot be given one**, and shipping it would leave a
// tree half in one encoding and half in the other — a state no gate in this repo can see, because
// `structureCarriedCompletely` reads `/Pg` liveness, `/ParentTree` ownership and form draw counts and
// never a kid's encoding.
//
// So the carry is abandoned and `honest` drops the claim. The document is worse off by one tagging
// claim and better off by not making a false one, which is the trade this whole file is built on.
func TestAnIntegerKidWithNoPlacementABANDONSTheCarry(t *testing.T) {
	one := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (ANCHORED) Tj ET\nEMC\n"
	two := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (FLOATING) Tj ET\nEMC\n"
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(one), one),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9: "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		// **No `/Pg`, and its parent is the tree root, which has none either.** Its integer kid
		// therefore resolves against nothing the carry can repoint.
		10: "<< /Type /StructElem /S /P /P 7 0 R /K [0] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 14 0 R /StructParents 1 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(two), two),
	})

	// Stimulus, before the response is graded: at least one element IS repointable, or the carry
	// would be refused by the `repointed == 0` rule instead and this test would grade that.
	tree, terr := readTree(t, src)
	if terr != nil {
		t.Fatalf("readTree: %v", terr)
	}
	anchored, floating := 0, 0
	for _, e := range tree.elems {
		if e.pgObj != 0 {
			anchored++
			continue
		}
		for _, k := range e.kids {
			if k.kind == kidMCID {
				floating++
			}
		}
	}
	if anchored == 0 {
		t.Fatalf("setup: no element carries /Pg, so the carry refuses on `repointed == 0` and the " +
			"rule under test never runs")
	}
	if floating == 0 {
		t.Fatalf("setup: no element has an integer kid with no page in its ancestry, so the rule " +
			"under test has no subject")
	}

	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	if inspectTags(out).claims() {
		t.Error("the n-up shipped a tagging claim over a tree it could only half convert — the " +
			"unplaceable element's kid is still a bare integer, claiming content in a sheet stream " +
			"that holds only the Do")
	}
}

// TestAKidNamingAnUnreachedStreamFallsBackToThePage — the fallback, asserted in BOTH of its halves.
//
// A `/Stm` may name a stream the page walk never enters: an annotation's appearance stream, a form
// drawn nowhere, one nested past the depth ceiling, or a reference into a free xref slot after an
// incremental update. The narrow index has nothing for those, and the reader falls back to the
// page-wide index — which is exactly what it did before this slice existed, so the change cannot take
// text away from a document it was not written for.
//
// **The rect half is the one that needed writing.** A blind mutation pass — an adversary shown the
// production code and none of the tests — proposed taking the narrow rectangle *unconditionally*
// while leaving the narrow text conditional, and it survived every assertion in this file: the text
// fell back as designed and the box silently became the zero rect with `hasRect` false. Text is
// asserted far more often than geometry, which is precisely why that was the mutation nobody had
// covered.
func TestAKidNamingAnUnreachedStreamFallsBackToThePage(t *testing.T) {
	page := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (ON THE PAGE) Tj ET\nEMC\n"
	// Object 11 is a well-formed form XObject that NO page draws, so `readPageRuns` never enters it.
	orphan := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 0 0 Td (never drawn) Tj ET\nEMC\n"
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K " +
			"<< /Type /MCR /Pg 3 0 R /Stm 11 0 R /MCID 0 >> >>",
		9: "<< /Nums [0 [8 0 R]] >>",
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(orphan), orphan),
	})

	// Stimulus: the kid really does name a stream, and the page walk really does not reach it.
	tree, terr := readTree(t, pdf)
	if terr != nil {
		t.Fatalf("readTree: %v", terr)
	}
	named := 0
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if k.kind == kidMCR && k.stm > 0 {
				named++
			}
		}
	}
	if named != 1 {
		t.Fatalf("setup: %d kid(s) name a stream, want 1 — the fallback has no subject", named)
	}
	ctx, cerr := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if cerr != nil {
		t.Fatalf("read: %v", cerr)
	}
	pr, rerr := readPageRuns(ctx, 1)
	if rerr != nil {
		t.Fatalf("readPageRuns: %v", rerr)
	}
	for _, r := range pr.runs {
		if r.stm != 0 {
			t.Fatalf("setup: a run was read from stream %d, so the page walk DID reach the orphan "+
				"form and this fixture does not exercise the fallback", r.stm)
		}
	}

	v := viewOf(t, pdf)
	if len(v.elements) != 1 {
		t.Fatalf("the fixture yields %d element(s), want 1", len(v.elements))
	}
	e := v.elements[0]
	if e.text != "ON THE PAGE" {
		t.Errorf("the element reads %q; a kid naming a stream nothing reached must fall back to the "+
			"page index, which is what this reader has always done", e.text)
	}
	if !e.hasRect {
		t.Error("the element has no rect. The TEXT fell back to the page index and the BOX did not — " +
			"the two halves of one lookup disagreeing, which is the shape a text-only assertion " +
			"cannot see")
	}
	if e.hasRect && e.rect[2]-e.rect[0] <= 0 {
		t.Errorf("the element's rect %v is degenerate", e.rect)
	}
}
