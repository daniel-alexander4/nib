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
