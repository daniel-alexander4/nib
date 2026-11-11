package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// imageDecoders are the files that decode a picture today, each with how many decode calls it makes. Replacing a
// picture adds none (PLAN-image-edit P03.S01): its picture is decoded only inside Import, under Import's caps. A new
// row is a new decoder, decided here.
var imageDecoders = map[string]int{
	"pdfops/exportpage.go":     1,
	"pdfops/heifimport.go":     1,
	"pdfops/importraster.go":   6,
	"pdfops/pageimages.go":     4,
	"pdfops/pagestamp.go":      1,
	"pdfops/pdfops.go":         2,
	"pdfops/shrinkpictures.go": 1,
}

// TestNoNewImageDecoder counts the calls into the standard library's and x/image's picture decoders (Decode,
// DecodeConfig, DecodeAll on an `image/...` or `golang.org/x/image/...` import, or `image.Decode` itself) in pdfops,
// the server, the CLI and objedit — every package a replace runs through — by file, and holds them to the list.
func TestNoNewImageDecoder(t *testing.T) {
	got := map[string]int{}
	for _, d := range []struct{ dir, pkg string }{{".", "pdfops"}, {filepath.Join("..", "server"), "server"},
		{filepath.Join("..", "cli"), "cli"}, {filepath.Join("..", "objedit"), "objedit"}} {
		dir := d.dir
		ents, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			n := e.Name()
			if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, n), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			pictures := map[string]bool{}
			for _, im := range f.Imports {
				p, _ := strconv.Unquote(im.Path.Value)
				if p != "image" && !strings.HasPrefix(p, "image/") && !strings.HasPrefix(p, "golang.org/x/image/") {
					continue
				}
				name := filepath.Base(p)
				if im.Name != nil {
					name = im.Name.Name
				}
				pictures[name] = true
			}
			ast.Inspect(f, func(x ast.Node) bool {
				call, ok := x.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				switch {
				case !ok || !pictures[id.Name]:
				case sel.Sel.Name == "Decode", sel.Sel.Name == "DecodeConfig", sel.Sel.Name == "DecodeAll":
					got[d.pkg+"/"+n]++
				}
				return true
			})
		}
	}
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	for k := range imageDecoders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		if got[k] != imageDecoders[k] {
			t.Errorf("%s decodes pictures %d times, and the list says %d: a picture is decoded only where a row says so", k, got[k], imageDecoders[k])
		}
	}
	if len(got) == 0 {
		t.Fatal("no decode call was found: the census is reading nothing")
	}
}
