package pdfops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// referenceContentDigest is ContentDigest's body EXACTLY as it stood before /pending 488 —
// `ctx.PageDict(i, false)` per page, and no stream memo.
//
// # Why a reference implementation and not only the golden corpus
//
// The goldens pin real files, and every file this machine can reach is small: veraPDF's corpus is
// hand-built conformance fixtures. **Neither of the two costs removed by that item is visible at
// that size, and neither is the risk.** The one-pass page-tree walk can only disagree with
// `PageDict` on a tree with interior nodes, and the memo can only be wrong where an object is
// reached from more than one page. A 2-page fixture exercises neither.
//
// This function closes that: it is the old algorithm, compiled into the new build, so the two can
// be run over the SAME `model.Context` and compared on a document of any size. It is a test of
// EQUIVALENCE rather than of a pinned value, so it holds for documents whose bytes change every
// run — which, per `digestUnstable`, is most of the ones with annotations in them.
//
// Keeping a second copy of the walk would normally be exactly what ADR-009 forbids. It is allowed
// here because it is not a second CALLER of the rule: nothing in production reaches it, and its
// whole purpose is to disagree if the real one changes behaviour.
func referenceContentDigest(pdf []byte) (string, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return "", err
	}
	return referenceDigestBody(ctx)
}

func referenceDigestBody(ctx *model.Context) (string, error) {
	h := sha256.New()
	hashChunk(h, []byte("nib-content-digest"))
	hashUint(h, ContentDigestVersion)
	hashUint(h, uint64(ctx.PageCount))
	for i := 1; i <= ctx.PageCount; i++ {
		d, _, _, err := ctx.PageDict(i, false)
		if err != nil || d == nil {
			return "", fmt.Errorf("page %d is unreadable: %w", i, err)
		}
		c, err := ctx.PageContent(d, i)
		if err != nil && err != model.ErrNoContent {
			return "", fmt.Errorf("page %d content: %w", i, err)
		}
		hashChunk(h, c)
		for _, key := range []string{"MediaBox", "CropBox", "Rotate"} {
			hashChunk(h, []byte(key))
			// nil memo: every stream decoded afresh, which is what the old code did.
			hashObject(ctx.XRefTable, d[key], h, 0, nil)
		}
		hashPageResources(ctx.XRefTable, d, h, nil)
		hashChunk(h, []byte("Annots"))
		hashObject(ctx.XRefTable, d["Annots"], h, 0, nil)
	}
	if err := hashEmbeddedFiles(ctx, h); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func reportDigest(pdf []byte) string {
	d, err := ContentDigest(pdf)
	if err != nil {
		return flattenRowValue("ERROR:" + err.Error())
	}
	return d
}

func reportReference(pdf []byte) string {
	d, err := referenceContentDigest(pdf)
	if err != nil {
		return flattenRowValue("ERROR:" + err.Error())
	}
	return d
}

// TestTheFastPageWalkAndTheStreamMemoAgreeWithTheOldAlgorithm — the acceptance for /pending 488's
// two implemented terms, on documents the golden corpus cannot reach.
//
// A disagreement here is a `ContentDigestVersion` bump wearing a performance change's clothes:
// ADR-013 records that a digest moved by a point release is reported to the user as *"these are
// not the same document"*.
func TestTheFastPageWalkAndTheStreamMemoAgreeWithTheOldAlgorithm(t *testing.T) {
	for _, d := range generatedDigestCorpus(t) {
		if got, want := reportDigest(d.PDF), reportReference(d.PDF); got != want {
			t.Errorf("%s: the one-pass walk plus memo gives %s and the per-page walk gives %s",
				d.Name, got, want)
		}
	}

	// A MULTI-PAGE document with an embedded font shared by every page and a real interior page
	// tree — the only shape where either change can be wrong. 120 clauses is ~15 pages, which is
	// enough for the shared font to be reached many times and cheap enough for the ordinary suite.
	big, err := tagMarkdown(bigMarkdown(120), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	pages, err := PageCount(big)
	if err != nil {
		t.Fatal(err)
	}
	if pages < 5 {
		t.Fatalf("setup: the shared-resource fixture is only %d pages, which cannot exercise a "+
			"memo that spans pages", pages)
	}
	if got, want := reportDigest(big), reportReference(big); got != want {
		t.Errorf("on a %d-page document with a shared embedded font the one-pass walk plus memo "+
			"gives %s and the per-page walk gives %s", pages, got, want)
	}
}

// TestASharedFontIsDecodedTwiceAndNotOncePerPage — the SPEED half, asserted as a counter.
//
// **A clock cannot assert this and must not be asked to.** The cost removed is O(pages) decodes of
// the same embedded font; a timing test for it fails when the machine is busy and passes when it
// is quiet, which is the opposite of what a regression test is for. The counter does not move with
// the load.
//
// # The predicate is TOTAL decodes, and "per page" was the wrong one
//
// The first cut asserted that decodes PER PAGE do not grow with the document, and it was vacuous —
// found by trying to red-prove it. Without the memo each page decodes its own resources once per
// resource name, which is a constant per page: 2.0 at 15 pages and 2.0 at 60. The defect is a
// MULTIPLIER on a constant, so a ratio of per-page rates cannot see it, and the test would have
// gone green against the exact regression it was written for.
//
// What the memo actually buys is that the shared objects are decoded a fixed number of times for
// the whole document rather than once per page. So the honest assertion is on the TOTAL: quadruple
// the pages of a document whose pages share one set of embedded faces, and the total must stay put.
// Measured: 8 decodes at 15 pages and 8 at 60 with the memo; 30 and 120 without it.
func TestASharedFontIsDecodedTwiceAndNotOncePerPage(t *testing.T) {
	small, err := tagMarkdown(bigMarkdown(120), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	large, err := tagMarkdown(bigMarkdown(480), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	_, ss, err := contentDigest(small)
	if err != nil {
		t.Fatal(err)
	}
	_, ls, err := contentDigest(large)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := PageCount(small)
	if err != nil {
		t.Fatal(err)
	}
	lp, err := PageCount(large)
	if err != nil {
		t.Fatal(err)
	}
	if sp < 3 || lp < 2*sp {
		t.Fatalf("setup: %d pages and %d pages do not straddle a doubling, so this measures nothing",
			sp, lp)
	}
	if !ss.fastPath || !ls.fastPath {
		t.Errorf("the one-pass page walk was not taken on a document pdfcpu itself wrote "+
			"(small=%v large=%v); digestPageDicts fell back, so every page costs a walk from the "+
			"page-tree root again", ss.fastPath, ls.fastPath)
	}
	t.Logf("%d pages: %d decodes (%.2f/page); %d pages: %d decodes (%.2f/page)",
		sp, ss.decodes, float64(ss.decodes)/float64(sp),
		lp, ls.decodes, float64(ls.decodes)/float64(lp))
	if ss.decodes == 0 {
		t.Fatal("setup: the small document decoded no streams at all, so the comparison below is " +
			"between two zeroes")
	}
	// The pages quadrupled and share one set of embedded faces, so the total must not. Doubling is
	// the ceiling: it is loose enough that a document needing one more face at the larger size
	// passes, and far under the 4x the defect produces.
	if ls.decodes > 2*ss.decodes {
		t.Errorf("%d pages cost %d decodes and %d pages cost %d — the total is scaling with the "+
			"page count on a document whose pages share their fonts, which is the once-per-page "+
			"re-decode the stream memo exists to remove", sp, ss.decodes, lp, ls.decodes)
	}
}

// TestSkippingPdfcpusOptimizeWOULDMoveTheDigest — why term 1 of /pending 488 was refused.
//
// The item proposed dropping `Optimize` from this read, noting the cost and saying *"measure
// whether the digest changes"*. It does, and the mechanism is one line of pdfcpu:
// `optimizeResourceDicts` (pkg/pdfcpu/optimize.go:1602) assigns `d["Resources"] =
// inhPAttrs.Resources` — the CONSOLIDATED resources, which are only the ones the page's content
// stream actually requires. A page carrying a font it never draws with therefore presents a
// different `/Font` dict to `hashPageResources` depending on whether Optimize ran, and
// `ContentDigest` hashes the resource NAMES.
//
// **295 of 295 corpus files were identical with and without it**, which is exactly why this test
// is a hand-built document rather than a corpus sweep: the shape that separates them is a page
// with an unused resource, and conformance fixtures do not have one. A corpus that cannot reach
// the defect is not evidence that the defect is absent.
//
// This asserts the REASON, so if pdfcpu ever stops consolidating, this goes red and the decision
// can be retaken deliberately instead of the digest moving under a point release (ADR-013).
func TestSkippingPdfcpusOptimizeWOULDMoveTheDigest(t *testing.T) {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (Tagged heading) Tj ET\nEMC\n"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		// /F2 sits in the page's /Resources and the content stream never names it.
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font " +
			"<< /F1 5 0 R /F2 6 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
	}
	pdf := assembleFixture(objs)

	withOptimize, err := ContentDigest(pdf)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateContext(ctx); err != nil {
		t.Fatal(err)
	}
	withoutOptimize, err := digestReadContext(ctx, &digestStats{})
	if err != nil {
		t.Fatal(err)
	}

	if withOptimize == withoutOptimize {
		t.Errorf("a page carrying an unused /F2 hashes to %s whether or not pdfcpu's Optimize "+
			"ran. That is the premise term 1 of /pending 488 was REFUSED on — if it no longer "+
			"holds, dropping Optimize from ContentDigest's read is back on the table and should "+
			"be measured again rather than left refused by a comment", short16(withOptimize))
	}
}

// TestTheDigestIsUnchangedByTheMemosBudget — the memo stops storing past its ceiling, and stopping
// must cost speed and never bytes.
//
// `streamMemoBudget` is a real branch and the branch nobody exercises: every document in every
// corpus here is far below 64 MiB of decoded streams, so the "stop storing" arm never runs in the
// suite. A budget that has never been crossed is an untested code path guarding the value the
// whole file is about.
func TestTheDigestIsUnchangedByTheMemosBudget(t *testing.T) {
	pdf, err := tagMarkdown(bigMarkdown(120), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	want, err := ContentDigest(pdf)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	// A memo that can never store anything — the state a document past the budget reaches on its
	// first oversized stream.
	st := &digestStats{}
	starved := newStreamMemo(st)
	starved.spent = streamMemoBudget
	got, err := digestWithMemo(ctx, starved, st)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("with the memo full the digest is %s and with it working it is %s — exceeding "+
			"streamMemoBudget changes the VALUE and not just the cost", short16(got), short16(want))
	}
	if st.decodes <= 0 {
		t.Error("the starved memo recorded no decodes, so this measured nothing")
	}
}

func short16(s string) string {
	if len(s) > 16 {
		return s[:16]
	}
	return s
}

// TestEveryExternalCorpusDocumentAgreesWithTheReferenceWalk — equivalence over the real files too.
//
// The golden file pins the VALUE; this pins the two walks against each other, so a change that
// moved both together (a regenerated golden, say) still cannot hide.
func TestEveryExternalCorpusDocumentAgreesWithTheReferenceWalk(t *testing.T) {
	root := os.Getenv(externalCorpusEnv)
	if root == "" {
		root = defaultExternalCorpus()
	}
	if root == "" {
		t.Skipf("no external corpus root; set %s", externalCorpusEnv)
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("the external corpus is not on this machine (%s)", root)
	}
	var files []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	if len(files) == 0 {
		t.Skipf("no PDFs under %s", root)
	}
	fell := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, f)
		if got, want := reportDigest(b), reportReference(b); got != want {
			t.Errorf("%s: fast %s vs reference %s", filepath.ToSlash(rel), got, want)
		}
		if _, st, err := contentDigest(b); err == nil && !st.fastPath {
			fell++
			t.Logf("%s: fell back to the per-page walk (the two page-tree walks read it "+
				"differently) — correct, and recorded so a rise in this count is visible",
				filepath.ToSlash(rel))
		}
	}
	t.Logf("%d corpus documents agree with the reference walk; %d took the fallback", len(files), fell)
}
