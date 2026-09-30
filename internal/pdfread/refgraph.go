package pdfread

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The reference door — `/pending 764`, ADR-069.
//
// pdfcpu v0.13.0's validator recurses through a document's references, and on most chains it remembers nothing it
// has visited. Where it does not, three shapes are hostile, and `refuseUnboundedReferences` refuses each before the
// validator runs:
//
//   - **A loop** recurses until Go's stack limit. That is fatal, not a panic — no `recover()` catches it — so the
//     whole nib process dies with every open document's unsaved work. `/UseCMap` (`/pending 675`) was the first
//     loop found; an 802-byte tiling pattern whose `/Resources` names itself (`/pending 764`) the second.
//   - **Sharing** is walked once per PATH, not once per object. Measured: tiling patterns each naming the next twice,
//     depth 20, 4.8 KB — 2.8 s to validate, doubling per level; the open never returns and cannot be interrupted.
//   - **Depth** without a loop still grows the stack, one set of pdfcpu frames per level.
//
// The edge table below is pdfcpu's own recursion, traced from its source (call graph over `validate/`, every
// cycle's guard read; `memory/instruments/pending-763-764.md`). An edge is followed only where pdfcpu follows it
// WITHOUT a guard. Where pdfcpu marks an object before recursing through it — form and image XObjects
// (xObject.go:763), fonts (font.go:1112-1124), an ExtGState's `/SMask` and every other `validateStreamDictEntry`
// (object.go:1041), ICCBased streams (colorspace.go:183) — the walk stops, because pdfcpu handles a loop through
// that object and refusing it would refuse files pdfcpu reads. Outlines, the page tree, form-field `/Kids`, the
// structure tree and thread beads carry their own guards and are not followed at all.
//
// A loop is a repeated (object, role) on the current path: the role is how pdfcpu reached the object (an object
// named as a pattern is validated as a pattern whatever it is), and pdfcpu recurses on the role, not the shape.

// ErrReferenceCycle is the refusal of a document whose objects name each other in a loop pdfcpu's validator would
// follow without end.
var ErrReferenceCycle = errors.New("its objects name each other in a loop")

// ErrReferencePaths is the refusal of a document whose shared objects would take pdfcpu's validator through more
// paths than `maxValidatorPaths` allows.
var ErrReferencePaths = errors.New("its objects share one another so deeply that validating it would not finish")

// ErrReferenceDepth is the refusal of a chain of references deeper than `maxReferenceDepth`.
var ErrReferenceDepth = errors.New("its objects name one another in a chain too deep to validate")

const (
	// maxReferenceDepth bounds a path through the edges below. pdfcpu's own guarded walks stop at 100
	// (model/configuration.go:361); a reply thread of annotations (`/IRT`) is the longest legitimate chain here.
	maxReferenceDepth = 1 << 13
	// basePaths is the path budget every document gets, whatever its size: 2^18 paths measured at ~0.74 s of
	// validation on the shape that costs most per path (a tiling pattern).
	basePaths = 1 << 18
	// pathsPerObject grows the budget with the document, so a large legitimate one — every page walking its own
	// shared resources — is not refused for being large. Sharing that multiplies is what exceeds it.
	pathsPerObject = 16
	// saturate keeps a path count from overflowing.
	saturate = 1 << 50
)

// role is how pdfcpu reached an object, which decides which of its keys the validator recurses on.
type role uint8

const (
	roleResourced      role = iota // a page, form, Type 3 font, tiling pattern or property list: its `/Resources`, `/Annots`
	roleCMap                       // a CMap stream: `/UseCMap` (font.go:961 → :886)
	roleFunction                   // `/FunctionType 3`: `/Functions` (function.go:176 → :231)
	roleColorSpace                 // a colour-space array, by its family (colorspace.go:584-600)
	roleDeviceNAttrs               // a DeviceN attributes dict: `/Colorants`, `/Process` (colorspace.go:~430)
	roleDeviceNProcess             // `/ColorSpace` (colorspace.go:346)
	roleShading                    // a shading: `/ColorSpace`, `/Function`
	roleShadingPattern             // a shading pattern dict: `/Shading`, `/ExtGState`
	roleExtGState                  // `/TR`, `/TR2`, `/BG`, `/BG2`, `/UCR`, `/UCR2`, `/HT`, `/SMask` `/TR`
	roleSoftMask                   // an ExtGState's `/SMask` dict: `/TR` (its `/G` is a guarded form)
	roleHalftone                   // `/HalftoneType 5`: its colorant entries (extGState.go:457-464)
	roleImage                      // an image XObject: `/Mask` stream, `/Alternates` (xObject.go:363, :404)
	roleAltImage                   // an alternate image: `/Mask` only (isAlternate skips `/Alternates`)
	roleAction                     // `/Next`; GoToE `/T`; GoTo3DView `/TA`; Rendition `/R` (action.go)
	roleTarget                     // a GoToE target dict: `/T` (action.go:97)
	roleAnnot                      // markup `/IRT`; Link, Widget, Screen `/A`; Widget, Screen `/AA` (annotation.go)
	roleRendition                  // a selector rendition's `/R` (media.go:1038 → :942)
	roleMediaClip                  // a media clip section's `/D` (media.go:604 → :537)
	roleTree                       // a name or number tree node with `/Limits`: `/Kids` — pdfcpu caps its depth, so a loop is its own; sharing is counted
	numRoles
)

