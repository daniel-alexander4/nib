package pdfops

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The one merge door — `PLAN-ua-coverage.md` P02.S07a, ADR-048.
//
// # Why nib owns the loop instead of calling `api.MergeRaw`
//
// `MergeRaw` reads the first document, and for each later one runs `pdfcpu.MergeXRefTables`, which
// renumbers every source object IN PLACE, copies them into the destination, appends the page tree and
// then FREES the source catalog. The source's structure tree is still in the merged context after
// that — renumbered, and reachable from nothing — so the write leaves it out. A graft therefore has
// to happen between the merge and the write, and `MergeRaw` offers no place to stand there.
//
// What `MergeRaw` did, this does, in the same order and with the same configuration: `MERGECREATE`,
// relaxed validation, no bookmarks, the PDF 2.0 refusal, an optimize before the write.
//
// # What it fixes, measured before it was written
//
// A page's `/StructParents` is an INTEGER, so the merge's renumbering never touches it. Two tagged
// documents merged gave both pages `/StructParents 0`, and page 2 — document B's words — resolved to
// document A's element: a page asserting another document's text, `/ActualText` included. The census
// could not see it because it drove every merge with an untagged second document.
//
// # The rule: a merge only EXTENDS a claim the host already makes
//
// The host is the first document, which is the order the user chose. A later document's tree is
// grafted onto the host's only when the host has one; otherwise the later document's claims are
// stripped, so no page keeps a key that resolves into a tree that is not there — or into someone
// else's. Grafting a tagged document into an untagged one would make the result claim tagging over
// pages nobody tagged, which is ADR-031's worst case: a screen reader told a document is tagged stops
// reaching for the fallbacks it would otherwise use.

// graftSource is one later document's tree, prepared in its OWN context for the merge: its claims
// already offset past the host's keys, so the renumbering that follows has nothing left to collide.
type graftSource struct {
	root    types.Dict // the source's /StructTreeRoot — the same map the merge patches in place
	nextKey int        // the first key after the source's own, already offset
	lang    string     // the source catalog's /Lang, carried onto its elements when it differs
	tier    tagSource  // the source tree's recorded tier, or "" when it records none
}

// hostTree is the destination's structure tree, when it has one a graft can extend.
type hostTree struct {
	rootRef types.IndirectRef
	root    types.Dict
	pt      types.Dict
	nextKey int
	lang    string
}

// mergeDocs concatenates pdfs in order, grafting each later document's structure tree onto the first
// one's where it can. An incomplete graft — judged on the bytes written, as every carry is — falls back
// to the strip-only merge, which is the honest loss.
func mergeDocs(pdfs [][]byte) ([]byte, error) {
	out, grafted, err := mergeOnce(pdfs, true)
	if err != nil {
		return nil, err
	}
	if !grafted || carryIsComplete(out) {
		return out, nil
	}
	out, _, err = mergeOnce(pdfs, false)
	return out, err
}

