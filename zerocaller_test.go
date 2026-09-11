package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryExportedFunctionUnderInternalHasAProductionCaller — /pending 445.
//
// # What this closes
//
// This repo has roughly ninety source-scanning guards that police call routing, ordering and
// one-door rules, and until this one **none of them could answer "who calls this".** The gap was
// found behind /pending 441: `BindingMAC` and `CheckBindingMAC` had zero production callers since
// v1.109.47, three commits ever, one of them **a crypto fix landed in code nothing calls** — and
// nothing in the tree could see it. `staticcheck`'s `U1000` cannot either: it does not report
// exported identifiers, which is exactly this case. There is no `.github`, no `.golangci.yml`, no
// `staticcheck.conf`, and neither `Makefile` nor `build.sh` runs any analysis (searched, 2026-09-09).
//
// # Why go/ast and not a name scan, which is the whole reason this was parked
//
// A regex prototype reported **three live functions as dead** — `DisarmSession`, `Records`,
// `SeedSample` — and laundered two dead ones: with comments left in, `BindingMAC` vanished from the
// report because three shipped comments mention it. Both failure modes are the same defect, and
// parsing removes both by construction: `parser.ParseFile` with mode 0 attaches no comments, and
// `ast.Inspect` never visits a comment or the inside of a string literal. Confirmed here rather than
// assumed — `p2p.ExchangeBudget()` and `Server.Ping` each appear in exactly one non-test file
// besides their own, in a COMMENT, and both are reported as uncalled.
//
// # What it counts as a use, and why the bias is deliberate
//
// Every `*ast.Ident` occurrence in every non-test file, except the name a `FuncDecl` declares. That
// over-counts: a local variable sharing a method's name registers as a use, and a method dispatched
// through an interface registers because the interface declares the name. **Both biases point the
// same way — towards silence.** That is the correct direction for this guard, because a check that
// names a LIVE function dead is worse than no check at all; this repo has paid for that shape twice
// (`/pending 386`'s "hypothesis printed as a finding", and the ADR-010 harness configured past its
// own disagreement).
//
// **Declared blind spot: transitive death.** A function called only by a dead one is not reported:
// the scan is one level. `BindingMAC`/`CheckBindingMAC` were the worked example — the first was
// called only by the second, so the pair reported as one row — and they are gone, deleted at
// v1.129.3 when `/pending 442` disposed of them. The blind spot is not; whoever resolves a row
// here should look one level down.
//
// **And this guard is now what holds that disposition.** The exemption row for the pair was
// removed with the code, so re-adding an exported binding helper with no caller goes red here —
// which is the only standing check that the refusal at `internal/ceremony/invitation.go`'s
// caveat-11 block stays a refusal.
func TestEveryExportedFunctionUnderInternalHasAProductionCaller(t *testing.T) {
	// The exemption map, and every row carries the reason it is not a defect. The four prefixes are
	// different claims, not a style: `interface` means the call site cannot exist textually,
	// `test-support` means the function is exported FOR tests, `test-only` means production really
	// does not call it and that is a judgement someone made, and `finding` means it IS the defect,
	// recorded under a pending item rather than fixed here.
	declared := map[string]string{
		"CheckDocument": "finding — /pending 458.",
		"(Record).Hops": "finding — /pending 443. Deleted once as dead and restored: its only use " +
			"is a stimulus floor in record_test.go requiring a 3-party roster to report 2 hops " +
			"before any hop-mapping assertion runs.",
		"SignatureWidgets": "test-only — the structural half of 'is a signature block actually on " +
			"the page'. Its own doc says the rendered half needs pdf.js and belongs at tier 3, so " +
			"production has nothing to ask it.",

		"Form": "test-support — internal/testpdf exists for tests and Form has 111 callers across " +
			"44 test files. A package whose whole purpose is fixtures cannot have production callers.",

		"(*cidGen).ConnectionIDLen":      "interface — quic-go's ConnectionIDGenerator, dispatched by the library.",
		"(*cidGen).GenerateConnectionID": "interface — quic-go's ConnectionIDGenerator, dispatched by the library.",
		"(*side).SetWriteDeadline":       "interface — net.Conn. Its three siblings on the same type are called; this one is not, and dropping it would stop the type satisfying the interface.",

		"(*Socket).Describe":  "test-only — the multicast interface selection rendered for a human. Read by the discovery tests and by nothing on a running path.",
		"(*Socket).LocalPort": "test-only — the bound port, which only a harness needs to know.",

		"ExchangeBudget":        "test-only — D16's Stage 6 nesting pin is a PROPERTY, and the only thing that can read a property is the guard that asserts it. Exported so internal/server's own deadline tests can ask this package rather than keeping a second copy of the number.",
		"MaxRemoteDecisionWait": "test-only — as ExchangeBudget: the consent window fitting inside the dialer's budget is asserted, not computed.",
		"ReceiveArrivalLag":     "test-only — as ExchangeBudget. Its own doc names the test that asserts the population it sums.",

		"(*Server).IdleExitCancels": "test-only — D4's two counters, kept apart deliberately. A seam instrument whose declared reader is tier 1.",
		"(*Server).Ping":            "test-only — a rendezvous liveness probe used by the DHT tests. The one non-test mention of it is a comment in its own file.",
		"RemoveAttachment":          "test-only — the inverse of Embed, for tests asking what a document looks like without the record. Its own doc addresses that caller.",
		"WrapMulti":                 "test-only — multi-recipient SSH sealing. The vault seals to one recipient today, so nothing on a running path asks for several.",
	}

	repo, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	type decl struct{ key, pos string }
	var decls []decl
	used := map[string]int{}
	files := 0

	walkErr := filepath.WalkDir(repo, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "web", "test", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", p, perr)
		}
		files++
		rel, _ := filepath.Rel(repo, p)
		declared := map[*ast.Ident]bool{}
		for _, dd := range f.Decls {
			fd, ok := dd.(*ast.FuncDecl)
			if !ok {
				continue
			}
			declared[fd.Name] = true
			if !fd.Name.IsExported() || !strings.HasPrefix(rel, "internal"+string(filepath.Separator)) {
				continue
			}
			decls = append(decls, decl{
				key: funcKey(fd),
				pos: fmt.Sprintf("%s:%d", rel, fset.Position(fd.Name.Pos()).Line),
			})
		}
		// Every identifier EXCEPT the one a FuncDecl declares. Comments and string literals are
		// not in the tree at all, which is the whole reason this is parsed rather than matched.
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && !declared[id] {
				used[id.Name]++
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}

	// The stimulus floor, and it is two separate claims. A walk that parsed nothing, or one whose
	// identifier index came out empty, would report every export in the repo as dead — and a walk
	// that found no exports at all would report none, which is the silent-pass shape.
	if files < 50 {
		t.Fatalf("the walk parsed %d non-test .go files, which is far too few — it is not reading "+
			"this repository and every count below is meaningless", files)
	}
	if len(decls) < 250 {
		t.Fatalf("the walk found %d exported functions under internal/, and this repository has "+
			"well over three hundred — the population is wrong, so an empty report means nothing",
			len(decls))
	}
	if used["Fingerprint"] == 0 {
		t.Fatal("the identifier index does not contain `Fingerprint`, which internal/sign declares " +
			"and a dozen packages call — so the index is empty or unpopulated, and every export " +
			"would report as uncalled")
	}

	var undeclared []string
	seen := map[string]bool{}
	for _, d := range decls {
		if used[lastSegment(d.key)] != 0 {
			continue
		}
		seen[d.key] = true
		if _, ok := declared[d.key]; ok {
			continue
		}
		undeclared = append(undeclared, fmt.Sprintf("%s  (%s)", d.key, d.pos))
	}
	sort.Strings(undeclared)
	if len(undeclared) > 0 {
		t.Errorf("%d exported function(s) under internal/ have no production caller and no "+
			"declared reason (/pending 445). Either give each one a caller, delete it, or add it "+
			"to `declared` above with the reason it is not dead — `interface`, `test-support`, "+
			"`test-only` or `finding` with its pending number. Look one level down before "+
			"deciding: a function called only by another dead one is reported and its callee is "+
			"not.\n  %s", len(undeclared), strings.Join(undeclared, "\n  "))
	}

	// And the other direction, or the map rots into a list of functions that are fine now. This is
	// the half that made `KNOWN_UNPINNED`-style lists worth keeping in this repo.
	var stale []string
	for k := range declared {
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("%d exemption(s) name a function this scan no longer reports as uncalled — it "+
			"gained a caller, or it was renamed or deleted. Remove the row: an exemption for a "+
			"function that is fine is a reason nobody can check.\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}

// funcKey renders a declaration the way the exemption map spells it: `Name` for a function,
// `(Recv).Name` for a method, so two methods of the same name on different types stay distinct.
func funcKey(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	return "(" + exprName(fd.Recv.List[0].Type) + ")." + fd.Name.Name
}

// lastSegment is the bare identifier a key ends in — what the index counts.
func lastSegment(key string) string {
	if i := strings.LastIndex(key, "."); i >= 0 {
		return key[i+1:]
	}
	return key
}

// exprName renders a receiver type as it is written, pointers included.
func exprName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return "*" + exprName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // a generic receiver
		return exprName(t.X)
	}
	return "?"
}
