package pdfops

import (
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// carriedPageLabels re-expresses the document's `/PageLabels` over the pages a selection kept
// (/pending 554, ADR-046).
//
// # Why this is not the outline's carry with a different key
//
// `/pending 524` carried `/Outlines` and refuted the claim that `/PageLabels` is "the same shape".
// An outline destination names a page OBJECT, and `selectPages` rewrites the page tree in place, so
// the carry is a PRUNE with nothing to remap. A page label names a page INDEX — `/Nums` is a number
// tree keyed by the 0-based position of the page a range starts on — so nothing about it survives a
// permutation untouched. It is rebuilt here, from the source's answer for each page, against the
// output's positions.
//
// The failure mode is also different in kind, and it is the sharper one. A lost bookmark is an
// absence a reader can see. A stale label is a page that SAYS it is page 7 while it is page 3 — a
// wrong statement rather than a missing one, which is the class `pdfops.go`'s original drop comment
// was written about. So the rule is all-or-nothing per page: every kept page gets the label the
// source gave it, or the key is not written at all.
//
// # The label follows the PAGE, and that is the repo's own ruling one key over
//
// Two readings are available for a reorder. Either the labelling is POSITIONAL — front matter is
// whatever now sits at the front, so a reordered document renumbers itself i, ii, iii, 1, 2 — or
// the label belongs to the page and travels with it. The second is what `outlinecarry.go` already
// decided for the sibling key, in those words: *"the surviving order is the SOURCE's, not the new
// page order… re-sorting is refused because the outline is an authored HIERARCHY"*. A page label is
// authored in exactly the same way — nib's own `SetPageLabels` is a user typing "this is where the
// body starts" — so renumbering on a reorder would be this operation inventing a claim about the
// document that the user did not make.
//
// It follows that a reordered output can carry labels that are not ascending, and that two output
// pages can carry the same label if the selection named one source page twice. Both are true of the
// source's own answer for those pages, which is the only thing this carry is entitled to say.
//
// # There is no range ceiling, and the reason is measured rather than assumed
//
// A permutation shatters a contiguous range: 200 pages labelled `i…iii, 1…197` reversed need one
// `/Nums` entry per page. The obvious guard is to drop the key past some number of ranges — and it
// is refused, because the document it produces is strictly worse. A document with 200 correct labels
// is right; a document with none is a subset that silently dropped what its source said. The cost
// that would justify the guard is not there: a worst-case 200-page full reversal — 200 ranges, one
// per page — is measured at **+6,582 bytes of output, and no time cost whose SIGN is even stable
// across nine runs**, against a `Collect` that already costs 30-60 ms on that document and a route
// with no timeout on the way here (`cmd/nib/main.go:160`, `web/app.js:14337`). See
// `TestTheWorstCasePageLabelCarryIsAffordable`, which is that measurement rather than this sentence.
//
// # Runs are collapsed, so the common case stays the shape it was
//
// Consecutive output pages whose labels continue naturally — same style, same prefix, and the number
// one higher — become one range. An order-preserving subset of a two-range document therefore comes
// out as two ranges, not as one per page, and the shattering above is confined to the selections
// that actually shattered it.
func carriedPageLabels(xt *model.XRefTable, root types.Dict, keep []int) types.Object {
	src := derefDict(xt, root["PageLabels"])
	if src == nil {
		return nil
	}
	nums, ok := xt.DereferenceArray(src["Nums"])
	if ok != nil || len(nums) < 2 {
		return nil
	}
	ranges := readPageLabelRanges(xt, nums)
	if len(ranges) == 0 {
		return nil
	}

	out := types.Array{}
	var prev pageLabel
	have := false
	for j, srcPage := range keep {
		lab, labelled := labelAt(ranges, srcPage-1)
		if !labelled {
			// A page the source's own tree does not reach carries no label, and saying so needs an
			// entry: without one it would inherit the range above it in the OUTPUT, which is a
			// label this document never gave it. `/S` and `/P` both absent is the empty label
			// (§12.4.2), the same shape `SetPageLabels` writes for style "none".
			if have && prev.empty() {
				continue
			}
			out = append(out, types.Integer(j), types.Dict{"Type": types.Name("PageLabel")})
			prev, have = pageLabel{}, true
			continue
		}
		if have && prev.continuesInto(lab) {
			prev = lab
			continue
		}
		out = append(out, types.Integer(j), lab.dict())
		prev, have = lab, true
	}
	if len(out) == 0 {
		return nil
	}
	return types.Dict{"Nums": out}
}

// pageLabel is one page's resolved label: the style and prefix its range carries, and the number
// this particular page falls on within it.
type pageLabel struct {
	style  string
	prefix string
	number int
}

func (l pageLabel) empty() bool { return l.style == "" && l.prefix == "" }

// continuesInto is the collapse rule: the next page belongs to the same range if nothing about the
// label changed except the number, which advanced by exactly one.
func (l pageLabel) continuesInto(next pageLabel) bool {
	return l.style == next.style && l.prefix == next.prefix && next.number == l.number+1
}

func (l pageLabel) dict() types.Dict {
	d := types.Dict{"Type": types.Name("PageLabel")}
	if l.style != "" {
		d["S"] = types.Name(l.style)
	}
	if l.prefix != "" {
		// Carried as it was read. `SetPageLabels` escapes on the way IN because the prefix arrives
		// from client JSON; what is here has already been through that door or through whatever
		// producer wrote the file, and re-escaping an escaped literal is how a legitimate "(" turns
		// into a broken one.
		d["P"] = types.StringLiteral(l.prefix)
	}
	if l.number != 1 {
		d["St"] = types.Integer(l.number)
	}
	return d
}

// labelRange is one entry of the source's number tree, resolved.
type labelRange struct {
	start  int // 0-based page index the range begins on
	style  string
	prefix string
	first  int // the label number the range's own first page carries (/St, default 1)
}

// readPageLabelRanges reads `/Nums` into ascending ranges, skipping anything malformed.
//
// **Malformed entries are skipped rather than failing the carry**, which is the asymmetry this
// operation wants: a producer that wrote one unreadable range should not cost the document every
// label it got right, and the pages that range covered fall back to the range above them — which is
// exactly what a reader does with the same tree.
func readPageLabelRanges(xt *model.XRefTable, nums types.Array) []labelRange {
	var out []labelRange
	for i := 0; i+1 < len(nums); i += 2 {
		key, kerr := xt.Dereference(nums[i])
		if kerr != nil {
			continue
		}
		n, isInt := key.(types.Integer)
		if !isInt || n.Value() < 0 {
			continue
		}
		d := derefDict(xt, nums[i+1])
		if d == nil {
			continue
		}
		r := labelRange{start: n.Value(), style: nameVal(d, "S"), first: 1}
		if s := d.StringLiteralEntry("P"); s != nil {
			r.prefix = s.Value()
		} else if s := d.StringEntry("P"); s != nil {
			r.prefix = *s
		}
		if st := d.IntEntry("St"); st != nil && *st >= 1 {
			r.first = *st
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// labelAt resolves one source page index against the ranges: the last range starting at or before
// it, and the number that page falls on inside it. `labelled` is false for a page before the first
// range, which the source leaves unlabelled.
func labelAt(ranges []labelRange, page int) (pageLabel, bool) {
	idx := -1
	for i, r := range ranges {
		if r.start > page {
			break
		}
		idx = i
	}
	if idx < 0 {
		return pageLabel{}, false
	}
	r := ranges[idx]
	return pageLabel{style: r.style, prefix: r.prefix, number: r.first + (page - r.start)}, true
}
