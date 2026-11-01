package server

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `/pending 874` (ADR-180) — the route that removes an ended ceremony.

func removeEnded(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ceremonies/ended/remove", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	srv.handleEndedRemove(rec, req)
	return rec
}

func TestTheEndedRemoveRouteRemovesOnlyWhenPrimaryAndOnlyTheNamedCeremony(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv, _ := unlockedServer(t)
	root := defaultOutputDir()
	const a, b = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for _, id := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(root, "ended", id), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "ended", id, "document.pdf"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	has := func(id string) bool { _, err := os.Stat(filepath.Join(root, "ended", id)); return err == nil }

	// Not primary: another Nib holds the instance record. It must not remove, and the folder is still there.
	srv.mu.Lock()
	srv.SetInstanceToken("")
	srv.mu.Unlock()
	if rec := removeEnded(t, srv, `{"ceremony":"`+a+`"}`); rec.Code != http.StatusConflict || !has(a) {
		t.Fatalf("a non-primary Nib answered %d and the folder exists=%v; want 409 and the folder kept", rec.Code, has(a))
	}

	// Primary: the control. Without it the arm above is satisfied by a route that never removes anything.
	srv.mu.Lock()
	srv.SetInstanceToken("this-one")
	srv.mu.Unlock()
	if rec := removeEnded(t, srv, `{"ceremony":"`+a+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("the primary Nib answered %d: %s", rec.Code, rec.Body.String())
	}
	if has(a) || !has(b) {
		t.Errorf("after removing %s: it exists=%v and the other exists=%v; want gone and kept", a[:4], has(a), has(b))
	}

	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"not json":          {`nope`, http.StatusBadRequest},
		"a path, not an id": {`{"ceremony":"../ended/` + b + `"}`, http.StatusBadRequest},
		"an id not ended":   {`{"ceremony":"cccccccccccccccccccccccccccccccc"}`, http.StatusNotFound},
		"already removed":   {`{"ceremony":"` + a + `"}`, http.StatusNotFound},
	} {
		if rec := removeEnded(t, srv, tc.body); rec.Code != tc.want {
			t.Errorf("%s answered %d (%s), want %d", name, rec.Code, strings.TrimSpace(rec.Body.String()), tc.want)
		}
	}
	if !has(b) {
		t.Error("a refused request removed the other ceremony")
	}
}

// ADR-180: `ceremony.RemoveEnded` has ONE production caller, the handler — a second site that deletes a ceremony
// folder is a new decision. Counted over the whole tree's non-test Go files by AST, so a call in another package
// or behind an alias import is seen.
func TestRemoveEndedHasOneCaller(t *testing.T) {
	var callers []string
	err := filepath.Walk(filepath.Join("..", ".."), func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == "node_modules" || info.Name() == ".git" || info.Name() == ".claude") {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if perr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "RemoveEnded" {
					callers = append(callers, filepath.ToSlash(p))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) != 1 || !strings.HasSuffix(callers[0], "internal/server/endedremove.go") {
		t.Errorf("ceremony.RemoveEnded is called from %v; ADR-180 allows internal/server/endedremove.go alone", callers)
	}
}
