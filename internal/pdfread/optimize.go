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
	// maxPairDepth bounds `pairSteps`' recursion: three levels a compared pair (stream, /Resources, /XObject).
	maxPairDepth = 1 << 12
	// maxShapeDepth bounds `shape`'s recursion, as maxReferenceDepth bounds the reference door's: a form whose graph
	// reaches a chain `N 0 obj [N+1 0 R]` under a key pdfcpu's validator never follows passed the door, and `shape`
	// walked it to a fatal stack overflow at ~2,000,000 links (the phase-close review of PLAN-returned-document P02).
	maxShapeDepth = 1 << 13
)

// ReadOptimized is nib's one `ReadValidateAndOptimize`: it reads and validates through `Validated` (so the reference door
// runs before pdfcpu's validator can recurse), exactly as pdfcpu does, then optimizes only
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

// ReadForInspection is `ReadOptimized` without pdfcpu's per-page resource step, for a reader that NEVER writes the
// context back out — `/pending 754` (the carry gate, where it began) and `/pending 763`.
//
// The step (`optimizeResourceDicts`, optimize.go:1592, v0.13.0) calls `PageDict(i, true)` for every page, which on a
// flat page tree dereferences every kid before the one it wants: quadratic in pages, ~15 s of a 7,059-page document's
// prepare. It does two things. It prunes each page's `/Resources` to the names its content uses — a reader that
// looks names up from the content sees the same objects — and it puts inherited `/Resources` on the page, which a
// reader of a page's own dictionary does see, so that half is restored here from one `Pages` walk (the nearest
// ancestor's dictionary, which is what PDF's inheritance means; pdfcpu merges every ancestor's, and the difference
// is names a page cannot draw). The rest of the pass — the form and font merging a reader may depend on — runs.
//
// **Only for a context nothing writes.** A writer would ship the unpruned resources, and `ctx.Conf` keeps the step
// off for any later `PageDict(n, true)` on this context. Every caller is held against `ReadOptimized`'s answer by
// `pdfops`' `TestReadOnlySitesAgreeAcrossReadings`.
func ReadForInspection(pdf []byte) (*model.Context, error) {
	conf := model.NewDefaultConfiguration()
	conf.OptimizeResourceDicts = false
	ctx, err := ReadOptimized(pdf, conf)
	if err != nil {
		return nil, err
	}
	InheritResources(ctx)
	return ctx, nil
}

// InheritResources puts on every page that has no `/Resources` of its own the ones it inherits — the half of pdfcpu's
// per-page resource step a reader of a page's own dictionary depends on, for a context read with that step OFF
// (`ReadForInspection`, and the accessibility checker's read, `/pending 782`). The nearest ancestor's dictionary, which
// is what PDF's inheritance means; it never prunes, so every name the file binds is still bound.
func InheritResources(ctx *model.Context) {
	for _, pg := range Pages(ctx) {
		if pg.Err != nil || pg.Dict == nil || pg.Attrs == nil || len(pg.Attrs.Resources) == 0 {
			continue
		}
		if _, own := pg.Dict.Find("Resources"); !own {
			pg.Dict["Resources"] = pg.Attrs.Resources
		}
	}
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
	pages := pageDicts(ctx)
	for _, res := range pageResources(ctx, pages) {
		if !e.walk(res, nil) {
			return fmt.Sprintf("its resources reach form XObjects along more than %d paths, and the optimizer "+
				"walks every one", maxOptimizeWalk)
		}
	}
	units := e.compareUnits()
	if e.tooDeep {
		return fmt.Sprintf("its form XObjects reach a chain of objects more than %d deep, and the optimizer "+
			"compares every link", maxShapeDepth)
	}
	if units > maxOptimizeCompare {
		return fmt.Sprintf("comparing its form XObjects for duplicates would take more than %d steps "+
			"(forms of one length that differ deep inside their resources)", maxOptimizeCompare)
	}
	return contentUnaffordable(ctx, pages)
}

