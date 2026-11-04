package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execSites returns, for one source file, how many exec.Command / exec.CommandContext calls it makes and
// whether it ever assigns a `.Env` field (the environment a converter is handed).
func execSites(t *testing.T, name, src string) (calls int, setsEnv bool) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "exec" && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
					calls++
				}
			}
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "Env" {
					setsEnv = true
				}
			}
		}
		return true
	})
	return
}

func TestTheExecCensusSeesWhatItGuards(t *testing.T) {
	calls, env := execSites(t, "x.go", `package x; import "os/exec"
func f() { c := exec.Command("a"); d := exec.CommandContext(nil, "b"); c.Env = nil; _ = d }`)
	if calls != 2 || !env {
		t.Fatalf("calls=%d env=%v, want 2 and true", calls, env)
	}
	if calls, env := execSites(t, "y.go", `package y; func f() { run("x") }`); calls != 0 || env {
		t.Fatalf("the scan sees an exec site in code that has none: %d %v", calls, env)
	}
}

// TestEveryProcessNibStartsIsDeclared — D33's exec census (ADR-185). Nib starts a handful of programs and
// each is named here with why. A converter — a program that interprets a stranger's file — must also set
// `cmd.Env`: before the allowlist (pdfops.toolEnv) both inherited Nib's whole environment, proxy settings
// and credentials included. A new site fails until it is declared; a declared site that no longer starts
// anything fails too, so the list cannot rot.
func TestEveryProcessNibStartsIsDeclared(t *testing.T) {
	declared := map[string]struct {
		why       string
		converter bool // interprets a document: must set Env
	}{
		"internal/pdfops/office.go":              {"LibreOffice converts an Office document", true},
		"internal/pdfops/pdfa_gs.go":             {"Ghostscript converts to PDF/A", true},
		"internal/pdfops/psimport.go":            {"Ghostscript interprets a PostScript program, and the probe that asks whether it is confined (ADR-198)", true},
		"internal/browser/browser.go":            {"launches the user's own browser window and its handoff", false},
		"internal/browser/openfolder.go":         {"asks the desktop to open a folder (xdg-open / open / explorer)", false},
		"internal/pdfops/convertproc_windows.go": {"`taskkill` ends a converter's process tree on Windows (compiled and argv-tested only)", false},
		"internal/docwrite/wtest/wtest.go":       {"the writers' tests open a written package in LibreOffice, headless, to confirm the XML oracle (ADR-194); test support only, skipped where soffice is absent", true},
		"internal/browser/raise.go":              {"asks the window manager to raise the window", false},
		"internal/cli/watchopen.go":              {"`nib watch` re-launches nib on a file that changed", false},
	}
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "web", "docs", "test", "third_party", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 60 {
		t.Fatalf("setup: only %d source files found — this guard is not walking the repo", len(files))
	}
	seen := map[string]bool{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		calls, env := execSites(t, path, string(b))
		if calls == 0 {
			continue
		}
		key := filepath.ToSlash(path)
		d, ok := declared[key]
		if !ok {
			t.Errorf("%s starts a process (%d site(s)) and is not declared in this census — name it and say why, "+
				"and if it interprets a document, give it pdfops.toolEnv's allowlisted environment", key, calls)
			continue
		}
		seen[key] = true
		if d.converter && !env {
			t.Errorf("%s runs a converter and never sets cmd.Env, so the tool inherits Nib's whole environment", key)
		}
	}
	for k := range declared {
		if !seen[k] {
			t.Errorf("%s is declared as starting a process and starts none; remove the entry", k)
		}
	}
}
