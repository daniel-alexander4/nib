package p2p

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// TestEveryVerificationInitiatorIsAKnownRoleWriter — ADR-028, and this guard exists because its
// absence cost a deterministic tier-4d failure.
//
// # The defect it was written from
//
// The role frame was added to `Initiate` and `SendDocument`'s call sites, on the reasoning that
// those are the two dialing verbs. There is a THIRD — `Carry`, the non-signing convener's baton
// hop — and it initiates the verification exactly as the other two do. So its commitment arrived
// where the far side expected a role byte: the arm read a 32-byte SHA-256 digest as a one-byte
// frame, refused it, re-raced, and the hop failed as `rendezvous-unreachable`. Two-party tier 4
// passed throughout, because the relay is the only shape that uses `Carry`.
//
// **The enumeration was "the functions I knew" rather than "everything with the property", and
// that is the whole lesson.** The property is *runs the verification as initiator* — one grep —
// and it is what this guard enumerates. A fourth initiator added later is caught here rather than
// by a four-party run.
func TestEveryVerificationInitiatorIsAKnownRoleWriter(t *testing.T) {
	// The initiating verbs this build knows, each of which MUST have `WriteRole` at every one of
	// its call sites (`TestEveryDialDeclaresItsRole` in internal/server holds that half).
	//
	// **Adding a name here is the deliberate act**: it is a claim that every caller of that
	// function declares a role first, and the server-side guard is what makes the claim true.
	known := map[string]bool{
		"Initiate":     true,
		"Carry":        true,
		"SendDocument": true,
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	responders := 0
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					id, ok := call.Fun.(*ast.Ident)
					if !ok || id.Name != "runVerification" || len(call.Args) < 2 {
						return true
					}
					lit, ok := call.Args[1].(*ast.Ident)
					if !ok {
						return true
					}
					switch lit.Name {
					case "true":
						found[fn.Name.Name] = true
					case "false":
						responders++
					}
					return true
				})
			}
		}
	}

	// STIMULUS, and both halves. A scan that matched nothing agrees with every assertion below,
	// and one that matched only initiators would not prove it can tell the two apart — which is
	// the distinction the whole guard rests on.
	if len(found) == 0 {
		t.Fatal("the scan found no verification initiators at all — `runVerification` has been " +
			"renamed or its second argument is no longer a literal, and this guard is reading nothing")
	}
	if responders == 0 {
		t.Fatal("the scan found no verification RESPONDERS, so it is not distinguishing the " +
			"initiator argument from the call — every `runVerification` would read as an initiator")
	}

	var unknown []string
	for name := range found {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Errorf("%v runs the verification as INITIATOR and is not a known role writer. Every "+
			"initiating verb must have `p2p.WriteRole` at its call sites, or its first frame — a "+
			"32-byte commitment — arrives where the far side expects a one-byte role and the hop "+
			"fails as an unreachable peer. That is exactly what `Carry` did, and it was found by "+
			"a four-party run rather than by this test. Add it to `known` AND to the call-site "+
			"guard in internal/server.", unknown)
	}
	// And the reverse: a name listed here that no longer initiates is a claim covering nothing.
	for name := range known {
		if !found[name] {
			t.Errorf("%q is listed as a verification initiator and no longer runs one. An entry "+
				"that stops matching reads exactly like a clean run", name)
		}
	}
}
