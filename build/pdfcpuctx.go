//go:build ignore

// pdfcpuctx threads a context.Context into the calls the compiler says now need one (PLAN-pdfcpu-upgrade P02.S01).
//
// Usage: go build -gcflags=-e ./... 2>&1 | go run build/pdfcpuctx.go [-vet] > edits.log
//
// It reads `not enough arguments in call to X` diagnostics from stdin, finds the CallExpr whose closing parenthesis is
// at the reported position, and inserts `context.Background()` as the FIRST argument (v0.16's convention for every
// call it changed). Nothing else is edited, which is what the plan's scan checks: every hunk of the commit it makes adds
// that one argument (and, once per file, the "context" import).
//
// **Background is the policy for library code**, and the plan's decision is not "everywhere": the install, commit,
// hand-off and ceremony doors must NOT honour a request's cancellation (handoff.go: /pending 783), and nothing in
// pdfread or pdfops holds a request context to pass. A read-only preview that wants cancellation is a later change that
// threads a context to it from its handler; this script never invents one.
//
// Re-runnable: it is idempotent (a call that already has the argument draws no diagnostic).
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var diag = regexp.MustCompile(`^(?:\d+\.\s+)?(?:vet: )?(\S+?\.go):(\d+):(\d+): not enough arguments in call to (\S+)`)

// diagPos is a diagnostic: where the compiler pointed and which callee it named. The position is NOT the call's closing
// parenthesis (it is the start of the call, or of its first argument), so a call is found by name and by containing it.
type diagPos struct {
	line, col int
	callee    string
}

// trailing names the callees whose new signature ALSO ends in an argument the old one lacked (a *model.Configuration the
// caller used to leave to the library's default). nil is that default, which is what the old signature meant.
var trailing = map[string]string{
	"api.TextWatermark":  ", nil",
	"api.ImageWatermark": ", nil",
	"api.PDFWatermark":   ", nil",
	// The progress options v0.16 added to the three entry points that read a whole document.
	"api.ReadValidateAndOptimize": ", nil",
	"api.Optimize":                ", nil",
	"api.Validate":                ", nil",
}

type edit struct {
	off  int
	text string
}

func main() {
	byFile := map[string][]diagPos{}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := diag.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		l, _ := strconv.Atoi(m[2])
		c, _ := strconv.Atoi(m[3])
		byFile[m[1]] = append(byFile[m[1]], diagPos{l, c, m[4]})
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	total := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var calls []*ast.CallExpr
		ast.Inspect(af, func(n ast.Node) bool {
			if ce, ok := n.(*ast.CallExpr); ok {
				calls = append(calls, ce)
			}
			return true
		})
		var edits []edit
		taken := map[*ast.CallExpr]bool{}
		unmatched := 0
		for _, d := range byFile[f] {
			var best *ast.CallExpr
			for _, ce := range calls {
				if taken[ce] || string(src[fset.Position(ce.Fun.Pos()).Offset:fset.Position(ce.Fun.End()).Offset]) != d.callee {
					continue
				}
				s, e := fset.Position(ce.Pos()), fset.Position(ce.End())
				in := (d.line > s.Line || d.line == s.Line && d.col >= s.Column) && (d.line < e.Line || d.line == e.Line && d.col <= e.Column)
				if in && (best == nil || ce.End()-ce.Pos() < best.End()-best.Pos()) {
					best = ce
				}
			}
			if best == nil {
				unmatched++
				continue
			}
			taken[best] = true
			lp := fset.Position(best.Lparen)
			// A call this script (or a hand) already gave its context needs only what is still missing: never a second.
			if len(best.Args) > 0 && string(src[fset.Position(best.Args[0].Pos()).Offset:fset.Position(best.Args[0].End()).Offset]) == "context.Background()" {
				if extra, ok := trailing[d.callee]; ok {
					edits = append(edits, edit{off: fset.Position(best.Rparen).Offset, text: extra})
				}
				continue
			}
			text := "context.Background(), "
			if len(best.Args) == 0 {
				text = "context.Background()"
			}
			edits = append(edits, edit{off: lp.Offset + 1, text: text})
			if extra, ok := trailing[d.callee]; ok {
				edits = append(edits, edit{off: fset.Position(best.Rparen).Offset, text: extra})
			}
		}
		if unmatched != 0 {
			fmt.Fprintf(os.Stderr, "%s: %d diagnostic(s) matched no call expression\n", f, unmatched)
			os.Exit(1)
		}
		sort.Slice(edits, func(i, j int) bool { return edits[i].off > edits[j].off })
		out := string(src)
		for _, e := range edits {
			out = out[:e.off] + e.text + out[e.off:]
		}
		if len(edits) > 0 && !hasContextImport(af) {
			out = addContextImport(out, af, fset)
		}
		if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("EDIT %s %d\n", f, len(edits))
		total += len(edits)
	}
	fmt.Printf("TOTAL %d\n", total)
}

func hasContextImport(af *ast.File) bool {
	for _, im := range af.Imports {
		if im.Path.Value == `"context"` && im.Name == nil {
			return true
		}
	}
	return false
}

// addContextImport puts "context" first in the file's import block (gofmt sorts it afterwards), or makes a block.
func addContextImport(src string, af *ast.File, fset *token.FileSet) string {
	for _, d := range af.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		if gd.Lparen.IsValid() {
			off := fset.Position(gd.Lparen).Offset + 1
			return src[:off] + "\n\t\"context\"" + src[off:]
		}
		off := fset.Position(gd.Pos()).Offset
		end := fset.Position(gd.End()).Offset
		return src[:off] + "import (\n\t\"context\"\n\t" + strings.TrimPrefix(src[off:end], "import ") + "\n)" + src[end:]
	}
	off := fset.Position(af.Name.End()).Offset
	return src[:off] + "\n\nimport \"context\"" + src[off:]
}
