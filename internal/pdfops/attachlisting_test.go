package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// externalFileDoc carries one embedded-files entry whose filespec has no /EF — an ordinary reference to an
// EXTERNAL file (ISO 32000-1 §7.11.3). pdfcpu's ListAttachments nil-dereferences on it (`model/attach.go:155`).
func externalFileDoc() []byte {
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [(contract.docx) 4 0 R] >> >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
		4: "<< /Type /Filespec /F (contract.docx) /UF (contract.docx) >>",
	})
}

// TestAttachingToADocumentThatReferencesAnExternalFile — the attach route (`POST /api/attachments`) and `nib
// attach` reach AddAttachment, which asked pdfcpu's ListAttachments for the names already taken. On a document
// whose tree holds an external-file reference that is a nil dereference, not an error, and `fault.Catch` re-panics
// anything that is not a pdfcpu fault — so attaching anything to a legal document crashed the request.
func TestAttachingToADocumentThatReferencesAnExternalFile(t *testing.T) {
	doc := externalFileDoc()
	var out []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("AddAttachment panicked on an external-file filespec: %v", r)
			}
		}()
		var err error
		if out, err = AddAttachment(doc, "notes.txt", []byte("hello")); err != nil {
			t.Fatalf("AddAttachment refused a legal document: %v", err)
		}
	}()
	list, err := Attachments(out)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range list {
		got[a.ID] = true
	}
	if !got["contract.docx"] || !got["notes.txt"] {
		t.Fatalf("after attaching, the list is %+v; want the external reference and the new file", list)
	}
	// The duplicate check still reads the tree it now lists itself: the external reference's name is taken.
	if _, err := AddAttachment(doc, "contract.docx", []byte("x")); err == nil {
		t.Fatal("AddAttachment accepted a name the external reference already holds")
	}
	// Reading the reference fails BY NAME — it has no bytes in the document — rather than panicking.
	if _, _, err := ReadAttachment(out, "contract.docx"); err == nil {
		t.Fatal("ReadAttachment returned bytes for a file the document only references")
	}
}

// pdfcpuAttachmentReaders is every pdfcpu entry point that parses the embedded-files tree's filespecs through
// `fileSpecStreamDictInfo` — and so nil-dereferences on an external-file reference. Context methods are matched by
// name on any receiver; package functions only when the qualifier resolves to the pdfcpu package that holds them.
var (
	pdfcpuAttachmentMethods = map[string]bool{
		"ListAttachments": true, "ExtractAttachments": true, "ExtractAttachment": true,
		"SearchEmbeddedFilesNameTreeNodeByContent": true, "RemoveAttachments": true, "RemoveAttachment": true,
		"AddAttachmentsToInfoDigest": true,
	}
	pdfcpuAttachmentFuncs = map[string]map[string]bool{
		"github.com/pdfcpu/pdfcpu/pkg/api": {
			"Attachments": true, "ExtractAttachments": true, "ExtractAttachmentsRaw": true,
			"ExtractAttachmentsFile": true, "RemoveAttachments": true, "RemoveAttachmentsFile": true,
			"PDFInfo": true,
		},
		"github.com/pdfcpu/pdfcpu/pkg/pdfcpu": {"Info": true},
	}
)

// pdfcpuAttachmentCalls reports every call in src that reaches one of pdfcpu's attachment readers.
func pdfcpuAttachmentCalls(fset *token.FileSet, name string, src any) ([]string, error) {
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	imports := map[string]string{}
	for _, im := range f.Imports {
		path, _ := strconv.Unquote(im.Path.Value)
		local := path[strings.LastIndex(path, "/")+1:]
		if im.Name != nil {
			local = im.Name.Name
		}
		imports[local] = path
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok {
			if path, isPkg := imports[id.Name]; isPkg && id.Obj == nil {
				if pdfcpuAttachmentFuncs[path][sel.Sel.Name] {
					hits = append(hits, fset.Position(call.Pos()).String()+" "+id.Name+"."+sel.Sel.Name)
				}
				return true
			}
		}
		if pdfcpuAttachmentMethods[sel.Sel.Name] {
			hits = append(hits, fset.Position(call.Pos()).String()+" ."+sel.Sel.Name)
		}
		return true
	})
	return hits, nil
}

// TestNothingAsksPdfcpuWhatIsAttached — ADR-009: nib lists the embedded-files tree through ONE door,
// `treeFiles`, because pdfcpu's own readers panic on a legal external-file reference. v1.182.29 took StripActive
// and RemoveFilesAndMedia off ListAttachments and AddAttachment was still on it; this census holds every
// production file in the module (internal/, cmd/, the root) off all of pdfcpu's attachment readers.
func TestNothingAsksPdfcpuWhatIsAttached(t *testing.T) {
	// The stimulus: the matcher sees each shape it exists to refuse, and not nib's own same-named door.
	fset := token.NewFileSet()
	probe := `package p
import (
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"nib/internal/pdfops"
)
func f(ctx any) {
	ctx.ListAttachments()
	api.ExtractAttachmentsRaw(nil, "", nil, nil)
	pdfcpu.Info(nil, "", nil, false)
	pdfops.Attachments(nil)
	pdfops.ExtractAttachment(nil, "")
}`
	hits, err := pdfcpuAttachmentCalls(fset, "probe.go", probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("the matcher found %d calls in the probe, want exactly the 3 pdfcpu ones: %v", len(hits), hits)
	}

	root := filepath.Join("..", "..")
	var files int
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "testdata", ".claude", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files++
		hits, perr := pdfcpuAttachmentCalls(fset, path, nil)
		if perr != nil {
			return perr
		}
		for _, h := range hits {
			t.Errorf("%s: calls pdfcpu's attachment listing, which panics on an external-file filespec; read the tree through treeFiles", h)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 {
		t.Fatalf("census read %d production files; it is not reading the module", files)
	}
}
