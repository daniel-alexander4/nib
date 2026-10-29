package pdfread

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"weak"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A catalog name tree the validator cannot read is read first, and one it still loses is never called empty —
// ADR-164, `/pending 880`.
//
// pdfcpu's relaxed validator walks each tree under the catalog's `/Names` by recursion and gives up on a kid more
// than `MaxRecursionDepth` (100) nodes down (`validate/nameTree.go:707`, v0.13.0). It does not refuse the
// document: the error is swallowed where the kid was asked for (`:750-756`, `continue`), the upper bound the
// failed call returned — the empty string — is written over the parent's (`:750`, and into the file's own
// `/Limits` by `:688-690`), and a tree whose bound is empty is not kept (`validate/xReftable.go:325`). The
// catalog's `/Names` is then deleted and rebuilt from what was kept (`:333-336`). So a tree nested past the limit
// on its LAST kid is gone whole, and one nested past it on an earlier kid loses that kid and everything under it —
// and nothing says so. Every reader then saw a document with no attachments (no named destinations, no scripts),
// and every write wrote one.
//
// **pdfcpu's own writer builds such a tree.** `model.Node.Add` (`model/nameTree.go:309`, `maxEntries = 3`) splits
// a full leaf in two and never rebalances, so keys added in sorted order — `scan-001.pdf`, `scan-002.pdf` — make
// a chain one node deeper for every two keys. Measured through `pdfops.AddAttachment`: the 204th file leaves a
// tree 101 deep, `Attachments` lists none, `pdfdetach` lists 204 in the same bytes, and the next rewrite writes a
// document with none. 1,000 files in reverse order list 200.
//
// Three things are done about it, all here.
//
//  1. **Read before validation** (`settleNameTrees`). Each tree is walked by a loop of nib's own — no recursion,
//     every node at most once — and one deeper than the validator takes is written FLAT into its own root, in the
//     context, before the validator sees it. The validator then reads every key and vouches for every value
//     itself, which is why this is not the put-back ADR-129 refused at the read: nothing is carried past the
//     validator, the tree is only given a shape it can walk. A name tree's shape carries no meaning (ISO 32000-1
//     §7.9.6).
//  2. **What the validator still drops is marked, never called empty** (`rememberNameTrees`). A kid can be
//     dropped for other reasons (a value the validator refuses, a loop in `/Kids`), and a tree nib's walk could
//     not read whole is left as it is. Where the tree the validator kept holds fewer names than the walk counted,
//     the tree is marked unread: a reader asks `NameTreeUnread` and answers with an error, and `Write` refuses
//     the context (`ErrNameTreeLost`) unless the operation is the removal of that very tree
//     (`ForgetNameTree`).
//  3. **No write nests a tree past `maxWrittenNameTreeDepth`** (`shapeNameTrees`, `Write`'s). Whatever built the
//     chain — an attach, a merge's `AddTree` of sorted destinations — the door rebuilds it balanced before
//     pdfcpu binds it, so nib never writes a tree an older nib, or pdfcpu, reads as empty.
//
// The mark does not follow a context's objects into another context. Both merges ask `RefuseNameTreeLoss` of each
// source, because a merge carries the source's trees; a cut or an imposition makes a new document that takes none.

// catalogNameTrees are the trees pdfcpu's validator reads under the catalog's `/Names`
// (`validate/xReftable.go:292-293`). A key of that dictionary not on this list is dropped by the same rebuild
// (`:300-305`) — a loss this file does not address, and on purpose (`/pending 883`): the ten are every entry of
// the name dictionary in ISO 32000-1 Table 31 and 32000-2 Table 32, so what is dropped is a key no standard
// defines and no reader of nib's asks for.
//
// `/Dests` has three readers in `pdfops` that take `xt.Names["Dests"]` as it stands and do not ask
// `NameTreeUnread` (`anchors.go`, `outlinecarry.go`, `pageselect.go`). Left so (`/pending 883`): each answers a
// yes-or-no inside an operation that ends at `Write`, which refuses the context whole, so a lost tree read as
// "none" never reaches a file. The one answer that is shown without a write is whether a paragraph has a
// destination anchored to it (`grouping.go`).
var catalogNameTrees = []string{"Dests", "AP", "JavaScript", "Pages", "Templates", "IDS", "URLS",
	"EmbeddedFiles", "AlternatePresentations", "Renditions"}

