package server

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// postedOp posts `pdf` to one of the two posted-bytes routes with the given fields, straight at the
// handler, and returns the status and the body.
func postedOp(t *testing.T, s *Server, route string, pdf []byte, fields map[string]string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, route, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	switch route {
	case "/api/pages":
		s.handlePages(rec, req)
	case "/api/outline":
		s.handleOutlineSet(rec, req)
	default:
		t.Fatalf("no handler for %s", route)
	}
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, string(body)
}

// TestAPostedOperationBuiltOnAnOlderRevisionIsRefused — /pending 792 and 667 #1, ADR-146.
//
// The page and outline routes commit a PDF the window posts, so the snapshot guard (/pending 646) has
// nothing to compare and two of them built on the same document both landed: the second replaced the
// first, and undo returned to the state before the FIRST. The window now names the revision it built
// on; a stale one is refused as a stale snapshot is, and the document and its history stay as the
// first operation left them.
func TestAPostedOperationBuiltOnAnOlderRevisionIsRefused(t *testing.T) {
	for _, tc := range []struct {
		route  string
		fields map[string]string
	}{
		{"/api/pages", map[string]string{"op": "rotate", "pages": "1", "deg": "90"}},
		{"/api/outline", map[string]string{"outline": `[{"title":"One","page":1,"level":0}]`}},
	} {
		t.Run(tc.route, func(t *testing.T) {
			pdf := threePagePDF(t)
			s := openTestServer(t, pdf)
			doc := s.activeDoc()
			with := func(rev string) map[string]string {
				f := map[string]string{}
				for k, v := range tc.fields {
					f[k] = v
				}
				if rev != "-" {
					f["baseRev"] = rev
				}
				return f
			}

			code, body := postedOp(t, s, tc.route, pdf, with("0"))
			if code != http.StatusOK {
				t.Fatalf("an operation built on the document as opened answered %d: %s", code, body)
			}
			var meta docResponse
			if err := json.Unmarshal([]byte(body), &meta); err != nil {
				t.Fatal(err)
			}
			if meta.Rev != 1 {
				t.Fatalf("the answer names revision %d after one commit, want 1 — the window has nothing to echo", meta.Rev)
			}
			first := s.docBytes(doc)

			// A second window, still holding the document as opened.
			code, body = postedOp(t, s, tc.route, pdf, with("0"))
			if code != http.StatusConflict || !strings.Contains(body, errDocChanged.Error()) {
				t.Fatalf("an operation built on revision 0 of a document at revision 1 answered %d %s, "+
					"want 409 and errDocChanged — the first edit is lost and undo cannot bring it back", code, body)
			}
			if !sameSlice(s.docBytes(doc), first) {
				t.Error("the refused operation still replaced the document")
			}
			if len(doc.undo) != 1 {
				t.Errorf("undo holds %d entries after one landed commit; the refused one pushed history", len(doc.undo))
			}

			// The controls: the current revision commits, and so does a request that names none
			// (the CLI and the Go tests — ADR-146's declared gap).
			if code, body = postedOp(t, s, tc.route, first, with("1")); code != http.StatusOK {
				t.Errorf("an operation built on the current revision answered %d: %s", code, body)
			}
			if code, body = postedOp(t, s, tc.route, pdf, with("-")); code != http.StatusOK {
				t.Errorf("an operation naming no revision answered %d: %s", code, body)
			}
			// A revision that is not a number is a client that tried to say and failed.
			undoBefore := len(doc.undo)
			if code, _ = postedOp(t, s, tc.route, pdf, with("one")); code != http.StatusBadRequest {
				t.Errorf("baseRev=one answered %d, want 400", code)
			}
			if len(doc.undo) != undoBefore {
				t.Error("a request with an unreadable revision committed")
			}
		})
	}
}