// optimizeEstimate is the state of one estimate.
type optimizeEstimate struct {
	ctx   *model.Context
	steps int
	forms map[int]*types.StreamDict // every form XObject the walk reached, by object number
	memo  map[int]objShape
	// depth is `shape`'s current recursion depth; tooDeep is set once it passed maxShapeDepth, and refuses.
	depth   int
	tooDeep bool
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
		members    map[int]uint64 // object number → shape hash
		cyclic     bool
	}
	groups := map[int64]*group{}
	for nr, sd := range e.forms {
		length := int64(-1)
		if sd.StreamLength != nil {
			length = *sd.StreamLength
		}
		g := groups[length]
		if g == nil {
			g = &group{shapes: map[uint64]bool{}, members: map[int]uint64{}}
			groups[length] = g
		}
		s := e.shape(*types.NewIndirectRef(nr, 0), map[int]bool{})
		g.n++
		g.shapes[s.hash] = true
		g.members[nr] = s.hash
		g.cyclic = g.cyclic || s.cyclic
		if s.size > g.largest {
			g.largest = s.size
		}
	}
	units := 0
	for _, g := range groups {
		units = satAdd(units, satMul(satMul(g.n-1, len(g.shapes)), g.largest))
	}
	// A group holding a cycle is charged by walking its comparisons, because its unfolded size says nothing about
	// them — `pairSteps`. As above, the cache can hold only structurally distinct forms apart, so every pair of
	// distinct shapes is charged and each further copy one comparison with the first of its shape.
	for _, g := range groups {
		if !g.cyclic || units > maxOptimizeCompare {
			continue
		}
		nrs := make([]int, 0, len(g.members))
		for nr := range g.members {
			nrs = append(nrs, nr)
		}
		sort.Ints(nrs)
		first := map[uint64]int{}
		var reps []int
		for _, nr := range nrs {
			if _, seen := first[g.members[nr]]; !seen {
				first[g.members[nr]] = nr
				reps = append(reps, nr)
			}
		}
		charge := func(a, b int) {
			left := maxOptimizeCompare - units + 1
			units += e.pairSteps(*types.NewIndirectRef(a, 0), *types.NewIndirectRef(b, 0), nil, &left, 0)
		}
		for i := 1; i < len(reps) && units <= maxOptimizeCompare; i++ {
			for j := 0; j < i && units <= maxOptimizeCompare; j++ {
				charge(reps[i], reps[j])
			}
		}
		for _, nr := range nrs {
			if rep := first[g.members[nr]]; rep != nr && units <= maxOptimizeCompare {
				charge(nr, rep)
			}
		}
	}
	return units
}

// pairSteps counts the steps pdfcpu's `model.EqualObjects` (v0.13.0, model/equal.go) takes comparing a and b, and
// stops counting once *left runs out — `/pending 715`. It mirrors the walk: equal references are equal at once, a
// pair of object numbers already on the path is equal at once (the ONLY cut on a cycle, which is why two cycles of
// different lengths are walked to their lcm), and otherwise dictionaries and arrays of one size descend. It errs
// towards cost: it does not stop at the first difference, because pdfcpu ranges a dictionary in map order and so
// may meet the difference last.
func (e *optimizeEstimate) pairSteps(a, b types.Object, pairs []int, left *int, depth int) int {
	if *left <= 0 {
		return 0
	}
	if depth > maxPairDepth {
		// pdfcpu recurses as deep as this walk does, and a walk this deep is a pair path no document needs: it is
		// charged everything left, so the pass is skipped rather than this walk (or pdfcpu's) running the stack out.
		all := *left
		*left = 0
		return all
	}
	*left--
	steps := 1
	ra, aRef := a.(types.IndirectRef)
	rb, bRef := b.(types.IndirectRef)
	if aRef && bRef {
		if ra == rb {
			return steps
		}
		x, y := ra.ObjectNumber.Value(), rb.ObjectNumber.Value()
		if x > y {
			x, y = y, x
		}
		for i := 0; i+1 < len(pairs); i += 2 {
			*left-- // pdfcpu's containsPair is a linear scan
			steps++
			if pairs[i] == x && pairs[i+1] == y {
				return steps
			}
		}
		pairs = append(pairs[:len(pairs):len(pairs)], x, y)
	}
	da, erra := e.ctx.Dereference(a)
	db, errb := e.ctx.Dereference(b)
	if erra != nil || errb != nil || da == nil || db == nil {
		return steps
	}
	switch va := da.(type) {
	case types.Dict:
		if vb, ok := db.(types.Dict); ok && len(va) == len(vb) {
			steps += e.pairDictSteps(va, vb, pairs, left, depth)
		}
	case types.StreamDict:
		if vb, ok := db.(types.StreamDict); ok && len(va.Dict) == len(vb.Dict) {
			steps += e.pairDictSteps(va.Dict, vb.Dict, pairs, left, depth)
		}
	case types.Array:
		if vb, ok := db.(types.Array); ok && len(va) == len(vb) {
			for i := range va {
				steps += e.pairSteps(va[i], vb[i], pairs, left, depth+1)
			}
		}
	}
	return steps
}

