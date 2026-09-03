package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// /pending 365 — a GET route that mutates must be behind the door that refuses a cross-site one.
//
// # The defect this exists to catch, which happened
//
// `requireUnlocked` applies the CSRF check and the loopback-origin check to **non-GET methods
// only**. So every GET behind that gate runs on a request from any page in the user's browser —
// an `<img src="http://127.0.0.1:PORT/api/…">` is enough, and it does not even need to read the
// response. Harmless for a pure read; a live vulnerability for a handler that writes.
//
// P08.S06 hung the close-out sweep — which **moves ceremony directories and drops vault pins** —
// off `GET /api/ceremonies` behind `requireUnlocked`. P06.S01 found it and moved that route onto
// `requirePublicLoopback`, which refuses `Sec-Fetch-Site: cross-site` before the handler runs.
//
// # The rule, and why the guard checks the DOOR
//
// **A GET handler that contains a write verb is registered behind `requirePublicLoopback`.**
//
// ADR-009: the guard asserts routing through the door, not a list of names somebody keeps in
// agreement. A `declared` map would have to be edited for every legitimate mutating GET, and an
// entry in it is a claim a person typed; the wrapper is a fact the code carries. The exemption and
// the fix are therefore the same edit — which is the point, because the fix is cheap and correct.
//
// # What "write-shaped" means, and the two things it cannot see
//
// A keyword set over the handler's own body: the filesystem verbs, the vault's prune and add
// doors, and the ceremony-state doors this repo actually hung off a GET by accident.
//
//   - **It does not follow calls.** A mutation behind a helper — `s.doTheThing()` whose body
//     writes — is invisible. Matching the call graph is accurate and much more machinery; /pending
//     365 says so in the item that produced this guard. What this catches is the shape that
//     occurred: a write verb typed directly into a handler that answers a GET.
//   - **It can fire on a benign write** — a `MkdirAll` for a cache. That is the cheap direction:
//     the answer is the wrapper, one edit, and a cross-site GET that fills a cache is not a thing
//     anybody wants either.
//
// A behavioural test cannot ask this question at all: no observable distinguishes "this handler is
// read-only" from "this handler happened not to write on the input the test supplied". The
// property is about what the code CAN do, which only the source says.
func TestEveryMutatingGETIsBehindTheLoopbackDoor(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("internal/server/*.go")
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the package is really being read. A glob that matched nothing reports a clean bill
	// for the wrong reason — the vacuity this repo keeps finding in source scans.
	if len(files) < 20 {
		t.Fatalf("setup: only %d files matched internal/server/*.go — this guard is not reading "+
			"the package", len(files))
	}

	type reg struct{ route, gate string }
	getHandlers := map[string]reg{}      // handler method name -> its GET registration
	bodies := map[string]*ast.FuncDecl{} // every method body, so the names above resolve

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("%s: %v", path, perr)
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Name != nil {
				bodies[fd.Name.Name] = fd
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || !strings.HasPrefix(lit.Value, `"GET `) {
				return true
			}
			name, gate := unwrap(call.Args[1])
			if name != "" {
				getHandlers[name] = reg{route: strings.Trim(lit.Value, `"`), gate: gate}
			}
			return true
		})
	}

	// STIMULUS: the scan resolved the routes. A pass that resolved none would report a clean bill
	// over an empty set — the same vacuity one level in.
	if len(getHandlers) < 15 {
		t.Fatalf("setup: resolved only %d GET handlers; the server registers far more, so this "+
			"guard is looking at almost nothing. Did a registration shape change?", len(getHandlers))
	}

	// The verbs. Each is a direct write, and the last group is the doors this repo hung off a GET.
	writes := []string{
		"WriteFile", "WriteDurable", "Rename", "RemoveAll", "MkdirAll", "Remove",
		"PruneCeremonyPeers", "PruneCeremonySecrets", "PruneCeremonyInvitations",
		"AddCeremonyPeer", "AddCeremonySecret", "AddCeremonyInvitation",
		"closeOutEnded", "closeOutCeremony", "CloseOutMirror", "WriteMirror", "WriteMe",
		"markDelivered", "WriteReceipt",
	}

	checked, mutating := 0, 0
	for name, r := range getHandlers {
		fd, ok := bodies[name]
		if !ok || fd.Body == nil {
			continue // a shape this scan cannot resolve; the floor above bounds how many
		}
		checked++
		if r.gate == "requirePublicLoopback" {
			continue // behind the door — a write here is guarded, which is the whole rule
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			for _, w := range writes {
				if sel.Sel.Name != w {
					continue
				}
				mutating++
				t.Errorf("%s answers %s behind %s and calls %s.\n"+
					"requireUnlocked applies CSRF and the origin check to non-GET methods ONLY, "+
					"so this write runs on a request from any page in the user's browser — an "+
					"<img src> is enough. P08.S06 hung a close-out sweep off a GET exactly this "+
					"way. Wrap the registration in requirePublicLoopback, which refuses a "+
					"cross-site request before the handler runs, or make the route a POST.",
					name, r.route, r.gate, w)
			}
			return true
		})
	}
	t.Logf("%d GET handlers resolved, %d bodies walked, %d mutating outside the door",
		len(getHandlers), checked, mutating)
}

// unwrap pulls a handler's method name and the middleware it is registered behind out of a
// registration argument: `s.handleFoo` (no gate), `s.requireUnlocked(s.handleFoo)`,
// `requirePublicLoopback(s.handleFoo)`. The gate returned is the OUTERMOST wrapper, because that
// is the one that decides whether the request reaches the handler at all.
func unwrap(e ast.Expr) (name, gate string) {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name, ""
	case *ast.Ident:
		return v.Name, ""
	case *ast.CallExpr:
		if len(v.Args) != 1 {
			return "", ""
		}
		switch fn := v.Fun.(type) {
		case *ast.Ident:
			gate = fn.Name
		case *ast.SelectorExpr:
			gate = fn.Sel.Name
		}
		inner, _ := unwrap(v.Args[0])
		return inner, gate
	}
	return "", ""
}
