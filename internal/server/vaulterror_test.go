package server

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/vault"
)

// TestARefusedVaultWriteSaysWhyOnEveryRoute — /pending 873 part 3 (ADR-154).
//
// The stimulus is a second Nib process having replaced the vault file with one this process never
// opened; every write from this one is then refused by the vault with a sentence of its own. Only
// the key-removal route passed such a sentence on. Each route below answered its fixed "could not
// …" with a 500.
func TestARefusedVaultWriteSaysWhyOnEveryRoute(t *testing.T) {
	s, v := unlockedServer(t)
	other, _ := unlockedServer(t)
	theirs, err := os.ReadFile(vault.Path(other.configDir))
	if err != nil {
		t.Fatal(err)
	}
	// As another process's import does it: no handle on this process's vault.
	if err := vault.Replace(vault.Path(s.configDir), nil, theirs); err != nil {
		t.Fatal(err)
	}

	routes := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		body    string
		path    map[string]string
	}{
		{"profile", s.handleProfileSet, "POST", `{"name":"x"}`, nil},
		{"settings", s.handleSettings, "POST", `{"theme":"dark"}`, nil},
		{"unpin a peer", s.handlePeersRemove, "POST", `{"fingerprint":"` + strings.Repeat("ab", 32) + `"}`, nil},
		{"remove the certificate", s.handleExternalSignerRemove, "POST", ``, nil},
		{"delete an image", s.handleImageDelete, "DELETE", ``, map[string]string{"id": "none"}},
		{"remove a key", s.handleKeysRemove, "POST", `{"pubKey":"ssh-ed25519 AAAA"}`, nil},
		{"the ceremony draft", s.handleCeremonyDraft, "POST", `{"draft":"{}"}`, nil},
	}
	want := vault.ErrChangedElsewhere.Error()
	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, "/x", strings.NewReader(rt.body))
		req.Header.Set("Content-Type", "application/json")
		for k, val := range rt.path {
			req.SetPathValue(k, val)
		}
		rr := httptest.NewRecorder()
		rt.handler(rr, req.WithContext(context.WithValue(req.Context(), vaultCtxKey{}, v)))
		var body map[string]string
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		if rr.Code != http.StatusConflict || body["cause"] != vaultRefusalCause ||
			!strings.Contains(strings.ToLower(body["error"]), strings.ToLower(want)) {
			t.Errorf("%s: a write the vault refused answered %d %s; want 409, cause %q and the "+
				"vault's own sentence (%q)", rt.name, rr.Code, strings.TrimSpace(rr.Body.String()), vaultRefusalCause, want)
		}
	}
}

// TestEveryVaultRefusalIsRecognisedAndNothingElseIs — the recognition itself, each sentinel alone.
func TestEveryVaultRefusalIsRecognisedAndNothingElseIs(t *testing.T) {
	for _, e := range []error{vault.ErrKeyChanged, vault.ErrChangedElsewhere, vault.ErrBusy, vault.ErrReplaced} {
		// As a handler that words the failure itself hands it over, wrapped once on the way.
		rr := httptest.NewRecorder()
		vaultWriteFailed(rr, fmt.Errorf("pin: %w", e), http.StatusInternalServerError, "could not pin peer")
		if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), vaultRefusalCause) ||
			strings.Contains(rr.Body.String(), "could not pin peer") {
			t.Errorf("%v through vaultWriteFailed = %d %s; want 409 with the cause and the vault's sentence", e, rr.Code, rr.Body.String())
		}
		// As a handler that voices the error inside a sentence of its own.
		rr = httptest.NewRecorder()
		httpError(rr, http.StatusInternalServerError, "nothing was convened: "+e.Error())
		if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "nothing was convened") {
			t.Errorf("%v inside a handler's sentence = %d %s; want 409 with the sentence whole", e, rr.Code, rr.Body.String())
		}
	}
	rr := httptest.NewRecorder()
	vaultWriteFailed(rr, os.ErrPermission, http.StatusInternalServerError, "could not pin peer")
	if rr.Code != http.StatusInternalServerError || strings.Contains(rr.Body.String(), "cause") ||
		!strings.Contains(rr.Body.String(), "could not pin peer") {
		t.Errorf("an ordinary failed write = %d %s; want the handler's own 500 and words, no cause", rr.Code, rr.Body.String())
	}
}

// TestNoHandlerWordsAFailedVaultWriteWithoutTheVaultsSentence — the routing (ADR-009).
//
// A call to a vault mutator whose error a handler answers must answer it through vaultWriteFailed,
// or with the error's own text in the message (which httpError recognises). A bare
// `httpError(w, 500, "could not …")` in that position drops the vault's sentence, and is what
// twenty-odd routes did. The mutators are read from the vault package: every exported method that
// begins a write.
func TestNoHandlerWordsAFailedVaultWriteWithoutTheVaultsSentence(t *testing.T) {
	fset := token.NewFileSet()
	vf, err := parser.ParseFile(fset, filepath.Join("..", "vault", "vault.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	mutators := map[string]bool{}
	for _, d := range vf.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil || fd.Recv == nil || !fd.Name.IsExported() {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "beginWrite" || sel.Sel.Name == "addPinned") {
					mutators[fd.Name.Name] = true
				}
			}
			return true
		})
	}
	if len(mutators) < 19 {
		t.Fatalf("read %d vault mutators; there are at least nineteen — the scan has stopped matching", len(mutators))
	}

	callsMutator := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && mutators[sel.Sel.Name] {
					found = true
				}
			}
			return !found
		})
		return found
	}
	// bare reports each httpError in region whose message carries no error's text.
	bare := func(region ast.Node) (lines []int) {
		ast.Inspect(region, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := c.Fun.(*ast.Ident); !ok || id.Name != "httpError" || len(c.Args) != 3 {
				return true
			}
			carries := false
			ast.Inspect(c.Args[2], func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
					carries = true
				}
				return !carries
			})
			if !carries {
				lines = append(lines, fset.Position(c.Pos()).Line)
			}
			return true
		})
		return lines
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sites := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		report := func(region ast.Node) {
			sites++
			for _, line := range bare(region) {
				t.Errorf("%s:%d answers a failed vault write in words of its own and drops the vault's: "+
					"use vaultWriteFailed(w, err, code, msg), so a write another running Nib caused to be "+
					"refused says so", name, line)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch st := n.(type) {
			case *ast.IfStmt:
				// if err := v.M(…); err != nil { … }
				if st.Init != nil && callsMutator(st.Init) {
					report(st.Body)
				}
			case *ast.SwitchStmt:
				// switch err := v.M(…); { … default: … } — the named cases are the vault's
				// validation answers; the default is the failed write.
				if st.Init != nil && callsMutator(st.Init) {
					for _, cc := range st.Body.List {
						if c := cc.(*ast.CaseClause); c.List == nil {
							report(c)
						}
					}
				}
			case *ast.BlockStmt:
				// x, err := v.M(…)  followed by  if err != nil { … }
				for i, s := range st.List {
					as, ok := s.(*ast.AssignStmt)
					if !ok || !callsMutator(as) || i+1 >= len(st.List) {
						continue
					}
					if next, ok := st.List[i+1].(*ast.IfStmt); ok && next.Init == nil {
						report(next.Body)
					}
				}
			}
			return true
		})
	}
	if sites < 12 {
		t.Errorf("the scan judged %d failed-write sites; there are at least twelve — it has stopped matching", sites)
	}
}
