package pdfread

import "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

// Hooks for the external tests of the page-content estimate (`contentcost.go`).
var (
	DecodeOnce             = &decodeOnce
	PassDecodesPageContent = passDecodesPageContent
	ValidatorPaths         = validatorPaths // the reference door's walk (refgraph.go), without the budget
)

// WalkedInOnePass reports whether Pages answered from its one walk rather than asking PageDict page by page.
func WalkedInOnePass(ctx *model.Context) bool {
	_, ok := PagesWalked(ctx)
	return ok
}
