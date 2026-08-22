package p2p

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// protocolMethods is the termination protocol, by name.
//
// These five are what `tlsListener` and `quicListener` each carried a near-verbatim copy of:
// the idempotent `Close` that closes `done` before the transport beneath it, the `start` that
// launches the loop once, the `Accept` that selects on `ready` and `done`, and the
// `setCloseErr`/`closeErr` pair whose contract is that Accept's terminal error ALWAYS wraps
// net.ErrClosed.
var protocolMethods = []string{"Accept", "Close", "closeErr", "setCloseErr", "start"}

// TestBothListenersRunOneTerminationProtocol is ADR-009's guard for this rule, and it checks
// the DOOR rather than the text.
//
// # Why a source guard and not a behavioural one
//
// `TestTheListenerTerminatesThroughExactlyOneDoor` already runs both listeners through the
// protocol's observable contract, and it is the reason the second copy was survivable. But a
// behavioural test over a table of two says nothing about a THIRD listener added without an
// entry — ADR-009's own finding, that eight copies checked for agreement say nothing about a
// ninth site added without one. This asks the structural question instead: does every type in
// this package that presents itself as a Listener get the protocol from the one place that
// implements it?
//
// A copy re-declared on a listener would satisfy every behavioural test in the suite on the
// day it was written. That is exactly how the duplication this replaced came about: P05.S02
// gave `quicListener` the protocol by copying `tlsListener`, and it was correct — and then it
// was one edit away from not being.
func TestBothListenersRunOneTerminationProtocol(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// embeds[type] = true if it embeds listenerCore; owns[type] = methods it declares itself.
	embeds := map[string]bool{}
	owns := map[string][]string{}
	// listeners are the types that present as a Listener, discovered by the one method the
	// interface has that nothing else here declares. DISCOVERED and not listed, because a
	// listed population cannot see the listener somebody adds next.
	listeners := map[string]bool{}
	coreOwns := map[string]bool{}

	parsed := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, perr := parser.ParseFile(fset, f, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		parsed++
		ast.Inspect(af, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.TypeSpec:
				st, ok := d.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, fld := range st.Fields.List {
					if len(fld.Names) != 0 {
						continue // named field, not an embed
					}
					if id, ok := fld.Type.(*ast.Ident); ok && id.Name == "listenerCore" {
						embeds[d.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) != 1 {
					return true
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				id, ok := recv.(*ast.Ident)
				if !ok {
					return true
				}
				if id.Name == "listenerCore" {
					coreOwns[d.Name.Name] = true
					return true
				}
				owns[id.Name] = append(owns[id.Name], d.Name.Name)
				// `Transport() string` is the Listener interface's discriminator: Addr is
				// net.Listener's too and Accept now comes from the core, so this is the one
				// method a listener must declare for itself.
				if d.Name.Name == "Transport" {
					listeners[id.Name] = true
				}
			}
			return true
		})
	}

	// STIMULUS, both halves. Without these the assertions below are equally true of a glob
	// that read nothing and of a package with no listeners in it at all.
	if parsed < 3 {
		t.Fatalf("parsed %d non-test files in internal/p2p — the glob is not seeing the "+
			"package, so everything below passes on almost nothing", parsed)
	}
	if len(listeners) < 2 {
		t.Fatalf("found %d listener types (%v); this package has at least tlsListener and "+
			"quicListener, so the discovery is broken and the checks below are vacuous",
			len(listeners), keys(listeners))
	}

	// The door exists and implements the whole protocol. A core missing a method would let
	// the per-listener check below pass by the name simply being absent everywhere.
	for _, m := range protocolMethods {
		if !coreOwns[m] {
			t.Errorf("listenerCore does not declare %s, so the check that no listener "+
				"declares it is satisfied by the method not existing at all", m)
		}
	}

	for lt := range listeners {
		if !embeds[lt] {
			t.Errorf("%s presents as a Listener but does not embed listenerCore — it is "+
				"carrying its own copy of the termination protocol, which is the duplication "+
				"ADR-009 forbids and which every behavioural test in this suite would pass on "+
				"the day it was written", lt)
			continue
		}
		for _, m := range owns[lt] {
			for _, p := range protocolMethods {
				if m == p {
					t.Errorf("%s declares %s itself, shadowing listenerCore's. A shadowed "+
						"method is a second copy of the rule: it can drift from the core's "+
						"without one test changing, because the tests call it through the "+
						"interface either way.", lt, m)
				}
			}
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
