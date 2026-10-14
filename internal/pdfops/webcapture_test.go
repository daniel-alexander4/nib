package pdfops

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// /pending 889 part 2: a Web Capture record pdfcpu's validator could not read whole refuses every change, says so
// by name, and goes when asked — taking the whole record, so the change can then be made.

func webCaptureKinds(t *testing.T, pdf []byte) int {
	t.Helper()
	rep, err := Scan(pdf)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range rep.Findings {
		if f.Kind == "webCapture" {
			n++
		}
	}
	return n
}

func TestARefusedWebCaptureRecordIsNamedAndRemovedWhenAsked(t *testing.T) {
	w := testpdf.WholeWebCapture
	w.Refused = true
	doc := testpdf.WebCaptured(w)
	// Stimulus: the record IS marked unread. Without it, nothing below is about this case.
	ctx, err := pdfread.Validated(doc, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := pdfread.UnreadNameTrees(ctx.XRefTable); len(got) != 1 || got[0] != "URLS" {
		t.Fatalf("setup: unread trees %v, want [URLS] — the validator kept the refused leaf, or dropped more", got)
	}

	// A change is refused, naming the record — not attachments, destinations or scripts.
	_, err = Rotate(doc, nil, 90)
	if !errors.Is(err, pdfread.ErrNameTreeLost) {
		t.Fatalf("a rotation answered %v, want ErrNameTreeLost", err)
	}
	if msg := err.Error(); !strings.Contains(msg, pdfread.WebCaptureWords) || strings.Contains(msg, "attachments") {
		t.Errorf("the refusal says %q — it must name the web capture record and nothing it did not lose", msg)
	}

	// The scan lists it; the removal takes the whole record; the change then goes through.
	if webCaptureKinds(t, doc) != 1 {
		t.Errorf("the scan does not list the web capture record")
	}
	out, err := RemoveWebCapture(doc)
	if err != nil {
		t.Fatalf("RemoveWebCapture: %v", err)
	}
	if webCaptureKinds(t, out) != 0 {
		t.Errorf("the scan still lists a web capture record after its removal")
	}
	rctx, err := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := rctx.Catalog()
	if _, ok := root.Find("SpiderInfo"); ok {
		t.Errorf("/SpiderInfo is still in the catalog")
	}
	if names := derefDict(rctx.XRefTable, root["Names"]); names != nil {
		for _, k := range []string{"URLS", "IDS"} {
			if _, ok := names.Find(k); ok {
				t.Errorf("/%s is still in /Names", k)
			}
		}
	}
	_ = eachPage(rctx.XRefTable, root, func(page types.Dict, _ int) {
		for _, k := range []string{"ID", "PZ"} {
			if _, ok := page.Find(k); ok {
				t.Errorf("the page still carries /%s", k)
			}
		}
	})
	if _, err := Rotate(out, nil, 90); err != nil {
		t.Errorf("a rotation after the removal answered %v — the record was removed and the change is still refused", err)
	}
}

// Each part of the record, alone, is what the scan finds and the removal takes.
func TestEveryPartOfAWebCaptureRecordIsSeenAndTaken(t *testing.T) {
	for name, w := range map[string]testpdf.WebCapture{
		"/URLS":                {URLS: true},
		"/IDS":                 {IDS: true},
		"/SpiderInfo":          {SpiderInfo: true},
		"a page's /ID and /PZ": {Page: true},
	} {
		doc := testpdf.WebCaptured(w)
		if webCaptureKinds(t, doc) != 1 {
			t.Errorf("%s alone: the scan does not see the record", name)
			continue
		}
		out, err := RemoveWebCapture(doc)
		if err != nil {
			t.Errorf("%s alone: %v", name, err)
			continue
		}
		if webCaptureKinds(t, out) != 0 {
			t.Errorf("%s alone: the removal left it", name)
		}
	}
	if webCaptureKinds(t, testpdf.WebCaptured(testpdf.WebCapture{})) != 0 {
		t.Error("a document with no record is said to carry one")
	}
}

// ForgetNameTree is ADR-164's removal door, and each caller is a removal the user asked for: exactly three, named.
// A fourth is a new decision, not a new call.
func TestForgetNameTreeHasItsThreeNamedCallers(t *testing.T) {
	want := map[string]bool{"StripActive": true, "removeAllAttachments": true, "RemoveWebCapture": true}
	got := map[string]int{}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "testdata", "third_party":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if perr != nil {
			return perr
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "ForgetNameTree" {
					got[fn.Name.Name]++
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name := range got {
		if !want[name] {
			t.Errorf("%s calls pdfread.ForgetNameTree — a removal of a list Nib could not read is a decision (ADR-164); "+
				"name it here and in the function's comment, or route through an existing one", name)
		}
	}
	for name := range want {
		if got[name] == 0 {
			t.Errorf("%s no longer calls pdfread.ForgetNameTree — update this list and the function's comment", name)
		}
	}
}
