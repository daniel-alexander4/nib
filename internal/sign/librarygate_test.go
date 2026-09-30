package sign

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
)

// ungatedCertification drives `certifiedIn` over a reader opened WITHOUT ADR-041's gate, so the
// corrupt-input walk test still reaches the walk's own recover on documents pdfcpu would refuse.
func ungatedCertification(pdf []byte) (certified bool, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			certified, err = false, errors.New("read pdf: panic")
		}
	}()
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return false, err
	}
	return certifiedIn(r)
}

// certificationOf asks the question the way `SignApproval` does: through the gated reader.
func certificationOf(t *testing.T, pdf []byte) bool {
	t.Helper()
	r, err := libraryReader(pdf)
	if err != nil {
		t.Fatalf("libraryReader: %v", err)
	}
	got, err := certifiedIn(r)
	if err != nil {
		t.Fatalf("certifiedIn: %v", err)
	}
	return got
}

// unterminatedMemberDoc is ADR-041's measured shape: an unsigned one-page document whose object
// stream 4 has a damaged `/Filter` name and holds one member, 5, a literal string left open — which
// `digitorus/pdf` lexes past the stream's end until the process runs out of memory, and which pdfcpu
// refuses (`problem decoding object stream 4`). Nothing the catalog reaches names object 5, so the
// library's reader OPENS it: only a gate in front of it keeps it out.
func unterminatedMemberDoc() []byte {
	doc := rawObjStmDoc(map[int]stmSpec{4: {hdr: "5 0 ", content: "(aaa", n: 1, first: -1, flate: true}},
		map[int][2]int{5: {4, 0}})
	return bytes.Replace(doc, []byte("/Filter/FlateDecode"), []byte("/Filter/FlateDecodX"), 1)
}

// TestNothingIsSignedThatPdfcpuCannotRead — /pending 712 R6-2, /pending 761. ADR-041's rule was
// enforced only inside `Verify`; every signing door (`Sign`, `SignApproval`, `SignExternal`, all
// through `runSign`) and `SignApproval`'s certification walk opened the library's reader on the
// bytes they were handed. The observable is the library itself: it must never be handed the
// document.
func TestNothingIsSignedThatPdfcpuCannotRead(t *testing.T) {
	doc := unterminatedMemberDoc()
	// STIMULUS: pdfcpu refuses the fixture, and the library's reader opens it — so a refusal below
	// is the gate's, and a library call below is a real crossing of the gate's line.
	if err := pdfcpuCanRead(doc); err == nil {
		t.Fatal("STIMULUS: pdfcpu reads the fixture, so it cannot tell a gated signer from an ungated one")
	}
	if _, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc))); err != nil {
		t.Fatalf("STIMULUS: the library's reader refuses the fixture on its own (%v), so no gate is needed to keep it out", err)
	}
	var reached []string
	saved := librarySign
	defer func() { librarySign = saved }()
	cert, key, err := GenerateIdentity("Gate")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"Sign", func() error { _, err := Sign(doc, cert, key, Options{Name: "G", When: time.Now()}); return err }},
		{"SignApproval", func() error {
			_, err := SignApproval(doc, cert, key, Options{Name: "G", When: time.Now()})
			return err
		}},
	} {
		lib := false
		librarySign = func(io.ReadSeeker, io.Writer, *dpdf.Reader, int64, sign.SignData) error {
			lib = true
			return errors.New("stub library")
		}
		err := c.run()
		if lib {
			reached = append(reached, c.name)
		}
		if err == nil || !strings.Contains(err.Error(), "nib cannot read this document") {
			t.Errorf("%s: err = %v, want the readability gate's refusal", c.name, err)
		}
	}
	if len(reached) > 0 {
		t.Errorf("the signing library was handed a document pdfcpu cannot read, via %v — ADR-041's gate "+
			"does not stand in front of the signer, and the library's lexer does not bound this shape", reached)
	}
}

// TestEveryLibraryReaderIsBehindTheGate — /pending 712 R6-2. The rule has one door for the signing
// paths (`libraryReader`), and the census is of the doors, not of their text (ADR-009): every
// `dpdf.NewReader` in this package's production code sits in a function named here, with the
// reason it may.
func TestEveryLibraryReaderIsBehindTheGate(t *testing.T) {
	allowed := map[string]string{
		"libraryReader": "the door: pdfcpu's gate, then the reader, then the lookup-cost ceiling",
		"sweep": "both callers gate first — Verify (pdfcpuRead) and signedAsIntended (pdfcpuCanRead over " +
			"out; runSign's libraryReader over in)",
		"signatureBlobPresent": "UNGATED, declared: /pending 712 R6-2's HasSignatureBlob half is parked on " +
			"cost (a pdfcpu read per call on the save/undo paths)",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "NewReader" {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "dpdf" {
					return true
				}
				seen++
				if _, ok := allowed[fd.Name.Name]; !ok {
					t.Errorf("%s: %s opens digitorus/pdf's reader itself. Open it through libraryReader, "+
						"which runs ADR-041's readability gate first — a recover cannot contain the "+
						"library's out-of-memory — or name the function here with the reason it may",
						fset.Position(call.Pos()), fd.Name.Name)
				}
				return true
			})
		}
	}
	// STIMULUS: the scan found the door itself, so a clean result is not a blind walk.
	if seen < len(allowed) {
		t.Fatalf("found %d dpdf.NewReader call(s), fewer than the %d allowed sites — the scan is not seeing them", seen, len(allowed))
	}
}
