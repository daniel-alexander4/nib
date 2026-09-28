package pdfread

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optimize pass's cost, bounded before it runs — `/pending 706`.
//
// Every read in nib that wants pdfcpu's consolidated context calls `ReadOptimized` — `pdfops`, the checker
// (`uacheck.open`, `/pending 714`) and mdpdf's packet — and every place that optimizes a context it built
// itself (`pdfops.mergeInto`) calls `Optimize`. Both ask
// `Unaffordable` first. **pdfcpu's optimize pass has no budget of its own** (v0.13.0: no
// `ResourceLimits` field reaches `optimize.go`), and two of its shapes are hostile:
//
//   - **The comparison.** `optimizeXObjectForm` compares each form XObject with every cached form of the
//     same `/Length` through a deep `model.EqualObjects`, which follows `/Resources` into the forms they
//     name and memoises nothing across siblings. Measured on `RemovePages` over a chain of forms of equal
//     length, each under its own `/Resources`: depth 25 (6.7 KB) 27 ms, 50 200 ms, 100 (19 KB) 1.5 s,
//     200 (36 KB) 12.9 s — cubic.
//   - **The resource walk.** `optimizeXObjectResourcesDict` stops a loop with a per-PATH visited list and
//     nothing else, so a resource graph with sharing is walked once per path. Measured on `OptimizeContext`
//     alone: a diamond chain (each level two forms sharing one `/Resources`) 2.6 ms at 10 levels, 23 ms at
//     14, 318 ms at 18 — doubling per level; twenty forms whose direct `/Resources` all name one shared
//     `/XObject` dictionary 2.0 s from 5.9 KB, and forty did not finish in four minutes.
//
// **Past either budget the pass is SKIPPED, never interrupted** — pdfcpu takes no context and a goroutine
// abandoned mid-pass keeps burning and mutating the context. What the pass buys is deduplication (equal
// fonts, images and forms written once) and nothing a reader sees, so skipping it degrades only the size of
// the file an operation writes: every page, form and resource is still there. The one caller whose whole
// PURPOSE is the pass — `Optimize`, `conf.Cmd == model.OPTIMIZE` — is refused instead, because a skipped
// pass there would report success for a file it never optimised.
//
// **The estimates are upper bounds**, so they err towards skipping: the walk is simulated with only the
// path's ancestors as visited (pdfcpu also marks earlier siblings, so it walks no more), every reached form
// is counted as a comparison candidate even where pdfcpu would find it a duplicate, and a comparison is
// charged the whole unfolded size of the larger graph where `EqualObjects` stops at the first difference.

const (
	// maxOptimizeWalk bounds the simulated resource walk, in pdfcpu's own units of work: one per resource
	// entry examined along one path, plus one per visited entry its linear `visited` scans. An ordinary
	// document examines each of its forms once per page that names it.
	maxOptimizeWalk = 1 << 22
	// maxOptimizeCompare bounds the comparison estimate: for every group of forms sharing a `/Length`, the
	// forms in it times its structurally distinct forms (the most the cache can hold) times the largest
	// unfolded graph among them. Identical copies of one form cost one comparison each, not one per pair.
	maxOptimizeCompare = 1 << 22
	// maxUnfold saturates one graph's unfolded size, so the estimate cannot overflow.
	maxUnfold = 1 << 24
)

// ReadOptimized is nib's one `ReadValidateAndOptimize`: it reads and validates through `Validated` (so a `/UseCMap`
// cycle is refused before pdfcpu's validator can recurse on it), exactly as pdfcpu does, then optimizes only
// when `Unaffordable` says the pass is bounded.
func ReadOptimized(pdf []byte, conf *model.Configuration) (*model.Context, error) {
	return readOptimized(pdf, conf, false)
}