// markup is pdfcpu's list of the annotation subtypes it validates as markup, and so follows `/IRT` on
// (annotation.go:1640-1660, the `markup` column).
var markup = map[string]bool{
	"Text": true, "FreeText": true, "Line": true, "Polygon": true, "PolyLine": true, "Highlight": true,
	"Underline": true, "Squiggly": true, "StrikeOut": true, "Square": true, "Circle": true, "Stamp": true,
	"Caret": true, "Ink": true, "FileAttachment": true, "Sound": true, "Redact": true,
}

// halftoneColorants is what pdfcpu validates on a type 5 halftone (extGState.go:457-464).
var halftoneColorants = []string{"Default", "Gray", "Red", "Green", "Blue", "Cyan", "Magenta", "Yellow", "Black"}

// transferKeys are an ExtGState's function entries (extGState.go).
var transferKeys = []string{"TR", "TR2", "BG", "BG2", "UCR", "UCR2"}

type node struct {
	nr   int
	role role
}

type edge struct {
	to  node
	key string
}

// refGraph walks ctx's references along the edges pdfcpu's validator follows unguarded. state and count are
// indexed by `slot`.
type refGraph struct {
	ctx     *model.Context
	size    int
	state   []uint8 // 0 unvisited, 1 on the path, 2 done
	count   []uint64
	reached []bool // named by an edge the walk follows, so its paths are counted in whatever names it
}

// slot is n's index in state and count, or -1 for an object number outside the table (which names nothing).
func (g *refGraph) slot(n node) int {
	if n.nr < 0 || n.nr >= g.size {
		return -1
	}
	return n.nr*int(numRoles) + int(n.role)
}

// refuseUnboundedReferences refuses ctx when pdfcpu's validator would recurse on it without end, through more
// paths than `pathBudget`, or deeper than `maxReferenceDepth`.
func refuseUnboundedReferences(ctx *model.Context) error {
	paths, err := validatorPaths(ctx)
	if err != nil {
		return err
	}
	if budget := pathBudget(ctx); paths > budget {
		return fmt.Errorf("nib will not read this document: %w (%d paths through its patterns, functions, colour "+
			"spaces, actions or trees, against a budget of %d), and pdfcpu's validator walks every one of them with "+
			"no way to stop it", ErrReferencePaths, paths, budget)
	}
	return nil
}

// pathBudget is `basePaths` plus `pathsPerObject` for each object in ctx.
func pathBudget(ctx *model.Context) uint64 {
	return uint64(basePaths + pathsPerObject*len(ctx.Table))
}

// validatorPaths is the number of paths pdfcpu's validator takes along the edges below, or the loop or depth refusal
// the walk meets first. Every object is walked from its own shape, so no loop is missed; the paths are summed only
// over the objects nothing walked names, because an object an edge reaches is counted in the paths of whatever
// reaches it — summing it again would count a chain once per link, where pdfcpu walks it once. Each (object, role)
// is expanded once, so the walk is linear in the edges it follows.
func validatorPaths(ctx *model.Context) (uint64, error) {
	nrs := make([]int, 0, len(ctx.Table))
	for nr, e := range ctx.Table {
		if e != nil && !e.Free && e.Object != nil {
			nrs = append(nrs, nr)
		}
	}
	sort.Ints(nrs)
	size := 1
	if len(nrs) > 0 {
		size = nrs[len(nrs)-1] + 1
	}
	slots := size * int(numRoles)
	g := &refGraph{ctx: ctx, size: size, state: make([]uint8, slots), count: make([]uint64, slots), reached: make([]bool, slots)}
	var roots []int
	for _, nr := range nrs {
		for _, r := range rootRoles(ctx.Table[nr].Object) {
			if _, err := g.walk(node{nr, r}); err != nil {
				return 0, err
			}
			roots = append(roots, g.slot(node{nr, r}))
		}
	}
	var total uint64
	for _, i := range roots {
		if !g.reached[i] {
			total = min(total+g.count[i], saturate)
		}
	}
	return total, nil
}

