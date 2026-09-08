package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"nib/internal/p2p"
)

// TestAnArmServesOnlyTheRolesItsModeAllows — ADR-028's policy half.
//
// # The property, and why it is a check rather than an assignment
//
// `mode` is what an arm was opened to do; the wire's role is what a dialer is asking for. The
// obvious simplification is `mode = role` — let the dial decide — and it is wrong in the one
// direction that matters: a listener the user armed for a one-way transfer would co-sign because
// a peer asked it to. The wire never overrides the arm.
//
// The delivery arm is the exception and it is the whole of /pending 385: it serves BOTH, because
// a party that has committed holds no other arm and the resumed hop has nowhere else to land.
func TestAnArmServesOnlyTheRolesItsModeAllows(t *testing.T) {
	cases := []struct {
		mode  string
		role  p2p.Role
		serve bool
		why   string
	}{
		{sessionModeReceive, p2p.RoleTransfer, true, "a receive arm's whole purpose"},
		{sessionModeReceive, p2p.RoleCoSign, false,
			"a listener armed for a transfer must never co-sign because a peer asked it to"},
		{sessionModeCoSign, p2p.RoleCoSign, true, "a co-sign arm's whole purpose"},
		{sessionModeCoSign, p2p.RoleTransfer, false,
			"an arm armed to co-sign must not silently accept a document instead"},
		{"", p2p.RoleCoSign, true,
			"the empty mode is co-sign — what older clients send, an accepted spelling"},
		{"", p2p.RoleTransfer, false, "and it is not a wildcard"},
		{sessionModeDelivery, p2p.RoleTransfer, true, "the delivery leg it was armed for"},
		{sessionModeDelivery, p2p.RoleCoSign, true,
			"AND the resumed hop, which is /pending 385: a party that has committed has a " +
				"record, so the hop sweep skips it and this is the only arm it holds"},
	}
	for _, c := range cases {
		if got := armServesRole(c.mode, c.role); got != c.serve {
			t.Errorf("armServesRole(%q, %s) = %v, want %v — %s", c.mode, c.role, got, c.serve, c.why)
		}
	}

	// An unknown mode serves NOTHING. Failing closed matters more here than anywhere: a mode this
	// build does not know reaching a wildcard would arm an exchange nobody chose.
	for _, role := range []p2p.Role{p2p.RoleCoSign, p2p.RoleTransfer} {
		if armServesRole("something-else", role) {
			t.Errorf("an unknown arm mode serves %s. It must fail closed — `checkSessionMode` "+
				"refuses an unknown mode rather than defaulting, and this is the same rule at "+
				"the point the mode is used", role)
		}
	}

	// STIMULUS: the table really exercised both answers. A predicate stuck at one value would
	// satisfy half of it and nothing would say which half.
	var yes, no int
	for _, c := range cases {
		if c.serve {
			yes++
		} else {
			no++
		}
	}
	if yes == 0 || no == 0 {
		t.Fatalf("the table asserts %d serves and %d refusals — a predicate returning one "+
			"constant would pass it", yes, no)
	}
}

// TestTheDeliveryArmSendsACoSignThroughTheREALGates — ADR-028's soundness half, and the one that
// would be a hole rather than a bug if it were wrong.
//
// `deliverOneLeg`'s own header says the unattended gates are sound on a delivery leg **and only
// there**: the two parties met at their hop and answered those words about that pin. A resumed
// hop is that hop being taken for the first time, so it owes the human spoken check and the human
// consent. If the co-sign branch reached `autoVerifier`, ADR-028 would have removed the spoken
// check from a signature — silently, and on the recovery path specifically.
//
// **Structural, because the behavioural version needs two processes.** Asserting it at tier 1
// would need a real dial into a real delivery arm; tier 4d is where that runs. What is checkable
// here is the routing, and the routing is the property: the co-sign branch goes through
// `serveOneSession`, which is `p2p.Receive`'s one production call site and builds the real
// `sessionConfirmer` and `sessionVerifier`. `TestTheUnattendedGatesHaveOneDoor` holds the other
// half — that the auto types stay reachable only from `deliverOneLeg`.
func TestTheDeliveryArmSendsACoSignThroughTheREALGates(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Name.Name != "armForDelivery" {
					continue
				}
				var sb strings.Builder
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						sb.WriteString(id.Name)
						sb.WriteString(" ")
					}
					if sel, ok := n.(*ast.SelectorExpr); ok {
						sb.WriteString(sel.Sel.Name)
						sb.WriteString(" ")
					}
					return true
				})
				body = sb.String()
			}
		}
	}
	// STIMULUS: the function was found and read. An empty body agrees with every check below.
	if len(body) < 200 {
		t.Fatalf("armForDelivery's body did not parse (%d chars) — this guard is reading nothing",
			len(body))
	}
	if !strings.Contains(body, "ReadRole") {
		t.Error("the delivery arm never reads the dial's role, so it cannot tell a delivery leg " +
			"from a resumed hop — which is /pending 385: the arm answers the hop with a gate " +
			"that auto-confirms and an exchange that cannot serve a stored contribution")
	}
	if !strings.Contains(body, "serveOneSession") {
		t.Error("the delivery arm's co-sign branch does not route through serveOneSession. That " +
			"is `p2p.Receive`'s one production call site and the only place the REAL Confirmer " +
			"and Verifier are built; a second path here would be a co-signature taken without " +
			"the human spoken check, on the recovery path specifically")
	}
	if strings.Contains(body, "autoVerifier") {
		t.Error("the delivery arm names autoVerifier directly. The auto gates are sound on a " +
			"delivery LEG and only there (deliverOneLeg's header); reaching one from the branch " +
			"that serves a resumed hop would remove the spoken check from a signature")
	}
	if !strings.Contains(body, "sessionModeDelivery") {
		t.Error("the delivery arm does not pass sessionModeDelivery, so armServesRole cannot " +
			"know this is the one arm permitted to serve both roles")
	}
}
