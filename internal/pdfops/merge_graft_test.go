package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
