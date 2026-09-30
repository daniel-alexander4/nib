package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTheCeremonyListingWaitsForASweepInFlight — /pending 710 R4-2.
//
// `GET /api/ceremonies` runs `closeOutEnded` synchronously before it lists, and ran it OUTSIDE
// `sweepMu`. The unlock sweep's comment says close-out and re-arm "reach opposite conclusions about
// the same ceremony … running them concurrently is a race whose loser is whichever finishes
// second", and the lock is what keeps them apart — so a listing opened while the unlock's re-arm
// ran could move a ceremony's folder out from under a delivery being armed for it, and two
// overlapping listings raced one `CloseOutMirror`.
//
// Driven: a sweep is holding the lock, and the listing must not answer until it lets go.
func TestTheCeremonyListingWaitsForASweepInFlight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")

	srv.sweepMu.Lock() // a sweep in flight
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		srv.handleCeremonies(rec, httptest.NewRequest(http.MethodGet, "/api/ceremonies", nil))
		done <- rec.Code
	}()
	select {
	case <-done:
		srv.sweepMu.Unlock()
		t.Fatal("the ceremony listing ran its close-out while a sweep held the lock — close-out and " +
			"the unlock's re-arm then run concurrently over the same ceremonies, and the re-arm can " +
			"arm a delivery for a ceremony whose folder the listing has just moved")
	case <-time.After(300 * time.Millisecond):
	}
	srv.sweepMu.Unlock()
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("setup: the listing answered %d once the sweep let go, want 200", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the listing never answered after the sweep released the lock")
	}
}

// TestEveryCloseOutRunsInsideTheCeremonySweep — the routing half (ADR-009). Every production call
// to `closeOutEnded` sits in a function literal handed to `runCeremonySweep` or `inCeremonySweep`,
// so a new trigger cannot reach the close-out outside the lock the way the listing did.
func TestEveryCloseOutRunsInsideTheCeremonySweep(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var calls int
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "closeOutEnded" {
				return true
			}
			calls++
			inside := false
			for i := len(stack) - 2; i >= 1 && !inside; i-- {
				if _, ok := stack[i].(*ast.FuncLit); !ok {
					continue
				}
				if outer, ok := stack[i-1].(*ast.CallExpr); ok {
					if fsel, ok := outer.Fun.(*ast.SelectorExpr); ok &&
						(fsel.Sel.Name == "runCeremonySweep" || fsel.Sel.Name == "inCeremonySweep") {
						inside = true
					}
				}
			}
			if !inside {
				t.Errorf("%s: closeOutEnded is called outside the ceremony sweep's lock — it races "+
					"the re-arms that read the same listing", fset.Position(call.Pos()))
			}
			return true
		})
	}
	if calls == 0 {
		t.Fatal("setup: no call to closeOutEnded found — this guard walked nothing")
	}
}