// maxNameTreeNames is the most names nib's own walk reads from one tree; past it the walk stops and the tree is
// marked unread. Far above any document's (a million attachments, destinations or scripts), and the walk's cost
// is linear in it (measured in ADR-164). A variable only so a test can reach the arm with a small document.
var maxNameTreeNames = 1 << 20

// maxWrittenNameTreeDepth is the deepest name tree `Write` hands pdfcpu's writer as it stands; a deeper one is
// rebuilt (`shapeNameTrees`). pdfcpu's own trees over keys in random order measure 16 deep at 1,000 names and 23
// at 10,000, so the figure leaves those alone and is a third of what the validator reads.
const maxWrittenNameTreeDepth = 32

// nameTreeFan is how many names a rebuilt leaf holds and how many kids a rebuilt node holds: three levels reach
// 262,144 names.
const nameTreeFan = 64

// NameTreeRefusal is the end of the sentence a user reads when a write is refused, after what the lost list holds
// (`nameTreeWords`); the server's error door recognises it to answer 422 with a cause.
const NameTreeRefusal = "which Nib could not read in full and cannot carry through a change — so nothing was changed"

// WebCaptureWords is what a lost Web Capture tree (`/URLS`, `/IDS`) is called, in a refusal and wherever a
// window offers to remove it (/pending 889 part 2); the server's error door recognises it to say the list can go.
const WebCaptureWords = "a record of the web pages it was captured from"

// nameTreeWords is what each catalog name tree holds, in a user's words. The refusal used to name three of the ten
// whatever was lost, and a document whose only loss was its Web Capture record was told about its attachments.
var nameTreeWords = map[string]string{
	"Dests":                  "its named destinations",
	"AP":                     "its named appearances",
	"JavaScript":             "its scripts",
	"Pages":                  "its named pages",
	"Templates":              "its page templates",
	"IDS":                    WebCaptureWords,
	"URLS":                   WebCaptureWords,
	"EmbeddedFiles":          "its attachments",
	"AlternatePresentations": "its slide shows",
	"Renditions":             "its media renditions",
}

// ErrNameTreeLost is Write's refusal of a context read from a document with a name tree nib could not read whole.
// `RefuseNameTreeLoss` wraps it in a sentence naming what was lost; errors.Is still answers it.
var ErrNameTreeLost = errors.New("this document holds a list " + NameTreeRefusal)

// nameTreeLost is ErrNameTreeLost naming the lists, each once.
type nameTreeLost struct{ what string }

func (e nameTreeLost) Error() string { return "this document holds " + e.what + ", " + NameTreeRefusal }
func (e nameTreeLost) Unwrap() error { return ErrNameTreeLost }

// ErrNameTreeUnread is what a READER of a marked tree answers with, in place of "the document has none".
var ErrNameTreeUnread = errors.New("this part of the document could not be read in full, so what it holds is not known")

// nameTreePair is one key and value as a leaf holds them: the key decoded, and both objects as written.
type nameTreePair struct {
	key  string
	k, v types.Object
}

// walkNameTree reads the tree under root without recursion and without pdfcpu's validator. depth is the deepest
// node met (root 0). whole is false where the walk could not vouch for having read everything exactly once: a
// node reached twice (a loop in `/Kids`, or a shared kid), a kid or a `/Names` that is not what it must be, a key
// that is not a string, or more than maxNameTreeNames names.
//
// `ctx.Dereference` and a type assertion, never a typed dereference: this runs before the validator (ADR-116).
func walkNameTree(ctx *model.Context, root types.Dict) (pairs []nameTreePair, depth int, whole bool) {
	pairs, depth, whole, _ = walkNameTreeShared(ctx, root)
	return pairs, depth, whole
}