func (e *optimizeEstimate) pairDictSteps(a, b types.Dict, pairs []int, left *int, depth int) int {
	steps := 0
	for k, va := range a {
		if vb, ok := b[k]; ok {
			steps += e.pairSteps(va, vb, pairs, left, depth+1)
		}
	}
	return steps
}

// objShape is an object's structural hash — equal objects hash equal, so the number of distinct hashes
// bounds how many forms pdfcpu's cache can hold apart — and its size unfolded as a tree, which is what
// `EqualObjects` walks, since it memoises nothing across siblings.
type objShape struct {
	hash uint64
	size int
	// cyclic is set when the object's graph meets an object already on its own path — a size that stopped there.
	cyclic bool
}

// shape computes o's objShape, memoised by object number. An object met again on the current path hashes by
// its object number, so two cyclic graphs pdfcpu would call equal may count as distinct: the estimate grows,
// never shrinks.
func (e *optimizeEstimate) shape(o types.Object, onPath map[int]bool) objShape {
	if e.depth >= maxShapeDepth {
		// Cut, and counted as a cycle so the estimate only grows; Unaffordable refuses on tooDeep regardless.
		e.tooDeep = true
		return objShape{size: 1, cyclic: true}
	}
	e.depth++
	defer func() { e.depth-- }()
	h := fnv.New64a()
	switch v := o.(type) {
	case types.IndirectRef:
		nr := v.ObjectNumber.Value()
		if s, ok := e.memo[nr]; ok {
			return s
		}
		if onPath[nr] {
			// The size cut here is only the size of one pass round the cycle, which is NOT what a comparison
			// costs: the shape is marked cyclic, and `compareUnits` counts such a group's comparisons by walking
			// them (`pairSteps`, /pending 715).
			fmt.Fprintf(h, "cycle %d", nr)
			return objShape{hash: h.Sum64(), size: 1, cyclic: true}
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
		return objShape{hash: h.Sum64(), size: s.size, cyclic: s.cyclic}
	case types.Array:
		fmt.Fprintf(h, "array %d", len(v))
		size, cyclic := 1, false
		for _, el := range v {
			c := e.shape(el, onPath)
			fmt.Fprintf(h, " %x", c.hash)
			size = satAdd(size, c.size)
			cyclic = cyclic || c.cyclic
		}
		return objShape{hash: h.Sum64(), size: min(size, maxUnfold), cyclic: cyclic}
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
	size, cyclic := 1, false
	for _, k := range sortedKeys(d) {
		c := e.shape(d[k], onPath)
		fmt.Fprintf(h, " /%s %x", k, c.hash)
		size = satAdd(size, c.size)
		cyclic = cyclic || c.cyclic
	}
	return objShape{hash: h.Sum64(), size: min(size, maxUnfold), cyclic: cyclic}
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
func pageResources(ctx *model.Context, pages []types.Dict) []types.Dict {
	out := make([]types.Dict, 0, len(pages))
	for _, d := range pages {
		if res, e := ctx.DereferenceDict(d["Resources"]); e == nil && res != nil {
			out = append(out, res)
		}
	}
	return out
}

// pageDicts is every page's dictionary that resolves, in page order, gathered ONCE per estimate and handed to each
// reader of it (`pageResources`, `contentNamings`). pdfcpu's `PageDict` walks the page tree from the root at every
// call, so each pass over the pages is quadratic in their count: a second pass cost 11.2 s and 2.3 GiB on a
// 7,059-page document (/pending 748). And the one pass that remained was itself quadratic on a flat tree, 12 s of
// the same document, so it goes through `Pages`, which answers every page from one walk (/pending 753).
func pageDicts(ctx *model.Context) []types.Dict {
	out := make([]types.Dict, 0, ctx.PageCount)
	for _, pg := range Pages(ctx) {
		if pg.Err == nil && pg.Dict != nil {
			out = append(out, pg.Dict)
		}
	}
	return out
}
