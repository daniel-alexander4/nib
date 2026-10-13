package pdfread

import "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

// Hooks for the external tests of the page-content estimate (`contentcost.go`).
var (
	DecodeOnce             = &decodeOnce
	PassDecodesPageContent = passDecodesPageContent
	ValidatorPaths         = validatorPaths // the reference door's walk (refgraph.go), without the budget
	CountPaths             = countPaths     // the same walk stopped past a budget, with the edges it followed
	ObjectLevels           = objectLevels   // the every-reference depth pass (refall.go)
	MaxObjectLevels        = maxObjectLevels
	FixFreeReferences      = fixFreeReferences         // pdfcpu's free-reference walk, without recursion (optimize.go)
	FreeReferencePass      = &freeReferencePass        // what `optimize` runs ahead of pdfcpu's pass
	RefuseUnbounded        = refuseUnboundedReferences // the whole door, on a context nothing has validated
	PathBudget             = pathBudget                // the reference door's budget (refgraph.go)
	ValidatorDepth         = validatorDepth            // the depth pass through guarded edges (refdepth.go)
	SimulatePages          = simulatePages             // the tolerant page walk (pagesim.go), with the work it spent
	SimBudget              = simBudget
)

// WalkedInOnePass reports whether Pages answered from its one walk rather than asking PageDict page by page.
func WalkedInOnePass(ctx *model.Context) bool {
	_, ok := PagesWalked(ctx)
	return ok
}

// The name-tree door (nametree.go, ADR-164).
const MaxWrittenNameTreeDepth = maxWrittenNameTreeDepth

var MaxNameTreeNames = &maxNameTreeNames

// WalkNameTree is nib's own walk of the catalog name tree called kind in a context nothing has validated: how
// many names it read, how deep the tree is, and whether it vouches for having read all of it once.
func WalkNameTree(ctx *model.Context, kind string) (names, depth int, whole bool) {
	root, _ := ctx.XRefTable.Catalog()
	nd, _ := ctx.DereferenceDict(root["Names"])
	td, _ := ctx.DereferenceDict(nd[kind])
	pairs, depth, whole := walkNameTree(ctx, td)
	return len(pairs), depth, whole
}