// TestEveryDoorThatReplacesADocumentsBytesMovesItsRevision — ADR-146. A door that installed bytes and
// left the revision alone would let a window post an operation built on the bytes before it, naming a
// revision the document still reports.
func TestEveryDoorThatReplacesADocumentsBytesMovesItsRevision(t *testing.T) {
	pdf := threePagePDF(t)
	fresh := func() []byte { return append([]byte(nil), pdf...) }
	path := filepath.Join(t.TempDir(), "d.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	doc := newPathDoc(path, pdf)
	doc.ceremony = "c1"
	s.mu.Lock()
	s.registerLocked(doc)
	s.mu.Unlock()

	step := func(name string, do func()) {
		t.Helper()
		before := doc.rev
		do()
		if doc.rev != before+1 {
			t.Errorf("%s moved the revision %d → %d, want +1", name, before, doc.rev)
		}
		if got := s.docResponse(doc).Rev; got != doc.rev {
			t.Errorf("after %s the answer names revision %d, the document is at %d", name, got, doc.rev)
		}
	}
	step("commitMutation", func() {
		if err := s.commitMutation(doc, snapshotBase(s.docBytes(doc)), fresh(), false); err != nil {
			t.Fatal(err)
		}
	})
	step("undo", func() { s.handleUndo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/undo", nil)) })
	step("redo", func() { s.handleRedo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/redo", nil)) })
	step("save", func() {
		rec := httptest.NewRecorder()
		s.handleSave(rec, httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(fresh())))
		if rec.Code != http.StatusOK {
			t.Fatalf("save: %d %s", rec.Code, rec.Body)
		}
	})
	step("installCeremonyResult", func() {
		if _, err := s.installCeremonyResult("c1", "d.pdf", fresh()); err != nil {
			t.Fatal(err)
		}
	})
	step("commitBarrier", func() {
		if err := s.commitBarrier(doc, fresh(), false); err != nil {
			t.Fatal(err)
		}
	})
	// Undo with nothing to undo installs nothing, so it names the same revision.
	before := doc.rev
	s.handleUndo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/undo", nil))
	if doc.rev != before {
		t.Errorf("an undo with no history moved the revision %d → %d", before, doc.rev)
	}
}

// TestEveryInstallOfDocumentBytesBumpsTheRevision is the census behind ADR-146: `document.install` is
// the ONE assignment to a `.data` field in this package's production code, and it moves the revision.
// A sixth door that wrote `doc.data = …` itself would pass every behaviour test above and leave the
// window's echo naming bytes the document no longer holds.
func TestEveryInstallOfDocumentBytesBumpsTheRevision(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	writes, calls, bumps := 0, 0, false
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
			if !ok || fn.Body == nil {
				continue
			}
			door := fn.Name.Name == "install" && fn.Recv != nil
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range v.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if !ok || sel.Sel.Name != "data" {
							continue
						}
						writes++
						if !door {
							pos := fset.Position(v.Pos())
							t.Errorf("%s:%d (%s) assigns a `.data` field itself — a document's bytes are "+
								"replaced through document.install, which moves the revision a window echoes (ADR-146)",
								pos.Filename, pos.Line, fn.Name.Name)
						}
					}
				case *ast.IncDecStmt:
					if sel, ok := v.X.(*ast.SelectorExpr); ok && door && sel.Sel.Name == "rev" && v.Tok == token.INC {
						bumps = true
					}
				case *ast.CallExpr:
					if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "install" && len(v.Args) == 2 {
						calls++
					}
				}
				return true
			})
		}
	}
	if writes != 1 {
		t.Errorf("found %d assignments to a `.data` field, want exactly the one in document.install", writes)
	}
	if !bumps {
		t.Error("document.install does not increment rev — every install would name the revision before it")
	}
	if calls < 5 {
		t.Errorf("found %d calls of document.install, expected at least 5 (two commit doors, the history "+
			"step, the save, the ceremony install) — this census has gone blind", calls)
	}
}
