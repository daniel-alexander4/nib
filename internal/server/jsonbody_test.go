package server

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// /pending 419 — a response body written as a MAP or an ANONYMOUS struct is invisible to both
// published-field scans, so a field can be shipped to the client and read by nobody.
//
// `observables_test.go` walks `*ast.TypeSpec` → `*ast.StructType`; `published.test.mjs` matches
// `type\s+(\w+)\s+struct\s*\{`. Neither can see `writeJSON(w, map[string]any{…})` or
// `writeJSON(w, struct{…}{…})` — which is eight response bodies in this package, six map and two
// anonymous. The named-struct surface is genuinely clean (228 tagged fields swept, three
// candidates, all false positives), and that is exactly what makes the HOLE worth closing rather
// than the fields.
//
// This is the same defect class as the scan it complements, one shape out: a pass over named
// structs cannot see a body that has no name.
//
// **What it cannot see, stated rather than implied, and MEASURED against the three fields
// `/pending 419` had already found by hand — it catches one of them.**
//
// The reader test is a bare-name search of `web/app.js`, so a key whose name is used elsewhere
// in the client for something else reads as read. `leave.go`'s `{ceremony, state}` body and
// `delivery.go`'s `ceremony` are both genuinely unread — the client never calls `res.json()` on
// the first and reads only `d.parties` from the second — and both slip through here, because
// `.ceremony` and `.state` appear in app.js for other reasons. Catching those needs the scan to
// know WHICH response a client read belongs to, which means following the fetch call, and that
// is a materially bigger scan than this one.
//
// It is loose in that direction on purpose: a false positive is a guard nobody trusts, and this
// repo has paid for one twice. What it does close is the shape where a key appears nowhere in
// the client at all, which is the case that found `names`.
//
// Also outside it: a key built at runtime rather than written as a literal, and `internal/cli`,
// which neither scan covers and which does not call API routes at all — it reaches `pdfops` as
// Go, never over HTTP.
func TestEveryMapOrAnonymousResponseBodyHasAReader(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	client, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(client)

	type body struct{ where string }
	keys := map[string][]body{} // json key -> where it is published
	bodies := 0
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 2 {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || id.Name != "writeJSON" {
					return true
				}
				lit, ok := call.Args[1].(*ast.CompositeLit)
				if !ok {
					return true
				}
				pos := fset.Position(call.Pos())
				where := fmt.Sprintf("%s:%d", filepath.Base(path), pos.Line)
				switch lit.Type.(type) {
				case *ast.MapType:
					bodies++
					for _, e := range lit.Elts {
						kv, ok := e.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if k, ok := kv.Key.(*ast.BasicLit); ok {
							keys[strings.Trim(k.Value, `"`)] = append(keys[strings.Trim(k.Value, `"`)], body{where})
						}
					}
				case *ast.StructType:
					bodies++
					st := lit.Type.(*ast.StructType)
					for _, f := range st.Fields.List {
						if f.Tag == nil {
							continue
						}
						tag := f.Tag.Value
						i := strings.Index(tag, `json:"`)
						if i < 0 {
							continue
						}
						name := tag[i+6:]
						if j := strings.IndexAny(name, `",`); j >= 0 {
							name = name[:j]
						}
						if name != "" && name != "-" {
							keys[name] = append(keys[name], body{where})
						}
					}
				}
				return true
			})
		}
	}

	// STIMULUS: the walk found the bodies at all. Without this the loop below is satisfied by a
	// scan that discovered nothing — the vacuous green this file's sibling keeps finding.
	if bodies < 6 {
		t.Fatalf("found %d map/anonymous response bodies; this package has eight. The scan is "+
			"broken, so a clean result means nothing.", bodies)
	}

	for key, where := range keys {
		if reason, ok := unreadJSONKeys[key]; ok {
			if reason == "" {
				t.Errorf("%q is exempted with no reason — an unexplained exemption is how a "+
					"genuinely unread field gets parked and forgotten", key)
			}
			continue
		}
		// A client read is `.key` or `['key']` or a destructure; the bare-name test is loose on
		// purpose, because a false positive here is a guard nobody trusts.
		if strings.Contains(js, "."+key) || strings.Contains(js, `"`+key+`"`) || strings.Contains(js, "'"+key+"'") {
			continue
		}
		t.Errorf("the response body at %v publishes %q and web/app.js never reads it. A field "+
			"shipped to a client that does not want it is a claim nobody checks — and a map or "+
			"anonymous body is invisible to both named-struct scans, which is why this one "+
			"exists (/pending 419).", where, key)
	}
}

// unreadJSONKeys are keys published in a map or anonymous body that the client does not read,
// each with the reason it stays. An UNEXPLAINED entry is the failure this guard exists for.
var unreadJSONKeys = map[string]string{}
