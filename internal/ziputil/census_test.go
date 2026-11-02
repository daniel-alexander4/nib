package ziputil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The archive census (PLAN-import-export R8, ADR-187, ADR-009): this package is the one door Nib reads or writes a
// zip through, and a caller that opens `archive/zip` itself has its own idea of which names are safe and how far
// an archive may expand. Enumeration source: every non-test .go file outside third_party, parsed (not grepped,
// so a comment or a string that says `zip.NewWriter` is not a caller).

// exempt are the callers that predate the door, each with why it is allowed to stay. The list can only shrink: an
// entry whose file no longer calls the archive is itself a failure.
var exempt = map[string]string{
	"internal/server/form.go":        "writes a bundle of filled forms as a plain deflate zip; no stored-first member, no reader",
	"internal/server/export.go":      "writes the export bundle (one page per file) as a plain deflate zip",
	"internal/pdfops/pdfops.go":      "writes the extracted-attachments / page-images bundle as a plain deflate zip",
	"internal/pdfops/spreadsheet.go": "GridToXLSX: an OOXML package with no stored member; migrates with P03's DOCX writer",
	"internal/testpdf/hostile.go":    "test-fixture generator for hostile archives; it must be able to write what the door refuses",
	"internal/testpdf/docx.go":       "test-fixture generator for a minimal DOCX",
}

// zipCallers returns, per file under root, the archive/zip constructors it calls.
func zipCallers(t *testing.T, root string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if strings.HasPrefix(rel, "third_party") || strings.HasPrefix(rel, "node_modules") || (rel != "." && strings.HasPrefix(filepath.Base(rel), ".")) || rel == "internal/ziputil" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if perr != nil {
			return nil // a file that does not parse is the compiler's to report
		}
		alias := ""
		for _, im := range f.Imports {
			if im.Path.Value == `"archive/zip"` {
				alias = "zip"
				if im.Name != nil {
					alias = im.Name.Name
				}
			}
		}
		if alias == "" {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == alias {
					switch sel.Sel.Name {
					case "NewWriter", "NewReader", "OpenReader":
						out[rel] = append(out[rel], sel.Sel.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEveryArchiveIsReadAndWrittenThroughTheDoor(t *testing.T) {
	root, _ := filepath.Abs("../..")
	got := zipCallers(t, root)
	var rogue []string
	for f := range got {
		if _, ok := exempt[f]; !ok {
			rogue = append(rogue, f)
		}
	}
	sort.Strings(rogue)
	if len(rogue) > 0 {
		t.Errorf("these files open archive/zip themselves; route them through internal/ziputil or name an exemption with a reason: %v", rogue)
	}
	for f, why := range exempt {
		if len(got[f]) == 0 {
			t.Errorf("exemption %s (%s) no longer calls archive/zip: remove it", f, why)
		}
	}
	// No exempt file READS an archive: every reader goes through Open (the limits are the point).
	for f, calls := range got {
		for _, c := range calls {
			if c != "NewWriter" && !strings.HasPrefix(f, "internal/testpdf/") {
				t.Errorf("%s calls zip.%s: archives are READ only through ziputil.Open", f, c)
			}
		}
	}
}

func TestTheArchiveCensusSeesAPlantedCaller(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package pkg\nimport \"archive/zip\"\nvar _ = zip.NewReader\n"
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// A comment or a string naming the constructor is not a caller.
	if err := os.WriteFile(filepath.Join(dir, "pkg", "b.go"), []byte("package pkg\n// zip.NewWriter\nvar s = \"zip.NewWriter\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := zipCallers(t, dir)
	if len(got["pkg/a.go"]) != 1 || len(got["pkg/b.go"]) != 0 {
		t.Fatalf("planted census = %v", got)
	}
}
