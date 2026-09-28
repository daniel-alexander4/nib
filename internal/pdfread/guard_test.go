package pdfread_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryValidatingReadRoutesThroughTheDoor — ADR-009: the `/UseCMap` refusal (`/pending 675`) and the optimize
// budget (`/pending 706`, `/pending 714`) hold only if nothing reaches pdfcpu's validator or its optimize pass
// around this package. It checks ROUTING, over every non-test Go file in the module outside this package:
//
//  1. No call to pdfcpu's validating reads or its pass (`api.ReadAndValidate`, `api.ReadValidateAndOptimize`,
//     `api.ValidateContext`, `api.OptimizeContext`, `pdfcpu.OptimizeXRefTable`), and no import of pdfcpu's
//     `validate` package — read through `Validated`/`ReadOptimized`, optimize through `Optimize`.
//  2. In any function that calls pdfcpu's `api`, no `bytes.NewReader` except as the direct argument of a
//     call that is not pdfcpu's (a CSV or image decoder), of `api.ReadContext` (the unvalidated read, which does not follow `/UseCMap`) or at a named not-a-PDF
//     position — `api.Create`'s JSON spec, `api.ImportImages`' images, `api.ImageWatermarkForReader`'s image.
//     Every other reader handed to pdfcpu comes from `Reader`.
//
// **What it cannot see:** a reader built in one function and handed to pdfcpu from another, and a reader of
// any other constructor than `bytes.NewReader` / `strings.NewReader`. None does today; a new one would pass.
func TestEveryValidatingReadRoutesThroughTheDoor(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bannedAPI := map[string]bool{"ReadAndValidate": true, "ReadValidateAndOptimize": true, "ValidateContext": true, "OptimizeContext": true}
	bannedPdfcpu := map[string]bool{"OptimizeXRefTable": true, "OptimizeContext": true}
	// notAPDF is each api function's argument position that takes something other than a PDF.
	notAPDF := map[string]int{"Create": 1, "ImportImages": 2, "ImageWatermarkForReader": 0}
	scanned, apiFiles := 0, 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir // .git, and .claude/worktrees: other checkouts, not this module
			}
			switch d.Name() {
			case "node_modules", "vendor", "testdata", "pdfread":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		scanned++
		apiName, pdfcpuName, bytesName, stringsName, ioName := "", "", "", "", ""
		for _, im := range file.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			local := p[strings.LastIndex(p, "/")+1:]
			if im.Name != nil {
				local = im.Name.Name
			}
			switch p {
			case "github.com/pdfcpu/pdfcpu/pkg/api":
				apiName = local
			case "github.com/pdfcpu/pdfcpu/pkg/pdfcpu":
				pdfcpuName = local
			case "bytes":
				bytesName = local
			case "strings":
				stringsName = local
			case "io":
				ioName = local
			case "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/validate":
				t.Errorf("%s imports pdfcpu's validate package — validate through pdfread.Validated", rel)
			}
		}
		if apiName == "" && pdfcpuName == "" {
			return nil
		}
		apiFiles++
		isSel := func(e ast.Expr, pkg string) (string, bool) {
			sel, ok := e.(*ast.SelectorExpr)
			if !ok || pkg == "" {
				return "", false
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok || x.Name != pkg {
				return "", false
			}
			return sel.Sel.Name, true
		}
		isNewReader := func(e ast.Expr) bool {
			c, ok := e.(*ast.CallExpr)
			if !ok {
				return false
			}
			if name, ok := isSel(c.Fun, bytesName); ok && name == "NewReader" {
				return true
			}
			name, ok := isSel(c.Fun, stringsName)
			return ok && name == "NewReader"
		}
		// **Every reference, not only a call, and the whole file, not only function bodies** (the review of
		// /pending 675/714): `var read = api.ReadAndValidate` at package level, or `f := api.X; f(...)`, reached
		// pdfcpu's validator past a guard that inspected call sites inside functions. And every `api.*File`
		// function reads a path from disk through its own validating read.
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if name, ok := isSel(sel, apiName); ok && (bannedAPI[name] || strings.HasSuffix(name, "File")) {
				t.Errorf("%s:%d: names api.%s — read through pdfread (Validated, ReadOptimized, Reader), optimize "+
					"through pdfread.Optimize", rel, fset.Position(sel.Pos()).Line, name)
			}
			if name, ok := isSel(sel, pdfcpuName); ok && bannedPdfcpu[name] {
				t.Errorf("%s:%d: names pdfcpu.%s — optimize through pdfread.Optimize", rel, fset.Position(sel.Pos()).Line, name)
			}
			return true
		})
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			callsAPI := false
			allowed := map[ast.Expr]bool{}
			var readers []ast.Expr
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				// A conversion into package io (`io.ReadSeeker(bytes.NewReader(b))`) is not a consumer: the reader
				// it wraps goes on to wherever the conversion goes, so it excuses nothing (the review, bypass 1).
				_, isIOConv := isSel(c.Fun, ioName)
				if _, ok := isSel(c.Fun, apiName); !ok && !isIOConv {
					// A reader handed straight to something that is not pdfcpu (csv, image decoding) is not one
					// pdfcpu reads; `append` is excluded because appending is how a reader reaches `api.MergeRaw`.
					if id, isIdent := c.Fun.(*ast.Ident); !isIdent || id.Name != "append" {
						for _, a := range c.Args {
							if isNewReader(a) {
								allowed[a] = true
							}
						}
					}
				}
				if name, ok := isSel(c.Fun, apiName); ok {
					callsAPI = true
					for i, a := range c.Args {
						if name == "ReadContext" && i == 0 || notAPDF[name] == i && name != "ReadContext" && hasKey(notAPDF, name) {
							ast.Inspect(a, func(m ast.Node) bool {
								if e, ok := m.(ast.Expr); ok && isNewReader(e) {
									allowed[e] = true
								}
								return true
							})
						}
					}
				}
				if isNewReader(c) {
					readers = append(readers, c)
				}
				return true
			})
			if !callsAPI {
				continue
			}
			for _, r := range readers {
				if !allowed[r] {
					t.Errorf("%s:%d: %s hands pdfcpu a reader it built itself — take it from pdfread.Reader, so "+
						"the /UseCMap loop is refused before pdfcpu validates", rel, fset.Position(r.Pos()).Line, fn.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 200 || apiFiles < 20 {
		t.Fatalf("scanned %d source files, %d using pdfcpu's api — the guard is not reading the module", scanned, apiFiles)
	}
}

func hasKey(m map[string]int, k string) bool { _, ok := m[k]; return ok }
