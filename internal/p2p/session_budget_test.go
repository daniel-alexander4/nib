package p2p

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestSessionBudgetIsTheSumOfWhatInitiateArms — the arithmetic, stated once.
func TestSessionBudgetIsTheSumOfWhatInitiateArms(t *testing.T) {
	if want := 2*exchangeDeadline + remoteDecisionDeadline; SessionBudget() != want {
		t.Errorf("SessionBudget() = %s, want %s", SessionBudget(), want)
	}
	// And it is materially larger than one phase — the specific mistake it exists to stop a
	// caller making. Asserted as a RELATION, not a literal, so a constant change does not
	// force a fixture edit while still catching a caller that reaches for the phase budget.
	if SessionBudget() <= ExchangeBudget() {
		t.Errorf("SessionBudget() %s is not larger than one ExchangeBudget() %s — either the "+
			"deadlines collapsed or this guard has stopped meaning anything",
			SessionBudget(), ExchangeBudget())
	}
	if got, want := SessionBudget(), 24*time.Minute; got != want {
		t.Errorf("SessionBudget() = %s, want %s — if the constants moved deliberately, this "+
			"literal moves with them AND internal/server's ceremonyHopBudget doc is re-read, "+
			"because the plan's per-hop figure is quoted in P07's C20", got, want)
	}
}

// TestSessionBudgetCountsEveryDeadlineInitiateArms is the guard that keeps the sum honest.
//
// SessionBudget adds up three arms. Nothing in the type system says Initiate arms three — so
// a fourth `SetDeadline` added later would silently make the budget an under-estimate, which
// is the exact direction that hurts: a caller reserves too little and asks somebody to consent
// to a signature on a proceeding that has already ended.
//
// A source scan rather than a runtime observation, because the arms are wall-clock deadlines
// on a real connection and there is nothing to count at run time without a session.
func TestSessionBudgetCountsEveryDeadlineInitiateArms(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	body := funcBody(t, string(src), "func Initiate(")
	arms := regexp.MustCompile(`SetDeadline\(`).FindAllString(body, -1)
	// Stimulus first: if the scan found NOTHING, it is reading the wrong function and every
	// count below is a comparison between two zeros.
	if len(arms) == 0 {
		t.Fatal("no SetDeadline call found in Initiate — this scan is not reading the function " +
			"it names, so its count means nothing")
	}
	const want = 3
	if len(arms) != want {
		t.Errorf("Initiate arms %d deadlines and SessionBudget adds up %d. A budget smaller than "+
			"what the session can actually spend is the defect this exists to prevent — update "+
			"SessionBudget and this count together.", len(arms), want)
	}
}

// TestDeliveryLegBudgetCountsEveryDeadlineSendDocumentArms is the same guard for the other
// function, and it is separate ON PURPOSE (P08.S05b).
//
// `DeliveryLegBudget` equals `SessionBudget` today only because `SendDocument` and `Initiate`
// happen to arm the same three deadlines. Sharing one guard would tie a claim about the transfer
// path to a count taken over the co-sign path, so a fourth arm added to either would be graded
// against the other's number — which is the duplicate derivation `SessionBudget`'s own doc grades
// critical, one level up. Two functions, two scans, and they are expected to diverge at P08.S05d.
func TestDeliveryLegBudgetCountsEveryDeadlineSendDocumentArms(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	// **Counted from the AST, not from the source text (P08.S05d).** It used to regex
	// `SetDeadline\(` over the function's bytes, and the first comment written ABOUT this guard —
	// inside `SendDocument`, naming the call it counts — pushed the count from 3 to 4 and failed
	// it. A scan a comment can satisfy is a scan a comment can also break, and this repo has the
	// mirror of that on record: *"a scan satisfied by prose that merely names the expression is
	// how a freeze guard once read its own explanation as proof of coverage"* (v1.117.155). Here
	// it read an explanation as an ARM. Same defect, opposite sign.
	fset := token.NewFileSet()
	f, perr := parser.ParseFile(fset, "session.go", src, 0)
	if perr != nil {
		t.Fatal(perr)
	}
	arms := 0
	var seen bool
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "SendDocument" || fn.Body == nil {
			continue
		}
		seen = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetDeadline" {
				arms++
			}
			return true
		})
	}
	// STIMULUS, two directions: the function was found, and it really arms something. Either
	// missing makes "the count is 3" true of a scan that read nothing.
	if !seen {
		t.Fatal("SendDocument not found in session.go — this guard is pinned to a function that " +
			"no longer exists under that name")
	}
	if arms == 0 {
		t.Fatal("no SetDeadline call found in SendDocument — this scan is not reading the " +
			"function it names, so its count means nothing")
	}
	const want = 3
	if arms != want {
		t.Errorf("SendDocument arms %d deadlines and DeliveryLegBudget adds up %d. A delivery term "+
			"smaller than what the leg can actually spend means Convene admits a ceremony whose "+
			"round cannot finish inside the deadline the user set.", arms, want)
	}
	// **And the third arm goes through the door, whichever figure it picks (P08.S05d).** The count
	// above cannot see that `remoteDecisionFor` is what chooses between 12m and 2m; a site
	// inlining either constant would keep the count at 3 and put the budget and the arm back on
	// two separate derivations, which is the defect DeliveryLegBudget was written to close.
	body := funcBody(t, string(src), "func SendDocument(")
	if !strings.Contains(stripComments(body), "remoteDecisionFor(") {
		t.Error("SendDocument's third arm does not call remoteDecisionFor — the budget and the " +
			"deadline are then derived separately and can diverge silently")
	}
}

