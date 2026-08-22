package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryCandidateSourceIsNamedAndRouted is ADR-009's guard for the candidate-source list.
//
// # What went wrong, so the guard is not read as ceremony
//
// There were THREE lists of candidate sources and they disagreed. The constants
// (`discover.go`), `dropReport`'s walk (`lan.go`) and `TestTheTwoLawFiguresBindTheRace`'s
// fixture each carried their own, and P05.S04 added `sourceDHT` to the first only.
//
// The consequence was not cosmetic. A race flooded from the meeting point counted its drops
// per source and then reported **"source unknown"**, because the reporter's list had two
// entries — on the one path where the flooding source is attacker-supplied (D6), which is the
// case `raceFailure`'s own doc says the split exists for. Meanwhile the test's list kept the
// global cap unreachable, and the test had *written down in a comment* that S04 would make it
// reachable and that someone should come back. Nobody did.
//
// So this checks the ROUTING, not the agreement of the copies: eight copies checked for
// agreement say nothing about a ninth site added without one (ADR-009).
func TestEveryCandidateSourceIsNamedAndRouted(t *testing.T) {
	// --- half one: every source names itself, distinctly, with no silent default ---
	all := allCandidateSources()
	if len(all) == 0 {
		t.Fatal("setup: allCandidateSources() is empty, so every assertion below is vacuous")
	}
	seen := map[string]candidateSource{}
	for _, src := range all {
		name := src.String()
		if name == "" {
			t.Errorf("source %d names itself as the empty string", uint8(src))
			continue
		}
		if strings.Contains(name, "unnamed source") {
			t.Errorf("source %d has no case in String() and falls through to the unnamed "+
				"form; it would reach a user's failure sentence with no name", uint8(src))
		}
		if prev, dup := seen[name]; dup {
			t.Errorf("sources %d and %d both name themselves %q — a failure sentence naming "+
				"one of them cannot say which", uint8(prev), uint8(src), name)
		}
		seen[name] = src
	}

	// **The list covers exactly the declared constants.** They are `iota`, so the value one
	// past the end must have no name of its own; if it does, a constant was added and this
	// list was not extended — which is the defect, one instance later.
	next := candidateSource(len(all))
	if !strings.Contains(next.String(), "unnamed source") {
		t.Errorf("candidateSource(%d) names itself %q, so a source exists that "+
			"allCandidateSources() does not list. Every site that walks the sources reads "+
			"that list, so the new one is silently excluded from all of them", len(all), next)
	}

	// --- half two: the list has exactly one door ---
	//
	// A `[]candidateSource{...}` literal anywhere but inside `allCandidateSources` is a
	// second list, which is what this whole test is about. Test files are scanned too: one of
	// the two stale copies WAS a test, and a guard that exempts tests would have missed it.
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sites []string
	var scanned int
	for _, f := range files {
		src, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		af, perr := parser.ParseFile(fset, f, src, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		scanned++
		for _, d := range af.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				at, ok := cl.Type.(*ast.ArrayType)
				if !ok {
					return true
				}
				id, ok := at.Elt.(*ast.Ident)
				if !ok || id.Name != "candidateSource" {
					return true
				}
				sites = append(sites, f+":"+fn.Name.Name)
				return true
			})
		}
	}
	// STIMULUS: the scan really read this package. A glob that matched nothing would report
	// zero sites and read as a pass — the shape this file exists to refuse.
	if scanned == 0 {
		t.Fatal("setup: the scan parsed no files, so it could not have found a site")
	}
	if len(sites) == 0 {
		t.Fatalf("setup: found no []candidateSource literal at all across %d files — the scan "+
			"is looking for the wrong node, not proving there is one door", scanned)
	}
	if len(sites) != 1 || !strings.HasSuffix(sites[0], ":allCandidateSources") {
		t.Errorf("a []candidateSource list is built at %v; it must be built only in "+
			"allCandidateSources (ADR-009). A second list is how sourceDHT came to be counted "+
			"by the racer and named by nothing", sites)
	}
}
