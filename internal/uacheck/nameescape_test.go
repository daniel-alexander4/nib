package uacheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// /pending 782: a name in a content stream is read through ONE door (`nameKey`), which decodes its `#xx` escapes.
// Six sites — a BMC/BDC tag, a named `/Properties` list, an inline property list's keys, and the `Do` and `scn`
// resource lookups — compared the raw bytes while `Tf` and `gs` decoded, so `/Artif#61ct` was not an artifact and
// `/X#30 Do` drew nothing. The page-level `Do`, `scn` and named `/Properties` cases needed a second fix: pdfcpu's
// per-page resource step pruned the resource they name, because its own content scan does not decode either
// (`checkerConfig`). Measured on veraPDF 1.30.2 (2026-10-03), every row's `want` is veraPDF's: Fail where it fails,
// and Pass where it records no failure.

// nameEscapeCase is one fixture: a clause, the verdict veraPDF gives it, and the document.
type nameEscapeCase struct {
	name   string
	clause string
	want   Verdict
	pdf    []byte
}

// refFormDrawnAs is one page drawing, under the name `drawn`, the form XObject bound as `/X0` — which carries a
// `/Ref`, so a form that is reached FAILS 7.20 t1.
func refFormDrawnAs(drawn string) []byte {
	content := drawn + " Do"
	return buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X0 10 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		7:  "<< /Type /StructTreeRoot >>",
		10: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Ref << /F (external.pdf) /Page 0 >> /Length 0 >>\nstream\n\nendstream",
	})
}

func nameEscapeCases() []nameEscapeCase {
	// patternDoc's drawn tiling pattern carrying a bad /Lang, selected by `name` — rebuilt rather than byte-replaced,
	// so the cross-reference offsets stay true (veraPDF refuses a file whose xref is off; pdfcpu repairs it).
	patternAs := func(name string) []byte {
		span := "/Span << /Lang (en_US) >> BDC 0 0 5 5 re f EMC"
		content := "/P << /MCID 0 >> BDC /Pattern cs " + name + " scn 0 0 100 100 re f EMC"
		return buildPDF(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << /Pattern << /P0 20 0 R >> >> /Contents 4 0 R >>",
			4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
			7:  "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R >>",
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
			9:  "<< /Nums [0 [8 0 R]] >>",
			20: fmt.Sprintf("<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> /Length %d >>\nstream\n%s\nendstream", len(span), span),
		})
	}
	return []nameEscapeCase{
		{"a BMC tag spelling Artifact with an escape", "7.1 t3", Pass, pageWithContent("/Artif#61ct BMC 0 0 1 1 re f EMC")},
		{"control: a BMC tag escaping to another name", "7.1 t3", Fail, pageWithContent("/Artif#78ct BMC 0 0 1 1 re f EMC")},
		{"a BDC tag spelling Artifact with an escape", "7.1 t3", Pass, pageWithContent("/Artif#61ct << >> BDC 0 0 1 1 re f EMC")},
		{"an inline /MCID key spelled with an escape", "7.1 t3", Pass,
			langClauseDoc("/Lang (en-US)", "", "/P << /MC#49D 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC", "", false)},
		{"an inline /Lang key spelled with an escape", "7.2 t29", Fail,
			langClauseDoc("/Lang (en-US)", "", langSpanText("<< /L#61ng (en_US) >>"), "", false)},
		{"a named /Properties list spelled with an escape", "7.2 t29", Fail,
			langClauseDoc("/Lang (en-US)", "", langSpanText("/P#4c"), "", false)},
		{"a form XObject drawn by an escaped name", "7.20 t1", Fail, refFormDrawnAs("/X#30")},
		{"control: the same form drawn by its plain name", "7.20 t1", Fail, refFormDrawnAs("/X0")},
		{"a tiling pattern selected by an escaped name", "7.2 t29", Fail, patternAs("/P#30")},
		{"control: the same pattern selected by its plain name", "7.2 t29", Fail, patternAs("/P0")},
	}
}

// TestAnEscapedNameInAContentStreamIsTheNameItSpells is the item's six sites, each a document whose verdict turns
// on the escape being decoded, against the verdict veraPDF 1.30.2 gives it.
func TestAnEscapedNameInAContentStreamIsTheNameItSpells(t *testing.T) {
	for _, c := range nameEscapeCases() {
		if got := verdictOf(t, c.pdf, c.clause); got.Verdict != c.want {
			t.Errorf("%s: %s reports %v (%s at %s), want %v — veraPDF decodes a name's #xx escapes", c.name, c.clause,
				got.Verdict, got.Why, got.Where, c.want)
		}
	}
}

// TestNameEscapeFixturesForTheOracle writes the fixtures above to $NIB_NAMEESCAPE_OUT, for measuring against
// veraPDF by hand; it does nothing otherwise.
func TestNameEscapeFixturesForTheOracle(t *testing.T) {
	dir := os.Getenv("NIB_NAMEESCAPE_OUT")
	if dir == "" {
		t.Skip("NIB_NAMEESCAPE_OUT is not set")
	}
	for i, c := range nameEscapeCases() {
		name := fmt.Sprintf("%02d-%s-%s.pdf", i, strings.ReplaceAll(c.clause, " ", ""), strings.ReplaceAll(c.name, " ", "_"))
		if err := os.WriteFile(filepath.Join(dir, strings.ReplaceAll(name, "/", "")), c.pdf, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTheContentWalkReadsANameThroughOneDoor is ADR-009's guard for `nameKey`: in the content walk, no name token
// is cut at its slash and nothing calls `fontcode.Name` except the door. A seventh site written as `name[1:]` is
// the shape the six above had, and it fails here rather than on the document that spells its name with an escape.
func TestTheContentWalkReadsANameThroughOneDoor(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "content.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name == "nameKey" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SliceExpr:
				if lit, ok := n.Low.(*ast.BasicLit); ok && lit.Value == "1" && n.High == nil {
					t.Errorf("%s: %s cuts a value at its first byte — a name is read through nameKey, which decodes "+
						"its #xx escapes", fset.Position(n.Pos()), fn.Name.Name)
				}
			case *ast.SelectorExpr:
				if x, ok := n.X.(*ast.Ident); ok && x.Name == "fontcode" && n.Sel.Name == "Name" {
					t.Errorf("%s: %s calls fontcode.Name directly — read the token through nameKey", fset.Position(n.Pos()), fn.Name.Name)
				}
			}
			return true
		})
	}
}
