package rendezvous

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/anacrolix/dht/v2"
)

// TestOurServerConfigAnswersEveryFieldTheLibraryDefaults is a guard for a defect class this
// package has now been bitten by TWICE, silently, in production behaviour.
//
// # The class
//
// `dht.NewServer` fills in only a few fields for a caller-supplied `ServerConfig`. Every other
// sensible default lives in `NewDefaultServerConfig`, which caveat 7 forbids us — it opens its
// own socket, and the whole point of the shared endpoint is that it must not. So any field
// that function sets and ours does not is left at its ZERO VALUE, and a zero value is not
// "unset", it is a decision:
//
//   - `Exp` unset meant "expire everything immediately", so our own published record was
//     deleted the first time anyone read it, including us.
//   - `DefaultWant` unset meant `find_node` went out asking for nothing, so a responder
//     answered with the query source's family — v4 seeds, v4 nodes, forever. The routing table
//     could not learn an IPv6 node, which made D8's tier 2 unreachable and
//     `SelfAddress.V6` structurally the zero value. Measured 2026-08-22: with `want` set, one
//     of the three shipped v4 seeds returned eight IPv6 nodes; without it, none did.
//
// Both were found by chasing a symptom. Neither was findable by reading our config, because
// what is wrong with it is what is ABSENT — and absence is exactly what a reader does not see.
//
// # What this checks
//
// The library's own defaults are the population, discovered by reflection rather than listed,
// so a field added by a dependency upgrade is in scope the day it appears. For each, our
// literal must either set it or name it here with a reason. It checks the SOURCE rather than a
// built config because the literal lives inside `Open`, and reaching it would mean opening a
// socket to ask a question about a struct.
func TestOurServerConfigAnswersEveryFieldTheLibraryDefaults(t *testing.T) {
	// Fields we deliberately leave to `NewServer`, each with why.
	exempt := map[string]string{
		"Store": "NewServer defaults it (server.go:212-224) and a memory store is what we want; " +
			"gateQuery refuses inbound writes, so the only items in it are our own.",
		"StartingNodes": "ours are the cached table plus the shipped literals, wired separately " +
			"in Open — D6 forbids the hostname bootstrap GlobalBootstrapAddrs performs.",
	}

	// The population: every field NewDefaultServerConfig actually sets, discovered.
	def := dht.NewDefaultServerConfig()
	zero := dht.ServerConfig{}
	rv, rz := reflect.ValueOf(*def), reflect.ValueOf(zero)
	var defaulted []string
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Type().Field(i)
		if !f.IsExported() {
			continue
		}
		a, b := rv.Field(i), rz.Field(i)
		set := false
		switch a.Kind() {
		case reflect.Func, reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface:
			set = !a.IsNil()
		default:
			// A struct field can contain a slice, which makes it non-comparable and makes
			// reflect.Value.Equal PANIC rather than report false. DeepEqual is the total
			// function here; the panic is a crash, not a finding.
			set = !reflect.DeepEqual(a.Interface(), b.Interface())
		}
		if set {
			defaulted = append(defaulted, f.Name)
		}
	}
	// STIMULUS: reflection really found the library's defaults. If this were empty every
	// assertion below would be vacuous and the guard would report a clean pass forever.
	if len(defaulted) == 0 {
		t.Fatal("setup: NewDefaultServerConfig appears to set no fields, so this guard is " +
			"checking nothing — the library's shape changed and this test must be rewritten")
	}

	// What our literal sets, read from the source.
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "dht.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ours := map[string]bool{}
	var found bool
	ast.Inspect(af, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := cl.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ServerConfig" {
			return true
		}
		found = true
		for _, e := range cl.Elts {
			if kv, ok := e.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok {
					ours[id.Name] = true
				}
			}
		}
		return true
	})
	if !found {
		t.Fatal("setup: no dht.ServerConfig literal found in dht.go — the scan is looking for " +
			"the wrong node, not proving the config is complete")
	}

	for _, name := range defaulted {
		if ours[name] {
			continue
		}
		if why, ok := exempt[name]; ok && why != "" {
			continue
		}
		t.Errorf("NewDefaultServerConfig sets %s and our ServerConfig does not, with no reason "+
			"recorded. It is therefore at its ZERO value, which is a decision and not an "+
			"absence — Exp cost us our own published record and DefaultWant cost us the whole "+
			"IPv6 tier, both silently. Set it, or add it to `exempt` with why.", name)
	}
	// An exemption for a field the library no longer defaults quietly covers the next field
	// of that name — the same hole `unreadKnown` polices one level out.
	for name := range exempt {
		var still bool
		for _, d := range defaulted {
			if d == name {
				still = true
				break
			}
		}
		if !still {
			t.Errorf("%s is exempt here but NewDefaultServerConfig no longer sets it", name)
		}
	}
	t.Logf("library defaults %d field(s): %v", len(defaulted), defaulted)
}
