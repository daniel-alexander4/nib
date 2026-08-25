package p2p

import (
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
