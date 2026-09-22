package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `PLAN-ua-coverage.md` P02.S07a, ADR-048 — the one merge door grafts a later document's structure
// tree onto the host's, and only EXTENDS a claim the host already makes.

// pageClaims is each page's /StructParents, -1 where it has none.
func pageClaims(t *testing.T, pdf []byte) []int {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil {
			t.Fatal(err)
		}
		out[p-1] = -1
		if v, ok := pdfNumber(ctx.XRefTable, d["StructParents"]); ok {
			out[p-1] = int(v)
		}
	}
	return out
}

// structRoot reads the output's /StructTreeRoot, or nil.
func structRoot(t *testing.T, pdf []byte) (*model.XRefTable, types.Dict) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	return ctx.XRefTable, derefDict(ctx.XRefTable, cat["StructTreeRoot"])
}

// TestAMergeOfTaggedDocumentsGraftsEachTree — the defect the slice exists for. Before it, both pages
// of `Append(tagged, tagged)` carried `/StructParents 0` and page 2 resolved to document A's element.
func TestAMergeOfTaggedDocumentsGraftsEachTree(t *testing.T) {
	src := taggedFixture()
	_, _, _, one := carryOf(t, src)
	if one == 0 {
		t.Fatal("setup: the fixture has no elements, so a graft of it is indistinguishable from none")
	}
	// A host that does not DECLARE its next key: the graft must find it from the keys present, or its
	// first grafted claim lands on the host's last one.
	undeclared, err := writeMutated(src, func(ctx *model.Context) error {
		cat, err := ctx.XRefTable.Catalog()
		if err != nil {
			return err
		}
		delete(derefDict(ctx.XRefTable, cat["StructTreeRoot"]), "ParentTreeNextKey")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, root := structRoot(t, undeclared); root["ParentTreeNextKey"] != nil {
		t.Fatal("setup: the undeclared host still declares /ParentTreeNextKey")
	}
	for _, tc := range []struct {
		name  string
		merge func() ([]byte, error)
		n     int
	}{
		{"Append", func() ([]byte, error) { return Append(src, src) }, 2},
		{"Combine", func() ([]byte, error) { return Combine([][]byte{src, src, src}) }, 3},
		{"UndeclaredNextKey", func() ([]byte, error) { return Append(undeclared, src) }, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.merge()
			if err != nil {
				t.Fatal(err)
			}
			claims := pageClaims(t, out)
			seen := map[int]bool{}
			for p, k := range claims {
				if k < 0 || seen[k] {
					t.Errorf("page %d claims key %d; pages %v — every page needs a key of its own", p+1, k, claims)
				}
				seen[k] = true
			}
			verdict, defects, orphans, elems := carryOf(t, out)
			if verdict != "carried" {
				t.Errorf("fate %q, want carried", verdict)
			}
			if len(defects) != 0 {
				t.Errorf("%d completeness defects: %v", len(defects), defects)
			}
			if len(orphans) != 0 {
				t.Errorf("orphan page objects %v", orphans)
			}
			if elems != tc.n*one {
				t.Errorf("%d elements, want %d — one tree per document", elems, tc.n*one)
			}
			if !carryIsComplete(out) {
				t.Error("the output gate rejects the graft, so production would have fallen back to the strip")
			}
			// Every top-level element's parent is THE root. A grafted one left naming the second
			// document's own root walks a reader up into a tree the catalog no longer holds.
			ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			cat, _ := ctx.XRefTable.Catalog()
			rootRef, _ := cat["StructTreeRoot"].(types.IndirectRef)
			// The declared next key is past every key present — the one a later writer allocates from.
			root := derefDict(ctx.XRefTable, rootRef)
			next, _ := pdfNumber(ctx.XRefTable, root["ParentTreeNextKey"])
			nums := derefArray(ctx.XRefTable, derefDict(ctx.XRefTable, root["ParentTree"])["Nums"])
			for i := 0; i+1 < len(nums); i += 2 {
				if k, _ := pdfNumber(ctx.XRefTable, nums[i]); k >= next {
					t.Errorf("/ParentTreeNextKey is %v but key %v is present", next, k)
				}
			}
			for i, k := range derefArray(ctx.XRefTable, derefDict(ctx.XRefTable, rootRef)["K"]) {
				p, _ := derefDict(ctx.XRefTable, k)["P"].(types.IndirectRef)
				if p.ObjectNumber != rootRef.ObjectNumber {
					t.Errorf("top-level element %d's /P is %v, not the root %v", i, p, rootRef)
				}
			}
		})
	}
}

// TestAMergeIntoAnUntaggedHostClaimsNothing — extend-only. Grafting would make the result claim tagging
// over the host's pages, which nobody tagged; and the second document's keys, left alone, would index a
// tree that is not there.
func TestAMergeIntoAnUntaggedHostClaimsNothing(t *testing.T) {
	tagged := taggedFixture()
	if pc := pageClaims(t, tagged); pc[0] < 0 {
		t.Fatal("setup: the tagged fixture's page makes no claim, so a strip of it cannot be observed")
	}
	out, err := Append(untaggedFixture(), tagged)
	if err != nil {
		t.Fatal(err)
	}
	if ClaimsTagging(out) {
		t.Error("a merge onto an untagged host claims tagging over pages nobody tagged")
	}
	for p, k := range pageClaims(t, out) {
		if k >= 0 {
			t.Errorf("page %d keeps /StructParents %d with no tree to index", p+1, k)
		}
	}
}

// TestAGraftedDocumentKeepsItsOwnLanguage — a grafted element inherits the HOST catalog's /Lang unless
// it states its own, so the second document would otherwise be read aloud in the first one's language.
func TestAGraftedDocumentKeepsItsOwnLanguage(t *testing.T) {
	host, err := SetLang(taggedFixture(), "fr-FR")
	if err != nil {
		t.Fatal(err)
	}
	other := subsetFixture() // /Lang (en-GB)
	out, err := Append(host, other)
	if err != nil {
		t.Fatal(err)
	}
	xt, root := structRoot(t, out)
	kids := derefArray(xt, root["K"])
	if len(kids) < 2 {
		t.Fatalf("setup: the root has %d kid(s); the graft did not happen", len(kids))
	}
	if got := readLang(xt, derefDict(xt, kids[0])["Lang"]); got != "" {
		t.Errorf("the host's own element was stamped %q; only grafted elements change", got)
	}
	if got := readLang(xt, derefDict(xt, kids[len(kids)-1])["Lang"]); got != "en-GB" {
		t.Errorf("the grafted element's language is %q, want en-GB — it would be read as French", got)
	}
}

// TestARoleMapIsMergedAndAConflictingOneIsRefused — a custom role the second document maps survives the
// graft; one the two documents map to DIFFERENT standard types is not guessed at, and that document's
// claims are stripped instead.
func TestARoleMapIsMergedAndAConflictingOneIsRefused(t *testing.T) {
	other := subsetFixture() // /RoleMap << /Para /P >>
	merged, err := Append(taggedFixture(), other)
	if err != nil {
		t.Fatal(err)
	}
	if got := roleMapOf(t, merged)["Para"]; got != "P" {
		t.Errorf("the grafted document's /Para maps to %q after the merge, want P", got)
	}

	objs := subsetFixtureObjects()
	objs[38] = "<< /Para /H1 >>"
	host := assembleFixture(objs)
	out, err := Append(host, other)
	if err != nil {
		t.Fatal(err)
	}
	if got := roleMapOf(t, out)["Para"]; got != "H1" {
		t.Errorf("the host's /Para maps to %q, want its own H1 kept", got)
	}
	claims := pageClaims(t, out)
	for p := 4; p < 8; p++ {
		if claims[p] >= 0 {
			t.Errorf("page %d (the conflicting document's) keeps /StructParents %d; it was not grafted, so it must not index the host's tree", p+1, claims[p])
		}
	}
	if got := fate(out); got != "partial" {
		t.Errorf("fate %q, want partial: the host's pages are described and the second document's are not", got)
	}
}

// TestAGraftRecordsTheWeakerTier — the tier describes the whole tree, so a tree built partly by the
// autotagger is not `Exact` because its first half was.
func TestAGraftRecordsTheWeakerTier(t *testing.T) {
	withTier := func(tier tagSource) []byte {
		out, err := writeMutated(taggedFixture(), func(ctx *model.Context) error { return setTagSource(ctx, tier) })
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, tc := range []struct {
		host, other []byte
		want        string
	}{
		{withTier(sourceExact), withTier(sourceInferred), string(sourceInferred)},
		{withTier(sourceInferred), withTier(sourceExact), string(sourceInferred)},
		{withTier(sourceExact), taggedFixture(), ""}, // a tree partly unrecorded records nothing
	} {
		out, err := Append(tc.host, tc.other)
		if err != nil {
			t.Fatal(err)
		}
		_, root := structRoot(t, out)
		got, _ := root[tagSourceKey].(types.Name)
		if string(got) != tc.want {
			t.Errorf("tier %q, want %q", got, tc.want)
		}
	}
}

// TestAMergeOfTwoConformantDocumentsAddsNoUA1Clause — the acceptance clause veraPDF grades: the census
// document merged with itself fails nothing its input did not (5 t1 aside — ADR-032 drops the claim).
func TestAMergeOfTwoConformantDocumentsAddsNoUA1Clause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so a grafted merge's ua1 delta is UNCHECKED in this run")
	}
	src, err := LabelUA(labelReady(t, censusMarkdown()), true)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	merged, err := Append(src, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(merged); got != "carried" {
		t.Fatalf("setup: the merge's fate is %q; veraPDF would be grading a strip, not a graft", got)
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "src.pdf"), filepath.Join(dir, "merged.pdf")
	for p, d := range map[string][]byte{a: src, b: merged} {
		if err := os.WriteFile(p, d, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := ua1FailedClauses(t, vp, []string{a, b})
	base, out := got["src.pdf"], got["merged.pdf"]
	if base == nil || out == nil {
		t.Fatalf("veraPDF could not process one of the files: %v", got)
	}
	for c := range out {
		if !base[c] && c != "5 t1" {
			t.Errorf("the grafted merge fails %s, which its input does not", c)
		}
	}
}

// TestAnIncompleteGraftFallsBackToTheStrip — the output gate's stimulus. A source whose `/ParentTree`
// holds a row no page claims is grafted without complaint and then fails completeness condition 3 (a
// key with no live owner); the merge must not ship that, and must not ship the second document's keys
// into a tree that does not describe them either.
func TestAnIncompleteGraftFallsBackToTheStrip(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[9] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 10 0 R >>"
	other := assembleFixture(objs)
	if _, defects, _, _ := carryOf(t, other); len(defects) == 0 {
		t.Fatal("setup: the source is complete, so its graft would be too and the gate would have nothing to refuse")
	}
	out, err := Append(taggedFixture(), other)
	if err != nil {
		t.Fatal(err)
	}
	if _, defects, _, _ := carryOf(t, out); len(defects) != 0 {
		t.Errorf("the merge shipped an incomplete graft: %v", defects)
	}
	claims := pageClaims(t, out)
	for p := 1; p < len(claims); p++ {
		if claims[p] >= 0 {
			t.Errorf("page %d keeps /StructParents %d after the fallback; the strip should have taken it", p+1, claims[p])
		}
	}
	if claims[0] < 0 {
		t.Error("the host's own page lost its claim; the fallback strips the SOURCE, not the host")
	}
}

// TestAHostWhoseRootHoldsOneDIRECTElementKeepsIt — `/K` may be a single element dictionary rather than
// an array or a reference, and a graft that read only those two shapes replaced the host's tree with
// the second document's.
func TestAHostWhoseRootHoldsOneDIRECTElementKeepsIt(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[30] = "<< /Type /StructTreeRoot /K << /Type /StructElem /S /Document /K [32 0 R 33 0 R 34 0 R 35 0 R 36 0 R 37 0 R] >> " +
		"/ParentTree 39 0 R /RoleMap 38 0 R /ParentTreeNextKey 6 >>"
	host := assembleFixture(objs)
	_, root := structRoot(t, host)
	if _, direct := root["K"].(types.Dict); !direct {
		t.Fatalf("setup: the host root's /K is %T, not a direct element — the shape is gone", root["K"])
	}
	_, _, _, hostElems := carryOf(t, host)
	_, _, _, otherElems := carryOf(t, taggedFixture())
	out, err := Append(host, taggedFixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, elems := carryOf(t, out); elems != hostElems+otherElems {
		t.Errorf("%d elements after the graft, want %d + %d — the host's direct root element was lost", elems, hostElems, otherElems)
	}
}

// TestAGraftStartsPastAKeyTheHostClaimsWithoutARow — a claim spends its key whether or not the
// `/ParentTree` has a row for it, and nothing requires one (a note with a `/StructParent` and no element
// is a claim nobody answers, not a defect). The graft's offset read only the rows and the declared next
// key, so the grafted page took the key the host's note already held and the graft was refused.
func TestAGraftStartsPastAKeyTheHostClaimsWithoutARow(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[9] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> /Contents 10 0 R /StructParents 3 /Annots [24 0 R] >>"
	objs[24] = "<< /Type /Annot /Subtype /Text /Rect [72 72 92 92] /P 9 0 R /StructParent 6 /Contents (note) >>"
	host := assembleFixture(objs) // rows 0-5, /ParentTreeNextKey 6, and a note claiming 6
	_, defects, _, hostElems := carryOf(t, host)
	if len(defects) != 0 {
		t.Fatalf("setup: the host is incomplete (%v), so a refused graft could be its fault", defects)
	}
	src := taggedFixture()
	_, _, _, srcElems := carryOf(t, src)

	out, err := Append(host, src)
	if err != nil {
		t.Fatal(err)
	}
	_, defects, _, elems := carryOf(t, out)
	if len(defects) != 0 {
		t.Errorf("the merge shipped an incomplete tree: %v", defects)
	}
	if elems != hostElems+srcElems {
		t.Errorf("%d elements, want %d + %d — the second document's graft was refused", elems, hostElems, srcElems)
	}
	if got := pageClaims(t, out)[4]; got < 7 {
		t.Errorf("the grafted page claims key %d; the host's note holds 6, so the first free key is 7", got)
	}
}

// TestAClassMapEntryIsComparedByWhatItNames — the two documents' `/ClassMap`s are read before the merge,
// in two cross-reference tables, so `50 0 R` in each is two unrelated objects. Compared by spelling, a
// class naming DIFFERENT attributes passed as a match and the graft then overwrote the host's entry with
// the source's, restyling every host element of that class.
func TestAClassMapEntryIsComparedByWhatItNames(t *testing.T) {
	hostObjs := subsetFixtureObjects()
	hostObjs[30] = "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R /RoleMap 38 0 R /ClassMap << /C1 50 0 R >> /ParentTreeNextKey 6 >>"
	hostObjs[50] = "<< /O /Layout /Placement /Block >>"
	host := assembleFixture(hostObjs)
	source := func(classObj int, placement string) []byte {
		content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (Tagged heading) Tj ET\nEMC\n"
		return assembleFixture(map[int]string{
			1:        "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
			2:        "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:        "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
			4:        streamObj(content),
			5:        "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
			7:        fmt.Sprintf("<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 /ClassMap << /C1 %d 0 R >> >>", classObj),
			8:        "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] /C /C1 >>",
			9:        "<< /Nums [0 [8 0 R]] >>",
			classObj: "<< /O /Layout /Placement /" + placement + " >>",
		})
	}
	hostPlacement := func(t *testing.T, pdf []byte) string {
		t.Helper()
		xt, root := structRoot(t, pdf)
		attrs := derefDict(xt, derefDict(xt, root["ClassMap"])["C1"])
		if attrs == nil {
			t.Fatal("the merged tree has no /C1 class")
		}
		return nameVal(attrs, "Placement")
	}

	// The SAME spelling naming a different dictionary: a conflict, so the source is not grafted.
	out, err := Append(host, source(50, "Inline"))
	if err != nil {
		t.Fatal(err)
	}
	if got := hostPlacement(t, out); got != "Block" {
		t.Errorf("the host's /C1 places %q after the merge, want its own Block", got)
	}
	if got := pageClaims(t, out)[4]; got >= 0 {
		t.Errorf("the conflicting document's page keeps claim %d; its class disagrees with the host's, so it must not be grafted", got)
	}

	// A DIFFERENT spelling naming an equal dictionary: the same class, so the graft goes ahead.
	out, err = Append(host, source(60, "Block"))
	if err != nil {
		t.Fatal(err)
	}
	if got := hostPlacement(t, out); got != "Block" {
		t.Errorf("the host's /C1 places %q, want Block", got)
	}
	if got := pageClaims(t, out)[4]; got < 0 {
		t.Error("a document whose /C1 names the same attributes as the host's was refused the graft")
	}
}

// TestAClassMapComparisonVisitsEachPairOnce — both documents in a merge may be untrusted, and an
// attribute graph whose every level holds two references to the next is 2^depth paths over a few dozen
// objects. Compared path by path, forty levels is a trillion comparisons and the merge hangs; compared
// pair by pair it is forty.
func TestAClassMapComparisonVisitsEachPairOnce(t *testing.T) {
	const levels = 40
	chain := func(objs map[int]string, first int) {
		for i := 0; i < levels; i++ {
			objs[first+i] = fmt.Sprintf("<< /L %d 0 R /R %d 0 R >>", first+i+1, first+i+1)
		}
		objs[first+levels] = "<< /O /Layout /Placement /Block >>"
	}
	hostObjs := subsetFixtureObjects()
	hostObjs[30] = "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R /RoleMap 38 0 R /ClassMap << /C1 50 0 R >> /ParentTreeNextKey 6 >>"
	chain(hostObjs, 50)
	host := assembleFixture(hostObjs)
	srcObjs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: streamObj("/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (Tagged heading) Tj ET\nEMC\n"),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 /ClassMap << /C1 100 0 R >> >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] /C /C1 >>",
		9: "<< /Nums [0 [8 0 R]] >>",
	}
	chain(srcObjs, 100)
	src := assembleFixture(srcObjs)

	done := make(chan error, 1)
	var out []byte
	go func() {
		var err error
		out, err = Append(host, src)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("merging two documents whose classes share a doubled attribute graph did not finish in 20 s")
	}
	// The stimulus is real only if the comparison ran to the bottom and found the graphs equal: a graft
	// refused on the first level would also be fast.
	if got := pageClaims(t, out)[4]; got < 0 {
		t.Error("the second document was refused the graft; its /C1 is the same graph as the host's")
	}
}