// walkNameTreeShared is walkNameTree that also says whether a node was reached twice, a loop in `/Kids` or a kid
// two parents share: the shape pdfcpu v0.16.2's validator refuses the whole document over (`ErrNameTreeCycle`,
// `ErrNameTreeDuplicate`, `model/recursion.go:58-62`).
func walkNameTreeShared(ctx *model.Context, root types.Dict) (pairs []nameTreePair, depth int, whole, shared bool) {
	type frame struct {
		d     types.Dict
		depth int
	}
	raw := func(o types.Object) types.Object {
		o, err := ctx.Dereference(o)
		if err != nil {
			return nil
		}
		return o
	}
	whole = true
	seen := map[int]bool{}
	stack := []frame{{root, 0}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.depth > depth {
			depth = f.depth
		}
		if kidsObj, has := f.d["Kids"]; has {
			kids, ok := raw(kidsObj).(types.Array)
			if !ok {
				whole = false
				continue
			}
			// Pushed last kid first, so the leaves are met in the tree's own order.
			for i := len(kids) - 1; i >= 0; i-- {
				if ref, isRef := kids[i].(types.IndirectRef); isRef {
					n := ref.ObjectNumber.Value()
					if seen[n] {
						whole, shared = false, true
						continue
					}
					seen[n] = true
				}
				kd, ok := raw(kids[i]).(types.Dict)
				if !ok {
					whole = false
					continue
				}
				stack = append(stack, frame{kd, f.depth + 1})
			}
			continue
		}
		names, ok := raw(f.d["Names"]).(types.Array)
		if !ok || len(names)%2 == 1 {
			whole = false
			continue
		}
		for i := 0; i+1 < len(names); i += 2 {
			s, err := types.StringOrHexLiteral(raw(names[i]))
			if err != nil || s == nil {
				whole = false
				continue
			}
			if len(pairs) >= maxNameTreeNames {
				return pairs, depth, false, shared
			}
			pairs = append(pairs, nameTreePair{*s, names[i], names[i+1]})
		}
	}
	return pairs, depth, whole, shared
}

// nameTreeRead is what nib's own walk made of one catalog name tree before validation.
type nameTreeRead struct {
	names  int  // names the walk counted
	capped bool // the walk stopped at maxNameTreeNames, so names is not the tree's count
	// aside is set when the tree was taken out of the validator's sight (settleNameTrees): the validator now refuses
	// the whole DOCUMENT over a looped, shared or over-deep tree, so it never sees one, and the tree is unread.
	aside bool
}

// settleNameTrees walks every catalog name tree of ctx BEFORE validation and returns what it counted, by tree. A
// tree read whole that nests deeper than pdfcpu's validator follows is rewritten flat into its own root dictionary
// — every key and value as the file wrote them, in key order — so the validator reads all of it.
//
// A tree the validator would refuse the whole document over — a node reached twice, or nested past the limit with
// something the walk could not vouch for (`validate/tree_traversal.go:76`, v0.16.2: fatal even in relaxed mode, where
// the release before dropped the tree and read on) — is taken out of the catalog's `/Names` for the validation and
// put back by the returned function, and recorded as `aside`: it is unread, so every reader answers `ErrNameTreeUnread`
// and the write refuses, exactly as for a tree the validator dropped (ADR-164, ADR-171). The walk that decides this is
// iterative and visited-set bounded, which is what a carry of a hostile shape needs (D4 pin 3).
//
// Nothing is returned for a document with no `/Names`, which is most documents: the read pays one catalog lookup.
func settleNameTrees(ctx *model.Context) (map[string]nameTreeRead, func()) {
	noop := func() {}
	if ctx == nil || ctx.XRefTable == nil {
		return nil, noop
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return nil, noop
	}
	o, err := ctx.Dereference(root["Names"])
	if err != nil {
		return nil, noop
	}
	names, ok := o.(types.Dict)
	if !ok || len(names) == 0 {
		return nil, noop
	}
	limit := ctx.XRefTable.MaxRecursionDepth()
	out := map[string]nameTreeRead{}
	var put []func()
	for _, tree := range catalogNameTrees {
		to, err := ctx.Dereference(names[tree])
		if err != nil {
			continue
		}
		td, ok := to.(types.Dict)
		if !ok || len(td) == 0 {
			continue
		}
		pairs, depth, whole, shared := walkNameTreeShared(ctx, td)
		r := nameTreeRead{names: len(pairs), capped: len(pairs) >= maxNameTreeNames && !whole}
		if shared || (!whole && depth > limit) {
			r.aside = true
			orig := names[tree]
			delete(names, tree)
			put = append(put, func() { names[tree] = orig })
		}
		out[tree] = r
		if !whole || depth <= limit {
			continue
		}
		flattenNameTree(td, pairs)
	}
	return out, func() {
		for _, p := range put {
			p()
		}
	}
}