// rootRoles are the roles an object is walked from, by its own shape. An object reached only through another
// (a GoToE target, a DeviceN attributes dict) is walked from whatever reaches it.
func rootRoles(o types.Object) []role {
	switch o := o.(type) {
	case types.Array:
		if len(o) > 1 {
			if n, ok := o[0].(types.Name); ok && isRecursiveColorSpace(n.Value()) {
				return []role{roleColorSpace}
			}
		}
		return nil
	case types.StreamDict:
		rs := []role{roleCMap}
		if nameOf(o.Dict, "Subtype") == "Image" {
			rs = append(rs, roleImage)
		}
		return append(rs, dictRoles(o.Dict)...)
	case types.Dict:
		return dictRoles(o)
	}
	return nil
}

func dictRoles(d types.Dict) []role {
	var rs []role
	if _, ok := d["Resources"]; ok {
		rs = append(rs, roleResourced)
	}
	if intOf(d, "FunctionType") == 3 {
		rs = append(rs, roleFunction)
	}
	if intOf(d, "HalftoneType") == 5 {
		rs = append(rs, roleHalftone)
	}
	if nameOf(d, "S") != "" {
		rs = append(rs, roleAction, roleRendition, roleMediaClip)
	}
	if nameOf(d, "Subtype") != "" {
		rs = append(rs, roleAnnot)
	}
	if _, ok := d["Limits"]; ok {
		rs = append(rs, roleTree)
	}
	return rs
}

func isRecursiveColorSpace(family string) bool {
	switch family {
	case model.IndexedCS, model.PatternCS, model.SeparationCS, model.DeviceNCS:
		return true
	}
	return false
}

type frame struct {
	n     node
	edges []edge
	next  int
	sum   uint64
}

// walk returns the number of paths pdfcpu's validator takes from n, walking iteratively so that the walk itself
// holds no Go stack in proportion to the document.
func (g *refGraph) walk(root node) (uint64, error) {
	if i := g.slot(root); g.state[i] == 2 {
		return g.count[i], nil
	}
	stack := []*frame{g.open(root)}
	for {
		f := stack[len(stack)-1]
		if f.next < len(f.edges) {
			e := f.edges[f.next]
			f.next++
			i := g.slot(e.to)
			if i < 0 {
				continue
			}
			g.reached[i] = true
			switch g.state[i] {
			case 1:
				if e.to.role == roleTree {
					continue // pdfcpu caps a tree's depth (nameTree.go:707, numberTree.go:173): a loop there is its own
				}
				return 0, g.loop(stack, e)
			case 2:
				f.sum = min(f.sum+g.count[i], saturate)
			default:
				if len(stack) >= maxReferenceDepth {
					return 0, fmt.Errorf("nib will not read this document: %w (more than %d levels from object %d), "+
						"and pdfcpu's validator would recurse through every level", ErrReferenceDepth, maxReferenceDepth, root.nr)
				}
				stack = append(stack, g.open(e.to))
			}
			continue
		}
		n := min(1+f.sum, saturate)
		i := g.slot(f.n)
		g.state[i], g.count[i] = 2, n
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			return n, nil
		}
		p := stack[len(stack)-1]
		p.sum = min(p.sum+n, saturate)
	}
}

func (g *refGraph) open(n node) *frame {
	g.state[g.slot(n)] = 1
	f := &frame{n: n}
	if e, ok := g.ctx.Table[n.nr]; ok && e != nil && !e.Free {
		g.expand(e.Object, n.role, "", func(to node, key string) { f.edges = append(f.edges, edge{to, key}) })
	}
	return f
}

// loop words the refusal of the edge e, which returns to an object on the path.
func (g *refGraph) loop(stack []*frame, e edge) error {
	i := 0
	for i < len(stack) && stack[i].n != e.to {
		i++
	}
	var objs, keys []string
	for _, f := range stack[i:] {
		objs = append(objs, fmt.Sprint(f.n.nr))
		keys = append(keys, f.edges[f.next-1].key)
	}
	return fmt.Errorf("nib will not read this document: %w (objects %s → %d, through %s), and pdfcpu's validator "+
		"would follow that loop until nib ran out of stack", ErrReferenceCycle, strings.Join(objs, " → "), e.to.nr,
		strings.Join(keys, ", "))
}

