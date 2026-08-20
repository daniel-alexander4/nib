package pdfops

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/format"
)

// TestPdfcpuStillEatsAPercent is the STIMULUS for everything below.
//
// stampText exists because of what pdfcpu does, not because of what its doc says. If a
// future pdfcpu stops substituting, the escape becomes unnecessary and these tests would
// otherwise go on passing while asserting nothing. Measured here, against the library, so
// the premise is re-checked on every run rather than trusted from a comment.
func TestPdfcpuStillEatsAPercent(t *testing.T) {
	for _, c := range []struct{ in, want, why string }{
		{"CONFIDENTIAL 100%", "CONFIDENTIAL 100", "a bare % is dropped"},
		{"50%P", "503", "%P substitutes the page count"},
		{"%", "", "reduces to empty — pdfcpu then computes a nil bounding box and panics"},
	} {
		got, _ := format.Text(c.in, "", 1, 3)
		if got != c.want {
			t.Errorf("pdfcpu no longer %s: format.Text(%q) = %q, want %q — stampText's "+
				"premise has changed and the tests below now assert nothing",
				c.why, c.in, got, c.want)
		}
	}
	// The one that makes this a signing defect rather than a rendering one.
	if v, _ := format.Text("%v", "", 1, 3); !strings.Contains(v, "v0.") {
		t.Errorf("format.Text(%q) = %q — expected the pdfcpu VERSION string; if this "+
			"stopped being true the %%v case is no longer a document that certifies a "+
			"dependency's version where the signer typed two characters", "%v", v)
	}
}

// TestStampTextKeepsWhatTheUserTyped — or refuses, and never quietly alters.
func TestStampTextKeepsWhatTheUserTyped(t *testing.T) {
	// Cases that MUST survive byte-identical through pdfcpu.
	for _, in := range []string{"CONFIDENTIAL 100%", "Paid 100%", "%x", "50 % off", "%", "a % b"} {
		esc, err := stampText(in)
		if err != nil {
			t.Errorf("stampText(%q) refused text it can represent: %v", in, err)
			continue
		}
		// The round trip THROUGH pdfcpu is the assertion, not the escaping itself:
		// asserting esc == doubled(in) would test our own ReplaceAll and nothing else.
		got, _ := format.Text(esc, "", 1, 3)
		if got != in {
			t.Errorf("%q survived stampText as %q and pdfcpu rendered it %q", in, esc, got)
		}
	}

	// Cases pdfcpu CANNOT represent. These must be refused, never altered — measured:
	// no run of % escapes a placeholder letter, %%%%P renders as %%%3.
	// Runs of two or more are refused for the same reason: the doubling advances one
	// character, so "a%%b" escapes to "a%%%%b" and renders "a%%%b".
	for _, in := range []string{"50%P", "%v", "%p", "%t", "done %P of %P", "a%%b", "100%%"} {
		if _, err := stampText(in); !errors.Is(err, ErrStampTextUnrepresentable) {
			t.Errorf("stampText(%q) did not refuse — err = %v. This is the case that "+
				"bakes a page count or a library version into a document the user is "+
				"about to sign", in, err)
		}
	}

	// Nothing to draw is a skip, not an error: empty text is the input that panics.
	for _, s := range []string{"", "   ", "\t\n"} {
		esc, err := stampText(s)
		if err != nil || esc != "" {
			t.Errorf("stampText(%q) = (%q, %v), want (\"\", nil) so the caller skips", s, esc, err)
		}
	}
}

// TestEveryStampSiteGoesThroughStampText.
//
// P03's recorded lesson, applied: "a guard tested a predicate and not that anything called
// it". The helper can be perfect while a fifth call site passes raw text — which is exactly
// the state this package was in, with FOUR sites and THREE different policies (escape,
// strip, nothing). By shape, not by name, so a rename cannot disarm it.
func TestEveryStampSiteGoesThroughStampText(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sites int
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "TextWatermark" || len(call.Args) == 0 {
				return true
			}
			sites++
			// The first argument must be an identifier that stampText produced. Anything
			// else — a field selector, a literal, a Sprintf — is raw text.
			id, ok := call.Args[0].(*ast.Ident)
			if !ok || id.Obj == nil {
				t.Errorf("%s:%d: TextWatermark's text argument is not a local bound from "+
					"stampText — a %% in it is dropped, substituted, or panics, and on the "+
					"finalize path the result is SIGNED",
					f, fset.Position(call.Args[0].Pos()).Line)
				return true
			}
			as, ok := id.Obj.Decl.(*ast.AssignStmt)
			if !ok || len(as.Rhs) != 1 {
				t.Errorf("%s:%d: %s is not assigned from a call", f,
					fset.Position(call.Pos()).Line, id.Name)
				return true
			}
			rhs, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				t.Errorf("%s:%d: %s is not assigned from a call", f,
					fset.Position(call.Pos()).Line, id.Name)
				return true
			}
			fn, ok := rhs.Fun.(*ast.Ident)
			if !ok || fn.Name != "stampText" {
				t.Errorf("%s:%d: TextWatermark's text comes from %s, not stampText", f,
					fset.Position(call.Pos()).Line, id.Name)
			}
			return true
		})
	}
	// The floor. Zero sites means the matcher stopped matching and every assertion above
	// ran over nothing — the package has four stamp sites and has had four for a year.
	if sites < 4 {
		t.Fatalf("found %d TextWatermark call site(s); expected at least 4 (StampFields, "+
			"StampWatermark, StampPageNumbers, StampTextLayer) — the matcher has gone blind",
			sites)
	}
	t.Logf("%d stamp site(s), all through stampText", sites)
}