// flattenNameTree rewrites the tree under root as one leaf: every pair in key order in the root's own `/Names`,
// each key and value the object the file wrote.
func flattenNameTree(root types.Dict, pairs []nameTreePair) {
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	flat := make(types.Array, 0, 2*len(pairs))
	for _, p := range pairs {
		flat = append(flat, p.k, p.v)
	}
	delete(root, "Kids")
	delete(root, "Limits")
	root["Names"] = flat
}

// settleIDTree does for the structure tree's `/IDTree` what settleNameTrees does for the catalog's trees
// (`/pending 883`). It is a name tree, it is not under `/Names`, and pdfcpu's validator treats it worse than it
// treats those: past the depth it follows it drops nothing and instead writes EMPTY strings into the `/Limits` of
// every node it did reach (`validate/nameTree.go`, the relaxed kid loop carries on with an unset bound and
// `validateNameTreeDictLimitsEntry` stores it in the document's own array), and the write carries them out. A
// reader that finds an id by `/Limits` then finds none. So a tree read whole and nested deeper than the validator
// follows is made flat here; `pdfops`' `setIDTreeEntry` adds to a root leaf as it does to any other.
//
// It stays flat in the output: `shapeNameTrees` rebuilds what `ctx.Names` holds, and the validator hands this tree
// to nobody. Declared: a deep tree the walk could NOT read whole is left to the validator, damage and all.
func settleIDTree(ctx *model.Context) {
	if ctx == nil || ctx.XRefTable == nil {
		return
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return
	}
	o, err := ctx.Dereference(root["StructTreeRoot"])
	if err != nil {
		return
	}
	st, ok := o.(types.Dict)
	if !ok {
		return
	}
	if o, err = ctx.Dereference(st["IDTree"]); err != nil {
		return
	}
	td, ok := o.(types.Dict)
	if !ok || len(td) == 0 {
		return
	}
	pairs, depth, whole := walkNameTree(ctx, td)
	if !whole || depth <= ctx.XRefTable.MaxRecursionDepth() {
		return
	}
	flattenNameTree(td, pairs)
}

// unreadTrees holds, per cross-reference table, the catalog name trees pdfcpu's validator kept less of than nib's
// own walk counted. Keyed weakly, as `afOnly` is, and by the TABLE: the readers that must ask are handed one.
var unreadTrees = struct {
	sync.Mutex
	m map[weak.Pointer[model.XRefTable]]map[string]bool
}{m: map[weak.Pointer[model.XRefTable]]map[string]bool{}}

// rememberNameTrees marks, after validation, each tree of which the validator kept fewer names than read counted.
func rememberNameTrees(ctx *model.Context, read map[string]nameTreeRead) {
	if len(read) == 0 {
		return
	}
	xt := ctx.XRefTable
	var lost map[string]bool
	for tree, r := range read {
		kept := 0
		if node := xt.Names[tree]; node != nil {
			_ = node.Process(context.Background(), xt, func(*model.XRefTable, string, *types.Object) error {
				kept++
				return nil
			})
		}
		if r.aside || r.capped || kept < r.names {
			if lost == nil {
				lost = map[string]bool{}
			}
			lost[tree] = true
		}
	}
	if lost == nil {
		return
	}
	key := weak.Make(xt)
	unreadTrees.Lock()
	unreadTrees.m[key] = lost
	unreadTrees.Unlock()
	runtime.AddCleanup(xt, func(k weak.Pointer[model.XRefTable]) {
		unreadTrees.Lock()
		delete(unreadTrees.m, k)
		unreadTrees.Unlock()
	}, key)
}

// NameTreeUnread reports whether the catalog name tree called tree, in the document xt was read from, holds names
// the context does not: pdfcpu's validator dropped part or all of it. A reader of that tree must answer with
// ErrNameTreeUnread — "could not read them" — and never with what `xt.Names` holds, which may be nothing.
func NameTreeUnread(xt *model.XRefTable, tree string) bool {
	if xt == nil {
		return false
	}
	unreadTrees.Lock()
	defer unreadTrees.Unlock()
	return unreadTrees.m[weak.Make(xt)][tree]
}