type emitFn func(to node, key string)

// follow is an edge to o in role r: to an object when o is a reference, otherwise through the direct value, which
// pdfcpu validates in place.
func (g *refGraph) follow(o types.Object, r role, key string, emit emitFn) {
	if ir, ok := o.(types.IndirectRef); ok {
		emit(node{ir.ObjectNumber.Value(), r}, key)
		return
	}
	g.expand(o, r, key, emit)
}

// deref is one step of dereference, for the containers pdfcpu reads through on the way to an edge (a
// `/Resources` dict, a `/Functions` array). They are not nodes: nothing recurses on them.
func (g *refGraph) deref(o types.Object) types.Object {
	if ir, ok := o.(types.IndirectRef); ok {
		if e, ok := g.ctx.Table[ir.ObjectNumber.Value()]; ok && e != nil && !e.Free {
			return e.Object
		}
		return nil
	}
	return o
}

func (g *refGraph) dict(o types.Object) types.Dict {
	switch o := g.deref(o).(type) {
	case types.Dict:
		return o
	case types.StreamDict:
		return o.Dict
	}
	return nil
}

func (g *refGraph) array(o types.Object) types.Array {
	a, _ := g.deref(o).(types.Array)
	return a
}

// each follows every element of an array, or the one value when it is not one — `/Next`, `/Function` and a
// transfer function take either form.
func (g *refGraph) each(o types.Object, r role, key string, emit emitFn) {
	if a := g.array(o); a != nil {
		for _, v := range a {
			g.follow(v, r, key, emit)
		}
		return
	}
	if o != nil {
		g.follow(o, r, key, emit)
	}
}

func (g *refGraph) values(o types.Object, r role, key string, emit emitFn) {
	d := g.dict(o)
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g.follow(d[k], r, key+" /"+k, emit)
	}
}

