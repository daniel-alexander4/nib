package pdfread

// Hooks for the external tests of the page-content estimate (`contentcost.go`).
var (
	DecodeOnce             = &decodeOnce
	PassDecodesPageContent = passDecodesPageContent
)
