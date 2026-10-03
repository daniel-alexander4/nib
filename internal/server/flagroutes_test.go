package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// A signing flag (`pdfops/flags.go`) is a page coordinate with no content anchor (/pending 457), and it travels
// in the document's Info dictionary, which every pdfcpu write carries through unchanged. So every route that
// rewrites a document the server holds must keep the flag beside what it was placed beside, one of four ways:
//
//   - keepsGeometry: the rewrite moves nothing the page draws. Each rewrite named is driven over flagged
//     documents — a turned, cropped page among them — by `pdfops`'s
//     TestEveryRewriteOfAHeldDocumentKeepsWhatAFlagWasPlacedBeside.
//   - carriesFlags: the rewrite moves text and moves the flags with it (the reflow door, `pdfops/anchormove.go`).
//   - flagsStripped: the route rewrites bytes the client POSTS, and the client posts `bakedBytes`, which strips
//     NibFlags from a flagged document before anything else sees it (web/app.js).
//   - installsFile: the bytes come from the file on disk, whose flags and content arrived together.
//
// A route that rewrites a held document and is in none of these is a route that can leave a "sign here" flag
// over whatever the page now draws there; an anchor in the flag format is what it would need, and that is a
// format change to a document-travelling blob (an ADR), not a row here.
type flagRoute struct {
	class   string
	calls   []string // what the handler calls to produce the bytes it commits, as selector expressions
	keepers []string // the names pdfops's geometry census drives, for keepsGeometry
}

var flagRoutes = map[string]flagRoute{
	"handleOCR":           {"keepsGeometry", []string{"pdfops.TagOCRLayer", "pdfops.SetLang"}, []string{"TagOCRLayer"}},
	"handleSanitize":      {"keepsGeometry", []string{"pdfops.StripActive", "pdfops.RemoveFilesAndMedia", "pdfops.StripMetadata"}, []string{"StripActive", "RemoveFilesAndMedia", "StripMetadata"}},
	"handleDecrypt":       {"keepsGeometry", []string{"pdfops.RemovePassword"}, []string{"RemovePassword"}},
	"handleAttachmentAdd": {"keepsGeometry", []string{"pdfops.AddAttachment"}, []string{"AddAttachment"}},
	"handleTagsCommit":    {"keepsGeometry", []string{"tagwrite.Commit"}, []string{"CommitTags"}},
	"handleTagsEdit":      {"keepsGeometry", []string{"tagwrite.Edit"}, []string{"EditStructure"}},
	"handleReflow":        {"carriesFlags", []string{"pdfops.ReflowParagraph"}, nil},
	"handlePages":         {"flagsStripped", []string{"formFileBytes"}, nil},
	"handleOutlineSet":    {"flagsStripped", []string{"formFileBytes"}, nil},
	"handleReload":        {"installsFile", nil, nil},
}

// TestEveryRewriteOfAHeldDocumentIsClassifiedForItsFlags is the census: every function in this package that
// commits a rewrite calls commitMutation, and each must have a row above — a new route without one fails here
// — and each row must still describe its handler.
func TestEveryRewriteOfAHeldDocumentIsClassifiedForItsFlags(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := map[string]map[string]bool{} // handler -> the selector calls in its body
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == "commitMutation" {
				continue
			}
			calls := map[string]bool{}
			commits := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					if f.Sel.Name == "commitMutation" {
						commits = true
					}
					if x, ok := f.X.(*ast.Ident); ok {
						calls[x.Name+"."+f.Sel.Name] = true
					}
				case *ast.Ident:
					calls[f.Name] = true
				}
				return true
			})
			if commits {
				found[fn.Name.Name] = calls
			}
		}
	}
	var unclassified []string
	for h := range found {
		if _, ok := flagRoutes[h]; !ok {
			unclassified = append(unclassified, h)
		}
	}
	sort.Strings(unclassified)
	for _, h := range unclassified {
		t.Errorf("%s commits a rewrite and has no flagRoutes row: say how a signing flag keeps its place through it", h)
	}
	keepers, err := os.ReadFile("../pdfops/flaggeometry_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for h, row := range flagRoutes {
		calls, ok := found[h]
		if !ok {
			t.Errorf("flagRoutes names %s, which no longer commits a rewrite", h)
			continue
		}
		for _, c := range row.calls {
			if !calls[c] {
				t.Errorf("%s no longer calls %s, so its row (%s) describes another handler", h, c, row.class)
			}
		}
		for _, k := range row.keepers {
			if !strings.Contains(string(keepers), "\t\""+k+"\":") {
				t.Errorf("%s is classified keepsGeometry through %s, which pdfops's geometry census does not drive", h, k)
			}
		}
	}
	// flagsStripped rests on the client: the bake strips NibFlags from a flagged document.
	app, err := os.ReadFile("../../web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "if (owner.docHadFlags) {\n    try { out = await embedFlags(out, null, docId); }") {
		t.Errorf("bakedBytes no longer strips NibFlags from a flagged document, so the flagsStripped rows are false")
	}
	if len(found) < 8 {
		t.Fatalf("census saw %d committing handlers; it is not reading the package", len(found))
	}
}