// ForgetNameTree takes the mark off tree. Its callers are the operations that REMOVE that tree from the document
// on purpose — exactly three, held by `TestForgetNameTreeHasItsThreeNamedCallers`: `pdfops`' Remove active content
// (scripts), Remove files (attachments) and Remove the Web Capture record (/pending 889 part 2). The validator has
// already taken the tree out of the context, the write leaves it out, and that is what was asked for.
func ForgetNameTree(xt *model.XRefTable, tree string) {
	if xt == nil {
		return
	}
	unreadTrees.Lock()
	defer unreadTrees.Unlock()
	delete(unreadTrees.m[weak.Make(xt)], tree)
}

// RefuseNameTreeLoss is ErrNameTreeLost where ctx was read from a document with a name tree still marked unread.
// `Write` asks it; so does a merge of each source, whose trees the merge carries and whose mark it does not.
func RefuseNameTreeLoss(ctx *model.Context) error {
	if ctx == nil || ctx.XRefTable == nil {
		return nil
	}
	trees := UnreadNameTrees(ctx.XRefTable)
	if len(trees) == 0 {
		return nil
	}
	return nameTreeLost{NameTreeWords(trees)}
}

// NameTreeWords says what the catalog name trees named hold, each thing once — the ONE wording of a lost list, for
// the refusal and for the document's answer the window words its notice from.
func NameTreeWords(trees []string) string {
	var said []string
	for _, t := range trees {
		if w := nameTreeWords[t]; w != "" && !slices.Contains(said, w) {
			said = append(said, w)
		}
	}
	return strings.Join(said, " and ")
}

// UnreadNameTrees is every catalog name tree still marked unread in the document xt was read from, sorted — what a
// document's answer says it holds and Nib cannot carry (`pdfops.ReadAnswerFacts`).
func UnreadNameTrees(xt *model.XRefTable) []string {
	if xt == nil {
		return nil
	}
	unreadTrees.Lock()
	defer unreadTrees.Unlock()
	var trees []string
	for t, lost := range unreadTrees.m[weak.Make(xt)] {
		if lost {
			trees = append(trees, t)
		}
	}
	slices.Sort(trees)
	return trees
}

// nameNodeDepth is the deepest node under n (n itself 0), by a loop.
func nameNodeDepth(n *model.Node) int {
	type frame struct {
		n     *model.Node
		depth int
	}
	deepest := 0
	stack := []frame{{n, 0}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.depth > deepest {
			deepest = f.depth
		}
		for _, k := range f.n.Kids {
			if k != nil {
				stack = append(stack, frame{k, f.depth + 1})
			}
		}
	}
	return deepest
}

// shapeNameTrees rebuilds, in ctx, every name tree nested deeper than maxWrittenNameTreeDepth: the same keys and
// values in the same order, in leaves of nameTreeFan under nodes of nameTreeFan. `Write` is its caller. The root
// keeps its dictionary, so the catalog still names it; the old kids are left for the writer to not reach.
func shapeNameTrees(ctx *model.Context) {
	for _, root := range ctx.Names {
		if root == nil || nameNodeDepth(root) <= maxWrittenNameTreeDepth {
			continue
		}
		var level []*model.Node
		var leaf *model.Node
		_ = root.Process(context.Background(), ctx.XRefTable, func(_ *model.XRefTable, k string, v *types.Object) error {
			if leaf == nil || len(leaf.Names) == nameTreeFan {
				leaf = &model.Node{Kmin: k}
				level = append(level, leaf)
			}
			leaf.AppendToNames(k, *v)
			leaf.Kmax = k
			return nil
		})
		for len(level) > nameTreeFan {
			var up []*model.Node
			for i := 0; i < len(level); i += nameTreeFan {
				kids := level[i:min(i+nameTreeFan, len(level))]
				up = append(up, &model.Node{Kids: kids, Kmin: kids[0].Kmin, Kmax: kids[len(kids)-1].Kmax})
			}
			level = up
		}
		if len(level) == 0 {
			continue
		}
		root.Kids, root.Names = level, nil
		root.Kmin, root.Kmax = level[0].Kmin, level[len(level)-1].Kmax
	}
}
