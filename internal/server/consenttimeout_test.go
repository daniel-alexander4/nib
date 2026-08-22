package server

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryConsentGateNamesATimeoutAsATimeout.
//
// # The rule
//
// A gate that runs out its `sessionConsentTimeout` must say **nobody answered**, and must not
// say **the user refused**. They are different facts about a person, they are reported to a
// peer over the wire, and the peer's user is shown one of them.
//
// # Why a source guard rather than a behavioural one
//
// `sessionConsentTimeout` is five minutes, so driving the branch takes five minutes per gate
// — which is why none of the three was ever driven, and why two of them shipped returning a
// bare `(false, nil)`: indistinguishable from a decline before any sentinel existed, so the
// peer was sent `ackDeclined` and shown `{"declined": true}` about somebody who had merely
// walked away from the machine.
//
// The verification gate got this right from the start and wrote down why:
// *"ErrVerificationTimedOut: nobody answered. Distinct from declining, because it means
// something different to the user and to whoever reads the log."* Three sites, one rule, and
// one of them correct is exactly the shape ADR-009 exists for — so the guard asks the
// question of every site it can find rather than of the two somebody remembered.
//
// The wire behaviour these sentinels produce IS driven, in `internal/p2p`:
// `TestARefusalTellsThePeerWHICHRefusalItWas` runs all four refusals over both transports.
// This guard covers the half that test cannot see — that a real gate returns the sentinel the
// fake confirmer is standing in for.
func TestEveryConsentGateNamesATimeoutAsATimeout(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	gates := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, perr := parser.ParseFile(fset, f, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			cc, ok := n.(*ast.CommClause)
			if !ok || cc.Comm == nil {
				return true
			}
			var comm strings.Builder
			if perr := printer.Fprint(&comm, fset, cc.Comm); perr != nil {
				t.Fatal(perr)
			}
			if !strings.Contains(comm.String(), "sessionConsentTimeout") {
				return true
			}
			gates++
			var body strings.Builder
			for _, st := range cc.Body {
				if perr := printer.Fprint(&body, fset, st); perr != nil {
					t.Fatal(perr)
				}
				body.WriteString("\n")
			}
			pos := fset.Position(cc.Pos())
			// The branch must name the outcome. `TimedOut` is the naming convention both
			// sentinels already follow (`p2p.ErrConsentTimedOut`,
			// `p2p.ErrVerificationTimedOut`), and matching on it rather than on either
			// specific name lets a third gate use whichever fits its flow.
			if !strings.Contains(body.String(), "TimedOut") {
				t.Errorf("%s:%d — a consent gate's timeout branch returns no TimedOut "+
					"sentinel:\n\t%s\nA timeout that returns the same value as a refusal "+
					"tells the peer's user that a person declined when nobody was at the "+
					"machine. It crosses the wire, so it is a false statement about somebody "+
					"else's decision.",
					filepath.Base(pos.Filename), pos.Line,
					strings.ReplaceAll(strings.TrimSpace(body.String()), "\n", "\n\t"))
			}
			// And it must not ALSO reach for a decline: a branch returning both is one that
			// has been half-converted.
			if strings.Contains(body.String(), "Declined") {
				t.Errorf("%s:%d — a consent gate's timeout branch names a DECLINE:\n\t%s",
					filepath.Base(pos.Filename), pos.Line, strings.TrimSpace(body.String()))
			}
			return true
		})
	}
	// STIMULUS. Without this the loop above passes on a glob that matched nothing, on a
	// renamed constant, and on a refactor that moved every gate out of this package —
	// three ways to report full coverage of zero sites.
	if gates < 3 {
		t.Fatalf("found %d consent gates; this package has at least three (the co-signature "+
			"confirmer, the transfer accepter, and the spoken verification). The discovery is "+
			"broken, so every check above passed on nothing.", gates)
	}
	t.Logf("checked %d consent gates", gates)
}