// TestDeliveryLegBudgetIsNotSmallerThanTheLegCanSpend — the arithmetic half, now over BOTH arms.
//
// The figure this replaced was `bootstrapBudget + connectDeadline + postConsentDeadline`, on the
// reasoning that a delivery leg runs no local human gate. That confused expected latency with
// armed budget: `SendDocument` armed `remoteDecisionDeadline` regardless of the verifier, and
// `ReceiveDocument`'s Accepter was a live human gate on the production path. It was short by 22
// minutes per leg.
//
// # P08.S05d earned the shrink, and this guard is what makes it earned rather than asserted
//
// The refuted figure and the shrunk one are NOT the same number and that is the whole point. The
// refuted one dropped the third arm to `postConsentDeadline` while `SendDocument` still armed
// `remoteDecisionDeadline` — a budget smaller than the code spends. The shrunk one drops the ARM
// too, through `remoteDecisionFor`, so the two move together by construction. So this asserts both
// arms of `PeerGates` against their own arithmetic, and asserts they DIFFER: a `remoteDecisionFor`
// that ignored its argument would satisfy either arm alone.
func TestDeliveryLegBudgetIsNotSmallerThanTheLegCanSpend(t *testing.T) {
	// ── The interactive arm is unchanged, and that is half the property ──────────────────────
	if got, want := DeliveryLegBudget(PeerGatesHuman), 2*exchangeDeadline+remoteDecisionDeadline; got != want {
		t.Errorf("DeliveryLegBudget(PeerGatesHuman) = %s, want %s (the three arms SendDocument "+
			"takes when a person is on the far side)", got, want)
	}
	if got, want := DeliveryLegBudget(PeerGatesHuman), 24*time.Minute; got != want {
		t.Errorf("DeliveryLegBudget(PeerGatesHuman) = %s, want %s. Change this literal "+
			"deliberately, not as a consequence of moving a constant.", got, want)
	}

	// ── The unattended arm, which is what the round reserves ─────────────────────────────────
	if got, want := DeliveryLegBudget(PeerGatesUnattended), 2*exchangeDeadline+postConsentDeadline; got != want {
		t.Errorf("DeliveryLegBudget(PeerGatesUnattended) = %s, want %s", got, want)
	}
	if got, want := DeliveryLegBudget(PeerGatesUnattended), 14*time.Minute; got != want {
		t.Errorf("DeliveryLegBudget(PeerGatesUnattended) = %s, want %s. Change this literal "+
			"deliberately, with the reservation it feeds — internal/server's "+
			"ceremonyDeliveryLegBudget and Convene's DeliveryBudget.", got, want)
	}

	// ── THE DISCRIMINATOR. Without it a remoteDecisionFor that ignored its argument would pass
	// whichever arm happened to match, and the two literals above would be two spellings of one
	// number. This is the assertion the refuted figure could not have made.
	if DeliveryLegBudget(PeerGatesHuman) == DeliveryLegBudget(PeerGatesUnattended) {
		t.Error("both PeerGates arms budget the same, so remoteDecisionFor is not reading its " +
			"argument — an interactive send would then reserve the unattended figure, ten " +
			"minutes less than it can spend")
	}
	if DeliveryLegBudget(PeerGatesUnattended) >= DeliveryLegBudget(PeerGatesHuman) {
		t.Error("the unattended leg budgets at least as much as the attended one, which inverts " +
			"the reason for the distinction")
	}

	// **The refuted figure, named by its own arithmetic so a shrink has to argue past it.** The
	// slice was firmed at `bootstrapBudget + connectDeadline + postConsentDeadline`; the p2p half
	// of that is `postConsentDeadline` ALONE — not the 14m below it, which keeps both
	// `exchangeDeadline` arms. Naming it separately is what stops "we shrank it" from being read
	// as "the refuted figure was right after all".
	for _, g := range []PeerGates{PeerGatesHuman, PeerGatesUnattended} {
		if refuted := postConsentDeadline; DeliveryLegBudget(g) == refuted {
			t.Errorf("DeliveryLegBudget(%v) = %s — the refuted figure's p2p half. SendDocument "+
				"arms two exchangeDeadlines whatever the far side's gates do.", g, refuted)
		}
	}
}

// funcBody returns the source of the function whose signature starts with prefix, from its
// opening brace to the matching close.
func funcBody(t *testing.T, src, prefix string) string {
	t.Helper()
	i := strings.Index(src, prefix)
	if i < 0 {
		t.Fatalf("cannot find %q in the source", prefix)
	}
	rest := src[i:]
	open := strings.Index(rest, "{")
	if open < 0 {
		t.Fatalf("no opening brace after %q", prefix)
	}
	depth := 0
	for j := open; j < len(rest); j++ {
		switch rest[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[open : j+1]
			}
		}
	}
	t.Fatalf("unbalanced braces after %q", prefix)
	return ""
}

// stripComments removes // and /* */ comments so a scan cannot be satisfied — or broken — by prose
// that merely names the expression it looks for. P08.S05d added it after a comment written ABOUT
// the arm count changed the count.
func stripComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "//") {
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				break
			}
			i += j
			continue
		}
		if strings.HasPrefix(src[i:], "/*") {
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				break
			}
			i += 2 + j + 2
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}
