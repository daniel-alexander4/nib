package nib

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	// The exemption map, and every row carries the reason it is not a defect. The FIVE prefixes are
	// different claims, not a style: `interface` means the call site cannot exist textually,
	// `test-support` means the function is exported FOR tests, `test-only` means production really
	// does not call it and that is a judgement someone made, `finding` means it IS the defect,
	// recorded under a pending item rather than fixed here, and `gated` means production does not
	// call it YET.
	//
	// **`gated` is the newest and the only one that expires**, added at `PLAN-accessibility.md`
	// P05.S01. It exists because a plan can legitimately build a capability one slice before the
	// slice that consumes it — and because this repo has already paid for the alternative reading:
	// `/pending 442` deleted exported crypto that nothing called, which had taken a fix nobody ran
	// plus a standing exemption row that outlived its reason by months.
	//
	// So a `gated` row **must name the plan coordinate that will call it**, and
	// `TestNoGatedExemptionOutlivesItsCoordinate` fails once that coordinate is marked done. The
	// exemption retires itself instead of waiting to be noticed.
	declared := map[string]string{
		// **The `gated` rows that were here retired themselves**, which is the mechanism working on
		// the first coordinate it was written for. P05.S01 built the content-stream walker with
		// five rows gated on P05.S04; P05.S04 shipped the wrapping emitter, `Tokenize`, `NewEdit`,
		// `InsertBefore` and `Apply` gained production callers, and
		// `TestNoGatedExemptionOutlivesItsCoordinate` failed the moment the slice was marked done —
		// before the commit, because the marker is written first.
		//
		// Two are left, and they are `test-support` rather than `gated`: nothing schedules a caller
		// for them, so a coordinate would be a date nobody is keeping.
		// **`TagAuthored` is unwired BY DECISION, and this row is what keeps that a decision.**
		// P05.S05 built it and deliberately did not point it at the authoring doors: a `/Div`-per-
		// page tree asserts structure carrying none of the distinctions a reader navigates by, and
		// ADR-031's asymmetry says that can hand a user less than the untagged document. P06 wires
		// it with real structure from mdpdf's AST — and this row fails the day P06 is marked done,
		// so the decision cannot quietly become an omission.
		"TagAuthored": "gated — PLAN-accessibility.md P06.",

		// D4's whole point is that *the user is told* which tier produced the tree they are looking
		// at, and nothing shows them yet. P07's exit criterion puts a conformance report in front of
		// a user from the UI and the CLI, which is where a provenance line belongs — so this is
		// gated there rather than left as a reader with no reader.
		"StructureSource": "gated — PLAN-accessibility.md P07.",

		"WriteTokens": "test-support — the round-trip law's entry point. `Edit.Apply` with no " +
			"edits returns the original slice WITHOUT touching tokens, so it cannot prove the " +
			"tokenization is total and faithful; writing the tokens back is what does, and that " +
			"property is what every later slice's correctness rests on.",
		"(Token).Describe": "test-support — renders a token for a failure message. Named " +
			"`Describe` and not `String` on purpose: a method called `String` that takes an " +
			"argument is not a fmt.Stringer, so `%v` on a Token would silently print the struct.",
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

// gatedRow matches a `gated` exemption and captures the plan file and coordinate it names.
var gatedRow = regexp.MustCompile(`^gated — (PLAN-[A-Za-z0-9-]+\.md) (P\d+(?:\.S\d+)?)\.`)

// TestNoGatedExemptionOutlivesItsCoordinate — `PLAN-accessibility.md` P05.S01.
//
// A `gated` exemption in `zerocaller_test.go` says *production does not call this YET, and here is
// the plan coordinate that will*. That is a legitimate claim exactly once: while the coordinate is
// still open. The moment it ships, the row is either wrong — the caller exists and the sibling
// staleness check will say so — or it is right and the plan did not do what it said, which is the
// more interesting failure and the one nothing else would report.
//
// **This is the check `/pending 442` cost the repo for want of.** Dead exported crypto sat behind a
// standing exemption row until somebody read it; the row itself could not tell anyone it had
// expired. A gate with no caller is this repo's own name for a rule enforced only by a sentence
// saying it is required.
func TestNoGatedExemptionOutlivesItsCoordinate(t *testing.T) {
	// Re-run the guard's own map by calling it indirectly is not possible, so the rows are read
	// from this file's source — the one place they are written — rather than duplicated here.
	src, err := os.ReadFile("zerocaller_test.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`"gated — (PLAN-[A-Za-z0-9-]+\.md) (P\d+(?:\.S\d+)?)\."`).
		FindAllStringSubmatch(string(src), -1)
	if len(rows) == 0 {
		t.Skip("SKIP (not a pass): no `gated` exemption is declared, so this has nothing to check")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		planFile, coord := r[1], r[2]
		key := planFile + " " + coord
		if seen[key] {
			continue
		}
		seen[key] = true
		plan, rerr := os.ReadFile(planFile)
		if rerr != nil {
			t.Errorf("a gated exemption names %s, which does not exist: %v", planFile, rerr)
			continue
		}
		// The coordinate's heading, and whether it carries a done marker.
		head := regexp.MustCompile(`(?m)^#+ ` + regexp.QuoteMeta(coord) + `\b.*$`).Find(plan)
		if head == nil {
			t.Errorf("a gated exemption names %s %s, which is not a heading in that plan — the "+
				"coordinate was renamed or never existed, so the exemption points at nothing",
				planFile, coord)
			continue
		}
		if bytes.Contains(head, []byte("*(done")) {
			t.Errorf("%s %s has SHIPPED, and exemptions are still gated on it:\n  %s\n\t"+
				"Either the slice built the caller — in which case remove those rows — or it did "+
				"not, and exported code nothing calls has just been released behind a reason that "+
				"expired. /pending 442 is what the second one costs.",
				planFile, coord, head)
		}
	}
	t.Logf("%d gated coordinate(s) checked, all still open", len(seen))
}