func mergeOnce(pdfs [][]byte, graft bool) (out []byte, grafted bool, err error) {
	defer fault.Catch(&err)
	if len(pdfs) < 2 {
		return nil, false, fmt.Errorf("pdfops: a merge needs at least two documents")
	}
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.MERGECREATE
	conf.ValidationMode = model.ValidationRelaxed
	conf.CreateBookmarks = false

	dest, err := api.ReadAndValidate(bytes.NewReader(pdfs[0]), conf)
	if err != nil {
		return nil, false, err
	}
	dest.EnsureVersionForWriting()
	var host *hostTree
	if graft {
		host = readHostTree(dest)
	}
	want := dest.PageCount
	for i, b := range pdfs[1:] {
		src, err := api.ReadAndValidate(bytes.NewReader(b), dest.Configuration)
		if err != nil {
			return nil, false, err
		}
		if dest.XRefTable.Version() < model.V20 && src.XRefTable.Version() == model.V20 {
			return nil, false, pdfcpu.ErrUnsupportedVersion
		}
		want += src.PageCount
		var gs *graftSource
		if host != nil {
			gs = prepareGraft(src, host, dest.XRefTable)
		}
		if gs == nil {
			stripClaims(src)
		}
		if err := pdfcpu.MergeXRefTables(strconv.Itoa(i), src, dest, false, false); err != nil {
			return nil, false, err
		}
		// `MergeXRefTables` returns nil when appending the page tree fails (pdfcpu `merge.go`: `if err
		// != nil { return nil }`), so a lost document would otherwise arrive as a short success.
		if dest.PageCount != want {
			return nil, false, fmt.Errorf("pdfops: the merge produced %d pages where %d were expected", dest.PageCount, want)
		}
		if gs != nil {
			if err := attachGraft(dest, host, gs); err != nil {
				return nil, false, err
			}
			grafted = true
		}
	}
	// ADR-032: the first document's catalog carries its PDF/UA identification onto content it never
	// described, so it goes — inside this write, never as a second one.
	if _, err := dropUAIdentification(dest); err != nil {
		return nil, false, err
	}
	if conf.OptimizeBeforeWriting {
		if err := api.OptimizeContext(dest); err != nil {
			return nil, false, err
		}
	}
	var buf bytes.Buffer
	if err := api.WriteContext(dest, &buf); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), grafted, nil
}

// readHostTree returns the destination's tree when a graft can extend it: a structure tree root with
// a FLAT `/ParentTree`. A nested one is refused rather than rebalanced, the refusal `carryStructure`
// already makes (0 of 294 veraPDF corpus files nest one).
func readHostTree(ctx *model.Context) *hostTree {
	xt := ctx.XRefTable
	cat, err := xt.Catalog()
	if err != nil {
		return nil
	}
	ref, ok := cat["StructTreeRoot"].(types.IndirectRef)
	if !ok {
		return nil
	}
	root := derefDict(xt, ref)
	if root == nil {
		return nil
	}
	pt := derefDict(xt, root["ParentTree"])
	if pt == nil {
		return nil
	}
	if _, nested := pt["Kids"]; nested {
		return nil
	}
	return &hostTree{rootRef: ref, root: root, pt: pt, nextKey: nextParentTreeKey(xt, root, pt),
		lang: readLang(xt, cat["Lang"])}
}

// nextParentTreeKey is the first key a new claim may take: past both the declared
// `/ParentTreeNextKey` and every key actually present, because a producer's declaration can lag.
func nextParentTreeKey(xt *model.XRefTable, root, pt types.Dict) int {
	next := 0
	if n, ok := pdfNumber(xt, root["ParentTreeNextKey"]); ok && int(n) > next {
		next = int(n)
	}
	nums := derefArray(xt, pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		if k, ok := pdfNumber(xt, nums[i]); ok && int(k)+1 > next {
			next = int(k) + 1
		}
	}
	return next
}

// prepareGraft readies a source document's tree for grafting onto host, in the source's own context,
// or returns nil when it cannot be grafted: no tree, a nested `/ParentTree`, or a role or class name
// the host maps differently. Returning nil costs nothing — the caller strips the claims instead.
func prepareGraft(src *model.Context, host *hostTree, hostXT *model.XRefTable) *graftSource {
	xt := src.XRefTable
	cat, err := xt.Catalog()
	if err != nil {
		return nil
	}
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		return nil
	}
	pt := derefDict(xt, root["ParentTree"])
	if pt == nil {
		return nil
	}
	if _, nested := pt["Kids"]; nested {
		return nil
	}
	// A name the two documents map to different targets cannot be merged without rewriting one side's
	// elements, and a merge is not the place to decide which reading of a custom role is right.
	for _, k := range []string{"RoleMap", "ClassMap"} {
		if conflicts(derefDict(xt, root[k]), derefDict(hostXT, host.root[k])) {
			return nil
		}
	}
	offset := host.nextKey
	srcNext := nextParentTreeKey(xt, root, pt)
	eachParentTreeClaim(src, func(key int, set func(int)) { set(key + offset) })
	nums := derefArray(xt, pt["Nums"])
	shifted := make(types.Array, len(nums))
	copy(shifted, nums)
	for i := 0; i+1 < len(shifted); i += 2 {
		if k, ok := pdfNumber(xt, shifted[i]); ok {
			shifted[i] = types.Integer(int(k) + offset)
		}
	}
	pt["Nums"] = shifted
	tier := tagSource("")
	if s, ok := root[tagSourceKey].(types.Name); ok && tagSource(s).valid() {
		tier = tagSource(s)
	}
	return &graftSource{root: root, nextKey: srcNext + offset, lang: readLang(xt, cat["Lang"]), tier: tier}
}

