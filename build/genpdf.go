//go:build ignore

// genpdf writes a small, genuinely valid PDF, for harnesses that are not written in Go.
//
// Usage: go run build/genpdf.go <out.pdf> [page text ...]
//
// It exists because `build/winrepro.sh` used to build its fixture as
//
//	cp .playwright-mcp/shots/docA.pdf "$WHOME/nibprobe/report.pdf" 2>/dev/null \
//	  || printf '%%PDF-1.7\n' > "$WHOME/nibprobe/report.pdf"
//
// — and that source is a gitignored scratch artifact, so on any machine but the one it
// was captured on the fallback fires and the "PDF" is a NINE-BYTE HEADER. Every check in
// that harness passed against it, because `LooksLikePDF` only wants the header and
// nothing in a headless run renders the file. The checks were therefore silent about
// whether Nib can open a real document on Windows, while reading as though they had said
// so — and the same nine bytes would have been the input to P07's hand-off checks, which
// resolve a path through the ordinary install path.
//
// Go rather than bash: winrepro already requires a Go toolchain (it cross-compiles
// nib.exe), so this adds no dependency, where hand-rolling xref byte offsets in shell
// would add both a dependency and a second implementation of a thing the repo has.
// `//go:build ignore` keeps it out of `go build ./...` while `go run` still takes it.
package main

import (
	"fmt"
	"os"

	"nib/internal/testpdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run build/genpdf.go <out.pdf> [page text ...]")
		os.Exit(2)
	}
	pages := os.Args[2:]
	if len(pages) == 0 {
		pages = []string{"nib test page"}
	}
	b, err := testpdf.Text(pages...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generating the pdf: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[1], b, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}