// expand emits the edges pdfcpu's validator recurses on from o in role r.
func (g *refGraph) expand(o types.Object, r role, via string, emit emitFn) {
	at := func(key string) string { return strings.TrimSpace(via + " /" + key) }
	if r == roleColorSpace {
		a := g.array(o)
		if len(a) < 2 {
			return
		}
		switch n, _ := a[0].(types.Name); n.Value() {
		case model.IndexedCS, model.PatternCS:
			g.follow(a[1], roleColorSpace, at(n.Value()), emit)
		case model.SeparationCS:
			if len(a) > 3 {
				g.follow(a[2], roleColorSpace, at("Separation"), emit)
				g.follow(a[3], roleFunction, at("Separation"), emit)
			}
		case model.DeviceNCS:
			if len(a) > 3 {
				g.follow(a[2], roleColorSpace, at("DeviceN"), emit)
				g.follow(a[3], roleFunction, at("DeviceN"), emit)
			}
			if len(a) > 4 {
				g.follow(a[4], roleDeviceNAttrs, at("DeviceN"), emit)
			}
		}
		return
	}
	d := g.dict(o)
	if d == nil {
		if r == roleAction { // a `/Next` array
			g.each(o, roleAction, at("Next"), emit)
		}
		return
	}
	switch r {
	case roleResourced:
		if res := g.dict(d["Resources"]); res != nil {
			for _, v := range g.dict(res["Pattern"]) {
				if _, stream := g.deref(v).(types.StreamDict); stream {
					g.follow(v, roleResourced, at("Resources /Pattern"), emit) // a tiling pattern (pattern.go:128)
				} else {
					g.follow(v, roleShadingPattern, at("Resources /Pattern"), emit)
				}
			}
			g.values(res["Properties"], roleResourced, at("Resources /Properties"), emit)
			g.values(res["ColorSpace"], roleColorSpace, at("Resources /ColorSpace"), emit)
			g.values(res["Shading"], roleShading, at("Resources /Shading"), emit)
			g.values(res["ExtGState"], roleExtGState, at("Resources /ExtGState"), emit)
		}
		for _, v := range g.array(d["Annots"]) {
			g.follow(v, roleAnnot, at("Annots"), emit)
		}
	case roleCMap:
		if ir, ok := d["UseCMap"].(types.IndirectRef); ok { // a name is a predefined CMap
			emit(node{ir.ObjectNumber.Value(), roleCMap}, at("UseCMap"))
		}
	case roleFunction:
		if g.int(d["FunctionType"]) == 3 {
			for _, v := range g.array(d["Functions"]) {
				g.follow(v, roleFunction, at("Functions"), emit)
			}
		}
	case roleDeviceNAttrs:
		g.values(d["Colorants"], roleColorSpace, at("Colorants"), emit)
		if p, ok := d["Process"]; ok {
			g.follow(p, roleDeviceNProcess, at("Process"), emit)
		}
	case roleDeviceNProcess:
		if cs, ok := d["ColorSpace"]; ok {
			g.follow(cs, roleColorSpace, at("ColorSpace"), emit)
		}
	case roleShading:
		if cs, ok := d["ColorSpace"]; ok {
			g.follow(cs, roleColorSpace, at("ColorSpace"), emit)
		}
		g.each(d["Function"], roleFunction, at("Function"), emit)
	case roleShadingPattern:
		if s, ok := d["Shading"]; ok {
			g.follow(s, roleShading, at("Shading"), emit)
		}
		if gs, ok := d["ExtGState"]; ok {
			g.follow(gs, roleExtGState, at("ExtGState"), emit)
		}
	case roleExtGState:
		for _, k := range transferKeys {
			g.each(d[k], roleFunction, at(k), emit)
		}
		if ht, ok := d["HT"]; ok {
			g.follow(ht, roleHalftone, at("HT"), emit)
		}
		if g.dict(d["SMask"]) != nil { // or the name /None
			g.follow(d["SMask"], roleSoftMask, at("SMask"), emit)
		}
	case roleSoftMask:
		g.each(d["TR"], roleFunction, at("TR"), emit)
	case roleHalftone:
		if _, stream := g.deref(o).(types.StreamDict); !stream && g.int(d["HalftoneType"]) == 5 {
			for _, k := range halftoneColorants {
				if v, ok := d[k]; ok {
					g.follow(v, roleHalftone, at(k), emit)
				}
			}
		}
	case roleImage, roleAltImage:
		if b, _ := g.deref(d["ImageMask"]).(types.Boolean); !b.Value() {
			if _, stream := g.deref(d["Mask"]).(types.StreamDict); stream {
				g.follow(d["Mask"], roleImage, at("Mask"), emit)
			}
		}
		if r == roleImage {
			for _, v := range g.array(d["Alternates"]) {
				g.follow(v, roleAltImage, at("Alternates"), emit)
			}
		}
		if cs, ok := d["ColorSpace"]; ok {
			g.follow(cs, roleColorSpace, at("ColorSpace"), emit)
		}
	case roleAction:
		if next, ok := d["Next"]; ok {
			g.each(next, roleAction, at("Next"), emit)
		}
		switch nameOf(d, "S") {
		case "GoToE":
			if t, ok := d["T"]; ok {
				g.follow(t, roleTarget, at("T"), emit)
			}
		case "GoTo3DView":
			if ta, ok := d["TA"]; ok {
				g.follow(ta, roleAnnot, at("TA"), emit)
			}
		case "Rendition":
			if rd, ok := d["R"]; ok {
				g.follow(rd, roleRendition, at("R"), emit)
			}
		}
	case roleTarget:
		if t, ok := d["T"]; ok {
			g.follow(t, roleTarget, at("T"), emit)
		}
	case roleAnnot:
		st := nameOf(d, "Subtype")
		if markup[st] {
			if irt, ok := d["IRT"]; ok {
				g.follow(irt, roleAnnot, at("IRT"), emit)
			}
		}
		switch st {
		case "Link", "Widget", "Screen":
			if a, ok := d["A"]; ok {
				g.follow(a, roleAction, at("A"), emit)
			}
		}
		if st == "Widget" || st == "Screen" {
			g.values(d["AA"], roleAction, at("AA"), emit)
		}
	case roleRendition:
		if nameOf(d, "S") == "SR" {
			for _, v := range g.array(d["R"]) {
				g.follow(v, roleRendition, at("R"), emit)
			}
		}
	case roleMediaClip:
		if nameOf(d, "S") == "MCS" {
			if cd, ok := d["D"]; ok {
				g.follow(cd, roleMediaClip, at("D"), emit)
			}
		}
	case roleTree:
		for _, v := range g.array(d["Kids"]) {
			g.follow(v, roleTree, at("Kids"), emit)
		}
	}
}

func (g *refGraph) int(o types.Object) int {
	if i, ok := g.deref(o).(types.Integer); ok {
		return i.Value()
	}
	return -1
}

func nameOf(d types.Dict, key string) string {
	if n, ok := d[key].(types.Name); ok {
		return n.Value()
	}
	return ""
}

func intOf(d types.Dict, key string) int {
	if i, ok := d[key].(types.Integer); ok {
		return i.Value()
	}
	return -1
}