// ReadOptimizedOrRefuse is `ReadOptimized` for a reader whose ANSWER depends on the pass having run, where a
// skipped pass would not degrade the output's size but change what is reported: past the budget it refuses.
//
// **The checker is that reader (`uacheck.open`, `/pending 714`).** Its rules were measured against the
// optimized reading — the pass fuses equal forms (`formTwins`) and puts a page's inherited `/Resources` on the
// page — and a run of its suite with the pass switched off (2026-09-28) failed four of its tests on moved
// verdicts (two equal forms drawn once each read as one form drawn twice: 7.20 t2 Fail; merged-font
// CannotChecks answered), though law 5 held over veraPDF's corpus. A report computed on the
// other reading would be a confident answer about a document the checker was not calibrated to read; an
// error says what happened.
func ReadOptimizedOrRefuse(pdf []byte, conf *model.Configuration) (*model.Context, error) {
	return readOptimized(pdf, conf, true)
}

func readOptimized(pdf []byte, conf *model.Configuration, strict bool) (*model.Context, error) {
	ctx, err := Validated(pdf, conf)
	if err != nil {
		return nil, err
	}
	if conf.Optimize || cmdAssumingOptimization(conf.Cmd) {
		if err := optimize(ctx, strict); err != nil {
			return nil, err
		}
	}
	// What `ReadValidateAndOptimize` does after the pass, whether or not the pass ran (api.go:237-240).
	if err := pdfcpu.CacheFormFonts(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// cmdAssumingOptimization is pdfcpu's unexported list of the commands that optimize whatever
// `conf.Optimize` says (pkg/api/api.go:205, v0.13.0), restated because `ReadOptimized` replaces the function
// that consults it.
func cmdAssumingOptimization(cmd model.CommandMode) bool {
	switch cmd {
	case model.OPTIMIZE, model.FILLFORMFIELDS, model.RESETFORMFIELDS, model.LISTIMAGES,
		model.UPDATEIMAGES, model.EXTRACTIMAGES, model.EXTRACTFONTS, model.REMOVESIGNATURES:
		return true
	}
	return false
}

// Optimize runs pdfcpu's optimize pass on ctx when it is affordable, skips it when it is not, and
// refuses when the pass is the operation itself (see the file comment).
func Optimize(ctx *model.Context) error {
	return optimize(ctx, false)
}

// OptimizeOrRefuse is Optimize for a caller whose pass is the POINT — a normalization that proves a document
// survives what comes after it — so an unaffordable pass is refused (ErrUnaffordable) rather than skipped. Skipping
// would admit the document into a later pdfcpu write that runs the pass unbounded (`mdpdf`'s merge).
func OptimizeOrRefuse(ctx *model.Context) error {
	return optimize(ctx, true)
}

// optimize is Optimize, refusing past the budget when strict (`ReadOptimizedOrRefuse`) as well as when the
// pass is the operation.
func optimize(ctx *model.Context, strict bool) error {
	if why := Unaffordable(ctx); why != "" {
		if ctx.Cmd == model.OPTIMIZE {
			return fmt.Errorf("nib will not optimize this document: %s", why)
		}
		if strict {
			return fmt.Errorf("%w: %s", ErrUnaffordable, why)
		}
		return nil
	}
	return api.OptimizeContext(ctx)
}

// ErrUnaffordable is `ReadOptimizedOrRefuse`'s refusal: the pass its reader depends on would exceed its budget.
var ErrUnaffordable = errors.New("nib cannot read this document the way its checks were measured, because " +
	"pdfcpu's optimize pass over it would not finish in reasonable time")

// Unaffordable says why pdfcpu's optimize pass over ctx would exceed a budget, or "" when it would not.
func Unaffordable(ctx *model.Context) string {
	e := optimizeEstimate{ctx: ctx, forms: map[int]*types.StreamDict{}, memo: map[int]objShape{}}
	for _, res := range pageResources(ctx) {
		if !e.walk(res, nil) {
			return fmt.Sprintf("its resources reach form XObjects along more than %d paths, and the optimizer "+
				"walks every one", maxOptimizeWalk)
		}
	}
	if units := e.compareUnits(); units > maxOptimizeCompare {
		return fmt.Sprintf("comparing its form XObjects for duplicates would take more than %d steps "+
			"(forms of one length that differ deep inside their resources)", maxOptimizeCompare)
	}
	return ""
}

// optimizeEstimate is the state of one estimate.
type optimizeEstimate struct {
	ctx   *model.Context
	steps int
	forms map[int]*types.StreamDict // every form XObject the walk reached, by object number
	memo  map[int]objShape
}

// walk mirrors pdfcpu's `optimizeResources` over one resource dictionary (optimize.go:866): indirect XObjects,
// and the `/SMask /G` forms of indirect ExtGStates, recursing through each form's `/Resources`. vis is
// pdfcpu's own visited list with pdfcpu's own growth — an entry is appended as the loop ENTERS it, so a
// later sibling's subtree sees the earlier siblings too, and a form's indirect `/Resources` is appended
// before its walk. The one difference is order: pdfcpu ranges over a Go map, so its count varies run to
// run, and this walks by sorted name. It reports false once more than `maxOptimizeWalk` entries are entered.
func (e *optimizeEstimate) walk(res types.Dict, vis []int) bool {
	if res == nil {
		return true
	}
	// seen is pdfcpu's `visited` (a linear scan), and it is charged what it costs: one unit per entry
	// examined plus one per visited entry scanned. Charging only the entries ENTERED let a dictionary of
	// two thousand forms, each naming it again, cost this estimate more than a minute on its own.
	seen := func(nr int) bool {
		e.steps += 1 + len(vis)
		for _, v := range vis {
			if v == nr {
				return true
			}
		}
		return false
	}
	if xobjs, err := e.ctx.DereferenceDict(res["XObject"]); err == nil {
		for _, name := range sortedKeys(xobjs) {
			ir, isRef := xobjs[name].(types.IndirectRef)
			if !isRef {
				continue
			}
			if seen(ir.ObjectNumber.Value()) {
				if e.steps > maxOptimizeWalk {
					return false
				}
				continue
			}
			if e.steps > maxOptimizeWalk {
				return false
			}
			vis = append(vis, ir.ObjectNumber.Value())
			if !e.form(ir, vis) {
				return false
			}
		}
	}
	if gss, err := e.ctx.DereferenceDict(res["ExtGState"]); err == nil {
		for _, name := range sortedKeys(gss) {
			ir, isRef := gss[name].(types.IndirectRef)
			if !isRef {
				continue
			}
			if seen(ir.ObjectNumber.Value()) {
				if e.steps > maxOptimizeWalk {
					return false
				}
				continue
			}
			if e.steps > maxOptimizeWalk {
				return false
			}
			vis = append(vis, ir.ObjectNumber.Value())
			gs, gerr := e.ctx.DereferenceDict(ir)
			if gerr != nil || gs == nil {
				continue
			}
			if sm, serr := e.ctx.DereferenceDict(gs["SMask"]); serr == nil && sm != nil {
				if g, gRef := sm["G"].(types.IndirectRef); gRef && !e.form(g, vis) {
					return false
				}
			}
		}
	}
	return true
}

// form records one reached form XObject and walks its resources, as pdfcpu's `optimizeForm` does.
func (e *optimizeEstimate) form(ir types.IndirectRef, vis []int) bool {
	sd, _, err := e.ctx.DereferenceStreamDict(ir)
	if err != nil || sd == nil {
		return true
	}
	if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
		return true
	}
	e.forms[ir.ObjectNumber.Value()] = sd
	r := sd.Dict["Resources"]
	if rr, isRef := r.(types.IndirectRef); isRef {
		rn := rr.ObjectNumber.Value()
		for _, v := range vis {
			if v == rn {
				return true
			}
		}
		vis = append(vis, rn)
	}
	inner, rerr := e.ctx.DereferenceDict(r)
	if rerr != nil {
		return true
	}
	return e.walk(inner, vis)
}

// compareUnits is the comparison estimate over every reached form (see `maxOptimizeCompare`).
func (e *optimizeEstimate) compareUnits() int {
	type group struct {
		n, largest int
		shapes     map[uint64]bool
	}
	groups := map[int64]*group{}
	for nr, sd := range e.forms {
		length := int64(-1)
		if sd.StreamLength != nil {
			length = *sd.StreamLength
		}
		g := groups[length]
		if g == nil {
			g = &group{shapes: map[uint64]bool{}}
			groups[length] = g
		}
		s := e.shape(*types.NewIndirectRef(nr, 0), map[int]bool{})
		g.n++
		g.shapes[s.hash] = true
		if s.size > g.largest {
			g.largest = s.size
		}
	}
	units := 0
	for _, g := range groups {
		units = satAdd(units, satMul(satMul(g.n-1, len(g.shapes)), g.largest))
	}
	return units
}

// objShape is an object's structural hash — equal objects hash equal, so the number of distinct hashes
// bounds how many forms pdfcpu's cache can hold apart — and its size unfolded as a tree, which is what
// `EqualObjects` walks, since it memoises nothing across siblings.
type objShape struct {
	hash uint64
	size int
}

// shape computes o's objShape, memoised by object number. An object met again on the current path hashes by
// its object number, so two cyclic graphs pdfcpu would call equal may count as distinct: the estimate grows,
// never shrinks.
func (e *optimizeEstimate) shape(o types.Object, onPath map[int]bool) objShape {
	h := fnv.New64a()
	switch v := o.(type) {
	case types.IndirectRef:
		nr := v.ObjectNumber.Value()
		if s, ok := e.memo[nr]; ok {
			return s
		}
		if onPath[nr] {
			fmt.Fprintf(h, "cycle %d", nr)
			return objShape{hash: h.Sum64(), size: 1}
		}
		onPath[nr] = true
		d, err := e.ctx.Dereference(v)
		var s objShape
		if err != nil || d == nil {
			fmt.Fprintf(h, "null")
			s = objShape{hash: h.Sum64(), size: 1}
		} else {
			s = e.shape(d, onPath)
		}
		delete(onPath, nr)
		e.memo[nr] = s
		return s
	case types.Dict:
		return e.shapeDict(h, "dict", v, onPath)
	case types.StreamDict:
		s := e.shapeDict(h, "stream", v.Dict, onPath)
		h.Reset()
		fmt.Fprintf(h, "%x", s.hash)
		h.Write(v.Raw)
		return objShape{hash: h.Sum64(), size: s.size}
	case types.Array:
		fmt.Fprintf(h, "array %d", len(v))
		size := 1
		for _, el := range v {
			c := e.shape(el, onPath)
			fmt.Fprintf(h, " %x", c.hash)
			size = satAdd(size, c.size)
		}
		return objShape{hash: h.Sum64(), size: min(size, maxUnfold)}
	case nil:
		fmt.Fprintf(h, "null")
	default:
		fmt.Fprintf(h, "%T %s", v, v.PDFString())
	}
	return objShape{hash: h.Sum64(), size: 1}
}

func (e *optimizeEstimate) shapeDict(h interface {
	Write([]byte) (int, error)
	Sum64() uint64
}, kind string, d types.Dict, onPath map[int]bool) objShape {
	fmt.Fprintf(h, "%s %d", kind, len(d))
	size := 1
	for _, k := range sortedKeys(d) {
		c := e.shape(d[k], onPath)
		fmt.Fprintf(h, " /%s %x", k, c.hash)
		size = satAdd(size, c.size)
	}
	return objShape{hash: h.Sum64(), size: min(size, maxUnfold)}
}

func sortedKeys(d types.Dict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// satAdd and satMul saturate at maxUnfold², far above either budget, so an estimate cannot wrap.
func satAdd(a, b int) int { return min(a+b, maxUnfold*maxUnfold) }

func satMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	if a > maxUnfold*maxUnfold/b {
		return maxUnfold * maxUnfold
	}
	return a * b
}

// pageResources is each page's own `/Resources`, uninherited, in page order — what `pdfops.scanPages` gives the
// estimate there, restated because this package sits below `pdfops`. pdfcpu's pass starts from the same
// dictionaries (optimize.go's `optimizeResources` over each page's `Resources`).
func pageResources(ctx *model.Context) []types.Dict {
	out := make([]types.Dict, 0, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		if res, e := ctx.DereferenceDict(d["Resources"]); e == nil && res != nil {
			out = append(out, res)
		}
	}
	return out
}