// conflicts reports whether a source name map gives any name a different value than the host's does.
// Each map is read in its own context: until the merge joins them, the two documents' objects live in
// two cross-reference tables.
func conflicts(src, host types.Dict) bool {
	for name, v := range src {
		if hv, ok := host[name]; ok && hv.String() != v.String() {
			return true
		}
	}
	return false
}

// stripClaims deletes every structure key a document's pages, annotations and forms claim — for a
// document merged in whose tree is not carried, so that none of its pages resolves into a tree that is
// not there, or into the host's.
func stripClaims(ctx *model.Context) {
	eachParentTreeClaim(ctx, func(_ int, set func(int)) { set(-1) })
}

// attachGraft joins a prepared source tree to the host, after `MergeXRefTables` has renumbered it: the
// source root's kids become the host root's last kids, its `/ParentTree` rows join the host's, and its
// role and class names join the host's maps.
func attachGraft(ctx *model.Context, host *hostTree, gs *graftSource) error {
	xt := ctx.XRefTable
	kids, hostKids := rootKids(xt, gs.root), rootKids(xt, host.root)
	merged := append(append(types.Array{}, hostKids...), kids...)
	for _, k := range kids {
		elem := derefDict(xt, k)
		if elem == nil {
			continue
		}
		elem["P"] = host.rootRef
		// A grafted element inherits the HOST's language from the catalog unless it says its own, so
		// a document written in another language would be read aloud in the wrong one (PDF/UA 7.2).
		if gs.lang != "" && gs.lang != host.lang {
			if _, has := elem["Lang"]; !has {
				elem["Lang"] = types.StringLiteral(gs.lang)
			}
		}
	}
	host.root["K"] = merged

	hostNums := derefArray(xt, host.pt["Nums"])
	srcNums := derefArray(xt, derefDict(xt, gs.root["ParentTree"])["Nums"])
	host.pt["Nums"] = append(append(types.Array{}, hostNums...), srcNums...)
	host.nextKey = gs.nextKey
	host.root["ParentTreeNextKey"] = types.Integer(gs.nextKey)

	for _, k := range []string{"RoleMap", "ClassMap"} {
		src := derefDict(xt, gs.root[k])
		if len(src) == 0 {
			continue
		}
		dst := derefDict(xt, host.root[k])
		if dst == nil {
			dst = types.Dict{}
			host.root[k] = dst
		}
		for name, v := range src {
			dst[name] = v
		}
	}

	// The tier describes the whole tree, so it is the weaker of the two — and a tree partly built by
	// something that recorded no tier records none.
	hostTier, hostOK := host.root[tagSourceKey].(types.Name)
	switch {
	case !hostOK || !tagSource(hostTier).valid() || gs.tier == "":
		delete(host.root, tagSourceKey)
	default:
		host.root[tagSourceKey] = types.Name(lowerTier(tagSource(hostTier), gs.tier))
	}
	return nil
}

// rootKids is a structure root's `/K` as a list. `/K` may be an array, or ONE kid — an indirect
// element or a direct element dictionary — and reading only the array and the reference would drop a
// direct one from the merged root, taking the host's whole tree with it.
func rootKids(xt *model.XRefTable, root types.Dict) types.Array {
	k := root["K"]
	if k == nil {
		return nil
	}
	if arr := derefArray(xt, k); arr != nil {
		return arr
	}
	return types.Array{k}
}
